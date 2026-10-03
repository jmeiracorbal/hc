package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LatestBackup returns the newest index.db.bak.<utc> next to dbPath.
func LatestBackup(dbPath string) (string, error) {
	if dbPath == "" {
		return "", fmt.Errorf("db path is required")
	}
	dir := filepath.Dir(dbPath)
	base := filepath.Base(dbPath)
	prefix := base + ".bak."
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no backup found for %s", dbPath)
		}
		return "", err
	}
	var cands []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, prefix) {
			cands = append(cands, filepath.Join(dir, name))
		}
	}
	if len(cands) == 0 {
		return "", fmt.Errorf("no backup found for %s", dbPath)
	}
	sort.Strings(cands)
	return cands[len(cands)-1], nil
}

// RestoreBackup replaces dbPath with bakPath (or LatestBackup when bakPath empty).
// Current db is moved to index.db.pre-restore.<utc> when it exists.
func RestoreBackup(dbPath, bakPath string) (restoredFrom string, err error) {
	if dbPath == "" {
		return "", fmt.Errorf("db path is required")
	}
	if bakPath == "" {
		bakPath, err = LatestBackup(dbPath)
		if err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(bakPath); err != nil {
		return "", err
	}
	if _, err := os.Stat(dbPath); err == nil {
		_, _ = Backup(dbPath) // best-effort; unreadable db still moved aside
		stamp := time.Now().UTC().Format("20060102T150405Z")
		aside := dbPath + ".pre-restore." + stamp
		if err := os.Rename(dbPath, aside); err != nil {
			return "", err
		}
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := copyFile(bakPath, dbPath); err != nil {
		return "", err
	}
	return bakPath, nil
}
