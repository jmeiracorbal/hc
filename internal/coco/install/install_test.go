package install_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/coco/install"
	"github.com/jmeiracorbal/hybrid-coco/internal/config"
)

func TestFromLocalMiniCoco(t *testing.T) {
	root := findRepoRoot(t)
	tmp := t.TempDir()
	config.SetDataRootForTest(tmp)
	t.Cleanup(func() { config.SetDataRootForTest("") })
	config.SetCoreVersion("v0.1.2")
	t.Cleanup(func() { config.SetCoreVersion("") })

	dir := filepath.Join(root, "internal", "coco", "testdata", "mini")
	ctx := context.Background()
	res, err := install.FromLocal(ctx, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "hc/test-mini" {
		t.Fatalf("id=%s", res.ID)
	}
	if _, err := os.Stat(filepath.Join(res.Dir, "coco.wasm")); err != nil {
		t.Fatal(err)
	}
	if err := install.Uninstall("hc/test-mini"); err != nil {
		t.Fatal(err)
	}
}

func TestParseRef(t *testing.T) {
	r, err := install.ParseRef("github.com/acme/java-coco@v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if r.Module != "github.com/acme/java-coco" || r.Version != "v1.2.3" {
		t.Fatalf("%+v", r)
	}
	if _, err := install.ParseRef("github.com/acme/java-coco@latest"); err == nil {
		t.Fatal("expected error")
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
