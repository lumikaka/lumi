#!/usr/bin/env python3
"""Read-only comparison of explicit local installed instances and Forge archives."""
import hashlib
import json
from pathlib import Path
import sys
import zipfile

repo = Path(__file__).resolve().parents[2]
installed_root = Path(sys.argv[1]).resolve(strict=True)
assert installed_root.name == '_root'
assert installed_root.parent.name == '_ns' and installed_root.parent.parent.name == 'cindy-brain'
rows = []
for name, author in (
    ('lumi-dev', repo / 'build/cindy-dev/lumi-dev'),
    ('lumi-host-acceptance-20261101', repo / 'build/cindy-probe/lumi-host-acceptance-20261101'),
):
    installed = installed_root / name
    manifest = json.loads((author / 'ghost.json').read_text())
    package = author / f"{name}-{manifest['version']}.cindy"
    build = json.loads((installed / 'build-info.json').read_text())
    config = json.loads((installed / 'node/runtime-config.json').read_text())
    with zipfile.ZipFile(package) as archive:
        for entry in archive.infolist():
            if not entry.is_dir():
                assert (installed / entry.filename).read_bytes() == archive.read(entry.filename) == (author / entry.filename).read_bytes(), entry.filename
        count = len(archive.infolist())
    binary = installed / 'bin/lumi_dev'
    assert binary.stat().st_mode & 0o777 == 0o755
    assert hashlib.sha256(binary.read_bytes()).hexdigest() == build['cli_sha256']
    rows.append({'instance': name, 'namespace': None, 'installed_build': build, 'runtime_config': config,
                 'archive_sha256': hashlib.sha256(package.read_bytes()).hexdigest(), 'entries': count,
                 'all_installed_entry_bytes_equal_source_and_archive': True})

formal = json.loads((installed_root / 'lumi/build-info.json').read_text())
previous = json.loads((repo / 'docs/features/cindy-plugins/acceptance/codex-alignment-evidence/media-formal-baseline-20261104.json').read_text())['build']
assert formal['build_id'] == previous['build_id'] and formal['cli_sha256'] == previous['cli_sha256']
assert hashlib.sha256((installed_root / 'lumi/bin/lumi').read_bytes()).hexdigest() == previous['cli_sha256']
record = {'kind': 'read-only-installed-file-verification', 'date': '2026-11-04', 'instances': rows,
          'formal_build_observed': formal, 'formal_installed_this_turn': False, 'formal_build_and_cli_match_earlier_observation_this_turn': True}
target = repo / 'docs/features/cindy-plugins/acceptance/codex-alignment-evidence/media-installed-files-20261104.json'
target.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
print('PASS: installed dev/probe entries equal source and archive; CLI mode/hash verified; formal build/CLI match earlier observation this turn')
