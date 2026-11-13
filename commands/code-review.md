---
description: Review uncommitted changes (or a given range/PR) against their plan in docs/plans/ and across contract, Go, SQL, web and security, by severity with file:line and fixes.
argument-hint: "[git range | PR number | path] [docs/plans/<slug>.md]"
---

Review: $ARGUMENTS (if empty: uncommitted changes (git diff HEAD))

1. Find the plan: a `docs/plans/*.md` in the arguments; otherwise the plan file the change edits
   (it ticks the step it implements); otherwise an approved plan whose steps name the changed
   files. If more than one could apply, ask which with AskUserQuestion. A small change in one
   module may have no plan; say so.
2. Use the **code-reviewer** agent on the whole change, with the plan path: the check against the
   plan's acceptance criteria and scope, contract compatibility, module boundaries, cross-cutting
   issues, and which specialists apply.
3. Run the specialists it names, in parallel when they are independent, and give each the plan
   path and the acceptance criteria its files implement: **go-reviewer** (backend Go),
   **database-reviewer** (migrations, queries, repositories), **typescript-reviewer** (web),
   **security-reviewer** (auth, money, personal data, external calls), **seo-specialist**
   (indexable pages).
4. Merge the reports into one list ordered by severity, deduplicated, each finding with
   file:line, the concrete failure and the fix. Start with the plan check (each criterion: met,
   with the test that proves it; not met; or not part of this change). End with a verdict:
   BLOCK / APPROVE WITH NITS / APPROVE.

Report only; don't edit files unless the user asks you to fix specific findings.
