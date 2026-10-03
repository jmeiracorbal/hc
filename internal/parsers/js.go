package parsers

import (
	"fmt"
	"log"
	"strings"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type JSParser struct {
	langName string
	lang     *sitter.Language
}

func NewJSParser(lang string) (*JSParser, error) {
	var ptr unsafe.Pointer
	switch lang {
	case "typescript":
		ptr = tree_sitter_typescript.LanguageTypescript()
	case "tsx":
		ptr = tree_sitter_typescript.LanguageTSX()
	case "javascript":
		ptr = tree_sitter_javascript.Language()
	default:
		return nil, fmt.Errorf("unknown js language: %s", lang)
	}
	return &JSParser{
		langName: lang,
		lang:     sitter.NewLanguage(ptr),
	}, nil
}

func (p *JSParser) Parse(source []byte, filepath string) ParseResult {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(p.lang); err != nil {
		log.Printf("js_parser: set language for %s: %v", filepath, err)
		return ParseResult{}
	}

	tree := parser.Parse(source, nil)
	if tree == nil {
		log.Printf("js_parser: failed to parse %s", filepath)
		return ParseResult{}
	}
	defer tree.Close()

	var out ParseResult
	p.visit(tree.RootNode(), source, &out, "", "")
	return out
}

func (p *JSParser) visit(node *sitter.Node, source []byte, out *ParseResult, parentName, enclosing string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "function_declaration":
		name := firstIdentifier(node, source)
		if name != "" {
			kind := "function"
			if parentName != "" {
				kind = "method"
			}
			out.Symbols = append(out.Symbols, Symbol{
				Name:       name,
				Kind:       kind,
				LineStart:  int(node.StartPosition().Row) + 1,
				LineEnd:    int(node.EndPosition().Row) + 1,
				Signature:  "function " + name + "()",
				ParentName: parentName,
			})
			appendContainsRef(&out.Refs, parentName, name, int(node.StartPosition().Row)+1)
		}
		body := childByType(node, "statement_block")
		if body != nil {
			for i := uint(0); i < body.ChildCount(); i++ {
				p.visit(body.Child(i), source, out, parentName, name)
			}
			return
		}

	case "class_declaration":
		name := firstIdentifier(node, source)
		if name != "" {
			out.Symbols = append(out.Symbols, Symbol{
				Name:       name,
				Kind:       "class",
				LineStart:  int(node.StartPosition().Row) + 1,
				LineEnd:    int(node.EndPosition().Row) + 1,
				Signature:  "class " + name,
				ParentName: parentName,
			})
			appendContainsRef(&out.Refs, parentName, name, int(node.StartPosition().Row)+1)
			body := childByType(node, "class_body")
			if body != nil {
				for i := uint(0); i < body.ChildCount(); i++ {
					p.visit(body.Child(i), source, out, name, enclosing)
				}
			}
			return
		}

	case "method_definition":
		name := firstIdentifier(node, source)
		if name != "" {
			out.Symbols = append(out.Symbols, Symbol{
				Name:       name,
				Kind:       "method",
				LineStart:  int(node.StartPosition().Row) + 1,
				LineEnd:    int(node.EndPosition().Row) + 1,
				Signature:  name + "()",
				ParentName: parentName,
			})
			appendContainsRef(&out.Refs, parentName, name, int(node.StartPosition().Row)+1)
		}
		body := childByType(node, "statement_block")
		if body != nil {
			for i := uint(0); i < body.ChildCount(); i++ {
				p.visit(body.Child(i), source, out, parentName, name)
			}
			return
		}

	case "import_statement":
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

	case "lexical_declaration":
		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child == nil || child.Kind() != "variable_declarator" {
				continue
			}
			varName := firstIdentifier(child, source)
			var val *sitter.Node
			for j := uint(0); j < child.ChildCount(); j++ {
				c := child.Child(j)
				if c != nil && (c.Kind() == "arrow_function" || c.Kind() == "function_expression") {
					val = c
					break
				}
			}
			if varName != "" && val != nil {
				kind := "function"
				if parentName != "" {
					kind = "method"
				}
				out.Symbols = append(out.Symbols, Symbol{
					Name:       varName,
					Kind:       kind,
					LineStart:  int(node.StartPosition().Row) + 1,
					LineEnd:    int(node.EndPosition().Row) + 1,
					Signature:  "const " + varName + " = () => ...",
					ParentName: parentName,
				})
				appendContainsRef(&out.Refs, parentName, varName, int(node.StartPosition().Row)+1)
				body := childByType(val, "statement_block")
				if body != nil {
					for j := uint(0); j < body.ChildCount(); j++ {
						p.visit(body.Child(j), source, out, parentName, varName)
					}
				} else {
					// expression body of arrow — still walk for calls
					for j := uint(0); j < val.ChildCount(); j++ {
						p.visit(val.Child(j), source, out, parentName, varName)
					}
				}
			}
		}
		return

	case "call_expression":
		fn := node.Child(0)
		to := callTargetName(fn, source)
		qual := callQualifier(fn, source, parentName, "", "")
		appendCallRef(&out.Refs, enclosing, to, qual, int(node.StartPosition().Row)+1)
	}

	if node.Kind() != "class_declaration" && node.Kind() != "function_declaration" && node.Kind() != "method_definition" {
		for i := uint(0); i < node.ChildCount(); i++ {
			p.visit(node.Child(i), source, out, parentName, enclosing)
		}
	}
}
