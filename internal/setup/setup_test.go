package setup_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmeiracorbal/hc/internal/config"
	"github.com/jmeiracorbal/hc/internal/setup"
	"github.com/jmeiracorbal/hc/skills"
)

func TestInstallGlobal_Skills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dataRoot := filepath.Join(home, ".local", "share", "hc")
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
	wantDB := filepath.Join(dataRoot, config.DBFile)
	if r.SharedIndexPath != wantDB {
		t.Fatalf("SharedIndexPath=%s want %s", r.SharedIndexPath, wantDB)
	}
	if _, err := os.Stat(r.SharedIndexPath); err != nil {
		t.Fatalf("shared hc.db missing: %v", err)
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
	t.Setenv("HOME", home)
	config.SetDataRootForTest(filepath.Join(home, ".local", "share", config.DataDirName))
	t.Cleanup(func() { config.SetDataRootForTest("") })

	claudeDir := filepath.Join(home, ".claude")
	if _, err := setup.InstallGlobal(claudeDir); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.InstallGlobal(claudeDir); err != nil {
		t.Fatal(err)
	}
}

func TestInstallGlobal_CustomClaudeConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	config.SetDataRootForTest(filepath.Join(home, ".local", "share", config.DataDirName))
	t.Cleanup(func() { config.SetDataRootForTest("") })

	claudeDir := filepath.Join(home, "profiles", "work", "claude")
	r, err := setup.InstallGlobal(claudeDir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.HooksInstalled || !r.AwarenessWritten {
		t.Fatalf("result=%+v", r)
	}

	settingsRaw, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	preHook := filepath.Join(claudeDir, "hooks", "hc-pre-tool-use.sh")
	postHook := filepath.Join(claudeDir, "hooks", "hc-post-tool-use.sh")
	if !strings.Contains(string(settingsRaw), preHook) {
		t.Fatalf("settings missing absolute pre hook %s:\n%s", preHook, settingsRaw)
	}
	if !strings.Contains(string(settingsRaw), postHook) {
		t.Fatalf("settings missing absolute post hook %s:\n%s", postHook, settingsRaw)
	}
	if strings.Contains(string(settingsRaw), "~/.claude/hooks/") {
		t.Fatal("settings still hardcodes ~/.claude/hooks/")
	}

	for _, name := range skills.Names {
		canon := filepath.Join(home, ".agents", "skills", name, "SKILL.md")
		if _, err := os.Stat(canon); err != nil {
			t.Fatalf("skills must stay under real HOME/.agents: %v", err)
		}
		link := filepath.Join(claudeDir, "skills", name)
		if _, err := os.Lstat(link); err != nil {
			t.Fatalf("skill symlink under custom claude dir: %v", err)
		}
	}
}

func TestRegisterMCP_NewProjectFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".mcp.json")
	if _, err := setup.RegisterMCP(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["mcpServers"]["hc"]["command"] != "hc" {
		t.Fatalf("hc server missing: %s", raw)
	}
}

func TestRegisterMCP_PreservesUserConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	orig := `{"numStartups": 1760000000000, "tip": "a<b && c>d", "mcpServers": {"mnemo": {"command": "mnemo"}}}`
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.RegisterMCP(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`1760000000000`, `"a<b && c>d"`, `"mnemo"`, `"hc"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in:\n%s", want, text)
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v, want 0600", st.Mode().Perm())
	}
}

func TestRegisterMCP_RefusesInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.RegisterMCP(path); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != `{not json` {
		t.Fatalf("file was modified: %s", raw)
	}
}
