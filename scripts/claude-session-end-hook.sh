#!/bin/bash
set -euo pipefail

STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"
OUT_DIR="${AGENT_RACER_SESSION_END_DIR:-$STATE_HOME/agent-racer/session-end}"

mkdir -p "$OUT_DIR"

# Use python3 -c so that stdin remains available for the JSON payload
# from Claude Code. The previous heredoc approach (python3 - <<'PY')
# consumed stdin for the script itself, preventing json.load(sys.stdin)
# from ever reading the hook payload.
python3 -c '
import json
import os
import sys
import time

out_dir = sys.argv[1]

try:
    payload = json.load(sys.stdin)
except (json.JSONDecodeError, ValueError):
    sys.exit(0)

session_id = payload.get("session_id") or payload.get("sessionId")
if not session_id:
    sys.exit(0)

marker = {
    "session_id": session_id,
    "transcript_path": payload.get("transcript_path") or "",
    "cwd": payload.get("cwd"),
    "reason": payload.get("reason"),
    "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
}

marker_path = os.path.join(out_dir, f"{session_id}.json")
with open(marker_path, "w", encoding="utf-8") as handle:
    json.dump(marker, handle)
' "$OUT_DIR"
