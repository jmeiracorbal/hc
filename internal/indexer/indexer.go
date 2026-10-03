package indexer

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/parsers"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
)

const binaryCheckBytes = 512

type Result struct {
	Indexed int
	Skipped int
	Errors  int
	Pruned  int
}

func IndexPath(start string, force bool) (Result, error) {
	root, id, dbPath, err := config.ResolveProject(start)
	if err != nil {
		return Result{}, err
	}

	st, err := openStore(id, dbPath)
	if err != nil {
		return Result{}, err
	}
	defer st.Close()

	gi := loadGitignore(root)
	var result Result
	seen := map[string]struct{}{}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			log.Printf("walk error %s: %v", path, walkErr)
			result.Errors++
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			name := d.Name()
			if rel != "." {
				if _, skip := config.AlwaysIgnore[name]; skip {
					return filepath.SkipDir
				}
				if strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if shouldSkipFile(rel, gi) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("cannot read %s: %v", path, err)
			result.Errors++
			return nil
		}
		if isBinary(data) {
			return nil
		}

		lang := parsers.DetectLanguage(path)
		if lang == "" {
			return nil
		}

		seen[rel] = struct{}{}
		sha := sha256Hex(data)
		existing, err := st.GetFile(rel)
		if err != nil {
			result.Errors++
			return nil
		}
		if !force && existing != nil && existing.SHA256 == sha {
			result.Skipped++
			return nil
		}

		fileID, err := st.UpsertFile(rel, sha, lang)
		if err != nil {
			result.Errors++
			return nil
		}
		if err := st.DeleteFileEdges(fileID); err != nil {
			result.Errors++
			return nil
		}
		if err := st.DeleteFileSymbols(fileID); err != nil {
			result.Errors++
			return nil
		}
		parsed := parsers.ParseFile(path, data)
		if err := st.InsertSymbols(fileID, parsed.Symbols); err != nil {
			result.Errors++
			return nil
		}
		edges, err := buildEdges(st, fileID, parsed.Refs)
		if err != nil {
			result.Errors++
			return nil
		}
		if err := st.InsertEdges(edges); err != nil {
			result.Errors++
			return nil
		}
		result.Indexed++
		return nil
	})
	if err != nil {
		return result, err
	}

	all, err := st.AllFiles()
	if err != nil {
		return result, err
	}
	for _, f := range all {
		if _, ok := seen[f.Path]; !ok {
			if err := st.DeleteFile(f.Path); err != nil {
				result.Errors++
				continue
			}
			result.Pruned++
		}
	}

	if err := st.ResolveAllEdges(); err != nil {
		return result, fmt.Errorf("resolve edges: %w", err)
	}
	return result, nil
}

func openStore(id, dbPath string) (*store.Store, error) {
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			path, err := config.EnsureSharedIndex()
			if err != nil {
				return nil, err
			}
			dbPath = path
		} else {
			return nil, err
		}
	}
	return store.OpenForProject(dbPath, id)
}

func RequireStore(start string) (*store.Store, error) {
	_, id, dbPath, err := config.ResolveProject(start)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("shared index not found at %s. Run: hc setup", dbPath)
		}
		return nil, err
	}
	return store.OpenForProject(dbPath, id)
}

func buildEdges(st *store.Store, fileID int64, refs []parsers.Ref) ([]store.EdgeInput, error) {
	syms, err := st.SymbolsByFile(fileID)
	if err != nil {
		return nil, err
	}
	byName := map[string][]store.SymbolRow{}
	for _, sym := range syms {
		if sym.Kind == "import" {
			continue
		}
		byName[sym.Name] = append(byName[sym.Name], sym)
	}

	resolveLocal := func(name string) sql.NullInt64 {
		if name == "" {
			return sql.NullInt64{}
		}
		cands := byName[name]
		if len(cands) == 1 {
			return sql.NullInt64{Int64: cands[0].ID, Valid: true}
		}
		return sql.NullInt64{}
	}

	var edges []store.EdgeInput
	for _, ref := range refs {
		if ref.ToName == "" || ref.Kind == "" {
			continue
		}
		e := store.EdgeInput{
			Kind:        ref.Kind,
			ToName:      ref.ToName,
			ToQualifier: ref.ToQualifier,
			FromFileID:  fileID,
			Line:        ref.Line,
		}
		switch ref.Kind {
		case parsers.RefCalls, parsers.RefContains:
			e.FromSymbolID = resolveLocal(ref.FromName)
			e.ToSymbolID = resolveLocal(ref.ToName)
		case parsers.RefImports:
		default:
			continue
		}
		edges = append(edges, e)
	}
	return edges, nil
}

func loadGitignore(root string) *ignore.GitIgnore {
	giPath := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(giPath); err != nil {
		return nil
	}
	gi, err := ignore.CompileIgnoreFile(giPath)
	if err != nil {
		log.Printf("gitignore: %v", err)
		return nil
	}
	return gi
}

func shouldSkipFile(rel string, gi *ignore.GitIgnore) bool {
	parts := strings.Split(rel, "/")
	for _, part := range parts[:len(parts)-1] {
		if _, ok := config.AlwaysIgnore[part]; ok {
			return true
		}
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	if gi != nil && gi.MatchesPath(rel) {
		return true
	}
	return false
}

func isBinary(data []byte) bool {
	n := binaryCheckBytes
	if len(data) < n {
		n = len(data)
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
