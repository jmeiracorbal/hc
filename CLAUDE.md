# hybrid-coco — Agent Rules

## Session Protocol (mandatory)

**1. Load context before any work:**
```
mem_context project=hybrid-coco
```

**2. Save after any significant decision:**
```
mem_save title="..." type="decision|architecture|bugfix" project=hybrid-coco
```

**3. Summarize at session end:**
```
mem_session_summary project=hybrid-coco
```

Do NOT put volatile state in this file. Use Engram.

---

## Architect / Orchestrator / Subagent Model

- **Architect** (human): defines phases, reviews, corrects course
- **Orchestrator** (Claude, main session): reads `.claude/context/plan.md`, delegates to subagents, saves to Engram
- **Subagents**: implement specific phases — they read Engram context + this file only

Before delegating: check Engram for prior context on that phase.

---

## Non-Negotiable Constraints

- SQLite only — no PostgreSQL, no Docker, no server infrastructure
- FTS5 + tree-sitter before any embedding/vector layer
- Go only — single `hc` binary (CLI + MCP). No Python runtime
- Install: `curl -sSf …/install.sh | bash && hc init` — two commands, done
- MCP server configured for Claude Code (`.claude/settings.json`), NOT `claude_desktop_config.json`
- tool names `hc_*` must never be truncated

---

## Context References

- **Roadmap + phase status**: `.claude/context/plan.md`
- **Phase specs and state**: `.claude/context/phases/phase-0X.md`
- **Architecture decisions**: `.claude/context/decisions/adr-XXX.md`
- **Component docs**: `.claude/context/components/`
- **Live progress**: Engram — `mem_search project=hybrid-coco`

@AGENTS.md

<!-- mnemo:claude-start -->
### MEMORY SYSTEM

The Claude Code file-based memory system that writes under `~/.claude/projects/*/memory/` and maintains `MEMORY.md` is DISABLED for this workspace.
Never use it or any plaintext file as a fallback. The mandatory memory authority and fallback behavior are defined in `AGENTS.md`.

### RECOVER CONTEXT

Use `mem_context` when:
- A new session starts in a project you have worked on before
- The context window was just compacted (PostCompact hook fires this automatically)
- You need a broad overview of recent session history before acting

`mem_context` returns the most recent observations and session summaries for the project. Use it to orient yourself at the start of a session before doing any significant work.
<!-- mnemo:claude-end -->
