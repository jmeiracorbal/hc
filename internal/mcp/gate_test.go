package mcp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/indexer"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
)

func TestOpenProjectStore_RequiresMarker(t *testing.T) {
	config.SetCoreVersion("test")
	t.Cleanup(func() { config.SetCoreVersion("") })
	config.SetDataRootForTest(t.TempDir())
	t.Cleanup(func() { config.SetDataRootForTest("") })

	dir := t.TempDir()
	_, _, err := openProjectStore(dir)
	if !errors.Is(err, config.ErrMarkerNotFound) {
		t.Fatalf("err=%v want ErrMarkerNotFound", err)
	}
}

func TestOpenProjectStore_WithMarkerAndIndex(t *testing.T) {
	config.SetCoreVersion("test")
	t.Cleanup(func() { config.SetCoreVersion("") })
	home := t.TempDir()
	config.SetDataRootForTest(home)
	t.Cleanup(func() { config.SetDataRootForTest("") })

	proj := t.TempDir()
	src := filepath.Join(proj, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.py"), []byte("def foo():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	canon, id, dbPath, err := config.InitProject(proj)
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

	st, gotDB, err := openProjectStore(canon)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if gotDB == "" {
		t.Fatal("empty dbPath")
	}
	stats, err := st.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files < 1 || stats.Symbols < 1 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestOpenProjectStore_MarkerWithoutEnrollment(t *testing.T) {
	config.SetCoreVersion("test")
	t.Cleanup(func() { config.SetCoreVersion("") })
	config.SetDataRootForTest(t.TempDir())
	t.Cleanup(func() { config.SetDataRootForTest("") })

	proj := t.TempDir()
	_, _, dbPath, err := config.InitProject(proj)
	if err != nil {
		t.Fatal(err)
	}
	// shared DB exists but project not enrolled
	admin, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Close(); err != nil {
		t.Fatal(err)
	}

	_, _, err = openProjectStore(proj)
	if !errors.Is(err, store.ErrProjectNotEnrolled) {
		t.Fatalf("err=%v want ErrProjectNotEnrolled", err)
	}
}

func TestWithStore_NoMarkerReturnsToolError(t *testing.T) {
	config.SetCoreVersion("test")
	t.Cleanup(func() { config.SetCoreVersion("") })
	config.SetDataRootForTest(t.TempDir())
	t.Cleanup(func() { config.SetDataRootForTest("") })

	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	res, err := withStore(func(st *store.Store, _ string) (*mcpgo.CallToolResult, error) {
		t.Fatal("fn must not run without marker")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected tool error result, got %+v", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("expected error content")
	}
	text, ok := res.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("content type %T", res.Content[0])
	}
	if !strings.Contains(text.Text, "marker .hc not found") {
		t.Fatalf("msg=%q", text.Text)
	}
}
