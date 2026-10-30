#!/usr/bin/env python3
"""Attempt harmless writes only in a disposable fixture directory supplied by cwd."""
import json
import os
from pathlib import Path

fixture = Path.cwd().resolve()
marker = fixture / ".lumi-codex-spike-fixture"
if (fixture.name != "project" or not fixture.parent.name.startswith("lumi-codex-config-")
        or not marker.is_file() or marker.read_text() != "lumi-codex-spike\n"):
    raise SystemExit("Refusing writes outside a config_hooks_probe.py disposable fixture")

results = []
for name in ("allowed.txt", "project.sqlite", "project.sqlite-wal", "project.sqlite-shm", "project.sqlite-journal",
             ".lumi/probe.txt", "assets/probe.txt", "assets/new.txt"):
    path = Path(name)
    try:
        path.write_text("spike write\n")
        results.append({"path": name, "write": "allowed"})
    except OSError as exc:
        results.append({"path": name, "write": "denied", "errno": exc.errno, "error": str(exc)})
for operation, target in (("replace", "project.sqlite"), ("unlink", "project.sqlite"), ("rename", "assets")):
    try:
        if operation == "replace":
            Path("replacement.tmp").write_text("replacement fixture\n")
            os.replace("replacement.tmp", target)
        elif operation == "unlink":
            Path(target).unlink()
        else:
            Path(target).rename("assets-moved")
        results.append({"path": target, "operation": operation, "result": "allowed"})
    except OSError as exc:
        results.append({"path": target, "operation": operation, "result": "denied", "errno": exc.errno})
print(json.dumps(results, indent=2))
