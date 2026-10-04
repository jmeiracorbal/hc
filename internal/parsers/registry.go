package parsers

import (
	"path/filepath"
	"strings"
	"sync"
)

// legacy language map kept for GetParser(language) callers that pass a language key.
var (
	parserMu sync.Mutex
	parsers  = map[string]Parser{}
	langExt  = map[string][]string{
		"python":     {".py"},
		"javascript": {".js", ".jsx"},
		"typescript": {".ts"},
		"tsx":        {".tsx"},
		"rust":       {".rs"},
		"go":         {".go"},
	}
)

// DetectLanguage returns the built-in language key for a file path, or empty if unsupported.
// Prefer coco.Resolver when installed cocos / pins are in play.
func DetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	for lang, exts := range langExt {
		for _, e := range exts {
			if e == ext {
				return lang
			}
		}
	}
	return ""
}

// GetParser returns a cached built-in parser for the language.
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

// ParseFile detects built-in language and extracts symbols + refs.
// Prefer coco.ParseAndValidate via coco.Resolver for modular cocos.
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
