# Backlog conventions — Ultron-AP

How we use `aitri backlog` and `aitri bug` for non-feature work.

## Priority

Priority is declared by the author at `aitri backlog add --priority`. Not
auto-calculated.

| Prio | When to use | Examples |
|------|-------------|----------|
| P0 / P1 | **Don't use here.** Active blockers and live bugs go through `aitri bug add` directly, not the backlog. | — |
| **P2** | Real risk visible to a user or to operations: memory/data loss under realistic Pi conditions, security gaps that an authenticated actor could exploit, or features with concrete demand. Worth scheduling dedicated time. | BL-002 (stream-encrypt backup → OOM on large DB), BL-005 (backup path validation, closed), BL-011 (helper log redaction, closed), BL-020 (settings revamp) |
| **P3** | Quality / hardening that improves the system but isn't visible today: silent error logging, minor unbounded growth, defensive hardening, observability debt. Batched in rounds when there's slack. | BL-003, BL-004, BL-006, BL-012, BL-015, BL-021 |

## Backlog vs. bug

Use `aitri bug add` when the issue is a defect against an existing FR
that the user can hit today (whether reported or self-discovered).
Use `aitri backlog add` for everything else: hardening that has no
acceptance-criteria match, refactor proposals, feature ideas before
they are scoped.

A backlog item often spawns a bug when scheduled — see the BL-005 →
BG-025 flow: register the bug, fix, `bug fix <sha>`, `bug verify`,
then `backlog done`.

## Sequencing

When picking the next item, prefer:

1. **Discovery before build** — analysis tasks (e.g. UI audits, security
   reviews) that unblock larger features should run before the feature
   they inform. BL-019 ran before BL-020 for this reason.
2. **Batch by theme** — security hardening items (BL-005 + BL-011) ship
   together so verify-run cycles and review context aren't paid twice.
3. **Cost vs. blast radius** — small, well-scoped P3s can be cheaper to
   batch in one round than one P2 that takes a full feature pipeline.

## Closing

```
aitri backlog done <BL-NNN>
```

If the work was a bug, run `aitri bug verify <BG-NNN>` first so the
bug closes cleanly, then close the backlog item.
