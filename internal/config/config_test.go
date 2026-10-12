package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmeiracorbal/hc/internal/config"
)

func withDataRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	config.SetDataRootForTest(dir)
	config.SetCoreVersion("test")
	t.Cleanup(func() {
		config.SetDataRootForTest("")
		config.SetCoreVersion("")
	})
	return dir
}

func TestInitAndResolve(t *testing.T) {
	data := withDataRoot(t)
	proj := t.TempDir()

	canon, id, dbPath, err := config.InitProject(proj)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(canon, config.MarkerFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dbPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	// EnsureSharedIndex creates data root; db file may not exist until store.Open
	if st, err := os.Stat(data); err != nil || !st.IsDir() {
		t.Fatalf("data root missing: %v", err)
	}
	wantDB := filepath.Join(data, config.DBFile)
	if dbPath != wantDB {
		t.Fatalf("dbPath=%s want %s", dbPath, wantDB)
	}
	if id == "" {
		t.Fatal("empty id")
	}

	root, gotID, gotDB, err := config.ResolveProject(canon)
	if err != nil {
		t.Fatal(err)
	}
	if root != canon || gotID != id || gotDB != dbPath {
		t.Fatalf("resolve mismatch root=%s id=%s db=%s", root, gotID, gotDB)
	}

	sub := filepath.Join(canon, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	root2, _, _, err := config.ResolveProject(sub)
	if err != nil || root2 != canon {
		t.Fatalf("walk-up failed: root=%s err=%v", root2, err)
	}
}

func TestMarkerRequired(t *testing.T) {
	withDataRoot(t)
	proj := t.TempDir()
	_, _, _, err := config.ResolveProject(proj)
	if !errors.Is(err, config.ErrMarkerNotFound) {
		t.Fatalf("err=%v want ErrMarkerNotFound", err)
	}
}

func TestPathChanged(t *testing.T) {
	withDataRoot(t)
	proj := t.TempDir()
	canon, _, _, err := config.InitProject(proj)
	if err != nil {
		t.Fatal(err)
	}
	// corrupt marker id
	if err := config.WriteMarker(canon, "deadbeef"); err != nil {
		t.Fatal(err)
	}
	_, _, _, err = config.ResolveProject(canon)
	if !errors.Is(err, config.ErrPathChanged) {
		t.Fatalf("err=%v want ErrPathChanged", err)
	}
}

func TestReset(t *testing.T) {
	data := withDataRoot(t)
	proj := t.TempDir()
	canon, _, dbPath, err := config.InitProject(proj)
	if err != nil {
		t.Fatal(err)
	}
	if dbPath != filepath.Join(data, config.DBFile) {
		t.Fatalf("dbPath=%s", dbPath)
	}
	if _, _, err := config.ResetProject(canon); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(canon, config.MarkerFile)); !os.IsNotExist(err) {
		t.Fatal("marker should be gone")
	}
	// ResetProject only removes marker; shared hc.db is left for caller/CLI
	if st, err := os.Stat(data); err != nil || !st.IsDir() {
		t.Fatalf("data root should remain: %v", err)
	}
}

func TestWriteMarkerRejectsEmptyID(t *testing.T) {
	config.SetCoreVersion("test")
	t.Cleanup(func() { config.SetCoreVersion("") })
	proj := t.TempDir()
	if err := config.WriteMarker(proj, ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteMarkerRejectsUnsetCoreVersion(t *testing.T) {
	config.SetCoreVersion("")
	proj := t.TempDir()
	if err := config.WriteMarker(proj, "abc"); !errors.Is(err, config.ErrCoreVersionUnset) {
		t.Fatalf("err=%v", err)
	}
}

func TestVersionMismatch(t *testing.T) {
	withDataRoot(t)
	proj := t.TempDir()
	canon, _, _, err := config.InitProject(proj)
	if err != nil {
		t.Fatal(err)
	}
	config.SetCoreVersion("other")
	_, _, _, err = config.ResolveProject(canon)
	if !errors.Is(err, config.ErrVersionMismatch) {
		t.Fatalf("err=%v want ErrVersionMismatch", err)
	}
}

func TestResetIgnoresVersionMismatch(t *testing.T) {
	withDataRoot(t)
	proj := t.TempDir()
	canon, _, _, err := config.InitProject(proj)
	if err != nil {
		t.Fatal(err)
	}
	config.SetCoreVersion("other")
	if _, _, err := config.ResetProject(canon); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(canon, config.MarkerFile)); !os.IsNotExist(err) {
		t.Fatal("marker should be gone")
	}
}

func TestDataDirNameIsHC(t *testing.T) {
	if config.DataDirName != "hc" {
		t.Fatalf("DataDirName=%q", config.DataDirName)
	}
	if config.DBFile != "hc.db" {
		t.Fatalf("DBFile=%q", config.DBFile)
	}
}

func TestClaudeConfigDir_Default(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	got, err := config.ClaudeConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".claude")
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestClaudeConfigDir_EnvOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	override := filepath.Join(home, "profiles", "work", "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", override)
	got, err := config.ClaudeConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != override {
		t.Fatalf("got %s want %s", got, override)
	}
}

func TestClaudeUserMCPFile_Default(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	got, err := config.ClaudeUserMCPFile()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".claude.json"); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestClaudeUserMCPFile_EnvOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "profiles", "work", "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", override)
	got, err := config.ClaudeUserMCPFile()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(override, ".claude.json"); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
