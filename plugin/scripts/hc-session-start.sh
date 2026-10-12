#!/usr/bin/env bash
# hc SessionStart hook
# Ensures hc.md exists under CLAUDE_CONFIG_DIR (default ~/.claude).
# Runs once per session — no-op if already installed.

CLAUDE_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
AWARENESS="${CLAUDE_DIR}/hc.md"
CLAUDE_MD="${CLAUDE_DIR}/CLAUDE.md"
PLUGIN_AWARENESS="${CLAUDE_PLUGIN_ROOT}/awareness/hc.md"

# Write awareness file if missing
if [ ! -f "$AWARENESS" ]; then
  mkdir -p "$CLAUDE_DIR"
  cp "$PLUGIN_AWARENESS" "$AWARENESS"
fi

# Add @hc.md to CLAUDE.md if not already referenced
if ! grep -q "@hc.md" "$CLAUDE_MD" 2>/dev/null; then
  [ -f "$CLAUDE_MD" ] && echo "" >> "$CLAUDE_MD"
  echo "@hc.md" >> "$CLAUDE_MD"
fi

exit 0
