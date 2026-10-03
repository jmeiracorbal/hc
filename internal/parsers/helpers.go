package parsers

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func nodeText(node *sitter.Node, source []byte) string {
	return string(source[node.StartByte():node.EndByte()])
}

func childByType(node *sitter.Node, typ string) *sitter.Node {
	for i := uint(0); i < node.ChildCount(); i++ {
		c := node.Child(i)
		if c != nil && c.Kind() == typ {
			return c
		}
	}
	return nil
}

func firstIdentifier(node *sitter.Node, source []byte) string {
	for i := uint(0); i < node.ChildCount(); i++ {
		c := node.Child(i)
		if c == nil {
			continue
		}
		if c.Kind() == "identifier" || c.Kind() == "property_identifier" || c.Kind() == "type_identifier" || c.Kind() == "field_identifier" {
			return nodeText(c, source)
		}
	}
	return ""
}

func stripQuotes(raw string) string {
	raw = strings.TrimSpace(raw)
	for _, q := range []string{`"""`, `'''`, `"`, `'`} {
		if strings.HasPrefix(raw, q) && strings.HasSuffix(raw, q) && len(raw) >= 2*len(q) {
			return strings.TrimSpace(raw[len(q) : len(raw)-len(q)])
		}
	}
	return raw
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// callTargetName returns the simple callee name from a call/selector expression.
func callTargetName(fn *sitter.Node, source []byte) string {
	if fn == nil {
		return ""
	}
	switch fn.Kind() {
	case "identifier", "property_identifier", "field_identifier":
		return nodeText(fn, source)
	case "attribute":
		// python: obj.method — take the attribute identifier
		for i := fn.ChildCount(); i > 0; i-- {
			c := fn.Child(i - 1)
			if c != nil && (c.Kind() == "identifier" || c.Kind() == "attribute") {
				if c.Kind() == "identifier" {
					return nodeText(c, source)
				}
				return callTargetName(c, source)
			}
		}
	case "member_expression", "selector_expression":
		for i := fn.ChildCount(); i > 0; i-- {
			c := fn.Child(i - 1)
			if c == nil {
				continue
			}
			switch c.Kind() {
			case "property_identifier", "field_identifier", "identifier":
				return nodeText(c, source)
			}
		}
	case "scoped_identifier":
		// rust: foo::bar — take last identifier
		for i := fn.ChildCount(); i > 0; i-- {
			c := fn.Child(i - 1)
			if c != nil && c.Kind() == "identifier" {
				return nodeText(c, source)
			}
		}
	}
	// fallback: last identifier descendant
	return deepestIdentifier(fn, source)
}

func deepestIdentifier(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "identifier", "property_identifier", "field_identifier", "type_identifier":
		return nodeText(node, source)
	}
	var last string
	for i := uint(0); i < node.ChildCount(); i++ {
		if n := deepestIdentifier(node.Child(i), source); n != "" {
			last = n
		}
	}
	return last
}

// callQualifier returns type/class tip for method-style calls.
// parentName = enclosing class/type when known.
// recvVar/recvType used for Go same-receiver calls (may be empty).
func callQualifier(fn *sitter.Node, source []byte, parentName, recvVar, recvType string) string {
	if fn == nil {
		return ""
	}
	switch fn.Kind() {
	case "attribute":
		obj := fn.Child(0)
		if obj == nil || obj.Kind() != "identifier" {
			return ""
		}
		text := nodeText(obj, source)
		if text != "self" && text != "cls" {
			return ""
		}
		return parentName
	case "member_expression":
		obj := fn.Child(0)
		if obj == nil {
			return ""
		}
		if obj.Kind() == "this" || nodeText(obj, source) == "this" {
			return parentName
		}
		return ""
	case "selector_expression":
		if recvVar == "" || recvType == "" {
			return ""
		}
		obj := fn.Child(0)
		if obj == nil || obj.Kind() != "identifier" {
			return ""
		}
		if nodeText(obj, source) != recvVar {
			return ""
		}
		return recvType
	case "field_expression":
		if parentName == "" {
			return ""
		}
		obj := fn.Child(0)
		if obj == nil {
			return ""
		}
		if obj.Kind() == "self" || nodeText(obj, source) == "self" {
			return parentName
		}
		return ""
	}
	return ""
}

func appendCallRef(refs *[]Ref, enclosing, toName, toQualifier string, line int) {
	// from vacío = call a nivel de módulo/archivo; from_symbol_id queda NULL
	if toName == "" {
		return
	}
	*refs = append(*refs, Ref{
		Kind:        RefCalls,
		FromName:    enclosing,
		ToName:      toName,
		ToQualifier: toQualifier,
		Line:        line,
	})
}

func appendContainsRef(refs *[]Ref, parent, child string, line int) {
	if parent == "" || child == "" {
		return
	}
	*refs = append(*refs, Ref{
		Kind:     RefContains,
		FromName: parent,
		ToName:   child,
		Line:     line,
	})
}

func appendImportRef(refs *[]Ref, toName string, line int) {
	toName = strings.TrimSpace(toName)
	if toName == "" {
		return
	}
	*refs = append(*refs, Ref{
		Kind:   RefImports,
		ToName: truncate(toName, 120),
		Line:   line,
	})
}
