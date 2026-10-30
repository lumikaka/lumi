#!/usr/bin/env python3
"""Disposable, dependency-free stdio MCP probe. Never logs arbitrary env/secrets."""
import base64
import hashlib
import json
import os
from pathlib import Path
import struct
import sys
import time
import zlib

LOG = Path(os.environ.get("SPIKE_LOG", "/tmp/lumi-codex-spike-mcp.jsonl"))
ENV_KEYS = ("PWD", "CODEX_HOME", "CODEX_THREAD_ID", "CODEX_SESSION_ID",
            "CODEX_CWD", "CODEX_WORKSPACE_ROOT", "PLUGIN_ROOT", "PLUGIN_DATA",
            "CLAUDE_PLUGIN_ROOT", "CLAUDE_PLUGIN_DATA")


def record(kind, **fields):
    with LOG.open("a") as f:
        f.write(json.dumps({"time": time.time(), "pid": os.getpid(),
                            "kind": kind, **fields}) + "\n")


def send(message):
    print(json.dumps({"jsonrpc": "2.0", **message}), flush=True)


def png():
    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data))
    # Asymmetric reference: red left half, blue right half, yellow top-left square.
    rows = []
    for y in range(128):
        row = b"".join(bytes((255, 220, 0) if x < 24 and y < 24 else
                            (220, 30, 40) if x < 64 else (20, 90, 220)) for x in range(128))
        rows.append(b"\0" + row)
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 128, 128, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(b"".join(rows))) + chunk(b"IEND", b""))


TOOLS = [
    {"name": "inspect", "description": "Return this probe MCP process cwd, PID and safe environment.",
     "inputSchema": {"type": "object", "properties": {}},
     "_meta": {"ui": {"visibility": ["model", "app"]}}},
    {"name": "read_image", "description": "Read a local image path and return its SHA256 and byte count.",
     "inputSchema": {"type": "object", "properties": {"path": {"type": "string"}}, "required": ["path"]}},
    {"name": "reference_image", "description": "Return a synthetic PNG as MCP image content for a reference-image test.",
     "inputSchema": {"type": "object", "properties": {}}},
    {"name": "show_app", "description": "Show a disposable MCP Apps image/CSP/tool-callback diagnostic.",
     "inputSchema": {"type": "object", "properties": {}},
     "_meta": {"ui": {"resourceUri": "ui://lumi-spike/app.html", "visibility": ["model", "app"]}}},
]


def main():
    record("startup", cwd=os.getcwd(), ppid=os.getppid(),
           env={k: os.environ[k] for k in ENV_KEYS if k in os.environ})
    for line in sys.stdin:
        try:
            msg = json.loads(line)
            method = msg.get("method")
            if not method:
                record("client_reply", message=msg)
                continue
            # Tool inputs contain only test paths; no broad transcript/env capture.
            record("request", message=msg)
            req_id = msg.get("id")
            if method == "initialize":
                result = {"protocolVersion": msg["params"]["protocolVersion"],
                          "capabilities": {"tools": {}, "resources": {}},
                          "serverInfo": {"name": "lumi-spike", "version": "0.1.0"}}
            elif method == "notifications/initialized":
                send({"id": "probe-roots", "method": "roots/list", "params": {}})
                continue
            elif method == "tools/list":
                result = {"tools": TOOLS}
            elif method == "resources/list":
                result = {"resources": [{"uri": "ui://lumi-spike/app.html", "name": "Lumi spike",
                                         "mimeType": "text/html;profile=mcp-app"}]}
            elif method == "resources/templates/list":
                result = {"resourceTemplates": []}
            elif method == "resources/read":
                port = os.environ.get("SPIKE_IMAGE_PORT", "18765")
                html = (Path(__file__).with_name("app.html").read_text().replace("__PORT__", port)
                        .replace("__REFERENCE_B64__", base64.b64encode(png()).decode()))
                result = {"contents": [{"uri": "ui://lumi-spike/app.html",
                          "mimeType": "text/html;profile=mcp-app", "text": html,
                          "_meta": {"ui": {"csp": {
                              "resourceDomains": ["http://127.0.0.1:" + port],
                              "connectDomains": ["http://127.0.0.1:" + port]}}}}]}
            elif method == "tools/call":
                name = msg["params"]["name"]
                if name == "reference_image":
                    result = {"content": [{"type": "image", "mimeType": "image/png",
                                            "data": base64.b64encode(png()).decode()}]}
                else:
                    data = {"pid": os.getpid(), "cwd": os.getcwd(),
                            "env": {k: os.environ[k] for k in ENV_KEYS if k in os.environ}}
                    if name == "read_image":
                        path = Path(msg["params"]["arguments"]["path"])
                        body = path.read_bytes()
                        data = {"path": str(path), "bytes": len(body),
                                "sha256": hashlib.sha256(body).hexdigest(), "signature": body[:16].hex()}
                    record("tool_result", name=name, data=data)
                    result = {"content": [{"type": "text", "text": json.dumps(data)}], "structuredContent": data}
            elif method == "ping":
                result = {}
            else:
                if req_id is not None:
                    send({"id": req_id, "error": {"code": -32601, "message": "Unsupported probe method"}})
                continue
            if req_id is not None:
                send({"id": req_id, "result": result})
        except Exception as exc:
            record("error", error=str(exc))
            if "msg" in locals() and msg.get("id") is not None:
                send({"id": msg["id"], "error": {"code": -32603, "message": str(exc)}})


if __name__ == "__main__":
    main()
