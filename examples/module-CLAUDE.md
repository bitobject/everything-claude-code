<!-- Template for backend/internal/<module>/CLAUDE.md (ecc plugin: examples/module-CLAUDE.md).

How Claude Code loads it: the root CLAUDE.md loads at launch; this file does not (unless Claude is
started inside this directory). It loads the first time Claude, or a subagent such as a reviewer,
reads, writes or edits a file anywhere under backend/internal/<module>/; a Bash cat or head of a
single file counts as a read. After /compact it comes back only when such a file is touched again.
Block HTML comments like this one are stripped before the text reaches Claude: guidance here costs
no context, the visible text does.

Keep the visible text under about 60 lines. Write only what Claude can't read from the code: why
the boundary exists, rules a signature doesn't show, which data is personal, which dependencies are
allowed. Point to files instead of copying them; copied code drifts.

Write paths in backticks. An at sign followed by a path, outside backticks, is an import: the file
is loaded together with this one (it saves no context), relative paths resolve from this file's
directory, imports nest at most four levels, and a file outside the repository needs a one-time
approval. Nothing here needs an import.

Every module uses the same headings in the same order, because agents and reviewers look for them.
Replace each <hint>; write "none" instead of deleting a section. -->

# <module> module

## Purpose
<One or two sentences in the users' words: the business capability this module owns, and why it is
a module of its own (its own language, a consistency boundary, a different pace of change).>
Subdomain: <core | supporting | generic> (`docs/architecture/subdomains.md`).

## Profile
<full | thin>: <the criterion that decided it>.
<!-- full if at least one holds: a lifecycle or state machine; money or legal obligations;
invariants across several fields or aggregates; several entry points (HTTP, jobs, other modules);
about 10 or more use cases. thin otherwise (reference data, content). The layout of each profile is
in the go-clean-architecture skill; a thin module still has its own schema and its own contract. -->

## Glossary
<Terms of this module that an agent would otherwise name or use differently, one line each:>
- **<Term>** (<Russian term users say>) — <meaning here>; <not to be confused with …>.

## Public API
<!-- The contract package is all that other modules may import from this module: a facade, DTOs
and events. Name them and point to the files; don't copy signatures. -->
- Facade: `contract.<Name>` (`contract/<file>.go`) — <who calls it, for what>.
- DTOs: <the main ones>.
- What callers must know that the signatures don't say: <idempotency, ordering, limits, or none>.

## Events
<!-- Published events carry IDs and facts that never change, never personal data, and change only
by adding fields. Subscribers are wired in cmd, so don't list them here: the context map shows
who listens. -->
- Published (`contract/<file>.go`): <`<Event>` v1 — when it fires>.
- Consumed: <`<other>.<Event>` v1 → `delivery/worker/<file>.go` — what happens; idempotent by <key>>.

## Invariants
<Rules that must always hold, each with where it is enforced and the test that proves it:>
- <Only a new lead can be assigned> — `entity.<Type>.<Method>`; `<TestName>`.

## Data
- Schema `<module>`; tables: <name — what one row is>.
- Personal data: <columns and what they hold (phone, name, problem description), or none>.
- Retention and deletion: <when rows are deleted or anonymized; how a deletion request reaches
  them>.

## Depends on
<!-- Everything this module calls, and nothing else is allowed; it must match the arrows in
docs/architecture/context-map.md and the go-arch-lint configuration. -->
- `<other>` — sync: port `usecase.<Port>`, adapter `gateway/<other>` over `<other>/contract`; <why>.
- `<other>` — events: `<Event>` v1.

## Commands
- Tests of this module: <e.g. `cd backend && go test -race ./internal/<module>/...`>.
- Repository tests need PostgreSQL: <how they get a database here>.
- Boundaries: `cd backend && bin/go-arch-lint check`. Done: `/ecc:verify`.
