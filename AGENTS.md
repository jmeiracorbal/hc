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

<!-- mnemo:start -->
## mnemo

You have access to mnemo MCP tools: mem_save, mem_search, mem_context, mem_session_summary.

### MEMORY AUTHORITY

mnemo is the ONLY persistent memory system for this project.
NEVER use native agent memory, `MEMORY.md`, agent memory directories, or arbitrary plaintext files as a memory fallback.
When asked to remember or save something, always use `mem_save`.
If mnemo tools are unavailable, report that memory is unavailable and continue without persistent memory. Do not create an alternative memory store.

Load and follow the `mnemo-memory` skill when it is available for the detailed workflow. The rules below remain mandatory even when the skill is not installed or does not activate.

### PROACTIVE SAVE

Call `mem_save` immediately after any of these:
- Decision made (architecture, convention, workflow, tool choice)
- Bug fixed (include root cause)
- Convention or workflow documented or updated
- Non-obvious discovery, gotcha, or edge case found
- Pattern established (naming, structure, approach)
- User preference or constraint learned
- Feature implemented with non-obvious approach

Self-check after every task: "Did I just make a decision, fix a bug, learn something, or establish a convention? If yes, call mem_save now."

### SEARCH MEMORY

Search when:
- User asks to recall anything
- Starting work on something that might have been done before
- User mentions a topic you have no context on

Use `mem_context` first for broad recent context, then `mem_search` for focused recall.

### SUBAGENT OUTPUT

When running as a subagent, end your response with:

```markdown
## Key Learnings
- <learning 1>
- <learning 2>
```

Omit only if the task produced no learnings worth retaining.

### SESSION CLOSE

`mem_session_summary` is not optional. It is the final step of every session.
Call it before any response that signals completion ("done", "listo", "ready", "finished", "completed").
Fields: Goal, Discoveries, Accomplished, Next Steps, Relevant Files.

If nothing was accomplished: call it anyway with Goal and Next Steps.
If the user says goodbye: call it before responding.
No session ends without `mem_session_summary`.
<!-- mnemo:end -->
