package parsers

// Symbol is a code symbol extracted by a language parser.
type Symbol struct {
	Name       string
	Kind       string // function | method | class | import
	LineStart  int
	LineEnd    int
	Signature  string
	Docstring  string
	ParentName string
}

const (
	RefCalls     = "calls"
	RefContains  = "contains"
	RefImports   = "imports"
)

// Ref is a structural edge extracted from source (call site, containment, import).
type Ref struct {
	Kind        string // calls | contains | imports
	FromName    string // enclosing symbol; empty for file-level imports
	ToName      string
	ToQualifier string // type/class tip for method-style calls; empty if unknown
	Line        int
}

// ParseResult is the full parse output for one file.
type ParseResult struct {
	Symbols []Symbol
	Refs    []Ref
}

// Parser extracts symbols and structural refs from source bytes.
type Parser interface {
	Parse(source []byte, filepath string) ParseResult
}
