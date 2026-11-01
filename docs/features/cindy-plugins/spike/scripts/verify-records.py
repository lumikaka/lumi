#!/usr/bin/env python3
"""Read-only checks of recorded evidence. Does not rerun or certify Host tests."""
import hashlib
import json
import pathlib
import re
import subprocess
import zipfile

BASE = pathlib.Path(__file__).resolve().parents[1]
def unpack(value):
    if isinstance(value, dict):
        return {k: unpack(v) for k, v in value.items()}
    if isinstance(value, list):
        return [unpack(v) for v in value]
    if isinstance(value, str) and value.startswith(('{', '[')):
        try:
            return unpack(json.loads(value))
        except (ValueError, TypeError):
            pass
    return value

files = list((BASE / 'evidence').glob('*.json'))
for p in files:
    unpack(json.loads(p.read_text()))
for p in (BASE / 'evidence').glob('*.jsonl'):
    for line in p.read_text().splitlines():
        json.loads(line)
print(f'PASS: {len(files)} evidence JSON files and all JSONL parse')

for name, expected in {
    'native-fox.png': '9bba276dc1d324b0e7c0d9362c90e1b5f40df0bbb35e33ec0298b02698a8b8f1',
    'core-fox.png': 'f09a1d3de26dbe010ed12cf9ff6594ea4b064f927c260ca41670e0b055b9652c',
}.items():
    assert hashlib.sha256((BASE / 'media' / name).read_bytes()).hexdigest() == expected
print('PASS: both original PNG SHA256 values match the handoff records')

snapshot = {r['version']: r for r in json.loads((BASE / 'evidence/package-snapshot.json').read_text())['packages']}
for p in (BASE / 'evidence').glob('pack-*.json'):
    record = json.loads(p.read_text())
    version = record.get('version', p.stem.removeprefix('pack-'))
    package = BASE / 'probe' / f'lumi-step0-probe-20261101-{version}.cindy'
    current = snapshot[version]
    assert package.stat().st_size == current['bytes']
    assert hashlib.sha256(package.read_bytes()).hexdigest() == current['sha256']
    if current['sha256'] != record['sha256']:
        print(f'NOTE: {version} current archive differs from historical pack receipt; exact earlier bytes not retained')
    with zipfile.ZipFile(package) as archive:
        assert (archive.getinfo('bin/lumi').external_attr >> 16) & 0o777 == 0o755
        assert json.loads(archive.read('ghost.json'))['version'] == version
        assert hashlib.sha256(archive.read('bin/lumi')).hexdigest() == '88a8f2ec9664d9ad3ed3a09dca3f49aacf1bcf4125c53b1d69c92a81837dd62d'
print('PASS: current package snapshots, manifest versions, CLI bytes and executable modes match')

for p in (BASE / 'probe').rglob('*'):
    if p.suffix in ('.js', '.cjs'):
        subprocess.run(['node', '--check', str(p)], check=True)
print('PASS: probe JavaScript syntax')

for doc in [BASE.parent / 'spike.md', BASE / 'README.md']:
    for target in re.findall(r'\]\(([^)]+)\)', doc.read_text()):
        if '://' in target or target.startswith('#'):
            continue
        path = re.sub(r':\d+$', '', target.split('#')[0])
        assert (doc.parent / path).exists(), (doc.name, target)
print('PASS: report and README local links resolve')

for p in (BASE / 'evidence').glob('task-*.json'):
    def check(value):
        if isinstance(value, dict):
            for k, v in value.items():
                if k in ('callId', 'invocationId'):
                    assert v == '<redacted-current-call-id>'
                check(v)
        elif isinstance(value, list):
            for v in value:
                check(v)
    check(unpack(json.loads(p.read_text())))
print('PASS: task evidence does not retain current callId capabilities')
cleanup_path = BASE / 'evidence/cleanup.json'
if cleanup_path.exists():
    cleanup = json.loads(cleanup_path.read_text())
    assert cleanup['confirmed_absent']
    for value in cleanup['removed']:
        assert not (pathlib.Path(value) if value.startswith('/') else BASE / value).exists()
    for name in ('uninstall-discovery.json', 'uninstall-call.json'):
        response = unpack(json.loads((BASE / 'evidence' / name).read_text()))
        assert response['content'][0]['text']['errorCode'] == 'GHOST_NOT_FOUND'
    assert json.loads((BASE / 'evidence/uninstall-final-processes.json').read_text())['matches'] == []
    print('PASS: uninstall discovery/call receipts, final process evidence and removed temporary paths')
repo = BASE.parents[3]
changes = subprocess.check_output(['git', 'status', '--short', '--untracked-files=all'], cwd=repo, text=True).splitlines()
assert all(line[3:].startswith(('docs/features/cindy-plugins/spike/', 'docs/features/cindy-plugins/spike.md')) for line in changes)
print(f'PASS: {len(changes)} changed/untracked paths stay within the authorized probe/report scope')
print('NOTE: artifact checks do not certify the untested subcases listed in spike.md')
