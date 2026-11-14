// Browser integration fixture: real bundled App and default Web runtime,
// synthetic host acknowledgements and REST facts; never a Codex-host claim.
import { createServer } from 'vite'
import react from '@vitejs/plugin-react'
import { readFile } from 'node:fs/promises'
const uuid = n => `01989abc-def0-7000-8000-${String(n).padStart(12,'0')}`
const project=uuid(1), chapter=uuid(2), page=uuid(3), asset=uuid(4)
const book={uuid:project,name:'引用验收作品',revision:1,picture_book:{format:'classic_picture_book'}}
const ch={uuid:chapter,title:'星光启程',chapter_code:'vol01.ch01',revision:1}
const section={uuid:page,title:'第一个场景',section_no:1,page_role:'body',revision:1,description_md:'画面方向',current_storyboard:{uuid:uuid(5),content_md:'保存的分镜正文',version_no:1},premise_assets:[]}
const setting={uuid:asset,title:'小狐狸',asset_type:'character',summary:'红色的小狐狸',revision:1,tags:[]}
function resource(path) {
  if(path==='project') return book
  if(path==='chapters') return {items:[ch]}
  if(path===`chapters/${chapter}`) return ch
  if(path===`chapters/${chapter}/comic-sections`) return {items:[section]}
  if(path===`premise-assets/${asset}`) return setting
  if(path==='premise-assets') return {items:[setting]}
  return {items:[]}
}
const facts={project,chapter,page,asset,book,ch,section,setting}
const host=`<!doctype html><html><meta charset="utf-8"><title>模拟宿主 · 引用验收</title><body style="margin:0"><iframe style="width:100%;height:99vh;border:0" src="/review.html"></iframe><script>
const facts=${JSON.stringify(facts)};
const frame=document.querySelector('iframe');window.exchanges=[];window.mode='ok';window.contexts=[];
window.launch=(view='page')=>{const args=view==='page'?{chapter_uuid:facts.chapter,page_uuid:facts.page}:view==='settings'?{setting_uuid:facts.asset}:view==='candidates'?{kind:'text',candidates:[{title:'候选一',text:'第一份完整候选正文'},{title:'候选二',text:'第二份完整候选正文'},{title:'候选三',text:'第三份完整候选正文'}]}:{};
frame.contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/tool-result',params:{structuredContent:{success:true,data:{view,project_uuid:facts.project,target_uuid:view==='page'?facts.page:view==='settings'?facts.asset:facts.project,revision:1,open_url:location.origin+'/projects/'+facts.project+'/',request:{name:'review_'+view,arguments:args}}}}},'*')};
window.addEventListener('message',async event=>{if(event.source!==frame.contentWindow)return;const m=event.data;if(m.jsonrpc!=='2.0')return;window.exchanges.push(m);let result={};
if(m.method==='ui/initialize')result={protocolVersion:'2026-01-26',hostCapabilities:new URLSearchParams(location.search).has('unsupported')?{}:{updateModelContext:{text:{},structuredContent:{}}}};
else if(m.method==='ui/notifications/initialized'){launch();return}
else if(m.method==='tools/call'){if(m.params.name==='watch_app_changes')return;const response=await fetch('/facts/'+m.params.arguments.resource).then(r=>r.json());result={_meta:{'lumi/app_response':{success:true,data:response}}}}
else if(m.method==='ui/update-model-context'){if(window.mode==='delay')await new Promise(r=>setTimeout(r,500));if(window.mode==='deny'){frame.contentWindow.postMessage({jsonrpc:'2.0',id:m.id,error:{code:'permission_denied',message:'模拟宿主拒绝'}},'*');return}if(window.mode==='timeout')return;window.contexts.push(m.params)}
else if(!m.id)return;
frame.contentWindow.postMessage({jsonrpc:'2.0',id:m.id,result},'*')});
</script></body></html>`
const entry=`import React from 'react';import{createRoot}from'react-dom/client';import{QueryClient,QueryClientProvider}from'@tanstack/react-query';import{MemoryRouter,Routes,Route}from'react-router-dom';import{SimplePageView,SimpleBooksPage,SimpleSettingDetailPage}from'/src/pages/SimpleProjectPages.jsx';import TextCandidates from '/src/workspace/TextCandidates.jsx';import '/mcp-app/workspace.scss';
const project='${project}';const view=new URLSearchParams(location.search).get('view')||'page';const path=view==='chapters'?'/projects/'+project+'/chapters':view==='settings'?'/projects/'+project+'/premise/assets/${asset}':view==='candidates'?'/candidates':'/projects/'+project+'/chapters/${chapter}/sections/${page}';
createRoot(document.getElementById('root')).render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><MemoryRouter initialEntries={[path]}><main className="simple-project-body"><Routes><Route path="/projects/:projectUuid/chapters" element={<SimpleBooksPage projectUuid={project}/>}/><Route path="/projects/:projectUuid/chapters/:chapterUuid/sections/:sectionUuid" element={<SimplePageView projectUuid={project} project={${JSON.stringify(book)}}/>}/><Route path="/projects/:projectUuid/premise/assets/:assetUuid" element={<SimpleSettingDetailPage projectUuid={project}/>}/><Route path="/candidates" element={<TextCandidates candidates={[{title:'候选一',text:'第一份正文'},{title:'候选二',text:'第二份正文'}]}/>}/></Routes></main></MemoryRouter></QueryClientProvider>);`
const server=await createServer({configFile:false,root:new URL('../',import.meta.url).pathname,plugins:[react(),{name:'reference-fixture',resolveId:id=>id==='virtual:reference-web'?'\0reference-web':null,load:id=>id==='\0reference-web'?{code:entry,map:null}:null,transform(code,id){if(id==='\0reference-web')return import('vite').then(({transformWithEsbuild})=>transformWithEsbuild(code,'fixture.jsx',{loader:'jsx',jsx:'automatic'}))},configureServer(server){server.middlewares.use(async(req,res,next)=>{
const url=new URL(req.url,'http://fixture');
if(url.pathname==='/fixture-host'){res.setHeader('Content-Type','text/html');res.end(host);return}
if(url.pathname==='/review.html'){res.setHeader('Content-Type','text/html');res.end((await readFile(new URL('../../internal/textmcp/appdist/review.html',import.meta.url),'utf8')).replace('__LUMI_CONNECT_SRC__',"'none'"));return}
if(url.pathname==='/web-fixture'){res.setHeader('Content-Type','text/html');res.end(await server.transformIndexHtml(req.url,'<html><div id="root"></div><script type="module" src="/@id/__x00__reference-web"></script></html>'));return}
if(url.pathname.startsWith('/facts/')||url.pathname.startsWith('/api/v1/projects/')){const path=url.pathname.startsWith('/facts/')?url.pathname.slice(7):url.pathname.slice(('/api/v1/projects/'+project).length+1)||'project';res.setHeader('Content-Type','application/json');const data=resource(path);res.end(JSON.stringify(url.pathname.startsWith('/facts/')?data:{success:true,data}));return}
next()
})}}],server:{host:'127.0.0.1',port:5189,strictPort:true}})
await server.listen();console.log('Simulated host: http://127.0.0.1:5189/fixture-host ; default Web: /web-fixture')
