#!/usr/bin/env python3
"""Record only this disposable session's hook payload to an explicit temp log."""
import json
import os
from pathlib import Path
import sys

payload = json.load(sys.stdin)
path = payload.get("transcript_path")
with Path(sys.argv[1]).open("a") as stream:
    stream.write(json.dumps({"process_cwd": os.getcwd(), "payload": payload,
                             "transcript_exists": bool(path and Path(path).is_file())}) + "\n")
