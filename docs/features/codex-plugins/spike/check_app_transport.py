#!/usr/bin/env python3
"""Nonvisual preflight in isolated CODEX_HOME. Does not test an App iframe."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import struct
import subprocess
import zlib

from run_probe import Rpc

SIZES = (0, 65536, 262144, 1048576, 4194304, 8388608)


def check_png(body, padding):
    assert body[:8] == b"\x89PNG\r\n\x1a\n"
    offset, text, decoded = 8, None, None
    while offset < len(body):
        length = struct.unpack_from(">I", body, offset)[0]
        tag, payload = body[offset+4:offset+8], body[offset+8:offset+8+length]
        assert zlib.crc32(tag + payload) == struct.unpack_from(">I", body, offset+8+length)[0]
        if tag == b"tEXt":
            text = payload
        if tag == b"IDAT":
            decoded = zlib.decompress(payload)
        offset += length + 12
    assert offset == len(body) and text == b"spike\0" + b"x" * padding
    assert len(decoded) == 128 * (1 + 128 * 3)
    assert decoded[1:4] == bytes((255, 220, 0))  # top-left yellow
    assert decoded[1+64*3:1+65*3] == bytes((20, 90, 220))  # right blue


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path)
    args = parser.parse_args()
    root = args.root.resolve()
    if root.parent not in (Path("/tmp"), Path("/private/tmp")) or not root.name.startswith("lumi-mcp-apps-"):
        parser.error("Expected prepare_app_acceptance.py fixture")
    manifest = json.loads((root / "acceptance-manifest.json").read_text())
    codex = manifest["environment"]["bundled_cli"]
    report = {"scope": "Isolated app-server transport, PNG integrity, annotations and temporary marker; NO GUI or host approval evidence",
              "environment": manifest["environment"], "variants": []}
    for variant in manifest["variants"]:
        home = root / variant["variant"] / "transport-home"
        home.mkdir()
        env = os.environ.copy()
        env.update(CODEX_HOME=str(home), SPIKE_CODEX_BIN=codex, PYTHONDONTWRITEBYTECODE="1")
        for key in ("OPENAI_API_KEY", "CODEX_API_KEY"):
            env.pop(key, None)
        commands = ([codex, "plugin", "marketplace", "add", variant["marketplace_path"], "--json"],
                    [codex, "plugin", "add", variant["plugin"] + "@" + variant["marketplace"], "--json"])
        for command in commands:
            done = subprocess.run(command, cwd=root, env=env, capture_output=True, text=True, timeout=40)
            if done.returncode:
                raise RuntimeError(done.stderr + done.stdout)
        rpc = Rpc(env, root, root / variant["variant"] / "transport.stderr")
        row = {"variant": variant["variant"], "version": variant["version"], "payloads": []}
        try:
            start = rpc.call("thread/start", {"cwd": str(root), "approvalPolicy": "never"})
            thread = start["result"]["thread"]["id"]
            status = rpc.call("mcpServerStatus/list", {"threadId": thread, "limit": 50})
            server = next(s for s in status["result"]["data"] if "probe" in s["name"])
            tools = {t["name"]: t for t in server["tools"].values()} if isinstance(server["tools"], dict) else {t["name"]: t for t in server["tools"]}
            assert tools["inspect"]["annotations"]["readOnlyHint"] is True
            assert tools["write_marker"]["annotations"]["readOnlyHint"] is False
            assert tools["write_marker"]["_meta"]["ui"]["visibility"] == ["app"]
            row["annotations_checked"] = True
            def call(name, arguments):
                result = rpc.call("mcpServer/tool/call", {"threadId": thread, "server": server["name"],
                                  "tool": name, "arguments": arguments})
                rpc.events.clear()  # Never keep base64 in the compact evidence report.
                assert "error" not in result, str(result.get("error"))
                return result["result"]
            resource = rpc.call("mcpServer/resource/read", {"threadId": thread, "server": server["name"], "uri": variant["ui_uri"]})["result"]["contents"][0]
            assert "__BUILD__" not in resource["text"] and "__REFERENCE_B64__" not in resource["text"]
            csp = resource["_meta"]["ui"]["csp"]
            assert ("resourceDomains" in csp) == (variant["variant"] == "allow")
            assert csp["connectDomains"] == []
            row.update(ui_uri=resource["uri"], csp=csp, html_bytes=len(resource["text"].encode()))
            assert call("inspect", {})["structuredContent"]["build"] == variant["version"]
            assert call("show_app", {})["structuredContent"]["build"] == variant["version"]
            token = "transport-preflight-" + variant["variant"]
            assert call("write_marker", {"request_token": token})["structuredContent"]["request_token"] == token
            assert any(json.loads(line)["request_token"] == token for line in (root / "markers.jsonl").read_text().splitlines())
            assert call("fail_tool", {"request_token": token})["isError"] is True
            for padding in SIZES:
                for name, mode in (("get_image", "data_uri"), ("get_image", "image"), ("show_app", "data_uri")):
                    result = call(name, {"padding_bytes": padding, **({"mode": mode} if name == "get_image" else {})})
                    data = result["structuredContent"]
                    encoded = data["data_uri"].split(",", 1)[1] if mode == "data_uri" else next(c["data"] for c in result["content"] if c["type"] == "image")
                    body = base64.b64decode(encoded, validate=True)
                    check_png(body, padding)
                    assert hashlib.sha256(body).hexdigest() == data["sha256"]
                    row["payloads"].append({"tool": name, "mode": mode, "padding_bytes": padding,
                        "png_bytes": len(body), "base64_chars": len(encoded), "result_json_bytes": len(json.dumps(result).encode()),
                        "sha256": data["sha256"], "transport_and_integrity": "passed", "iframe": "not_tested"})
        finally:
            rpc.close()
        report["variants"].append(row)
    path = root / "transport-report.json"
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"report": str(path), "variants": len(report["variants"]),
                      "payload_checks": sum(len(v["payloads"]) for v in report["variants"]), "iframe": "not_tested"}))


if __name__ == "__main__":
    main()
