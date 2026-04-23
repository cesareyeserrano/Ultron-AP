# AGENTS.md — Aitri Pipeline Rules

This project is managed with **Aitri** — an SDLC pipeline CLI.
All agents (Claude, Codex, Gemini, etc.) must follow these rules.

---

## Starting a session

Always run first:

```
aitri resume
```

This gives you the current pipeline state, open requirements, test coverage, and your next action.
Do not start working without it.

---

## During the pipeline

- Follow the **PIPELINE INSTRUCTION** at the end of each Aitri command output exactly.
- Your only next action is the one Aitri specifies. Do not choose an alternative.
- Do not skip phases, re-open approved phases, or implement before Phase 4 is approved.
- Do not write code during Phases 1, 2, 3. These are planning phases.

---

## When the pipeline is complete

If `aitri status` shows all phases approved:

- Run `aitri validate` to confirm deployment readiness.
- Do **NOT** re-open approved phases (`aitri run-phase 1`, etc.).
- Do **NOT** implement new functionality outside the pipeline.

---

## Adding new functionality

Ask: does this change **add, modify, or remove behavior** described in a project artifact
(requirements, design, test cases)?

- **Yes → functional change:** run `aitri feature init <name>`, then follow the feature pipeline.
- **No → minor change** (typo, style tweak, config value): implement directly without the pipeline.

When in doubt, treat it as functional.
