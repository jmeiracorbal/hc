package parsers

import (
	"fmt"
	"strings"
)

// Allowed symbol kinds in the coco contract.
var AllowedSymbolKinds = map[string]struct{}{
	"function": {},
	"method":   {},
	"class":    {},
	"import":   {},
}

// Allowed ref kinds in the coco contract.
var AllowedRefKinds = map[string]struct{}{
	RefCalls:     {},
	RefContains:  {},
	RefImports:   {},
}

// ValidateParseResult fails fast on contract violations.
func ValidateParseResult(r ParseResult) error {
	for i, s := range r.Symbols {
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("symbol[%d]: name is required", i)
		}
		if _, ok := AllowedSymbolKinds[s.Kind]; !ok {
			return fmt.Errorf("symbol[%d] %q: unsupported kind %q", i, s.Name, s.Kind)
		}
		if s.LineStart < 1 {
			return fmt.Errorf("symbol[%d] %q: line_start must be >= 1", i, s.Name)
		}
		if s.LineEnd < s.LineStart {
			return fmt.Errorf("symbol[%d] %q: line_end must be >= line_start", i, s.Name)
		}
	}
	for i, ref := range r.Refs {
		if _, ok := AllowedRefKinds[ref.Kind]; !ok {
			return fmt.Errorf("ref[%d]: unsupported kind %q", i, ref.Kind)
		}
		if strings.TrimSpace(ref.ToName) == "" {
			return fmt.Errorf("ref[%d]: to_name is required", i)
		}
		if ref.Line < 1 {
			return fmt.Errorf("ref[%d]: line must be >= 1", i)
		}
	}
	return nil
}
