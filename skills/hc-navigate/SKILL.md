---
name: hc-navigate
description: ALWAYS ACTIVE when .hc exists — prefer hc_* MCP tools over blind Read/Grep for code navigation. Gate on marker .hc; use file/dir/search tools and ranged Read only.
---

# hc-navigate

Use hybrid-coco MCP tools for deterministic code navigation. Index once, query always.

## Gate

1. Resolve project root (cwd or git root).
2. Require marker file `.hc` (JSON with `version` + `id`).
3. If missing or invalid → stop navigation workflow and follow **hc-index** (`hc init`).
4. Without `.hc`, hooks and `hc_*` tools do not apply.

Languages indexed: Go, Python, JavaScript/TypeScript, Rust.

## Decision tree

```
Need a file's structure?
  └─ hc_file_context("path")     ← always before full Read
       └─ need one body? → Read(path, offset=N, limit=M)  ← both required

Need a directory/package outline?
  └─ hc_package("dir")

Need to find something?
  ├─ know the name? → hc_symbol("name")
  └─ know a pattern? → hc_search("query")

Index health?
  └─ hc_status()
```

Callers / blast radius / call paths → use skill **hc-graph** (`hc_explore`, `hc_impact`, `hc_path`).

## Tools

| Tool | Use when |
|---|---|
| `hc_file_context(path)` | Before Read — symbols + range hints |
| `hc_package(dir)` | Before exploring a folder |
| `hc_search(query, limit?)` | Before Grep — FTS5 over names/signatures/docstrings |
| `hc_symbol(name)` | Exact/prefix lookup + call neighbors |
| `hc_status()` | Check index coverage before exploring |

## Ranged Read rule

After `hc_file_context` / `hc_package`, read only the needed slice:

```
Read("path", offset=N, limit=M)
```

Full Read only when the task needs most of the file (full refactor, line-by-line review, short file). Full Read without offset+limit is blocked by the PreToolUse hook when `.hc` is present.
