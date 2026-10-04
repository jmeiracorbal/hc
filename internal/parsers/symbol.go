package parsers

// Symbol is a code symbol extracted by a language parser.
type Symbol struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"` // function | method | class | import
	LineStart  int    `json:"line_start"`
	LineEnd    int    `json:"line_end"`
	Signature  string `json:"signature,omitempty"`
	Docstring  string `json:"docstring,omitempty"`
	ParentName string `json:"parent_name,omitempty"`
}

const (
	RefCalls    = "calls"
	RefContains = "contains"
	RefImports  = "imports"
)

// Ref is a structural edge extracted from source (call site, containment, import).
type Ref struct {
	Kind        string `json:"kind"` // calls | contains | imports
	FromName    string `json:"from_name,omitempty"`
	ToName      string `json:"to_name"`
	ToQualifier string `json:"to_qualifier,omitempty"`
	Line        int    `json:"line"`
}

// ParseResult is the full parse output for one file.
type ParseResult struct {
	Symbols []Symbol `json:"symbols"`
	Refs    []Ref    `json:"refs"`
	Error   string   `json:"error,omitempty"`
}

// Parser extracts symbols and structural refs from source bytes.
type Parser interface {
	Parse(source []byte, filepath string) ParseResult
}
