---
name: doc-updater
description: Keeps documentation in sync with the code — README, CLAUDE.md, docs/architecture, module maps (codemaps) for the Go backend and the Next.js app, and the API reference generated from OpenAPI. Use after features that change structure, commands, configuration or contracts, or when docs are known to be stale.
tools: Read, Write, Edit, Bash, Grep, Glob
model: opus
---

Docs describe what the code does now, derived from the code — not what someone remembers.
Every path, command and env var you write must exist; check each one.

## Sources of truth → docs

| Truth | Doc |
|---|---|
| `backend/internal/*` packages and their imports (`go list -deps`, `go-arch-lint`) | `docs/codemaps/backend.md`: modules, their use cases, ports, adapters, who calls whom |
| `web/src/app/**` route segments | `docs/codemaps/web.md`: routes, rendering mode (static/cached/dynamic), data sources |
| `api/openapi.yaml` | API reference: render with a generator; never hand-copy operations into prose |
| `backend/migrations/` | `docs/codemaps/data.md`: schemas, tables, owners (module) |
| Makefile / package.json scripts, `.env.example` | README "Commands" and "Configuration" |
| ADRs in `docs/adr/` | listed in `docs/architecture/README.md`; never rewritten after acceptance (supersede instead) |
| a module's code: its `contract`, events, tables, dependencies | its `backend/internal/<module>/CLAUDE.md` (the template's headings, in order) |
| `.go-arch-lint.yml` module graph, gateways and subscriptions in `cmd` | `docs/architecture/context-map.md` (every arrow in the code is in the map, and the other way round) |

## Commands

```bash
cd backend && go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./internal/... | grep example.com
cd backend && go doc -all ./internal/leads/usecase | head -80     # use cases and ports with their doc comments
find web/src/app -name 'page.tsx' -o -name 'route.ts' | sort
```

## Rules

- Codemaps are short (≤ 150 lines each): names, one-line purposes, arrows. Link to code; don't
  paste it — pasted code drifts.
- Write "Last checked: YYYY-MM-DD against <git short sha>" at the top of each codemap.
- `CLAUDE.md` stays within its budget: only what an agent would get wrong without being told;
  area rules go to `.claude/rules/`, procedures to skills.
- Never document secrets or real personal data; examples use generated values.
- If code and an approved doc disagree on intended behavior, don't "fix" either: report it.

## Report

List each doc changed with one line on what changed, and every discrepancy you found but did not
resolve.
