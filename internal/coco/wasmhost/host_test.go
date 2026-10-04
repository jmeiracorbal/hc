package wasmhost_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/coco"
	"github.com/jmeiracorbal/hybrid-coco/internal/coco/wasmhost"
)

func TestMiniCocoParse(t *testing.T) {
	root := findRepoRoot(t)
	dir := filepath.Join(root, "internal", "coco", "testdata", "mini")
	wasm := filepath.Join(dir, "test-mini-wasm32-wasi.wasm")
	if _, err := os.Stat(wasm); err != nil {
		cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", wasm, ".")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build wasm: %v\n%s", err, out)
		}
	}
	ctx := context.Background()
	mod, err := wasmhost.Load(ctx, "hc/test-mini", "minilang", []string{".mini"}, 1, wasm)
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close(ctx)

	res, err := coco.ParseAndValidate(mod, []byte("anything"), "probe.mini")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Symbols) < 2 {
		t.Fatalf("symbols=%+v", res.Symbols)
	}
	found := false
	for _, s := range res.Symbols {
		if s.Name == "Probe" && s.Kind == "class" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing Probe: %+v", res.Symbols)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
