package migrate_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/migrate"

	_ "modernc.org/sqlite"
)

func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := migrate.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedV4 creates a v4 shared schema without edges.to_qualifier.
func seedV4(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    root        TEXT NOT NULL UNIQUE,
    enrolled_at INTEGER NOT NULL
);
CREATE TABLE files (
    id          INTEGER PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    sha256      TEXT NOT NULL,
    language    TEXT,
    indexed_at  INTEGER NOT NULL,
    UNIQUE(project_id, path)
);
CREATE TABLE symbols (
    id          INTEGER PRIMARY KEY,
    file_id     INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL,
    line_start  INTEGER NOT NULL,
    line_end    INTEGER NOT NULL,
    signature   TEXT,
    docstring   TEXT,
    parent_name TEXT
);
CREATE TABLE edges (
    id             INTEGER PRIMARY KEY,
    kind           TEXT NOT NULL,
    from_symbol_id INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
    to_symbol_id   INTEGER REFERENCES symbols(id) ON DELETE SET NULL,
    to_name        TEXT NOT NULL,
    from_file_id   INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    line           INTEGER NOT NULL,
    UNIQUE(kind, from_symbol_id, to_name, from_file_id, line)
);
INSERT INTO meta (key, value) VALUES ('schema_version', '4');
INSERT INTO projects (id, root, enrolled_at) VALUES ('p1', '/tmp/p1', 1);
INSERT INTO files (id, project_id, path, sha256, language, indexed_at)
 VALUES (1, 'p1', 'a.py', 'sha', 'python', 1);
INSERT INTO symbols (id, file_id, name, kind, line_start, line_end)
 VALUES (1, 1, 'foo', 'function', 1, 2);
INSERT INTO edges (kind, from_symbol_id, to_name, from_file_id, line)
 VALUES ('calls', 1, 'bar', 1, 2);
`)
	if err != nil {
		t.Fatal(err)
	}
}

func hasToQualifier(t *testing.T, db *sql.DB) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('edges') WHERE name = 'to_qualifier'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestApplyPending_V4ToV5(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db := openRaw(t, path)
	seedV4(t, db)

	if hasToQualifier(t, db) {
		t.Fatal("v4 fixture must not have to_qualifier")
	}
	applied, err := migrate.ApplyPending(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 2 || applied[0] != 5 || applied[1] != 6 {
		t.Fatalf("applied=%v want [5 6]", applied)
	}
	if !hasToQualifier(t, db) {
		t.Fatal("expected to_qualifier after migrate")
	}
	ver, err := migrate.ReadVersion(db)
	if err != nil || ver != migrate.CurrentSchema() {
		t.Fatalf("version=%d err=%v want %d", ver, err, migrate.CurrentSchema())
	}
	var cocos int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='cocos'`).Scan(&cocos); err != nil || cocos != 1 {
		t.Fatalf("cocos table missing: count=%d err=%v", cocos, err)
	}
	var root string
	if err := db.QueryRow(`SELECT root FROM projects WHERE id='p1'`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if root != "/tmp/p1" {
		t.Fatalf("data lost: root=%q", root)
	}
	var edgeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&edgeCount); err != nil {
		t.Fatal(err)
	}
	if edgeCount != 1 {
		t.Fatalf("edges=%d want 1", edgeCount)
	}
}

func TestApplyPending_V5Noop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db := openRaw(t, path)
	if _, err := migrate.ApplyPending(db); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.ApplyPending(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("applied=%v want empty", applied)
	}
	if err := migrate.CheckReady(db); err != nil {
		t.Fatal(err)
	}
}

func TestPending_SchemaAhead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db := openRaw(t, path)
	if _, err := migrate.ApplyPending(db); err != nil {
		t.Fatal(err)
	}
	ahead := migrate.CurrentSchema() + 1
	if _, err := db.Exec(`UPDATE meta SET value=? WHERE key='schema_version'`, strconv.Itoa(ahead)); err != nil {
		t.Fatal(err)
	}
	_, have, want, err := migrate.Pending(db)
	if !errors.Is(err, migrate.ErrSchemaAhead) {
		t.Fatalf("err=%v want ErrSchemaAhead", err)
	}
	if have != ahead || want != migrate.CurrentSchema() {
		t.Fatalf("have=%d want=%d", have, want)
	}
	if err := migrate.CheckReady(db); !errors.Is(err, migrate.ErrSchemaAhead) {
		t.Fatalf("CheckReady err=%v", err)
	}
}

func TestCheckReady_PendingV4(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db := openRaw(t, path)
	seedV4(t, db)
	err := migrate.CheckReady(db)
	if !errors.Is(err, migrate.ErrMigrationsPending) {
		t.Fatalf("err=%v want ErrMigrationsPending", err)
	}
	var se *migrate.SchemaError
	if !errors.As(err, &se) || se.Have != 4 || se.Want != migrate.CurrentSchema() {
		t.Fatalf("SchemaError=%+v", err)
	}
}

func TestBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db := openRaw(t, path)
	if _, err := migrate.ApplyPending(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	bak, err := migrate.Backup(path)
	if err != nil {
		t.Fatal(err)
	}
	if bak == "" {
		t.Fatal("empty bak path")
	}
}
