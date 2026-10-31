#!/usr/bin/env python3
"""Create a disposable real Lumi project and immutable plugin fixture. Never installs it."""
import argparse
import base64
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import plistlib
import shlex
import shutil
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[3]

def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')

class MCP:
    def __init__(self, binary, root, data):
        self.log = (data.parent / 'cli-stderr.log').open('a')
        self.proc = subprocess.Popen([str(binary), 'mcp', '--data-dir', str(data)], cwd=root,
                                     stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=self.log, text=True)
        self.serial = 0
        self.rpc('initialize', {'protocolVersion': '2025-06-18', 'capabilities': {}, 'clientInfo': {'name': 'lumi-step5-acceptance', 'version': '1'}})
        self.proc.stdin.write(json.dumps({'jsonrpc': '2.0', 'method': 'notifications/initialized'}) + '\n'); self.proc.stdin.flush()
    def rpc(self, method, params):
        self.serial += 1
        self.proc.stdin.write(json.dumps({'jsonrpc': '2.0', 'id': self.serial, 'method': method, 'params': params}) + '\n'); self.proc.stdin.flush()
        while True:
            line = self.proc.stdout.readline()
            if not line: raise RuntimeError('MCP disconnected')
            reply = json.loads(line)
            if reply.get('id') == self.serial:
                if 'error' in reply: raise RuntimeError(reply['error'])
                return reply['result']
    def tool(self, name, args):
        result = self.rpc('tools/call', {'name': name, 'arguments': args})
        envelope = result['structuredContent']
        if not envelope['success']: raise RuntimeError(envelope['error'])
        return envelope['data']
    def close(self):
        self.proc.stdin.close(); self.proc.wait(timeout=10); self.log.close()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    args = parser.parse_args()
    root = Path(tempfile.mkdtemp(prefix='lumi-step5-', dir='/tmp')).resolve()
    binary = root / 'lumi'; shutil.copy2(args.binary.resolve(), binary)
    project = root / 'project'; project.mkdir(); data = root / 'data'
    mcp = MCP(binary, project, data)
    init = mcp.tool('project_init', {'idea': '小狐狸把星星送回天空', 'name': '星星回家 · 审阅验收'})
    setup = mcp.tool('project_setup', {'action': 'update', 'expected_revision': init['setup']['revision'], 'project_name': '星星回家 · 审阅验收', 'generation_language': 'zh-Hans', 'overall_style': '水彩，靛蓝雨夜，暖金色星光', 'generation_brief': '一只穿雨衣的小狐狸帮助迷路的星星回家。', 'picture_book': {'format': 'classic_picture_book', 'aspect_ratio': {'mode': 'landscape'}, 'large_image_minimal_text': False}})
    mcp.tool('project_setup', {'action': 'finalize', 'expected_revision': setup['revision']})
    overview = mcp.tool('project_overview', {})
    chapter = mcp.tool('chapters', {'action': 'create', 'chapter_code': 'vol01.ch01', 'title': '雨夜里的小小光芒', 'expected_revision': overview['project']['revision']})
    images = [REPO / 'docs/features/codex-plugins/spike/evidence' / name for name in ('fox-original.png', 'fox-boots.png')]
    pages = []
    for i, title in enumerate(('一颗迷路的星星', '穿过雨夜的森林', '抬头就是家')):
        listing = mcp.tool('pages', {'action': 'list', 'chapter_uuid': chapter['uuid']})
        page = mcp.tool('pages', {'action': 'create', 'chapter_uuid': chapter['uuid'], 'title': title, 'description_md': f'第 {i + 1} 页：小狐狸说：“别怕，我陪你回家。”\n雨滴在树叶上，唱起轻轻的歌。', 'storyboard_md': '穿黄色雨衣的小狐狸，橘红色毛发，温柔的眼神。星星发出金色光芒，照亮蓝色雨夜。水彩质感，画面上方留出文字空间。', 'expected_revision': listing['state']['revision']})
        pages.append(page)
        if i < 2:
            for image in (images if i == 0 else images[:1]):
                target = mcp.tool('image_target', {'target_uuid': page['uuid'], 'slot': 'page_image'})
                mcp.tool('import_image', {'path': str(image), 'target_uuid': page['uuid'], 'slot': 'page_image', 'expected_revision': target['expected_revision'], 'prompt': '真实 Codex App 宿主生成的小狐狸水彩图，用作 Lumi 产品预览验收。'})
    settings = mcp.tool('settings', {'action': 'list'})
    setting = mcp.tool('settings', {'action': 'create', 'asset_type': 'character', 'title': '小狐狸', 'summary': '橘红色毛发、白色胸口，穿黄色雨衣。遇到黑暗时会害怕，但愿意为了朋友再走一步。', 'expected_revision': settings['premise']['revision']})
    mcp.tool('import_image', {'path': str(images[0]), 'target_uuid': setting['uuid'], 'slot': 'setting_image', 'expected_revision': setting['revision'], 'prompt': '角色参考图'})
    settings = mcp.tool('settings', {'action': 'list'})
    mcp.tool('settings', {'action': 'create', 'asset_type': 'scene', 'title': '雨夜森林', 'summary': '靛蓝色的树影与湿润的苔藓。星光在积水里映出回家的路。暂无参考图。', 'expected_revision': settings['premise']['revision']})
    page_args = {'chapter_uuid': chapter['uuid'], 'page_uuid': pages[0]['uuid']}
    requests = {'book': {'name': 'review_book', 'arguments': {}}, 'page': {'name': 'review_page', 'arguments': page_args}, 'candidates': {'name': 'review_candidates', 'arguments': {'kind': 'page', **page_args}}, 'settings': {'name': 'review_settings', 'arguments': {}}}
    for view, request in requests.items(): write(root / 'results' / f'{view}.json', mcp.rpc('tools/call', request))
    tool = next(t for t in mcp.rpc('tools/list', {})['tools'] if t['name'] == 'review_book')
    uri = tool['_meta']['ui']['resourceUri']
    resource = mcp.rpc('resources/read', {'uri': uri}); write(root / 'results/resource.json', resource)
    (root / 'review.html').write_text(resource['contents'][0]['text'])
    review = json.loads((root / 'results/page.json').read_text())['structuredContent']['data']
    asset = review['items'][0]['asset_uuid']
    boundaries = []
    for edge in (256, 384, 1024):
        result = mcp.rpc('tools/call', {'name': 'read_app_image', 'arguments': {'asset_uuid': asset, 'max_edge': edge}})
        d = result['structuredContent']['data']; jpeg = base64.b64decode(d['data_uri'].split(',', 1)[1])
        assert len(jpeg) <= 120 * 1024 and len(json.dumps(result, ensure_ascii=False).encode()) <= 256 * 1024
        (root / f'preview-{edge}.jpg').write_bytes(jpeg)
        boundaries.append({'requested_max_edge': edge, 'width': d['width'], 'height': d['height'], 'jpeg_bytes': len(jpeg), 'mcp_result_bytes': len(json.dumps(result, ensure_ascii=False, separators=(',', ':')).encode()), 'sha256': hashlib.sha256(jpeg).hexdigest()})
    mcp.close()
    plugin_name = 'lumi-review-' + root.name.rsplit('-', 1)[-1]
    marketplace = root / 'marketplace'; plugin = marketplace / 'plugins' / plugin_name
    plugin.mkdir(parents=True)
    launcher = plugin / 'launch.py'
    shutil.copy2(HERE / 'log_stdio.py', plugin / 'log_stdio.py')
    # A fixed cwd is solely for this disposable fixture. Production lumi mcp
    # always binds its actual startup cwd and has no project-path tool argument.
    launcher.write_text('import runpy,sys\nsys.argv=[' + repr(str(plugin / 'log_stdio.py')) + ',' + repr(str(root)) + ']\nrunpy.run_path(sys.argv[0],run_name="__main__")\n')
    write(plugin / '.codex-plugin/plugin.json', {'name': plugin_name, 'version': '0.5.0', 'description': 'Disposable real Lumi MCP Apps review acceptance'})
    write(plugin / '.mcp.json', {'mcpServers': {'lumi_review': {'command': '/usr/bin/python3', 'args': [str(launcher)], 'env': {'PYTHONDONTWRITEBYTECODE': '1'}}}})
    write(marketplace / '.agents/plugins/marketplace.json', {'name': plugin_name + '-local', 'interface': {'displayName': 'Lumi Step 5 review acceptance'}, 'plugins': [{'name': plugin_name, 'source': {'source': 'local', 'path': './plugins/' + plugin_name}, 'policy': {'installation': 'AVAILABLE', 'authentication': 'ON_INSTALL'}, 'category': 'Productivity'}]})
    app = Path('/Applications/ChatGPT.app'); info = plistlib.loads((app / 'Contents/Info.plist').read_bytes()); codex = app / 'Contents/Resources/codex-cli/bin/codex'
    manifest = {'root': str(root), 'binary': str(binary), 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'project': str(project), 'data_dir': str(data), 'project_uuid': init['project']['uuid'], 'chapter_uuid': chapter['uuid'], 'page_uuid': pages[0]['uuid'], 'setting_uuid': setting['uuid'], 'resource_uri': uri, 'requests': requests, 'product_boundaries': boundaries, 'app_version': info['CFBundleShortVersionString'], 'app_build': info['CFBundleVersion'], 'bundled_cli_version': subprocess.check_output([str(codex), '--version'], text=True).strip(), 'host_status': 'not_yet_tested', 'plugin': plugin_name, 'marketplace': plugin_name + '-local', 'register_command': shlex.join([str(codex), 'plugin', 'marketplace', 'add', str(marketplace)]), 'remove_command': shlex.join([str(codex), 'plugin', 'marketplace', 'remove', plugin_name + '-local'])}
    write(root / 'manifest.json', manifest)
    print(json.dumps(manifest, ensure_ascii=False, indent=2))
if __name__ == '__main__': main()
