package indexer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/indexer"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
)

const samplePython = `class Greeter:
    '''A simple greeter.'''
    def greet(self, name: str) -> str:
        '''Return a greeting string.'''
        return f"Hello, {name}!"

def standalone(x: int) -> int:
    '''Double the value.'''
    return x * 2
`

const helperPython = `def normalize(s: str) -> str:
    return s.strip().lower()

def store_city(name: str) -> str:
    return normalize(name)
`

const callerPython = `from helper import store_city, normalize

def handle_checkout(city: str) -> str:
    return store_city(city)

def sync_cities(cities: list) -> list:
    out = []
    for c in cities:
        out.append(store_city(c))
    return out
`

func withDataRoot(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	config.SetDataRootForTest(dir)
	config.SetCoreVersion("test")
	t.Cleanup(func() {
		config.SetDataRootForTest("")
		config.SetCoreVersion("")
	})
}

func initProject(t *testing.T, dir string) string {
	t.Helper()
	withDataRoot(t)
	canon, id, dbPath, err := config.InitProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnrollProject(id, canon); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	return canon
}

func fixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sample.py"), []byte(samplePython), 0o644); err != nil {
		t.Fatal(err)
	}
	return initProject(t, dir)
}

func graphFixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "helper.py"), []byte(helperPython), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "caller.py"), []byte(callerPython), 0o644); err != nil {
		t.Fatal(err)
	}
	return initProject(t, dir)
}

func openIndexed(t *testing.T, dir string) *store.Store {
	t.Helper()
	_, id, dbPath, err := config.ResolveProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.OpenForProject(dbPath, id)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestIndexCreatesSymbols(t *testing.T) {
	dir := fixtureDir(t)
	result, err := indexer.IndexPath(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Indexed != 1 {
		t.Fatalf("indexed=%d want 1", result.Indexed)
	}
	if result.Errors != 0 {
		t.Fatalf("errors=%d", result.Errors)
	}

	st := openIndexed(t, dir)
	defer st.Close()

	stats, err := st.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 1 || stats.Symbols == 0 {
		t.Fatalf("stats=%+v", stats)
	}

	syms, err := st.LookupSymbol("Greeter")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) == 0 || syms[0].Kind != "class" {
		t.Fatalf("Greeter=%+v", syms)
	}

	syms2, err := st.LookupSymbol("standalone")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms2) == 0 || syms2[0].Kind != "function" {
		t.Fatalf("standalone=%+v", syms2)
	}
}

func TestIncrementalNoChange(t *testing.T) {
	dir := fixtureDir(t)
	r1, err := indexer.IndexPath(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Indexed != 1 {
		t.Fatalf("r1.indexed=%d", r1.Indexed)
	}
	r2, err := indexer.IndexPath(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Indexed != 0 || r2.Skipped != 1 {
		t.Fatalf("r2=%+v", r2)
	}
}

func TestIncrementalChange(t *testing.T) {
	dir := fixtureDir(t)
	if _, err := indexer.IndexPath(dir, false); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "src", "sample.py")
	if err := os.WriteFile(f, []byte(samplePython+"\ndef extra(): pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r2, err := indexer.IndexPath(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Indexed != 1 || r2.Skipped != 0 {
		t.Fatalf("r2=%+v", r2)
	}
}

func TestFTSSearch(t *testing.T) {
	dir := fixtureDir(t)
	if _, err := indexer.IndexPath(dir, false); err != nil {
		t.Fatal(err)
	}
	st := openIndexed(t, dir)
	defer st.Close()
	results, err := st.FTSSearch("Greeter", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected FTS hits for Greeter")
	}
}

func TestPruneDeleted(t *testing.T) {
	dir := fixtureDir(t)
	if _, err := indexer.IndexPath(dir, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "src", "sample.py")); err != nil {
		t.Fatal(err)
	}
	r, err := indexer.IndexPath(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Pruned != 1 {
		t.Fatalf("pruned=%d want 1", r.Pruned)
	}
	st := openIndexed(t, dir)
	defer st.Close()
	stats, _ := st.Stats()
	if stats.Files != 0 {
		t.Fatalf("files=%d after prune", stats.Files)
	}
}

func TestCallGraphExplore(t *testing.T) {
	dir := graphFixtureDir(t)
	r, err := indexer.IndexPath(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Indexed != 2 {
		t.Fatalf("indexed=%d want 2", r.Indexed)
	}
	if r.Errors != 0 {
		t.Fatalf("errors=%d", r.Errors)
	}

	st := openIndexed(t, dir)
	defer st.Close()

	stats, err := st.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Edges == 0 {
		t.Fatal("expected edges > 0")
	}

	ex, err := st.Explore("store_city")
	if err != nil {
		t.Fatal(err)
	}
	if ex == nil {
		t.Fatal("store_city not found")
	}
	if ex.FanIn < 2 {
		t.Fatalf("fan-in=%d want >= 2 callers", ex.FanIn)
	}
	if ex.FanOut < 1 {
		t.Fatalf("fan-out=%d want >= 1 (normalize)", ex.FanOut)
	}
	foundNormalize := false
	for _, c := range ex.Callees {
		if c.SymbolName == "normalize" {
			foundNormalize = true
		}
	}
	if !foundNormalize {
		t.Fatalf("expected callee normalize, got %+v", ex.Callees)
	}
	callerNames := map[string]bool{}
	for _, c := range ex.Callers {
		callerNames[c.SymbolName] = true
	}
	if !callerNames["handle_checkout"] || !callerNames["sync_cities"] {
		t.Fatalf("callers=%v want handle_checkout and sync_cities", callerNames)
	}

	path, err := st.Path("handle_checkout", "normalize")
	if err != nil {
		t.Fatal(err)
	}
	if len(path) == 0 {
		t.Fatal("expected path handle_checkout → normalize")
	}
	last := path[len(path)-1]
	if last.To != "normalize" {
		t.Fatalf("path end=%q want normalize; hops=%+v", last.To, path)
	}

	impact, err := st.Impact("store_city")
	if err != nil {
		t.Fatal(err)
	}
	if impact == nil || len(impact.Callers) < 2 {
		t.Fatalf("impact callers=%+v", impact)
	}
}

func TestGoParserIndexed(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	code := `package pkg

import "fmt"

func Helper() string {
	return "x"
}

func Run() {
	fmt.Println(Helper())
}
`
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	dir = initProject(t, dir)
	r, err := indexer.IndexPath(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Indexed != 1 || r.Errors != 0 {
		t.Fatalf("result=%+v", r)
	}
	st := openIndexed(t, dir)
	defer st.Close()
	ex, err := st.Explore("Run")
	if err != nil {
		t.Fatal(err)
	}
	if ex == nil {
		t.Fatal("Run not found")
	}
	found := false
	for _, c := range ex.Callees {
		if c.SymbolName == "Helper" || strings.Contains(c.SymbolName, "Println") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Helper or Println in callees: %+v", ex.Callees)
	}
	helper, err := st.Explore("Helper")
	if err != nil {
		t.Fatal(err)
	}
	if helper == nil || helper.FanIn < 1 {
		t.Fatalf("Helper fan-in=%v", helper)
	}
}

func TestIndexRequiresMarker(t *testing.T) {
	withDataRoot(t)
	dir := t.TempDir()
	_, err := indexer.IndexPath(dir, false)
	if err == nil {
		t.Fatal("expected error without marker")
	}
}

func TestPackageOutline(t *testing.T) {
	dir := graphFixtureDir(t)
	if _, err := indexer.IndexPath(dir, false); err != nil {
		t.Fatal(err)
	}
	st := openIndexed(t, dir)
	defer st.Close()

	if _, err := st.PackageOutline(""); err == nil {
		t.Fatal("empty dir must error")
	}
	if _, err := st.PackageOutline("/"); err == nil {
		t.Fatal("slash-only dir must error")
	}

	out, err := st.PackageOutline("src")
	if err != nil {
		t.Fatal(err)
	}
	if out.FileCount != 2 {
		t.Fatalf("files=%d want 2", out.FileCount)
	}
	if len(out.Entries) == 0 {
		t.Fatal("expected symbols under src")
	}
	var storeCity *store.PackageEntry
	for i := range out.Entries {
		if out.Entries[i].Symbol.Name == "store_city" {
			storeCity = &out.Entries[i]
			break
		}
	}
	if storeCity == nil {
		t.Fatal("store_city missing from package outline")
	}
	if storeCity.FanIn < 2 {
		t.Fatalf("store_city fan-in=%d want >= 2", storeCity.FanIn)
	}
}

func TestPackageOutlineEmptyDir(t *testing.T) {
	dir := graphFixtureDir(t)
	if _, err := indexer.IndexPath(dir, false); err != nil {
		t.Fatal(err)
	}
	st := openIndexed(t, dir)
	defer st.Close()
	out, err := st.PackageOutline("does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if out.FileCount != 0 || len(out.Entries) != 0 {
		t.Fatalf("expected empty outline, got %+v", out)
	}
}

func TestModuleLevelCallFanIn(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	code := `normalize("x")

def normalize(s: str) -> str:
    return s.strip()
`
	if err := os.WriteFile(filepath.Join(src, "mod.py"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	dir = initProject(t, dir)
	if _, err := indexer.IndexPath(dir, false); err != nil {
		t.Fatal(err)
	}
	st := openIndexed(t, dir)
	defer st.Close()

	ex, err := st.Explore("normalize")
	if err != nil {
		t.Fatal(err)
	}
	if ex == nil {
		t.Fatal("normalize not found")
	}
	if ex.FanIn < 1 {
		t.Fatalf("fan-in=%d want >= 1 (module-level call)", ex.FanIn)
	}
	foundModule := false
	for _, c := range ex.Callers {
		if c.SymbolName == "" || c.SymbolID == 0 {
			foundModule = true
			break
		}
	}
	if !foundModule {
		t.Fatalf("expected module-level caller (empty from_symbol), got %+v", ex.Callers)
	}
}

func TestGraphFixtureCallStats(t *testing.T) {
	dir := graphFixtureDir(t)
	if _, err := indexer.IndexPath(dir, false); err != nil {
		t.Fatal(err)
	}
	st := openIndexed(t, dir)
	defer st.Close()

	stats, err := st.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.CallsResolved <= 0 {
		t.Fatalf("CallsResolved=%d want > 0", stats.CallsResolved)
	}
	if stats.CallsResolved > stats.CallsTotal {
		t.Fatalf("CallsResolved=%d > CallsTotal=%d", stats.CallsResolved, stats.CallsTotal)
	}
	if len(stats.CallsByLanguage) == 0 {
		t.Fatal("CallsByLanguage empty")
	}
	foundPy := false
	for _, lc := range stats.CallsByLanguage {
		if lc.Language == "python" && lc.Total > 0 {
			foundPy = true
			if lc.Resolved > lc.Total {
				t.Fatalf("python resolved=%d > total=%d", lc.Resolved, lc.Total)
			}
		}
	}
	if !foundPy {
		t.Fatalf("want python in CallsByLanguage: %+v", stats.CallsByLanguage)
	}
}

func TestIndex_QualifierResolvesSelfCall(t *testing.T) {
	withDataRoot(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	code := `class A:
    def run(self):
        return 1
    def m(self):
        return self.run()

class B:
    def run(self):
        return 2
`
	if err := os.WriteFile(filepath.Join(src, "ab.py"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	dir = initProject(t, dir)
	if _, err := indexer.IndexPath(dir, false); err != nil {
		t.Fatal(err)
	}
	st := openIndexed(t, dir)
	defer st.Close()

	ex, err := st.Explore("m")
	if err != nil {
		t.Fatal(err)
	}
	if ex == nil {
		t.Fatal("Explore(m) nil")
	}
	if len(ex.Callees) != 1 {
		t.Fatalf("callees=%+v want 1", ex.Callees)
	}
	cal := ex.Callees[0]
	if cal.SymbolName != "run" {
		t.Fatalf("callee name=%q want run", cal.SymbolName)
	}
	if cal.SymbolID == 0 {
		t.Fatal("callee unresolved; want A's run via to_qualifier")
	}
	// confirm target is A's run, not B's
	syms, err := st.LookupSymbol("run")
	if err != nil {
		t.Fatal(err)
	}
	var runAID, runBID int64
	for _, s := range syms {
		switch s.ParentName {
		case "A":
			runAID = s.ID
		case "B":
			runBID = s.ID
		}
	}
	if runAID == 0 || runBID == 0 {
		t.Fatalf("want both A.run and B.run: %+v", syms)
	}
	if cal.SymbolID != runAID {
		t.Fatalf("resolved to %d; want A's run=%d (not B=%d)", cal.SymbolID, runAID, runBID)
	}
}
