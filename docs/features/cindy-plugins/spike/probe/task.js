const ID='lumi-step0-probe-20261101',channel=new BroadcastChannel(ID);
const session=new URLSearchParams(location.search).get('session'),pending=new Map();
function receipt(event,extra={}){channel.postMessage({kind:'page-receipt',session,event,...extra});}
receipt('script-loaded',{origin:location.origin});
async function calibrate(reason){
  const wake=await(await fetch('/wake')).json();
  receipt('wake',{wake,reason,theme:getComputedStyle(document.documentElement).colorScheme});
  const reqId=crypto.randomUUID();
  const request={kind:'calibrate',session,reqId,reason};let attempts=0;
  const emit=()=>{channel.postMessage(request);if(++attempts>=20){clearInterval(pending.get(reqId));pending.delete(reqId);document.querySelector('#state').textContent='绑定失效或宿主未回复';}};
  pending.set(reqId,setInterval(emit,300));emit();
}
channel.onmessage=({data})=>{
  if(data?.session!==session)return;
  if(data.kind==='reply'&&pending.has(data.reqId)){
    clearInterval(pending.get(data.reqId));pending.delete(data.reqId);
    document.querySelector('#state').textContent=data.ok?'已校准':'校准失败';
    document.querySelector('#result').textContent=JSON.stringify(data.result||data.error,null,2);receipt('reply',{ok:data.ok});
  }
  if(data.kind==='invalidate')calibrate(data.frame?.event==='phx_reply'?'join':'ws-change');
};
document.querySelector('#refresh').onclick=()=>calibrate('manual');
window.addEventListener('focus',()=>calibrate('focus'));
window.addEventListener('pagehide',()=>{for(const timer of pending.values())clearInterval(timer);});
calibrate('first-load').catch(error=>{document.querySelector('#state').textContent=error.message;});
