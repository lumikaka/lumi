#!/usr/bin/env python3
"""Cross-platform packaged CLI smoke, no Codex login or model calls.

Exercises the actual binary's detached server, embedded web UI, stdio MCP,
project setup, synthetic image import and PDF/ZIP export in disposable state.
This is not Windows App/native-image-generation acceptance.
"""
import argparse
import json
import os
from pathlib import Path
import queue
import struct
import sys
import subprocess
import tempfile
import threading
import urllib.request
import zipfile
import zlib


def png():
    def chunk(kind, data):
        return struct.pack('!I', len(data)) + kind + data + struct.pack('!I', zlib.crc32(kind + data) & 0xffffffff)
    pixels = b''.join(b'\0' + bytes((65, 135, 100)) * 96 for _ in range(64))
    return (b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('!2I5B', 96, 64, 8, 2, 0, 0, 0))
            + chunk(b'IDAT', zlib.compress(pixels)) + chunk(b'IEND', b''))


class MCP:
    def __init__(self, binary, root, data, env):
        self.serial = 0
        self.messages = queue.Queue()
        self.log = (root.parent / 'mcp.stderr').open('w', encoding='utf-8')
        self.proc = subprocess.Popen([str(binary), 'mcp', '--data-dir', str(data)], cwd=root, env=env,
                                     stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=self.log,
                                     encoding='utf-8', bufsize=1)
        def reader():
            try:
                for line in self.proc.stdout:
                    self.messages.put(json.loads(line))
            except Exception as exc:
                self.messages.put(exc)
            finally:
                self.messages.put(None)
        threading.Thread(target=reader, daemon=True).start()
        self.call('initialize', {'protocolVersion': '2025-03-26', 'capabilities': {},
                                'clientInfo': {'name': 'lumi-packaged-smoke', 'version': '1.0'}})
        self.send({'method': 'notifications/initialized'})

    def send(self, message):
        self.proc.stdin.write(json.dumps({'jsonrpc': '2.0', **message}) + '\n')
        self.proc.stdin.flush()

    def call(self, method, params):
        self.serial += 1
        self.send({'id': self.serial, 'method': method, 'params': params})
        while True:
            message = self.messages.get(timeout=90)
            if message is None or isinstance(message, Exception):
                raise RuntimeError(f'MCP exited or returned invalid JSON: {message}')
            if message.get('id') == self.serial and 'method' not in message:
                assert 'error' not in message, message
                return message['result']
            if 'id' in message and 'method' in message:
                self.send({'id': message['id'], 'error': {'code': -32601, 'message': 'No host requests in smoke'}})

    def tool(self, name, args=None):
        result = self.call('tools/call', {'name': name, 'arguments': args or {}})
        envelope = result.get('_meta', {}).get('lumi/app_response') or result.get('structuredContent')
        if not envelope:
            envelope = json.loads(next(c['text'] for c in result['content'] if c['type'] == 'text'))
        assert envelope['success'], envelope
        return envelope['data']

    def close(self):
        self.proc.stdin.close()
        try:
            self.proc.wait(timeout=15)
        except subprocess.TimeoutExpired:
            self.proc.terminate()
            self.proc.wait(timeout=10)
        self.proc.stdout.close()
        self.log.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    env = dict(os.environ)
    for name in ('LUMI_DATA_DIR', 'LUMI_DESKTOP_ACCESS_TOKEN'):
        env.pop(name, None)
    with tempfile.TemporaryDirectory(prefix='lumi-packaged-cli-') as temporary:
        root = Path(temporary).resolve()
        project = root / 'book with spaces 绘本'
        project.mkdir()
        data = root / 'isolated data'
        client = None
        def command(name, expected=0):
            result = subprocess.run([str(binary), name, '--data-dir', str(data), '--json'], cwd=project,
                                    env=env, capture_output=True, encoding='utf-8', timeout=120)
            assert result.returncode == expected, result.stdout + result.stderr
            response = json.loads(result.stdout)
            assert response['success'], response
            return response['data']
        try:
            assert command('status', 3)['running'] is False
            first = command('serve')
            assert command('serve')['pid'] == first['pid'], 'serve did not reuse singleton'
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            with opener.open(first['endpoint'] + '/', timeout=10) as response:
                assert '<html' in response.read().decode().lower(), 'embedded Web UI missing'
            client = MCP(binary, project, data, env)
            tools = {t['name'] for t in client.call('tools/list', {})['tools']}
            assert {'project_init', 'project_setup', 'import_image', 'review_book', 'export_book'} <= tools
            init = client.tool('project_init', {'idea': 'A fox helps a star return home', 'name': 'CLI smoke'})
            setup = client.tool('project_setup', {'action': 'update', 'expected_revision': init['setup']['revision'],
                                'project_name': 'CLI smoke', 'generation_language': 'en', 'overall_style': 'Watercolor',
                                'picture_book': {'format': 'classic_picture_book', 'aspect_ratio': {'mode': 'landscape'}}})
            client.tool('project_setup', {'action': 'finalize', 'expected_revision': setup['revision']})
            overview = client.tool('project_overview')
            chapter = client.tool('chapters', {'action': 'create', 'chapter_code': 'vol01.ch01', 'title': 'Home',
                                  'expected_revision': overview['project']['revision'], 'content': 'Fox helps the star.'})
            pages = client.tool('pages', {'action': 'list', 'chapter_uuid': chapter['uuid']})
            page = client.tool('pages', {'action': 'create', 'chapter_uuid': chapter['uuid'], 'title': 'Star',
                               'description_md': 'Fox brings the star home.', 'storyboard_md': 'Fox and star on a hill.',
                               'expected_revision': pages['state']['revision']})
            target = client.tool('image_target', {'target_uuid': page['uuid'], 'slot': 'page_image'})
            prepared = client.tool('prepare_page_image', {'page_uuid': page['uuid'], 'setting_uuids': []})
            image = root / 'synthetic image.png'
            image.write_bytes(png())
            client.tool('import_image', {'path': str(image), 'target_uuid': page['uuid'], 'slot': 'page_image',
                        'expected_revision': target['expected_revision'], 'fit_mode': 'keep', 'description': 'Synthetic fixture',
                        'prompt': prepared['generation_prompt'], 'preparation': prepared['preparation']})
            for kind in ('pdf', 'zip'):
                exported = client.tool('export_book', {'format': kind, 'allow_missing_images': False})
                path = Path(exported['path'])
                assert path.is_relative_to(project) and exported['missing_image_count'] == 0
                assert path.stat().st_size == exported['byte_size']
                if kind == 'pdf':
                    assert path.read_bytes().startswith(b'%PDF-')
                else:
                    with zipfile.ZipFile(path) as archive:
                        assert archive.testzip() is None and archive.namelist()
            client.close()
            client = None
            assert command('stop')['running'] is False
            assert command('status', 3)['running'] is False
            assert command('serve')['running'] is True, 'restart after releasing locks failed'
            print('PASS: packaged CLI singleton, embedded Web, stdio MCP, Unicode project, image import, PDF/ZIP, stop/restart')
        except Exception:
            for log in (root / 'mcp.stderr', data / 'cli-production.log'):
                if log.exists():
                    print(log.read_text(encoding='utf-8', errors='replace')[-12000:], file=sys.stderr)
            raise
        finally:
            if client:
                client.close()
            command('stop')


if __name__ == '__main__':
    main()
