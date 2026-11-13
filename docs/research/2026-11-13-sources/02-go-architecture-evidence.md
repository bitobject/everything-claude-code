<!-- Independent reviewer's report, 2026-11-13: Go architecture: real codebases and critiques.
     Raw model output, kept for its sources and detail. Which claims were re-verified is stated in
     ../2026-11-13-best-practices-review.md (sections marked [проверено] / [опровергнуто]). -->

# Go architecture review: evidence from real codebases and the strongest critiques

## 1. Verdict

Our backend guidance is closer to current Go practice than most "Clean Architecture in Go" material. Four of its choices match what Three Dots Labs recommended in 2024–2025 and what Sourcegraph, Coder, PocketBase and Boundary do in production: modules by business capability, consumer-side ports, hand wiring, and `UpdateFn` transactions with a River outbox.

It departs from the evidence in three ways:
- **Uniformity and ceremony.** Every module gets the same six components, four models per aggregate, and fakes of business logic. The most-cited Go practitioners now say: start with two layers, let different modules use different patterns, and mock only I/O.
- **Overstated enforcement.** go-arch-lint never checks standard-library imports, and nothing checks that a module reads only its own schema.
- **Unspecified hard cases.** Atomic flows across modules (charging a lawyer for a lead) and reads across modules have no rule yet.

**Bottom line (question 4):** keep the modular monolith and the inner dependency rule for modules with real domain logic. Simplify everything else (criteria in §5). Don't replace it.

Assumptions: one small team in one repository. Read-only review on 2026-11-13; nothing was run or built.

## 2. Confirmed

- **Modules by business capability, layers inside.** Avoids the god packages of layer-first codebases: Mattermost's `app` package has 171 non-test files and its store interface about 1,000 methods. Source: Grafana's [package-hierarchy.md](https://github.com/grafana/grafana/blob/main/contribute/backend/package-hierarchy.md) ("packaged by feature").
- **Small interfaces declared by the consumer; no interfaces package; no generic repository.** Go Code Review Comments; Google Go style best practices.
- **Hand wiring in `main`, no wire or fx.** TDL podcast: "if you can't inject by hand, the tree is too complex". `google/wire` was archived on 2025-08-25 and Grafana now keeps its own fork in `pkg/build/wire`. Temporal's fx use (42 `fx.go` files) shows a DI framework is a choice, not a requirement.
- **`repo.Update(ctx, id, fn)` by default, unit of work rarely, no transaction in `context`.** This is TDL's 2024 "go-to" pattern (they call the transaction provider "risky"). Sourcegraph (`WithTransact(func(tx DB))`), Coder (`InTx(func(Store))`), PocketBase (`RunInTransaction(func(txApp))`) and Boundary (`DoTx`) all pass explicit transaction handles.
  - Gitea had to add a runtime `contextSafetyCheck` to catch misuse of a session carried in the context.
  - This is contested, not settled: Avito's go-transaction-manager, Ory Kratos and Harbor carry the transaction in the context.
- **One schema per module.** Jovanović, Dec 2024.
- **Separate domain, storage and API models where they differ.** TDL 2025: one model for everything is an anti-pattern.
- **Entities with unexported state and validating constructors.** wild-workouts `domain/*`.
- **Architecture rules as executable checks.** Go's own `go/build/deps_test.go` ("a statement of policy… DO NOT CHANGE THIS DATA TO FIX BUILDS"), Kubernetes import-boss, and Grafana's depguard rule sets. Shopify's Packwerk retrospective: "holding the line… at the base layer" worked.
- **Repository tests on real PostgreSQL.** Coder, Ardan Labs, Boundary, Mattermost.
- **One standard shape copied from a reference module.** Monzo: "the large majority of services… use the same code structure… reduces cognitive overhead" (InfoQ 2020). Grafana's and Coder's AGENTS.md likewise tell agents to follow existing patterns and lean on generators and linters.

## 3. Problems

**P1. Enforcement is overstated.**
- **Where:** STACK.md §2, the heading "The dependency rule (enforced by go-arch-lint…)" and the line "Inner layers never see… `net/http`… JSON tags or SQL"; also the SKILL section "The rule".
- **Evidence:** go-arch-lint's checker does `case models.ImportTypeStdLib: return true, nil`. So `net/http`, `database/sql`, `os` and `log` all pass in `entity` and `usecase`.
- **Also unchecked:**
  - struct tags on entities;
  - that only repositories call `entity.RestoreLead`. It has to be exported because entity and repository are separate packages, which is Dave Cheney's "wide, shallow API" cost.
  - `exhaustive.default-signifies-exhaustive: true` lets a new `apperr.Kind` fall silently into `default` in `StatusOf`.
- **Severity:** medium. **Confidence:** high.
- **Change:**
  - Add depguard rule sets (already part of golangci-lint) for `**/internal/*/entity/**` and `**/internal/*/usecase/**`: strict mode, `$gostd` minus `net/`, `database/`, `os`, `log$`.
  - Add one Go test in the style of `deps_test.go` that forbids struct tags in `entity` and `Restore*` calls outside `repository/`.
  - Set `default-signifies-exhaustive: false`.

**P2. "A module reads only its own schema" is enforced by nothing.**
- **Where:** STACK.md §2, the Database row; the reference `sqlc.yaml`.
- **What happens:** every module's sqlc entry uses `schema: migrations` (all tables), so a `leads` query that joins `billing.*` compiles.
- **Evidence:** data coupling is the boundary that erodes first in a modular monolith, and practitioners enforce it at the data level (Jovanović 2024: schema per module, a database user per schema).
- **Severity:** medium. **Confidence:** high.
- **Change:** a test that every schema-qualified name in `internal/<m>/repository/postgres/queries/*.sql` is `<m>.` (the postgres skill already requires schema-qualified tables). Later, a database role per module.

**P3. The reference teaches silent data loss.**
- **Where:** `repository.go` and `leads_test.go` in the reference.
- **What happens:**
  - `Update` saves only `status` (`UpdateLeadStatus`), although `fn` may change any field.
  - The use-case fake stores the same pointer, so changes "persist" in tests even when the real adapter drops them.
  - The fake does not roll back when `fn` fails.
  - The reference has no repository integration test.
- **Why it matters:** agents are told to read the reference "before inventing a shape".
- **Evidence:** TDL's `UpdateFn` rewrites all fields ("acceptable unless performance testing shows otherwise").
- **Severity:** medium. **Confidence:** high.
- **Change:** an `UpdateLead` query that writes every mutable column; the fake stores copies and commits only on success; add the testcontainers integration test to the reference.

**P4. The mapping "becomes a compile error" claim is only half true.**
- **Where:** SKILL, "Data crossing the boundaries".
- **What happens:**
  - A new field in a generated struct compiles even when the mapping omits it.
  - `openapi.LeadStatus(o.Status)` in `handler.go` converts any string, because `LeadOutput.Status` is a `string`, so a new entity status reaches the wire unchecked.
- **Severity:** low. **Confidence:** high.
- **Change:** enable `exhaustruct` limited to mapping targets (`openapi.*`, `sqlcgen.*Params`, `*Output`); keep enums typed through the use case and map them with exhaustive switches.

**P5. Interfaces and fakes for business logic, not only for I/O.**
- **Where:** SKILL, "delivery/httpapi"; the golang-testing table (worker and gateway tests use fake use cases).
- **What happens:** the `LeadUseCases` interface copies every method of `*usecase.Leads`.
- **Evidence:**
  - TDL 2025: "mock only things that are IO"; warns against interfaces that mirror the implementation.
  - Google best practices: "prefer to design the API so that it can be tested using the public API of the real implementation."
  - Declaring the interface on the consumer side is idiomatic (Code Review Comments), so this is a cost judgement, not a rule violation.
- **The cost:** one interface plus one fake per adapter per module, with invented behaviour. Error-kind mapping is then tested against kinds the real use case may never return.
- **Severity:** medium. **Confidence:** medium.
- **Change:** handlers and workers take the concrete interactor; test them with the real interactor over fake I/O ports, or through component tests (G2).

**P6. Every read loads the full aggregate and maps three times.**
- **Where:** SKILL, "Data crossing the boundaries".
- **What happens:** a read goes row → entity → `LeadOutput` → `openapi.Lead`, and `Get` just passes the call through three layers.
- **Evidence:**
  - Ousterhout's "pass-through method" warning sign.
  - TDL's own wild-workouts serves queries from read models (`app/query`, `AllTrainingsReadModel`) that bypass the domain.
  - TDL 2025 blames struct copying on "using all layers when the project is not complex enough".
  - Our SEO pages and cabinet lists will be read-heavy, so this cost recurs on most endpoints.
- **Severity:** medium. **Confidence:** medium.
- **Change:** a CQRS-lite rule. Write use cases go through entities. Read use cases call a `…Queries` port that returns view structs straight from sqlc. Keep `…Output` only where the use case shapes the data.

**P7. Gateways couple to the other module's entire `usecase` package.**
- **Where:** STACK.md §2 table.
- **What happens:** the providing module cannot say which use cases and types are meant for other modules; its job args, ports and inputs are all reachable.
- **Evidence:**
  - TDL's monolith-microservice-shop calls a dedicated provider-side `interfaces/private/intraprocess` API with its own DTOs.
  - Grafana puts each service's contract in a root package.
  - Jovanović: "start with nothing public".
- **Severity:** low. **Confidence:** medium.
- **Change:** when a module gets its first caller, give it a small published API package with purpose-built DTOs; go-arch-lint lets gateways import only that package.

**P8. Job args are a stored wire format, but have no JSON tags.**
- **Where:** STACK.md §2, Background jobs row.
- **What happens:** the args structs live in `usecase`, where JSON tags are not allowed. River stores args as JSON, so renaming a Go field silently zeroes it in jobs already queued.
- **Severity:** low. **Confidence:** medium.
- **Change:** allow `json` tags on job-args structs as the one documented exception.

**P9 (later). One generated `gen/openapi` package for all modules.**
- **What happens:** it couples every module's handlers to every other module's types, and one file keeps growing.
- **Option:** oapi-codegen's `include-tags` can generate a separate package per module.
- **Severity:** low. **Confidence:** medium.

## 4. Gaps

- **G1. Consistency across modules and unit-of-work discipline (now).** Nothing says how "assign lead + debit the lawyer's balance" stays atomic, and that is the core money flow.
  - A gateway calls the other module's use case on its own pool, outside the unit of work. Calling it, or the interactor's pool-based repositories, inside `UnitOfWork.Do` loses atomicity and can starve the pool.
  - Coder documents "pool starvation and `idle in transaction` incidents" from exactly this mistake.
  - Decide one of three: the money invariant lives in one module; or a saga over River jobs with idempotency keys and compensations; or a deliberate shared transaction across modules, as Ardan Labs does with `NewWithTx`.
  - Add a rule: inside `Do`, use only `TxRepos`.
- **G2. A component test tier (now).** Today nothing between fake-based unit tests and Playwright runs the real wiring: generated router → handlers → use cases → sqlc → PostgreSQL.
  - Ardan Labs (`apitest` + `dbtest`), Mattermost (`api4.Setup` on a real store), Coder (`coderdtest`) and wild-workouts (`service/component_test.go`) all build their test suites around exactly this.
  - Add API tests per module on testcontainers PostgreSQL, with OpenAPI response validation and fakes only for external I/O.
- **G3. Reads across modules (now).** The cabinet and the SEO pages need data from leads, lawyers and the city/topic catalogs. With only synchronous gateways, the default becomes N+1 gateway calls. Specify read-only SQL views owned by the providing module, or projections updated by River jobs.
- **G4. How to draw modules (now).** The only guidance is "boundaries follow business capabilities".
  - Shopify: enforcing a badly drawn dependency graph adds indirection, and violation backlogs grow "faster than [teams'] capacity to fix them".
  - TDL: one feature touching many units means the split is too fine.
  - Advice: start with fewer, larger modules (one transactional consistency boundary and its data each) and split later.
- **G5. The reference covers only the easy path (now).** It has no second module or gateway, no unit of work with River `InsertTx`, no worker, and no repository or component test. These are exactly the shapes agents will invent.
- **G6. Module privacy enforced by the compiler (later).** Go's `internal/` rule could make each module's adapters private (`internal/leads/internal/...`), with a constructor in the module that wires itself. It needs no tool and would shrink the go-arch-lint config. Worth it once there are more modules and contributors.

## 5. Over-engineering, critiques, decision criteria

**O1. The same full shape for every module.** Catalogs (cities, legal topics), SEO page content and lawyer profiles are mostly CRUD.
- TDL: the wild-workouts users module is "a CRUD domain with no layers at all"; modular monoliths can use different patterns per module; "if you see more than four [layers]… be very careful".
- Khononov's heuristics: simple logic → a simple layered design and mostly integration tests; a real domain model → ports and adapters.

**O2. An anti-corruption layer on every dependency between modules, inside one team.** Each synchronous edge costs a port, a gateway package, a fake of the other module and arch-lint entries. A published API with a `<m>test` fake (Grafana's model) costs one interface per providing module instead of one per edge. Keep gateways where real translation happens. This one is my judgement.

**Critiques that apply to us:**
- pass-through layers (Ousterhout) on read paths;
- interface pollution (Code Review Comments, Google, Harsanyi) on the use-case ports declared for handlers, workers and gateways;
- mapping amplification: four models and about eight places to edit for every new field;
- package proliferation: six identically named packages per module, aliases everywhere, exported `Restore*`.

**Critiques that don't apply:** DI frameworks and reflection; producer-side interface packages; generic repositories; abstracting the database "to swap it"; transactions leaking through `context`; top-level horizontal layering.

**Evidence on AI agents:** I found only blog-level claims; no study shows that layered or vertical structure changes agent success. My judgement: consistent shapes and mechanical checks matter more than the particular layering, while ceremony costs tokens and multiplies the places an edit can go wrong.

**Decision criteria.** Use full layering (entity, use case, adapters) for a module if any of these hold:
- it has a lifecycle or state machine;
- it handles money or legal obligations;
- it has invariants that span several fields or aggregates;
- it is driven from several places (HTTP, jobs, other modules);
- it has about ten or more use cases.

Otherwise use a "thin module": one package with handlers, service functions and a sqlc store. It keeps its own schema and its published API, and no other module imports its internals. Promote it to full layering when real rules appear.

**Cheapest changes, in order:**
1. depguard rules, the deps test and the SQL-schema test (P1, P2).
2. Fix the reference `Update` and fake; add integration and component tests (P3, G2).
3. The read-model rule (P6).
4. The thin-module profile in STACK.md and `new-module` (O1).
5. Drop the use-case ports in handlers and workers as tests move to component level (P5).
6. Rules for consistency across modules (G1).

## Repositories examined

| Repository | Domain | Structure | Cross-module dependencies, transactions, tests | Link |
|---|---|---|---|---|
| wild-workouts-go-ddd-example | fitness (teaching) | Clean/hexagonal per service; `users` is one flat package | consumer ports implemented by gRPC adapters; HTTP uses the concrete `app.Application`; `UpdateFn`; component tests | https://github.com/ThreeDotsLabs/wild-workouts-go-ddd-example |
| monolith-microservice-shop | shop (teaching) | modular monolith: application/domain/infrastructure/interfaces | consumer adapter calls the provider's dedicated intraprocess API | https://github.com/ThreeDotsLabs/monolith-microservice-shop |
| go-clean-template | template | top-level layers entity/usecase/repo/controller | interfaces on the provider side (`contracts.go`), mockgen | https://github.com/evrone/go-clean-template |
| ardanlabs/service | sales API | layer first, then domain (`xapp`, `xbus/stores/xdb`) | `productbus` imports `userbus`; transaction begun in middleware and bound across domains with `NewWithTx`; API tests on a real database | https://github.com/ardanlabs/service |
| pocketbase | backend as a service | one `core` package (71 files) | direct calls; `RunInTransaction(func(txApp))` | https://github.com/pocketbase/pocketbase |
| mattermost | chat | layer first: api4 → app → store (+ generated cache/retry layers) | through `app`; tests on a real database | https://github.com/mattermost/mattermost |
| grafana | observability | per feature: root interface, `ximpl`, `xtest` | providers publish interfaces; Wire fork; depguard boundary rules | https://github.com/grafana/grafana |
| gitea | git forge | `models`/`services`/`routers` per feature | direct imports; transaction in context plus a runtime safety check | https://github.com/go-gitea/gitea |
| coder | dev environments | one large `coderd` package over sqlc `Store` | `InTx(func(Store))`; `dbauthz` decorator; real Postgres | https://github.com/coder/coder |
| sourcegraph (snapshot) | code search | stores in `internal/database` | `WithTransact(func(tx DB))` | https://github.com/sourcegraph/sourcegraph-public-snapshot |
| temporal | workflow engine | services by role + pluggable persistence | uber/fx; in-process cluster tests | https://github.com/temporalio/temporal |
| boundary | access management | flat domain packages + proto-first handlers | `DoTx(func(Reader, Writer))`; real Postgres | https://github.com/hashicorp/boundary |
| kratos | identity | feature packages (handler + manager + persistence port) | consumer interfaces of `Provider`s served by one Registry; transaction in context | https://github.com/ory/kratos |
| harbor | container registry | swagger-first handler → controller → manager → dao | controllers orchestrate across domains; transaction in context | https://github.com/goharbor/harbor |

**Synthesis (question 1):** only the teaching repositories use full Clean/hexagonal layering. Production codebases organize by feature (Grafana, Boundary, Kratos) or by layer with feature subpackages (Mattermost, Gitea, Harbor, Ardan Labs). Most keep one model with storage tags and put logic in a service layer. Modules depend on APIs published by the providing module, and tests lean on a real database.

**Russian companies:** the only primary source I found was Avito's context-based go-transaction-manager. I found no authoritative architecture write-ups from Ozon or Wildberries; the Habr articles I found are personal opinion pieces.

## 6. Sources

- Three Dots Labs, "Is Clean Architecture Overengineering?" podcast, M. Smółka and R. Laszczak, 2025-04-03 — https://threedots.tech/episode/is-clean-architecture-overengineering/
- Three Dots Labs, "Database Transactions in Go with Layered Architecture", M. Smółka, 2024-09-05 — https://threedots.tech/post/database-transactions-in-go/
- Go team, Code Review Comments, Interfaces — https://go.dev/wiki/CodeReviewComments#interfaces
- Google, Go Style Best Practices (Interfaces, Package size) — https://google.github.io/styleguide/go/best-practices
- Dave Cheney, "Practical Go" §5.1, 2019 — https://dave.cheney.net/practical-go/presentations/qcon-china.html
- J. Ousterhout, *A Philosophy of Software Design*, 2nd ed., 2021; Ousterhout and Martin, "aposd-vs-clean-code", 2024–25 — https://github.com/johnousterhout/aposd-vs-clean-code
- T. Harsanyi, *100 Go Mistakes* #5, Manning, 2022 — https://100go.co/5-interface-pollution/
- B. Johnson, "Packages as layers, not groups", 2021-01-12 — https://www.gobeyond.dev/packages-as-layers/
- V. Khononov, *Learning Domain-Driven Design*, ch. 10, O'Reilly, 2021 — https://www.oreilly.com/library/view/learning-domain-driven-design/9781098100124/
- M. Jovanović, "Internal vs. Public APIs in Modular Monoliths", 2024-12-14 — https://milanjovanovic.tech/blog/internal-vs-public-apis-in-modular-monoliths
- Shopify, "A Packwerk Retrospective", 2024-02-07 — https://shopify.engineering/a-packwerk-retrospective
- go-arch-lint v1.19.1, checker source — https://github.com/fe3dback/go-arch-lint/blob/master/internal/services/checker/checker_imports.go
- depguard v2 — https://github.com/OpenPeeDeeP/depguard
- Go, `deps_test.go` — https://github.com/golang/go/blob/master/src/go/build/deps_test.go
- Kubernetes import-boss — https://pkg.go.dev/k8s.io/code-generator/cmd/import-boss
- Grafana `.golangci.yml` (depguard rule sets) — https://github.com/grafana/grafana/blob/main/.golangci.yml
- Coder, DATABASE.md ("Transaction Safety with InTx") — https://github.com/coder/coder/blob/main/.claude/docs/DATABASE.md
- Gitea, `models/db/context.go` — https://github.com/go-gitea/gitea/blob/main/models/db/context.go
- Avito, go-transaction-manager (v2 RC, 2026) — https://github.com/avito-tech/go-transaction-manager
- InfoQ, Monzo Q&A with Suhail Patel, 2020-08 — https://www.infoq.com/news/2020/08/banking-microservices
- google/wire, archived 2025-08-25 — https://github.com/google/wire
- oapi-codegen configuration schema (`include-tags`) — https://github.com/oapi-codegen/oapi-codegen/blob/main/configuration-schema.json
