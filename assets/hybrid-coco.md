# hybrid-coco — Local Code Intelligence

Index-based navigation. Same context, fewer tokens.

**Requires `hc init`** → marker `.hc` + shared index at `~/.local/share/hybrid-coco/index.db`. Without `.hc`, hooks and `hc_*` do not apply.

## Skills (repo `skills/` → installed by `hc setup`)

Installed globally to `~/.agents/skills/` and symlinked in `~/.claude/skills/`:

| Skill | When |
|---|---|
| `hc-navigate` | ALWAYS with `.hc` — prefer `hc_*` over blind Read/Grep |
| `hc-graph` | Callers, blast radius, call path before edit |
| `hc-index` | Missing marker, version mismatch, stale/empty index |
| `hc-doctor` | Migrations pending, schema errors, binary outdated, legacy indexes/, post-upgrade repair |

## Quick tree

```
file?  → hc_file_context → Read(offset, limit)
dir?   → hc_package
name?  → hc_symbol / hc_explore
pattern? → hc_search
edit impact? → hc_impact / hc_path
no .hc? → hc init (skill hc-index)
```

Full Read only when the task needs most of the file.
