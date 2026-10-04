package migrate

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"

	_ "modernc.org/sqlite"
)

var (
	ErrSchemaAhead       = errors.New("schema ahead of binary; upgrade hc")
	ErrSchemaTooOld      = errors.New("schema too old; recreate with: hc doctor --fix --yes")
	ErrMigrationsPending = errors.New("migrations pending; run: hc migrate")
)

// Migration is one forward schema step.
type Migration struct {
	Version int
	Name    string
	Up      func(*sql.Tx) error
}

// SchemaError carries have/want for schema readiness failures.
type SchemaError struct {
	Have int
	Want int
	Err  error
}

func (e *SchemaError) Error() string {
	return fmt.Sprintf("%v: have %d want %d", e.Err, e.Have, e.Want)
}

func (e *SchemaError) Unwrap() error { return e.Err }

// Migrations is the ordered forward-only registry.
var Migrations = []Migration{
	{
		Version: 5,
		Name:    "baseline_v5",
		Up:      upEnsureV5,
	},
	{
		Version: 6,
		Name:    "cocos_registry",
		Up:      upEnsureV6Cocos,
	},
}

const baselineSQL = `
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    root        TEXT NOT NULL UNIQUE,
    enrolled_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS files (
    id          INTEGER PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    sha256      TEXT NOT NULL,
    language    TEXT,
    indexed_at  INTEGER NOT NULL,
    UNIQUE(project_id, path)
);

CREATE TABLE IF NOT EXISTS symbols (
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

CREATE TABLE IF NOT EXISTS edges (
    id             INTEGER PRIMARY KEY,
    kind           TEXT NOT NULL,
    from_symbol_id INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
    to_symbol_id   INTEGER REFERENCES symbols(id) ON DELETE SET NULL,
    to_name        TEXT NOT NULL,
    to_qualifier   TEXT,
    from_file_id   INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    line           INTEGER NOT NULL,
    UNIQUE(kind, from_symbol_id, to_name, from_file_id, line)
);

CREATE INDEX IF NOT EXISTS idx_files_project ON files(project_id);
CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(kind, to_symbol_id);
CREATE INDEX IF NOT EXISTS idx_edges_to_name ON edges(kind, to_name);
CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(kind, from_symbol_id);
CREATE INDEX IF NOT EXISTS idx_edges_file ON edges(from_file_id);
CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file_id);

CREATE VIRTUAL TABLE IF NOT EXISTS symbols_fts USING fts5(
    name, kind, signature, docstring,
    tokenize='trigram'
);

CREATE TRIGGER IF NOT EXISTS symbols_ad_fts
AFTER DELETE ON symbols BEGIN
    DELETE FROM symbols_fts WHERE rowid = OLD.id;
END;

CREATE TABLE IF NOT EXISTS cocos (
    id            TEXT PRIMARY KEY NOT NULL,
    language      TEXT NOT NULL,
    version       TEXT NOT NULL,
    priority      INTEGER NOT NULL,
    contract      TEXT NOT NULL,
    wasm_path     TEXT NOT NULL,
    manifest_path TEXT NOT NULL,
    installed_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cocos_language ON cocos(language);
`

func CurrentSchema() int {
	return config.SchemaVersion
}

func OpenDB(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("db path is required")
	}
	db, err := sql.Open("sqlite", openDSN(dbPath))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

func openDSN(dbPath string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(dbPath)}
	q := u.Query()
	q.Set("_foreign_keys", "on")
	q.Set("_journal_mode", "WAL")
	u.RawQuery = q.Encode()
	return u.String()
}

// ReadVersion returns meta.schema_version as int. 0 if missing or no meta table.
func ReadVersion(db *sql.DB) (int, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, config.SchemaVersionKey).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid schema_version %q: %w", value, err)
	}
	return n, nil
}

// IsEmpty reports a brand-new DB: no schema_version and no projects table/rows.
func IsEmpty(db *sql.DB) (bool, error) {
	have, err := ReadVersion(db)
	if err != nil {
		return false, err
	}
	if have != 0 {
		return false, nil
	}
	var tableCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='projects'`,
	).Scan(&tableCount); err != nil {
		return false, err
	}
	if tableCount == 0 {
		return true, nil
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}

// Pending returns migrations with Version in (have, want].
func Pending(db *sql.DB) (pending []Migration, have, want int, err error) {
	have, err = ReadVersion(db)
	if err != nil {
		return nil, 0, 0, err
	}
	want = CurrentSchema()
	if have > want {
		return nil, have, want, &SchemaError{Have: have, Want: want, Err: ErrSchemaAhead}
	}
	for _, m := range Migrations {
		if m.Version > have && m.Version <= want {
			pending = append(pending, m)
		}
	}
	return pending, have, want, nil
}

// CheckReady returns nil when schema matches current binary.
// Empty DBs are ready for Open to ApplyPending baseline.
func CheckReady(db *sql.DB) error {
	empty, err := IsEmpty(db)
	if err != nil {
		return err
	}
	if empty {
		return nil
	}
	have, err := ReadVersion(db)
	if err != nil {
		return err
	}
	want := CurrentSchema()
	if have == want {
		return nil
	}
	if have > want {
		return &SchemaError{Have: have, Want: want, Err: ErrSchemaAhead}
	}
	if have > 0 && have < 4 {
		return &SchemaError{Have: have, Want: want, Err: ErrSchemaTooOld}
	}
	// have==0 con datos, o have in [4, want)
	return &SchemaError{Have: have, Want: want, Err: ErrMigrationsPending}
}

// ApplyPending runs each pending migration in its own transaction.
func ApplyPending(db *sql.DB) (applied []int, err error) {
	pending, have, want, err := Pending(db)
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return nil, nil
	}
	empty, err := IsEmpty(db)
	if err != nil {
		return nil, err
	}
	if !empty && have > 0 && have < 4 {
		return nil, &SchemaError{Have: have, Want: want, Err: ErrSchemaTooOld}
	}
	for _, m := range pending {
		if m.Up == nil {
			return applied, fmt.Errorf("migration %d (%s): Up is required", m.Version, m.Name)
		}
		tx, err := db.Begin()
		if err != nil {
			return applied, err
		}
		if err := m.Up(tx); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
		}
		if err := setVersion(tx, m.Version); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("migration %d (%s) set version: %w", m.Version, m.Name, err)
		}
		if err := tx.Commit(); err != nil {
			return applied, err
		}
		applied = append(applied, m.Version)
	}
	return applied, nil
}

func setVersion(tx *sql.Tx, version int) error {
	_, err := tx.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		config.SchemaVersionKey, strconv.Itoa(version),
	)
	return err
}

func upEnsureV5(tx *sql.Tx) error {
	var projectsTable int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='projects'`,
	).Scan(&projectsTable); err != nil {
		return err
	}
	if projectsTable == 0 {
		if _, err := tx.Exec(baselineSQL); err != nil {
			return err
		}
		return nil
	}
	var edgesTable int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='edges'`,
	).Scan(&edgesTable); err != nil {
		return err
	}
	if edgesTable > 0 {
		hasCol, err := columnExists(tx, "edges", "to_qualifier")
		if err != nil {
			return err
		}
		if !hasCol {
			if _, err := tx.Exec(`ALTER TABLE edges ADD COLUMN to_qualifier TEXT`); err != nil {
				return err
			}
		}
	}
	// asegurar meta existe para setVersion posterior
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`); err != nil {
		return err
	}
	return nil
}

func upEnsureV6Cocos(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS cocos (
    id            TEXT PRIMARY KEY NOT NULL,
    language      TEXT NOT NULL,
    version       TEXT NOT NULL,
    priority      INTEGER NOT NULL,
    contract      TEXT NOT NULL,
    wasm_path     TEXT NOT NULL,
    manifest_path TEXT NOT NULL,
    installed_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cocos_language ON cocos(language);
`)
	return err
}

func columnExists(tx *sql.Tx, table, column string) (bool, error) {
	var n int
	err := tx.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`,
		table, column,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Backup checkpoints WAL then copies dbPath to <db>.bak.<utc>.
func Backup(dbPath string) (bakPath string, err error) {
	if dbPath == "" {
		return "", fmt.Errorf("db path is required")
	}
	if _, err := os.Stat(dbPath); err != nil {
		return "", err
	}
	db, err := OpenDB(dbPath)
	if err != nil {
		return "", err
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(FULL)`); err != nil {
		_ = db.Close()
		return "", fmt.Errorf("wal_checkpoint: %w", err)
	}
	if err := db.Close(); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	bakPath = dbPath + ".bak." + stamp
	if err := copyFile(dbPath, bakPath); err != nil {
		return "", err
	}
	return bakPath, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}
