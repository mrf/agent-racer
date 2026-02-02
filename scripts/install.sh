#!/bin/bash
set -e

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_ROOT="${XDG_CONFIG_HOME:-$HOME/.config}"
STATE_ROOT="${XDG_STATE_HOME:-$HOME/.local/state}"
CONFIG_DIR="${CONFIG_ROOT}/agent-racer"
STATE_DIR="${STATE_ROOT}/agent-racer"
BINARY_NAME="agent-racer"

echo "Building agent-racer..."

cd "$(dirname "$0")/.."
make build

echo "Installing to ${INSTALL_DIR}..."

if [ -w "$INSTALL_DIR" ]; then
  cp "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
else
  sudo cp "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

# Install default config
mkdir -p "$CONFIG_DIR"
if [ ! -f "${CONFIG_DIR}/config.yaml" ]; then
  cp config.yaml "${CONFIG_DIR}/config.yaml"
  echo "Installed default config to ${CONFIG_DIR}/config.yaml"
fi

# Install Claude SessionEnd hook
HOOK_DIR="${CONFIG_DIR}/hooks"
HOOK_SCRIPT="${HOOK_DIR}/session-end.sh"
SESSION_END_DIR="${STATE_DIR}/session-end"
mkdir -p "$HOOK_DIR" "$SESSION_END_DIR"
cp scripts/claude-session-end-hook.sh "$HOOK_SCRIPT"
chmod +x "$HOOK_SCRIPT"

SETTINGS_PATH="${HOME}/.claude/settings.json"
python3 - "$SETTINGS_PATH" "$HOOK_SCRIPT" <<'PY'
import json
import os
import sys

settings_path = sys.argv[1]
hook_script = sys.argv[2]

if os.path.exists(settings_path):
    with open(settings_path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
else:
    data = {}

hooks = data.setdefault("hooks", {})
session_end = hooks.setdefault("SessionEnd", [])

def has_hook():
    for entry in session_end:
        for hook in entry.get("hooks", []):
            if hook.get("type") == "command" and hook.get("command") == hook_script:
                return True
    return False

if not has_hook():
    session_end.append({"hooks": [{"type": "command", "command": hook_script}]})

os.makedirs(os.path.dirname(settings_path), exist_ok=True)
with open(settings_path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY

echo ""
echo "Installation complete!"
echo "  Binary: ${INSTALL_DIR}/${BINARY_NAME}"
echo "  Config: ${CONFIG_DIR}/config.yaml"
echo "  SessionEnd hook: ${HOOK_SCRIPT}"
echo ""
echo "Claude Code may require hook approval. Run /hooks in Claude Code to review."
echo ""
echo "Usage:"
echo "  agent-racer                    # Real mode - monitors actual Claude sessions"
echo "  agent-racer --mock             # Mock mode - demo with simulated sessions"
echo "  agent-racer --config path.yaml # Custom config"
echo "  agent-racer --port 9090        # Custom port"
echo ""
echo "Open http://localhost:8080 in your browser"
