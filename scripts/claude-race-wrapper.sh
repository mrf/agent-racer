#!/bin/bash
# Optional wrapper for Claude Code that emits start/complete markers.
# Usage: claude-race-wrapper.sh [claude args...]
# This is not required for the dashboard to work, but provides
# explicit completion signaling as a supplement to process monitoring.

MARKER_DIR="${HOME}/.claude/race-markers"
mkdir -p "$MARKER_DIR"

SESSION_ID="$(date +%s)-$$"
MARKER_FILE="${MARKER_DIR}/${SESSION_ID}.marker"

# Write start marker
cat > "$MARKER_FILE" <<EOF
{
  "event": "start",
  "pid": $$,
  "cwd": "$(pwd)",
  "args": "$*",
  "timestamp": "$(date -Iseconds)"
}
EOF

# Run Claude Code
claude "$@"
EXIT_CODE=$?

# Write completion marker
cat > "$MARKER_FILE" <<EOF
{
  "event": "complete",
  "pid": $$,
  "cwd": "$(pwd)",
  "exit_code": $EXIT_CODE,
  "timestamp": "$(date -Iseconds)"
}
EOF

exit $EXIT_CODE
