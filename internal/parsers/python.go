package parsers

import (
	"log"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

type PythonParser struct {
	lang *sitter.Language
}

func NewPythonParser() (*PythonParser, error) {
	return &PythonParser{
		lang: sitter.NewLanguage(tree_sitter_python.Language()),
	}, nil
}

func (p *PythonParser) Parse(source []byte, filepath string) ParseResult {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(p.lang); err != nil {
		log.Printf("python_parser: set language for %s: %v", filepath, err)
		return ParseResult{}
	}

	tree := parser.Parse(source, nil)
	if tree == nil {
		log.Printf("python_parser: failed to parse %s", filepath)
		return ParseResult{}
	}
	defer tree.Close()

	var out ParseResult
	p.visit(tree.RootNode(), source, &out, "", "")
	return out
}

func (p *PythonParser) visit(node *sitter.Node, source []byte, out *ParseResult, parentName, enclosing string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "function_definition":
		name := firstIdentifier(node, source)
		params := childByType(node, "parameters")
		body := childByType(node, "block")
		doc := ""
		if body != nil {
			doc = pythonDocstring(body, source)
		}
		kind := "function"
		if parentName != "" {
			kind = "method"
		}
		var sig string
		if name != "" && params != nil {
			sig = "def " + name + nodeText(params, source)
		}
		if name != "" {
			out.Symbols = append(out.Symbols, Symbol{
				Name:       name,
				Kind:       kind,
				LineStart:  int(node.StartPosition().Row) + 1,
				LineEnd:    int(node.EndPosition().Row) + 1,
				Signature:  sig,
				Docstring:  doc,
				ParentName: parentName,
			})
			appendContainsRef(&out.Refs, parentName, name, int(node.StartPosition().Row)+1)
		}
		if body != nil {
			for i := uint(0); i < body.ChildCount(); i++ {
				p.visit(body.Child(i), source, out, parentName, name)
			}
		}
		return

	case "class_definition":
		name := firstIdentifier(node, source)
		body := childByType(node, "block")
		doc := ""
		if body != nil {
			doc = pythonDocstring(body, source)
		}
		if name != "" {
			out.Symbols = append(out.Symbols, Symbol{
				Name:       name,
				Kind:       "class",
				LineStart:  int(node.StartPosition().Row) + 1,
				LineEnd:    int(node.EndPosition().Row) + 1,
				Signature:  "class " + name,
				Docstring:  doc,
				ParentName: parentName,
			})
			appendContainsRef(&out.Refs, parentName, name, int(node.StartPosition().Row)+1)
			if body != nil {
				for i := uint(0); i < body.ChildCount(); i++ {
					p.visit(body.Child(i), source, out, name, enclosing)
				}
			}
		}
		return

	case "import_statement", "import_from_statement":
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

	case "call":
		fn := node.Child(0)
		to := callTargetName(fn, source)
		qual := callQualifier(fn, source, parentName, "", "")
		appendCallRef(&out.Refs, enclosing, to, qual, int(node.StartPosition().Row)+1)
	}

	for i := uint(0); i < node.ChildCount(); i++ {
		p.visit(node.Child(i), source, out, parentName, enclosing)
	}
}

func pythonDocstring(body *sitter.Node, source []byte) string {
	for i := uint(0); i < body.ChildCount(); i++ {
		child := body.Child(i)
		if child == nil {
			continue
		}
		if child.Kind() == "expression_statement" {
			if child.ChildCount() > 0 {
				inner := child.Child(0)
				if inner != nil && inner.Kind() == "string" {
					return stripQuotes(nodeText(inner, source))
				}
			}
			break
		}
		if child.Kind() != "comment" && child.Kind() != "\n" {
			break
		}
	}
	return ""
}
