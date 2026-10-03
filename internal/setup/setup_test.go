package setup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/setup"
	"github.com/jmeiracorbal/hybrid-coco/skills"
)

func TestInstallGlobal_Skills(t *testing.T) {
	home := t.TempDir()
	dataRoot := filepath.Join(home, ".local", "share", "hybrid-coco")
	config.SetDataRootForTest(dataRoot)
	t.Cleanup(func() { config.SetDataRootForTest("") })

	claudeDir := filepath.Join(home, ".claude")
	r, err := setup.InstallGlobal(claudeDir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.AwarenessWritten || !r.HooksInstalled {
		t.Fatalf("result=%+v", r)
	}
	if len(r.SkillsInstalled) != len(skills.Names) {
		t.Fatalf("skills=%v want %v", r.SkillsInstalled, skills.Names)
	}
	if r.SharedIndexPath == "" {
		t.Fatal("SharedIndexPath empty")
	}
	wantDB := filepath.Join(dataRoot, config.IndexFile)
	if r.SharedIndexPath != wantDB {
		t.Fatalf("SharedIndexPath=%s want %s", r.SharedIndexPath, wantDB)
	}
	if _, err := os.Stat(r.SharedIndexPath); err != nil {
		t.Fatalf("shared index.db missing: %v", err)
	}

	for _, name := range skills.Names {
		canon := filepath.Join(home, ".agents", "skills", name, "SKILL.md")
		data, err := os.ReadFile(canon)
		if err != nil {
			t.Fatalf("canonical skill %s: %v", name, err)
		}
		if !strings.Contains(string(data), "name: "+name) {
			t.Fatalf("canonical %s missing frontmatter name", name)
		}

		link := filepath.Join(claudeDir, "skills", name)
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("claude skill link %s: %v", name, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s is not a symlink", link)
		}
		target, err := filepath.EvalSymlinks(link)
		if err != nil {
			t.Fatal(err)
		}
		want, err := filepath.EvalSymlinks(filepath.Join(home, ".agents", "skills", name))
		if err != nil {
			t.Fatal(err)
		}
		if target != want {
			t.Fatalf("symlink %s -> %s want %s", link, target, want)
		}
	}
}

func TestInstallGlobal_SkillsIdempotent(t *testing.T) {
	home := t.TempDir()
	config.SetDataRootForTest(filepath.Join(home, ".local", "share", "hybrid-coco"))
	t.Cleanup(func() { config.SetDataRootForTest("") })

	claudeDir := filepath.Join(home, ".claude")
	if _, err := setup.InstallGlobal(claudeDir); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.InstallGlobal(claudeDir); err != nil {
		t.Fatal(err)
	}
}
