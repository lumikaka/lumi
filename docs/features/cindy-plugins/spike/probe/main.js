const ID='lumi-step0-probe-20261101';
const channel=new BroadcastChannel(ID);
const bindings=new Map(), pages=[], notifications=[];
const cache=new Map();
const hookTasks=new Set();
let lastReference=null;
const schema={type:'object',properties:{slot:{type:'string',enum:['a','b']}}};
const catalog={
  runtime:['environment','context','parallel_projects','status','stop','crash','ws_connect','calibrate','mutate','session_probe'],
  media:['receive_media','reference','media_denied'],
  view:['view','view_focus_denied','view_receipts'],
  archive:['archive_boundary'],
  tasks:['task_create','task_send','task_read'],
  lifecycle:['lifecycle_start','lifecycle_report'],
  hooks:['hooks_enable','hooks_report'],
};
const rules={fixture:'只操作带 owner 标记的独立临时目录；结果不能冒充两个真实任务验收。',
  media:'顶层 attachments 授权，最多 20 MiB，48 KiB 分块；不得猜 Host 缓存路径。',
  archive:'只测试当前任务读取边界；不保存实际聊天内容，不默认采集。',
  view:'session 只路由，只有当前 tool-call 建立的绑定可请求页面校准；callId 不给页面。'};
async function node(method,params={},callId){
  const r=await cindy.node.request({method,params,...(callId?{callId}:{}),timeoutMs:60000});
  if(!r.ok)throw new Error(JSON.stringify(r));return r.result;
}
function b64(bytes){let s='';for(const b of bytes)s+=String.fromCharCode(b);return btoa(s);}
function redactTask(value){
  if(Array.isArray(value))return value.map(redactTask);
  if(value&&typeof value==='object')return Object.fromEntries(Object.entries(value).map(([k,v])=>[k,/(work.?dir|cwd|path)/i.test(k)?'<redacted>':redactTask(v)]));
  if(typeof value==='string')return value.replace(/\/Users\/[^\s"']+/g,'<redacted-user-path>');
  return value;
}
async function mediaBytes(value){
  const token=String(value).replace(/^.*\//,'');
  if(!/^[a-f0-9]{64}(\.[a-z0-9]+)?$/i.test(token))throw new Error('Invalid media fingerprint');
  // The handoff supplies a bare hash. This spike's two real specimens are PNG.
  // Match the documented /media/<fingerprint><suffix> route explicitly.
  const response=await fetch(`/media/${token.includes('.')?token:token+'.png'}`);
  if(!response.ok)throw new Error(`Media HTTP ${response.status}; origin=${location.origin}; token=${token}`);
  const declared=Number(response.headers.get('content-length'));
  if(declared>20*1024*1024)throw new Error('Media exceeds 20 MiB');
  const reader=response.body.getReader(),parts=[];let length=0;
  for(;;){const {value,done}=await reader.read();if(done)break;length+=value.length;
    if(length>20*1024*1024){await reader.cancel();throw new Error('Media exceeds 20 MiB');}parts.push(value);}
  const bytes=new Uint8Array(length);let pos=0;for(const p of parts){bytes.set(p,pos);pos+=p.length;}
  return {bytes,mime:response.headers.get('content-type'),token};
}
async function run(name,args,msg){
  const ctx=msg.args.session_context;
  const writes=['parallel_projects','stop','crash','ws_connect','calibrate','mutate','receive_media','reference','task_create','task_send','lifecycle_start'];
  if(writes.includes(name)&&(!ctx?.workdir_is_local||ctx.workdir_is_read_only))
    return {success:false,data:null,error:{code:'CONTEXT_NOT_WRITABLE',message:'Probe requires a trusted local writable task',details:''}};
  if(name==='context')return node('context',{context:ctx,call_id_present:!!msg.callId},msg.callId);
  if(name==='hooks_enable'){
    if(!ctx?.workdir_is_local||ctx.workdir_is_read_only)throw new Error('Local writable context required');
    const state=await node('task_state',{},msg.callId);
    for(const task of Object.values(state))hookTasks.add(task.taskId);
    return {enabled_only_for:[...hookTasks],records:'event metadata and text SHA256 only',verdict:'always allow'};
  }
  if(name==='hooks_report')return {enabled_only_for:[...hookTasks],records:await node('hook_report',{},msg.callId)};
  if(name==='session_probe')return node('session_probe',{context:ctx,call_id_present:!!msg.callId},msg.callId);
  if(name==='lifecycle_start'||name==='lifecycle_report')return cindy.node.request({entry:'node/lifecycle.cjs',method:name==='lifecycle_start'?'start':'report',params:{},callId:msg.callId});
  if(name==='task_create'){
    const which=args.slot||'a';
    const task=await cindy.tasks.create({requestKey:'step0-20261101-'+which,title:'Lumi Step 0 temporary '+which,isolatedWorkspace:true,callId:msg.callId});
    const state=await node('task_state',{slot:which,register:task},msg.callId);
    return {task:state[which],created:true};
  }
  if(name==='task_send'||name==='task_read'){
    const which=args.slot||'a',state=await node('task_state',{},msg.callId),saved=state[which];
    if(!saved)throw new Error('Create the isolated task first');
    const task=await cindy.tasks.get({taskId:saved.taskId});
    if(name==='task_send')return cindy.tasks.send({taskId:saved.taskId,expectedRevision:task.revision,requestKey:'step0-20261101-'+which+'-probe',invocationId:msg.callId,
      text:'这是用户授权的 Lumi Step 0 临时宿主验证。只做一次探针调用，不编辑文件、不启动其他任务。先 ghost_info({ghost_id:"lumi-step0-probe-20261101",namespace:null})，再 ghost_call({ghost_id:"lumi-step0-probe-20261101",namespace:null,tool:"list_tools",args:{category:"runtime"}})，最后 ghost_call({ghost_id:"lumi-step0-probe-20261101",namespace:null,tool:"call_tool",args:{name:"session_probe"}})。此操作只处理父任务已创建并标记的临时项目与 data-dir。仅回复返回的 boot、slot、own_project、shared_project、own_pid、shared_pid；禁止把 workdir 原路径写入回复。失败如实报告，不改权限、不用 shell 替代。'});
    const messages=await cindy.tasks.readMessages({taskId:saved.taskId,limit:20});
    const runs=await cindy.tasks.listRuns({taskId:saved.taskId,limit:10});
    return redactTask({taskId:saved.taskId,permissionMode:task.permissionMode,messages,runs});
  }
  if(catalog.runtime.includes(name))return node(name,args,msg.callId);
  if(name==='receive_media'){
    const attachments=msg.args.attachments;
    if(!Array.isArray(attachments)||attachments.length!==1)throw new Error('Pass exactly one top-level attachment');
    const {bytes,mime,token}=await mediaBytes(attachments[0]);
    const transfer=crypto.randomUUID();await node('media_begin',{transfer,bytes:bytes.length},msg.callId);
    for(let pos=0,index=0;pos<bytes.length;pos+=48*1024,index++)
      await node('media_chunk',{transfer,index,base64:b64(bytes.subarray(pos,pos+48*1024))},msg.callId);
    const result=await node('media_end',{transfer},msg.callId);
    lastReference=bytes.length<=6*1024*1024?{base64:b64(bytes),sha256:result.sha256}:null;
    let dimensions=null;
    if(typeof createImageBitmap==='function'){
      const bitmap=await createImageBitmap(new Blob([bytes],{type:mime}));dimensions={width:bitmap.width,height:bitmap.height};bitmap.close();
    }
    return {...result,mime,dimensions,authorized_token:token};
  }
  if(name==='reference'){
    const source=lastReference||await node('fixture',{},msg.callId);
    const deposited=await cindy.send({type:'cindy-request',kind:'deposit_media',data:source.base64,
      label:lastReference?'Step 0 authorized original reference':'Step 0 synthetic 1px reference',callId:msg.callId});
    return {deposited,source_sha256:source.sha256||null,synthetic:!lastReference,__probe_images:[{data:source.base64}]};
  }
  if(name==='media_denied'){
    const r=await fetch(`/media/${'0'.repeat(64)}.png`);
    return {status:r.status,ok:r.ok,note:'nonexistent unowned hash only; not proof of cross-plugin ownership rejection'};
  }
  if(name==='view'||name==='view_focus_denied'){
    if(!ctx?.session_id||!ctx.workdir_is_local||ctx.workdir_is_read_only)throw new Error('No writable local trusted session context');
    bindings.set(ctx.session_id,{slot:args.slot||'a'});
    return cindy.taskView({sessionId:ctx.session_id,...(name==='view_focus_denied'?{focus:true}:{})});
  }
  if(name==='view_receipts')return {origin:location.origin,pages,notifications,bound_sessions:[...bindings.keys()]};
  if(name==='archive_boundary'){
    const capabilities=await cindy.tasks.capabilities();let current_task;
    try {const r=await cindy.tasks.readMessages({taskId:ctx.session_id,limit:1});current_task={unexpected_read:true,keys:Object.keys(r)};}
    catch(e){current_task={code:e.code,message:e.message};}
    return {capabilities,current_task,note:'No message contents retained'};
  }
  return {success:false,data:null,error:{code:'UNKNOWN_OPERATION',message:'Read list_tools(category)',details:{available:Object.values(catalog).flat()}}};
}
channel.onmessage=async({data})=>{
  if(data?.kind==='page-receipt'){pages.push(data);if(pages.length>60)pages.shift();return;}
  if(data?.kind!=='calibrate'||!bindings.has(data.session)||typeof data.reqId!=='string')return;
  const key=data.session+':'+data.reqId;
  if(!cache.has(key))cache.set(key,(async()=>{
    try{return {ok:true,result:await node('calibrate',{...bindings.get(data.session),reason:data.reason})};}
    catch(e){return {ok:false,error:e.message};}
  })());
  const response=await cache.get(key);channel.postMessage({kind:'reply',session:data.session,reqId:data.reqId,...response});
  if(cache.size>100)cache.delete(cache.keys().next().value);
};
// Default off. Only an explicit hooks_enable call opts the registered test tasks in.
// Other sessions are allowed immediately and their text is never inspected or stored.
cindy.onHostMessage(async msg=>{
  if(msg.type==='event'&&['will-user-message','will-assistant-message'].includes(msg.name)){
    await cindy.send({type:'event-verdict',hookId:msg.hookId,action:'allow'});
    if(!hookTasks.has(msg.data?.sessionId))return;
    const bytes=new TextEncoder().encode(msg.data?.text||'');
    const digest=await crypto.subtle.digest('SHA-256',bytes);
    await node('hook_sample',{session_id:msg.data.sessionId,name:msg.name,seq:msg.seq??null,dropped:msg.dropped??0,
      data_keys:Object.keys(msg.data).sort(),message_id:msg.data.messageId??null,
      text_bytes:bytes.length,text_sha256:[...new Uint8Array(digest)].map(x=>x.toString(16).padStart(2,'0')).join('')});
    return;
  }
  if(msg.type==='event'&&msg.name?.startsWith('did-')){
    if(hookTasks.has(msg.data?.sessionId))await node('hook_sample',{session_id:msg.data.sessionId,name:msg.name,
      seq:msg.seq??null,dropped:msg.dropped??0,data_keys:Object.keys(msg.data).sort(),end_reason:msg.data.endReason??null});
    return;
  }
  if(msg.type==='event'&&(msg.name==='node-notification'||msg.name==='node-status')){
    notifications.push({name:msg.name,method:msg.method,params:msg.params,state:msg.state});
    if(notifications.length>60)notifications.shift();
    if(msg.method==='lumi-change')for(const [session,binding]of bindings)
      if(binding.slot===msg.params?.slot)channel.postMessage({kind:'invalidate',session,frame:msg.params.frame});
    return;
  }
  if(msg.type!=='tool-call')return;
  try{
    let result;
    if(msg.tool==='list_tools'){
      const category=msg.args.category;
      result=category?{category,tools:(catalog[category]||[]).map(name=>({name,parameters:schema})),rules}
        :{categories:Object.entries(catalog).map(([name,items])=>({name,count:items.length}))};
    }else if(msg.tool==='call_tool'){
      const args=msg.args.args||{};
      if(typeof msg.args.name!=='string'||(args.slot!==undefined&&!['a','b'].includes(args.slot)))
        result={success:false,data:null,error:{code:'INVALID_ARGS',message:'Expected a known operation and slot a/b',details:{schema,rules}}};
      else result=await run(msg.args.name,args,msg);
    }else throw new Error('Unknown top-level tool');
    const images=result?.__probe_images;if(images)delete result.__probe_images;
    await cindy.send({type:'tool-result',callId:msg.callId,ok:true,result,...(images?{images}:{})});
  }catch(error){await cindy.send({type:'tool-result',callId:msg.callId,ok:false,errorCode:'PROBE_FAILED',message:error.message});}
});
