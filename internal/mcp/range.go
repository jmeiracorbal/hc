package mcp

import (
	"fmt"
)

// ReadRangeHint builds the Claude Code Read offset/limit for a symbol span.
// lineStart and lineEnd are 1-based inclusive. Both must be valid — no defaults.
func ReadRangeHint(lineStart, lineEnd int) (string, error) {
	if lineStart < 1 {
		return "", fmt.Errorf("line_start must be >= 1, got %d", lineStart)
	}
	if lineEnd < lineStart {
		return "", fmt.Errorf("line_end %d < line_start %d", lineEnd, lineStart)
	}
	limit := lineEnd - lineStart + 1
	return fmt.Sprintf("read offset=%d limit=%d", lineStart, limit), nil
}
