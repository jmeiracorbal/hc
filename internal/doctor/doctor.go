package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/migrate"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
	"github.com/jmeiracorbal/hybrid-coco/internal/upgrade"
)

const (
	RepairApplyMigrations     = "apply_migrations"
	RepairSyncMarkerVersion   = "sync_marker_version"
	RepairRecreateEmptyDB     = "recreate_empty_db"
	RepairPurgeLegacyIndexes  = "purge_legacy_indexes"
	RepairRestoreLatestBackup = "restore_latest_backup"
)

type Report struct {
	Binary           BinaryReport   `json:"binary"`
	Schema           SchemaReport   `json:"schema"`
	DB               DBReport       `json:"db"`
	Marker           MarkerReport   `json:"marker"`
	Projects         ProjectsReport `json:"projects"`
	Legacy           LegacyReport   `json:"legacy"`
	Calls            *CallsReport   `json:"calls,omitempty"`
	RepairsAvailable []string       `json:"repairs_available"`
	Healthy          bool           `json:"healthy"`
}

type LegacyReport struct {
	Present bool     `json:"present"`
	IDs     []string `json:"ids,omitempty"`
}

type BinaryReport struct {
	Version       string `json:"version"`
	Latest        string `json:"latest,omitempty"`
	Outdated      bool   `json:"outdated"`
	CheckSkipped  bool   `json:"check_skipped,omitempty"`
	LatestUnknown bool   `json:"latest_unknown,omitempty"`
}

type SchemaReport struct {
	Path    string   `json:"path"`
	Have    int      `json:"have"`
	Want    int      `json:"want"`
	Pending []string `json:"pending"`
}

type DBReport struct {
	Readable    bool `json:"readable"`
	ForeignKeys bool `json:"foreign_keys"`
	WAL         bool `json:"wal"`
}

type MarkerReport struct {
	Present      bool   `json:"present"`
	Path         string `json:"path,omitempty"`
	Version      string `json:"version,omitempty"`
	VersionMatch bool   `json:"version_match"`
	IDMatch      bool   `json:"id_match"`
	ID           string `json:"id,omitempty"`
}

type ProjectsReport struct {
	EnrolledCount int `json:"enrolled_count"`
}

type CallsReport struct {
	Resolved int `json:"resolved"`
	Total    int `json:"total"`
}

type FixOpts struct {
	Yes bool
}

// Run gathers diagnostics for cwd and the shared index.
func Run(cwd string) (Report, error) {
	if cwd == "" {
		return Report{}, fmt.Errorf("cwd is required")
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return Report{}, err
	}
	var r Report
	r.Binary.Version = config.CoreVersion()
	r.Schema.Want = migrate.CurrentSchema()
	r.Schema.Pending = []string{}
	r.RepairsAvailable = []string{}

	gatherBinary(&r)
	dbPath, err := config.SharedIndexPath()
	if err != nil {
		return r, err
	}
	r.Schema.Path = dbPath
	gatherDB(&r, dbPath)
	gatherMarker(&r, abs)
	gatherLegacy(&r)
	if r.DB.Readable {
		gatherSchemaStats(&r, dbPath)
	}
	computeRepairs(&r)
	r.Healthy = isHealthy(r)
	return r, nil
}

func gatherLegacy(r *Report) {
	ids, err := config.ListLegacyIndexIDs()
	if err != nil {
		return
	}
	r.Legacy.IDs = ids
	r.Legacy.Present = len(ids) > 0
}

func gatherBinary(r *Report) {
	ver := r.Binary.Version
	if ver == "" || ver == "dev" {
		r.Binary.CheckSkipped = true
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	latest, err := upgrade.LatestRelease(ctx)
	if err != nil {
		r.Binary.LatestUnknown = true
		return
	}
	r.Binary.Latest = latest
	r.Binary.Outdated = upgrade.CompareVersions(ver, latest)
}

func gatherDB(r *Report, dbPath string) {
	if _, err := os.Stat(dbPath); err != nil {
		r.DB.Readable = false
		return
	}
	db, err := migrate.OpenDB(dbPath)
	if err != nil {
		r.DB.Readable = false
		return
	}
	defer db.Close()

	r.DB.Readable = true
	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err == nil {
		r.DB.ForeignKeys = fk == 1
	}
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err == nil {
		r.DB.WAL = mode == "wal" || mode == "WAL"
	}
}

func gatherMarker(r *Report, cwd string) {
	root, marker, err := config.FindRoot(cwd)
	if err != nil {
		if errors.Is(err, config.ErrMarkerNotFound) {
			r.Marker.Present = false
			return
		}
		r.Marker.Present = false
		return
	}
	r.Marker.Present = true
	r.Marker.Path = filepath.Join(root, config.MarkerFile)
	r.Marker.Version = marker.Version
	r.Marker.ID = marker.ID
	running := config.CoreVersion()
	r.Marker.VersionMatch = running != "" && marker.Version == running
	canon, err := config.CanonicalRoot(root)
	if err != nil {
		r.Marker.IDMatch = false
		return
	}
	r.Marker.IDMatch = config.RootID(canon) == marker.ID
}

func gatherSchemaStats(r *Report, dbPath string) {
	db, err := migrate.OpenDB(dbPath)
	if err != nil {
		return
	}
	defer db.Close()

	have, err := migrate.ReadVersion(db)
	if err != nil {
		return
	}
	r.Schema.Have = have
	pending, _, _, err := migrate.Pending(db)
	if err != nil {
		if errors.Is(err, migrate.ErrSchemaAhead) {
			r.Schema.Have = have
		}
		return
	}
	for _, m := range pending {
		r.Schema.Pending = append(r.Schema.Pending, m.Name)
	}

	var enrolled int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&enrolled); err == nil {
		r.Projects.EnrolledCount = enrolled
	}

	ready := migrate.CheckReady(db)
	if ready != nil {
		return
	}
	var total, resolved int
	_ = db.QueryRow(`SELECT COUNT(*) FROM edges WHERE kind = ?`, store.EdgeCalls).Scan(&total)
	_ = db.QueryRow(
		`SELECT COUNT(*) FROM edges WHERE kind = ? AND to_symbol_id IS NOT NULL`,
		store.EdgeCalls,
	).Scan(&resolved)
	r.Calls = &CallsReport{Resolved: resolved, Total: total}
}

func computeRepairs(r *Report) {
	if r.DB.Readable && len(r.Schema.Pending) > 0 && (r.Schema.Have >= 4 || r.Schema.Have == 0) {
		r.RepairsAvailable = append(r.RepairsAvailable, RepairApplyMigrations)
	}
	if r.Marker.Present && !r.Marker.VersionMatch && r.Marker.IDMatch &&
		r.Schema.Have == r.Schema.Want && len(r.Schema.Pending) == 0 {
		r.RepairsAvailable = append(r.RepairsAvailable, RepairSyncMarkerVersion)
	}
	if r.DB.Readable && r.Schema.Have > 0 && r.Schema.Have < 4 {
		r.RepairsAvailable = append(r.RepairsAvailable, RepairRecreateEmptyDB)
	}
	if !r.DB.Readable && r.Schema.Path != "" {
		if _, err := os.Stat(r.Schema.Path); err == nil {
			r.RepairsAvailable = append(r.RepairsAvailable, RepairRecreateEmptyDB)
		}
	}
	if r.Legacy.Present {
		r.RepairsAvailable = append(r.RepairsAvailable, RepairPurgeLegacyIndexes)
	}
	// restore only when db is broken / too old — not while pending migrations alone
	if r.Schema.Path != "" && ((!r.DB.Readable && fileExists(r.Schema.Path)) ||
		(r.DB.Readable && r.Schema.Have > 0 && r.Schema.Have < 4)) {
		if _, err := migrate.LatestBackup(r.Schema.Path); err == nil {
			r.RepairsAvailable = append(r.RepairsAvailable, RepairRestoreLatestBackup)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isHealthy(r Report) bool {
	if r.Binary.Outdated {
		return false
	}
	if len(r.Schema.Pending) > 0 {
		return false
	}
	if r.Schema.Have != r.Schema.Want && r.DB.Readable {
		return false
	}
	if r.Marker.Present && (!r.Marker.VersionMatch || !r.Marker.IDMatch) {
		return false
	}
	if r.DB.Readable && !r.DB.ForeignKeys {
		return false
	}
	if r.Legacy.Present {
		return false
	}
	return true
}

// Fix applies safe repairs listed in the report.
func Fix(report Report, opts FixOpts) ([]string, error) {
	var done []string
	for _, id := range report.RepairsAvailable {
		switch id {
		case RepairApplyMigrations:
			if err := fixApplyMigrations(report.Schema.Path); err != nil {
				return done, err
			}
			done = append(done, id)
		case RepairSyncMarkerVersion:
			if err := fixSyncMarker(report); err != nil {
				return done, err
			}
			done = append(done, id)
		case RepairRecreateEmptyDB:
			if !opts.Yes {
				return done, fmt.Errorf("recreate_empty_db requires --yes")
			}
			if err := fixRecreateDB(report.Schema.Path); err != nil {
				return done, err
			}
			done = append(done, id)
		case RepairPurgeLegacyIndexes:
			if !opts.Yes {
				return done, fmt.Errorf("purge_legacy_indexes requires --yes")
			}
			if err := config.PurgeLegacyIndexes(); err != nil {
				return done, err
			}
			done = append(done, id)
		case RepairRestoreLatestBackup:
			if !opts.Yes {
				return done, fmt.Errorf("restore_latest_backup requires --yes")
			}
			if _, err := migrate.RestoreBackup(report.Schema.Path, ""); err != nil {
				return done, err
			}
			done = append(done, id)
		default:
			return done, fmt.Errorf("unknown repair %q", id)
		}
	}
	return done, nil
}

func fixApplyMigrations(dbPath string) error {
	if dbPath == "" {
		return fmt.Errorf("schema path is required")
	}
	if _, err := os.Stat(dbPath); err == nil {
		if _, err := migrate.Backup(dbPath); err != nil {
			return err
		}
	}
	db, err := migrate.OpenDB(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = migrate.ApplyPending(db)
	return err
}

func fixSyncMarker(report Report) error {
	if report.Marker.Path == "" {
		return fmt.Errorf("marker path is required")
	}
	if report.Marker.ID == "" {
		return fmt.Errorf("marker id is required")
	}
	root := filepath.Dir(report.Marker.Path)
	return config.WriteMarker(root, report.Marker.ID)
}

func fixRecreateDB(dbPath string) error {
	if dbPath == "" {
		return fmt.Errorf("schema path is required")
	}
	if _, err := os.Stat(dbPath); err == nil {
		bak := dbPath + ".bak." + time.Now().UTC().Format("20060102T150405Z")
		if err := os.Rename(dbPath, bak); err != nil {
			return err
		}
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	}
	db, err := migrate.OpenDB(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = migrate.ApplyPending(db)
	return err
}

// FormatJSON returns stable JSON for agents.
func FormatJSON(r Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// FormatText is a human-readable summary.
func FormatText(r Report) string {
	out := fmt.Sprintf("hc doctor\n")
	out += fmt.Sprintf("  binary: %s", r.Binary.Version)
	if r.Binary.CheckSkipped {
		out += " (check skipped)"
	} else if r.Binary.LatestUnknown {
		out += " (latest unknown)"
	} else if r.Binary.Latest != "" {
		out += fmt.Sprintf(" latest=%s outdated=%v", r.Binary.Latest, r.Binary.Outdated)
	}
	out += "\n"
	out += fmt.Sprintf("  schema: %s have=%d want=%d pending=%v\n",
		r.Schema.Path, r.Schema.Have, r.Schema.Want, r.Schema.Pending)
	out += fmt.Sprintf("  db: readable=%v fk=%v wal=%v\n", r.DB.Readable, r.DB.ForeignKeys, r.DB.WAL)
	if r.Marker.Present {
		out += fmt.Sprintf("  marker: %s version_match=%v id_match=%v\n",
			r.Marker.Path, r.Marker.VersionMatch, r.Marker.IDMatch)
	} else {
		out += "  marker: absent\n"
	}
	out += fmt.Sprintf("  projects: enrolled=%d\n", r.Projects.EnrolledCount)
	if r.Legacy.Present {
		out += fmt.Sprintf("  legacy: indexes/ ids=%v (import: hc migrate --import-legacy --map id=/path)\n", r.Legacy.IDs)
	} else {
		out += "  legacy: absent\n"
	}
	if r.Calls != nil {
		out += fmt.Sprintf("  calls: %d/%d resolved\n", r.Calls.Resolved, r.Calls.Total)
	}
	out += fmt.Sprintf("  repairs: %v\n", r.RepairsAvailable)
	out += fmt.Sprintf("  healthy: %v\n", r.Healthy)
	return out
}
