package mcp_test

import (
	"strings"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/mcp"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
)

func TestReadRangeHint(t *testing.T) {
	h, err := mcp.ReadRangeHint(10, 14)
	if err != nil {
		t.Fatal(err)
	}
	if h != "read offset=10 limit=5" {
		t.Fatalf("got %q", h)
	}
}

func TestReadRangeHintRejectsInvalid(t *testing.T) {
	if _, err := mcp.ReadRangeHint(0, 5); err == nil {
		t.Fatal("expected error for line_start 0")
	}
	if _, err := mcp.ReadRangeHint(5, 4); err == nil {
		t.Fatal("expected error for line_end < line_start")
	}
}

func TestFormatFileContextIncludesRangeHints(t *testing.T) {
	text, err := mcp.FormatFileContext("src/a.py", &store.FileContext{
		Path:     "src/a.py",
		Language: "python",
		Symbols: []store.SymbolRow{
			{Name: "foo", Kind: "function", LineStart: 3, LineEnd: 7, Signature: "def foo()"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "read offset=3 limit=5") {
		t.Fatalf("missing range hint:\n%s", text)
	}
	if !strings.Contains(text, "offset+limit") {
		t.Fatalf("missing ranged-read instruction:\n%s", text)
	}
}

func TestFormatPackage(t *testing.T) {
	text, err := mcp.FormatPackage("src", &store.PackageOutline{
		Dir:       "src",
		FileCount: 1,
		Entries: []store.PackageEntry{
			{
				Symbol: store.SymbolRow{
					Name: "foo", Kind: "function", Path: "src/a.py",
					LineStart: 1, LineEnd: 2,
				},
				FanIn: 3,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "fan-in:3") {
		t.Fatalf("missing fan-in:\n%s", text)
	}
	if !strings.Contains(text, "read offset=1 limit=2") {
		t.Fatalf("missing hint:\n%s", text)
	}
}

func TestFormatStatusIncludesCalls(t *testing.T) {
	text := mcp.FormatStatus(store.Stats{
		Files:         2,
		Symbols:       5,
		Edges:         7,
		CallsTotal:    4,
		CallsResolved: 3,
		ByKind:        map[string]int{"function": 4, "class": 1},
		LastIndexed:   1,
	}, "/tmp/index.db")
	if !strings.Contains(text, "Calls:") {
		t.Fatalf("missing Calls line:\n%s", text)
	}
	if !strings.Contains(text, "3 resolved / 4 total") {
		t.Fatalf("missing resolved/total:\n%s", text)
	}
	if strings.Contains(text, "Calls by language:") {
		t.Fatalf("unexpected language breakdown with empty CallsByLanguage:\n%s", text)
	}
}

func TestFormatStatusCallsByLanguage(t *testing.T) {
	text := mcp.FormatStatus(store.Stats{
		Files:         2,
		Symbols:       5,
		Edges:         7,
		CallsTotal:    5,
		CallsResolved: 3,
		CallsByLanguage: []store.LangCallStats{
			{Language: "go", Total: 3, Resolved: 1},
			{Language: "python", Total: 2, Resolved: 2},
		},
		ByKind:      map[string]int{"function": 4},
		LastIndexed: 1,
	}, "/tmp/index.db")
	if !strings.Contains(text, "Calls by language:") {
		t.Fatalf("missing language breakdown:\n%s", text)
	}
	if !strings.Contains(text, "go: 1/3 resolved") {
		t.Fatalf("missing go line:\n%s", text)
	}
	if !strings.Contains(text, "python: 2/2 resolved") {
		t.Fatalf("missing python line:\n%s", text)
	}
}
