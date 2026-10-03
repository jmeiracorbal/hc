package doctor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/doctor"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
)

func TestLegacyIndexesDetected(t *testing.T) {
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
	_ = st.Close()

	legacy := filepath.Join(dataRoot, "indexes", "deadbeef")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, config.IndexFile), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := doctor.Run(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Legacy.Present {
		t.Fatal("legacy should be present")
	}
	if len(report.Legacy.IDs) != 1 || report.Legacy.IDs[0] != "deadbeef" {
		t.Fatalf("ids=%v", report.Legacy.IDs)
	}
	found := false
	for _, r := range report.RepairsAvailable {
		if r == doctor.RepairPurgeLegacyIndexes {
			found = true
		}
	}
	if !found {
		t.Fatalf("repairs=%v", report.RepairsAvailable)
	}
	if report.Healthy {
		t.Fatal("healthy should be false with legacy indexes")
	}

	// EnsureSharedIndex must not purge
	if _, err := config.EnsureSharedIndex(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(legacy, config.IndexFile)); err != nil {
		t.Fatal("EnsureSharedIndex must not purge legacy")
	}

	done, err := doctor.Fix(report, doctor.FixOpts{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	purged := false
	for _, d := range done {
		if d == doctor.RepairPurgeLegacyIndexes {
			purged = true
		}
	}
	if !purged {
		t.Fatalf("done=%v", done)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "indexes")); !os.IsNotExist(err) {
		t.Fatal("indexes/ should be purged")
	}
}
