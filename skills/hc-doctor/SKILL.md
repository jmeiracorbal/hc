---
name: hc-doctor
description: ALWAYS use on ErrMigrationsPending, schema errors, broken index/status after upgrade, marker mismatch, or legacy data-root. Auto-recovery is built in — run hc doctor --fix without asking the user for migrate.
---

# hc-doctor

Diagnose and **auto-recover** hybrid-coco. Schema migrate is not a user decision: the agent (and `hc upgrade --install`) apply safe repairs. Gate on project marker `.hc` when diagnosing marker/version mismatches.

## Auto-recovery (mandatory)

hc self-heals safe problems. When anything looks wrong (migrations pending, marker mismatch, legacy data root, post-upgrade breakage):

```bash
hc doctor --fix --json
```

Do **not** ask the user whether to migrate. Do **not** tell them to run `hc migrate` manually first. `doctor --fix` already applies `apply_migrations` (with backup), `migrate_data_root`, and `sync_marker_version`. It loops until stable.

Then re-check `healthy` in the JSON. If still unhealthy only for destructive repairs (`recreate_empty_db`, `purge_legacy_indexes`, `restore_latest_backup`), explain and ask once before:

```bash
hc doctor --fix --yes --json
```

Or restore explicitly:

```bash
hc migrate --restore-backup --yes
```

## Model

- Shared DB: `~/.local/share/hc/hc.db` (`meta.schema_version` int).
- Project marker `.hc`: opt-in; `version` must match running `hc` core version.
- Open fails fast on pending migrations (`ErrMigrationsPending`) — that is the signal to auto-recover, not to stop.
- `hc upgrade --install --yes` replaces the binary then **re-execs** `hc doctor --fix` on the new binary (old process cannot migrate to a newer schema).
- Rollback: `hc.db.bak.*` via `--restore-backup` — no down-migrations.
- Legacy `indexes/<id>/`: detected; purge needs `--yes`.

## Flow

1. `hc doctor --fix --json` immediately (auto-recovery).
2. If `binary.outdated` → propose `hc upgrade --install --yes` (needs user OK for download). After install, recovery runs automatically.
3. If still `legacy` / recreate / restore in `repairs_available` → ask user, then `--fix --yes` or `hc migrate --restore-backup --yes`.
4. Re-run until `healthy: true`.
5. `hc status` / `hc update` only if index empty or stale after recovery.

## Commands

| Command | When |
|---|---|
| `hc doctor --fix --json` | Default agent action — safe auto-recovery |
| `hc doctor --json` | Diagnose only |
| `hc doctor --fix --yes` | Include destructive recreate/purge/restore |
| `hc migrate` | Manual equivalent of schema repair (prefer doctor --fix) |
| `hc migrate --restore-backup --yes` | Restore latest (or `--backup PATH`) `.bak` |
| `hc upgrade --install --yes` | Binary update + automatic post-upgrade recovery |

## Do not

- Ask the user to decide on schema migrate.
- Hand-edit SQLite or delete `hc.db` without doctor backup/recreate path.
- Auto-download binaries without user OK.
- Assume down-migrations exist — use `--restore-backup`.
