import test from 'node:test'
import assert from 'node:assert/strict'
import { createAppRequest, appResponse, workspaceLaunch, workspaceOrigin } from './workspace.js'
import { createWorkspaceAPI } from '../src/workspace/services.js'
import { readSummaryPages } from '../src/api/summary.js'
const project='01989abc-def0-7000-8000-000000000001'
const chapter='01989abc-def0-7000-8000-000000000002'
const page='01989abc-def0-7000-8000-000000000003'
const wrap=data=>({_meta:{'lumi/app_response':{success:true,data}},structuredContent:{success:true,data:{state:'available',channel:'_meta'}},content:[{type:'text',text:'UI response'}]})
test('shared API factories use the host bridge for both reads and user-submitted revisions',async()=>{
 const calls=[]
 const api=createWorkspaceAPI(createAppRequest({call:async(name,args)=>{calls.push({name,args});return wrap({uuid:page,revision:4})}},project))
 await api.getComicSection(project,chapter,page)
 await api.updateComicSection(project,chapter,page,{title:'Changed',expected_revision:3})
 assert.equal(calls[0].name,'read_app_resource');assert.equal(calls[0].args.resource,`chapters/${chapter}/comic-sections/${page}`)
 assert.equal(calls[1].name,'write_app_resource');assert.deepEqual(calls[1].args.body,{title:'Changed',expected_revision:3})
 assert.equal(calls[1].args.method,'PATCH');assert.equal(calls.some(c=>c.name.startsWith('review_')),false)
})
test('transport rejects project switching, paths, missing metadata and unsupported paid/environment operations',async()=>{
 let calls=0;const request=createAppRequest({call:async()=>{calls++;return wrap({})}},project)
 for(const path of ['/tmp/project.sqlite','http://127.0.0.1:1/','/api/v1/projects/'+chapter,`/api/v1/projects/${project}/../${chapter}`,`/api/v1/projects/${project}/%2e%2e/`])await assert.rejects(request(path))
 await assert.rejects(request(`/api/v1/projects/${project}/chapters/${chapter}/comic-image-generation-batches`,{method:'POST',body:'{}'}),e=>e.code==='workspace_capability_unavailable')
 await assert.rejects(request(`/api/v1/projects/${project}/asset-uploads`,{method:'POST',body:new FormData()}))
 assert.equal(calls,0)
 assert.throws(()=>appResponse({structuredContent:{success:true,data:{story_md:'do not fall back'}}}),e=>e.code==='app_meta_missing')
 assert.throws(()=>appResponse({_meta:{'lumi/app_response':{success:false,data:null,error:{code:'production_conflict',message:'Conflict'}}}}),e=>e.status===409)
})
test('v3 and historical launch identities preserve chapter scope and runtime Web origin',()=>{
 const launch={contract_version:3,view:'book',project_uuid:project,target_uuid:project,revision:0,title:'Book',summary:'Short',open_url:`http://127.0.0.1:49123/projects/${project}/`,request:{name:'review_book',arguments:{}}}
 assert.equal(workspaceLaunch({structuredContent:{success:true,data:launch}}).path,`/projects/${project}/`)
 const legacy={...launch,contract_version:undefined,scope:'chapter',request:{name:'review_book',arguments:{chapter_uuid:chapter}}}
 assert.equal(workspaceLaunch({structuredContent:{success:true,data:legacy}}).path,`/projects/${project}/chapters/${chapter}`)
 assert.throws(()=>workspaceLaunch({structuredContent:{success:true,data:{...legacy,request:{name:'review_book',arguments:{}}}}}),/未扩大为整本/)
 const single={...launch,view:'page',target_uuid:page,request:{name:'review_page',arguments:{chapter_uuid:chapter,page_uuid:page}}}
 assert.equal(workspaceLaunch({structuredContent:{success:true,data:single}}).origin,'http://127.0.0.1:49123')
 assert.throws(()=>workspaceLaunch({structuredContent:{success:true,data:{...single,revision:-1}}}))
 assert.throws(()=>workspaceLaunch({structuredContent:{success:true,data:{...single,target_uuid:chapter}}}),/目标/)
})
test('all review launches and historical reader results open normal pages at the same scope',()=>{
 const base=`/projects/${project}`
 const launch={contract_version:3,view:'book',project_uuid:project,target_uuid:project,revision:4,title:'Book',open_url:`http://127.0.0.1:49123${base}/`,request:{name:'review_book',arguments:{}}}
 const open=data=>workspaceLaunch({structuredContent:{success:true,data}})
 for(const [view,args,target,path] of [
  ['book',{},project,`${base}/`],
  ['chapters',{},project,`${base}/chapters`],
  ['chapter',{chapter_uuid:chapter},chapter,`${base}/chapters/${chapter}`],
  ['page',{chapter_uuid:chapter,page_uuid:page},page,`${base}/chapters/${chapter}/sections/${page}`],
 ]) {
  assert.equal(open({...launch,view,target_uuid:target,request:{name:`review_${view}`,arguments:args}}).path,path)
 }
 for(const view of ['book','reader']) for(const contract_version of [undefined,2,3]) for(const scoped of [false,true]) {
  const args={mode:'preview',...(scoped?{chapter_uuid:chapter}:{})}
  const result=open({...launch,view,contract_version,scope:scoped?'chapter':'book',open_url:launch.open_url+(scoped?`chapters/${chapter}/preview`:'preview'),request:{name:'review_book',arguments:args}})
  assert.equal(result.path,scoped?`${base}/chapters/${chapter}`:`${base}/`)
  assert.equal(result.open_url,`http://127.0.0.1:49123${result.path}`)
  assert.equal(result.view,scoped?'chapter':'book')
  assert.equal(result.target_uuid,scoped?chapter:project)
  assert.equal(result.request.arguments.mode,undefined)
  assert.equal(result.request.name,scoped?'review_chapter':'review_book')
 }
 const old={...launch,view:'reader',scope:'chapter',request:{name:'review_book',arguments:{chapter_uuid:chapter}}}
 for(const invalid of [
  {...old,chapter_uuid:page},
  {...old,scope:'book'},
  {...old,request:{name:'review_book',arguments:{}}},
  {...old,request:{name:'review_book',arguments:{chapter_uuid:'invalid'}}},
 ]) assert.throws(()=>open(invalid))
 const textCandidates=open({...launch,view:'candidates',request:{name:'review_candidates',arguments:{kind:'text',candidates:[{title:'A',text:'A'},{title:'B',text:'B'}]}}})
 assert.equal(textCandidates.open_url,launch.open_url,'text candidates retain a real Web fallback')
})
test('summary navigation drains cursor pages without prose and rejects changed or incomplete snapshots',async()=>{
 const calls=[]
 const result=await readSummaryPages(async(path)=>{calls.push(path);const second=path.includes('after=2');return{items:second?[{uuid:page}]:[{uuid:project},{uuid:chapter}],total:3,snapshot:'a'.repeat(64),cursor_pagination:{next_cursor:second?'3':'2',has_more:!second}}},'/chapters')
 assert.equal(result.items.length,3);assert.equal(calls.length,2);assert.ok(calls.every(p=>p.includes('view=summary')))
 await assert.rejects(readSummaryPages(async()=>({items:[],total:2,snapshot:'a'.repeat(64),cursor_pagination:{next_cursor:'0',has_more:false}}),'/chapters'),/未读取完整/)
})

test('user file bytes travel only as a host attachment and read-only connections reject writes',async()=>{
 const calls=[];const bridge={call:async(...call)=>{calls.push(call);return wrap({uuid:page})}}
 const file=new File(['test-image-bytes'],'fixture.png',{type:'image/png'})
 const form=new FormData();form.append('purpose','comic_section_image');form.append('file',file)
 await createAppRequest(bridge,project)(`/api/v1/projects/${project}/asset-uploads`,{method:'POST',body:form})
 const [name,args,options]=calls[0]
 assert.equal(options.meta['lumi/expected_project_uuid'],project)
 assert.equal(name,'write_app_resource');assert.equal(args.body.file_name,'fixture.png')
 assert.equal(JSON.stringify(args).includes('test-image-bytes'),false)
 assert.equal(atob(options.meta['lumi/upload'].data_base64),'test-image-bytes')
 await assert.rejects(createAppRequest(bridge,project,{canWrite:false})(`/api/v1/projects/${project}`,{method:'PATCH',body:'{}'}),e=>e.code==='project_read_only')
 assert.equal(calls.length,1)
})

test('calibrated project origin follows a restarted CLI port without changing project scope',()=>{
 const launch={project_uuid:project,origin:'http://127.0.0.1:49123'}
 assert.equal(workspaceOrigin({open_url:`http://127.0.0.1:49234/projects/${project}/`},launch),'http://127.0.0.1:49234')
 assert.equal(workspaceOrigin({open_url:`http://127.0.0.1:49234/projects/${chapter}/`},launch),launch.origin)
 assert.equal(workspaceOrigin({open_url:`https://evil.test/projects/${project}/`},launch),launch.origin)
})
