#!/usr/bin/env python3
"""Exercise project-config/hook discovery and create a safe sandbox fixture."""
import argparse
import json
import os
from pathlib import Path
import shlex
import signal
import subprocess
import tempfile

from run_probe import Rpc, HERE

EVENTS = ("SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PermissionRequest",
          "PostToolUse", "PreCompact", "PostCompact", "SubagentStart", "SubagentStop", "Stop", "Interrupt")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-reviewed-hooks", action="store_true",
                        help="Run only the generated, reviewed test hooks for at most 5 seconds")
    args = parser.parse_args()
    root = Path(tempfile.mkdtemp(prefix="lumi-codex-config-", dir="/tmp")).resolve()
    home, project = root / "home", root / "project"
    home.mkdir()
    (project / ".codex").mkdir(parents=True)
    (project / ".lumi-codex-spike-fixture").write_text("lumi-codex-spike\n")
    for directory in (".lumi", "assets"):
        (project / directory).mkdir()
        (project / directory / "probe.txt").write_text("original\n")
    for name in ("project.sqlite", "project.sqlite-wal", "project.sqlite-shm"):
        (project / name).write_text("fake database fixture; not SQLite\n")
    subprocess.run(["git", "init", "-q", str(project)], check=True)
    (project / ".codex" / "config.toml").write_text('''default_permissions = "lumi-protect"

[permissions.lumi-protect]
extends = ":workspace"

[permissions.lumi-protect.filesystem.":workspace_roots"]
"project.sqlite" = "read"
"project.sqlite-wal" = "read"
"project.sqlite-shm" = "read"
"project.sqlite-journal" = "read"
".lumi" = "read"
"assets" = "read"
''')
    command = "python3 " + shlex.quote(str(HERE / "hook_probe.py")) + " " + shlex.quote(str(root / "hooks.jsonl"))
    (project / ".codex" / "hooks.json").write_text(json.dumps({"hooks": {
        event: [{"hooks": [{"type": "command", "command": command, "timeout": 3}]}] for event in EVENTS}}))
    env = os.environ.copy()
    env["CODEX_HOME"] = str(home)
    for name in ("OPENAI_API_KEY", "CODEX_API_KEY"):
        env.pop(name, None)
    report = {"root": str(root), "project": str(project), "home": str(home),
              "version": subprocess.check_output(["codex", "--version"], env=env, text=True).strip()}
    for trust in ("untrusted", "trusted"):
        (home / "config.toml").write_text('[projects.' + json.dumps(str(project)) + ']\ntrust_level = ' + json.dumps(trust) + '\n')
        rpc = Rpc(env, root, root / (trust + ".stderr"))
        try:
            report[trust] = {"config": rpc.call("config/read", {"cwd": str(project), "includeLayers": True}),
                             "hooks": rpc.call("hooks/list", {"cwds": [str(project)]})}
            started = rpc.call("thread/start", {"cwd": str(project), "approvalPolicy": "never"})
            report[trust]["thread"] = started
        finally:
            rpc.close()
    # Normal discovery never grants hook trust. The opt-in phase below exercises
    # only the known local fixture with an invocation-scoped trust bypass.
    if args.run_reviewed_hooks:
        cmd = ["codex", "--no-daemon", "exec", "--dangerously-bypass-hook-trust", "--json",
               "-s", "read-only", "-C", str(project),
               "-c", 'model_provider="spike"',
               "-c", 'model_providers.spike.name="Offline lifecycle test"',
               "-c", 'model_providers.spike.base_url="http://127.0.0.1:9/v1"',
               "-c", 'model_providers.spike.wire_api="responses"',
               "-c", 'model_providers.spike.requires_openai_auth=false',
               "-c", 'model_providers.spike.request_max_retries=0',
               "-c", 'model_providers.spike.stream_max_retries=0',
               "Reply exactly SPIKE_OK. Do not invoke tools."]
        proc = subprocess.Popen(cmd, env=env, stdin=subprocess.DEVNULL,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                text=True, start_new_session=True)
        timed_out = False
        try:
            stdout, stderr = proc.communicate(timeout=5)
        except subprocess.TimeoutExpired:
            timed_out = True
            os.killpg(proc.pid, signal.SIGTERM)
            stdout, stderr = proc.communicate(timeout=5)
        report["offline_hook_turn"] = {"argv": cmd, "exit_code": proc.returncode,
                                       "timed_out": timed_out, "stdout": stdout, "stderr": stderr}
        if (root / "hooks.jsonl").exists():
            report["hook_payloads"] = [json.loads(line) for line in (root / "hooks.jsonl").read_text().splitlines()]
    (root / "report.json").write_text(json.dumps(report, indent=2))
    command = ("CODEX_HOME=" + shlex.quote(str(home)) + " codex sandbox -C " + shlex.quote(str(project))
               + " -P lumi-protect -- python3 " + shlex.quote(str(HERE / "write_probe.py")))
    print(json.dumps({"root": str(root), "report": str(root / "report.json"), "sandbox_command": command}, indent=2))


if __name__ == "__main__":
    main()
