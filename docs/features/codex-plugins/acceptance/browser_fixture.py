#!/usr/bin/env python3
"""Browser-only integration harness. This is not Codex App host acceptance."""
import argparse
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import threading
from prepare_step5 import MCP

PAGE = '''<!doctype html><html><meta charset="utf-8"><title>Lumi browser integration fixture</title><style>body{margin:0;background:#dfe4df;font:14px system-ui}header{padding:10px;display:flex;gap:8px;flex-wrap:wrap}button{padding:8px}iframe{width:100%;height:1600px;border:0;background:white}</style><header><strong>Browser integration · real Lumi CLI</strong><button data-view="book">整本</button><button data-view="page">单页</button><button data-view="candidates">候选</button><button data-view="settings">设定</button><button id="theme">切换主题</button><button id="failure">图片错误</button><button id="mutate">修改临时页正文</button><button id="wrap">文本包装</button></header><iframe sandbox="allow-scripts" src="/review.html"></iframe><script>
const frame=document.querySelector('iframe');let view='book',dark=false,fail=false,wrapped=false;window.fixtureEvents=[];
async function call(name,args={}){return fetch('/call',{method:'POST',body:JSON.stringify({name,arguments:args,fail})}).then(r=>r.json())}
async function show(){const r=await fetch('/view/'+view).then(r=>r.json());frame.contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/tool-result',params:wrapped?{content:[{type:'text',text:JSON.stringify(r)}]}:r},'*')}
window.addEventListener('message',async event=>{if(event.source!==frame.contentWindow||event.data?.jsonrpc!=='2.0')return;const m=event.data;window.fixtureEvents.push({at:Date.now(),method:m.method,name:m.params?.name});let result;
try{if(m.method==='ui/initialize'){result={protocolVersion:'2026-01-26',hostInfo:{name:'browser-fixture',version:'1'},hostCapabilities:{},hostContext:{theme:dark?'dark':'light'}}}
else if(m.method==='ui/notifications/initialized'){await show();return}
else if(m.method==='tools/call')result=await call(m.params.name,m.params.arguments);
else if(m.method==='ui/open-link')result={};else return;
frame.contentWindow.postMessage({jsonrpc:'2.0',id:m.id,result},'*')}catch(error){frame.contentWindow.postMessage({jsonrpc:'2.0',id:m.id,error:{code:-32603,message:String(error)}},'*')}});
for(const button of document.querySelectorAll('[data-view]'))button.onclick=()=>{view=button.dataset.view;show()};
document.querySelector('#theme').onclick=()=>{dark=!dark;frame.contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/host-context-changed',params:{theme:dark?'dark':'light'}},'*')};
document.querySelector('#failure').onclick=()=>{fail=!fail;frame.src='/review.html?retry='+Date.now()};
document.querySelector('#wrap').onclick=()=>{wrapped=!wrapped;show()};
document.querySelector('#mutate').onclick=()=>fetch('/mutate',{method:'POST'});
</script></html>'''

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('root',type=Path);args=parser.parse_args();root=args.root
    manifest=json.loads((root/'manifest.json').read_text());mcp=MCP(Path(manifest['binary']),Path(manifest['project']),Path(manifest['data_dir']));lock=threading.Lock()
    class Handler(BaseHTTPRequestHandler):
        def reply(self,body,kind='application/json'):
            if not isinstance(body,bytes):body=json.dumps(body,ensure_ascii=False).encode()
            self.send_response(200);self.send_header('Content-Type',kind);self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
        def do_GET(self):
            if self.path=='/':return self.reply(PAGE.encode(),'text/html')
            if self.path.startswith('/review.html'):return self.reply((root/'review.html').read_bytes(),'text/html')
            if self.path.startswith('/view/'):
                with lock: result=mcp.rpc('tools/call',manifest['requests'][self.path.rsplit('/',1)[-1]])
                return self.reply(result)
            self.send_error(404)
        def do_POST(self):
            body=json.loads(self.rfile.read(int(self.headers.get('Content-Length','0'))) or '{}')
            with lock:
                if self.path=='/mutate':
                    p=mcp.tool('pages',{'action':'get','chapter_uuid':manifest['chapter_uuid'],'page_uuid':manifest['page_uuid']})
                    return self.reply(mcp.tool('pages',{'action':'update','chapter_uuid':manifest['chapter_uuid'],'page_uuid':manifest['page_uuid'],'expected_revision':p['revision'],'description_md':'WebSocket 提示后从 REST 重读的最新正文。'}))
                if self.path!='/call' or body['name'] not in ('read_app_image','read_app_connection',*['review_'+v for v in manifest['requests']]):return self.send_error(403)
                if body.get('fail') and body['name']=='read_app_image':
                    return self.reply({'isError':True,'structuredContent':{'success':False,'data':None,'error':{'code':'asset_not_found','message':'故意触发单图失败','details':'手动重试'}}})
                result=mcp.rpc('tools/call',{'name':body['name'],'arguments':body['arguments']})
                with (root/'browser-requests.jsonl').open('a') as log:log.write(json.dumps({'name':body['name'],'arguments':body['arguments'],'result_bytes':len(json.dumps(result).encode())})+'\n')
            self.reply(result)
        def log_message(self,*args):pass
    server=ThreadingHTTPServer(('127.0.0.1',0),Handler);address='http://127.0.0.1:'+str(server.server_port)
    (root/'browser-address.txt').write_text(address);print(address,flush=True)
    try:server.serve_forever()
    finally:server.server_close();mcp.close()
if __name__=='__main__':main()
