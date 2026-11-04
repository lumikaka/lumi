#!/usr/bin/env python3
"""Verify the exact local Forge archive; never installs or publishes."""
import hashlib
import json
import shutil
import sys
from pathlib import Path
import zipfile

repo = Path(__file__).resolve().parents[2]
channel = sys.argv[2] if len(sys.argv) > 2 else 'production'
assert channel in ('production', 'development')
author = repo / ('build/cindy-dev/lumi-dev' if channel == 'development' else 'cindy-plugins/lumi')
manifest = json.loads((author / 'ghost.json').read_text())
package = author / f"{manifest['id']}-{manifest['version']}.cindy"
build = json.loads((author / 'build-info.json').read_text())
with zipfile.ZipFile(package) as archive:
    entries = archive.infolist()
    assert len(entries) <= 2048
    assert sum(e.file_size for e in entries) <= 256 * 1024 * 1024
    assert package.stat().st_size <= 128 * 1024 * 1024
    assert json.loads(archive.read('ghost.json')) == manifest
    assert json.loads(archive.read('build-info.json')) == build
    binary = 'bin/lumi_dev' if channel == 'development' else 'bin/lumi'
    config = json.loads(archive.read('node/runtime-config.json'))
    assert config['channel'] == config['environment'] == build['channel'] == channel
    assert manifest['id'] == manifest['command'] == ('lumi-dev' if channel == 'development' else 'lumi')
    cli = archive.getinfo(binary)
    assert ((cli.external_attr >> 16) & 0o777) == 0o755
    assert hashlib.sha256(archive.read(binary)).hexdigest() == build['cli_sha256']
    for name in ('main.js', 'node/worker.cjs', 'node/worker-transport.cjs', 'node/runtime.cjs', 'node/protocol.cjs', 'node/errors.cjs',
                 'node/request-lifetime.cjs', 'node/project.cjs', 'node/catalog.cjs', 'node/business-specs.json', 'node/export.cjs',
                 'node/config.cjs', 'node/runtime-config.json', 'node/media.cjs', 'node/reference.cjs',
                 'node/authoring.cjs', 'node/media-contract.cjs', 'node/workspace.cjs', 'workspace.html',
                 'manual/runtime/MANUAL.md', 'manual/project/MANUAL.md', 'manual/authoring/MANUAL.md',
                 'manual/authoring/references/settings.md', 'assets/icon.png'):
        assert archive.read(name) == (author / name).read_bytes(), name
    # Verify all shipped files, including new capabilities, against this exact author tree.
    for entry in entries:
        if not entry.is_dir():
            assert archive.read(entry.filename) == (author / entry.filename).read_bytes(), entry.filename
    assert not any('/tests/' in e.filename or '/node_modules/' in e.filename or e.filename.endswith('.sqlite') for e in entries)
    record = {'kind': 'local-forge-archive-not-installed', 'package': str(package.relative_to(repo)),
              'bytes': package.stat().st_size, 'sha256': hashlib.sha256(package.read_bytes()).hexdigest(),
              'cli_sha256': build['cli_sha256'], 'cli_mode': '755', 'channel': channel, 'build_id': build['build_id'],
              'entries': [{'name': e.filename, 'bytes': e.file_size} for e in entries]}
# Forge install repacks into the same filename. Freeze the reviewed bytes under
# their digest before a later install, so the original archive remains auditable.
review = repo / 'build/cindy-reviewed' / manifest['id'] / (record['sha256'] + '.cindy')
review.parent.mkdir(parents=True, exist_ok=True)
if not review.exists():
    with review.open('xb') as target, package.open('rb') as source:
        shutil.copyfileobj(source, target)
assert hashlib.sha256(review.read_bytes()).hexdigest() == record['sha256']
record['review_archive'] = str(review.relative_to(repo))
step = sys.argv[1] if len(sys.argv) > 1 else 'step2'
assert step == 'codex-alignment' or (step.startswith('step') and step[4:].isdigit())
target = repo / f'docs/features/cindy-plugins/acceptance/{step}-evidence/{"package-development" if channel == "development" else "package"}.json'
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
print(f"PASS: {len(entries)} archive entries, manifest, source bytes, CLI SHA256/mode, size limits; no installation")
print(f"package_sha256={record['sha256']}")
