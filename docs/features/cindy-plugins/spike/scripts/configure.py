#!/usr/bin/env python3
"""Create this run's isolated fixture; no application/user configuration changes."""
import json
import pathlib
import secrets
import tempfile

base = pathlib.Path(__file__).resolve().parents[1]
config_path = base / 'probe/probe-config.json'
if config_path.exists():
    raise SystemExit('Existing run configuration: review/clean it before creating another run.')
root = pathlib.Path(tempfile.mkdtemp(prefix='lumi-cindy-step0-20261101-', dir='/private/tmp'))
owner = secrets.token_hex(16)
(root / '.probe-owner').write_text(owner)
for slot in ('a', 'b'):
    (root / slot / 'project').mkdir(parents=True)
    (root / slot / 'data').mkdir()
config = {'root': str(root), 'owner': owner, 'lumi_commit': '68235b5b4e9bc45d871895c8cb99ae4d6c0ba6e7'}
config_path.write_text(json.dumps(config, indent=2) + '\n')
print(json.dumps({'root': str(root), 'slots': ['a', 'b'], 'config': str(config_path.relative_to(base))}))
