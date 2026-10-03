package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jmeiracorbal/hybrid-coco/internal/migrate"
	"github.com/jmeiracorbal/hybrid-coco/internal/parsers"

	_ "modernc.org/sqlite"
)

var (
	ErrIncompatibleSchema = errors.New("shared index schema incompatible; run: hc doctor --fix --yes")
	ErrMigrationsPending  = migrate.ErrMigrationsPending
	ErrNotFound           = errors.New("not found")
	ErrProjectNotEnrolled = errors.New("project not enrolled in shared index; run: hc init")
	ErrProjectRequired    = errors.New("store project id is required")
)

const (
	EdgeCalls    = "calls"
	EdgeContains = "contains"
	EdgeImports  = "imports"
)

type Store struct {
	db        *sql.DB
	projectID string
}

type FileRow struct {
	ID        int64
	Path      string
	SHA256    string
	Language  string
	IndexedAt int64
}

type SymbolRow struct {
	ID         int64
	FileID     int64
	Name       string
	Kind       string
	LineStart  int
	LineEnd    int
	Signature  string
	Docstring  string
	ParentName string
	Path       string
	Language   string
}

type EdgeInput struct {
	Kind         string
	FromSymbolID sql.NullInt64
	ToSymbolID   sql.NullInt64
	ToName       string
	ToQualifier  string
	FromFileID   int64
	Line         int
}

type LangCallStats struct {
	Language string
	Total    int
	Resolved int
}

type EdgeLocus struct {
	SymbolID   int64
	SymbolName string
	Path       string
	Line       int
	Kind       string
}

type ExploreResult struct {
	Symbol   SymbolRow
	FanIn    int
	FanOut   int
	Callers  []EdgeLocus
	Callees  []EdgeLocus
	Parent   string
	Children []string
	Imports  []string
}

type ImpactResult struct {
	Symbol    SymbolRow
	Callers   []EdgeLocus
	Importers []EdgeLocus
}

type PathHop struct {
	From SymbolRow
	To   string
	Line int
	Path string
}

type Stats struct {
	Files           int
	Symbols         int
	Edges           int
	CallsTotal      int
	CallsResolved   int
	CallsByLanguage []LangCallStats
	ByKind          map[string]int
	LastIndexed     int64
	Hotspots        []Hotspot
}

type Hotspot struct {
	Name  string
	Path  string
	FanIn int
}

type FileContext struct {
	Path     string
	Language string
	Symbols  []SymbolRow
}

// Open opens the shared index (admin: no project scope).
// Empty DB: applies baseline migrations. Non-empty with pending: ErrMigrationsPending.
func Open(dbPath string) (*Store, error) {
	db, err := migrate.OpenDB(dbPath)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.assertForeignKeys(); err != nil {
		_ = db.Close()
		return nil, err
	}
	empty, err := migrate.IsEmpty(db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if empty {
		if _, err := migrate.ApplyPending(db); err != nil {
			_ = db.Close()
			return nil, err
		}
		return s, nil
	}
	if err := migrate.CheckReady(db); err != nil {
		_ = db.Close()
		if errors.Is(err, migrate.ErrMigrationsPending) {
			return nil, fmt.Errorf("%w", err)
		}
		if errors.Is(err, migrate.ErrSchemaTooOld) {
			return nil, fmt.Errorf("%w: %v", ErrIncompatibleSchema, err)
		}
		if errors.Is(err, migrate.ErrSchemaAhead) {
			return nil, err
		}
		return nil, err
	}
	return s, nil
}

// OpenForProject opens the shared index scoped to an enrolled project.
func OpenForProject(dbPath, projectID string) (*Store, error) {
	if projectID == "" {
		return nil, ErrProjectRequired
	}
	s, err := Open(dbPath)
	if err != nil {
		return nil, err
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, projectID).Scan(&n); err != nil {
		_ = s.Close()
		return nil, err
	}
	if n == 0 {
		_ = s.Close()
		return nil, ErrProjectNotEnrolled
	}
	s.projectID = projectID
	return s, nil
}

func (s *Store) ProjectID() string {
	return s.projectID
}

func (s *Store) requireProject() error {
	if s.projectID == "" {
		return ErrProjectRequired
	}
	return nil
}

// EnrollProject inserts or refreshes a project row (admin store).
func (s *Store) EnrollProject(id, root string) error {
	if id == "" {
		return fmt.Errorf("project id is required")
	}
	if root == "" {
		return fmt.Errorf("project root is required")
	}
	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT INTO projects (id, root, enrolled_at) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root = excluded.root`,
		id, root, now,
	)
	return err
}

// DeleteProject removes a project and all its index rows via CASCADE (admin store).
func (s *Store) DeleteProject(id string) error {
	if id == "" {
		return fmt.Errorf("project id is required")
	}
	_, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

func (s *Store) assertForeignKeys() error {
	var on int
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&on); err != nil {
		return fmt.Errorf("pragma foreign_keys: %w", err)
	}
	if on != 1 {
		return errors.New("sqlite foreign_keys pragma is off")
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) GetFile(path string) (*FileRow, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	row := s.db.QueryRow(
		`SELECT id, path, sha256, COALESCE(language,''), indexed_at
		 FROM files WHERE project_id = ? AND path = ?`,
		s.projectID, path,
	)
	var f FileRow
	if err := row.Scan(&f.ID, &f.Path, &f.SHA256, &f.Language, &f.IndexedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func (s *Store) UpsertFile(path, sha256, language string) (int64, error) {
	if err := s.requireProject(); err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	row := s.db.QueryRow(
		`INSERT INTO files (project_id, path, sha256, language, indexed_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, path) DO UPDATE SET
		   sha256=excluded.sha256,
		   language=excluded.language,
		   indexed_at=excluded.indexed_at
		 RETURNING id`,
		s.projectID, path, sha256, nullIfEmpty(language), now,
	)
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) DeleteFileSymbols(fileID int64) error {
	_, err := s.db.Exec(`DELETE FROM symbols WHERE file_id = ?`, fileID)
	return err
}

func (s *Store) DeleteFileEdges(fileID int64) error {
	_, err := s.db.Exec(`DELETE FROM edges WHERE from_file_id = ?`, fileID)
	return err
}

func (s *Store) DeleteFile(path string) error {
	f, err := s.GetFile(path)
	if err != nil {
		return err
	}
	if f == nil {
		return nil
	}
	_, err = s.db.Exec(`DELETE FROM files WHERE id = ? AND project_id = ?`, f.ID, s.projectID)
	return err
}

func (s *Store) InsertSymbols(fileID int64, symbols []parsers.Symbol) error {
	for _, sym := range symbols {
		res, err := s.db.Exec(
			`INSERT INTO symbols
			 (file_id, name, kind, line_start, line_end, signature, docstring, parent_name)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			fileID, sym.Name, sym.Kind, sym.LineStart, sym.LineEnd,
			nullIfEmpty(sym.Signature), nullIfEmpty(sym.Docstring), nullIfEmpty(sym.ParentName),
		)
		if err != nil {
			return err
		}
		rowid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(
			`INSERT INTO symbols_fts (rowid, name, kind, signature, docstring)
			 VALUES (?, ?, ?, ?, ?)`,
			rowid, sym.Name, sym.Kind, sym.Signature, sym.Docstring,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) InsertEdges(edges []EdgeInput) error {
	for _, e := range edges {
		if e.ToName == "" {
			return fmt.Errorf("edge to_name is required")
		}
		if e.Kind == "" {
			return fmt.Errorf("edge kind is required")
		}
		if e.FromFileID == 0 {
			return fmt.Errorf("edge from_file_id is required")
		}
		_, err := s.db.Exec(
			`INSERT OR IGNORE INTO edges
			 (kind, from_symbol_id, to_symbol_id, to_name, to_qualifier, from_file_id, line)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			e.Kind, nullInt64(e.FromSymbolID), nullInt64(e.ToSymbolID), e.ToName,
			nullIfEmpty(e.ToQualifier), e.FromFileID, e.Line,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SymbolsByFile(fileID int64) ([]SymbolRow, error) {
	rows, err := s.db.Query(
		`SELECT id, file_id, name, kind, line_start, line_end,
		        COALESCE(signature,''), COALESCE(docstring,''), COALESCE(parent_name,''),
		        '', ''
		 FROM symbols WHERE file_id = ?`,
		fileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SymbolRow
	for rows.Next() {
		var r SymbolRow
		if err := rows.Scan(
			&r.ID, &r.FileID, &r.Name, &r.Kind, &r.LineStart, &r.LineEnd,
			&r.Signature, &r.Docstring, &r.ParentName, &r.Path, &r.Language,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ResolveAllEdges links call edges to symbols. Precision over recall:
// qualifier+name unique, else same-file unique name, else project-wide unique name.
// Only kind=calls; contains/imports keep to_symbol_id from buildEdges.
func (s *Store) ResolveAllEdges() error {
	if err := s.requireProject(); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		UPDATE edges SET to_symbol_id = NULL
		WHERE kind = ?
		  AND from_file_id IN (SELECT id FROM files WHERE project_id = ?)`,
		EdgeCalls, s.projectID,
	)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(`
		SELECT e.id, e.to_name, e.from_file_id, COALESCE(e.to_qualifier,'') FROM edges e
		JOIN files f ON f.id = e.from_file_id
		WHERE f.project_id = ? AND e.kind = ?`,
		s.projectID, EdgeCalls,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type pair struct {
		id        int64
		name      string
		fromFile  int64
		qualifier string
	}
	var pending []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.name, &p.fromFile, &p.qualifier); err != nil {
			return err
		}
		pending = append(pending, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range pending {
		sid, ok, err := s.resolveCallTarget(p.name, p.qualifier, p.fromFile)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if _, err := s.db.Exec(`UPDATE edges SET to_symbol_id = ? WHERE id = ?`, sid, p.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) resolveCallTarget(name, qualifier string, fromFileID int64) (int64, bool, error) {
	const kinds = `s.kind IN ('function','method','class')`
	var n int
	var sid sql.NullInt64

	if qualifier != "" {
		err := s.db.QueryRow(
			`SELECT COUNT(*), MIN(s.id) FROM symbols s
			 WHERE s.file_id = ? AND s.name = ? AND s.parent_name = ? AND `+kinds,
			fromFileID, name, qualifier,
		).Scan(&n, &sid)
		if err != nil {
			return 0, false, err
		}
		if n == 1 && sid.Valid {
			return sid.Int64, true, nil
		}
		err = s.db.QueryRow(
			`SELECT COUNT(*), MIN(s.id) FROM symbols s
			 JOIN files f ON f.id = s.file_id
			 WHERE f.project_id = ? AND s.name = ? AND s.parent_name = ? AND `+kinds,
			s.projectID, name, qualifier,
		).Scan(&n, &sid)
		if err != nil {
			return 0, false, err
		}
		if n == 1 && sid.Valid {
			return sid.Int64, true, nil
		}
		return 0, false, nil
	}

	err := s.db.QueryRow(
		`SELECT COUNT(*), MIN(s.id) FROM symbols s
		 WHERE s.file_id = ? AND s.name = ? AND `+kinds,
		fromFileID, name,
	).Scan(&n, &sid)
	if err != nil {
		return 0, false, err
	}
	if n == 1 && sid.Valid {
		return sid.Int64, true, nil
	}
	err = s.db.QueryRow(
		`SELECT COUNT(*), MIN(s.id) FROM symbols s
		 JOIN files f ON f.id = s.file_id
		 WHERE f.project_id = ? AND s.name = ? AND `+kinds,
		s.projectID, name,
	).Scan(&n, &sid)
	if err != nil {
		return 0, false, err
	}
	if n == 1 && sid.Valid {
		return sid.Int64, true, nil
	}
	return 0, false, nil
}

func (s *Store) Stats() (Stats, error) {
	if err := s.requireProject(); err != nil {
		return Stats{}, err
	}
	var st Stats
	st.ByKind = map[string]int{}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE project_id = ?`, s.projectID).Scan(&st.Files); err != nil {
		return st, err
	}
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM symbols s
		JOIN files f ON f.id = s.file_id WHERE f.project_id = ?`, s.projectID).Scan(&st.Symbols); err != nil {
		return st, err
	}
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM edges e
		JOIN files f ON f.id = e.from_file_id WHERE f.project_id = ?`, s.projectID).Scan(&st.Edges); err != nil {
		return st, err
	}
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM edges e
		JOIN files f ON f.id = e.from_file_id
		WHERE f.project_id = ? AND e.kind = ?`, s.projectID, EdgeCalls).Scan(&st.CallsTotal); err != nil {
		return st, err
	}
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM edges e
		JOIN files f ON f.id = e.from_file_id
		WHERE f.project_id = ? AND e.kind = ? AND e.to_symbol_id IS NOT NULL`,
		s.projectID, EdgeCalls).Scan(&st.CallsResolved); err != nil {
		return st, err
	}
	langRows, err := s.db.Query(`
		SELECT COALESCE(f.language,''),
		       COUNT(*),
		       SUM(CASE WHEN e.to_symbol_id IS NOT NULL THEN 1 ELSE 0 END)
		FROM edges e
		JOIN files f ON f.id = e.from_file_id
		WHERE f.project_id = ? AND e.kind = ?
		GROUP BY COALESCE(f.language,'')`,
		s.projectID, EdgeCalls)
	if err != nil {
		return st, err
	}
	for langRows.Next() {
		var lc LangCallStats
		if err := langRows.Scan(&lc.Language, &lc.Total, &lc.Resolved); err != nil {
			_ = langRows.Close()
			return st, err
		}
		st.CallsByLanguage = append(st.CallsByLanguage, lc)
	}
	if err := langRows.Err(); err != nil {
		_ = langRows.Close()
		return st, err
	}
	if err := langRows.Close(); err != nil {
		return st, err
	}
	sort.Slice(st.CallsByLanguage, func(i, j int) bool {
		ui := st.CallsByLanguage[i].Total - st.CallsByLanguage[i].Resolved
		uj := st.CallsByLanguage[j].Total - st.CallsByLanguage[j].Resolved
		if ui != uj {
			return ui > uj
		}
		return st.CallsByLanguage[i].Language < st.CallsByLanguage[j].Language
	})
	rows, err := s.db.Query(`
		SELECT s.kind, COUNT(*) FROM symbols s
		JOIN files f ON f.id = s.file_id
		WHERE f.project_id = ?
		GROUP BY s.kind ORDER BY COUNT(*) DESC`, s.projectID)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			return st, err
		}
		st.ByKind[kind] = n
	}
	var last sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(indexed_at) FROM files WHERE project_id = ?`, s.projectID).Scan(&last); err != nil {
		return st, err
	}
	if last.Valid {
		st.LastIndexed = last.Int64
	}
	hot, err := s.Hotspots(5)
	if err != nil {
		return st, err
	}
	st.Hotspots = hot
	return st, nil
}

func (s *Store) Hotspots(limit int) ([]Hotspot, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}
	rows, err := s.db.Query(`
		SELECT e.to_name, COALESCE(tf.path, ''), COUNT(*) AS fan_in
		FROM edges e
		JOIN files ff ON ff.id = e.from_file_id
		LEFT JOIN symbols ts ON ts.id = e.to_symbol_id
		LEFT JOIN files tf ON tf.id = ts.file_id
		WHERE e.kind = ? AND ff.project_id = ?
		GROUP BY e.to_name
		ORDER BY fan_in DESC, e.to_name
		LIMIT ?`, EdgeCalls, s.projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Hotspot
	for rows.Next() {
		var h Hotspot
		if err := rows.Scan(&h.Name, &h.Path, &h.FanIn); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) LookupSymbol(name string) ([]SymbolRow, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	rows, err := s.querySymbols(
		`SELECT s.id, s.file_id, s.name, s.kind, s.line_start, s.line_end,
		        COALESCE(s.signature,''), COALESCE(s.docstring,''), COALESCE(s.parent_name,''),
		        f.path, COALESCE(f.language,'')
		 FROM symbols s JOIN files f ON f.id = s.file_id
		 WHERE f.project_id = ? AND lower(s.name) = lower(?)
		 ORDER BY s.kind, f.path, s.line_start
		 LIMIT 20`,
		s.projectID, name,
	)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	return s.querySymbols(
		`SELECT s.id, s.file_id, s.name, s.kind, s.line_start, s.line_end,
		        COALESCE(s.signature,''), COALESCE(s.docstring,''), COALESCE(s.parent_name,''),
		        f.path, COALESCE(f.language,'')
		 FROM symbols s JOIN files f ON f.id = s.file_id
		 WHERE f.project_id = ? AND lower(s.name) LIKE lower(?) || '%'
		 ORDER BY s.kind, f.path, s.line_start
		 LIMIT 20`,
		s.projectID, name,
	)
}

func (s *Store) FTSSearch(query string, limit int) ([]SymbolRow, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}
	return s.querySymbols(
		`SELECT s.id, s.file_id, s.name, s.kind, s.line_start, s.line_end,
		        COALESCE(s.signature,''), COALESCE(s.docstring,''), COALESCE(s.parent_name,''),
		        f.path, COALESCE(f.language,'')
		 FROM symbols s JOIN files f ON f.id = s.file_id
		 WHERE f.project_id = ?
		   AND s.id IN (
		     SELECT rowid FROM symbols_fts WHERE symbols_fts MATCH ?
		   )
		 ORDER BY f.path, s.line_start
		 LIMIT ?`,
		s.projectID, query, limit,
	)
}

func (s *Store) FileContext(path string) (*FileContext, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	var id int64
	var lang sql.NullString
	err := s.db.QueryRow(
		`SELECT id, language FROM files WHERE project_id = ? AND path = ?`,
		s.projectID, path,
	).Scan(&id, &lang)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT name, kind, line_start, line_end, COALESCE(signature,''), COALESCE(parent_name,'')
		 FROM symbols WHERE file_id = ? ORDER BY line_start`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fc := &FileContext{Path: path, Language: lang.String}
	for rows.Next() {
		var sym SymbolRow
		if err := rows.Scan(&sym.Name, &sym.Kind, &sym.LineStart, &sym.LineEnd, &sym.Signature, &sym.ParentName); err != nil {
			return nil, err
		}
		fc.Symbols = append(fc.Symbols, sym)
	}
	return fc, nil
}

func (s *Store) AllFiles() ([]FileRow, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, path, sha256, COALESCE(language,''), indexed_at
		 FROM files WHERE project_id = ? ORDER BY path`,
		s.projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileRow
	for rows.Next() {
		var f FileRow
		if err := rows.Scan(&f.ID, &f.Path, &f.SHA256, &f.Language, &f.IndexedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (s *Store) querySymbols(query string, args ...any) ([]SymbolRow, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SymbolRow
	for rows.Next() {
		var r SymbolRow
		if err := rows.Scan(
			&r.ID, &r.FileID, &r.Name, &r.Kind, &r.LineStart, &r.LineEnd,
			&r.Signature, &r.Docstring, &r.ParentName, &r.Path, &r.Language,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt64(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}
