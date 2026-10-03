---
name: hc-doctor
description: Use when hc migrate is needed, ErrMigrationsPending / schema errors, binary outdated, marker version mismatch after upgrade, legacy indexes/, or index/status broken post-update. Diagnose with hc doctor --json and apply safe repairs.
---

# hc-doctor

Diagnose and repair hybrid-coco shared index + marker + binary freshness. Prefer this over wiping `index.db` unless doctor offers `recreate_empty_db`.

## Model

- Shared DB: `~/.local/share/hybrid-coco/index.db` (`meta.schema_version` int).
- Migrations: forward-only via `hc migrate` (Open never auto-migrates non-empty DBs).
- Rollback: restore from `index.db.bak.*` — no down-migrations.
- Legacy layout: `indexes/<id>/index.db` is detected, not auto-deleted.
- Marker `.hc`: project opt-in; `version` must match running `hc` core version.
- Binary updates: `hc upgrade --install --yes` (checksum verified) or `install.sh`.

## Mandatory flow

```bash
hc doctor --json
```

1. Read `schema.pending`, `binary.outdated`, `marker.version_match`, `legacy.present`, `repairs_available`.
2. If `pending` non-empty → `hc migrate` (or `hc doctor --fix` when `apply_migrations` is listed).
3. If `binary.outdated` → tell user; propose `hc upgrade --install --yes`. Do not install without explicit user OK.
4. If `legacy.present` → list ids; import with `hc migrate --import-legacy --map id=/abs/path [--purge-legacy --yes]`, or purge via `hc doctor --fix --yes` (`purge_legacy_indexes`).
5. If only marker version mismatch and schema OK → `hc doctor --fix` (`sync_marker_version`) or `hc init` if id mismatch.
6. If `recreate_empty_db` / `restore_latest_backup` → require explicit user OK, then `hc doctor --fix --yes` or `hc migrate --restore-backup --yes`.
7. Re-run `hc doctor --json` until `healthy: true`.
8. Then `hc status` / `hc update` only if doctor/index still empty or stale.

## Commands

| Command | When |
|---|---|
| `hc doctor` | Human-readable diagnosis |
| `hc doctor --json` | Agent parse (stable fields) |
| `hc doctor --fix` | Safe repairs only |
| `hc doctor --fix --yes` | Include destructive recreate/purge/restore |
| `hc migrate` | Apply pending schema steps (+ backup) |
| `hc migrate --restore-backup --yes` | Restore latest (or `--backup PATH`) `.bak` |
| `hc migrate --import-legacy --map id=/path` | Import legacy per-project DB |
| `hc migrate --import-legacy --map id=/path --purge-legacy --yes` | Import then delete `indexes/` |
| `hc upgrade` | Compare running vs GitHub latest |
| `hc upgrade --yes` | Print `install.sh` curl hint |
| `hc upgrade --install --yes` | Download, verify sha256, replace binary |

## Do not

- Hand-edit SQLite.
- Delete `index.db` without doctor backup / recreate path.
- Auto-download binaries without user confirmation (`--install --yes` only after OK).
- Assume down-migrations exist — use `--restore-backup`.
