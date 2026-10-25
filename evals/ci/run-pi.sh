#!/usr/bin/env bash
# Runs only on the ephemeral Linux GitHub runner; no full host monorepo install.
set -euo pipefail
auth_dir=$(mktemp -d "$RUNNER_TEMP/pi-eval-auth.XXXXXX")
export PI_CODING_AGENT_DIR="$auth_dir"
trap 'rm -rf "$auth_dir"' EXIT
python3 - <<'PY'
import json, os
from pathlib import Path
p = Path(os.environ['PI_CODING_AGENT_DIR']) / 'auth.json'
p.write_text(json.dumps({'lumi-eval': {'type': 'api_key', 'key': os.environ['LUMI_EVAL_API_KEY']}}))
p.chmod(0o600)
PY
# Only the explicit runtime bind mount contains the key. Docker build cannot see it.
unset LUMI_EVAL_API_KEY
set +e
node --experimental-strip-types "$PI_DIR/packages/evals/src/cli.ts" \
  evals/models.docs.eval.ts evals/prompt-templates.docs.eval.ts \
  --provider lumi-eval --model "$EVAL_MODEL" --runs-per-variant "$EVAL_REPETITIONS" \
  2>&1 | tee "$OUT/pi-console.txt"
status=${PIPESTATUS[0]}
echo "$status" > "$OUT/pi-exit-code.txt"
exit "$status"
