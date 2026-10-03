package migrate_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/migrate"
)

func seedLegacyPerProject(t *testing.T, path string) {
	t.Helper()
	db, err := migrate.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE files (
    id INTEGER PRIMARY KEY,
    path TEXT NOT NULL UNIQUE,
    sha256 TEXT NOT NULL,
    language TEXT,
    indexed_at INTEGER NOT NULL
);
CREATE TABLE symbols (
    id INTEGER PRIMARY KEY,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    line_start INTEGER NOT NULL,
    line_end INTEGER NOT NULL,
    signature TEXT,
    docstring TEXT,
    parent_name TEXT
);
CREATE TABLE edges (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL,
    from_symbol_id INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
    to_symbol_id INTEGER REFERENCES symbols(id) ON DELETE SET NULL,
    to_name TEXT NOT NULL,
    from_file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    line INTEGER NOT NULL
);
INSERT INTO meta (key, value) VALUES ('schema_version', '3');
INSERT INTO files (id, path, sha256, language, indexed_at) VALUES (1, 'a.py', 'sha', 'python', 1);
INSERT INTO symbols (id, file_id, name, kind, line_start, line_end) VALUES (1, 1, 'foo', 'function', 1, 2);
INSERT INTO edges (kind, from_symbol_id, to_name, from_file_id, line) VALUES ('calls', 1, 'bar', 1, 2);
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestImportLegacy(t *testing.T) {
	data := t.TempDir()
	config.SetDataRootForTest(data)
	t.Cleanup(func() { config.SetDataRootForTest("") })

	projRoot := t.TempDir()
	legacyID := "abc123"
	legacyDir := filepath.Join(data, "indexes", legacyID)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyDB := filepath.Join(legacyDir, config.IndexFile)
	seedLegacyPerProject(t, legacyDB)

	shared := filepath.Join(data, config.IndexFile)
	maps := migrate.LegacyMap{legacyID: projRoot}
	res, err := migrate.ImportLegacy(shared, maps, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 1 || res.Imported[0] != legacyID {
		t.Fatalf("imported=%v", res.Imported)
	}
	if !res.Purged {
		t.Fatal("expected purge")
	}
	if _, err := os.Stat(filepath.Join(data, "indexes")); !os.IsNotExist(err) {
		t.Fatal("indexes/ should be gone")
	}

	db, err := migrate.OpenDB(shared)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var root string
	if err := db.QueryRow(`SELECT root FROM projects WHERE id=?`, legacyID).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if root != projRoot {
		t.Fatalf("root=%s", root)
	}
	var nFiles, nSym, nEdges, nFTS int
	_ = db.QueryRow(`SELECT COUNT(*) FROM files WHERE project_id=?`, legacyID).Scan(&nFiles)
	_ = db.QueryRow(`SELECT COUNT(*) FROM symbols`).Scan(&nSym)
	_ = db.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&nEdges)
	_ = db.QueryRow(`SELECT COUNT(*) FROM symbols_fts`).Scan(&nFTS)
	if nFiles != 1 || nSym != 1 || nEdges != 1 || nFTS != 1 {
		t.Fatalf("files=%d symbols=%d edges=%d fts=%d", nFiles, nSym, nEdges, nFTS)
	}
	var qual sql.NullString
	if err := db.QueryRow(`SELECT to_qualifier FROM edges`).Scan(&qual); err != nil {
		t.Fatal(err)
	}
}

func TestParseLegacyMaps(t *testing.T) {
	dir := t.TempDir()
	_, err := migrate.ParseLegacyMaps(nil)
	if err == nil {
		t.Fatal("expected error")
	}
	m, err := migrate.ParseLegacyMaps([]string{"id1=" + dir})
	if err != nil {
		t.Fatal(err)
	}
	if m["id1"] == "" {
		t.Fatal("empty map")
	}
}
