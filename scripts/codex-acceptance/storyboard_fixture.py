#!/usr/bin/env python3
"""Create isolated, identical novel inputs for a real Codex storyboard A/B run.

Only Lumi MCP reads/writes project facts. No image generation or fake reviews.
Raw host history and usage belong outside the repository beside manifest.json.
"""
import argparse
import hashlib
import json
import shutil
import subprocess
import tempfile
from pathlib import Path

from hierarchy_fixture import MCP, REPO


NOVEL = [
    ("vol01.ch01", "雨站的钥匙", """雨水沿候车棚的铁皮边缘滴落。林遥把红色外套的袖口卷起，左手护着一只蓝色布包。包里只有一张车票、一块干面包和母亲的信。她在旧车站等了整整一小时，最后一班车仍没有出现。站台尽头的钟停在七点，秒针被一根细蛛丝牵着，风来时轻轻颤动。
陈渡从售票窗后探出头。他穿灰色雨衣，右手举着一把银钥匙，钥匙柄上刻着半个月亮。林遥认出那是母亲信里画过的图案，但没有伸手。她记得母亲在信末提醒过：先问清钥匙要开的是哪一道门。售票窗旁的木牌已经褪色，只剩一个向东的箭头。
“这把钥匙是你母亲留下的。”陈渡说。
“她要我去哪儿？”林遥问。
陈渡走出小屋，用空着的左手指向东边。雨幕之外，白色灯塔的红顶时隐时现。多年前，林遥曾在那儿见过母亲修理灯具；那时灯塔每到夜晚就会照亮浅滩，如今却已经熄灭三天。车站没有别人，远处只有海浪撞击石阶的声音。
“先到灯塔，别把蓝包落下。”陈渡说。
他把银钥匙放进林遥空着的右手。她合拢手指，冰凉的钥匙柄压住掌心。蓝包仍由她左手提着，陈渡的双手都空了。他把售票窗关好，跟在她左后方，离开候车棚。林遥回头看了一眼停住的钟，把车票留在包内，没有等下一班车。
站台东侧的小路通向堤岸。林遥沿着石阶往上走，红外套在灰色雨幕里格外醒目。她没有打开蓝包，也没有把钥匙交还给陈渡。堤岸上积着浅浅的水，脚步一落就泛开细小的波纹。白塔越来越近，红顶下却没有一丝灯光。
“我会一直带着它。”林遥说。
两人在灯塔外的门廊停下。铁门紧闭，门锁上也刻着半个月亮。林遥的右手仍握银钥匙，左手仍提蓝包；陈渡站在她左后方，灰色雨衣的下摆滴着水。她举起钥匙，朝锁孔比了一下，尚未开门。雨从两人身后落进石阶缝隙，海面暗得看不见归船。
"""),
    ("vol01.ch02", "灯亮以后", """林遥站在白色灯塔的门廊，红外套的袖口仍卷着。她右手的银钥匙对准半月形锁孔，左手的蓝包贴在身侧。陈渡穿灰色雨衣，停在她左后方。门里没有声音。她把钥匙转了半圈，沉重的铁门向内开了一条缝，潮湿的石头气味从里面涌出来。
“门开了，跟紧我。”林遥说。
她拔下钥匙，把它收进红外套右侧口袋，然后仍用右手推门。蓝包没有换手。陈渡扶住门边，让她先走。塔内的旋梯向上盘绕，扶手上满是水汽。林遥一阶一阶往上走，没有奔跑。每到转角，她都停一下，等陈渡赶上再继续。
灯室的玻璃被雨水洗得透明。旧灯旁有一个铜制底座，底座中央缺了一枚小小的圆镜。母亲的信就装在蓝包内，信封背面曾画过这个底座。林遥把蓝包放在窗下的干木桌上，解开包口，拿出折好的信。车票和面包留在包里，没有掉到地上。
“信背面画着什么？”陈渡问。
林遥展开信，让他看背面的图。图上不是新的钥匙，而是一扇向海的小窗。她转身望向灯室东侧，那里果然有一扇小窗，窗台上伏着一面带木柄的圆镜。林遥用右手拿起圆镜，左手扶住铜底座，把镜片轻轻嵌入中央的凹槽。她没有从口袋里再取钥匙。
“把镜面对着海，不是对着我。”林遥说。
陈渡走到灯后，用双手转动沉重的支架。林遥扶着镜框，让镜面朝向东边的海。远处云层裂开一道窄缝，暮光落到圆镜上，再反射进灯罩。旧灯忽然亮了起来，金色的光扫过浅滩。两人都退后半步，才看清这束光已经越过堤岸，照到了车站后方的水面。
“原来她留下的是回家的路。”陈渡说。
林遥把信折好，放回蓝包，再把包口系紧，用左手重新提起。银钥匙仍在她红外套右侧口袋，陈渡身上没有钥匙。两人在灯室并肩朝海望去，林遥在右，陈渡在左。白塔红顶下的灯持续亮着，光里浮出一条正在归港的小船；他们没有离开灯塔，也没有登船。
"""),
]


def close(mcp):
    mcp.proc.stdin.close()
    mcp.proc.wait(timeout=10)
    mcp.log.close()


def snapshot(manifest):
    mcp = MCP(Path(manifest['binary']), Path(manifest['project']), Path(manifest['data_dir']))
    try:
        chapters = []
        for chapter in manifest['chapters']:
            rows, after = [], None
            while True:
                args = {'action': 'list', 'chapter_uuid': chapter['uuid'], 'limit': 100}
                if after:
                    args['after'] = after
                result = mcp.tool('pages', args)
                rows.extend(result['items'])
                pagination = result.get('cursor_pagination', {})
                if not pagination.get('has_more'):
                    break
                after = pagination['next_cursor']
            chapters.append({'chapter_uuid': chapter['uuid'], 'pages': rows})
        return {'project_uuid': manifest['project_uuid'], 'chapters': chapters}
    finally:
        close(mcp)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path)
    parser.add_argument('--baseline-skill', type=Path)
    parser.add_argument('--snapshot', type=Path)
    args = parser.parse_args()
    if args.snapshot:
        manifest_path = args.snapshot.resolve(strict=True)
        if not manifest_path.parent.name.startswith('lumi-storyboard-'):
            parser.error('snapshot requires this disposable fixture')
        value = snapshot(json.loads(manifest_path.read_text()))
        output = manifest_path.parent / 'snapshot.json'
        output.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps({'snapshot': str(output), 'page_counts': [len(c['pages']) for c in value['chapters']]}))
        return
    if not args.binary or not args.baseline_skill:
        parser.error('--binary and --baseline-skill are required')
    root = Path(tempfile.mkdtemp(prefix='lumi-storyboard-host-', dir='/private/tmp')).resolve()
    project, data = root / 'project', root / 'data'
    project.mkdir()
    binary = args.binary.resolve(strict=True)
    mcp = MCP(binary, project, data)
    try:
        init = mcp.tool('project_init', {'idea': '原创小说两章分镜隔离验收', 'name': '雨站与灯塔'})
        setup = mcp.tool('project_setup', {'action': 'update', 'expected_revision': init['setup']['revision'],
                         'project_name': '雨站与灯塔', 'overall_style': '清晰叙事的水彩绘本，中文文字逐字保留，角色和道具连续',
                         'picture_book': {'format': 'classic_picture_book', 'aspect_ratio': {'mode': 'square'}}})
        mcp.tool('project_setup', {'action': 'finalize', 'expected_revision': setup['revision']})
        chapters = []
        for code, title, content in NOVEL:
            revision = mcp.tool('project_overview', {})['project']['revision']
            chapter = mcp.tool('chapters', {'action': 'create', 'chapter_code': code, 'title': title,
                               'content': content, 'expected_revision': revision})
            chapters.append({'uuid': chapter['uuid'], 'chapter_code': code, 'title': title,
                             'source_sha256': hashlib.sha256(content.encode()).hexdigest()})
        subprocess.run([str(binary), 'init', '--data-dir', str(data), '--json'], cwd=project,
                       check=True, stdout=subprocess.DEVNULL)
    finally:
        close(mcp)
    # Snapshot the chosen skill, never alter the user's installed plugin/cache.
    skill = project / 'acceptance-skill'
    shutil.copytree(args.baseline_skill.resolve(strict=True), skill)
    (skill / 'references/cli.md').write_text(
        '# 隔离验收 CLI 入口\n\n只在本项目目录执行：\n\n'
        f'`"{binary}" <子命令> --data-dir "{data}" --json`\n\n'
        '该入口仅供本隔离项目，不使用正式数据目录。\n')
    manifest = {'root': str(root), 'binary': str(binary), 'project': str(project), 'data_dir': str(data),
                'project_uuid': init['project']['uuid'], 'chapters': chapters,
                'skill': str(skill / 'SKILL.md'), 'page_count_per_chapter': 3,
                'completion': '分镜逐页核对写回、划场与连续性交接保存；生图之前',
                'image_generation': 'not_run'}
    (root / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    (project / 'acceptance.json').write_text(json.dumps({k: manifest[k] for k in
        ('project_uuid', 'chapters', 'skill', 'page_count_per_chapter', 'completion')}, ensure_ascii=False, indent=2) + '\n')
    config = project / '.codex/config.toml'
    with config.open('a') as f:
        f.write('\n[mcp_servers.lumi_storyboard_acceptance]\ncommand = "/usr/bin/python3"\n'
                f'args = {json.dumps([str(REPO / "scripts/codex-acceptance/log_stdio.py"), str(root)])}\n'
                'startup_timeout_sec = 30\ntool_timeout_sec = 120\n')
    print(json.dumps({'manifest': str(root / 'manifest.json'), 'project': str(project)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
