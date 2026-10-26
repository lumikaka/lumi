"""Build preparation only; runs on GitHub's temporary runner, never invokes an Agent."""
import base64
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import sys
import tarfile
from urllib.request import urlopen
from baseline import command, dump, sha

HERE = Path(__file__).resolve().parent
LOCK = json.loads((HERE / 'pi-runtime-lock.json').read_text())
CATALOG_LOCK = LOCK['catalog']

def seed_catalog(pi_dir, archive):
    integrity = 'sha512-' + base64.b64encode(hashlib.sha512(archive).digest()).decode()
    if len(archive) > 8_000_000 or integrity != CATALOG_LOCK['integrity']:
        raise ValueError('Published pi catalog package integrity mismatch.')
    providers = pi_dir / 'packages/ai/src/providers'
    expected = {p.name.removesuffix('.models.ts') + '.json' for p in providers.glob('*.models.ts')}
    expected.add('.manifest.json')
    data = {}
    prefix = 'package/dist/providers/data/'
    with tarfile.open(fileobj=io.BytesIO(archive), mode='r:gz') as package:
        for entry in package.getmembers():
            if entry.name.startswith(prefix) and entry.isfile():
                name = entry.name[len(prefix):]
                if name not in expected or name in data or entry.size > 4_000_000:
                    raise ValueError('Unexpected published model catalog file.')
                data[name] = package.extractfile(entry).read()
    if set(data) != expected:
        raise ValueError('Published model catalog does not match pinned source provider shards.')
    manifest = json.loads(data['.manifest.json'])
    if manifest['schemaVersion'] != 6 or manifest['generatedAt'] != CATALOG_LOCK['catalog_generated_at']:
        raise ValueError('Unexpected catalog schema or generation date.')
    if set(manifest['files']) != expected - {'.manifest.json'}:
        raise ValueError('Published model catalog manifest inventory mismatch.')
    for name, digest in manifest['files'].items():
        if sha(data[name]) != digest:
            raise ValueError('Published model catalog file digest mismatch.')
    directory = providers / 'data'
    directory.mkdir()
    for name, value in data.items():
        (directory / name).write_bytes(value)


def main():
    pi_dir, out = Path(os.environ['PI_DIR']), Path(os.environ['OUT'])
    if command('git', '-C', str(pi_dir), 'rev-parse', 'HEAD') != LOCK['revision']:
        raise ValueError('Unexpected pi source revision')
    if command('git', '-C', str(pi_dir), 'status', '--porcelain', '--untracked-files=no'):
        raise ValueError('Tracked pi source has local modifications')
    evidence = out / 'pi-runtime'
    if sys.argv[1] == 'prepare':
        with urlopen(CATALOG_LOCK['url'], timeout=60) as response:
            seed_catalog(pi_dir, response.read(8_000_001))
        evidence.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(HERE / 'pi-runtime-lock.json', evidence / 'lock.json')
        shutil.copyfile(pi_dir / 'package-lock.json', evidence / 'package-lock.json')
        shutil.copytree(pi_dir / 'packages/ai/src/providers/data', evidence / 'model-catalog')
    elif sys.argv[1] == 'seal-runtime':
        paths = [p for package in ('chord', 'telemetry', 'ai', 'agent')
                 for folder in ('src', 'dist') for p in (pi_dir / 'packages' / package / folder).rglob('*') if p.is_file()]
        if not (pi_dir / 'packages/agent/dist/index.js').is_file():
            raise ValueError('Compiled pi Agent is unavailable')
        dump(evidence / 'manifest.json', {'revision': LOCK['revision'],
            'node': command('node', '--version'), 'npm': command('npm', '--version'),
            'files': {str(p.relative_to(pi_dir)): sha(p.read_bytes()) for p in sorted(paths)}})
    else:
        raise ValueError('Unknown runtime operation')

if __name__ == '__main__':
    main()
