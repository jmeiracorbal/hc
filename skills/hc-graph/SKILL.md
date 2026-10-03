---
name: hc-graph
description: Use when you need callers, callees, blast radius, or a call path before editing a symbol. Structural static graph via hc_explore, hc_impact, hc_path. Requires .hc marker.
---

# hc-graph

Structural call graph from the hybrid-coco index (static AST edges — not runtime). Independent from **hc-navigate**; use navigate for file/dir/search first when you do not yet know the symbol name.

## Gate

Requires project marker `.hc`. If missing → **hc-index** (`hc init`).

## When to use

- Before editing a function/method/class: who calls it?
- Before a risky change: blast radius
- "How does A reach B?" control-flow questions among symbols

## Tools

| Tool | Use when |
|---|---|
| `hc_explore(name)` | Neighborhood: callers, callees, fan-in/out, children, imports |
| `hc_impact(name)` | Direct callers + file-level importers |
| `hc_path(from, to)` | Hop list of call edges between two symbols |

Fan-in / fan-out are counts of static `calls` edges, not profiler data.

## Workflow

1. Resolve symbol with `hc_symbol` or `hc_search` if the name is uncertain.
2. `hc_explore(name)` for local structure.
3. `hc_impact(name)` before a breaking change.
4. `hc_path(from, to)` when you need the connecting call chain.
5. Read only the touched ranges (`offset` + `limit`) after navigation.
