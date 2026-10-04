package coco

import (
	"fmt"

	"github.com/jmeiracorbal/hybrid-coco/internal/parsers"
)

// ContractWASM is the only external coco transport in v1.
const ContractWASM = "wasm/v1"

// PlatformWASI is the release platform for wasm cocos.
const PlatformWASI = "wasm32/wasi"

// BuiltinPriority is the default priority for built-in language cocos.
const BuiltinPriority = 0

// Coco extracts symbols/refs for one or more file extensions.
type Coco interface {
	ID() string
	Language() string
	Extensions() []string
	Priority() int
	Parse(source []byte, path string) (parsers.ParseResult, error)
}

// Pins selects cocos for a project (from hc.toml).
type Pins struct {
	Cocos          []string          // preferred coco ids (order = preference)
	ExtensionPins  map[string]string // ".java" -> "acme/java-springboot"
}

// ParseAndValidate runs coco.Parse and enforces the contract.
func ParseAndValidate(c Coco, source []byte, path string) (parsers.ParseResult, error) {
	if c == nil {
		return parsers.ParseResult{}, fmt.Errorf("coco is required")
	}
	res, err := c.Parse(source, path)
	if err != nil {
		return parsers.ParseResult{}, err
	}
	if res.Error != "" {
		return parsers.ParseResult{}, fmt.Errorf("coco %s: %s", c.ID(), res.Error)
	}
	if err := parsers.ValidateParseResult(res); err != nil {
		return parsers.ParseResult{}, fmt.Errorf("coco %s: %w", c.ID(), err)
	}
	return res, nil
}
