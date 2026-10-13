package setup_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmeiracorbal/hc/internal/setup"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	return dir
}

func ignored(t *testing.T, dir, path string) bool {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "-q", path)
	cmd.Dir = dir
	return cmd.Run() == nil
}

func TestExcludeLocal_RepoRoot(t *testing.T) {
	repo := gitRepo(t)
	added, err := setup.ExcludeLocal(repo, ".hc", ".mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/.hc", "/.mcp.json"}; !reflect.DeepEqual(added, want) {
		t.Fatalf("added=%v want %v", added, want)
	}
	for _, p := range []string{".hc", ".mcp.json"} {
		if !ignored(t, repo, p) {
			t.Fatalf("%s not ignored", p)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".gitignore")); !os.IsNotExist(err) {
		t.Fatal(".gitignore must not be created")
	}
}

func TestExcludeLocal_Idempotent(t *testing.T) {
	repo := gitRepo(t)
	if _, err := setup.ExcludeLocal(repo, ".hc"); err != nil {
		t.Fatal(err)
	}
	added, err := setup.ExcludeLocal(repo, ".hc")
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 0 {
		t.Fatalf("added=%v on second run", added)
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), "/.hc\n"); n != 1 {
		t.Fatalf("pattern written %d times", n)
	}
}

func TestExcludeLocal_SubdirIsAnchored(t *testing.T) {
	repo := gitRepo(t)
	sub := filepath.Join(repo, "svc", "api")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	added, err := setup.ExcludeLocal(sub, ".mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/svc/api/.mcp.json"}; !reflect.DeepEqual(added, want) {
		t.Fatalf("added=%v want %v", added, want)
	}
	if !ignored(t, repo, "svc/api/.mcp.json") {
		t.Fatal("subdir .mcp.json not ignored")
	}
	if ignored(t, repo, ".mcp.json") {
		t.Fatal("root .mcp.json must stay visible")
	}
}

func TestExcludeLocal_Worktree(t *testing.T) {
	repo := gitRepo(t)
	gitRun(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	gitRun(t, repo, "worktree", "add", "-q", wt)
	if _, err := setup.ExcludeLocal(wt, ".hc"); err != nil {
		t.Fatal(err)
	}
	if !ignored(t, wt, ".hc") {
		t.Fatal(".hc not ignored in worktree")
	}
}

func TestExcludeLocal_NotARepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	added, err := setup.ExcludeLocal(dir, ".hc")
	if err != nil || added != nil {
		t.Fatalf("added=%v err=%v, want nil/nil", added, err)
	}
}
