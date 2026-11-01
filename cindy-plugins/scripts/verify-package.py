#!/usr/bin/env python3
"""Verify the exact local Forge archive; never installs or publishes."""
import hashlib
import json
import sys
from pathlib import Path
import zipfile

repo = Path(__file__).resolve().parents[2]
author = repo / 'cindy-plugins/lumi'
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
    cli = archive.getinfo('bin/lumi')
    assert ((cli.external_attr >> 16) & 0o777) == 0o755
    assert hashlib.sha256(archive.read('bin/lumi')).hexdigest() == build['cli_sha256']
    for name in ('main.js', 'node/worker.cjs', 'node/worker-transport.cjs', 'node/runtime.cjs', 'node/protocol.cjs', 'node/errors.cjs',
                 'node/request-lifetime.cjs', 'node/project.cjs', 'node/catalog.cjs', 'node/business-specs.json', 'node/export.cjs',
                 'manual/runtime/MANUAL.md', 'manual/project/MANUAL.md', 'manual/authoring/MANUAL.md', 'assets/icon.png'):
        assert archive.read(name) == (author / name).read_bytes(), name
    assert not any('/tests/' in e.filename or '/node_modules/' in e.filename or e.filename.endswith('.sqlite') for e in entries)
    record = {'kind': 'local-forge-archive-not-installed', 'package': str(package.relative_to(repo)),
              'bytes': package.stat().st_size, 'sha256': hashlib.sha256(package.read_bytes()).hexdigest(),
              'cli_sha256': build['cli_sha256'], 'cli_mode': '755',
              'entries': [{'name': e.filename, 'bytes': e.file_size} for e in entries]}
step = sys.argv[1] if len(sys.argv) > 1 else 'step2'
assert step.startswith('step') and step[4:].isdigit()
target = repo / f'docs/features/cindy-plugins/acceptance/{step}-evidence/package.json'
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
print(f"PASS: {len(entries)} archive entries, manifest, source bytes, CLI SHA256/mode, size limits; no installation")
print(f"package_sha256={record['sha256']}")
