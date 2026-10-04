package registry

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/migrate"

	_ "modernc.org/sqlite"
)

// Record is an installed coco row.
type Record struct {
	ID           string
	Language     string
	Version      string
	Priority     int
	Contract     string
	WasmPath     string
	ManifestPath string
	InstalledAt  time.Time
}

// Open opens the shared hc.db (ensuring schema) for coco registry access.
func Open() (*sql.DB, error) {
	path, err := config.EnsureSharedIndex()
	if err != nil {
		return nil, err
	}
	db, err := migrate.OpenDB(path)
	if err != nil {
		return nil, err
	}
	empty, err := migrate.IsEmpty(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	if empty {
		if _, err := migrate.ApplyPending(db); err != nil {
			db.Close()
			return nil, err
		}
	} else if err := migrate.CheckReady(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := importLegacyCocosDB(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// importLegacyCocosDB copies rows from obsolete cocos.db into shared hc.db, then removes it.
func importLegacyCocosDB(dst *sql.DB) error {
	root, err := config.DataRoot()
	if err != nil {
		return err
	}
	legacyPath := filepath.Join(root, config.LegacyCocosDBFile)
	if _, err := os.Stat(legacyPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	src, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		return err
	}
	defer src.Close()

	rows, err := src.Query(`SELECT id, language, version, priority, contract, wasm_path, manifest_path, installed_at FROM cocos`)
	if err != nil {
		// empty/corrupt legacy: still remove so we do not keep a second DB
		if err2 := os.Remove(legacyPath); err2 != nil {
			return fmt.Errorf("read legacy cocos.db: %w (cleanup: %v)", err, err2)
		}
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var r Record
		var at string
		if err := rows.Scan(&r.ID, &r.Language, &r.Version, &r.Priority, &r.Contract, &r.WasmPath, &r.ManifestPath, &at); err != nil {
			return err
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			t, err = time.Parse(time.RFC3339, at)
			if err != nil {
				return fmt.Errorf("legacy coco %s installed_at: %w", r.ID, err)
			}
		}
		r.InstalledAt = t
		if err := Upsert(dst, r); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_ = src.Close()
	return os.Remove(legacyPath)
}

// DirForID returns the install directory for a coco id (author/name).
func DirForID(id string) (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "cocos", id), nil
}

// Upsert inserts or replaces a coco record.
func Upsert(db *sql.DB, rec Record) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	if rec.ID == "" || rec.Language == "" || rec.Version == "" || rec.WasmPath == "" || rec.ManifestPath == "" {
		return fmt.Errorf("id, language, version, wasm_path, manifest_path are required")
	}
	if rec.Contract == "" {
		return fmt.Errorf("contract is required")
	}
	if rec.InstalledAt.IsZero() {
		rec.InstalledAt = time.Now().UTC()
	}
	_, err := db.Exec(`
INSERT INTO cocos (id, language, version, priority, contract, wasm_path, manifest_path, installed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  language=excluded.language,
  version=excluded.version,
  priority=excluded.priority,
  contract=excluded.contract,
  wasm_path=excluded.wasm_path,
  manifest_path=excluded.manifest_path,
  installed_at=excluded.installed_at
`, rec.ID, rec.Language, rec.Version, rec.Priority, rec.Contract, rec.WasmPath, rec.ManifestPath, rec.InstalledAt.UTC().Format(time.RFC3339Nano))
	return err
}

// List returns all installed coco records.
func List(db *sql.DB) ([]Record, error) {
	rows, err := db.Query(`SELECT id, language, version, priority, contract, wasm_path, manifest_path, installed_at FROM cocos ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		var at string
		if err := rows.Scan(&r.ID, &r.Language, &r.Version, &r.Priority, &r.Contract, &r.WasmPath, &r.ManifestPath, &at); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			t, err = time.Parse(time.RFC3339, at)
			if err != nil {
				return nil, fmt.Errorf("coco %s installed_at: %w", r.ID, err)
			}
		}
		r.InstalledAt = t
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get returns one record by id.
func Get(db *sql.DB, id string) (Record, error) {
	var r Record
	var at string
	err := db.QueryRow(`SELECT id, language, version, priority, contract, wasm_path, manifest_path, installed_at FROM cocos WHERE id = ?`, id).
		Scan(&r.ID, &r.Language, &r.Version, &r.Priority, &r.Contract, &r.WasmPath, &r.ManifestPath, &at)
	if err != nil {
		return Record{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		t, err = time.Parse(time.RFC3339, at)
		if err != nil {
			return Record{}, err
		}
	}
	r.InstalledAt = t
	return r, nil
}

// Delete removes a coco record by id.
func Delete(db *sql.DB, id string) error {
	res, err := db.Exec(`DELETE FROM cocos WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("coco %s not found", id)
	}
	return nil
}

// ByLanguage returns installed records with the given language key.
func ByLanguage(db *sql.DB, language string) ([]Record, error) {
	rows, err := db.Query(`SELECT id, language, version, priority, contract, wasm_path, manifest_path, installed_at FROM cocos WHERE language = ?`, language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		var at string
		if err := rows.Scan(&r.ID, &r.Language, &r.Version, &r.Priority, &r.Contract, &r.WasmPath, &r.ManifestPath, &at); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339Nano, at)
		r.InstalledAt = t
		out = append(out, r)
	}
	return out, rows.Err()
}
