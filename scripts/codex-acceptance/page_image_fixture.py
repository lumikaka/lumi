#!/usr/bin/env python3
"""Isolated real MCP page preparation/import with an explicit host-image stub.

First prepare, then view the printed reference_image_path with the host viewer.
Complete using --complete MANIFEST --viewed-path EXACT_PATH. No model call,
provider configuration, plugin cache write or real image generation is made.
"""
import argparse
import hashlib
import json
import shutil
import subprocess
import tempfile
from pathlib import Path

from hierarchy_fixture import MCP, REPO


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path)
    parser.add_argument('--complete', type=Path)
    parser.add_argument('--viewed-path', type=Path)
    args = parser.parse_args()
    if args.complete:
        manifest = args.complete.resolve()
        state = json.loads(manifest.read_text())
        root = manifest.parent
        if root.parent != Path(tempfile.gettempdir()).resolve() or not root.name.startswith('lumi-page-image-'):
            raise ValueError('Only this isolated fixture may be completed')
        prepared = state['prepared']
        reference = Path(prepared['reference_image_path'])
        if args.viewed_path != reference or not reference.is_file():
            raise ValueError('Read the exact returned path with the host viewer first')
        if hashlib.sha256(reference.read_bytes()).hexdigest() != state['collage_sha256']:
            raise ValueError('Reference bytes changed')
        # Explicit host imagegen stand-in. Its only reference is the same
        # absolute path that the agent viewed; its prompt is passed unchanged.
        host_input = {'prompt': prepared['generation_prompt'], 'referenced_image_paths': [str(reference)]}
        output = root / 'actual-host-stub-output.png'
        shutil.copyfile(REPO / 'codex-plugins/lumi/assets/icon.png', output)
        mcp = MCP(Path(state['binary']), Path(state['project']), Path(state['data_dir']))
        try:
            imported = mcp.tool('import_image', {'path': str(output), **prepared['image_write_back'],
                                'prompt': host_input['prompt'], 'preparation': prepared['preparation']})
            state.update(host_input=host_input, imported=imported, real_host_image_generation=False)
            manifest.write_text(json.dumps(state, ensure_ascii=False, indent=2))
            print(json.dumps({'manifest': str(manifest), 'variant_uuid': imported['variant_uuid'],
                              'reference_count': len(host_input['referenced_image_paths']),
                              'digest': prepared['preparation']['digest'],
                              'same_prompt': imported['asset']['metadata']['original_prompt'] == host_input['prompt'],
                              'real_host_image_generation': False}, ensure_ascii=False))
        finally:
            mcp.proc.terminate()
            mcp.proc.wait(timeout=10)
        return
    if not args.binary:
        parser.error('--binary is required for preparation')
    binary = args.binary.resolve()
    root = Path(tempfile.mkdtemp(prefix='lumi-page-image-')).resolve()
    project, data = root / 'project', root / 'data'
    project.mkdir()
    mcp = MCP(binary, project, data)
    try:
        initialized = mcp.tool('project_init', {'idea': '四项设定、中文标签和宿主合图测试', 'name': '合图验证', 'generation_language': 'zh-Hans'})
        setup = mcp.tool('project_setup', {'action': 'update', 'project_name': '合图验证', 'overall_style': '保持原有画风，完整主体和对白',
                         'expected_revision': initialized['setup']['revision'],
                         'picture_book': {'format': 'classic_picture_book', 'aspect_ratio': {'mode': 'landscape'}}})
        mcp.tool('project_setup', {'action': 'finalize', 'expected_revision': setup['revision']})
        settings = []
        for title, kind in [('月光小狐狸', 'character'), ('黄昏灯塔', 'scene'), ('银色邮包', 'prop'), ('星空地图', 'prop')]:
            listing = mcp.tool('settings', {'action': 'list'})
            setting = mcp.tool('settings', {'action': 'create', 'title': title, 'asset_type': kind,
                               'summary': '测试替身图，仅验证来源和合图，不代表实际美术', 'expected_revision': listing['premise']['revision']})
            mcp.tool('import_image', {'path': str(REPO / 'codex-plugins/lumi/assets/icon.png'),
                     'target_uuid': setting['uuid'], 'slot': 'setting_image', 'expected_revision': setting['revision']})
            settings.append(setting['uuid'])
        project_detail = mcp.tool('project_overview', {})['project']
        chapter = mcp.tool('chapters', {'action': 'create', 'chapter_code': 'vol01.ch01', 'title': '四项设定', 'expected_revision': project_detail['revision']})
        pages = mcp.tool('pages', {'action': 'list', 'chapter_uuid': chapter['uuid']})
        page = mcp.tool('pages', {'action': 'create', 'chapter_uuid': chapter['uuid'], 'title': '合图验证页',
                        'description_md': '小狐狸说：“天亮了！”',
                        'storyboard_md': '小狐狸背着银色邮包站在黄昏灯塔下，手持星空地图。完整保留对白：“天亮了！”',
                        'expected_revision': pages['state']['revision']})
        result = mcp.rpc('tools/call', {'name': 'prepare_page_image', 'arguments': {
                        'page_uuid': page['uuid'], 'setting_uuids': settings, 'additional_direction': '保留主体和对白，预留安全边距。'}})
        assert len(result['content']) == 1 and result['content'][0]['type'] == 'text'
        assert result['structuredContent']['success'], result
        prepared = result['structuredContent']['data']
        reference = Path(prepared['reference_image_path'])
        assert reference.is_absolute() and reference.is_file() and not reference.is_relative_to(project)
        review = mcp.tool('review_page', {'chapter_uuid': chapter['uuid'], 'page_uuid': page['uuid']})
        status = json.loads(subprocess.check_output([str(binary), 'status', '--data-dir', str(data), '--json'], text=True))['data']
        state = {'binary': str(binary), 'project': str(project), 'data_dir': str(data), 'prepared': prepared,
                 'project_uuid': project_detail['uuid'], 'chapter_uuid': chapter['uuid'], 'page_uuid': page['uuid'],
                 'web_url': review['open_url'], 'endpoint': status['endpoint'],
                 'collage_sha256': hashlib.sha256(reference.read_bytes()).hexdigest(), 'real_host_image_generation': False}
        manifest = root / 'manifest.json'
        manifest.write_text(json.dumps(state, ensure_ascii=False, indent=2))
        print(json.dumps({'manifest': str(manifest), 'reference_image_path': str(reference), 'sources': prepared['sources'],
                          'web_url': state['web_url'], 'digest': prepared['preparation']['digest']}, ensure_ascii=False))
    finally:
        mcp.proc.terminate()
        mcp.proc.wait(timeout=10)


if __name__ == '__main__':
    main()
