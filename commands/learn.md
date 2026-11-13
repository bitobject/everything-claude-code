---
description: Turn a lesson from this session (a correction, a non-obvious fix, a library quirk, a project convention) into durable project knowledge — a test or lint rule, a .claude/rules file, a skill, or a CLAUDE.md line — after the user approves the exact text.
argument-hint: "[what to capture]"
---

Capture: $ARGUMENTS (if empty: review this session for corrections and non-obvious fixes)

1. Find candidates: places where the user corrected you, a fix took several attempts, a library
   behaved unlike its docs or your expectation, or a convention was discovered.
   Skip one-offs (typos, outages) and anything the code or git history already records.
2. Choose the strongest home for each lesson (a check can't be forgotten; prose can):
   1. **Mechanically checkable** → a test, a `forbidigo`/`depguard` lint rule, a go-arch-lint
      rule, or a hook.
   2. **One area** (a directory or file type) → `.claude/rules/<area>.md` with `paths:` frontmatter.
   3. **One backend module** → its `backend/internal/<module>/CLAUDE.md`, under the template's
      heading for it (invariants, data, dependencies, …).
   4. **Multi-step procedure** → `.claude/skills/<name>/SKILL.md` in the project.
   5. **Short and applies everywhere** → one line in the project's `CLAUDE.md` Conventions,
      in the form "rule — why".
   6. **A decision with trade-offs** → an ADR in `docs/adr/NNNN-<title>.md` (MADR 4.0, from
      `docs/adr/TEMPLATE.md`; the `architect` agent drafts it, the user accepts it).
3. Write it as an instruction with its reason and a pointer to real code (a file or an `Example`
   test), not pasted code. Show the user the exact text and target file; save only after approval.
4. If the lesson contradicts an existing rule, show both and ask which wins; never leave both.
