package skills_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/skills"
)

var (
	fmName = regexp.MustCompile(`(?m)^name:\s*(.+)$`)
	fmDesc = regexp.MustCompile(`(?m)^description:\s*(.+)$`)
)

func TestFrontmatter(t *testing.T) {
	for _, name := range skills.Names {
		data, err := skills.MD(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		mName := fmName.FindStringSubmatch(text)
		if mName == nil || strings.TrimSpace(mName[1]) != name {
			t.Fatalf("skill %s: frontmatter name mismatch (%v)", name, mName)
		}
		mDesc := fmDesc.FindStringSubmatch(text)
		if mDesc == nil || strings.TrimSpace(mDesc[1]) == "" {
			t.Fatalf("skill %s: empty description", name)
		}
		if !strings.Contains(text, ".hc") {
			t.Fatalf("skill %s: missing .hc marker mention", name)
		}
	}
}

func TestRequiredToolsMentioned(t *testing.T) {
	cases := map[string][]string{
		"hc-navigate": {"hc_file_context", "hc_package", "hc_search", "hc_symbol", "hc_status"},
		"hc-graph":    {"hc_explore", "hc_impact", "hc_path"},
		"hc-index":    {"hc init", "hc reset", "hc setup"},
		"hc-doctor":   {"hc doctor --json", "hc migrate", "hc upgrade --install --yes", "--restore-backup"},
	}
	for name, needles := range cases {
		data, err := skills.MD(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, n := range needles {
			if !strings.Contains(text, n) {
				t.Fatalf("skill %s missing %q", name, n)
			}
		}
	}
}

func TestPluginSkillsMatchCanonical(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
	for _, name := range skills.Names {
		canonical, err := skills.MD(name)
		if err != nil {
			t.Fatal(err)
		}
		pluginPath := filepath.Join(root, "plugin", "skills", name, "SKILL.md")
		disk, err := os.ReadFile(pluginPath)
		if err != nil {
			t.Fatalf("plugin skill %s: %v", name, err)
		}
		if !bytes.Equal(canonical, disk) {
			t.Fatalf("plugin/skills/%s diverges from skills/%s", name, name)
		}
	}
}
