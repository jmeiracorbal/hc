package hooktest

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/indexer"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
)

func TestPreToolUseAllowsRangedRead(t *testing.T) {
	// capture before HOME override — go build must not write GOMODCACHE under t.TempDir
	buildEnv := append([]string(nil), os.Environ()...)

	home := t.TempDir()
	t.Setenv("HOME", home)
	config.SetDataRootForTest("")
	config.SetCoreVersion("dev") // matches default main.version in built binary
	t.Cleanup(func() { config.SetCoreVersion("") })

	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	rel := "src/m.py"
	if err := os.WriteFile(filepath.Join(root, rel), []byte("def foo():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	canon, id, dbPath, err := config.InitProject(root)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.EnrollProject(id, canon); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	if err := admin.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := indexer.IndexPath(canon, false); err != nil {
		t.Fatal(err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	hcDir := t.TempDir()
	hcBin := filepath.Join(hcDir, "hc")
	build := exec.Command("go", "build", "-o", hcBin, "./cmd/hc")
	build.Dir = repoRoot
	build.Env = buildEnv
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hc: %v\n%s", err, out)
	}

	hook := filepath.Join(repoRoot, "assets", "hooks", "hc-pre-tool-use.sh")
	absFile := filepath.Join(canon, rel)
	pathEnv := hcDir + string(os.PathListSeparator) + os.Getenv("PATH")

	runHook := func(payload any) string {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", hook)
		cmd.Dir = canon
		cmd.Env = append(os.Environ(), "PATH="+pathEnv, "HOME="+home)
		cmd.Stdin = bytes.NewReader(body)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				t.Fatalf("hook run: %v\n%s", err, out.String())
			}
		}
		return out.String()
	}

	fullOut := runHook(map[string]any{
		"tool_name":  "Read",
		"tool_input": map[string]any{"file_path": absFile},
	})
	if !strings.Contains(fullOut, `"decision":"block"`) {
		t.Fatalf("expected block, got: %s", fullOut)
	}
	if !strings.Contains(fullOut, "read offset=") {
		t.Fatalf("expected range hints: %s", fullOut)
	}

	rangeOut := runHook(map[string]any{
		"tool_name": "Read",
		"tool_input": map[string]any{
			"file_path": absFile,
			"offset":    1,
			"limit":     2,
		},
	})
	if strings.Contains(rangeOut, `"decision":"block"`) {
		t.Fatalf("ranged Read must not block: %s", rangeOut)
	}

	partialOut := runHook(map[string]any{
		"tool_name": "Read",
		"tool_input": map[string]any{
			"file_path": absFile,
			"offset":    1,
		},
	})
	if !strings.Contains(partialOut, `"decision":"block"`) {
		t.Fatalf("offset-only Read must block: %s", partialOut)
	}
}

func TestPreToolUseNoMarkerNoop(t *testing.T) {
	root := t.TempDir()
	rel := "src/m.py"
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(repoRoot, "assets", "hooks", "hc-pre-tool-use.sh")
	payload, _ := json.Marshal(map[string]any{
		"tool_name":  "Read",
		"tool_input": map[string]any{"file_path": filepath.Join(root, rel)},
	})
	cmd := exec.Command("bash", hook)
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(payload)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), `"decision":"block"`) {
		t.Fatalf("without .hc must not block: %s", out.String())
	}
}
