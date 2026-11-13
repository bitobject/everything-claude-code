---
description: Restate the request, surface open questions and risks, and write an ordered test-first plan to docs/plans/<slug>.md (testable acceptance criteria, scope, small steps that each merge). Waits for your "go" before any code is written.
argument-hint: <what to build or change>
---

Use the **planner** agent to plan: $ARGUMENTS

1. If the request needs a design decision (new module, cross-module flow, data model,
   infrastructure), use the **architect** agent first. It writes its proposal into the project:
   an ADR in `docs/adr/` with `status: proposed`, and `docs/architecture/` or a module's
   `CLAUDE.md` where the design changes them. The plan links the ADR.
2. The planner reads `STACK.md`, the `go-clean-architecture` skill, `docs/architecture/`, the
   `CLAUDE.md` of the modules involved and the closest existing example in the codebase, then
   writes the plan to `docs/plans/<slug>.md` with `Status: draft`. A file rather than a chat
   message: you can read and edit it, a changed plan shows as a diff, it survives compaction and
   new sessions, and reviewers check the change against it. A draft is not code: nothing is built
   from it until you approve it.
3. If anything the plan depends on is undecided, ask with AskUserQuestion (concrete options,
   recommendation first) instead of assuming. Record each answer under "Decisions" in the plan
   file and let the planner finish the steps.
4. Show the path, the acceptance criteria and the steps, and stop. Write no code until the user
   replies "go" or approves a changed plan. Then put the changes into the file first, set
   `Status: approved <YYYY-MM-DD>`, and set the linked ADRs the user agreed to `status: accepted`.
5. After "go", one step per change: `/ecc:tdd docs/plans/<slug>.md step <n>`, then `/ecc:verify`
   and `/ecc:code-review` (it checks the change against the plan's acceptance criteria). The change
   ticks its step's box. If the plan turns out wrong, stop, update the plan and get the user's OK
   for the change before going on.
