#!/usr/bin/env python3
"""Run real codex-cli probes in a fresh CODEX_HOME. Does not reuse credentials.

Usage: python3 docs/features/codex-plugins/spike/run_probe.py [--image ABS_PATH]
All mutable state and raw evidence remain in the printed temporary directory.
"""
import argparse
import json
import os
from pathlib import Path
import select
import shlex
import shutil
import subprocess
import tempfile
import time

from probe_server import png

HERE = Path(__file__).resolve().parent


class Rpc:
    def __init__(self, env, cwd, log, extra=()):
        self.events = []
        self.serial = 0
        self.err = open(log, "w")
        self.p = subprocess.Popen([env.get("SPIKE_CODEX_BIN", "codex"), "app-server", *extra], cwd=cwd, env=env,
                                  stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                  stderr=self.err, bufsize=0)
        self.buffer = b""
        self.call("initialize", {"clientInfo": {"name": "lumi-spike", "version": "0.1.0"},
                                 "capabilities": {"experimentalApi": True}})
        self.send({"method": "initialized", "params": {}})

    def send(self, body):
        self.p.stdin.write((json.dumps({"jsonrpc": "2.0", **body}) + "\n").encode())

    def call(self, method, params, timeout=40):
        self.serial += 1
        req_id = self.serial
        self.send({"id": req_id, "method": method, "params": params})
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            while b"\n" not in self.buffer:
                ready, _, _ = select.select([self.p.stdout], [], [], max(0, deadline-time.monotonic()))
                if not ready:
                    raise TimeoutError(method)
                chunk = os.read(self.p.stdout.fileno(), 65536)
                if not chunk:
                    raise RuntimeError("app-server exited during " + method)
                self.buffer += chunk
            line, self.buffer = self.buffer.split(b"\n", 1)
            if not line:
                continue
            msg = json.loads(line)
            self.events.append(msg)
            if msg.get("id") == req_id and "method" not in msg:
                return msg
            if "id" in msg and "method" in msg:
                # Never grant approval from a test harness.
                self.send({"id": msg["id"], "error": {"code": -32601, "message": "Probe does not approve requests"}})
        raise TimeoutError(method)

    def close(self):
        self.p.stdin.close()
        try:
            self.p.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.p.terminate()
            self.p.wait(timeout=5)
        self.err.close()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--image", type=Path)
    parser.add_argument("--codex", default="codex", help="CLI binary; may use the App-bundled CLI")
    args = parser.parse_args()
    root = Path(tempfile.mkdtemp(prefix="lumi-codex-spike-", dir="/tmp")).resolve()
    home, launcher, market = (root / n for n in ("home", "launcher", "marketplace"))
    for p in (home, launcher, market):
        p.mkdir()
    (root / "reference.png").write_bytes(png())
    env = os.environ.copy()
    env["CODEX_HOME"] = str(home)
    env["SPIKE_CODEX_BIN"] = args.codex
    # This probe does not need cloud auth and must not inherit an API key.
    for name in ("OPENAI_API_KEY", "CODEX_API_KEY"):
        env.pop(name, None)
    report = {"root": str(root), "binary": args.codex, "version": subprocess.check_output([args.codex, "--version"], env=env, text=True).strip(), "commands": [], "rpc": {}}
    plugin = market / "plugins" / "lumi-spike"
    (plugin / ".codex-plugin").mkdir(parents=True)
    for name in ("probe_server.py", "app.html"):
        shutil.copy2(HERE / name, plugin / name)
    (plugin / ".codex-plugin" / "plugin.json").write_text(json.dumps({"name": "lumi-spike", "version": "0.1.0", "description": "Disposable host-behavior probe"}))
    (plugin / ".mcp.json").write_text(json.dumps({"mcpServers": {"probe": {
        "command": shutil.which("python3"), "args": [str(plugin / "probe_server.py")],
        "env": {"SPIKE_LOG": str(root / "mcp.jsonl")}}}}))
    manifest_dir = market / ".agents" / "plugins"
    manifest_dir.mkdir(parents=True)
    (manifest_dir / "marketplace.json").write_text(json.dumps({"name": "lumi-spike-local", "plugins": [{
        "name": "lumi-spike", "source": {"source": "local", "path": "./plugins/lumi-spike"},
        "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"}, "category": "Productivity"}]}))
    for cmd in ([args.codex, "plugin", "marketplace", "add", str(market), "--json"],
                [args.codex, "plugin", "add", "lumi-spike@lumi-spike-local", "--json"],
                [args.codex, "plugin", "list", "--json"]):
        done = subprocess.run(cmd, cwd=launcher, env=env, capture_output=True, text=True, timeout=40)
        report["commands"].append({"argv": cmd, "exit_code": done.returncode, "stdout": done.stdout, "stderr": done.stderr})
    report["installed_config"] = (home / "config.toml").read_text() if (home / "config.toml").exists() else None
    projects = [root / n for n in ("project-a", "project-b")]
    for p in projects:
        p.mkdir()
        subprocess.run(["git", "init", "-q", str(p)], check=True)
    rpc = Rpc(env, launcher, root / "app-server.stderr")
    try:
        report["rpc"]["initial_config"] = rpc.call("config/read", {"cwd": str(projects[0]), "includeLayers": True})
        for p in projects:
            started = rpc.call("thread/start", {"cwd": str(p), "approvalPolicy": "never"})
            report["rpc"][p.name] = {"start": started}
            thread_id = started.get("result", {}).get("thread", {}).get("id")
            if not thread_id:
                continue
            status = rpc.call("mcpServerStatus/list", {"threadId": thread_id, "limit": 50})
            report["rpc"][p.name]["mcp_status"] = status
            servers = status.get("result", {}).get("data", [])
            for server in servers:
                if "probe" not in server.get("name", ""):
                    continue
                server_name = server["name"]
                report["rpc"][p.name]["inspect"] = rpc.call("mcpServer/tool/call", {
                    "threadId": thread_id, "server": server_name, "tool": "inspect", "arguments": {}})
                report["rpc"][p.name]["inspect_again"] = rpc.call("mcpServer/tool/call", {
                    "threadId": thread_id, "server": server_name, "tool": "inspect", "arguments": {}})
                report["rpc"][p.name]["show_app"] = rpc.call("mcpServer/tool/call", {
                    "threadId": thread_id, "server": server_name, "tool": "show_app", "arguments": {}})
                report["rpc"][p.name]["ui_resource"] = rpc.call("mcpServer/resource/read", {
                    "threadId": thread_id, "server": server_name, "uri": "ui://lumi-spike/app.html"})
                if args.image:
                    report["rpc"][p.name]["image_read"] = rpc.call("mcpServer/tool/call", {
                        "threadId": thread_id, "server": server_name, "tool": "read_image", "arguments": {"path": str(args.image.resolve())}})
        report["rpc"]["hooks"] = rpc.call("hooks/list", {"cwds": [str(p) for p in projects]})
    finally:
        rpc.close()
    report["events"] = rpc.events
    (root / "report.json").write_text(json.dumps(report, indent=2))
    print(json.dumps({"root": str(root), "report": str(root / "report.json"), "version": report["version"]}, indent=2))


if __name__ == "__main__":
    main()
