package coco

import (
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/parsers"
)

type stubCoco struct {
	id, lang string
	exts     []string
	prio     int
}

func (s *stubCoco) ID() string           { return s.id }
func (s *stubCoco) Language() string     { return s.lang }
func (s *stubCoco) Extensions() []string { return s.exts }
func (s *stubCoco) Priority() int        { return s.prio }
func (s *stubCoco) Parse([]byte, string) (parsers.ParseResult, error) {
	return parsers.ParseResult{}, nil
}

func TestResolvePriorityAndPin(t *testing.T) {
	base := &stubCoco{id: "hc/java", lang: "java", exts: []string{".java"}, prio: 0}
	spring := &stubCoco{id: "acme/java-springboot", lang: "java-springboot", exts: []string{".java"}, prio: 20}
	r, err := NewResolver([]Coco{base, spring})
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.Resolve("Foo.java", Pins{})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID() != "acme/java-springboot" {
		t.Fatalf("got %s", c.ID())
	}
	c, err = r.Resolve("Foo.java", Pins{ExtensionPins: map[string]string{".java": "hc/java"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID() != "hc/java" {
		t.Fatalf("pin got %s", c.ID())
	}
}

func TestResolvePriorityTie(t *testing.T) {
	a := &stubCoco{id: "a/java", lang: "java", exts: []string{".java"}, prio: 5}
	b := &stubCoco{id: "b/java", lang: "java", exts: []string{".java"}, prio: 5}
	r, err := NewResolver([]Coco{a, b})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Resolve("Foo.java", Pins{})
	if err == nil {
		t.Fatal("expected tie error")
	}
}
