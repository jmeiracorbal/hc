# hc

[![CI](https://github.com/jmeiracorbal/hc/actions/workflows/ci.yml/badge.svg)](https://github.com/jmeiracorbal/hc/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![GitHub issues](https://github.com/jmeiracorbal/hc/issues)](https://github.com/jmeiracorbal/hc/issues)

Local code intelligence for AI agents. Index your codebase once, query it deterministically: **~87% fewer tokens** than grep + cat.

`hc` builds a local SQLite index of your source code using tree-sitter, exposes it via a CLI and an MCP server, and integrates with Claude Code via hooks. No embeddings, no vector database, no Docker. One install command.

```
curl -sSf https://raw.githubusercontent.com/jmeiracorbal/hc/main/install.sh | bash
hc init
```

### The problem it solves

When Claude reads a file to find one function, it pays for the entire file:

```
# Without hc
cat("internal/store/store_test.go")              >  32,531 tokens  (whole file)

# With hc
hc file-context("internal/store/store_test.go")  >   3,448 tokens  (symbols)  < 89.4% savings
```

The hook intercepts `Read` and `Grep` calls and suggests the equivalent `hc_*` tool. Same answer, fraction of the tokens.

## How it works

```
Source files  ──tree-sitter──►  SQLite + FTS5  ──►  CLI (hc)
   (per project, .hc)                │
                                     │  ~/.local/share/hybrid-coco/index.db
                                     │  (shared, multi-project)
                                     └──────────────►  MCP server (hc_*)
                                                           │
                                                    Claude Code hooks
                                                    intercept Read/Grep
                                                    > suggest hc_* tools
```

1. **`hc setup`**: creates the shared DB and installs global hooks/skills/awareness (also run by `install.sh`).
2. **`hc init`**: writes marker `.hc`, enrolls the project in the shared index, indexes the tree, registers MCP, ensures hooks.
3. **Query CLI / MCP**: returns symbols, call graph, and file/package outlines — not whole files.
4. **Hooks**: with `.hc` present, PreToolUse suggests `hc_*` instead of blind `Read`/`Grep`; PostToolUse runs `hc update` after edits.

## Benchmark

Measured on [mnemo](https://github.com/jmeiracorbal/mnemo) (Go) with hc v0.1.2: 125 files, 1,372 symbols. Tokens ≈ chars/4.

| Query | Traditional | hc | Savings |
|---|---|---|---|
| Symbol lookup (`AddObservation`) | 6,502 tok | 319 tok | **95.1%** |
| Pattern search (`ImportObservation`) | 229 tok | 35 tok | **84.7%** |
| File structure (`migrate.go`) | 4,694 tok | 937 tok | **80.0%** |
| File structure (`projects.go`) | 5,339 tok | 1,528 tok | **71.4%** |
| File structure (`store_test.go`) | 32,531 tok | 3,448 tok | **89.4%** |
| Package outline (`internal/mcp`) | 15,344 tok | 1,841 tok | **88.0%** |
| Impact (`AddObservation`) | 6,502 tok | 456 tok | **93.0%** |
| **Total (7 queries)** | **71,141 tok** | **8,564 tok** | **~88%** |

Traditional = `rg` + `cat`. hc = `hc symbol` + `hc query` + `hc file-context` + `hc package` + `hc impact`. Content-only greps (e.g. SQL literals) are outside FTS scope and excluded.

## Quickstart

### 1. Install

**Option A: One-line installer (recommended)**

```bash
curl -sSf https://raw.githubusercontent.com/jmeiracorbal/hc/main/install.sh | bash
```

Downloads the `hc` binary from GitHub Releases into `~/.local/bin`, verifies the checksum, and runs `hc setup` (shared DB + Claude Code hooks/awareness). Requires macOS or Linux (amd64/arm64).

Pin a version:

```bash
curl -sSf https://raw.githubusercontent.com/jmeiracorbal/hc/main/install.sh | HC_VERSION=v0.3.0 bash
```

Later upgrades (same machine):

```bash
hc upgrade                 # check GitHub latest
hc upgrade --install --yes # download, verify sha256, replace this binary
```

**Option B: Claude Code plugin**

```bash
claude plugin marketplace add jmeiracorbal/hc
claude plugin install hybrid-coco@hybrid-coco
```

Registers the MCP server and hooks automatically. Requires `hc` in PATH — install the binary first (Option A).

**Option C: Build from source**

```bash
git clone https://github.com/jmeiracorbal/hc
cd hc
CGO_ENABLED=1 go build -o ~/.local/bin/hc ./cmd/hc/
hc setup
```

### 2. Index your project

```bash
cd your-project/
hc init
```

`hc init` does the following:
- Creates marker `.hc` (project opt-in; id + running `hc` version)
- Enrolls the project in the shared index at `~/.local/share/hybrid-coco/index.db`
- Indexes the tree (tree-sitter, SHA-256 incremental)
- Registers the MCP server in `.claude/settings.json`
- Ensures global hooks/skills under `~/.claude/`

Restart Claude Code to activate.

### 3. Use from Claude Code

MCP tools (require `.hc` in the project cwd):

```
hc_search("savings_pct")       # FTS5 search over names, signatures, docstrings
hc_symbol("TimedExecution")    # exact/prefix symbol lookup
hc_file_context("src/git.rs")  # symbols in a file + Read range hints
hc_package("internal/store")   # directory outline + fan-in
hc_explore("Open")             # callers, callees, containment
hc_impact("Open")              # blast radius
hc_path("main", "Open")        # call path (BFS)
hc_status()                    # index stats + call resolve ratio
```

## CLI reference

```
hc index [PATH]          Index PATH (default: cwd); --force reindexes all
hc update [PATH]         Re-index only changed files (SHA-256 diff) + prune deleted
hc status [PATH]         Index stats: files, symbols, edges, calls resolved
hc query <TEXT>          FTS5 trigram search on name, signature, docstring
hc symbol <NAME>         Exact name lookup, then prefix fallback
hc file-context <PATH>   All symbols in PATH (~97% savings vs cat)
hc package <DIR>         Directory outline with fan-in and Read range hints
hc explore <NAME>        Callers, callees, containment, fan-in/out
hc impact <NAME>         Blast radius: callers + file importers
hc path <FROM> <TO>      Call path between two symbols
hc serve                 Start MCP server (stdio)
hc init [PATH]           Marker .hc + enroll + index + register MCP + hooks
hc reset [PATH]          Remove .hc and this project's rows from the shared DB
hc setup                 Shared index.db + global hooks/skills (used by install.sh)
hc doctor [--json]       Diagnose binary, schema, marker, legacy indexes/
hc doctor --fix [--yes] Apply repairs (destructive ones need --yes)
hc migrate               Apply pending shared-schema migrations (+ backup)
hc migrate --restore-backup --yes [--backup PATH]
hc migrate --import-legacy --map id=/abs/path [--purge-legacy --yes]
hc upgrade               Compare running binary vs GitHub latest
hc upgrade --yes         Print install.sh hint
hc upgrade --install --yes
```

## Maintenance

Shared index path: `~/.local/share/hybrid-coco/index.db` (schema version in `meta`).

| Situation | Command |
|---|---|
| Schema pending after upgrade | `hc migrate` or `hc doctor --fix` |
| Broken / too-old DB, have `.bak` | `hc migrate --restore-backup --yes` |
| Old layout `indexes/<id>/` still on disk | `hc migrate --import-legacy --map id=/abs/path` then `--purge-legacy --yes`, or `hc doctor --fix --yes` to purge only |
| Binary outdated | `hc upgrade --install --yes` |
| Full diagnose for agents | `hc doctor --json` (skill `hc-doctor`) |

Open never auto-migrates a non-empty DB. There are no down-migrations — rollback is restore from `index.db.bak.*`.

## Supported languages

| Language | Parser |
|---|---|
| Go | tree-sitter-go |
| Python | tree-sitter-python |
| Rust | tree-sitter-rust |
| JavaScript | tree-sitter-javascript |
| TypeScript / TSX | tree-sitter-typescript |

Adding a language means implementing a parser in `internal/parsers/` (modular plugin load is a future track).

## Design decisions

**SQLite + FTS5, not a vector database**: deterministic results, zero infrastructure. One shared file for all enrolled projects; each project opts in with `.hc`.

**tree-sitter, not regex**: symbol and call extraction is grammar-aware. Call edges resolve with a precision ladder (qualifier → same-file → project).

**Single Go binary**: `hc` is CLI + MCP server. No Python runtime. Distributed via GitHub Releases + `install.sh`.

**No server process**: `hc serve` runs as a stdio MCP server launched on demand by Claude Code. There is no daemon to manage.

**Incremental by default**: `hc update` re-indexes only files whose SHA-256 has changed and prunes deleted files. Full re-index with `hc index --force`.

**Forward-only schema**: `hc migrate` applies pending steps; backups are automatic; restore via `--restore-backup`.

## Development

```bash
git clone https://github.com/jmeiracorbal/hc
cd hc
CGO_ENABLED=1 go build -o hc ./cmd/hc/
./hc --version
```

Run tests:

```bash
CGO_ENABLED=1 go test ./...
```
