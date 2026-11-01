'use strict';
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { execFile } = require('node:child_process');
const readline = require('node:readline');
const config = require('../probe-config.json');
const binary = path.resolve(__dirname, '../bin/lumi');
const boot = crypto.randomUUID();
const transfers = new Map();
const connections = new Map();
const frames = [];
let businessReads = 0;
const sha = value => crypto.createHash('sha256').update(value).digest('hex');
function guard() {
  if (!config.root.startsWith('/private/tmp/lumi-cindy-step0-') || fs.realpathSync(config.root) !== config.root ||
      fs.readFileSync(path.join(config.root, '.probe-owner'), 'utf8') !== config.owner) throw new Error('Invalid fixture owner');
}
function slot(value = 'a') {
  guard();
  if (!['a', 'b'].includes(value)) throw new Error('slot must be a or b');
  const root = path.join(config.root, value);
  for (const name of ['project', 'data']) {
    const p = path.join(root, name);
    if (fs.realpathSync(p) !== p) throw new Error('Fixture symlink rejected');
  }
  return { root, project: path.join(root, 'project'), data: path.join(root, 'data') };
}
function send(message) { process.stdout.write(JSON.stringify(message) + '\n'); }
function record(name, result) {
  guard();
  fs.appendFileSync(path.join(config.root, 'worker-evidence.jsonl'), JSON.stringify({time:new Date().toISOString(),boot,name,result})+'\n', {mode:0o600});
  return result;
}
function cli(action, which = 'a') {
  const s = slot(which);
  const env = { PATH:'/usr/bin:/bin:/usr/sbin:/sbin', HOME:s.root, TMPDIR:config.root };
  return new Promise((resolve, reject) => execFile(binary,
    [action,'--json','--environment','production','--data-dir',s.data,'--timeout','25s'],
    {cwd:s.project,env,timeout:30000,maxBuffer:512*1024}, (error,stdout,stderr) => {
      let body; try { body=JSON.parse(stdout); } catch { return reject(new Error(`CLI ${action}: ${error?.message||stdout||stderr}`)); }
      const code=error?error.code:0;
      if (error && !(action==='status' && code===3)) return reject(new Error(JSON.stringify(body)));
      resolve({exit_code:code,...body});
    }));
}
async function instance(which) {
  const r=await cli('serve',which);
  if (!r.success) throw new Error(JSON.stringify(r));
  return r.data;
}
async function rest(inst,route,method='GET',body) {
  if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(inst.endpoint)) throw new Error('Non-loopback endpoint');
  if (method==='GET') businessReads++;
  const r=await fetch(inst.endpoint+route,{method,headers:{'Content-Type':'application/json','If-Match':inst.nonce},
    body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(25000)});
  const value=await r.json();
  if (!r.ok||!value.success) throw new Error(JSON.stringify(value));
  return value.data;
}
async function project(which) {
  const s=slot(which),inst=await instance(which);
  // Existing REST avoids the Codex guidance side effects of `lumi init`.
  const p=await rest(inst,'/api/v1/project-initializations','POST',
    {root_path:s.project,name:`Cindy temporary probe ${which}`,generation_language:'en'});
  await rest(inst,'/api/v1/open-projects','POST',{root_path:s.project});
  return {inst,project:p};
}
async function handle(method,p={}) {
  guard();
  if(method==='hook_report'){
    const file=path.join(config.root,'hook-events.jsonl');
    return fs.existsSync(file)?fs.readFileSync(file,'utf8').trim().split('\n').filter(Boolean).map(JSON.parse):[];
  }
  if(method==='hook_sample'){
    const state=await handle('task_state');
    if(!Object.values(state).some(t=>t.taskId===p.session_id))throw new Error('Only registered probe tasks');
    const item={time:new Date().toISOString(),boot,...p};
    fs.appendFileSync(path.join(config.root,'hook-events.jsonl'),JSON.stringify(item)+'\n',{mode:0o600});
    return {recorded:true};
  }
  if(method==='task_state') {
    const file=path.join(config.root,'probe-tasks.json');
    const state=fs.existsSync(file)?JSON.parse(fs.readFileSync(file,'utf8')):{};
    if(p.register){
      if(!['a','b'].includes(p.slot)||typeof p.register.taskId!=='string')throw new Error('Invalid task registration');
      state[p.slot]={taskId:p.register.taskId,revision:p.register.revision,permissionMode:p.register.permissionMode,
        workdir_sha256:p.register.workingDir?sha(p.register.workingDir):null};
      fs.writeFileSync(file,JSON.stringify(state),{mode:0o600});
    }
    return state;
  }
  if(method==='session_probe') {
    const state=await handle('task_state');
    const which=Object.keys(state).find(s=>state[s].taskId===p.context?.session_id);
    if(!which)throw new Error('Only this probe\'s registered temporary tasks can run session_probe');
    const context=await handle('context',{context:p.context,call_id_present:p.call_id_present});
    if(!p.context.workdir_is_local||p.context.workdir_is_read_only)return {context,denied:true};
    const started=Date.now();
    const own=await project(which);
    await new Promise(r=>setTimeout(r,3000));
    const shared=await project('a');
    return record(method,{context,slot:which,started,ended:Date.now(),own_project:own.project.uuid,
      own_pid:own.inst.pid,shared_project:shared.project.uuid,shared_pid:shared.inst.pid,boot});
  }
  if(method==='mutate') {
    const which=p.slot||'a',obj=await project(which),route='/api/v1/projects/'+obj.project.uuid;
    const detail=await rest(obj.inst,route);
    const updated=await rest(obj.inst,route,'PATCH',{name:detail.name,description:'Step 0 mutation '+Date.now(),
      expected_revision:detail.revision,rename_directory:false});
    return record(method,{slot:which,uuid:obj.project.uuid,before_revision:detail.revision,after_revision:updated.revision});
  }
  if (method==='environment') return record(method, {boot,pid:process.pid,ppid:process.ppid,node:process.version,
    platform:process.platform,arch:process.arch,os_release:os.release(),exec_path:process.execPath,
    gui_path:(process.env.PATH||'').split(os.homedir()).join('<HOME>'),cli_sha256:sha(fs.readFileSync(binary)),
    cli_mode:(fs.statSync(binary).mode&0o777).toString(8),lumi_commit:config.lumi_commit,fixture:config.root});
  if (method==='context') return record(method,{boot,session_id:p.context?.session_id??null,
    workdir:p.context?.workdir?'<redacted; see workdir_sha256>':null,
    workdir_sha256:p.context?.workdir?sha(p.context.workdir):null,
    workdir_is_local:p.context?.workdir_is_local??null,workdir_is_read_only:p.context?.workdir_is_read_only??null,
    call_id_present:p.call_id_present===true});
  if (method==='parallel_projects') {
    const [a,b]=await Promise.all([project('a'),project('b')]);
    const [a1,a2,b1]=await Promise.all([cli('status','a'),cli('status','a'),cli('status','b')]);
    return record(method,{boot,a,b,a1,a2,b1,separate_servers:a.inst.pid!==b.inst.pid,
      separate_projects:a.project.uuid!==b.project.uuid,same_project_server_reused:a1.data.pid===a2.data.pid,
      evidence_level:'one worker; synthetic concurrent calls, not two Host tasks'});
  }
  if (method==='status') return record(method,{boot,a:await cli('status','a'),b:await cli('status','b'),business_reads:businessReads,frames:frames.slice(-20)});
  if (method==='stop') {
    for (const c of connections.values()) c.close(); connections.clear();
    return record(method,{boot,a:await cli('stop','a'),b:await cli('stop','b')});
  }
  if (method==='crash') {record(method,{intentional:true,pid:process.pid});process.exit(73);}
  if (method==='notify') {
    const count=Math.min(150,Math.max(1,Number(p.count)||1));
    for(let i=0;i<count;i++){send({jsonrpc:'2.0',method:'probe-tick',params:{boot,i}});await new Promise(r=>setTimeout(r,1000));}
    return {boot,count};
  }
  if (method==='ws_connect') {
    if(typeof WebSocket!=='function') throw new Error('Host Node has no global WebSocket');
    const which=p.slot||'a',obj=await project(which),topic=`project:${obj.project.uuid}`,ref=crypto.randomUUID();
    connections.get(which)?.close();
    // Node's browser-compatible WebSocket cannot set Origin. Reuse the existing
    // CLI's short-lived, single-project hint ticket; never forward it to the UI.
    const connection=await rest(obj.inst,'/api/v1/creation-requests','POST',
      {root_path:slot(which).project,tool:'read_app_connection',arguments:{}});
    const url=new URL(connection.websocket_url);
    if(url.origin!==obj.inst.endpoint.replace(/^http/,'ws')||url.pathname!=='/api/v1/ws'||!url.searchParams.get('app_ticket'))
      throw new Error('Invalid scoped hint connection');
    const ws=new WebSocket(url);connections.set(which,ws);
    return await new Promise((resolve,reject)=>{
      const timer=setTimeout(()=>{ws.close();reject(new Error('WS join timeout'));},5000);
      ws.addEventListener('open',()=>ws.send(JSON.stringify({topic,event:'phx_join',payload:{},ref,join_ref:ref})));
      ws.addEventListener('error',()=>{clearTimeout(timer);reject(new Error('WS error'));});
      ws.addEventListener('message',e=>{
        const frame=JSON.parse(String(e.data));frames.push(frame);if(frames.length>100)frames.shift();
        send({jsonrpc:'2.0',method:'lumi-change',params:{slot:which,frame}});
        if(frame.event==='phx_reply'&&frame.ref===ref){clearTimeout(timer);resolve(record(method,{boot,slot:which,frame}));}
      });
    });
  }
  if(method==='calibrate')return record(method,{reason:p.reason,business_reads:businessReads+1,
    projects:await rest(await instance(p.slot||'a'),'/api/v1/open-projects')});
  if(method==='media_begin'){
    if(!/^[a-zA-Z0-9_-]{1,80}$/.test(p.transfer)||transfers.size>=2)throw new Error('Invalid transfer');
    if(!Number.isSafeInteger(p.bytes)||p.bytes<1||p.bytes>20*1024*1024)throw new Error('Media limit 20 MiB');
    transfers.set(p.transfer,{bytes:p.bytes,chunks:[],received:0});return {accepted:true};
  }
  if(method==='media_chunk'){
    const t=transfers.get(p.transfer);
    if(!t||typeof p.base64!=='string'||p.base64.length>64*1024||p.index!==t.chunks.length)throw new Error('Invalid chunk');
    const b=Buffer.from(p.base64,'base64');if(t.received+b.length>t.bytes)throw new Error('Size exceeded');
    t.chunks.push(b);t.received+=b.length;return {received:t.received};
  }
  if(method==='media_end'){
    const t=transfers.get(p.transfer);transfers.delete(p.transfer);
    if(!t||t.received!==t.bytes)throw new Error('Incomplete media');
    const bytes=Buffer.concat(t.chunks),hash=sha(bytes),dir=path.join(config.root,'media');fs.mkdirSync(dir,{recursive:true});
    fs.writeFileSync(path.join(dir,hash),bytes,{mode:0o600});
    return record(method,{bytes:bytes.length,sha256:hash,magic_hex:bytes.subarray(0,16).toString('hex')});
  }
  if(method==='fixture')return {base64:'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aA1sAAAAASUVORK5CYII='};
  throw new Error('Unknown probe method');
}
readline.createInterface({input:process.stdin}).on('line',async line=>{
  let request;
  try{request=JSON.parse(line);send({jsonrpc:'2.0',id:request.id,result:await handle(request.method,request.params)});}
  catch(error){send({jsonrpc:'2.0',id:request?.id??null,error:{code:-32000,message:error.message}});}
});
