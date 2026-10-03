package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/parsers"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.db")
	admin, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	id := "proj-test"
	root := t.TempDir()
	if err := admin.EnrollProject(id, root); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	if err := admin.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := OpenForProject(path, id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestOpen_ForeignKeysEnabled(t *testing.T) {
	st := openTestStore(t)
	var on int
	if err := st.db.QueryRow(`PRAGMA foreign_keys`).Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != 1 {
		t.Fatalf("foreign_keys=%d want 1", on)
	}
}

func TestFK_RejectOrphanSymbol(t *testing.T) {
	st := openTestStore(t)
	err := st.InsertSymbols(999, []parsers.Symbol{{
		Name: "ghost", Kind: "function", LineStart: 1, LineEnd: 1,
	}})
	if err == nil {
		t.Fatal("expected FK violation for orphan symbol")
	}
}

func TestFK_RejectOrphanEdge(t *testing.T) {
	st := openTestStore(t)
	err := st.InsertEdges([]EdgeInput{{
		Kind:       EdgeCalls,
		ToName:     "x",
		FromFileID: 999,
		Line:       1,
	}})
	if err == nil {
		t.Fatal("expected FK violation for orphan edge")
	}
}

func TestFK_DeleteFileCascades(t *testing.T) {
	st := openTestStore(t)
	fileID, err := st.UpsertFile("a.py", "sha-a", "python")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(fileID, []parsers.Symbol{
		{Name: "foo", Kind: "function", LineStart: 1, LineEnd: 2},
	}); err != nil {
		t.Fatal(err)
	}
	syms, err := st.SymbolsByFile(fileID)
	if err != nil || len(syms) != 1 {
		t.Fatalf("syms=%v err=%v", syms, err)
	}
	if err := st.InsertEdges([]EdgeInput{{
		Kind:         EdgeCalls,
		FromSymbolID: sql.NullInt64{Int64: syms[0].ID, Valid: true},
		ToName:       "bar",
		FromFileID:   fileID,
		Line:         2,
	}}); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteFile("a.py"); err != nil {
		t.Fatal(err)
	}

	var nFiles, nSyms, nEdges, nFTS int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&nFiles)
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM symbols`).Scan(&nSyms)
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&nEdges)
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM symbols_fts`).Scan(&nFTS)
	if nFiles != 0 || nSyms != 0 || nEdges != 0 || nFTS != 0 {
		t.Fatalf("after cascade files=%d symbols=%d edges=%d fts=%d", nFiles, nSyms, nEdges, nFTS)
	}
}

func TestFK_DeleteCalleeSetsNull(t *testing.T) {
	st := openTestStore(t)
	callerFile, err := st.UpsertFile("caller.py", "sha-c", "python")
	if err != nil {
		t.Fatal(err)
	}
	calleeFile, err := st.UpsertFile("callee.py", "sha-t", "python")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(callerFile, []parsers.Symbol{
		{Name: "caller", Kind: "function", LineStart: 1, LineEnd: 3},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(calleeFile, []parsers.Symbol{
		{Name: "target", Kind: "function", LineStart: 1, LineEnd: 2},
	}); err != nil {
		t.Fatal(err)
	}
	callers, _ := st.SymbolsByFile(callerFile)
	callees, _ := st.SymbolsByFile(calleeFile)
	if err := st.InsertEdges([]EdgeInput{{
		Kind:         EdgeCalls,
		FromSymbolID: sql.NullInt64{Int64: callers[0].ID, Valid: true},
		ToSymbolID:   sql.NullInt64{Int64: callees[0].ID, Valid: true},
		ToName:       "target",
		FromFileID:   callerFile,
		Line:         2,
	}}); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteFileSymbols(calleeFile); err != nil {
		t.Fatal(err)
	}

	var toID sql.NullInt64
	var toName string
	err = st.db.QueryRow(`SELECT to_symbol_id, to_name FROM edges WHERE kind = ?`, EdgeCalls).
		Scan(&toID, &toName)
	if err != nil {
		t.Fatal(err)
	}
	if toID.Valid {
		t.Fatalf("to_symbol_id still set: %v", toID.Int64)
	}
	if toName != "target" {
		t.Fatalf("to_name=%q want target", toName)
	}
}

func TestFK_DeleteSymbolCleansFTS(t *testing.T) {
	st := openTestStore(t)
	fileID, err := st.UpsertFile("b.py", "sha-b", "python")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(fileID, []parsers.Symbol{
		{Name: "keepme", Kind: "function", LineStart: 1, LineEnd: 1},
	}); err != nil {
		t.Fatal(err)
	}
	var ftsBefore int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM symbols_fts`).Scan(&ftsBefore)
	if ftsBefore != 1 {
		t.Fatalf("fts before=%d want 1", ftsBefore)
	}

	if err := st.DeleteFileSymbols(fileID); err != nil {
		t.Fatal(err)
	}
	var ftsAfter int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM symbols_fts`).Scan(&ftsAfter)
	if ftsAfter != 0 {
		t.Fatalf("fts after=%d want 0", ftsAfter)
	}
}

func TestOpenForProject_NotEnrolled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	admin, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = OpenForProject(path, "missing")
	if !errors.Is(err, ErrProjectNotEnrolled) {
		t.Fatalf("err=%v want ErrProjectNotEnrolled", err)
	}
}

func TestTwoProjectIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	admin, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.EnrollProject("p1", "/root/p1"); err != nil {
		t.Fatal(err)
	}
	if err := admin.EnrollProject("p2", "/root/p2"); err != nil {
		t.Fatal(err)
	}
	if err := admin.Close(); err != nil {
		t.Fatal(err)
	}

	s1, err := OpenForProject(path, "p1")
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	s2, err := OpenForProject(path, "p2")
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	if _, err := s1.UpsertFile("a.py", "sha-1", "python"); err != nil {
		t.Fatal(err)
	}
	f2, err := s2.UpsertFile("b.py", "sha-2", "python")
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.InsertSymbols(f2, []parsers.Symbol{
		{Name: "only_p2", Kind: "function", LineStart: 1, LineEnd: 1},
	}); err != nil {
		t.Fatal(err)
	}

	st1, err := s1.Stats()
	if err != nil {
		t.Fatal(err)
	}
	st2, err := s2.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st1.Files != 1 || st1.Symbols != 0 {
		t.Fatalf("p1 stats=%+v", st1)
	}
	if st2.Files != 1 || st2.Symbols != 1 {
		t.Fatalf("p2 stats=%+v", st2)
	}

	hit, err := s1.LookupSymbol("only_p2")
	if err != nil {
		t.Fatal(err)
	}
	if len(hit) != 0 {
		t.Fatalf("p1 must not see p2 symbols: %+v", hit)
	}
	hit2, err := s2.LookupSymbol("only_p2")
	if err != nil {
		t.Fatal(err)
	}
	if len(hit2) != 1 {
		t.Fatalf("p2 lookup=%+v", hit2)
	}
}
