<!-- Independent reviewer's report, 2026-11-13: Building large systems: modular monolith, strategic design, workflow with AI agents.
     Raw model output, kept for its sources and detail. Which claims were re-verified is stated in
     ../2026-11-13-best-practices-review.md (sections marked [проверено] / [опровергнуто]). -->

# Review: building large systems (modular monolith, strategic design, testing, AI workflow)

**Assumptions.** This is a read-only review of the plugin as it stood on 2026-11-13 at 00:00. STACK.md, the go-clean-architecture skill, the Makefile, verification-loop and examples/CLAUDE.md changed while I was reviewing. I re-read all of them, and the module, testing and workflow rules I critique did not change. I ran nothing. I assume `.localenv/` is not committed to the product repo, and that partners will call the API (the openapi skill already plans an `Idempotency-Key` for them).

## 1. Verdict

The big calls match current practice:
- modular monolith first, with modules cut by business capability and layers inside each module
- one PostgreSQL schema per module
- ports declared by the caller, implemented by a mapping gateway
- jobs enqueued in the same transaction as the state change
- the dependency rule enforced as an executable check
- fakes instead of mocks
- a compiling reference module
- short instruction files and rules scoped by path

The weak area is everything *between* modules:
- No module has a published API.
- The documented asynchronous path can't be built as the linter is configured.
- There is no policy for cross-module reads, copied data, consistency or extracting a module.

These are the places where modular monoliths erode as they grow. The test pyramid has the right shape but lacks module-level tests and contract tests for fakes. There is no strategic-design step, so every module gets the heaviest style. The agent workflow is careful but almost entirely advisory. Current Claude Code guidance and DORA research say the done-gate, the guardrails and human sign-off must be enforced by tools: CI, branch protection, hooks, permission rules and review tiers by risk. They also say plans should be files.

## 2. Confirmed

- **Monolith first; split a service out only for a measured reason** (architect.md). Matches Fowler, *MonolithFirst*, and Grzybek.
- **Modules = business capabilities, layers inside, top-level layering rejected.** Matches Simon Brown's "package by component" and Spring Modulith.
- **Schema per module, no foreign keys or table reads across modules** (sql.md, database-reviewer). Matches Grzybek (2020), Jovanović (2025) and Vernon's "reference other aggregates by identity".
- **Caller-side port plus a mapping gateway.** This is the caller's anti-corruption layer (ACL), as Grzybek describes it.
- **River `InsertTx` as a transactional outbox.** Same guarantee as Spring Modulith's event publication registry.
- **go-arch-lint, squawk and tools-check inside `make check` as fitness functions.** Matches *Building Evolutionary Architectures* (2nd ed.). Shopify (2024) credits tooling with "holding the line" on dependencies.
- **Hand-written fakes, assertions on outcomes, real PostgreSQL through template databases.** Matches *Software Engineering at Google* ch. 13 ("Interaction testing should be avoided") and River's testing docs (template clone in about 100 ms).
- **Responses validated against the spec, clients generated from it, no Pact.** Right for one repo with one spec (my judgment).
- **Compiling reference module.** Thoughtworks Radar Vol. 35 lists "Anchoring coding agents to a reference application" in Trial (Nov 2026).
- **CLAUDE.md budget, path-scoped rules, skills, "mechanically checkable first".** Matches the Claude Code docs (CLAUDE.md under 200 lines) and the Radar's "Progressive context disclosure" (Adopt). Copying rules via setup-project is also correct: the plugin docs say a plugin's CLAUDE.md "isn't loaded".
- **Plan, then ask instead of guessing, then red/green; never weaken tests; report real output.** Matches the Claude Code best practices, Kent Beck (2025, "cheating" by deleting tests is a warning sign) and Anthropic's long-running-harness article ("unacceptable to remove or edit tests").
- **Read-only reviewers with a fresh context; hooks for generated code, with tests for the hooks.** Matches the Claude Code docs and the Radar's "Harness quality evaluation".

## 3. Problems

**P1. The asynchronous cross-module path cannot be built as specified.**
- *Where:* STACK.md l.67–69; go-clean-architecture "Between modules" l.156–157; rules/go.md l.17–18; `.go-arch-lint.yml` delivery dependencies.
- *We say:* A enqueues a River job in its transaction and "B's worker reacts".
- *Evidence:*
  - The job type lives in A's `usecase`. B's `delivery/worker` may not import it, so B has to re-declare the struct by convention.
  - River identifies a job by its kind, and `AddWorker` panics on a duplicate kind. So billing and notifications cannot both react to "lead assigned" unless A enqueues one job per consumer. Then A knows its consumers: that is a command, not an event.
  - Grzybek: integration events "are part of the contract" of the module that publishes them.
  - Spring Modulith stores one publication entry per listener inside the original transaction and retries each one independently.
- *Severity:* high. *Confidence:* high.
- *Change:*
  - A `contract` package per module holding its integration events: kind, version, IDs plus facts that never change, no personal data.
  - A small dispatcher in `platform` that maps each event to its subscribers' job kinds and inserts them with `InsertManyTx` in the same transaction.
  - Subscribers' workers import only the publisher's `contract`.
  - Event changes are additive only, with a golden-JSON test per event and an example in `reference/`.

**P2. No published module API and no module dependency graph.**
- *Where:* STACK.md l.61; skill l.77–78; `.go-arch-lint.yml` l.75–77; go-build-resolver l.54.
- *We say:* `gateway/<other>` imports `<other>/usecase`.
- *Evidence:*
  - That package is the other module's whole application layer: its interactor, ports, job types, and user-facing use cases that take an `auth.Principal` (security-review l.15). A call from another module has no user principal to pass.
  - Nothing prevents a leads↔lawyers cycle. go-build-resolver even recommends a gateway as the fix for "two modules import each other".
  - Spring Modulith verifies access "via API packages only" and "no cycles… directed acyclic graph".
  - Grzybek: "private by default", the callee exposes a facade.
  - Khononov (2024): depend on contracts built for the integration, especially toward volatile core modules.
  - Shopify dropped packwerk's privacy checks because a linter cannot design an API. That argues against the tool, not against having an API.
- *Severity:* high (cheap now, costly to retrofit). *Confidence:* high.
- *Change:*
  - Add `internal/<m>/contract`: a deep facade for internal callers, plus DTOs and events.
  - In go-arch-lint, let gateways and workers depend only on `<m>-contract`.
  - Write the allowed module DAG into the docs and into the lint config.
  - Optionally get compile-time privacy from Go `internal/` directories.

**P3. One implementation style for every module, and no strategic design step.**
- *Where:* STACK.md l.34 ("a bounded context"); new-module.md (always six components); architect.md "Principles".
- *Evidence:*
  - Khononov (*Learning DDD*, ch. 10): subdomain type and complexity choose the business-logic pattern (transaction script, active record or domain model), the architecture and the test shape.
  - Three Dots Labs, which the skill cites, says "not every project is complex enough to warrant advanced techniques like DDD".
  - Ousterhout: layers that only pass data through are shallow modules.
  - Jovanović (2026): "A module boundary is a guess"; "Splitting a coarse module later is cheap, and merging two is the expensive direction."
- *Severity:* medium. *Confidence:* medium (contested area).
- *Change:*
  - Classify subdomains. Likely core: lead routing and matching, lead pricing and sale. Supporting: lawyer profiles, content catalogue. Generic: identity, payment provider, SMS and email.
  - Offer two module profiles under the same dependency rule: full layers, or `usecase` + `repository` with no entity package.
  - Have `/ecc:new-module` ask which profile, and what justifies the boundary (its own language, a consistency boundary, different volatility).

**P4. Nothing forbids cross-module calls inside a transaction.**
- *Where:* skill "Transactions"; postgres-patterns l.213 and l.295, which forbid only I/O in `Update` and calls to external services in `Do`.
- *Evidence:* the transaction holds one pgxpool connection while the other module acquires a second from the same pool. Under load every connection is held by a caller waiting for a second one. HikariCP calls this pool-locking: a deadlock-free pool needs `Tn × (Cm − 1) + 1` connections. Row locks are also held across the other module's code.
- *Severity:* medium. *Confidence:* high.
- *Change:* read other modules' data before `Do`/`Update`, never call a gateway inside them, and send cross-module writes through events.

**P5. "A module reads only its own schema" is enforced only by reviewers.**
- *Where:* sql.md l.11; STACK.md l.81.
- *Evidence:*
  - Every module's sqlc entry uses `schema: migrations`, so it sees all modules' tables. A cross-schema query compiles and passes `make check`.
  - Jovanović (2025): "Enforce data boundaries in the database, not only in code", with one database role per module that has privileges only on its own schema.
- *Severity:* medium. *Confidence:* high.
- *Change:* add a `make check` step that rejects other modules' schema names in a module's `queries/*.sql`. Later, add per-module roles, used at least in repository tests via `SET ROLE`.

**P6. The test pyramid lacks the tests that catch this architecture's bugs.**
- *Where:* STACK.md l.95–105; golang-testing; tdd-workflow.
- *Evidence:*
  - *SWE at Google* ch. 13: "A fake must have its own tests", and contract tests run "against both the real implementation and the fake".
  - Fowler (2021): shape debates are "a distraction".
  - The honeycomb and trophy models, and Spring Modulith's module tests and Scenario API, all favour integration tests for database-centric code.
  - Here, use-case tests run on fakes (including a fake UnitOfWork) and repository tests cover adapters alone. Transactions, locking, `InsertTx` and mapping are only tested together by Playwright.
- *Severity:* medium. *Confidence:* high.
- *Change:*
  - One contract suite per port, run against both the fake and the pgx adapter.
  - A few module tests per use case on real PostgreSQL, using `rivertest.RequireInsertedTx`.
  - Cross-module flow tests with `rivertest.NewWorker` once two modules talk to each other.
  - Lean more on integration tests for simple modules.

**P7. API compatibility is a policy, not a check.**
- *Where:* openapi.md l.18–20; code-reviewer "Contract".
- *Evidence:*
  - The consumers are the separately deployed Next.js app and partners.
  - `oasdiff breaking` detects these changes mechanically and is actively maintained (v1.37.0, 2026-11-08; its README lists 3.1 support).
  - River job arguments are persisted JSON that jobs still in the queue must decode across deploys.
- *Severity:* medium. *Confidence:* high.
- *Change:*
  - Run oasdiff against the main branch in CI. Pin a release at least 14 days old, e.g. v1.35.0.
  - Add golden-JSON tests for job and event arguments.
  - Extend "never rename a Kind" to the fields of a deployed kind.

**P8. The definition of done and the guardrails are advisory.**
- *Where:* workflow.md "Done" and "Hard-to-undo"; examples/CLAUDE.md "Data protection"; the Stop hook checks only for debug output; no CI is defined anywhere.
- *Evidence:*
  - Claude Code best practices: "If something has to happen every time, make it a hook, a deny rule, or a managed setting". The last word belongs to "Branch protection, a person who approves the merge".
  - Radar Vol. 35: "Human-in-the-loop theater" (Caution) and "Risk-tiered human oversight" (Trial).
  - DORA (2026): AI raises both throughput and instability.
- *Severity:* high. *Confidence:* high.
- *Change:*
  - CI runs `make check`, `pnpm check` and oasdiff, with branch protection on the main branch.
  - Have setup-project install `.claude/settings.json`:
    - deny `Read(./.env*)`
    - ask before `git push` and migration commands
    - a PreToolUse hook that asks before edits to `.go-arch-lint.yml`, `.golangci.yml`, `.squawk.toml`, applied migrations, and test deletions
  - A Stop hook that runs a fast gate: build, vet, arch-lint.
  - Human approval required for the money, personal-data, migration and contract zones.
- *Two gate details:*
  - `-shuffle=on` is not in Go's list of cacheable test flags, so every `make check` reruns every integration test. Keep shuffle for CI or nightly runs.
  - `go mod tidy -diff` is in verification-loop l.32 but missing from `make check`, which that same skill says to run "not a subset".

**P9. Plans live only in chat, and reviews aren't anchored to them.**
- *Where:* planner.md (tools Read/Grep/Glob; ends with "Reply 'go'"); tdd.md, which delegates implementation to a subagent.
- *Evidence:*
  - Claude Code docs: write `SPEC.md`, build from it in a fresh session, and review against it, reporting correctness gaps and skipping style. A reviewer asked to find problems "will always find *something*", which leads to "extra abstraction layers".
  - Anthropic's harness article (2025): a feature list, a progress file, and one feature at a time.
  - Beck (2025) drives the agent from a `plan.md`.
  - Böckeler (2025): spec-first is valuable, but heavy spec tooling for small changes is "a sledgehammer".
- *Severity:* medium. *Confidence:* high.
- *Change:*
  - The planner writes `docs/plans/<slug>.md`: acceptance criteria as testable behaviours, a checklist, decisions made.
  - tdd-guide ticks items off as it goes; reviewers receive the plan.
  - Small, low-risk changes in one module skip the plan.

**P10. The architecture's source of truth sits outside the project.**
- *Where:* STACK.md l.3–4; examples/CLAUDE.md l.4–5 and l.13; setup-project copies the rules but not STACK.md.
- *Evidence:*
  - ADRs (Nygard; MADR 4.0, 2024) keep decisions versioned with the code.
  - Radar Vol. 35 warns that reference templates drift away from the real code.
- *Severity:* medium. *Confidence:* medium.
- *Change:* seed `docs/architecture.md` and ADRs in the repo from STACK.md, and make the plugin defer to them. Once a real module exists, make it the canonical example and keep `reference/` for bootstrapping only.

## 4. Gaps

- **Strategic-design docs that agents load on demand** (now).
  - A subdomain map, a context map (which module is upstream, ACL versus conformist), a glossary per module, and C4 context and container diagrams. c4model.com says those two "are sufficient for most software development teams".
  - A `CLAUDE.md` in each module directory, covering purpose, invariants, contract, events published and consumed, and allowed dependencies. Claude Code loads nested CLAUDE.md files only when it touches files there.
  - MADR's "Confirmation" field in ADRs, linking each decision to the test or lint rule that enforces it.
  - An owner and review zone per module (CODEOWNERS). If a second team appears, give it whole modules (Team Topologies, 2nd ed., 2025).
- **A policy for data across modules** (now).
  - Name an owner for reference data. Cities and legal topics drive SEO pages, lead routing and pricing; pick one of Newman's options: an owning module, a dedicated schema, or a static library.
  - Reads: a contract call for a single item; for lists and joins, a projection kept by the consumer and fed by events, a read-only view published by the owner, or composition in Next.js Server Components.
  - Copies are read-only, or snapshots such as the price at sale.
  - Personal data is never copied. Events and job arguments carry IDs, and deletion requests propagate as events (152-FZ).
- **Consistency across modules** (now, before billing). No transaction spans two modules. Selling a lead means assigning it and debiting the lawyer's balance; that flow either lives in one module or runs as a process manager on River with idempotent steps and compensation. Jovanović (2026): consistency requirements decide where the boundary goes.
- **Small batches and frequent integration** (now).
  - A size budget per change. Google: "100 lines is usually a reasonable size… 1000 lines is usually too large". Mark generated files so reviews skip them.
  - Merge to trunk "at least once a day" (DORA).
  - Unfinished features behind flags.
  - DORA's AI capabilities model names small batches and version-control practices among its seven capabilities.
- **Parallel agent work** (later).
  - A worktree per task (`claude -w`) and `/batch` for mechanical changes.
  - Goose's sequential numbering (postgres-patterns l.348–349) collides across parallel branches. Goose recommends timestamps during development and `goose fix` in CI.
  - Split `api/openapi.yaml` per module and bundle it before generation.
  - For unattended runs, the Claude Code docs' five checks: what done looks like, how it checks itself, what it may touch, what stops it, who signs off.
- **An extraction path** (later). Document what changes when a module becomes a service: gateway → a client generated from the module's contract, dispatcher → a broker relay, schema → its own database and role, migrations → their own directory.
- **Evaluating the harness itself** (later). `claude plugin eval` (early access) with a no-plugin baseline, for key behaviours: a cross-module call produces a port and a gateway; a job uses `InsertTx`; an ambiguous request gets a question. Run `/doctor prompt-audit` on each model upgrade. Radar: "Harness quality evaluation" (Assess), "Agent instruction bloat" (Caution).
- **Mutation testing** (later). Radar Vol. 35 puts it in Adopt as a guard against AI-written tests that stay green whatever the code does. Go tooling is immature (gremlins' last release was May 2024; avito-tech/go-mutesting has no tagged releases), so run it nightly on `entity`/`usecase` and treat it as advisory.

## 5. Over-engineering

- **Full layers, three hand-written mappings and fakes for CRUD-like modules (P3).** More code to review means a bigger "verification tax" (DORA, 2026) for little protection.
- **Review fan-out.** code-reviewer plus up to five specialist reviewers, all 14 agents on `model: opus`, and plan approval for any "new behavior". The docs: "Handing the same task to a subagent doesn't make the answer any better". Use cheaper models for mechanical agents, specialists only in their zones, and approvals by risk tier.
- **`/ecc:orchestrate`.** Prose handoffs between sequential subagents. Claude Code now has saved, resumable dynamic workflows and agent teams; for one feature, a single session plus a fresh-context review is enough.
- **Leftovers from the fork:**
  - strategic-compact documents the `tool == "Edit"` matcher that the plugin's own README says current Claude Code rejects. Its "manage context aggressively" advice is on the docs' "What you can stop doing" list.
  - eval-harness and `/ecc:eval` measure pass@k on deterministic features; the tests already are those evals.
  - `/ecc:checkpoint` reports coverage deltas, contradicting "no percentage targets".
  - `contexts/` is not a plugin component and is never loaded.

## 6. Sources

- martinfowler.com/bliki/MonolithFirst.html — Fowler, 2015
- kamilgrzybek.com: modular-monolith-integration-styles (2020-07-26); modular-monolith-architecture-enforcement (2020-03-22)
- shopify.engineering/a-packwerk-retrospective — McGibbon, Salzberg, 2024-02-07
- docs.spring.io/spring-modulith/reference/ (fundamentals, verification, events, testing) — Spring Modulith 2.1.1
- files.gotocon.com/…/gotoberlin2018-modular-monoliths.pdf — Simon Brown, 2018; *Clean Architecture* ch. 34, 2017
- *Learning Domain-Driven Design* — Khononov, O'Reilly, 2021; coupling.dev/posts/core-concepts/balance/ — Khononov, 2024–2026
- threedots.tech/post/ddd-lite-in-go-introduction/ — Three Dots Labs, 2020-07-01
- github.com/johnousterhout/aposd-vs-clean-code — Ousterhout and Martin, 2024–2025
- milanjovanovic.tech: how-to-keep-your-data-boundaries-intact-in-a-modular-monolith (2025-08-02); the-modular-monolith-boundary-i-couldnt-take-back (2026-06-20)
- *Monolith to Microservices* — Sam Newman, O'Reilly, 2019; *Effective Aggregate Design* — Vaughn Vernon, 2011
- github.com/brettwooldridge/HikariCP/wiki/About-Pool-Sizing — HikariCP wiki
- abseil.io/resources/swe-book/html/ch11.html and ch13.html — Winters, Manshreck, Wright (Google), 2020
- martinfowler.com/articles/2021-test-shapes.html — Fowler, 2021-06-02
- riverqueue.com/docs/testing; pkg.go.dev/github.com/riverqueue/river — River v0.51.2 (2026-11-09)
- github.com/oasdiff/oasdiff — v1.37.0, 2026-11-08
- pkg.go.dev/cmd/go (test caching) — Go team
- code.claude.com/docs/en/best-practices, /memory, /plugins-reference, /plugin-evals, /workflows — Anthropic, 2026
- anthropic.com/engineering/effective-harnesses-for-long-running-agents — Anthropic, 2025-11-26
- anthropic.com/engineering/effective-context-engineering-for-ai-agents — Anthropic, 2025-09-29
- martinfowler.com/articles/exploring-gen-ai/sdd-3-tools.html — Böckeler, 2025-10-15
- newsletter.kentbeck.com/p/augmented-coding-beyond-the-vibes — Beck, 2025-06-25
- thoughtworks.com/radar/techniques — Vol. 35, 2026-11-04 (anchoring to a reference application; mutation testing; agent instruction bloat; human-in-the-loop theater; risk-tiered human oversight; harness quality evaluation)
- cloud.google.com/blog/…/introducing-doras-inaugural-ai-capabilities-model — Storer, DeBellis, 2025-09-24
- dora.dev/insights/balancing-ai-tensions/ — Baolin, Harvey, 2026-03-10
- dora.dev/capabilities/trunk-based-development/ — DORA
- google.github.io/eng-practices/review/developer/small-cls.html — Google
- adr.github.io/madr/ — MADR 4.0.0, 2024-09-17; cognitect.com/blog/2011/11/15/documenting-architecture-decisions — Nygard, 2011
- c4model.com/diagrams — Simon Brown
- *Building Evolutionary Architectures*, 2nd ed. — Ford, Parsons, Kua, Sadalage, 2022
- *Team Topologies*, 2nd ed. — Skelton, Pais, 2025-09-23
- pkg.go.dev/github.com/pressly/goose/v3 (hybrid versioning) — pressly
- Testing honeycomb — Spotify Engineering, 2018; Testing trophy — Kent C. Dodds, 2021 (not re-fetched)
