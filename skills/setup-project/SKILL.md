---
name: setup-project
description: Install this plugin's rules, the CLAUDE.md templates (project root and one per backend module) and the starting documents (plan and ADR templates, an architecture starting point) into the current project, so they load automatically and are versioned with the code. Run once per repository, and again after the plugin's rules or templates change.
disable-model-invocation: true
argument-hint: "[project | user]"
---

# Set up a project for the ecc plugin

Plugins can't ship always-on rules or project documents, so this copies them. Source files,
relative to this skill's base directory: `../../rules/*.md`, `../../examples/project-CLAUDE.md`,
`../../examples/module-CLAUDE.md`, `../../examples/web-CLAUDE.md` and everything under
`../../examples/docs/`.

Target: $ARGUMENTS (if empty: `project`)
- `project` → the rules into `<repo root>/.claude/rules/` (shared with the team through git).
  Recommended.
- `user` → the rules into `$CLAUDE_CONFIG_DIR/rules/` (or `~/.claude/rules/`): only this user,
  every project.

The CLAUDE.md files and the documents always belong to the current repository, whatever the target.

Never overwrite silently: when a target file exists and differs, show the diff and ask whether to
replace, merge or keep it.

Steps:
1. Find the repository root (`git rev-parse --show-toplevel`; if not a git repo, the current
   directory) and list what already exists: the target `rules/` directory, `CLAUDE.md`, `docs/`
   and `backend/internal/*/CLAUDE.md`.
2. Rules: for each plugin rule (`workflow.md`, `go.md`, `sql.md`, `openapi.md`, `typescript.md`),
   copy it if the target file doesn't exist, otherwise compare as above. Adjust `paths:` globs if
   the project's layout differs from `backend/`, `web/`, `api/`.
3. Root `CLAUDE.md`: if the project has none, offer `../../examples/project-CLAUDE.md` as a starting point
   and fill the placeholders with the user (project name, modules, DSN for tests, ports). If one
   exists, show which template sections it lacks; don't rewrite it.
   If the repository has `web/` and no `web/CLAUDE.md`, offer `../../examples/web-CLAUDE.md` as
   `web/CLAUDE.md`: it imports `web/AGENTS.md` (`@AGENTS.md`), which create-next-app writes and
   `next dev` keeps current, and which Claude Code doesn't read on its own while the root
   `CLAUDE.md` exists.
4. Documents, from `../../examples/docs/` into `<repo root>/docs/` (with the `user` target, ask
   first):
   - `plans/TEMPLATE.md`: the plan format; `/ecc:plan` writes plans next to it as
     `docs/plans/<slug>.md`.
   - `adr/TEMPLATE.md` (MADR 4.0) and `adr/0000-record-architecture-decisions.md`. In ADR 0000 set
     `date` to today and `decision-makers` to the owner: it records that this project's accepted
     ADRs win over the plugin's `STACK.md`.
   - `architecture/`: `README.md`, `subdomains.md`, `context-map.md`, `c4.md`, `glossary.md`. They
     are a starting point pre-filled with what the plugin knows. Replace `<product>`, then go
     through each item marked *proposed* and each open question with the user (AskUserQuestion,
     recommendation first), write the answers in, and remove the markers they settle. Questions
     the user leaves open stay in the files and go in the report; they don't block the setup.
5. Module `CLAUDE.md`: for each module in `backend/internal/` (not `kernel`, `platform` or `gen`)
   that has no `CLAUDE.md`, offer one from `../../examples/module-CLAUDE.md`. Fill it from the
   module's code, `docs/architecture/` and the user's answers; keep the template's headings in
   their order; move the module's terms out of `docs/architecture/glossary.md` into it. Tell the
   user how it loads: not at launch from the repository root, but the first time Claude or a
   subagent reads, writes or edits a file under that module's directory. A module designed later
   gets its file from the `architect` agent.
6. Suggest (don't create without approval): `backend/.golangci.yml`, `backend/.go-arch-lint.yml`
   and `backend/.squawk.toml` from `../go-clean-architecture/reference/backend/`; the `Makefile`
   targets, `tools/tools.lock`, `scripts/` (installing and checking the pinned tools) and the
   `.gitignore` line for `backend/bin/` from `../go-clean-architecture/reference/`. Before copying
   `tools.lock`, check for newer releases under its 14-day policy.
7. Report what was copied, skipped or left for the user to decide, including the open questions
   that remain in `docs/architecture/`.
