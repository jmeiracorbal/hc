package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/migrate"
)

func TestOpen_PendingMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db, err := migrate.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE projects (
    id TEXT PRIMARY KEY, root TEXT NOT NULL UNIQUE, enrolled_at INTEGER NOT NULL
);
CREATE TABLE files (
    id INTEGER PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    path TEXT NOT NULL, sha256 TEXT NOT NULL, language TEXT, indexed_at INTEGER NOT NULL,
    UNIQUE(project_id, path)
);
CREATE TABLE symbols (
    id INTEGER PRIMARY KEY, file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name TEXT NOT NULL, kind TEXT NOT NULL, line_start INTEGER NOT NULL, line_end INTEGER NOT NULL,
    signature TEXT, docstring TEXT, parent_name TEXT
);
CREATE TABLE edges (
    id INTEGER PRIMARY KEY, kind TEXT NOT NULL,
    from_symbol_id INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
    to_symbol_id INTEGER REFERENCES symbols(id) ON DELETE SET NULL,
    to_name TEXT NOT NULL,
    from_file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    line INTEGER NOT NULL,
    UNIQUE(kind, from_symbol_id, to_name, from_file_id, line)
);
INSERT INTO meta (key, value) VALUES ('schema_version', '4');
INSERT INTO projects (id, root, enrolled_at) VALUES ('p1', '/r', 1);
`)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(path)
	if !errors.Is(err, ErrMigrationsPending) {
		t.Fatalf("err=%v want ErrMigrationsPending", err)
	}
}

func TestOpen_EmptyBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := migrate.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ver, err := migrate.ReadVersion(db)
	if err != nil || ver != migrate.CurrentSchema() {
		t.Fatalf("ver=%d err=%v", ver, err)
	}
}
