#!/usr/bin/env bash
# hybrid-coco PostToolUse hook — keeps index fresh after file writes (requires .hc)

command -v jq &>/dev/null || exit 0
command -v hc &>/dev/null || exit 0

INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null)

case "$TOOL_NAME" in
  Write|Edit) ;;
  *) exit 0 ;;
esac

for dir in "." ".." "../.." "../../.."; do
  if [ -f "$dir/.hc" ]; then
    PROJECT_ROOT=$(cd "$dir" && pwd -P)
    hc update "$PROJECT_ROOT" 2>/dev/null || true
    exit 0
  fi
done

exit 0
