package assets_test

import (
	"strings"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/assets"
	"github.com/jmeiracorbal/hybrid-coco/skills"
)

func TestAwarenessMentionsSkills(t *testing.T) {
	data, err := assets.AwarenessMD()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, name := range skills.Names {
		if !strings.Contains(text, name) {
			t.Fatalf("awareness missing skill %s", name)
		}
	}
	if !strings.Contains(text, ".hc") {
		t.Fatal("awareness missing .hc")
	}
}
