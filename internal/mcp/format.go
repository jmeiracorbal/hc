package mcp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmeiracorbal/hybrid-coco/internal/store"
)

const listCap = 20

func FormatSearch(query string, results []store.SymbolRow) string {
	if len(results) == 0 {
		return fmt.Sprintf("# hc_search(%q)\nNo results.", query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# hc_search(%q)\n", query)
	for _, r := range results {
		fmt.Fprintf(&b, "[%s:%d] %s %s\n", r.Path, r.LineStart, r.Kind, r.Name)
		if r.Signature != "" {
			fmt.Fprintf(&b, "  sig: %s\n", r.Signature)
		}
		if r.Docstring != "" {
			snippet := strings.ReplaceAll(r.Docstring, "\n", " ")
			if len(snippet) > 120 {
				snippet = snippet[:120]
			}
			fmt.Fprintf(&b, "  doc: %s\n", snippet)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

type SymbolEnrichment struct {
	FanIn   int
	Callers []store.EdgeLocus
	Callees []string
}

func FormatSymbol(name string, results []store.SymbolRow, enrich []SymbolEnrichment) string {
	if len(results) == 0 {
		return fmt.Sprintf("Symbol '%s' not found.", name)
	}
	var b strings.Builder
	for i, r := range results {
		parent := ""
		if r.ParentName != "" {
			parent = fmt.Sprintf(" (in %s)", r.ParentName)
		}
		fmt.Fprintf(&b, "%s %s%s @ %s:%d-%d\n", r.Kind, r.Name, parent, r.Path, r.LineStart, r.LineEnd)
		if r.Signature != "" {
			fmt.Fprintf(&b, "  sig: %s\n", r.Signature)
		}
		if r.Docstring != "" {
			snippet := strings.ReplaceAll(r.Docstring, "\n", " ")
			if len(snippet) > 120 {
				snippet = snippet[:120]
			}
			fmt.Fprintf(&b, "  doc: %s\n", snippet)
		}
		if i < len(enrich) {
			e := enrich[i]
			fmt.Fprintf(&b, "  called_by: %d\n", e.FanIn)
			if len(e.Callers) > 0 {
				b.WriteString("  callers:\n")
				writeLoci(&b, e.Callers, listCap)
			}
			if len(e.Callees) > 0 {
				fmt.Fprintf(&b, "  callees: %s\n", strings.Join(truncateNames(e.Callees, listCap), ", "))
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func FormatExplore(name string, data *store.ExploreResult) (string, error) {
	if data == nil {
		return fmt.Sprintf("Symbol '%s' not found.", name), nil
	}
	r := data.Symbol
	var b strings.Builder
	parent := ""
	if r.ParentName != "" {
		parent = fmt.Sprintf(" (in %s)", r.ParentName)
	}
	fmt.Fprintf(&b, "%s %s%s @ %s:%d-%d\n", r.Kind, r.Name, parent, r.Path, r.LineStart, r.LineEnd)
	hint, err := ReadRangeHint(r.LineStart, r.LineEnd)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "%s\n", hint)
	fmt.Fprintf(&b, "fan-in: %d  fan-out: %d\n", data.FanIn, data.FanOut)
	if data.Parent != "" {
		fmt.Fprintf(&b, "contained_in: %s\n", data.Parent)
	}
	if len(data.Children) > 0 {
		fmt.Fprintf(&b, "contains: %s\n", strings.Join(truncateNames(data.Children, listCap), ", "))
	}
	b.WriteString("callers:\n")
	if len(data.Callers) == 0 {
		b.WriteString("  (none)\n")
	} else {
		writeLoci(&b, data.Callers, listCap)
	}
	b.WriteString("callees:\n")
	if len(data.Callees) == 0 {
		b.WriteString("  (none)\n")
	} else {
		writeLoci(&b, data.Callees, listCap)
	}
	if len(data.Imports) > 0 {
		fmt.Fprintf(&b, "imports (file): %s\n", strings.Join(truncateNames(data.Imports, listCap), ", "))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func FormatImpact(name string, data *store.ImpactResult) string {
	if data == nil {
		return fmt.Sprintf("Symbol '%s' not found.", name)
	}
	r := data.Symbol
	var b strings.Builder
	fmt.Fprintf(&b, "impact for %s %s @ %s:%d\n", r.Kind, r.Name, r.Path, r.LineStart)
	fmt.Fprintf(&b, "direct callers (%d):\n", len(data.Callers))
	if len(data.Callers) == 0 {
		b.WriteString("  (none)\n")
	} else {
		writeLoci(&b, data.Callers, listCap)
	}
	fmt.Fprintf(&b, "file importers (%d):\n", len(data.Importers))
	if len(data.Importers) == 0 {
		b.WriteString("  (none)\n")
	} else {
		writeLoci(&b, data.Importers, listCap)
	}
	return strings.TrimRight(b.String(), "\n")
}

func FormatPath(from, to string, hops []store.PathHop) string {
	if hops == nil {
		return fmt.Sprintf("No call path from %s to %s (depth ≤ 6).", from, to)
	}
	if len(hops) == 0 {
		return fmt.Sprintf("No call path from %s to %s (depth ≤ 6).", from, to)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "path %s → %s (%d hops):\n", from, to, len(hops))
	for i, h := range hops {
		fmt.Fprintf(&b, "  %d. %s @ %s:%d → %s\n", i+1, h.From.Name, h.From.Path, h.Line, h.To)
	}
	return strings.TrimRight(b.String(), "\n")
}

func FormatFileContext(path string, data *store.FileContext) (string, error) {
	if data == nil {
		return fmt.Sprintf("File '%s' not found in index.", path), nil
	}
	var b strings.Builder
	if data.Language == "" {
		fmt.Fprintf(&b, "File: %s — %d symbols\n\n", path, len(data.Symbols))
	} else {
		fmt.Fprintf(&b, "File: %s (%s) — %d symbols\n\n", path, data.Language, len(data.Symbols))
	}
	b.WriteString("For symbol bodies: Read with offset+limit from hints below. Full-file Read is blocked by the hook.\n\n")

	byKind := map[string][]store.SymbolRow{}
	for _, sym := range data.Symbols {
		byKind[sym.Kind] = append(byKind[sym.Kind], sym)
	}

	kindOrder := []string{"class", "function", "method", "import"}
	seen := map[string]struct{}{}
	var ordered []string
	for _, k := range kindOrder {
		if _, ok := byKind[k]; ok {
			ordered = append(ordered, k)
			seen[k] = struct{}{}
		}
	}
	var extras []string
	for k := range byKind {
		if _, ok := seen[k]; !ok {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	ordered = append(ordered, extras...)

	plural := map[string]string{
		"class": "Classes", "function": "Functions", "method": "Methods", "import": "Imports",
	}
	for _, kind := range ordered {
		group := byKind[kind]
		label := plural[kind]
		if label == "" {
			label = strings.ToUpper(kind[:1]) + kind[1:] + "s"
		}
		fmt.Fprintf(&b, "%s (%d):\n", label, len(group))
		for _, sym := range group {
			if kind == "import" {
				fmt.Fprintf(&b, "  %s\n", sym.Name)
				continue
			}
			hint, err := ReadRangeHint(sym.LineStart, sym.LineEnd)
			if err != nil {
				return "", fmt.Errorf("symbol %s: %w", sym.Name, err)
			}
			if sym.Signature != "" {
				fmt.Fprintf(&b, "  %s @ %d-%d  %s  %s\n", sym.Name, sym.LineStart, sym.LineEnd, hint, sym.Signature)
			} else {
				fmt.Fprintf(&b, "  %s @ %d-%d  %s\n", sym.Name, sym.LineStart, sym.LineEnd, hint)
			}
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func FormatPackage(dir string, data *store.PackageOutline) (string, error) {
	if data == nil {
		return fmt.Sprintf("Package '%s' not found.", dir), nil
	}
	if len(data.Entries) == 0 {
		return fmt.Sprintf("Package '%s': 0 files, 0 symbols.", data.Dir), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Package: %s — %d files, %d symbols\n", data.Dir, data.FileCount, len(data.Entries))
	b.WriteString("Bodies: Read path with offset+limit from hints.\n\n")

	byFile := map[string][]store.PackageEntry{}
	var fileOrder []string
	for _, e := range data.Entries {
		p := e.Symbol.Path
		if _, ok := byFile[p]; !ok {
			fileOrder = append(fileOrder, p)
		}
		byFile[p] = append(byFile[p], e)
	}

	for _, path := range fileOrder {
		entries := byFile[path]
		fmt.Fprintf(&b, "%s (%d):\n", path, len(entries))
		for _, e := range entries {
			sym := e.Symbol
			if sym.Kind == "import" {
				fmt.Fprintf(&b, "  import %s\n", sym.Name)
				continue
			}
			hint, err := ReadRangeHint(sym.LineStart, sym.LineEnd)
			if err != nil {
				return "", fmt.Errorf("symbol %s: %w", sym.Name, err)
			}
			fmt.Fprintf(&b, "  %s %s @ %d-%d  fan-in:%d  %s\n",
				sym.Kind, sym.Name, sym.LineStart, sym.LineEnd, e.FanIn, hint)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func FormatStatus(stats store.Stats, dbPath string) string {
	plural := map[string]string{"class": "classes"}
	var kindParts []string
	type kv struct {
		k string
		n int
	}
	var items []kv
	for k, n := range stats.ByKind {
		items = append(items, kv{k, n})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].n > items[j].n })
	for _, it := range items {
		label := plural[it.k]
		if label == "" {
			label = it.k + "s"
		}
		kindParts = append(kindParts, fmt.Sprintf("%d %s", it.n, label))
	}
	updated := "never"
	if stats.LastIndexed > 0 {
		updated = time.Unix(stats.LastIndexed, 0).Format("2006-01-02 15:04")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Index: %s\nFiles:   %d indexed\nSymbols: %d (%s)\nEdges:   %d\nCalls:   %d resolved / %d total",
		dbPath, stats.Files, stats.Symbols, strings.Join(kindParts, ", "), stats.Edges,
		stats.CallsResolved, stats.CallsTotal)
	showLang := len(stats.CallsByLanguage) > 1
	if !showLang {
		for _, lc := range stats.CallsByLanguage {
			if lc.Total-lc.Resolved > 0 {
				showLang = true
				break
			}
		}
	}
	if showLang {
		b.WriteString("\nCalls by language:")
		for _, lc := range stats.CallsByLanguage {
			lang := lc.Language
			if lang == "" {
				lang = "?"
			}
			fmt.Fprintf(&b, "\n  %s: %d/%d resolved", lang, lc.Resolved, lc.Total)
		}
	}
	fmt.Fprintf(&b, "\nUpdated: %s", updated)
	if len(stats.Hotspots) > 0 {
		b.WriteString("\nHotspots (fan-in):")
		for _, h := range stats.Hotspots {
			loc := h.Name
			if h.Path != "" {
				loc = fmt.Sprintf("%s @ %s", h.Name, h.Path)
			}
			fmt.Fprintf(&b, "\n  %s — %d", loc, h.FanIn)
		}
	}
	return b.String()
}

func RelDB(root, db string) string {
	rel, err := filepath.Rel(root, db)
	if err != nil {
		return db
	}
	return rel
}

func EnrichSymbols(st *store.Store, results []store.SymbolRow) ([]SymbolEnrichment, error) {
	out := make([]SymbolEnrichment, len(results))
	for i, r := range results {
		fanIn, err := st.FanInCount(r)
		if err != nil {
			return nil, err
		}
		callers, err := st.TopCallers(r, 5)
		if err != nil {
			return nil, err
		}
		callees, err := st.CalleeNames(r.ID, 10)
		if err != nil {
			return nil, err
		}
		out[i] = SymbolEnrichment{FanIn: fanIn, Callers: callers, Callees: callees}
	}
	return out, nil
}

func writeLoci(b *strings.Builder, loci []store.EdgeLocus, lim int) {
	n := len(loci)
	more := 0
	if n > lim {
		more = n - lim
		n = lim
	}
	for i := 0; i < n; i++ {
		e := loci[i]
		name := e.SymbolName
		if name == "" {
			name = "?"
		}
		fmt.Fprintf(b, "  - %s @ %s:%d\n", name, e.Path, e.Line)
	}
	if more > 0 {
		fmt.Fprintf(b, "  … +%d more\n", more)
	}
}

func truncateNames(names []string, lim int) []string {
	if len(names) <= lim {
		return names
	}
	out := append([]string{}, names[:lim]...)
	out = append(out, fmt.Sprintf("… +%d more", len(names)-lim))
	return out
}
