package parsers

import (
	"log"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

type GoParser struct {
	lang *sitter.Language
}

func NewGoParser() (*GoParser, error) {
	return &GoParser{
		lang: sitter.NewLanguage(tree_sitter_go.Language()),
	}, nil
}

func (p *GoParser) Parse(source []byte, filepath string) ParseResult {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(p.lang); err != nil {
		log.Printf("go_parser: set language for %s: %v", filepath, err)
		return ParseResult{}
	}

	tree := parser.Parse(source, nil)
	if tree == nil {
		log.Printf("go_parser: failed to parse %s", filepath)
		return ParseResult{}
	}
	defer tree.Close()

	var out ParseResult
	p.visit(tree.RootNode(), source, &out, "", "", "", "")
	return out
}

func (p *GoParser) visit(node *sitter.Node, source []byte, out *ParseResult, parentName, enclosing, recvVar, recvType string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "function_declaration":
		name := firstIdentifier(node, source)
		sig := goFuncSignature(node, source, false)
		if name != "" {
			out.Symbols = append(out.Symbols, Symbol{
				Name:      name,
				Kind:      "function",
				LineStart: int(node.StartPosition().Row) + 1,
				LineEnd:   int(node.EndPosition().Row) + 1,
				Signature: truncate(sig, 200),
			})
		}
		body := childByType(node, "block")
		if body != nil {
			for i := uint(0); i < body.ChildCount(); i++ {
				p.visit(body.Child(i), source, out, parentName, name, "", "")
			}
			return
		}

	case "method_declaration":
		name := goMethodName(node, source)
		rv, rt := goReceiverInfo(node, source)
		sig := goFuncSignature(node, source, true)
		if name != "" {
			out.Symbols = append(out.Symbols, Symbol{
				Name:       name,
				Kind:       "method",
				LineStart:  int(node.StartPosition().Row) + 1,
				LineEnd:    int(node.EndPosition().Row) + 1,
				Signature:  truncate(sig, 200),
				ParentName: rt,
			})
			appendContainsRef(&out.Refs, rt, name, int(node.StartPosition().Row)+1)
		}
		body := childByType(node, "block")
		if body != nil {
			for i := uint(0); i < body.ChildCount(); i++ {
				p.visit(body.Child(i), source, out, rt, name, rv, rt)
			}
			return
		}

	case "type_declaration":
		p.visitTypeDecl(node, source, out, parentName)

	case "import_declaration":
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
		qual := callQualifier(fn, source, parentName, recvVar, recvType)
		appendCallRef(&out.Refs, enclosing, to, qual, int(node.StartPosition().Row)+1)
	}

	if node.Kind() != "function_declaration" && node.Kind() != "method_declaration" {
		for i := uint(0); i < node.ChildCount(); i++ {
			p.visit(node.Child(i), source, out, parentName, enclosing, recvVar, recvType)
		}
	}
}

func (p *GoParser) visitTypeDecl(node *sitter.Node, source []byte, out *ParseResult, parentName string) {
	for i := uint(0); i < node.ChildCount(); i++ {
		spec := node.Child(i)
		if spec == nil || spec.Kind() != "type_spec" {
			continue
		}
		name := firstIdentifier(spec, source)
		if name == "" {
			continue
		}
		kindWord := "type"
		for j := uint(0); j < spec.ChildCount(); j++ {
			c := spec.Child(j)
			if c == nil {
				continue
			}
			switch c.Kind() {
			case "struct_type":
				kindWord = "struct"
			case "interface_type":
				kindWord = "interface"
			}
		}
		out.Symbols = append(out.Symbols, Symbol{
			Name:       name,
			Kind:       "class",
			LineStart:  int(spec.StartPosition().Row) + 1,
			LineEnd:    int(spec.EndPosition().Row) + 1,
			Signature:  kindWord + " " + name,
			ParentName: parentName,
		})
		appendContainsRef(&out.Refs, parentName, name, int(spec.StartPosition().Row)+1)
	}
}

func goMethodName(node *sitter.Node, source []byte) string {
	for i := uint(0); i < node.ChildCount(); i++ {
		c := node.Child(i)
		if c == nil {
			continue
		}
		if c.Kind() == "field_identifier" || c.Kind() == "identifier" {
			// skip receiver; method name is field_identifier after parameter_list
			if i > 0 {
				prev := node.Child(i - 1)
				if prev != nil && prev.Kind() == "parameter_list" {
					return nodeText(c, source)
				}
			}
		}
	}
	var last string
	for i := uint(0); i < node.ChildCount(); i++ {
		c := node.Child(i)
		if c != nil && c.Kind() == "field_identifier" {
			last = nodeText(c, source)
		}
	}
	return last
}

func goReceiverInfo(node *sitter.Node, source []byte) (recvVar, recvType string) {
	recv := childByType(node, "parameter_list")
	if recv == nil {
		return "", ""
	}
	param := childByType(recv, "parameter_declaration")
	if param == nil {
		return "", ""
	}
	for i := uint(0); i < param.ChildCount(); i++ {
		c := param.Child(i)
		if c == nil {
			continue
		}
		switch c.Kind() {
		case "identifier":
			if recvVar == "" {
				recvVar = nodeText(c, source)
			}
		case "type_identifier":
			recvType = nodeText(c, source)
		case "pointer_type", "qualified_type", "generic_type":
			if t := deepestIdentifier(c, source); t != "" {
				recvType = t
			}
		}
	}
	return recvVar, recvType
}

func goFuncSignature(node *sitter.Node, source []byte, method bool) string {
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
	sig := strings.TrimSpace(strings.Join(parts, " "))
	if method && !strings.HasPrefix(sig, "func") {
		return "func " + sig
	}
	return sig
}
