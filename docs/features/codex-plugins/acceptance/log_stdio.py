#!/usr/bin/env python3
"""Transparent, redacted JSONL recorder for the disposable Step 5 host fixture."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import threading
from datetime import datetime, timezone

def main():
    root = Path(sys.argv[1])
    manifest = json.loads((root / 'manifest.json').read_text())
    process = subprocess.Popen([manifest['binary'], 'mcp', '--data-dir', manifest['data_dir']], cwd=manifest['project'], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=sys.stderr.buffer)
    lock = threading.Lock()
    def redact(value):
        if isinstance(value, dict):
            return {k: ({'omitted_chars': len(v), 'sha256': hashlib.sha256(v.encode()).hexdigest()} if isinstance(v, str) and (k in ('data_uri', 'image_data', 'websocket_url') or (k == 'data' and value.get('type') == 'image') or (k == 'text' and (v.lstrip().lower().startswith('<!doctype') or 'app_ticket=' in v))) else redact(v)) for k, v in value.items()}
        if isinstance(value, list): return [redact(v) for v in value]
        return value
    def record(direction, line):
        try: value = redact(json.loads(line))
        except Exception: value = {'unparsed_bytes': len(line)}
        with lock, (root / 'host-mcp.jsonl').open('a') as log:
            log.write(json.dumps({'at': datetime.now(timezone.utc).isoformat(), 'direction': direction, 'wire_bytes': len(line), 'message': value}, ensure_ascii=False) + '\n')
    def output():
        for line in process.stdout:
            record('result', line); sys.stdout.buffer.write(line); sys.stdout.buffer.flush()
    thread = threading.Thread(target=output); thread.start()
    try:
        for line in sys.stdin.buffer:
            record('request', line); process.stdin.write(line); process.stdin.flush()
    finally:
        process.stdin.close(); process.wait(); thread.join()
if __name__ == '__main__': main()
