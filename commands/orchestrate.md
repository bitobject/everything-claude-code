---
description: Run a multi-agent workflow (feature, bugfix, refactor, security, release) with handoff notes between agents and a final report.
argument-hint: <feature|bugfix|refactor|security|release> <task description>
---

Workflow and task: $ARGUMENTS

| Workflow | Agents in order |
|---|---|
| feature | architect (if boundaries change) → planner → *user confirms* → tdd-guide → go-reviewer / typescript-reviewer → security-reviewer (if auth, money, personal data) |
| bugfix | Explore (find the cause) → tdd-guide (failing test that reproduces it, then the fix) → go-reviewer / typescript-reviewer |
| refactor | architect → planner → *user confirms* → refactor-cleaner → go-reviewer / typescript-reviewer |
| security | security-reviewer → go-reviewer / typescript-reviewer → architect (if the fix is structural) |
| release | code-reviewer (whole diff since last tag) → security-reviewer → database-reviewer (migrations) → seo-specialist (public pages) → /ecc:verify pre-pr |

Rules:
- Each agent gets the task plus the previous agent's handoff:
  ```
  ## HANDOFF: <from> → <to>
  Context: what was asked and decided
  Findings / changes: files touched, tests added
  Open questions: what the next agent must not guess
  ```
- Stop at "*user confirms*" steps and at any open question; don't let agents fill gaps.
- Reviewers that don't depend on each other run in parallel.
- Final report: what changed, test and check results (real output), unresolved findings, and the
  verdict of each reviewer.
