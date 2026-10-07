package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	MarkerFile       = ".hc"
	LegacyHCDir      = ".hybrid-coco"
	IndexesDirName   = "indexes"
	// DBFile is the shared SQLite database under the data root.
	DBFile = "hc.db"
	// IndexFile is a deprecated alias of DBFile (kept for transitional call sites).
	IndexFile = DBFile
	// LegacySharedDBFile is the former shared DB name under the data root.
	LegacySharedDBFile = "index.db"
	// LegacyPerProjectDBFile is the DB name inside obsolete indexes/<id>/.
	LegacyPerProjectDBFile = "index.db"
	// LegacyCocosDBFile is the obsolete standalone coco registry DB.
	LegacyCocosDBFile = "cocos.db"
	SchemaVersion    = 6
	SchemaVersionKey = "schema_version"
	// DataDirName is the directory under ~/.local/share for shared state.
	DataDirName = "hc"
	// LegacyDataDirName is the pre-rename data root (hybrid-coco branding).
	LegacyDataDirName = "hybrid-coco"
)

var (
	ErrMarkerNotFound   = errors.New("marker .hc not found; run: hc init")
	ErrPathChanged      = errors.New("project path changed; run: hc init")
	ErrVersionMismatch  = errors.New("hc version mismatch; run: hc init")
	ErrCoreVersionUnset = errors.New("hc core version is not set")
)

var AlwaysIgnore = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"__pycache__":  {},
	".hybrid-coco": {},
	".venv":        {},
	"venv":         {},
	"dist":         {},
	"build":        {},
	"target":       {},
}

// Marker is the on-disk .hc activation file.
// Version is the hc binary version that created/last wrote the marker.
type Marker struct {
	Version string `json:"version"`
	ID      string `json:"id"`
}

var (
	dataRootMu sync.Mutex
	dataRoot   string // empty = production default under home

	coreVersionMu sync.Mutex
	coreVersion   string
)

// SetCoreVersion sets the running hc binary version. Required before WriteMarker / ResolveProject.
func SetCoreVersion(v string) {
	coreVersionMu.Lock()
	defer coreVersionMu.Unlock()
	coreVersion = v
}

func CoreVersion() string {
	coreVersionMu.Lock()
	defer coreVersionMu.Unlock()
	return coreVersion
}

func requireCoreVersion() (string, error) {
	v := CoreVersion()
	if v == "" {
		return "", ErrCoreVersionUnset
	}
	return v, nil
}

// SetDataRootForTest overrides the central data root. Empty restores default.
func SetDataRootForTest(path string) {
	dataRootMu.Lock()
	defer dataRootMu.Unlock()
	dataRoot = path
}

func DataRoot() (string, error) {
	dataRootMu.Lock()
	override := dataRoot
	dataRootMu.Unlock()
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("home directory is required")
	}
	return filepath.Join(home, ".local", "share", DataDirName), nil
}

// ClaudeConfigDir returns Claude Code's user config directory.
// Respects CLAUDE_CONFIG_DIR when set; otherwise ~/.claude (Claude Code default).
func ClaudeConfigDir() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("home directory is required")
	}
	return filepath.Join(home, ".claude"), nil
}

// LegacyDataRoot is the obsolete ~/.local/share/hybrid-coco path.
func LegacyDataRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("home directory is required")
	}
	return filepath.Join(home, ".local", "share", LegacyDataDirName), nil
}

// MigrateLegacyDataRoot renames ~/.local/share/hybrid-coco → ~/.local/share/hc
// when only the legacy path exists. Errors if both exist (manual resolve required).
func MigrateLegacyDataRoot() (migrated bool, err error) {
	dataRootMu.Lock()
	override := dataRoot
	dataRootMu.Unlock()
	if override != "" {
		// tests use an override; never touch real home legacy paths
		return false, nil
	}
	legacy, err := LegacyDataRoot()
	if err != nil {
		return false, err
	}
	current, err := DataRoot()
	if err != nil {
		return false, err
	}
	_, legErr := os.Stat(legacy)
	_, curErr := os.Stat(current)
	legacyExists := legErr == nil
	currentExists := curErr == nil
	if !legacyExists {
		return false, nil
	}
	if currentExists {
		return false, fmt.Errorf("both %s and %s exist; remove or merge manually before continuing", legacy, current)
	}
	parent := filepath.Dir(current)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(legacy, current); err != nil {
		return false, fmt.Errorf("migrate data root %s → %s: %w", legacy, current, err)
	}
	return true, nil
}

func CanonicalRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	canon, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return canon, nil
}

func RootID(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

// SharedIndexPath is the single SQLite file for all enrolled projects and cocos.
func SharedIndexPath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, DBFile), nil
}

// EnsureSharedIndex creates the data root directory and returns the shared DB path.
// Does not create the DB file; store.Open does.
// Does not remove obsolete indexes/<id>/ — doctor/migrate handle that.
// Migrates legacy data-root and index.db → hc.db when needed.
func EnsureSharedIndex() (string, error) {
	if _, err := MigrateLegacyDataRoot(); err != nil {
		return "", err
	}
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	if _, err := MigrateLegacySharedDB(root); err != nil {
		return "", err
	}
	return filepath.Join(root, DBFile), nil
}

// MigrateLegacySharedDB renames index.db → hc.db (and matching .bak.* files).
func MigrateLegacySharedDB(root string) (migrated bool, err error) {
	if root == "" {
		return false, fmt.Errorf("data root is required")
	}
	legacy := filepath.Join(root, LegacySharedDBFile)
	current := filepath.Join(root, DBFile)
	_, legErr := os.Stat(legacy)
	_, curErr := os.Stat(current)
	legacyExists := legErr == nil
	currentExists := curErr == nil
	if !legacyExists {
		return false, nil
	}
	if currentExists {
		return false, fmt.Errorf("both %s and %s exist; remove or merge manually before continuing", legacy, current)
	}
	if err := os.Rename(legacy, current); err != nil {
		return false, fmt.Errorf("migrate shared db %s → %s: %w", legacy, current, err)
	}
	// rename sidecar WAL/SHM if present next to old name (usually gone after close)
	_ = os.Rename(legacy+"-wal", current+"-wal")
	_ = os.Rename(legacy+"-shm", current+"-shm")
	entries, err := os.ReadDir(root)
	if err != nil {
		return true, nil
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, LegacySharedDBFile+".bak.") {
			newName := DBFile + strings.TrimPrefix(name, LegacySharedDBFile)
			_ = os.Rename(filepath.Join(root, name), filepath.Join(root, newName))
		}
		if strings.HasPrefix(name, LegacySharedDBFile+".pre-restore.") {
			newName := DBFile + strings.TrimPrefix(name, LegacySharedDBFile)
			_ = os.Rename(filepath.Join(root, name), filepath.Join(root, newName))
		}
	}
	return true, nil
}

// LegacyIndexesDir is the obsolete per-project layout under the data root.
func LegacyIndexesDir() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, IndexesDirName), nil
}

// ListLegacyIndexIDs returns project ids that still have indexes/<id>/index.db.
func ListLegacyIndexIDs() ([]string, error) {
	dir, err := LegacyIndexesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dbPath := filepath.Join(dir, e.Name(), LegacyPerProjectDBFile)
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		ids = append(ids, e.Name())
	}
	return ids, nil
}

// PurgeLegacyIndexes removes the obsolete indexes/ tree under the data root.
func PurgeLegacyIndexes() error {
	dir, err := LegacyIndexesDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(dir)
}

func ReadMarker(root string) (Marker, error) {
	path := filepath.Join(root, MarkerFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Marker{}, ErrMarkerNotFound
		}
		return Marker{}, err
	}
	var m Marker
	if err := json.Unmarshal(data, &m); err != nil {
		return Marker{}, fmt.Errorf("invalid %s: %w", MarkerFile, err)
	}
	if m.Version == "" {
		return Marker{}, fmt.Errorf("%s version is required (hc core version that wrote the marker)", MarkerFile)
	}
	if m.ID == "" {
		return Marker{}, fmt.Errorf("%s id is required", MarkerFile)
	}
	return m, nil
}

func WriteMarker(root, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	ver, err := requireCoreVersion()
	if err != nil {
		return err
	}
	m := Marker{Version: ver, ID: id}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(root, MarkerFile), data, 0o644)
}

func RemoveMarker(root string) error {
	err := os.Remove(filepath.Join(root, MarkerFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func RemoveLegacyProjectIndex(root string) error {
	legacy := filepath.Join(root, LegacyHCDir)
	if _, err := os.Stat(legacy); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(legacy)
}

// ResolveProject walks up from start for .hc, validates id and hc core version.
func ResolveProject(start string) (root string, id string, dbPath string, err error) {
	running, err := requireCoreVersion()
	if err != nil {
		return "", "", "", err
	}
	root, marker, err := FindRoot(start)
	if err != nil {
		return "", "", "", err
	}
	if marker.Version != running {
		return "", "", "", fmt.Errorf("%w: marker=%s running=%s", ErrVersionMismatch, marker.Version, running)
	}
	canon, err := CanonicalRoot(root)
	if err != nil {
		return "", "", "", err
	}
	computed := RootID(canon)
	if computed != marker.ID {
		return "", "", "", ErrPathChanged
	}
	dbPath, err = SharedIndexPath()
	if err != nil {
		return "", "", "", err
	}
	return root, marker.ID, dbPath, nil
}

func FindRoot(start string) (string, Marker, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", Marker{}, err
	}
	dir := abs
	info, err := os.Stat(dir)
	if err != nil {
		return "", Marker{}, err
	}
	if !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		markerPath := filepath.Join(dir, MarkerFile)
		if st, err := os.Stat(markerPath); err == nil && !st.IsDir() {
			m, err := ReadMarker(dir)
			if err != nil {
				return "", Marker{}, err
			}
			return dir, m, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", Marker{}, ErrMarkerNotFound
		}
		dir = parent
	}
}

// InitProject writes .hc and ensures the shared index path exists.
// Caller must EnrollProject in the store and index. Returns canon, id, db path.
func InitProject(root string) (canon string, id string, dbPath string, err error) {
	canon, err = CanonicalRoot(root)
	if err != nil {
		return "", "", "", err
	}
	if err := RemoveLegacyProjectIndex(canon); err != nil {
		return "", "", "", err
	}
	id = RootID(canon)
	if err := WriteMarker(canon, id); err != nil {
		return "", "", "", err
	}
	dbPath, err = EnsureSharedIndex()
	if err != nil {
		return "", "", "", err
	}
	return canon, id, dbPath, nil
}

// ResetProject removes the .hc marker only. Caller must DeleteProject in the store.
// Skips running-version check so upgrades can reset a marker from an older hc.
func ResetProject(start string) (root string, id string, err error) {
	root, marker, err := FindRoot(start)
	if err != nil {
		if errors.Is(err, ErrMarkerNotFound) {
			abs, aerr := filepath.Abs(start)
			if aerr != nil {
				return "", "", aerr
			}
			if rerr := RemoveLegacyProjectIndex(abs); rerr != nil {
				return "", "", rerr
			}
			return abs, "", ErrMarkerNotFound
		}
		return "", "", err
	}
	id = marker.ID
	if err := RemoveMarker(root); err != nil {
		return "", "", err
	}
	_ = RemoveLegacyProjectIndex(root)
	return root, id, nil
}
