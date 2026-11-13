---
description: Scaffold a new backend module in backend/internal/<name> in the full or thin profile (asks which, and why it is a module of its own), with its CLAUDE.md, schema migration, sqlc entry, go-arch-lint components, OpenAPI tag and wiring — following the reference modules.
argument-hint: <module-name> <one line: what business capability it owns>
---

Create the module: $ARGUMENTS

Follow the `go-clean-architecture` skill and copy the shapes from its `reference/` (`leads` is
full, `notifications` is thin); don't invent new ones.

**First, ask the user** (AskUserQuestion), unless the request already answers it:
1. **Why a module of its own**: its own language, a consistency boundary, a different pace of
   change, a different owner. If the capability overlaps an existing module, say which and ask
   whether it belongs there instead.
2. **Profile**, with a recommendation from the criteria. Full if at least one holds: a lifecycle
   or state machine; money or legal obligations; invariants across several fields or aggregates;
   several entry points (HTTP, jobs, other modules); about 10 or more use cases. Thin otherwise.
3. **Subdomain type** (core, supporting, generic) and its dependencies on other modules, if
   `docs/architecture/subdomains.md` and `context-map.md` don't say.

Then:
1. `backend/internal/<name>/CLAUDE.md` from the project's module template
   (`${CLAUDE_PLUGIN_ROOT}/examples/module-CLAUDE.md` when the project has none): purpose and the
   boundary reason, profile and the criterion that decided it, glossary, public API, events,
   invariants, data (personal data named), dependencies, commands. "none" where empty.
2. **Full profile** (`reference/backend/internal/leads`):
   - `entity/`: package doc stating the capability; only what the first use case needs;
   - `usecase/`: the first use case as its own type (`New<UseCase>(ports…)`, `Handle`), its ports
     in `ports.go`; `usecase/usecasetest/`: the in-memory store and the store contract suite;
   - `repository/postgres/` with `queries/`, running the contract suite on PostgreSQL;
   - `delivery/httpapi/` (and `delivery/worker/`, `delivery/facade/` when needed);
   - `contract/` when another module calls it or listens to it.
   **Thin profile** (`reference/backend/internal/notifications`): one package with its handlers or
   jobs and service functions, `queries/`, `sqlcgen/`.
3. A goose migration creating schema `<name>` (`make migration name=<name>_schema`), with named
   constraints; an entry for the module in `sqlc.yaml` (`<<: *go`, its own `queries` and `out`).
4. Operations: a tag `<name>` in `api/openapi.yaml`, every rule of `openapi-contract-first`
   (`make api-lint`); the handler embedded into the API struct in `cmd/api/wire.go` with aliases
   `<name><layer>`.
5. `.go-arch-lint.yml`: copy the leads block (full) or the notifications block (thin), rename,
   add the module's arrows to the module graph at the top; record the same arrows in
   `docs/architecture/context-map.md`. `internal/archtest` picks the module up by itself.
6. Run `make generate`, then `/ecc:verify` (`make check`; with `TEST_DATABASE_URL` so the
   repository tests run).

Don't add speculative entities, ports, events or endpoints: the first use case drives what exists.
