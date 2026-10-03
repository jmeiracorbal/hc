package store

import (
	"fmt"
	"strings"
)

// PackageEntry is one symbol under a directory prefix, with call fan-in.
type PackageEntry struct {
	Symbol SymbolRow
	FanIn  int
}

// PackageOutline is the compact map of a directory / package.
type PackageOutline struct {
	Dir       string
	FileCount int
	Entries   []PackageEntry
}

// PackageOutline returns symbols under dir (path == dir or path has dir/ prefix).
// dir is required and must be non-empty after trimming a single trailing slash.
func (s *Store) PackageOutline(dir string) (*PackageOutline, error) {
	if err := s.requireProject(); err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, fmt.Errorf("dir is required")
	}
	if strings.HasSuffix(dir, "/") {
		dir = strings.TrimSuffix(dir, "/")
		if dir == "" {
			return nil, fmt.Errorf("dir is required")
		}
	}

	rows, err := s.querySymbols(
		`SELECT s.id, s.file_id, s.name, s.kind, s.line_start, s.line_end,
		        COALESCE(s.signature,''), COALESCE(s.docstring,''), COALESCE(s.parent_name,''),
		        f.path, COALESCE(f.language,'')
		 FROM symbols s JOIN files f ON f.id = s.file_id
		 WHERE f.project_id = ?
		   AND (f.path = ? OR f.path LIKE ? || '/%')
		 ORDER BY f.path, s.line_start`,
		s.projectID, dir, dir,
	)
	if err != nil {
		return nil, err
	}

	files := map[string]struct{}{}
	out := &PackageOutline{Dir: dir, Entries: make([]PackageEntry, 0, len(rows))}
	for _, sym := range rows {
		files[sym.Path] = struct{}{}
		fanIn, err := s.FanInCount(sym)
		if err != nil {
			return nil, err
		}
		out.Entries = append(out.Entries, PackageEntry{Symbol: sym, FanIn: fanIn})
	}
	out.FileCount = len(files)
	return out, nil
}
