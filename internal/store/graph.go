package store

import (
	"database/sql"
	"fmt"
)

const maxGraphList = 20

// Explore returns structural neighborhood for the first matching symbol.
func (s *Store) Explore(name string) (*ExploreResult, error) {
	syms, err := s.LookupSymbol(name)
	if err != nil {
		return nil, err
	}
	if len(syms) == 0 {
		return nil, nil
	}
	sym := syms[0]
	out := &ExploreResult{Symbol: sym, Parent: sym.ParentName}

	callers, err := s.callersOf(sym)
	if err != nil {
		return nil, err
	}
	out.Callers = callers
	fanIn, err := s.FanInCount(sym)
	if err != nil {
		return nil, err
	}
	out.FanIn = fanIn

	callees, err := s.calleesOf(sym.ID)
	if err != nil {
		return nil, err
	}
	out.Callees = callees
	out.FanOut = len(callees)

	children, err := s.containsOf(sym.ID)
	if err != nil {
		return nil, err
	}
	out.Children = children

	imports, err := s.importsOfFile(sym.FileID)
	if err != nil {
		return nil, err
	}
	out.Imports = imports

	return out, nil
}

// Impact returns direct callers and file-level importers (blast radius).
func (s *Store) Impact(name string) (*ImpactResult, error) {
	syms, err := s.LookupSymbol(name)
	if err != nil {
		return nil, err
	}
	if len(syms) == 0 {
		return nil, nil
	}
	sym := syms[0]
	callers, err := s.callersOf(sym)
	if err != nil {
		return nil, err
	}
	importers, err := s.importersOfFile(sym.Path)
	if err != nil {
		return nil, err
	}
	return &ImpactResult{Symbol: sym, Callers: callers, Importers: importers}, nil
}

// Path finds a calls-path from → to (BFS, max depth 6).
func (s *Store) Path(fromName, toName string) ([]PathHop, error) {
	if fromName == "" || toName == "" {
		return nil, fmt.Errorf("from and to are required")
	}
	fromSyms, err := s.LookupSymbol(fromName)
	if err != nil {
		return nil, err
	}
	if len(fromSyms) == 0 {
		return nil, nil
	}
	start := fromSyms[0]

	type node struct {
		sym   SymbolRow
		hops  []PathHop
		depth int
	}
	queue := []node{{sym: start, depth: 0}}
	seen := map[int64]struct{}{start.ID: {}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= 6 {
			continue
		}
		callees, err := s.calleesOf(cur.sym.ID)
		if err != nil {
			return nil, err
		}
		for _, c := range callees {
			hop := PathHop{
				From: cur.sym,
				To:   c.SymbolName,
				Line: c.Line,
				Path: c.Path,
			}
			if c.SymbolName == toName || (c.SymbolID != 0 && nameOfID(c) == toName) {
				return append(append([]PathHop{}, cur.hops...), hop), nil
			}
			nextID := c.SymbolID
			if nextID == 0 {
				// hop solo si nombre exacto único (sin prefix fallback de LookupSymbol)
				uid, ok, err := s.uniqueCallTargetID(c.SymbolName)
				if err != nil || !ok {
					continue
				}
				nextID = uid
			}
			if _, ok := seen[nextID]; ok {
				continue
			}
			seen[nextID] = struct{}{}
			nextSym, err := s.symbolByID(nextID)
			if err != nil || nextSym == nil {
				continue
			}
			queue = append(queue, node{
				sym:   *nextSym,
				hops:  append(append([]PathHop{}, cur.hops...), hop),
				depth: cur.depth + 1,
			})
		}
	}
	return nil, nil
}

func nameOfID(c EdgeLocus) string {
	return c.SymbolName
}

// uniqueCallTargetID returns the sole function|method|class with exact name in the project.
func (s *Store) uniqueCallTargetID(name string) (int64, bool, error) {
	if name == "" {
		return 0, false, nil
	}
	var n int
	var sid sql.NullInt64
	err := s.db.QueryRow(
		`SELECT COUNT(*), MIN(s.id) FROM symbols s
		 JOIN files f ON f.id = s.file_id
		 WHERE f.project_id = ? AND s.name = ? AND s.kind IN ('function','method','class')`,
		s.projectID, name,
	).Scan(&n, &sid)
	if err != nil {
		return 0, false, err
	}
	if n != 1 || !sid.Valid {
		return 0, false, nil
	}
	return sid.Int64, true, nil
}

func (s *Store) symbolByID(id int64) (*SymbolRow, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	rows, err := s.querySymbols(
		`SELECT s.id, s.file_id, s.name, s.kind, s.line_start, s.line_end,
		        COALESCE(s.signature,''), COALESCE(s.docstring,''), COALESCE(s.parent_name,''),
		        f.path, COALESCE(f.language,'')
		 FROM symbols s JOIN files f ON f.id = s.file_id
		 WHERE f.project_id = ? AND s.id = ?`,
		s.projectID, id,
	)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (s *Store) callersOf(sym SymbolRow) ([]EdgeLocus, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT COALESCE(fs.id, 0), COALESCE(fs.name, ''), f.path, e.line, COALESCE(fs.kind, '')
		FROM edges e
		JOIN files f ON f.id = e.from_file_id
		LEFT JOIN symbols fs ON fs.id = e.from_symbol_id
		WHERE e.kind = ?
		  AND f.project_id = ?
		  AND (e.to_symbol_id = ? OR (e.to_symbol_id IS NULL AND e.to_name = ?))
		ORDER BY f.path, e.line
		LIMIT ?`,
		EdgeCalls, s.projectID, sym.ID, sym.Name, maxGraphList,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLoci(rows)
}

func (s *Store) calleesOf(fromSymbolID int64) ([]EdgeLocus, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT COALESCE(ts.id, 0), e.to_name, f.path, e.line, COALESCE(ts.kind, '')
		FROM edges e
		JOIN files f ON f.id = e.from_file_id
		LEFT JOIN symbols ts ON ts.id = e.to_symbol_id
		WHERE e.kind = ? AND e.from_symbol_id = ? AND f.project_id = ?
		ORDER BY e.line
		LIMIT ?`,
		EdgeCalls, fromSymbolID, s.projectID, maxGraphList,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLoci(rows)
}

func (s *Store) containsOf(symbolID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT e.to_name
		FROM edges e
		WHERE e.kind = ? AND e.from_symbol_id = ?
		ORDER BY e.to_name
		LIMIT ?`,
		EdgeContains, symbolID, maxGraphList,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) importsOfFile(fileID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT to_name FROM edges
		WHERE kind = ? AND from_file_id = ?
		ORDER BY line LIMIT ?`,
		EdgeImports, fileID, maxGraphList,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) importersOfFile(path string) ([]EdgeLocus, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	base := path
	if i := lastSlash(path); i >= 0 {
		base = path[i+1:]
	}
	rows, err := s.db.Query(`
		SELECT 0, e.to_name, f.path, e.line, 'import'
		FROM edges e
		JOIN files f ON f.id = e.from_file_id
		WHERE e.kind = ?
		  AND f.project_id = ?
		  AND (e.to_name LIKE '%' || ? || '%' OR e.to_name LIKE '%' || ? || '%')
		ORDER BY f.path, e.line
		LIMIT ?`,
		EdgeImports, s.projectID, path, base, maxGraphList,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLoci(rows)
}

func lastSlash(p string) int {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return i
		}
	}
	return -1
}

func scanLoci(rows *sql.Rows) ([]EdgeLocus, error) {
	var out []EdgeLocus
	for rows.Next() {
		var e EdgeLocus
		if err := rows.Scan(&e.SymbolID, &e.SymbolName, &e.Path, &e.Line, &e.Kind); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// FanInCount returns how many call sites target this symbol (uncapped).
func (s *Store) FanInCount(sym SymbolRow) (int, error) {
	if err := s.requireProject(); err != nil {
		return 0, err
	}
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM edges e
		JOIN files f ON f.id = e.from_file_id
		WHERE e.kind = ?
		  AND f.project_id = ?
		  AND (e.to_symbol_id = ? OR (e.to_symbol_id IS NULL AND e.to_name = ?))`,
		EdgeCalls, s.projectID, sym.ID, sym.Name,
	).Scan(&n)
	return n, err
}

// CalleeNames returns distinct callee names for a symbol (for hc_symbol enrichment).
func (s *Store) CalleeNames(symbolID int64, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}
	rows, err := s.db.Query(`
		SELECT DISTINCT to_name FROM edges
		WHERE kind = ? AND from_symbol_id = ?
		ORDER BY to_name LIMIT ?`,
		EdgeCalls, symbolID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// TopCallers returns up to limit caller loci for symbol enrichment.
func (s *Store) TopCallers(sym SymbolRow, limit int) ([]EdgeLocus, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}
	rows, err := s.db.Query(`
		SELECT COALESCE(fs.id, 0), COALESCE(fs.name, ''), f.path, e.line, COALESCE(fs.kind, '')
		FROM edges e
		JOIN files f ON f.id = e.from_file_id
		LEFT JOIN symbols fs ON fs.id = e.from_symbol_id
		WHERE e.kind = ?
		  AND f.project_id = ?
		  AND (e.to_symbol_id = ? OR (e.to_symbol_id IS NULL AND e.to_name = ?))
		ORDER BY f.path, e.line
		LIMIT ?`,
		EdgeCalls, s.projectID, sym.ID, sym.Name, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLoci(rows)
}
