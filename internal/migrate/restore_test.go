package migrate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/migrate"
)

func TestRestoreBackupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.db")
	db := openRaw(t, path)
	if _, err := migrate.ApplyPending(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO projects (id, root, enrolled_at) VALUES ('p1', '/tmp/p1', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	bak, err := migrate.Backup(path)
	if err != nil {
		t.Fatal(err)
	}

	db2 := openRaw(t, path)
	if _, err := db2.Exec(`INSERT INTO projects (id, root, enrolled_at) VALUES ('p2', '/tmp/p2', 2)`); err != nil {
		t.Fatal(err)
	}
	if err := db2.Close(); err != nil {
		t.Fatal(err)
	}

	from, err := migrate.RestoreBackup(path, bak)
	if err != nil {
		t.Fatal(err)
	}
	if from != bak {
		t.Fatalf("from=%s bak=%s", from, bak)
	}

	db3 := openRaw(t, path)
	var n int
	if err := db3.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("projects=%d want 1 after restore", n)
	}
	var id string
	if err := db3.QueryRow(`SELECT id FROM projects`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != "p1" {
		t.Fatalf("id=%s", id)
	}
}

func TestLatestBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.db")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	older := path + ".bak.20200101T000000Z"
	newer := path + ".bak.20260101T000000Z"
	if err := os.WriteFile(older, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := migrate.LatestBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != newer {
		t.Fatalf("got=%s want=%s", got, newer)
	}
}
