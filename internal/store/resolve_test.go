package store

import (
	"database/sql"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/parsers"
)

func TestResolveAllEdges_SameFilePreference(t *testing.T) {
	st := openTestStore(t)

	fileA, err := st.UpsertFile("a.py", "sha-a", "python")
	if err != nil {
		t.Fatal(err)
	}
	fileB, err := st.UpsertFile("b.py", "sha-b", "python")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(fileA, []parsers.Symbol{
		{Name: "caller", Kind: "function", LineStart: 1, LineEnd: 3},
		{Name: "normalize", Kind: "function", LineStart: 5, LineEnd: 6},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(fileB, []parsers.Symbol{
		{Name: "normalize", Kind: "function", LineStart: 1, LineEnd: 2},
	}); err != nil {
		t.Fatal(err)
	}

	symsA, err := st.SymbolsByFile(fileA)
	if err != nil {
		t.Fatal(err)
	}
	var callerID, normalizeAID int64
	for _, s := range symsA {
		switch s.Name {
		case "caller":
			callerID = s.ID
		case "normalize":
			normalizeAID = s.ID
		}
	}
	if callerID == 0 || normalizeAID == 0 {
		t.Fatalf("missing symbols in A: %+v", symsA)
	}

	if err := st.InsertEdges([]EdgeInput{{
		Kind:         EdgeCalls,
		FromSymbolID: sql.NullInt64{Int64: callerID, Valid: true},
		ToName:       "normalize",
		FromFileID:   fileA,
		Line:         2,
	}}); err != nil {
		t.Fatal(err)
	}

	if err := st.ResolveAllEdges(); err != nil {
		t.Fatal(err)
	}

	var toID sql.NullInt64
	err = st.db.QueryRow(
		`SELECT to_symbol_id FROM edges WHERE kind = ? AND from_file_id = ? AND to_name = ?`,
		EdgeCalls, fileA, "normalize",
	).Scan(&toID)
	if err != nil {
		t.Fatal(err)
	}
	if !toID.Valid {
		t.Fatal("to_symbol_id NULL; want A's normalize")
	}
	if toID.Int64 != normalizeAID {
		t.Fatalf("to_symbol_id=%d want A's normalize=%d", toID.Int64, normalizeAID)
	}
}

func TestResolveAllEdges_AmbiguousProjectWide(t *testing.T) {
	st := openTestStore(t)

	fileA, err := st.UpsertFile("a.py", "sha-a", "python")
	if err != nil {
		t.Fatal(err)
	}
	fileB, err := st.UpsertFile("b.py", "sha-b", "python")
	if err != nil {
		t.Fatal(err)
	}
	fileC, err := st.UpsertFile("c.py", "sha-c", "python")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(fileA, []parsers.Symbol{
		{Name: "foo", Kind: "function", LineStart: 1, LineEnd: 2},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(fileB, []parsers.Symbol{
		{Name: "foo", Kind: "function", LineStart: 1, LineEnd: 2},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(fileC, []parsers.Symbol{
		{Name: "caller", Kind: "function", LineStart: 1, LineEnd: 3},
	}); err != nil {
		t.Fatal(err)
	}

	symsC, err := st.SymbolsByFile(fileC)
	if err != nil || len(symsC) != 1 {
		t.Fatalf("symsC=%v err=%v", symsC, err)
	}

	if err := st.InsertEdges([]EdgeInput{{
		Kind:         EdgeCalls,
		FromSymbolID: sql.NullInt64{Int64: symsC[0].ID, Valid: true},
		ToName:       "foo",
		FromFileID:   fileC,
		Line:         2,
	}}); err != nil {
		t.Fatal(err)
	}

	if err := st.ResolveAllEdges(); err != nil {
		t.Fatal(err)
	}

	var toID sql.NullInt64
	err = st.db.QueryRow(
		`SELECT to_symbol_id FROM edges WHERE kind = ? AND from_file_id = ? AND to_name = ?`,
		EdgeCalls, fileC, "foo",
	).Scan(&toID)
	if err != nil {
		t.Fatal(err)
	}
	if toID.Valid {
		t.Fatalf("to_symbol_id=%d; want NULL (ambiguous project-wide)", toID.Int64)
	}
}

func TestResolveAllEdges_QualifierDisambiguates(t *testing.T) {
	st := openTestStore(t)

	fileID, err := st.UpsertFile("ab.py", "sha-ab", "python")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(fileID, []parsers.Symbol{
		{Name: "A", Kind: "class", LineStart: 1, LineEnd: 8},
		{Name: "run", Kind: "method", LineStart: 2, LineEnd: 3, ParentName: "A"},
		{Name: "m", Kind: "method", LineStart: 4, LineEnd: 6, ParentName: "A"},
		{Name: "B", Kind: "class", LineStart: 9, LineEnd: 12},
		{Name: "run", Kind: "method", LineStart: 10, LineEnd: 11, ParentName: "B"},
	}); err != nil {
		t.Fatal(err)
	}

	syms, err := st.SymbolsByFile(fileID)
	if err != nil {
		t.Fatal(err)
	}
	var mID, runAID, runBID int64
	for _, s := range syms {
		switch {
		case s.Name == "m" && s.ParentName == "A":
			mID = s.ID
		case s.Name == "run" && s.ParentName == "A":
			runAID = s.ID
		case s.Name == "run" && s.ParentName == "B":
			runBID = s.ID
		}
	}
	if mID == 0 || runAID == 0 || runBID == 0 {
		t.Fatalf("missing symbols: %+v", syms)
	}

	if err := st.InsertEdges([]EdgeInput{{
		Kind:         EdgeCalls,
		FromSymbolID: sql.NullInt64{Int64: mID, Valid: true},
		ToName:       "run",
		ToQualifier:  "A",
		FromFileID:   fileID,
		Line:         5,
	}}); err != nil {
		t.Fatal(err)
	}

	if err := st.ResolveAllEdges(); err != nil {
		t.Fatal(err)
	}

	var toID sql.NullInt64
	err = st.db.QueryRow(
		`SELECT to_symbol_id FROM edges WHERE kind = ? AND from_file_id = ? AND to_name = ?`,
		EdgeCalls, fileID, "run",
	).Scan(&toID)
	if err != nil {
		t.Fatal(err)
	}
	if !toID.Valid {
		t.Fatal("to_symbol_id NULL; want A's run")
	}
	if toID.Int64 != runAID {
		t.Fatalf("to_symbol_id=%d want A's run=%d (not B=%d)", toID.Int64, runAID, runBID)
	}
}

func TestResolveAllEdges_AmbiguousBareNameNULL(t *testing.T) {
	st := openTestStore(t)

	fileID, err := st.UpsertFile("ab.py", "sha-ab", "python")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(fileID, []parsers.Symbol{
		{Name: "A", Kind: "class", LineStart: 1, LineEnd: 4},
		{Name: "run", Kind: "method", LineStart: 2, LineEnd: 3, ParentName: "A"},
		{Name: "B", Kind: "class", LineStart: 5, LineEnd: 8},
		{Name: "run", Kind: "method", LineStart: 6, LineEnd: 7, ParentName: "B"},
		{Name: "caller", Kind: "function", LineStart: 9, LineEnd: 11},
	}); err != nil {
		t.Fatal(err)
	}

	syms, err := st.SymbolsByFile(fileID)
	if err != nil {
		t.Fatal(err)
	}
	var callerID int64
	for _, s := range syms {
		if s.Name == "caller" {
			callerID = s.ID
		}
	}
	if callerID == 0 {
		t.Fatalf("missing caller: %+v", syms)
	}

	if err := st.InsertEdges([]EdgeInput{{
		Kind:         EdgeCalls,
		FromSymbolID: sql.NullInt64{Int64: callerID, Valid: true},
		ToName:       "run",
		FromFileID:   fileID,
		Line:         10,
	}}); err != nil {
		t.Fatal(err)
	}

	if err := st.ResolveAllEdges(); err != nil {
		t.Fatal(err)
	}

	var toID sql.NullInt64
	err = st.db.QueryRow(
		`SELECT to_symbol_id FROM edges WHERE kind = ? AND from_file_id = ? AND to_name = ?`,
		EdgeCalls, fileID, "run",
	).Scan(&toID)
	if err != nil {
		t.Fatal(err)
	}
	if toID.Valid {
		t.Fatalf("to_symbol_id=%d; want NULL (ambiguous bare name)", toID.Int64)
	}
}

func TestStats_CallsByLanguage(t *testing.T) {
	st := openTestStore(t)

	pyFile, err := st.UpsertFile("a.py", "sha-py", "python")
	if err != nil {
		t.Fatal(err)
	}
	goFile, err := st.UpsertFile("a.go", "sha-go", "go")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(pyFile, []parsers.Symbol{
		{Name: "py_fn", Kind: "function", LineStart: 1, LineEnd: 2},
		{Name: "target", Kind: "function", LineStart: 3, LineEnd: 4},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSymbols(goFile, []parsers.Symbol{
		{Name: "GoFn", Kind: "function", LineStart: 1, LineEnd: 2},
	}); err != nil {
		t.Fatal(err)
	}
	symsPy, err := st.SymbolsByFile(pyFile)
	if err != nil {
		t.Fatal(err)
	}
	var pyCaller, targetID int64
	for _, s := range symsPy {
		switch s.Name {
		case "py_fn":
			pyCaller = s.ID
		case "target":
			targetID = s.ID
		}
	}
	symsGo, err := st.SymbolsByFile(goFile)
	if err != nil || len(symsGo) != 1 {
		t.Fatalf("go syms=%v err=%v", symsGo, err)
	}

	if err := st.InsertEdges([]EdgeInput{
		{
			Kind: EdgeCalls, FromSymbolID: sql.NullInt64{Int64: pyCaller, Valid: true},
			ToName: "target", FromFileID: pyFile, Line: 1,
		},
		{
			Kind: EdgeCalls, FromSymbolID: sql.NullInt64{Int64: pyCaller, Valid: true},
			ToName: "missing", FromFileID: pyFile, Line: 2,
		},
		{
			Kind: EdgeCalls, FromSymbolID: sql.NullInt64{Int64: symsGo[0].ID, Valid: true},
			ToName: "nowhere", FromFileID: goFile, Line: 1,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.ResolveAllEdges(); err != nil {
		t.Fatal(err)
	}

	stats, err := st.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.CallsByLanguage) == 0 {
		t.Fatal("CallsByLanguage empty")
	}
	byLang := map[string]LangCallStats{}
	for _, lc := range stats.CallsByLanguage {
		byLang[lc.Language] = lc
	}
	py, ok := byLang["python"]
	if !ok {
		t.Fatalf("missing python stats: %+v", stats.CallsByLanguage)
	}
	if py.Total != 2 || py.Resolved != 1 {
		t.Fatalf("python stats=%+v want total=2 resolved=1 (target id=%d)", py, targetID)
	}
	goStats, ok := byLang["go"]
	if !ok {
		t.Fatalf("missing go stats: %+v", stats.CallsByLanguage)
	}
	if goStats.Total != 1 || goStats.Resolved != 0 {
		t.Fatalf("go stats=%+v want total=1 resolved=0", goStats)
	}
	// unresolved desc: go (1) before python (1) → tie → language asc: go < python
	if stats.CallsByLanguage[0].Language != "go" {
		t.Fatalf("sort want go first (unresolved tie → lang asc), got %+v", stats.CallsByLanguage)
	}
}
