#!/usr/bin/env python3
"""Freeze a replacement fixture against the SAME approved temporary project.

Never changes the installed cache, marketplace index, permissions or project data.
The output plugin_source can be reviewed before updating the marketplace index.
"""
import argparse
import base64
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import tempfile

from prepare_step5 import HERE, REPO, MCP, write


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--base-manifest', required=True, type=Path)
    parser.add_argument('--version', required=True)
    args = parser.parse_args()
    if not all(c.isdigit() or c == '.' for c in args.version):
        parser.error('version must contain only digits and dots')
    base = json.loads(args.base_manifest.read_text())
    root = Path(tempfile.mkdtemp(prefix='lumi-step5-v' + args.version + '-', dir='/tmp')).resolve()
    binary = root / 'lumi'
    shutil.copy2(args.binary.resolve(), binary)
    plugin = Path(base['root']) / 'marketplace/plugins' / (base['plugin'] + '-v' + args.version)
    if plugin.exists():
        raise RuntimeError('Refusing to overwrite an existing immutable plugin source')
    mcp = MCP(binary, Path(base['project']), Path(base['data_dir']))
    try:
        overview = mcp.tool('project_overview', {})
        if overview['project']['uuid'] != base['project_uuid']:
            raise RuntimeError('Project identity changed')
        for view, request in base['requests'].items():
            write(root / 'results' / (view + '.json'), mcp.rpc('tools/call', request))
        tools = mcp.rpc('tools/list', {})
        write(root / 'results/tools.json', tools)
        uri = next(t for t in tools['tools'] if t['name'] == 'review_book')['_meta']['ui']['resourceUri']
        resource = mcp.rpc('resources/read', {'uri': uri})
        write(root / 'results/resource.json', resource)
        (root / 'review.html').write_text(resource['contents'][0]['text'])
        page = json.loads((root / 'results/page.json').read_text())['structuredContent']['data']
        boundaries = []
        for edge in (256, 384, 1024):
            result = mcp.rpc('tools/call', {'name': 'read_app_image', 'arguments': {'asset_uuid': page['items'][0]['asset_uuid'], 'max_edge': edge}})
            data = result['structuredContent']['data']
            jpeg = base64.b64decode(data['data_uri'].split(',', 1)[1])
            size = len(json.dumps(result, ensure_ascii=False, separators=(',', ':')).encode())
            assert len(jpeg) <= 120 * 1024 and size <= 256 * 1024
            (root / ('preview-' + str(edge) + '.jpg')).write_bytes(jpeg)
            boundaries.append({'requested_max_edge': edge, 'width': data['width'], 'height': data['height'], 'jpeg_bytes': len(jpeg), 'mcp_result_bytes': size, 'sha256': hashlib.sha256(jpeg).hexdigest()})
    finally:
        mcp.close()
    plugin.mkdir(parents=True)
    shutil.copy2(HERE / 'log_stdio.py', plugin / 'log_stdio.py')
    launcher = plugin / 'launch.py'
    launcher.write_text('import runpy,sys\nsys.argv=[' + repr(str(plugin / 'log_stdio.py')) + ',' + repr(str(root)) + ']\nrunpy.run_path(sys.argv[0],run_name="__main__")\n')
    write(plugin / '.codex-plugin/plugin.json', {'name': base['plugin'], 'version': args.version, 'description': 'Disposable real Lumi MCP Apps review acceptance'})
    write(plugin / '.mcp.json', {'mcpServers': {'lumi_review': {'command': '/usr/bin/python3', 'args': [str(launcher)], 'env': {'PYTHONDONTWRITEBYTECODE': '1'}}}})
    sources = sorted({*REPO.glob('cmd/lumi/*.go'), *REPO.glob('internal/textmcp/*.go'), *REPO.glob('internal/realtime/*.go'), *REPO.glob('web/mcp-app/*'), REPO / 'internal/textmcp/appdist/review.html', REPO / 'web/package.json'})
    manifest = {**base, 'root': str(root), 'binary': str(binary), 'binary_sha256': sha(binary), 'plugin_version': args.version, 'plugin_source': str(plugin), 'resource_uri': uri, 'product_boundaries': boundaries, 'host_status': 'not_yet_tested', 'final_build_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': {str(p.relative_to(REPO)): sha(p) for p in sources if p.is_file()}, 'upgrades_manifest': str(args.base_manifest.resolve())}
    write(root / 'manifest.json', manifest)
    print(json.dumps({'manifest': str(root / 'manifest.json'), 'plugin_source': str(plugin), 'resource_uri': uri, 'binary_sha256': manifest['binary_sha256']}, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
