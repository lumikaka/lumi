#!/usr/bin/env python3
"""Serve only the synthetic reference PNG on loopback, with cache disabled."""
import argparse
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
from pathlib import Path
from urllib.parse import urlsplit


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path)
    parser.add_argument("--port", type=int, default=18765)
    args = parser.parse_args()
    root = args.root.resolve()
    if root.parent not in (Path("/tmp"), Path("/private/tmp")) or not root.name.startswith("lumi-mcp-apps-"):
        parser.error("Expected prepare_app_acceptance.py temporary fixture")
    body = (root / "reference.png").read_bytes()

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            status = 200 if urlsplit(self.path).path == "/reference.png" else 404
            event = {"at_utc": datetime.now(timezone.utc).isoformat(), "method": "GET", "path": self.path,
                     "status": status, "bytes": len(body) if status == 200 else 0}
            with (root / "http.jsonl").open("a") as f:
                f.write(json.dumps(event) + "\n")
            print(json.dumps(event), flush=True)
            self.send_response(status)
            self.send_header("Content-Type", "image/png" if status == 200 else "text/plain")
            self.send_header("Cache-Control", "no-store, max-age=0")
            self.send_header("Content-Length", str(len(body) if status == 200 else 0))
            self.end_headers()
            if status == 200:
                self.wfile.write(body)

        def log_message(self, *_):
            pass

    with HTTPServer(("127.0.0.1", args.port), Handler) as server:
        print(json.dumps({"listening": f"http://127.0.0.1:{args.port}", "log": str(root / "http.jsonl")}), flush=True)
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            pass


if __name__ == "__main__":
    main()
