package migrate

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
)

// LegacyMap maps a legacy indexes/<id> directory name to an absolute project root.
type LegacyMap map[string]string

// ParseLegacyMaps parses repeated id=/abs/path flags.
func ParseLegacyMaps(flags []string) (LegacyMap, error) {
	if len(flags) == 0 {
		return nil, fmt.Errorf("--map is required (id=/abs/path)")
	}
	out := make(LegacyMap, len(flags))
	for _, f := range flags {
		id, root, ok := strings.Cut(f, "=")
		if !ok || id == "" || root == "" {
			return nil, fmt.Errorf("invalid --map %q; want id=/abs/path", f)
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("map %s: %w", id, err)
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("map %s root %s: %w", id, abs, err)
		}
		if _, dup := out[id]; dup {
			return nil, fmt.Errorf("duplicate --map id %q", id)
		}
		out[id] = abs
	}
	return out, nil
}

// ImportLegacyResult summarizes one import run.
type ImportLegacyResult struct {
	Imported []string
	Skipped  []string
	Purged   bool
}

// ImportLegacy copies per-project indexes/<id>/index.db rows into the shared DB.
// Each id in maps must exist under indexes/. Without maps covering an id, it is skipped.
// purge removes indexes/ after a successful import of at least one project.
func ImportLegacy(sharedPath string, maps LegacyMap, purge bool) (ImportLegacyResult, error) {
	var res ImportLegacyResult
	if sharedPath == "" {
		return res, fmt.Errorf("shared db path is required")
	}
	if len(maps) == 0 {
		return res, fmt.Errorf("legacy map is required")
	}
	ids, err := config.ListLegacyIndexIDs()
	if err != nil {
		return res, err
	}
	if len(ids) == 0 {
		return res, fmt.Errorf("no legacy indexes/<id>/index.db found")
	}
	present := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		present[id] = struct{}{}
	}
	for id := range maps {
		if _, ok := present[id]; !ok {
			return res, fmt.Errorf("legacy index id %q not found under indexes/", id)
		}
	}

	dst, err := OpenDB(sharedPath)
	if err != nil {
		return res, err
	}
	defer dst.Close()

	empty, err := IsEmpty(dst)
	if err != nil {
		return res, err
	}
	if empty {
		if _, err := ApplyPending(dst); err != nil {
			return res, err
		}
	} else if err := CheckReady(dst); err != nil {
		return res, err
	}

	indexesDir, err := config.LegacyIndexesDir()
	if err != nil {
		return res, err
	}

	for _, id := range ids {
		root, ok := maps[id]
		if !ok {
			res.Skipped = append(res.Skipped, id)
			continue
		}
		srcPath := filepath.Join(indexesDir, id, config.LegacyPerProjectDBFile)
		if err := importOneLegacy(dst, srcPath, id, root); err != nil {
			return res, fmt.Errorf("import %s: %w", id, err)
		}
		res.Imported = append(res.Imported, id)
	}
	if len(res.Imported) == 0 {
		return res, fmt.Errorf("nothing imported; pass --map for listed ids")
	}
	if purge {
		if err := config.PurgeLegacyIndexes(); err != nil {
			return res, err
		}
		res.Purged = true
	}
	return res, nil
}

func importOneLegacy(dst *sql.DB, srcPath, projectID, root string) error {
	src, err := OpenDB(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := src.Ping(); err != nil {
		return err
	}

	hasPID, err := dbColumnExists(src, "files", "project_id")
	if err != nil {
		return err
	}
	if hasPID {
		return fmt.Errorf("legacy db already has project_id; not a per-project index")
	}
	hasFiles, err := dbTableExists(src, "files")
	if err != nil {
		return err
	}
	if !hasFiles {
		return fmt.Errorf("legacy db missing files table")
	}

	var enrolled int
	if err := dst.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, projectID).Scan(&enrolled); err != nil {
		return err
	}
	if enrolled > 0 {
		var nFiles int
		if err := dst.QueryRow(`SELECT COUNT(*) FROM files WHERE project_id = ?`, projectID).Scan(&nFiles); err != nil {
			return err
		}
		if nFiles > 0 {
			return fmt.Errorf("project %s already enrolled with %d files", projectID, nFiles)
		}
	}

	now := time.Now().Unix()
	if _, err := dst.Exec(
		`INSERT INTO projects (id, root, enrolled_at) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root = excluded.root`,
		projectID, root, now,
	); err != nil {
		return err
	}

	tx, err := dst.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	fileMap := map[int64]int64{}
	rows, err := src.Query(`SELECT id, path, sha256, language, indexed_at FROM files`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var oldID int64
		var path, sha string
		var lang sql.NullString
		var indexedAt int64
		if err := rows.Scan(&oldID, &path, &sha, &lang, &indexedAt); err != nil {
			rows.Close()
			return err
		}
		res, err := tx.Exec(
			`INSERT INTO files (project_id, path, sha256, language, indexed_at) VALUES (?, ?, ?, ?, ?)`,
			projectID, path, sha, lang.String, indexedAt,
		)
		if err != nil {
			rows.Close()
			return err
		}
		newID, err := res.LastInsertId()
		if err != nil {
			rows.Close()
			return err
		}
		fileMap[oldID] = newID
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	symMap := map[int64]int64{}
	hasSymbols, err := dbTableExists(src, "symbols")
	if err != nil {
		return err
	}
	if hasSymbols {
		srows, err := src.Query(
			`SELECT id, file_id, name, kind, line_start, line_end, signature, docstring, parent_name FROM symbols`,
		)
		if err != nil {
			return err
		}
		for srows.Next() {
			var oldID, oldFileID int64
			var name, kind string
			var lineStart, lineEnd int
			var sig, doc, parent sql.NullString
			if err := srows.Scan(&oldID, &oldFileID, &name, &kind, &lineStart, &lineEnd, &sig, &doc, &parent); err != nil {
				srows.Close()
				return err
			}
			newFileID, ok := fileMap[oldFileID]
			if !ok {
				srows.Close()
				return fmt.Errorf("symbol %d references missing file %d", oldID, oldFileID)
			}
			res, err := tx.Exec(
				`INSERT INTO symbols (file_id, name, kind, line_start, line_end, signature, docstring, parent_name)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				newFileID, name, kind, lineStart, lineEnd, sig.String, doc.String, parent.String,
			)
			if err != nil {
				srows.Close()
				return err
			}
			newID, err := res.LastInsertId()
			if err != nil {
				srows.Close()
				return err
			}
			symMap[oldID] = newID
			if _, err := tx.Exec(
				`INSERT INTO symbols_fts (rowid, name, kind, signature, docstring) VALUES (?, ?, ?, ?, ?)`,
				newID, name, kind, sig.String, doc.String,
			); err != nil {
				srows.Close()
				return err
			}
		}
		if err := srows.Close(); err != nil {
			return err
		}
		if err := srows.Err(); err != nil {
			return err
		}
	}

	hasEdges, err := dbTableExists(src, "edges")
	if err != nil {
		return err
	}
	if hasEdges {
		hasQual, err := dbColumnExists(src, "edges", "to_qualifier")
		if err != nil {
			return err
		}
		q := `SELECT kind, from_symbol_id, to_symbol_id, to_name, from_file_id, line FROM edges`
		if hasQual {
			q = `SELECT kind, from_symbol_id, to_symbol_id, to_name, to_qualifier, from_file_id, line FROM edges`
		}
		erows, err := src.Query(q)
		if err != nil {
			return err
		}
		for erows.Next() {
			var kind, toName string
			var fromSym, toSym sql.NullInt64
			var fromFileID int64
			var line int
			var qual sql.NullString
			if hasQual {
				err = erows.Scan(&kind, &fromSym, &toSym, &toName, &qual, &fromFileID, &line)
			} else {
				err = erows.Scan(&kind, &fromSym, &toSym, &toName, &fromFileID, &line)
			}
			if err != nil {
				erows.Close()
				return err
			}
			newFileID, ok := fileMap[fromFileID]
			if !ok {
				erows.Close()
				return fmt.Errorf("edge references missing file %d", fromFileID)
			}
			var newFrom, newTo sql.NullInt64
			if fromSym.Valid {
				mapped, ok := symMap[fromSym.Int64]
				if !ok {
					erows.Close()
					return fmt.Errorf("edge references missing from_symbol %d", fromSym.Int64)
				}
				newFrom = sql.NullInt64{Int64: mapped, Valid: true}
			}
			if toSym.Valid {
				mapped, ok := symMap[toSym.Int64]
				if !ok {
					// unresolved target kept null
					newTo = sql.NullInt64{}
				} else {
					newTo = sql.NullInt64{Int64: mapped, Valid: true}
				}
			}
			if _, err := tx.Exec(
				`INSERT INTO edges (kind, from_symbol_id, to_symbol_id, to_name, to_qualifier, from_file_id, line)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				kind, newFrom, newTo, toName, nullString(qual), newFileID, line,
			); err != nil {
				erows.Close()
				return err
			}
		}
		if err := erows.Close(); err != nil {
			return err
		}
		if err := erows.Err(); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullString(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func dbTableExists(db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	return n > 0, err
}

func dbColumnExists(db *sql.DB, table, column string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`,
		table, column,
	).Scan(&n)
	return n > 0, err
}
