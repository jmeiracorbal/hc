# Hybrid Context

[![CI](https://github.com/jmeiracorbal/hc/actions/workflows/ci.yml/badge.svg)](https://github.com/jmeiracorbal/hc/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/jmeiracorbal/hc?display_name=tag)](https://github.com/jmeiracorbal/hc/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/jmeiracorbal/hc)](https://github.com/jmeiracorbal/hc/blob/main/go.mod)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)](https://github.com/jmeiracorbal/hc/releases)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![GitHub issues](https://img.shields.io/github/issues/jmeiracorbal/hc)](https://github.com/jmeiracorbal/hc/issues)

Local code intelligence for AI agents. Index your codebase once, query it deterministically: **~87% fewer tokens** than grep + cat.

`hc` builds a local SQLite index of your source code using tree-sitter, exposes it via a CLI and an MCP server, and integrates with Claude Code via hooks. Extra languages install as **wasm cocos** without recompiling the core. No embeddings, no vector database, no Docker. Two commands to start:

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
Source files  ──tree-sitter / coco──►  SQLite + FTS5  ──►  CLI (hc)
   (per project, .hc)                        │
                                             │  ~/.local/share/hc/hc.db
                                             │  (shared index + coco registry)
                                             └──────────────►  MCP server (hc_*)
                                                                   │
                                                            Claude Code hooks
                                                            intercept Read/Grep
                                                            > suggest hc_* tools
```

1. **`hc setup`**: creates the shared DB and installs global hooks/skills/awareness (also run by `install.sh`).
2. **`hc init`**: writes marker `.hc`, enrolls the project in the shared index, indexes the tree, registers MCP, ensures hooks.
3. **Query CLI / MCP**: returns symbols, call graph, and file/package outlines, not whole files.
4. **Hooks**: with `.hc` present, PreToolUse suggests `hc_*` instead of blind `Read`/`Grep`; PostToolUse runs `hc update` after edits.
5. **Cocos** (optional): install language packs (`hc coco install …`); the indexer loads them via wazero (`wasm/v1`).

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
curl -sSf https://raw.githubusercontent.com/jmeiracorbal/hc/main/install.sh | HC_VERSION=v0.2.0 bash
```

Later upgrades (same machine):

```bash
hc upgrade                 # check GitHub latest
hc upgrade --install --yes # download, verify sha256, replace binary, then auto-recovery
```

After `--install`, hc re-executes the **new** binary with `hc doctor --fix` so pending schema migrations and safe repairs run without a separate manual `hc migrate`.

**Option B: Claude Code plugin**

```bash
claude plugin marketplace add jmeiracorbal/hc
claude plugin install hybrid-coco@hybrid-coco
```

Registers the MCP server and hooks automatically. Requires `hc` in PATH; install the binary first (Option A).

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
- Enrolls the project in the shared index at `~/.local/share/hc/hc.db`
- Indexes the tree (tree-sitter / installed cocos, SHA-256 incremental)
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
hc setup                 Shared hc.db + global hooks/skills (used by install.sh)
hc coco install <ref>    Install coco from github.com/org/repo@vX.Y.Z
hc coco install --local DIR [--replace]
hc coco list             Built-ins + installed wasm cocos
hc coco uninstall <ID>   Remove an installed coco (e.g. hc/java)
hc doctor [--json]       Diagnose binary, schema, marker, legacy, cocos
hc doctor --fix [--yes] Safe auto-recovery (destructive repairs need --yes)
hc migrate               Apply pending shared-schema migrations (+ backup)
hc migrate --restore-backup --yes [--backup PATH]
hc migrate --import-legacy --map id=/abs/path [--purge-legacy --yes]
hc upgrade               Compare running binary vs GitHub latest
hc upgrade --yes         Print install.sh hint
hc upgrade --install --yes
```

## Maintenance

Shared index path: `~/.local/share/hc/hc.db` (schema version in `meta`). Cocos registry lives in the same file (`cocos` table) plus install dirs under `~/.local/share/hc/cocos/`.

| Situation | What happens / command |
|---|---|
| Schema pending after binary upgrade | **Automatic** via `hc upgrade --install --yes` (re-exec `doctor --fix`). Agents: `hc doctor --fix --json` (skill `hc-doctor`). Do not ask the user to migrate. |
| Broken / too-old DB, have `.bak` | `hc migrate --restore-backup --yes` or `hc doctor --fix --yes` |
| Old layout `indexes/<id>/` still on disk | `hc migrate --import-legacy --map id=/abs/path` then `--purge-legacy --yes`, or `hc doctor --fix --yes` to purge only |
| Legacy data root `~/.local/share/hybrid-coco` | Renamed automatically to `~/.local/share/hc` on first open |
| Binary outdated | `hc upgrade --install --yes` |
| Full diagnose for agents | `hc doctor --json` |

Open never auto-migrates a non-empty DB on its own (fail-fast `ErrMigrationsPending`). That error is the signal for `doctor --fix` / post-upgrade recovery. There are no down-migrations; rollback is restore from `hc.db.bak.*`.

## Supported languages

### Built-in (in-process tree-sitter)

| Language | Parser |
|---|---|
| Go | tree-sitter-go |
| Python | tree-sitter-python |
| Rust | tree-sitter-rust |
| JavaScript | tree-sitter-javascript |
| TypeScript / TSX | tree-sitter-typescript |

### Language cocos (wasm/v1)

Cocos are **separate GitHub repos** (not vendored inside `hc`). Anyone can publish a language pack; the core only hosts the contract, install CLI, and wazero runtime.

**Install**

```bash
hc coco install github.com/org/my-java-coco@v0.1.0
hc coco install --local /path/to/my-coco --replace   # while developing
hc coco list
hc coco uninstall hc/java
```

**Official reference packs** (sibling checkouts / own repos; regex-based starters, not full tree-sitter):

| Coco id | Extensions | Typical repo name |
|---|---|---|
| `hc/java` | `.java` | `hc-java-coco` |
| `hc/php` | `.php` | `hc-php-coco` |
| `hc/csharp` | `.cs` | `hc-csharp-coco` |

```bash
hc coco install --local ../official-cocos/hc-java-coco --replace
```

**Contract (writing a coco)**

1. Root `coco.toml`: `id`, `language`, `extensions`, `priority`, `contract = "wasm/v1"`, `platforms = ["wasm32/wasi"]`, and `[hc-version]`.
2. Wasm module (`wasm32/wasi`) exporting `coco_abi_version`, `coco_alloc`, `coco_free`, `coco_parse`.
3. GitHub Release assets: `{name}-wasm32-wasi.wasm` (from id suffix, e.g. `java-wasm32-wasi.wasm`) + `coco.toml`.

Go build tip (reactor + `//go:wasmexport`):

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o java-wasm32-wasi.wasm .
```

**Project pins** (optional `hc.toml` next to `.hc`):

```toml
cocos = ["hc/java"]
[extension_pins]
".java" = "hc/java"
```

Flavors (e.g. `java` vs `java-springboot`) are separate coco ids with different `priority`; ties require pins.

## Design decisions

**SQLite + FTS5, not a vector database**: deterministic results, zero infrastructure. One shared file for all enrolled projects; each project opts in with `.hc`.

**tree-sitter for built-ins**: symbol and call extraction is grammar-aware. Call edges resolve with a precision ladder (qualifier → same-file → project).

**Single Go binary**: `hc` is CLI + MCP server. No Python runtime. Distributed via GitHub Releases + `install.sh`. Extra languages load as wasm cocos (wazero), not linked into the binary.

**Modular cocos**: built-ins stay in-process; external packs use a prunesh-style install/registry UX with a `wasm/v1` ABI suited to per-file indexing.

**Auto-recovery on upgrade**: installing a new binary re-runs safe `doctor --fix` on that binary so schema bumps are not a manual user step. Agents use the `hc-doctor` skill the same way.

**No server process**: `hc serve` runs as a stdio MCP server launched on demand by Claude Code. There is no daemon to manage.

**Incremental by default**: `hc update` re-indexes only files whose SHA-256 has changed and prunes deleted files. Full re-index with `hc index --force`.

**Forward-only schema**: migrations apply forward; backups are automatic; restore via `--restore-backup`.

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
