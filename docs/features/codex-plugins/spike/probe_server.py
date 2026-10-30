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
BUILD = os.environ.get("SPIKE_BUILD", "0.1.0")
UI_URI = os.environ.get("SPIKE_UI_URI", "ui://lumi-spike/app.html")
VARIANT = os.environ.get("SPIKE_CSP_VARIANT", "allow")
MAX_PADDING = 8 * 1024 * 1024
ENV_KEYS = ("PWD", "CODEX_HOME", "CODEX_THREAD_ID", "CODEX_SESSION_ID",
            "CODEX_CWD", "CODEX_WORKSPACE_ROOT", "PLUGIN_ROOT", "PLUGIN_DATA",
            "CLAUDE_PLUGIN_ROOT", "CLAUDE_PLUGIN_DATA")


def record(kind, **fields):
    with LOG.open("a") as f:
        f.write(json.dumps({"time": time.time(), "pid": os.getpid(),
                            "kind": kind, **fields}) + "\n")


def send(message):
    print(json.dumps({"jsonrpc": "2.0", **message}), flush=True)


def png(padding=None):
    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data))
    # Asymmetric reference: red left half, blue right half, yellow top-left square.
    rows = []
    for y in range(128):
        row = b"".join(bytes((255, 220, 0) if x < 24 and y < 24 else
                            (220, 30, 40) if x < 64 else (20, 90, 220)) for x in range(128))
        rows.append(b"\0" + row)
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 128, 128, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(b"".join(rows)))
            + (chunk(b"tEXt", b"spike\0" + b"x" * padding) if padding is not None else b"")
            + chunk(b"IEND", b""))


def read_annotations():
    return {"readOnlyHint": True, "destructiveHint": False, "openWorldHint": False}


PADDING_SCHEMA = {"type": "integer", "minimum": 0, "maximum": MAX_PADDING}


TOOLS = [
    {"name": "inspect", "description": "Return this probe MCP process cwd, PID and safe environment.",
     "inputSchema": {"type": "object", "properties": {}},
     "annotations": read_annotations(),
     "_meta": {"ui": {"visibility": ["model", "app"]}}},
    {"name": "read_image", "description": "Read a local image path and return its SHA256 and byte count.",
     "inputSchema": {"type": "object", "properties": {"path": {"type": "string"}}, "required": ["path"]}},
    {"name": "reference_image", "description": "Return a synthetic PNG as MCP image content for a reference-image test.",
     "inputSchema": {"type": "object", "properties": {}}},
    {"name": "show_app", "description": "Show a disposable MCP Apps image/CSP/tool-callback diagnostic.",
     "inputSchema": {"type": "object", "properties": {"padding_bytes": PADDING_SCHEMA}},
     "annotations": read_annotations(),
     "_meta": {"ui": {"resourceUri": UI_URI, "visibility": ["model", "app"]}}},
    {"name": "get_image", "description": "Return the synthetic PNG, optionally padded, as data URI or image content. No network or user files.",
     "inputSchema": {"type": "object", "properties": {
         "padding_bytes": PADDING_SCHEMA, "mode": {"type": "string", "enum": ["data_uri", "image"]},
         "request_token": {"type": "string", "maxLength": 160}}},
     "annotations": read_annotations(), "_meta": {"ui": {"visibility": ["model", "app"]}}},
    {"name": "write_marker", "description": "Append one disposable marker in the generated acceptance fixture. Never modifies Lumi files.",
     "inputSchema": {"type": "object", "properties": {
         "request_token": {"type": "string", "maxLength": 160}}},
     "annotations": {"readOnlyHint": False, "destructiveHint": False, "idempotentHint": False, "openWorldHint": False},
     "_meta": {"ui": {"visibility": ["app"]}}},
    {"name": "fail_tool", "description": "Return a deliberate isError tool result to test UI error feedback.",
     "inputSchema": {"type": "object", "properties": {
         "request_token": {"type": "string", "maxLength": 160}}},
     "annotations": read_annotations(), "_meta": {"ui": {"visibility": ["app"]}}},
]


def tool_result(name, args):
    if name not in {tool["name"] for tool in TOOLS}:
        raise ValueError("Unknown probe tool")
    data = {"pid": os.getpid(), "cwd": os.getcwd(), "build": BUILD, "variant": VARIANT,
            "env": {k: os.environ[k] for k in ENV_KEYS if k in os.environ}}
    if name == "read_image":
        path = Path(args["path"])
        body = path.read_bytes()
        data = {"path": str(path), "bytes": len(body), "sha256": hashlib.sha256(body).hexdigest(),
                "signature": body[:16].hex()}
    elif name == "write_marker":
        root = Path(os.environ.get("SPIKE_ACCEPTANCE_ROOT", "/nonexistent")).resolve()
        if root.parent not in (Path("/tmp"), Path("/private/tmp")) or not root.name.startswith("lumi-mcp-apps-"):
            raise ValueError("write_marker requires a generated temporary acceptance fixture")
        data = {"build": BUILD, "request_token": args.get("request_token"), "time": time.time()}
        # Only this fixed file can be written; caller cannot supply a path.
        with (root / "markers.jsonl").open("a") as f:
            f.write(json.dumps(data) + "\n")
    elif name == "fail_tool":
        data = {"code": "PROBE_EXPECTED_ERROR", "message": "Intentional acceptance error",
                "request_token": args.get("request_token")}
        record("tool_result", name=name, data=data, is_error=True)
        return {"isError": True, "content": [{"type": "text", "text": json.dumps(data)}], "structuredContent": data}
    elif name in ("reference_image", "get_image") or (name == "show_app" and "padding_bytes" in args):
        padding = args.get("padding_bytes", 0) if name != "reference_image" else None
        if padding is not None and (type(padding) is not int or not 0 <= padding <= MAX_PADDING):
            raise ValueError("padding_bytes must be an integer in 0..8388608")
        mode = args.get("mode", "data_uri") if name == "get_image" else "image" if name == "reference_image" else "data_uri"
        if mode not in ("data_uri", "image"):
            raise ValueError("Unknown image mode")
        body = png(padding)
        encoded = base64.b64encode(body).decode()
        data = {"build": BUILD, "padding_bytes": padding, "png_bytes": len(body),
                "base64_chars": len(encoded), "sha256": hashlib.sha256(body).hexdigest(),
                "mode": mode, "request_token": args.get("request_token")}
        record("tool_result", name=name, data=data)
        result = {"content": [{"type": "text", "text": json.dumps(data)}], "structuredContent": dict(data)}
        if mode == "image":
            result["content"].append({"type": "image", "mimeType": "image/png", "data": encoded})
        else:
            result["structuredContent"]["data_uri"] = "data:image/png;base64," + encoded
        return result
    record("tool_result", name=name, data=data)
    return {"content": [{"type": "text", "text": json.dumps(data)}], "structuredContent": data}


def ui_resource():
    port = os.environ.get("SPIKE_IMAGE_PORT", "18765")
    origin = "http://127.0.0.1:" + port
    html = (Path(__file__).with_name("app.html").read_text().replace("__PORT__", port)
            .replace("__REFERENCE_B64__", base64.b64encode(png()).decode())
            .replace("__BUILD__", BUILD).replace("__VARIANT__", VARIANT))
    # In the control, omit resourceDomains altogether; connectDomains stays identical.
    csp = {"connectDomains": []}
    if VARIANT == "allow":
        csp["resourceDomains"] = [origin]
    return {"contents": [{"uri": UI_URI, "mimeType": "text/html;profile=mcp-app", "text": html,
                          "_meta": {"ui": {"csp": csp}}}]}


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
                          "serverInfo": {"name": "lumi-spike", "version": BUILD}}
            elif method == "notifications/initialized":
                send({"id": "probe-roots", "method": "roots/list", "params": {}})
                continue
            elif method == "tools/list":
                result = {"tools": TOOLS}
            elif method == "resources/list":
                result = {"resources": [{"uri": UI_URI, "name": "Lumi spike",
                                         "mimeType": "text/html;profile=mcp-app"}]}
            elif method == "resources/templates/list":
                result = {"resourceTemplates": []}
            elif method == "resources/read":
                if msg["params"]["uri"] != UI_URI:
                    raise ValueError("Unknown UI resource")
                result = ui_resource()
            elif method == "tools/call":
                name = msg["params"]["name"]
                result = tool_result(name, msg["params"].get("arguments", {}))
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
