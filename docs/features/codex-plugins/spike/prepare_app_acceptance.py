#!/usr/bin/env python3
"""Prepare immutable GUI fixtures. Never install plugins or change ~/.codex."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import plistlib
import shlex
import shutil
import subprocess
import sys
import tempfile

from probe_server import png

HERE = Path(__file__).resolve().parent


def write_json(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=18765)
    parser.add_argument("--app-bundle", type=Path, default=Path("/Applications/ChatGPT.app"))
    args = parser.parse_args()
    if not 1024 <= args.port <= 65535:
        parser.error("port must be 1024..65535")
    root = Path(tempfile.mkdtemp(prefix="lumi-mcp-apps-", dir="/tmp")).resolve()
    stamp = datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S") + "." + root.name.rsplit("-", 1)[-1]
    info = plistlib.loads((args.app_bundle / "Contents/Info.plist").read_bytes())
    codex = args.app_bundle / "Contents/Resources/codex-cli/bin/codex"
    environment = {"observed_at_utc": datetime.now(timezone.utc).isoformat(), "app_bundle": str(args.app_bundle),
                   "app_version": info["CFBundleShortVersionString"], "app_build": info["CFBundleVersion"],
                   "bundle_id": info["CFBundleIdentifier"], "bundled_cli": str(codex),
                   "bundled_cli_version": subprocess.check_output([str(codex), "--version"], text=True).strip(),
                   "scope": "Installed bundle metadata; not a running-renderer binary fingerprint"}
    (root / "reference.png").write_bytes(png())
    variants = []
    for number, variant in enumerate(("allow", "no-resource"), 1):
        version = f"0.4.{number}+{stamp}"
        plugin_name = f"lumi-apps-{variant}-{root.name.rsplit('-', 1)[-1]}"
        market_name = plugin_name + "-local"
        market = root / variant / "marketplace"
        plugin = market / "plugins" / plugin_name
        plugin.mkdir(parents=True)
        for name in ("probe_server.py", "app.html"):
            shutil.copy2(HERE / name, plugin / name)
        uri = f"ui://lumi-spike/{stamp}/{variant}/app.html"
        write_json(plugin / ".codex-plugin/plugin.json", {
            "name": plugin_name, "version": version, "description": "Disposable MCP Apps acceptance probe"})
        write_json(plugin / ".mcp.json", {"mcpServers": {"probe": {
            "command": sys.executable, "args": [str(plugin / "probe_server.py")], "env": {
                "PYTHONDONTWRITEBYTECODE": "1", "SPIKE_LOG": str(root / variant / "mcp.jsonl"),
                "SPIKE_IMAGE_PORT": str(args.port), "SPIKE_BUILD": version, "SPIKE_UI_URI": uri,
                "SPIKE_CSP_VARIANT": variant, "SPIKE_ACCEPTANCE_ROOT": str(root)}}}})
        write_json(market / ".agents/plugins/marketplace.json", {
            "name": market_name, "interface": {"displayName": f"Lumi Apps probe · {variant}"}, "plugins": [{
                "name": plugin_name, "source": {"source": "local", "path": "./plugins/" + plugin_name},
                "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"}, "category": "Productivity"}]})
        variants.append({"variant": variant, "version": version, "plugin": plugin_name, "marketplace": market_name,
                         "marketplace_path": str(market), "plugin_path": str(plugin), "ui_uri": uri,
                         "manual_register_command": shlex.join([str(codex), "plugin", "marketplace", "add", str(market)]),
                         "manual_remove_marketplace_command": shlex.join([str(codex), "plugin", "marketplace", "remove", market_name])})
    manifest = {"root": str(root), "environment": environment, "variants": variants,
                "source_sha256": {name: hashlib.sha256((HERE / name).read_bytes()).hexdigest()
                                  for name in ("probe_server.py", "app.html")},
                "serve_command": shlex.join([sys.executable, str(HERE / "serve_acceptance_images.py"), str(root), "--port", str(args.port)])}
    write_json(root / "acceptance-manifest.json", manifest)
    print(json.dumps(manifest, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
