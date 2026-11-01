// This is a protocol/integration check under the shell Node, NOT a Host pass.
const {spawn}=require('node:child_process');
const path=require('node:path');
const fs=require('node:fs');
const assert=require('node:assert/strict');
const base=path.resolve(__dirname,'..');
const worker=spawn(process.execPath,[path.join(base,'probe/node/worker.cjs')],{stdio:['pipe','pipe','inherit']});
const pending=new Map(),evidence=[];let id=0,buffer='';
worker.stdout.on('data',chunk=>{
  buffer+=chunk;
  while(buffer.includes('\n')){
    const index=buffer.indexOf('\n'),r=JSON.parse(buffer.slice(0,index));buffer=buffer.slice(index+1);
    if(r.id!==undefined){const p=pending.get(r.id);if(!p)continue;pending.delete(r.id);clearTimeout(p.timer);r.error?p.reject(Error(r.error.message)):p.resolve(r.result);}
    else evidence.push({notification:r});
  }
});
function call(method,params={}){
  return new Promise((resolve,reject)=>{
    const n=++id,timer=setTimeout(()=>{pending.delete(n);reject(Error('timeout '+method));},65000);
    pending.set(n,{resolve,reject,timer});worker.stdin.write(JSON.stringify({jsonrpc:'2.0',id:n,method,params})+'\n');
  }).then(result=>{evidence.push({method,result});return result;});
}
(async()=>{
  let failed;
  try{
    await call('environment');
    const r=await call('parallel_projects');
    assert.equal(r.separate_servers,true);assert.equal(r.separate_projects,true);assert.equal(r.same_project_server_reused,true);
    assert.deepEqual(r.a1.data.open_projects.map(p=>p.uuid),[r.a.project.uuid]);
    assert.deepEqual(r.b1.data.open_projects.map(p=>p.uuid),[r.b.project.uuid]);
    const join=await call('ws_connect');assert.equal(join.frame.payload.status,'ok');
    const calibrated=await call('calibrate',{reason:'local-check-join'});assert.equal(calibrated.projects.items.length,1);
    const {base64}=await call('fixture');const bytes=Buffer.from(base64,'base64');
    await call('media_begin',{transfer:'local-check',bytes:bytes.length});
    await call('media_chunk',{transfer:'local-check',index:0,base64});
    const media=await call('media_end',{transfer:'local-check'});assert.equal(media.bytes,bytes.length);
    console.log('PASS: parallel isolated CLI servers/projects; same-project reuse; WS join; REST calibration; media chunk integrity');
  }catch(error){failed=error;console.error('FAIL:',error.message);}
  finally{
    try{await call('stop');const stopped=await call('status');assert.equal(stopped.a.data.running,false);assert.equal(stopped.b.data.running,false);console.log('PASS: both probe CLI instances stopped');}
    catch(error){failed=error;console.error('CLEANUP FAILED:',error.message);}
    fs.writeFileSync(path.join(base,'evidence/local-check.json'),JSON.stringify({evidence_level:'local Node integration, not Cindy Host',evidence},null,2)+'\n');
    worker.kill();if(failed)process.exitCode=1;
  }
})();
