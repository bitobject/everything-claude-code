# Workflow (all files)

Loaded every session; keep it short. Plugin commands are namespaced `/ecc:<name>`.

- **Non-trivial work** (new behavior, a contract change, several files): `/ecc:plan` first (it writes
  `docs/plans/<slug>.md`) and wait for the user's "go". Design questions (boundaries, data model, new
  dependency): `architect` agent; hard-to-reverse decisions get an ADR in `docs/adr/`.
- **Small batches**: about 100 changed lines (generated code aside) is a normal change; cut work so
  each step can merge to main within a day; unfinished work stays behind a flag.
- **No guessing.** If the request, docs or code don't say how something should behave, ask with
  concrete options and a recommendation. Record minor choices you made in an "Assumptions" section.
- **Check current versions** before recommending a library, tool or language feature
  (`go list -m -versions`, `npm view <pkg> version`, release notes). Never from memory.
- **Library behavior is read, not assumed**: `go doc <pkg>.<Symbol>` / `-src`, the module cache,
  official docs (context7) before relying on what pgx, River, goose, Next.js or React does.
- **Tests first** (`/ecc:tdd`); a bug fix starts with a failing test that reproduces it. Existing
  test expectations change only when the requirement changes.
- **Done** = `/ecc:verify` green, then `/ecc:code-review` for anything non-trivial. Report real
  output; never weaken a check to pass it.
- **Git**: one concern per change; conventional commit messages (`feat(leads): assign lead`);
  never commit secrets, `.env`, dumps or generated data from production; never `--no-verify`;
  push and open PRs only when asked.
- **Personal data**: never request, paste or use real client data (DB dumps, logs, screenshots);
  generate test data.
- **Hard-to-undo actions** (pushing, deleting data, migrations on shared databases, deploys, CI
  changes): ask first.
- When the user corrects something that could recur: `/ecc:learn`.
