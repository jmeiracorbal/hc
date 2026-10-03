package doctor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/doctor"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
)

func TestReportJSONFields(t *testing.T) {
	dataRoot := t.TempDir()
	config.SetDataRootForTest(dataRoot)
	config.SetCoreVersion("0.1.0-test")
	t.Cleanup(func() {
		config.SetDataRootForTest("")
		config.SetCoreVersion("")
	})

	dbPath, err := config.EnsureSharedIndex()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	cwd := t.TempDir()
	report, err := doctor.Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := doctor.FormatJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"binary", "schema", "db", "marker", "projects", "legacy", "repairs_available", "healthy"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing json field %q in %s", key, raw)
		}
	}
	binary, _ := m["binary"].(map[string]any)
	if binary["version"] != "0.1.0-test" {
		t.Fatalf("binary.version=%v", binary["version"])
	}
	schema, _ := m["schema"].(map[string]any)
	for _, key := range []string{"path", "have", "want", "pending"} {
		if _, ok := schema[key]; !ok {
			t.Fatalf("schema missing %q", key)
		}
	}
	db, _ := m["db"].(map[string]any)
	for _, key := range []string{"readable", "foreign_keys", "wal"} {
		if _, ok := db[key]; !ok {
			t.Fatalf("db missing %q", key)
		}
	}
	if !db["readable"].(bool) {
		t.Fatal("expected readable db")
	}
	if int(schema["have"].(float64)) != config.SchemaVersion {
		t.Fatalf("have=%v want %d", schema["have"], config.SchemaVersion)
	}
}

func TestFixSyncMarker(t *testing.T) {
	dataRoot := t.TempDir()
	config.SetDataRootForTest(dataRoot)
	config.SetCoreVersion("1.0.0")
	t.Cleanup(func() {
		config.SetDataRootForTest("")
		config.SetCoreVersion("")
	})

	dbPath, err := config.EnsureSharedIndex()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	proj := t.TempDir()
	canon, id, _, err := config.InitProject(proj)
	if err != nil {
		t.Fatal(err)
	}
	// simular marker de binario viejo
	old := []byte("{\n  \"version\": \"0.0.1\",\n  \"id\": \"" + id + "\"\n}\n")
	if err := os.WriteFile(filepath.Join(canon, config.MarkerFile), old, 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := doctor.Run(canon)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range report.RepairsAvailable {
		if r == doctor.RepairSyncMarkerVersion {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected sync_marker_version in %v", report.RepairsAvailable)
	}
	done, err := doctor.Fix(report, doctor.FixOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(done) == 0 {
		t.Fatal("no repairs applied")
	}
	m, err := config.ReadMarker(canon)
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "1.0.0" {
		t.Fatalf("marker version=%q", m.Version)
	}
}
