#!/usr/bin/env python3
"""macOS acceptance: real installer + App app-server, no credentials/model calls.

Requires Python 3.9+. All state is isolated in the printed temporary root.
The JSON report records commands, output, version, RPC requests and results.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import select
import shutil
import struct
import subprocess
import tempfile
import time
import zlib

REPO = Path(__file__).resolve().parents[1]
APP_CODEX = Path('/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex')


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def png():
    def chunk(kind, value):
        return (struct.pack('!I', len(value)) + kind + value
                + struct.pack('!I', zlib.crc32(kind + value) & 0xffffffff))
    width, height = 96, 64
    data = b''.join(b'\0' + bytes((65, 135, 100)) * width for _ in range(height))
    return (b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('!2I5B', width, height, 8, 2, 0, 0, 0))
            + chunk(b'IDAT', zlib.compress(data)) + chunk(b'IEND', b''))


class Rpc:
    def __init__(self, env, root, report):
        self.report, self.serial, self.buffer = report, 0, b''
        self.stderr = (root / 'app-server.stderr').open('w')
        argv = [str(APP_CODEX), 'app-server']
        report['app_server_command'] = argv
        self.proc = subprocess.Popen(argv, cwd=root, env=env, stdin=subprocess.PIPE,
                                     stdout=subprocess.PIPE, stderr=self.stderr, bufsize=0)
        self.call('initialize', {'clientInfo': {'name': 'lumi-install-smoke', 'version': '1.0.0'},
                                 'capabilities': {'experimentalApi': True}})
        self.send({'method': 'initialized', 'params': {}})

    def send(self, body):
        self.proc.stdin.write((json.dumps({'jsonrpc': '2.0', **body}) + '\n').encode())

    def call(self, method, params):
        self.serial += 1
        request = {'id': self.serial, 'method': method, 'params': params}
        self.send(request)
        record = {'request': request}
        self.report.setdefault('rpc', []).append(record)
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline:
            while b'\n' not in self.buffer:
                ready, _, _ = select.select([self.proc.stdout], [], [], max(0, deadline - time.monotonic()))
                if not ready:
                    raise TimeoutError(method)
                data = os.read(self.proc.stdout.fileno(), 65536)
                if not data:
                    raise RuntimeError(f'app-server exited during {method}')
                self.buffer += data
            line, self.buffer = self.buffer.split(b'\n', 1)
            if not line:
                continue
            message = json.loads(line)
            if message.get('id') == request['id'] and 'method' not in message:
                record['response'] = message
                if 'error' in message:
                    raise RuntimeError(message['error'])
                return message['result']
            if 'id' in message and 'method' in message:
                self.send({'id': message['id'], 'error': {'code': -32601, 'message': 'Smoke harness does not approve requests'}})
        raise TimeoutError(method)

    def close(self):
        self.proc.stdin.close()
        try:
            self.proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.proc.terminate()
            self.proc.wait(timeout=5)
        self.stderr.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--assets', type=Path, required=True, help='Release artifact directory, including checksums')
    args = parser.parse_args()
    if platform.system() != 'Darwin' or not APP_CODEX.is_file():
        parser.error('This acceptance requires macOS and the actual App-bundled codex')
    root = Path(tempfile.mkdtemp(prefix='lumi-codex-acceptance-', dir='/tmp')).resolve()
    print(f'Acceptance root: {root}', flush=True)
    home, install, project = root / 'codex-home', root / 'install with spaces', root / 'empty-project'
    home.mkdir()
    project.mkdir()
    env = dict(os.environ, CODEX_HOME=str(home), LC_ALL='C')
    for key in ('OPENAI_API_KEY', 'CODEX_API_KEY', 'LUMI_DATA_DIR'):
        env.pop(key, None)
    # An unrelated plugin and marketplace exercise scoped config writes/removal.
    config = home / 'config.toml'
    config.write_text('# preserve this comment\nmodel = "test-unrelated-model"\n'
                      '[plugins."other@elsewhere"]\nenabled = false\n')
    report = {'root': str(root), 'codex': str(APP_CODEX), 'commands': [], 'checks': [],
              'limitations': ['No model inference, App GUI, native image generation or real-user approvals',
                              'Downloads supplied by local release fixtures, not a published GitHub release']}
    rpc = None
    binary = install / 'current/bin/lumi'

    def command(argv, expected=0, cwd=root):
        argv = list(map(str, argv))
        done = subprocess.run(argv, cwd=cwd, env=env, capture_output=True, text=True, timeout=120)
        report['commands'].append({'argv': argv, 'cwd': str(cwd), 'exit_code': done.returncode,
                                   'stdout': done.stdout, 'stderr': done.stderr})
        if (expected == 0 and done.returncode != 0) or (expected != 0 and done.returncode == 0):
            raise AssertionError(f'{argv}: {done.returncode}\n{done.stdout}\n{done.stderr}')
        return done.stdout

    def unchanged_other_config():
        actual = config.read_text()
        assert 'model = "test-unrelated-model"' in actual
        assert '[plugins."other@elsewhere"]\nenabled = false' in actual
        assert '# preserve this comment' in actual

    def snapshot():
        return {str(p.relative_to(install)): sha(p) for p in install.rglob('*') if p.is_file()}

    def check(label):
        report['checks'].append(label)
        print(f'PASS: {label}', flush=True)

    try:
        report['version'] = command([APP_CODEX, '--version']).strip()
        installer = ['bash', REPO / 'scripts/install-codex.sh', '--install-dir', install]
        assets = args.assets.resolve()
        command(installer + ['--from-dir', assets])
        first, first_config = snapshot(), config.read_bytes()
        command(installer + ['--from-dir', assets])
        assert snapshot() == first and config.read_bytes() == first_config
        unchanged_other_config()
        check('two installs produce identical payload and Codex config (path contains spaces)')
        mcp_path = install / 'current/marketplace/plugins/lumi/.mcp.json'
        mcp = json.loads(mcp_path.read_text())
        assert mcp['mcpServers']['lumi'] == {'command': str(binary), 'args': ['mcp']}
        check('generated stdio command is absolute and has no cwd')

        # A damaged download must fail before changing the installed state/config.
        corrupt = root / 'corrupt-assets'
        shutil.copytree(assets, corrupt)
        arch = 'arm64' if platform.machine() == 'arm64' else 'amd64'
        with (corrupt / f'lumi-darwin-{arch}').open('ab') as f:
            f.write(b'corrupt')
        command(installer + ['--from-dir', corrupt], expected=1)
        assert snapshot() == first and config.read_bytes() == first_config
        check('SHA256 failure preserves the existing installation and config')
        shutil.rmtree(corrupt)

        # Same marketplace name must not silently move an existing installation.
        command(['bash', REPO / 'scripts/install-codex.sh', '--install-dir', root / 'other-install',
                 '--from-dir', assets], expected=1)
        assert snapshot() == first and config.read_bytes() == first_config
        check('marketplace collision preserves the original installation')

        # A real Codex config-write failure after payload switch must roll back.
        # Only the disposable CODEX_HOME is made temporarily read-only.
        home.chmod(0o500)
        try:
            command(installer + ['--from-dir', assets], expected=1)
        finally:
            home.chmod(0o700)
        failed = report['commands'][-1]
        assert 'previous payload restored' in failed['stdout'] + failed['stderr']
        assert snapshot() == first and config.read_bytes() == first_config
        check('Codex config-write failure rolls back the switched payload')

        # Generate a distinct plugin version; the native binary stays the tested build.
        upgraded = root / 'upgrade-assets'
        upgraded.mkdir()
        for name in (f'lumi-darwin-{arch}', f'lumi-darwin-{arch}.sha256'):
            shutil.copy2(assets / name, upgraded / name)
        command(['node', REPO / 'scripts/package-codex.mjs', '999.0.1', upgraded])
        command(installer + ['--from-dir', upgraded])
        current_manifest = json.loads((install / 'current/marketplace/plugins/lumi/.codex-plugin/plugin.json').read_text())
        assert current_manifest['version'] == '999.0.1'
        cached = home / 'plugins/cache/lumi-local/lumi/999.0.1'
        assert json.loads((cached / '.codex-plugin/plugin.json').read_text())['version'] == '999.0.1'
        after_upgrade, config_after_upgrade = snapshot(), config.read_bytes()
        command(installer + ['--from-dir', upgraded])
        assert snapshot() == after_upgrade and config.read_bytes() == config_after_upgrade
        unchanged_other_config()
        check('version upgrade refreshes the plugin cache; repeating upgrade is idempotent')

        # Test-only instance isolation. Codex filters inherited MCP env; declare it
        # in the temporary generated marketplace and refresh the same-version cache.
        mcp['mcpServers']['lumi']['env'] = {'LUMI_DATA_DIR': str(root / 'lumi-data')}
        mcp_path.write_text(json.dumps(mcp))
        command([APP_CODEX, 'plugin', 'add', 'lumi@lumi-local', '--json'])
        assert json.loads((cached / '.mcp.json').read_text()) == mcp
        check('same-version add refreshes MCP config in cache')
        rpc = Rpc(env, root, report)
        skills = rpc.call('skills/list', {'cwds': [str(project)], 'forceReload': True})
        skill_entries = [entry for item in skills['data'] for entry in item.get('skills', [])]
        assert any(entry.get('name') == 'lumi:create-picture-book' and entry.get('enabled')
                   and entry.get('pluginId') == 'lumi@lumi-local' for entry in skill_entries), skills
        assert all(not item.get('errors') for item in skills['data']), skills
        thread = rpc.call('thread/start', {'cwd': str(project), 'approvalPolicy': 'never'})['thread']['id']
        status = rpc.call('mcpServerStatus/list', {'threadId': thread, 'limit': 100})
        server = next(item for item in status['data'] if 'project_overview' in str(item.get('tools')))
        check('App loader accepts manifest and skill, and discovers Lumi MCP tools')

        def tool(name, arguments=None, error=None):
            response = rpc.call('mcpServer/tool/call', {'threadId': thread, 'server': server['name'],
                                                       'tool': name, 'arguments': arguments or {}})
            # App server wraps the MCP CallToolResult as toolResult in this version.
            result = response.get('toolResult', response)
            envelope = result.get('structuredContent')
            if envelope is None:
                envelope = json.loads(next(c['text'] for c in result['content'] if c['type'] == 'text'))
            assert envelope['success'] == (error is None), envelope
            if error:
                assert envelope['error']['code'] == error, envelope
            return envelope.get('data')

        tool('project_overview', error='project_not_initialized')
        init = tool('project_init', {'idea': '小狐狸把雨夜迷路的星星送回天空', 'name': '安装验收绘本'})
        guidance = json.loads(command([binary, 'init', '--data-dir', root / 'lumi-data', '--json'], cwd=project))['data']
        assert guidance['uuid'] == init['project']['uuid'] and guidance['created'] is False
        assert guidance['write_protection']['configured'] is True
        draft = tool('project_setup', {'action': 'get'})
        assert draft['setup_status'] == 'draft' and draft['revision'] == init['setup']['revision']
        check('CLI init adds protection guidance to the MCP project without resetting its setup draft')
        setup = tool('project_setup', {'action': 'update', 'expected_revision': init['setup']['revision'],
                     'project_name': '安装验收绘本', 'generation_language': 'zh-Hans', 'overall_style': '温暖水彩',
                     'generation_brief': '一页完整故事，测试本地创作和导出。',
                     'picture_book': {'format': 'classic_picture_book', 'aspect_ratio': {'mode': 'landscape'},
                                      'large_image_minimal_text': False}})
        tool('project_setup', {'action': 'finalize', 'expected_revision': setup['revision']})
        overview = tool('project_overview')
        chapter = tool('chapters', {'action': 'create', 'chapter_code': 'vol01.ch01', 'title': '星星回家',
                       'expected_revision': overview['project']['revision'], 'content': '小狐狸举起叶子伞，陪星星走到山顶。'})
        pages = tool('pages', {'action': 'list', 'chapter_uuid': chapter['uuid']})
        page = tool('pages', {'action': 'create', 'chapter_uuid': chapter['uuid'], 'title': '我陪你回家',
                    'description_md': '小狐狸撑起伞：“我陪你回家。”', 'storyboard_md': '山顶雨夜，狐狸为星星撑伞，温暖水彩。',
                    'expected_revision': pages['state']['revision']})
        saved = tool('pages', {'action': 'get', 'chapter_uuid': chapter['uuid'], 'page_uuid': page['uuid']})
        assert saved['description_md'] == '小狐狸撑起伞：“我陪你回家。”'
        target = tool('image_target', {'target_uuid': page['uuid'], 'slot': 'page_image'})
        test_image = root / 'test-image.png'
        test_image.write_bytes(png())
        imported = tool('import_image', {'path': str(test_image), 'target_uuid': page['uuid'], 'slot': 'page_image',
                        'expected_revision': target['expected_revision'], 'fit_mode': 'keep',
                        'description': 'Synthetic acceptance fixture; not generated by Codex App'})
        assert imported['asset']['uuid']
        export = tool('export_book', {'format': 'pdf', 'allow_missing_images': False})
        pdf = Path(export['path'])
        assert pdf.is_relative_to(project) and export['missing_image_count'] == 0
        # Reading exactly the tool-returned deliverable is part of this test,
        # never open SQLite or enumerate protected project asset directories.
        assert pdf.read_bytes().startswith(b'%PDF-') and pdf.stat().st_size == export['byte_size']
        shutil.copy2(pdf, root / 'exported-book.pdf')
        report['export'] = {**export, 'sha256': sha(pdf)}
        check('empty-directory thread: initialize → setup → page text → import PNG → complete PDF')
        rpc.close()
        rpc = None
        command([binary, 'stop', '--data-dir', root / 'lumi-data', '--json'])
        # Use the saved installer to prove removal works without the checkout.
        command(['bash', install / 'current/install-codex.sh', '--install-dir', install, '--uninstall'])
        assert not (install / 'current').exists()
        assert not (home / 'plugins/cache/lumi-local/lumi').exists()
        unchanged_other_config()
        assert (project / 'project.sqlite').exists() and (root / 'exported-book.pdf').exists()
        command(installer + ['--uninstall'])
        check('offline uninstall and repeated uninstall preserve projects and unrelated Codex config')
        report['success'] = True
    except Exception as exc:
        report['success'] = False
        report['failure'] = str(exc)
        raise
    finally:
        if rpc:
            rpc.close()
        if binary.exists():
            command([binary, 'stop', '--data-dir', root / 'lumi-data', '--json'])
        (root / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        print(f'Report: {root / "report.json"}', flush=True)


if __name__ == '__main__':
    main()
