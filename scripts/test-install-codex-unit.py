#!/usr/bin/env python3
"""Fast macOS installer regressions: fixture Codex/curl, real Bash/plutil/tar.

No network, login, model calls, builds or changes to the user's Codex config.
The App integration remains in test-install-codex.py.
"""
import hashlib
import json
import os
from pathlib import Path
import platform
import shlex
import shutil
import subprocess
import tarfile
import tempfile
import unittest

REPO = Path(__file__).resolve().parents[1]
APP_ASSIGNMENT = "codex_bin='/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex'"

# Implements the observed Codex command contracts, including its own source
# collision guard. Tests assert filesystem/config outcomes, not shell text.
CODEX = r'''#!/usr/bin/env python3
import json, os, pathlib, shutil, sys
p=pathlib.Path(os.environ['LUMI_INSTALL_TEST_STATE'])
s=json.loads(p.read_text())
a=sys.argv[1:]
def save(): p.write_text(json.dumps(s))
def fail(message):
    print(message,file=sys.stderr)
    sys.exit(1)
def output(value): print(json.dumps(value))
with open(os.environ['LUMI_INSTALL_TEST_EVENTS'],'a') as f:
    f.write(json.dumps(['codex',a])+'\n')
if a==['--version']:
    print('codex-cli fixture')
elif '--help' in a:
    print('fixture help')
elif a[:2]==['plugin','list']:
    root=s.get('market_root')
    entries=[]
    if root:
        manifest=pathlib.Path(root)/'.agents/plugins/marketplace.json'
        if not manifest.is_file(): fail('lumi-local source is broken')
        catalog=json.loads(manifest.read_text())
        for entry in catalog['plugins']:
            plugin=pathlib.Path(root)/entry['source']['path']
            m=json.loads((plugin/'.codex-plugin/plugin.json').read_text())
            entries.append({'pluginId':m['name']+'@lumi-local','marketplaceName':'lumi-local',
                            'version':m['version'],'installed':bool(s.get('installed')),
                            'enabled':bool(s.get('installed')) and not s.get('disabled',False),
                            'marketplaceSource':{'sourceType':'local','source':root}})
    installed=[e for e in entries if e['installed']]
    available=[e for e in entries if not e['installed']] if '--available' in a else []
    output({'installed':installed,'available':available})
elif a[:3]==['plugin','marketplace','list']:
    if s.get('unrelated_broken'): fail('unrelated marketplace has no manifest')
    output({'marketplaces':[{'name':'lumi-local','root':s['market_root']}] if s.get('market_root') else []})
elif a[:3]==['plugin','marketplace','add']:
    root=str(pathlib.Path(a[3]).resolve())
    previous=s.get('market_root')
    if previous and previous!=root: fail('lumi-local is already added from a different source')
    s['market_root']=root
    save()
    output({'marketplaceName':'lumi-local','installedRoot':root,'alreadyAdded':bool(previous)})
elif a[:3]==['plugin','marketplace','remove']:
    s.pop('market_root',None)
    save()
    output({})
elif a[:2]==['plugin','add']:
    if s.pop('fail_add_once',False):
        save()
        fail('injected registration failure')
    source=pathlib.Path(s['market_root'])/'plugins/lumi'
    m=json.loads((source/'.codex-plugin/plugin.json').read_text())
    cache=pathlib.Path(os.environ['CODEX_HOME'])/'plugins/cache/lumi-local/lumi'/m['version']
    if cache.exists(): shutil.rmtree(cache)
    shutil.copytree(source,cache)
    if s.pop('bad_command_once',False):
        path=cache/'.mcp.json'
        content=json.loads(path.read_text())
        content['mcpServers']['lumi']['command']='/wrong/lumi'
        path.write_text(json.dumps(content))
    s['installed']=m['version']
    save()
    output({'pluginId':'lumi@lumi-local','version':m['version'],'installedPath':str(cache)})
elif a[:2]==['plugin','remove']:
    s.pop('installed',None)
    cache=pathlib.Path(os.environ['CODEX_HOME'])/'plugins/cache/lumi-local/lumi'
    if cache.exists(): shutil.rmtree(cache)
    save()
    output({})
else:
    fail('unexpected Codex command: '+repr(a))
'''

CURL = r'''#!/usr/bin/env python3
import json, os, pathlib, sys
args=sys.argv[1:]
url=next(a for a in args if a.startswith('https://'))
name=url.rsplit('/',1)[1]
with open(os.environ['LUMI_INSTALL_TEST_EVENTS'],'a') as f:
    f.write(json.dumps(['download',name])+'\n')
assets=pathlib.Path(os.environ['LUMI_INSTALL_TEST_ASSETS'])
if '/latest/' in url:
    source=assets/'latest'/name
else:
    version=url.split('/download/v',1)[1].split('/',1)[0]
    source=assets/version/name
if not source.is_file():
    print('fixture asset missing: '+str(source),file=sys.stderr)
    sys.exit(22)
pathlib.Path(args[args.index('-o')+1]).write_bytes(source.read_bytes())
'''


@unittest.skipUnless(platform.system() == 'Darwin', 'requires macOS plutil')
class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='lumi-installer-unit-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.home = self.root / 'codex-home'
        self.home.mkdir()
        self.install = self.root / 'install with spaces'
        self.cache = self.root / 'download cache'
        self.assets = self.root / 'assets'
        self.state_path = self.root / 'state.json'
        self.events_path = self.root / 'events.jsonl'
        self.state_path.write_text(json.dumps({'unrelated_config': {'model': 'keep', 'plugin_enabled': False}}))
        self.events_path.touch()
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        for name, content in [('codex', CODEX), ('curl', CURL)]:
            path = self.bin / name
            path.write_text(content)
            path.chmod(0o755)
        self.script = self.root / 'install-codex.sh'
        text = (REPO / 'scripts/install-codex.sh').read_text()
        assert text.count(APP_ASSIGNMENT) == 1
        self.script.write_text(text.replace(APP_ASSIGNMENT, 'codex_bin=' + shlex.quote(str(self.bin / 'codex'))))
        self.env = dict(os.environ, CODEX_HOME=str(self.home), LC_ALL='C',
                        PATH=str(self.bin) + os.pathsep + os.environ['PATH'],
                        LUMI_INSTALL_TEST_STATE=str(self.state_path),
                        LUMI_INSTALL_TEST_EVENTS=str(self.events_path),
                        LUMI_INSTALL_TEST_ASSETS=str(self.assets))
        self.version = '1.2.3'
        self.make_assets(self.version)

    def state(self, **updates):
        state = json.loads(self.state_path.read_text())
        if updates:
            state.update(updates)
            self.state_path.write_text(json.dumps(state))
        return state

    def make_assets(self, version):
        folder = self.assets / version
        folder.mkdir(parents=True)
        plugin = self.root / ('source-' + version) / 'lumi'
        (plugin / '.codex-plugin').mkdir(parents=True)
        (plugin / '.codex-plugin/plugin.json').write_text(json.dumps({'name': 'lumi', 'version': version}))
        (plugin / '.mcp.json').write_text(json.dumps({'mcpServers': {'lumi': {'command': '__LUMI_ABSOLUTE_BINARY__', 'args': ['mcp']}}}))
        (plugin / 'skills/create-picture-book').mkdir(parents=True)
        (plugin / 'skills/create-picture-book/SKILL.md').write_text('---\nname: create-picture-book\n---\nFixture skill.\n')
        with tarfile.open(folder / 'lumi-codex-plugin.tar.gz', 'w:gz') as archive:
            archive.add(plugin, arcname='lumi')
        arch = 'arm64' if platform.machine() == 'arm64' else 'amd64'
        (folder / ('lumi-darwin-' + arch)).write_text('#!/bin/bash\nexit 0\n')
        for name in ('lumi-darwin-' + arch, 'lumi-codex-plugin.tar.gz'):
            digest = hashlib.sha256((folder / name).read_bytes()).hexdigest()
            (folder / (name + '.sha256')).write_text(digest + '  ' + name + '\n')
        (folder / 'lumi-codex-version.txt').write_text(version + '\n')
        (self.assets / 'latest').mkdir(exist_ok=True)
        (self.assets / 'latest/lumi-codex-version.txt').write_text(version + '\n')
        return folder

    def run_installer(self, *extra, ok=True, install=None):
        result = subprocess.run(['/bin/bash', str(self.script), '--install-dir', str(install or self.install),
                                 '--cache-dir', str(self.cache), *map(str, extra)],
                                cwd=self.root, env=self.env, capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode == 0, ok, result.stdout + result.stderr)
        if ok and '--uninstall' not in extra:
            self.assertTrue(json.loads(result.stdout)['success'], result.stdout)
        self.assertEqual(self.state()['unrelated_config'], {'model': 'keep', 'plugin_enabled': False})
        return result

    def downloads(self, payload_only=False):
        names = [json.loads(line)[1] for line in self.events_path.read_text().splitlines()
                 if json.loads(line)[0] == 'download']
        if payload_only:
            names = [name for name in names if name.endswith('.tar.gz') or name.startswith('lumi-darwin-') and not name.endswith('.sha256')]
        return names

    def test_unrelated_broken_market_install_refresh_and_uninstall(self):
        self.state(unrelated_broken=True)
        result = self.run_installer()
        data = json.loads(result.stdout)['data']
        self.assertTrue(data['installed'] and data['enabled'])
        self.assertEqual(data['lumi_version'], self.version)
        self.assertEqual(data['cli_path'], str(self.install / 'current/bin/lumi'))
        again = self.run_installer()
        self.assertEqual(again.stderr.count('Cache hit (SHA256 verified)'), 2)
        self.assertEqual(len(self.downloads(payload_only=True)), 2)
        self.run_installer('--uninstall')
        self.run_installer('--uninstall')
        self.assertFalse((self.install / 'current').exists())
        self.assertTrue(self.state()['unrelated_broken'])

    def test_registration_failure_reuses_verified_downloads(self):
        self.state(fail_add_once=True)
        self.run_installer(ok=False)
        self.assertFalse((self.install / 'current').exists())
        self.assertFalse((self.install / '.lumi-codex-install').exists())
        self.assertFalse((self.install / '.install-lock').exists())
        self.assertEqual(len(self.downloads(payload_only=True)), 2)
        self.run_installer()
        self.assertEqual(len(self.downloads(payload_only=True)), 2)

    def test_broken_target_fails_before_download(self):
        self.state(market_root=str(self.root / 'missing-target'))
        result = self.run_installer(ok=False)
        self.assertIn('Cannot inspect lumi-local', result.stderr)
        self.assertEqual(self.downloads(), [])
        self.assertEqual(self.state()['market_root'], str(self.root / 'missing-target'))

    def test_existing_lock_fails_before_download(self):
        (self.install / '.install-lock').mkdir(parents=True)
        self.run_installer(ok=False)
        self.assertEqual(self.downloads(), [])
        self.assertTrue((self.install / '.install-lock').is_dir())

    def test_existing_market_collision_fails_before_download(self):
        self.run_installer()
        self.events_path.write_text('')
        before = self.state()
        self.run_installer(install=self.root / 'other-install', ok=False)
        self.assertEqual(self.downloads(), [])
        self.assertEqual(self.state(), before)
        self.assertTrue((self.install / 'current/bin/lumi').is_file())

    def test_empty_market_collision_never_overwritten(self):
        other = self.root / 'empty-market'
        (other / '.agents/plugins').mkdir(parents=True)
        (other / '.agents/plugins/marketplace.json').write_text(json.dumps({'name': 'lumi-local', 'plugins': []}))
        for broken in (False, True):
            with self.subTest(unrelated_broken=broken):
                self.state(market_root=str(other), unrelated_broken=broken)
                self.run_installer(ok=False)
                self.assertEqual(self.state()['market_root'], str(other))
                self.assertFalse((self.install / 'current').exists())
                self.assertFalse((self.install / '.lumi-codex-install').exists())
                if not broken:
                    self.assertEqual(self.downloads(), [])

    def test_corrupt_cache_is_rechecked_and_downloaded_again(self):
        self.run_installer()
        binaries = [p for p in self.cache.rglob('lumi-darwin-*') if p.is_file()]
        self.assertEqual(len(binaries), 1)
        binaries[0].write_text('corrupt')
        result = self.run_installer()
        self.assertIn('Cached SHA256 mismatch', result.stderr)
        self.assertEqual(len(self.downloads(payload_only=True)), 3)
        self.assertTrue(binaries[0].read_text().startswith('#!/bin/bash'))

    def test_from_dir_checks_supplied_assets_instead_of_network_cache(self):
        self.run_installer()
        before = self.state()
        corrupt = self.root / 'corrupt-assets'
        shutil.copytree(self.assets / self.version, corrupt)
        next(p for p in corrupt.glob('lumi-darwin-*') if not p.name.endswith('.sha256')).write_text('corrupt')
        self.events_path.write_text('')
        self.run_installer('--from-dir', corrupt, ok=False)
        self.assertEqual(self.downloads(), [])
        self.assertEqual(self.state(), before)
        self.assertEqual((self.install / 'current/version.txt').read_text().strip(), self.version)

    def test_post_install_cache_validation_failure_restores_previous_version(self):
        self.run_installer()
        self.make_assets('2.0.0')
        self.state(bad_command_once=True)
        result = self.run_installer(ok=False)
        self.assertIn('Cached MCP command does not point', result.stderr)
        self.assertIn('previous payload restored', result.stderr)
        self.assertEqual(self.state()['installed'], self.version)
        self.assertEqual((self.install / 'current/version.txt').read_text().strip(), self.version)
        self.assertFalse((self.install / '.previous').exists())
        self.assertFalse((self.install / '.install-lock').exists())
        self.run_installer()
        self.assertEqual(self.state()['installed'], '2.0.0')
        self.assertEqual(len(self.downloads(payload_only=True)), 4)

    def test_disabled_plugin_is_not_reported_as_success(self):
        self.state(disabled=True)
        result = self.run_installer(ok=False)
        self.assertIn('Plugin is not enabled', result.stderr)
        self.assertNotIn('"success": true', result.stdout)
        self.assertFalse((self.install / 'current').exists())


if __name__ == '__main__':
    unittest.main(verbosity=2)
