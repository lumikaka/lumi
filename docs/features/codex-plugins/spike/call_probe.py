#!/usr/bin/env python3
"""Call a test plugin through an isolated real Codex app-server, output MCP JSON.

Usage: python3 call_probe.py ROOT reference_image '{}'
ROOT is a temporary directory printed by run_probe.py, never ~/.codex.
"""
import json
import os
from pathlib import Path
import sys

from run_probe import Rpc

root = Path(sys.argv[1]).resolve()
if not root.name.startswith("lumi-codex-spike-") or root.parent != Path("/private/tmp"):
    # Linux /tmp resolves to /tmp; macOS resolves to /private/tmp.
    if not root.name.startswith("lumi-codex-spike-") or root.parent != Path("/tmp"):
        raise SystemExit("Expected a run_probe.py temporary fixture")
env = os.environ.copy()
env["CODEX_HOME"] = str(root / "home")
for name in ("OPENAI_API_KEY", "CODEX_API_KEY"):
    env.pop(name, None)
rpc = Rpc(env, root / "launcher", root / "call.stderr")
try:
    started = rpc.call("thread/start", {"cwd": str(root / "project-a"), "approvalPolicy": "never"})
    thread_id = started["result"]["thread"]["id"]
    rpc.call("mcpServerStatus/list", {"threadId": thread_id, "limit": 50})
    result = rpc.call("mcpServer/tool/call", {"threadId": thread_id, "server": "probe",
                      "tool": sys.argv[2], "arguments": json.loads(sys.argv[3])})
    print(json.dumps(result))
finally:
    rpc.close()
