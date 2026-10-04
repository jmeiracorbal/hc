package coco

import (
	"fmt"
	"sync"

	"github.com/jmeiracorbal/hybrid-coco/internal/parsers"
)

// builtin wraps a tree-sitter Parser as a Coco.
type builtin struct {
	id         string
	language   string
	extensions []string
	priority   int
	parser     parsers.Parser
}

func (b *builtin) ID() string               { return b.id }
func (b *builtin) Language() string         { return b.language }
func (b *builtin) Extensions() []string     { return append([]string(nil), b.extensions...) }
func (b *builtin) Priority() int            { return b.priority }
func (b *builtin) Parse(source []byte, path string) (parsers.ParseResult, error) {
	return b.parser.Parse(source, path), nil
}

var (
	builtinOnce sync.Once
	builtins    []Coco
	builtinErr  error
)

// Builtins returns the built-in language cocos (priority 0).
func Builtins() ([]Coco, error) {
	builtinOnce.Do(func() {
		type spec struct {
			id, lang string
			exts     []string
			newP     func() (parsers.Parser, error)
		}
		specs := []spec{
			{"hc/python", "python", []string{".py"}, func() (parsers.Parser, error) { return parsers.NewPythonParser() }},
			{"hc/javascript", "javascript", []string{".js", ".jsx"}, func() (parsers.Parser, error) { return parsers.NewJSParser("javascript") }},
			{"hc/typescript", "typescript", []string{".ts"}, func() (parsers.Parser, error) { return parsers.NewJSParser("typescript") }},
			{"hc/tsx", "tsx", []string{".tsx"}, func() (parsers.Parser, error) { return parsers.NewJSParser("tsx") }},
			{"hc/rust", "rust", []string{".rs"}, func() (parsers.Parser, error) { return parsers.NewRustParser() }},
			{"hc/go", "go", []string{".go"}, func() (parsers.Parser, error) { return parsers.NewGoParser() }},
		}
		out := make([]Coco, 0, len(specs))
		for _, s := range specs {
			p, err := s.newP()
			if err != nil {
				builtinErr = fmt.Errorf("builtin %s: %w", s.id, err)
				return
			}
			out = append(out, &builtin{
				id:         s.id,
				language:   s.lang,
				extensions: s.exts,
				priority:   BuiltinPriority,
				parser:     p,
			})
		}
		builtins = out
	})
	return builtins, builtinErr
}
