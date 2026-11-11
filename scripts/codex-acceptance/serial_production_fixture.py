#!/usr/bin/env python3
"""Create a disposable two-chapter, three-scene project for host acceptance.

Only real Lumi MCP calls; no provider or host image generation. The repository
icon and synthetic review evidence are explicit protocol doubles, not artwork.
The manifest can be passed to log_stdio.py for a real host connection.
"""
import argparse
import json
import subprocess
import tempfile
from pathlib import Path

from hierarchy_fixture import MCP, REPO


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path)
    parser.add_argument('--verify', type=Path, help='Reconnect and inspect an existing fixture without generating or importing')
    parser.add_argument('--snapshot', type=Path, help='Read current page identities and review state during real host acceptance')
    args = parser.parse_args()
    if args.verify or args.snapshot:
        manifest_path = (args.verify or args.snapshot).resolve(strict=True)
        root = manifest_path.parent
        if root.parent != Path(tempfile.gettempdir()).resolve() or not root.name.startswith('lumi-serial-host-'):
            parser.error('--verify only accepts this disposable fixture')
        manifest = json.loads(manifest_path.read_text())
        mcp = MCP(Path(manifest['binary']), Path(manifest['project']), Path(manifest['data_dir']))
        try:
            current = [mcp.tool('pages', {'action': 'get', 'chapter_uuid': p['chapter_uuid'],
                                        'page_uuid': p['page_uuid'], 'summary_only': True}) for p in manifest['pages']]
            if args.verify:
                assert [p['review_status'] for p in current] == ['passed', 'pending', 'missing_image', 'missing_image']
                assert [p['variant_uuid'] for p in current[:2]] == manifest['initial_variants']
            fields = ('uuid', 'revision', 'variant_uuid', 'version_no', 'review_status', 'review_revision')
            print(json.dumps({'manifest': str(manifest_path), 'reconnected': True,
                              'page_statuses': [p['review_status'] for p in current],
                              'pages': [{**identity, **{k: p[k] for k in fields if k in p}}
                                        for identity, p in zip(manifest['pages'], current)],
                              'initial_versions_verified': bool(args.verify),
                              'generation_or_import_calls': 0}, ensure_ascii=False))
        finally:
            mcp.proc.stdin.close()
            mcp.proc.wait(timeout=10)
            mcp.log.close()
        return
    if not args.binary:
        parser.error('--binary is required when not using --verify')
    binary = args.binary.resolve(strict=True)
    root = Path(tempfile.mkdtemp(prefix='lumi-serial-host-')).resolve()
    project, data = root / 'project', root / 'data'
    project.mkdir()
    mcp = MCP(binary, project, data)
    try:
        init = mcp.tool('project_init', {'idea': '两章串行制作协议验收；测试替身图不代表美术质量', 'name': '串行制作隔离验收'})
        setup = mcp.tool('project_setup', {'action': 'update', 'expected_revision': init['setup']['revision'],
                         'project_name': '串行制作隔离验收', 'overall_style': '协议测试替身图；不宣称真实文字、服装、道具或画风通过',
                         'picture_book': {'format': 'classic_picture_book', 'aspect_ratio': {'mode': 'square'}}})
        mcp.tool('project_setup', {'action': 'finalize', 'expected_revision': setup['revision']})
        chapters, pages = [], []
        for number, titles in [(10, ['码头：到达', '码头：钥匙', '灯塔：启程']), (11, ['海上：接续'])]:
            detail = mcp.tool('project_overview', {})['project']
            chapter = mcp.tool('chapters', {'action': 'create', 'chapter_code': f'vol01.ch{number}',
                               'title': f'第{number}章', 'content': '小悠穿红外套，右手拿铜钥匙，从码头经过灯塔后出海。',
                               'expected_revision': detail['revision']})
            chapters.append(chapter)
            for title in titles:
                listing = mcp.tool('pages', {'action': 'list', 'chapter_uuid': chapter['uuid'], 'summary_only': True})
                page = mcp.tool('pages', {'action': 'create', 'chapter_uuid': chapter['uuid'], 'title': title,
                                'description_md': '小悠：钥匙在这里。',
                                'storyboard_md': f'{title}，小悠穿红外套，右手持铜钥匙。旁白：继续前进。对白逐字：钥匙在这里。',
                                'expected_revision': listing['state']['revision']})
                pages.append({'chapter_uuid': chapter['uuid'], 'page_uuid': page['uuid'], 'title': title})
        listing = mcp.tool('settings', {'action': 'list'})
        setting = mcp.tool('settings', {'action': 'create', 'asset_type': 'character', 'title': '小悠：红外套与铜钥匙',
                           'summary': '小悠穿红外套，右手持铜钥匙。图为协议测试替身，不作为真实视觉核对依据。',
                           'expected_revision': listing['premise']['revision']})
        setting_image = mcp.tool('import_image', {'path': str(REPO / 'codex-plugins/lumi/assets/icon.png'),
                                'target_uuid': setting['uuid'], 'slot': 'setting_image', 'expected_revision': setting['revision']})
        adopted = []
        for index, page in enumerate(pages[:2]):
            readiness = mcp.tool('image_target', {'target_uuid': page['page_uuid'], 'slot': 'page_image',
                                 'setting_uuids': [setting['uuid']]})['setting_readiness']
            item = readiness['items'][0]
            evidence = {'setting_uuid': setting['uuid'], 'variant_uuid': item['variant_uuid'],
                        'expected_revision': item['expected_revision'], 'style_digest': readiness['style_digest'],
                        'verdict': 'passed', 'reference_basis': '协议测试替身，不代表真实视觉核对',
                        'comparison': '只验证版本与审阅持久化，不宣称美术符合分镜'}
            prepared = mcp.tool('prepare_page_image', {'page_uuid': page['page_uuid'], 'setting_uuids': [setting['uuid']],
                                'setting_reviews': [evidence]})
            imported = mcp.tool('import_image', {'path': str(REPO / 'codex-plugins/lumi/assets/icon.png'),
                               **prepared['image_write_back'], 'prompt': prepared['generation_prompt'],
                               'preparation': prepared['preparation']})
            if index == 0:
                mcp.tool('pages', {'action': 'record_image_review', **{k: page[k] for k in ('chapter_uuid', 'page_uuid')},
                         'variant_uuid': imported['variant_uuid'], 'expected_revision': imported['revision'],
                         'verdict': 'passed', 'observation': '自动化协议替身通过；不代表真实文字或画面核对'})
            adopted.append(imported['variant_uuid'])
        # Upgrade only official project guidance; no global host configuration.
        subprocess.run([str(binary), 'init', '--data-dir', str(data), '--json'], cwd=project,
                       check=True, stdout=subprocess.DEVNULL)
        manifest = {'root': str(root), 'binary': str(binary), 'project': str(project), 'data_dir': str(data),
                    'project_uuid': init['project']['uuid'], 'chapters': [c['uuid'] for c in chapters],
                    'pages': pages, 'scenes': [[p['page_uuid'] for p in pages[:2]], [pages[2]['page_uuid']], [pages[3]['page_uuid']]],
                    'setting_uuid': setting['uuid'], 'setting_variant_uuid': setting_image['variant_uuid'],
                    'initial_variants': adopted, 'image_generation': 'not_run', 'visual_reviews': 'synthetic_protocol_doubles'}
        manifest_path = root / 'manifest.json'
        manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps({'manifest': str(manifest_path), 'project': str(project),
                          'chapters': manifest['chapters'], 'page_statuses': ['passed', 'pending', 'missing_image', 'missing_image']}, ensure_ascii=False))
    finally:
        mcp.proc.stdin.close()
        mcp.proc.wait(timeout=10)
        mcp.log.close()


if __name__ == '__main__':
    main()
