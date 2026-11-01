'use strict';
// Separate broker entry: main worker requests cannot extend this entry's idle timer.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),readline=require('node:readline');
const config=require('../probe-config.json'),boot=crypto.randomUUID(),started=Date.now();
if(fs.readFileSync(path.join(config.root,'.probe-owner'),'utf8')!==config.owner)throw Error('Fixture mismatch');
const log=value=>fs.appendFileSync(path.join(config.root,'lifecycle.jsonl'),JSON.stringify({time:Date.now(),boot,pid:process.pid,...value})+'\n',{mode:0o600});
const send=value=>process.stdout.write(JSON.stringify(value)+'\n');
let timer=null,ticks=0;
log({event:'start'});
readline.createInterface({input:process.stdin}).on('line',line=>{
 const r=JSON.parse(line);
 if(r.method==='start'){
  if(!timer)timer=setInterval(()=>{
    ticks++;send({jsonrpc:'2.0',method:'idle-probe-tick',params:{boot,ticks}});
    if(ticks%5===0)log({event:'tick',ticks});
    if(ticks>=80){clearInterval(timer);timer=null;log({event:'ticks-complete',ticks});}
  },2000);
 }
 send({jsonrpc:'2.0',id:r.id,result:{boot,pid:process.pid,started,ticks}});
});
process.on('exit',code=>log({event:'exit',code,ticks}));
