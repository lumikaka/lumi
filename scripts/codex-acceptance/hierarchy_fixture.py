#!/usr/bin/env python3
"""Disposable four-level review fixture; real stdio -> CLI REST -> SQLite.

No model calls or exports. Uses the repository icon as a synthetic image fixture.
Browser host is an integration adapter, not Codex Desktop host acceptance.
Run: python3 hierarchy_fixture.py --binary /absolute/path/to/lumi
"""
import argparse
import concurrent.futures
import json
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]


class MCP:
    def __init__(self, binary, project, data):
        self.serial = 0
        self.lock = threading.Lock()
        self.pending = {}
        self.log = (data.parent / 'cli-stderr.log').open('a')
        self.proc = subprocess.Popen([str(binary), 'mcp', '--data-dir', str(data)], cwd=project,
                                     stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=self.log, text=True)
        threading.Thread(target=self.receive, daemon=True).start()
        self.rpc('initialize', {'protocolVersion': '2025-06-18', 'capabilities': {},
                               'clientInfo': {'name': 'hierarchy-acceptance', 'version': '1'}})
        self.proc.stdin.write(json.dumps({'jsonrpc': '2.0', 'method': 'notifications/initialized'}) + '\n')
        self.proc.stdin.flush()

    def receive(self):
        for line in self.proc.stdout:
            reply = json.loads(line)
            with self.lock:
                future = self.pending.pop(reply.get('id'), None)
            if future:
                if 'error' in reply:
                    future.set_exception(RuntimeError(reply['error']))
                else:
                    future.set_result(reply.get('result'))

    def rpc(self, method, params):
        with self.lock:
            self.serial += 1
            future = concurrent.futures.Future()
            self.pending[self.serial] = future
            self.proc.stdin.write(json.dumps({'jsonrpc': '2.0', 'id': self.serial, 'method': method, 'params': params}) + '\n')
            self.proc.stdin.flush()
        return future.result(timeout=120)

    def tool(self, name, args):
        result = self.rpc('tools/call', {'name': name, 'arguments': args})['structuredContent']
        if not result['success']:
            raise RuntimeError(result['error'])
        return result['data']


PAGE = '''<!doctype html><html><meta charset="utf-8"><title>Lumi 四层审阅验收</title>
<style>body{margin:0;background:#e6e9e2;font:13px system-ui}header{padding:10px;display:flex;gap:10px;align-items:center;flex-wrap:wrap}button{padding:6px}iframe{width:100%;height:calc(100vh - 100px);border:0;display:block}</style>
<header>测试宿主 · 真实 Lumi CLI <button data-view="book">作品</button><button data-view="chapters">章节列表</button><button data-view="chapter">章节</button><button data-view="page">单页</button><button data-view="empty">空章节</button><button data-view="legacy">历史章节结果</button><button data-view="legacy_book">旧整本参数</button><button data-view="legacy_chapter">旧章节参数</button><button data-view="empty_book">空作品</button><button id="theme">深/浅色</button><button id="mutate">实时修改测试正文</button><button id="delete">删除测试第2页</button><button id="delay">延迟下次单页</button><button id="fail">下次读取失败</button><button id="facts">读取采用结果</button><output id="fact-result"></output></header>
<iframe sandbox="allow-scripts allow-same-origin allow-forms" src="/review.html"></iframe><script>
const frame=document.querySelector('iframe');let current='book',dark=false,fixtureProject=null;window.fixtureEvents=[];window.openedLinks=[];window.delayNextPage=false;window.failNextReview=false;
async function show(view=current){current=view;const params=await fetch('/view/'+view).then(r=>r.json());fixtureProject=params.structuredContent.data.project_uuid;frame.contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/tool-result',params},'*')}
window.fixtureShow=show;
window.addEventListener('message',async event=>{if(event.source!==frame.contentWindow||event.data?.jsonrpc!=='2.0')return;const m=event.data;window.fixtureEvents.push({method:m.method,params:m.params,at:Date.now()});let result;
try{if(m.method==='ui/initialize')result={protocolVersion:'2026-01-26',hostInfo:{name:'hierarchy-fixture',version:'1'},hostCapabilities:{},hostContext:{theme:dark?'dark':'light'}};
else if(m.method==='ui/notifications/initialized'){show();return}
else if(m.method==='tools/call'){const delayed=window.delayNextPage&&m.params.name==='read_app_resource';const fail=window.failNextReview&&m.params.name==='read_app_resource';if(fail)window.failNextReview=false;if(delayed){window.delayNextPage=false;await new Promise(r=>setTimeout(r,1200))}result=await fetch('/call',{method:'POST',body:JSON.stringify({...m.params,fixture_project:fixtureProject,fixture_fail:fail})}).then(r=>r.json())}
else if(m.method==='ui/open-link'){window.openedLinks.push(m.params.url);result={}}else return;
frame.contentWindow.postMessage({jsonrpc:'2.0',id:m.id,result},'*')}catch(e){frame.contentWindow.postMessage({jsonrpc:'2.0',id:m.id,error:{code:-32603,message:String(e)}},'*')}});
for(const b of document.querySelectorAll('[data-view]'))b.onclick=()=>show(b.dataset.view);
document.querySelector('#mutate').onclick=()=>fetch('/mutate',{method:'POST'});
document.querySelector('#delete').onclick=()=>fetch('/delete',{method:'POST'});
document.querySelector('#delay').onclick=()=>{window.delayNextPage=true};
document.querySelector('#fail').onclick=()=>{window.failNextReview=true};
document.querySelector('#facts').onclick=async()=>{document.querySelector('#fact-result').textContent=JSON.stringify(await fetch('/facts',{method:'POST'}).then(r=>r.json()))};
document.querySelector('#theme').onclick=()=>{dark=!dark;frame.contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/host-context-changed',params:{theme:dark?'dark':'light'}},'*')};
</script></html>'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--reuse', type=Path)
    args = parser.parse_args()
    if args.reuse:
        root=args.reuse.resolve()
        manifest=json.loads((root/'manifest.json').read_text())
        if not root.name.startswith('lumi-review-hierarchy-') or root.parent != Path('/tmp').resolve():
            raise RuntimeError('Reuse is restricted to this disposable fixture workflow')
        binary=args.binary.resolve();project=Path(manifest['project'])
        mcp=MCP(binary,project,Path(manifest['data_dir']))
        empty_mcp=MCP(binary,root/'empty-project',Path(manifest['data_dir']))
        chapters=[{'uuid':v} for v in manifest['chapters']]
        pages=[{'uuid':v} for v in manifest['pages']]
        project_uuid=manifest['project_uuid'];requests=manifest['requests']
        tools=mcp.rpc('tools/list',{})['tools']
        uri=next(t for t in tools if t['name']=='review_book')['_meta']['ui']['resourceUri']
        resource=mcp.rpc('resources/read',{'uri':uri})
        (root/'review.html').write_text(resource['contents'][0]['text'])
        manifest.update(binary=str(binary),resource_uri=uri)
        allowed={t['name'] for t in tools if t['name'].startswith('review_')} | {'read_app_image', 'read_app_image_detail','watch_app_changes','read_app_resource','write_app_resource','read_app_review'}
    else:
        root = Path(tempfile.mkdtemp(prefix='lumi-review-hierarchy-', dir='/tmp')).resolve()
        project = root / 'project'
        project.mkdir()
        binary = args.binary.resolve()
        mcp = MCP(binary, project, root / 'data')
        init = mcp.tool('project_init', {'idea': '小狐狸沿着星光回家。这是一次隔离测试。', 'name': '星光旅途 · 四层审阅测试'})
        setup = mcp.tool('project_setup', {'action': 'update', 'expected_revision': init['setup']['revision'],
                        'project_name': '星光旅途 · 四层审阅测试', 'generation_language': 'zh-Hans', 'overall_style': '柔和水彩',
                        'picture_book': {'format': 'classic_picture_book', 'aspect_ratio': {'mode': 'landscape'}}})
        mcp.tool('project_setup', {'action': 'finalize', 'expected_revision': setup['revision']})
        chapters = []
        for i in range(22):
            p = mcp.tool('project_overview', {})['project']
            ch = mcp.tool('chapters', {'action': 'create', 'chapter_code': f'vol01.ch{i+1:02d}',
                          'title': ['星光启程', '森林回声'][i] if i < 2 else f'空章节 {i+1}', 'expected_revision': p['revision']})
            chapters.append(ch)
        mcp.tool('chapters', {'action': 'write', 'chapter_uuid': chapters[0]['uuid'], 'content':
                 '小狐狸在夜色中发现一颗迷路的星星。\n他穿过森林，沿着溪流，把星星送回山顶。\n当黎明来临时，回家的路也亮了。',
                 'content_format': 'txt', 'expected_revision': chapters[0]['revision']})
        pages = []
        image = REPO / 'codex-plugins/lumi/assets/icon.png'
        for i in [*range(1, 23), 23, 0, 24]:
            ch = chapters[0] if i < 24 else chapters[1]
            state = mcp.tool('pages', {'action': 'list', 'chapter_uuid': ch['uuid']})['state']
            role = 'front_cover' if i == 0 else 'back_cover' if i == 23 else 'body'
            page = mcp.tool('pages', {'action': 'create', 'chapter_uuid': ch['uuid'], 'page_role': role,
                   'title': '星光旅途 · 封面' if i == 0 else '星光旅途 · 封底' if i == 23 else f'星光第 {i} 页',
                   'description_md': f'完整正文 {i}：小狐狸说：“别怕，我陪你回家。”\n雨滴在树叶上唱起轻轻的歌。',
                   'storyboard_md': '分镜：水彩森林，靛蓝夜空，一道暖金色的星光。\n主体位于画面下半部分。',
                   'expected_revision': state['revision']})
            pages.append(page)
            if i in (0, 1, 2, 23, 24):
                prepared=mcp.tool('prepare_page_image', {'page_uuid': page['uuid'], 'setting_uuids': []})
                mcp.tool('import_image', {'path': str(image), **prepared['image_write_back'], 'prompt': prepared['generation_prompt'], 'preparation': prepared['preparation'], 'fit_mode': 'pad'})
        pages = mcp.tool('pages', {'action': 'list', 'chapter_uuid': chapters[0]['uuid'], 'limit': 100})['items'] + mcp.tool('pages', {'action': 'list', 'chapter_uuid': chapters[1]['uuid'], 'limit': 100})['items']
        current = mcp.tool('pages', {'action': 'get', 'chapter_uuid': chapters[0]['uuid'], 'page_uuid': pages[1]['uuid']})
        adopted = current['current_image']['uuid']
        prepared=mcp.tool('prepare_page_image', {'page_uuid': current['uuid'], 'setting_uuids': []})
        mcp.tool('import_image', {'path': str(image), **prepared['image_write_back'], 'prompt': prepared['generation_prompt'], 'preparation': prepared['preparation'], 'fit_mode': 'pad'})
        current = mcp.tool('pages', {'action': 'get', 'chapter_uuid': chapters[0]['uuid'], 'page_uuid': current['uuid']})
        mcp.tool('pages', {'action': 'select_image', 'chapter_uuid': chapters[0]['uuid'], 'page_uuid': current['uuid'], 'variant_uuid': adopted, 'expected_revision': current['revision']})
        listing = mcp.tool('settings', {'action': 'list'})
        setting = mcp.tool('settings', {'action': 'create', 'asset_type': 'character', 'title': '小狐狸', 'summary': '橘红毛发，黄色雨衣，温柔眼神。', 'expected_revision': listing['premise']['revision']})
        mcp.tool('import_image', {'path': str(image), 'target_uuid': setting['uuid'], 'slot': 'setting_image', 'expected_revision': setting['revision']})
        project_uuid = init['project']['uuid']
        requests = {'book': {'name': 'review_book', 'arguments': {}},
                    'chapters': {'name': 'review_chapters', 'arguments': {}},
                    'chapter': {'name': 'review_chapter', 'arguments': {'chapter_uuid': chapters[0]['uuid']}},
                    'page': {'name': 'review_page', 'arguments': {'chapter_uuid': chapters[0]['uuid'], 'page_uuid': pages[1]['uuid']}},
                    'empty': {'name': 'review_chapter', 'arguments': {'chapter_uuid': chapters[2]['uuid']}},
                    'legacy_book': {'name': 'review_book', 'arguments': {'mode': 'preview'}},
                    'legacy_chapter': {'name': 'review_book', 'arguments': {'mode': 'preview', 'chapter_uuid': chapters[0]['uuid']}},
                    'legacy': {'name': 'review_book', 'arguments': {'chapter_uuid': chapters[0]['uuid'], 'limit': 2}}}
        tools = mcp.rpc('tools/list', {})['tools']
        uri = next(t for t in tools if t['name'] == 'review_book')['_meta']['ui']['resourceUri']
        resource = mcp.rpc('resources/read', {'uri': uri})
        (root / 'review.html').write_text(resource['contents'][0]['text'])
        empty_project = root / 'empty-project'
        empty_project.mkdir()
        empty_mcp = MCP(binary, empty_project, root / 'data')
        empty_mcp.tool('project_init', {'idea': '空作品测试', 'name': '空作品'})
        manifest = {'root': str(root), 'project': str(project), 'binary': str(binary), 'data_dir': str(root/'data'),
                    'project_uuid': project_uuid, 'chapter_uuid': chapters[0]['uuid'], 'page_uuid': pages[1]['uuid'],
                    'adopted_variant_uuid': adopted, 'requests': requests, 'chapters': [c['uuid'] for c in chapters],
                    'pages': [p['uuid'] for p in pages], 'resource_uri': uri}
        (root / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
        allowed = {t['name'] for t in tools if t['name'].startswith('review_')} | {'read_app_image', 'read_app_image_detail', 'watch_app_changes', 'read_app_resource', 'write_app_resource', 'read_app_review'}
    # Reused fixtures can predate the retired-mode compatibility cases.
    requests.setdefault('legacy_book', {'name': 'review_book', 'arguments': {'mode': 'preview'}})
    requests.setdefault('legacy_chapter', {'name': 'review_book', 'arguments': {'mode': 'preview', 'chapter_uuid': chapters[0]['uuid']}})
    class Handler(BaseHTTPRequestHandler):
        def reply(self, body, kind='application/json'):
            if not isinstance(body, bytes): body = json.dumps(body, ensure_ascii=False).encode()
            self.send_response(200); self.send_header('Content-Type', kind); self.send_header('Content-Length', str(len(body))); self.end_headers()
            try: self.wfile.write(body)
            except BrokenPipeError: pass
        def do_GET(self):
            if self.path == '/': return self.reply(PAGE.encode(), 'text/html')
            if self.path.startswith('/review.html'): return self.reply((root/'review.html').read_bytes(), 'text/html')
            if self.path.startswith('/view/'):
                view = self.path.rsplit('/', 1)[-1]
                if view == 'legacy':
                    legacy = mcp.rpc('tools/call', {'name':'read_app_review','arguments':{'view':'reader', **requests['legacy']['arguments']}})
                    envelope = legacy['_meta']['lumi/app_response']
                    d=envelope['data'];d.pop('contract_version',None);d['view']='book'
                    # Reconstruct the historical project-targeted chapter snapshot.
                    d['target_uuid']=project_uuid;d['scope']='chapter';d['open_url']+='/preview'
                    d['request']={'name':'review_book','arguments':requests['legacy']['arguments']}
                    result={'structuredContent':envelope,'content':[{'type':'text','text':json.dumps(envelope,ensure_ascii=False)}]}
                else:
                    result = empty_mcp.rpc('tools/call', requests['book']) if view == 'empty_book' else mcp.rpc('tools/call', requests[view])
                return self.reply(result)
            return self.send_error(404)
        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', '0'))) or '{}')
            if self.path == '/mutate':
                current = mcp.tool('pages', {'action': 'get', 'chapter_uuid': chapters[0]['uuid'], 'page_uuid': pages[1]['uuid']})
                return self.reply(mcp.tool('pages', {'action': 'update', 'chapter_uuid': chapters[0]['uuid'], 'page_uuid': pages[1]['uuid'], 'description_md': '实时校准后的测试正文。', 'expected_revision': current['revision']}))
            if self.path == '/delete':
                current = mcp.tool('pages', {'action': 'get', 'chapter_uuid': chapters[0]['uuid'], 'page_uuid': pages[2]['uuid']})
                return self.reply(mcp.tool('pages', {'action': 'delete', 'chapter_uuid': chapters[0]['uuid'], 'page_uuid': current['uuid'], 'expected_revision': current['revision']}))
            if self.path == '/facts':
                current = mcp.tool('pages', {'action': 'get', 'chapter_uuid': chapters[0]['uuid'], 'page_uuid': pages[1]['uuid']})
                return self.reply({'adopted_variant_uuid': current['current_image']['uuid'], 'revision': current['revision'], 'text': current['description_md']})
            if self.path != '/call' or body.get('name') not in allowed: return self.send_error(403)
            if body.get('fixture_fail'):
                return self.reply({'isError': True, '_meta': {'lumi/app_response':{'success': False, 'data': None, 'error': {'code': 'fixture_failure', 'message': '测试读取失败'}}}, 'structuredContent':{'success':False,'data':None,'error':{'code':'fixture_failure','message':'测试读取失败'}}})
            target_project = body.get('arguments', {}).get('project_uuid') if body.get('name') == 'watch_app_changes' else body.get('fixture_project', project_uuid)
            target = empty_mcp if target_project != project_uuid else mcp
            result = target.rpc('tools/call', {'name': body['name'], 'arguments': body.get('arguments', {}), **({'_meta':body['_meta']} if '_meta' in body else {})})
            with (root/'browser-requests.jsonl').open('a') as log:
                log.write(json.dumps({'name': body['name'], 'arguments': body.get('arguments', {}), 'success': result.get('structuredContent', {}).get('success'), 'error': result.get('structuredContent', {}).get('error'), 'model_bytes':len(json.dumps({'content':result.get('content'),'structuredContent':result.get('structuredContent')},ensure_ascii=False).encode()), 'ui_bytes':len(json.dumps(result.get('_meta',{}),ensure_ascii=False).encode())})+'\n')
            self.reply(result)
        def log_message(self, *args): pass
    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    manifest['browser_url'] = 'http://127.0.0.1:' + str(server.server_port)
    (root / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
    print(json.dumps(manifest, ensure_ascii=False), flush=True)
    try: server.serve_forever()
    finally:
        server.server_close()
        for session in (mcp, empty_mcp): session.proc.stdin.close(); session.proc.wait(timeout=10); session.log.close()

if __name__ == '__main__': main()
