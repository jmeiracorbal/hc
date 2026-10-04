package parsers

import "testing"

func TestValidateParseResultOK(t *testing.T) {
	err := ValidateParseResult(ParseResult{
		Symbols: []Symbol{{Name: "Foo", Kind: "class", LineStart: 1, LineEnd: 2}},
		Refs:    []Ref{{Kind: RefCalls, FromName: "Bar", ToName: "Foo", Line: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateParseResultBadKind(t *testing.T) {
	err := ValidateParseResult(ParseResult{
		Symbols: []Symbol{{Name: "Foo", Kind: "interface", LineStart: 1, LineEnd: 1}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
