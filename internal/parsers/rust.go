package parsers

import (
	"log"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

type RustParser struct {
	lang *sitter.Language
}

func NewRustParser() (*RustParser, error) {
	return &RustParser{
		lang: sitter.NewLanguage(tree_sitter_rust.Language()),
	}, nil
}

func (p *RustParser) Parse(source []byte, filepath string) ParseResult {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(p.lang); err != nil {
		log.Printf("rust_parser: set language for %s: %v", filepath, err)
		return ParseResult{}
	}

	tree := parser.Parse(source, nil)
	if tree == nil {
		log.Printf("rust_parser: failed to parse %s", filepath)
		return ParseResult{}
	}
	defer tree.Close()

	var out ParseResult
	p.visit(tree.RootNode(), source, &out, "", "")
	return out
}

func (p *RustParser) visit(node *sitter.Node, source []byte, out *ParseResult, parentName, enclosing string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "function_item":
		name := firstIdentifier(node, source)
		sig := rustFunctionSignature(node, source)
		doc := rustDocComments(node, source)
		kind := "function"
		if parentName != "" {
			kind = "method"
		}
		if name != "" {
			out.Symbols = append(out.Symbols, Symbol{
				Name:       name,
				Kind:       kind,
				LineStart:  int(node.StartPosition().Row) + 1,
				LineEnd:    int(node.EndPosition().Row) + 1,
				Signature:  truncate(sig, 200),
				Docstring:  doc,
				ParentName: parentName,
			})
			appendContainsRef(&out.Refs, parentName, name, int(node.StartPosition().Row)+1)
		}
		body := childByType(node, "block")
		if body != nil {
			for i := uint(0); i < body.ChildCount(); i++ {
				p.visit(body.Child(i), source, out, parentName, name)
			}
			return
		}

	case "struct_item", "enum_item":
		name := firstIdentifier(node, source)
		doc := rustDocComments(node, source)
		kindWord := "struct"
		if node.Kind() == "enum_item" {
			kindWord = "enum"
		}
		if name != "" {
			out.Symbols = append(out.Symbols, Symbol{
				Name:       name,
				Kind:       "class",
				LineStart:  int(node.StartPosition().Row) + 1,
				LineEnd:    int(node.EndPosition().Row) + 1,
				Signature:  kindWord + " " + name,
				Docstring:  doc,
				ParentName: parentName,
			})
			appendContainsRef(&out.Refs, parentName, name, int(node.StartPosition().Row)+1)
		}

	case "impl_item":
		implType := firstIdentifier(node, source)
		body := childByType(node, "declaration_list")
		if body != nil {
			for i := uint(0); i < body.ChildCount(); i++ {
				p.visit(body.Child(i), source, out, implType, enclosing)
			}
		}
		return

	case "use_declaration":
		text := truncate(strings.TrimSpace(nodeText(node, source)), 120)
		line := int(node.StartPosition().Row) + 1
		out.Symbols = append(out.Symbols, Symbol{
			Name:      text,
			Kind:      "import",
			LineStart: line,
			LineEnd:   int(node.EndPosition().Row) + 1,
			Signature: text,
		})
		appendImportRef(&out.Refs, text, line)

	case "call_expression":
		fn := node.Child(0)
		to := callTargetName(fn, source)
		qual := callQualifier(fn, source, parentName, "", "")
		appendCallRef(&out.Refs, enclosing, to, qual, int(node.StartPosition().Row)+1)
	}

	if node.Kind() != "impl_item" && node.Kind() != "function_item" {
		for i := uint(0); i < node.ChildCount(); i++ {
			p.visit(node.Child(i), source, out, parentName, enclosing)
		}
	}
}

func rustFunctionSignature(node *sitter.Node, source []byte) string {
	var parts []string
	for i := uint(0); i < node.ChildCount(); i++ {
		c := node.Child(i)
		if c == nil {
			continue
		}
		if c.Kind() == "block" {
			break
		}
		parts = append(parts, nodeText(c, source))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func rustDocComments(node *sitter.Node, source []byte) string {
	parent := node.Parent()
	if parent == nil {
		return ""
	}
	var siblings []*sitter.Node
	for i := uint(0); i < parent.ChildCount(); i++ {
		siblings = append(siblings, parent.Child(i))
	}
	idx := -1
	for i, s := range siblings {
		if s != nil && s.Id() == node.Id() {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ""
	}
	var docs []string
	for i := idx - 1; i >= 0; i-- {
		sib := siblings[i]
		if sib == nil || sib.Kind() != "line_comment" {
			break
		}
		text := nodeText(sib, source)
		if !strings.HasPrefix(text, "///") {
			break
		}
		docs = append([]string{strings.TrimSpace(text[3:])}, docs...)
	}
	return strings.Join(docs, " ")
}
