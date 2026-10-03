package parsers

import (
	"path/filepath"
	"strings"
	"sync"
)

var extMap = map[string]string{
	".py":  "python",
	".js":  "javascript",
	".jsx": "javascript",
	".ts":  "typescript",
	".tsx": "tsx",
	".rs":  "rust",
	".go":  "go",
}

var (
	parserMu sync.Mutex
	parsers  = map[string]Parser{}
)

// DetectLanguage returns the language key for a file path, or empty if unsupported.
func DetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	return extMap[ext]
}

// GetParser returns a cached parser for the language.
func GetParser(language string) (Parser, error) {
	parserMu.Lock()
	defer parserMu.Unlock()

	if p, ok := parsers[language]; ok {
		return p, nil
	}

	var p Parser
	var err error
	switch language {
	case "python":
		p, err = NewPythonParser()
	case "javascript":
		p, err = NewJSParser("javascript")
	case "typescript":
		p, err = NewJSParser("typescript")
	case "tsx":
		p, err = NewJSParser("tsx")
	case "rust":
		p, err = NewRustParser()
	case "go":
		p, err = NewGoParser()
	default:
		return nil, ErrUnsupportedLanguage
	}
	if err != nil {
		return nil, err
	}
	parsers[language] = p
	return p, nil
}

// ParseFile detects language and extracts symbols + refs.
func ParseFile(path string, source []byte) ParseResult {
	lang := DetectLanguage(path)
	if lang == "" {
		return ParseResult{}
	}
	p, err := GetParser(lang)
	if err != nil {
		return ParseResult{}
	}
	return p.Parse(source, path)
}
