---
name: hc-index
description: Use when .hc is missing, hc version mismatches the marker, the index is stale/empty, or first-time hybrid-coco setup is needed. Covers hc init, index, update, status, reset, setup.
---

# hc-index

Project index lifecycle for hybrid-coco. Independent from **hc-navigate** / **hc-graph**; hand off to those skills once `.hc` exists and the index is healthy.

## Model

- Marker: `.hc` in the project root (`version` = running `hc` core version, `id` = sha256 of canonical root).
- Store: single shared DB at `~/.local/share/hybrid-coco/index.db` (not inside the repo); projects are enrolled rows.
- Without `.hc`: CLI/MCP that need a project fail; hooks no-op.

## Commands

| Command | When |
|---|---|
| `hc setup` | Global hooks + awareness + skills + create shared `index.db` |
| `hc init [path]` | Create `.hc`, enroll in shared index, index, register MCP |
| `hc index [path]` | Full (re)index when marker present and enrolled |
| `hc update [path]` | Incremental reindex (changed sha256) |
| `hc status [path]` | Files/symbols/edges/hotspots |
| `hc reset [path]` | Remove marker + project rows from shared index |

## Workflows

### First time in a project

```bash
hc --version          # binary on PATH
hc init .             # .hc + enroll + index + MCP in .claude/settings.json
# restart Claude Code so MCP/skills reload
hc_status()           # or: hc status
```

### Version mismatch

If tools/CLI report `hc version mismatch; run: hc init`:

```bash
hc init .
```

Marker `version` must equal the running binary version.

### Stale or empty results

```bash
hc status .
hc update .           # changed files only
# or
hc index .            # full rebuild
```

### Incompatible schema

Index schema is not migrated in place. Wipe shared index and rebuild:

```bash
rm ~/.local/share/hybrid-coco/index.db
hc setup
hc reset .            # if .hc still present
hc init .
```

Do not hand-edit SQLite. Do not recreate `.hybrid-coco/` in-repo (legacy; `hc init` removes it if present).

## After index is healthy

Use **hc-navigate** for Read/Grep replacement and **hc-graph** for callers/impact/path.
