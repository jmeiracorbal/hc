#!/usr/bin/env bash
# hybrid-coco PreToolUse hook — blocking
# Full Read/Grep on indexed content → hc data (only if .hc marker present).
# Ranged Read (offset+limit both set) is allowed through.

command -v jq &>/dev/null || exit 0
command -v hc &>/dev/null || exit 0

INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null)

case "$TOOL_NAME" in
  Read|Grep) ;;
  *) exit 0 ;;
esac

PROJECT_ROOT=""
for dir in "." ".." "../.." "../../.."; do
  if [ -f "$dir/.hc" ]; then
    PROJECT_ROOT=$(cd "$dir" && pwd -P)
    break
  fi
done
[ -n "$PROJECT_ROOT" ] || exit 0

relpath_to_root() {
  local target="$1"
  local root="$2"
  case "$target" in
    /*) ;;
    *) target="$(pwd -P)/$target" ;;
  esac
  local abs
  abs=$(cd "$(dirname "$target")" 2>/dev/null && pwd -P)/$(basename "$target") || return 1
  local prefix="${root}/"
  case "$abs" in
    "$root") echo "." ;;
    "$prefix"*) echo "${abs#"$prefix"}" ;;
    *) return 1 ;;
  esac
}

if [ "$TOOL_NAME" = "Read" ]; then
  FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null)
  [ -n "$FILE_PATH" ] || exit 0

  OFFSET=$(echo "$INPUT" | jq -r 'if .tool_input.offset == null then empty else (.tool_input.offset | tostring) end' 2>/dev/null)
  LIMIT=$(echo "$INPUT" | jq -r 'if .tool_input.limit == null then empty else (.tool_input.limit | tostring) end' 2>/dev/null)
  if [ -n "$OFFSET" ] && [ -n "$LIMIT" ]; then
    exit 0
  fi

  REL_PATH=$(relpath_to_root "$FILE_PATH" "$PROJECT_ROOT") || exit 0

  HC_OUTPUT=$(cd "$PROJECT_ROOT" && hc file-context "$REL_PATH" 2>/dev/null) || exit 0
  [ -n "$HC_OUTPUT" ] || exit 0
  case "$HC_OUTPUT" in
    *"not found in index"*) exit 0 ;;
  esac

  REASON=$(printf '[hybrid-coco] Full Read blocked for %s. Symbols + range hints:\n\n%s\n\nTo read a body: Read with BOTH offset and limit from the hints (e.g. offset=10 limit=5). Or hc_explore / hc_package.' \
    "$REL_PATH" "$HC_OUTPUT" | jq -Rs .)
  printf '{"decision":"block","reason":%s}\n' "$REASON"
  exit 0
fi

if [ "$TOOL_NAME" = "Grep" ]; then
  PATTERN=$(echo "$INPUT" | jq -r '.tool_input.pattern // empty' 2>/dev/null)
  [ -n "$PATTERN" ] || exit 0

  if printf '%s' "$PATTERN" | grep -qE '[.^$*+?{}\[\]\\|()]'; then
    exit 0
  fi

  HC_OUTPUT=$(cd "$PROJECT_ROOT" && hc query "$PATTERN" 2>/dev/null) || exit 0
  [ -n "$HC_OUTPUT" ] || exit 0
  case "$HC_OUTPUT" in
    "No results.") exit 0 ;;
  esac

  REASON=$(printf '[hybrid-coco] Search results for "%s":\n\n%s\n\nUse hc_search for further queries.' \
    "$PATTERN" "$HC_OUTPUT" | jq -Rs .)
  printf '{"decision":"block","reason":%s}\n' "$REASON"
  exit 0
fi

exit 0
