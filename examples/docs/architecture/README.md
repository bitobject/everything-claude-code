# Architecture

> Starting point copied from the ecc plugin by `/ecc:setup-project`. Items marked *proposed* and
> the open questions are guesses to confirm with the team; remove a marker once it is settled.

What was decided about this system, kept next to the code. The ecc plugin's `STACK.md` holds the
default decisions; where this project differs, an accepted ADR in `../adr/` says so and wins
(ADR 0000).

| File | Answers | Update when |
|---|---|---|
| `subdomains.md` | What the business consists of, which part is core, which module owns what, which profile it gets | a capability appears, moves or changes importance |
| `context-map.md` | Which module depends on which, and how | a cross-module call, event or copy of data appears or goes |
| `c4.md` | People, external systems, containers and how they talk | one of them appears or changes |
| `glossary.md` | One name per concept, in Russian and in code, per module | a term appears; a module is created (its terms move to its `CLAUDE.md`) |
| `../adr/` | Why each hard-to-reverse decision was made (MADR 4.0) | each such decision |
| `../plans/` | What a change will do: acceptance criteria, scope, steps | each non-trivial change |

- Each backend module also has `backend/internal/<module>/CLAUDE.md`: purpose, profile, glossary,
  public API, events, invariants, data, dependencies and test commands. Claude Code loads it when
  Claude or a subagent touches a file under that module's directory, so the module's rules arrive
  where they are needed and cost nothing elsewhere.
- These files record decisions; `docs/codemaps/` (`/ecc:update-codemaps`) describes what the code
  does. When the two disagree, either the code is wrong or a decision was never written down.
- The `architect` agent updates these files and drafts ADRs as part of a design; a person approves
  them. Keep each file short: a diagram, a table, the open questions.
