<!-- Independent reviewer's report, 2026-11-13: Go backend architecture: schools of thought.
     Raw model output, kept for its sources and detail. Which claims were re-verified is stated in
     ../2026-11-13-best-practices-review.md (sections marked [проверено] / [опровергнуто]). -->

# Review: Go backend architecture, schools of thought

Read-only review. I read STACK.md §1–2, the go-clean-architecture skill, the whole reference module, rules/go.md, architect.md and new-module.md. I also checked them against the current sources of Ardan `service` (2026-10-24), Wild Workouts (2026-08-27), WTF Dial and evrone, downloaded through the Go module proxy and not executed. River v0.51.2, go-arch-lint v1.19.1 and oapi-codegen v2.8.2 were read from the module cache.

## 1. Verdict

Our design is a mainstream way to build a large Go business service. Inside each module it is essentially Three Dots Labs' DDD-lite Clean Architecture: UpdateFn repositories, ports declared by the consumer, hand-written mapping, manual wiring and a linted dependency rule. Ardan Labs' 2026 `service`, which now also ships agent skills, arrives at the same ideas under other names. The Go team prescribes no layering at all, so "strict" layering is a defensible team choice rather than Go consensus. Its Go-specific costs (generic package names that collide across modules, many small packages) are cheap to fix now and expensive later.

The real issues are concrete:
- The reference `Update` saves only `status`.
- River fits neither the arch-lint config nor events with several subscribers.
- There is no read side and no Go test that runs through the real stack.

For a small team plus AI agents, one type per use case will scale better than one interactor per module.

## 2. Confirmed

**Where the schools agree:** the Go team, Ben Johnson, Mat Ryer, Ardan and Three Dots all favour a domain-oriented top level, small interfaces declared by their consumer, explicit constructor wiring, and separate models per layer with plain mapping once an app is large.

**Where they disagree:**
- **Layers inside a domain:** Cheney and Edwards prefer fewer, larger packages; Uncle Bob and Three Dots use domain + application + adapters.
- **Entities:** Three Dots encapsulates them; Ardan and Johnson keep them as plain data.
- **Use cases:** Three Dots uses one type per use case; Ardan and Edwards use one service struct.

What we do that matches current practice:
- Modules under `internal/`, binaries in `cmd/`: go.dev "Organizing a Go module".
- Dependency rule enforced by a tool rather than by review: Martin 2012; Three Dots 2020 (go-cleanarch in CI); Ardan 2026 (conformance checklists for agents).
- Ports declared by the consumer, small, no "interfaces" package: Go Code Review Comments; Google Go Style Guide.
- Separate models per layer with explicit mapping functions, no reflection copiers:
  - Three Dots 2021: "Arcane converters will be hard to grasp".
  - Kat Zień 2023: larger apps should marshal between layers.
  - Ardan: `toBus`/`fromBus`/`toDB` converters.
- `Update(ctx, id, fn)` as the default, UnitOfWork as the exception, no transaction in `context`: Three Dots 2024 calls UpdateFn "our go-to solution" and says a transaction in context "only masks the root issue".
- One repository per aggregate, no generic `Repository[T]`: Three Dots 2024.
- `apperr` with kinds and stable codes, internal by default: the same design as WTF Dial `error.go`.
- Manual DI, a `run()` that returns an error, no globals: Mat Ryer 2024. google/wire is archived (verified: last push 2025-08-22).
- Gateway on the caller side for synchronous calls, River `InsertTx` as the outbox: Grzybek 2020.
- Hand-written fakes instead of a mocking library: Three Dots 2020.
- Separate `entity` and `usecase` packages: Cockburn & Garrido de Paz (2024) say ports and adapters "has absolutely nothing to say about how you structure your application internally", so this split is Clean/Onion layered on top of hexagonal. It is worth keeping because it lets go-arch-lint keep I/O out of entities.

## 3. Problems

**P1. `Update(fn)` saves only `status`.** Severity high, confidence high.
- **Where:** `reference/.../repository/postgres/repository.go:59–87` with the `UpdateLeadStatus` query; the same code is repeated in postgres-patterns "Update: load, change, save".
- **What we say:** the port "runs fn and saves the result atomically" (`usecase/leads.go:19–20`).
- **What happens:** the adapter writes one column. Any later method that changes another field is dropped without an error. The obvious next case is the lawyer ID implied by the contract's own "Assign a new lead to a lawyer". new-module.md tells agents to "copy the shapes… don't invent new ones".
- **Evidence:** UpdateFn adapters save the whole aggregate (Wild Workouts `upsertHour` maps the full aggregate to its row).
- **Fix:** one `UpdateLead` query that writes every mutable column, plus a repository test that round-trips every field after `Update`.

**P2. River is missing from `.go-arch-lint.yml`.** Severity medium, confidence high.
- **Where:** the config declares only the `pgx` and `oapi` vendors; `leads-repository` has `canUse: [pgx]` and `leads-delivery` has `canUse: [oapi]`.
- **What we say:** STACK §2 allows River in `repository/postgres` (InsertTx) and in `delivery/worker`, and rules/go.md forbids widening the config "to make code pass".
- **What happens:** the first UnitOfWork or worker an agent writes from postgres-patterns fails the arch check, or the agent breaks the rule.
- **Fix:** add a `river` vendor and a separate `delivery/worker` component now.

**P3. Job arguments are untagged JSON that River stores.** Severity medium, confidence high.
- **Where:** STACK §2 puts job argument structs in `usecase` and bans JSON tags in inner layers. postgres-patterns shows `LeadAssigned struct{ LeadID uuid.UUID } // stored as JSON by River` and forbids renaming only the `Kind`.
- **What happens:** renaming a Go field changes the JSON of jobs already queued, and River retries for days. The old key decodes to a zero value.
- **Fix:** treat payloads as versioned contracts: tagged structs owned by an adapter or contract package, mapped from use-case types, with a golden JSON test per kind.

**P4. Async reactions between modules don't work as described.** Severity high, confidence high.
- **Where:** SKILL.md "Between modules" says "A enqueues a job…; B's worker reacts."
- **Why it fails:**
  - River allows exactly one worker per job kind (`AddWorker` panics on a duplicate), so each job has one subscriber and A must know every consumer.
  - arch-lint forbids B's `delivery` from importing A's `usecase`, where the job type lives, so B must copy A's payload type.
- **Why it matters:** lead → distribution / notification / billing is this product's core flow.
- **Fix:** see G1.

**P5. The same generic package names in every module.** Severity medium, confidence medium (contested).
- **Where:** STACK §2 layout and rules/go.md.
- **Go guidance:**
  - The Go blog: "packages that are frequently used together should have distinct names".
  - Google style lists `model` among uninformative names.
  - Cheney and Edwards prefer fewer, larger packages.
  - Our own golang-patterns bans `models`/`types`, yet every module has `entity`, `usecase` and `postgres`.
- **What happens:**
  - The reference `main.go` already needs three aliases for one module (`leadsuc`, `leadspg`, `leadshttp`) and has two packages named `postgres`.
  - Every gateway imports two packages named `usecase`.
- **Evidence:** Ardan avoids this with per-domain names (`userbus`, `userapp`, `userdb`).
- **Fix:** module-prefixed names, for example `leads/lead`, `leadapp`, `leadpg`, `leadhttp`, `leadjobs`.

**P6. One interactor per module (question 3).** Severity medium, confidence medium; both styles are common in Go, so this is my judgment for this team.
- **Where:** the SKILL.md use-case contract ("split past ~10 methods"); the reference `usecase/leads.go` holds the ports, DTOs and every method.
- **Evidence:**
  - Canonical Clean Architecture and Three Dots use one type per use case: "easier to build dependencies".
  - Mat Ryer: a handler "can bloody well ask for [its dependency] as an argument".
  - With one interactor, every test wires every port, decorators must pass through every method (Ardan's own skill: "Implement EVERY ExtBusiness method"), and one file becomes the merge hotspot for parallel agents.
- **Fix:** one file and one small type per use case, for example `AssignLead{leads, jobs}` with a `Handle` method, grouped in `App{Commands, Queries}` for wiring. Our go-reviewer example already assumes `usecase/assign.go`.

**P7. Gateways bind to the other module's whole `usecase` package.** Severity medium, confidence medium.
- **Where:** STACK §2, `gateway/<other>` imports `<other>/usecase`.
- **What happens:** that package also holds the other module's ports, internal DTOs and job types, so all of it becomes public API in practice.
- **Evidence:** Grzybek: a gateway on the caller side and a facade on the called side.
- **Fix:** see G5.

**P8. `NewLead`/`RestoreLead` take positional parameters of the same type.** Severity low–medium, confidence high.
- **Where:** `entity/lead.go:73,86`; `topic, city string` are adjacent, and swapping them compiles.
- **Evidence:** Google style: "adjacent parameters of the same type become easier to confuse"; it suggests an option struct.
- **Fix:** `RestoreLead(LeadState{…})`.

**P9. Fakes can drift from the real adapters.** Severity low–medium, confidence high.
- **Where:** `reference/.../usecase/leads_test.go:51–62` changes the stored lead even when `fn` fails. golang-testing shows the corrected copy-on-write fake, but nothing proves that fakes and adapters behave the same.
- **Evidence:** Wild Workouts runs one repository test suite against the memory, MySQL and Firestore implementations.
- **Fix:** shared contract tests run against both the fake and the PostgreSQL adapter.

**P10. The UnitOfWork guidance lacks Three Dots' warnings.** Severity low–medium, confidence medium.
- **Where:** the SKILL.md and postgres-patterns UnitOfWork sections.
- **What is missing:**
  - Inside a shared transaction "all the SELECTs need to include the FOR UPDATE clause".
  - Nothing forbids calling a gateway inside `Do`, which opens another module's transaction and connection while rows stay locked.
- **Fix:** inside `Do`, read only through `Update`/`FOR UPDATE`, and make no gateway calls.

**P11. A cited source contradicts our own rules.** Severity low, confidence high.
- **Where:** SKILL.md cites evrone `go-clean-template`.
- **What it does:** v1.19.3 (2026-11-11) layers by technical type at the top (`internal/entity|usecase|repo|controller`) and declares interfaces next to their implementations with mockgen. Both are on our anti-pattern list.
- **Fix:** remove it, or mark it as a contrast.

## 4. Gaps

**G1. Events with fan-out between modules (now).**
- Each module publishes events as versioned, tagged JSON in a contract package.
- A inserts one `leads.lead_created` job in its own transaction.
- A dispatcher worker in `platform` inserts one job per subscriber registered in `cmd`, and completes the event job in the same transaction (`InsertManyTx` + `river.JobCompleteTx`).
- A never knows its subscribers. This is Grzybek's "Direct Call and Messaging together".
- Optionally, aggregates record their events and `Update` saves them in its own transaction. The UnitOfWork is then needed only when several aggregates change together.

**G2. A query side (now).**
- Lawyer cabinet lists, admin reports and SEO page data should come from query ports that return use-case read DTOs straight from sqlc rows: no `Restore`, no re-parsing, no phone loaded.
- Today `ListLeads` builds full aggregates.
- Three Dots: read models are "often driven by the website's UI", and "we don't need to add a read model interface for every query". Ardan's `vproductbus` is a read-only domain over a SQL view.

**G3. Reads that combine several modules (now).**
- A city × topic page needs lawyers, ratings and content. With "reads only its own schema" plus gateways, that becomes N+1 orchestration.
- Allow either a read/BFF module that calls query facades, or modules that publish read-only SQL views as part of their contract (Ardan's `view_products`).

**G4. Go component tests through the real stack (now).**
- Mat Ryer tests by calling `run`; Ardan has per-domain `apitest` suites on a real database; Wild Workouts has `component_test.go`.
- We go straight from layer tests with fakes to Playwright.
- Make `run(ctx, args, environ, stdout)` injectable (`env.ParseWithOptions` accepts a supplied environment) and add one test per operation. This catches wiring, middleware order, validator setup and real transaction behaviour.

**G5. A facade per module with compiler-enforced encapsulation (now, cheap before code exists).**
- Move each module's internals under `internal/<m>/internal/…`.
- The module root package exposes `New(deps)` (wiring shared by `cmd/api` and `cmd/worker`), its HTTP handler and its contract for other modules.
- Go's `internal` rule then blocks imports between modules at compile time.
- go-arch-lint layer rules can become wildcards that cover every module (glob precedence still to be verified) instead of six copied components per module.
- Precedents: MallBots' `module.go` + `internal/` + `basketspb`; Simon Brown, "The Missing Chapter" (let the compiler enforce the architecture).

**G6. Pick the module style by complexity (now).**
- Khononov ch. 10: transaction script or active record with layers for simple subdomains, a domain model with ports and adapters for the complex core.
- Three Dots left a tiny service "untouched".
- Leads and billing deserve the full shape; catalog/content (cities, topics, FAQ) does not.

**G7. Cross-cutting concerns as decorators (later).** One generic decorator per concern for tracing and metrics (Three Dots 2022). It is cheap only once there is one type per use case (P6).

**G8. Policies spanning several modules (later).** Lead-distribution rules (city, topic, balance, quotas) span three modules. They need guidance on pure domain services fed by gateway data.

## 5. Over-engineering

- **The full shape for every module:** getters, `Restore`, mirror interfaces and UnitOfWork even where a module has no invariants (see G6).
- **The `LeadUseCases` interface mirrored in delivery, plus its fake:** each new use case means editing four places. Google style prefers testing through the real implementation; tests can drive the real use case with fakes of the storage ports, as in Cockburn's test-harness setup.
- **Three mappings per read** (row → entity → `LeadOutput` → `openapi.Lead`): G2 removes one step and the re-validation.
- **arch-lint config grows by six copied components per module:** G5 replaces it with a fixed-size config.

## 6. Sources

- go.dev, "Organizing a Go module" (Go team, current) — https://go.dev/doc/modules/layout
- "Package names", Sameer Ajmani, 2015-02-04 — https://go.dev/blog/package-names
- Go Code Review Comments (Go team, wiki) — https://go.dev/wiki/CodeReviewComments
- Google Go Style Guide: Decisions and Best Practices — https://google.github.io/styleguide/go/
- Ben Johnson, "Packages as layers, not groups", 2021-01-12 — https://www.gobeyond.dev/packages-as-layers/ ; WTF Dial v0.2.2 `error.go` (2021)
- Ardan Labs `service` @9ea13168, 2026-10-24: `.agents/skills/layered-architecture-types`, `business-layer-extensions`, `vproductbus`, `apitest` — https://github.com/ardanlabs/service
- Three Dots Labs:
  - Smółka, "How to implement Clean Architecture in Go", 2020-09-01 — https://threedots.tech/post/introducing-clean-architecture/
  - Laszczak, "Introducing basic CQRS", 2020-09-28 — https://threedots.tech/post/basic-cqrs-in-go/
  - Smółka, "Common Anti-Patterns in Go Web Applications", 2021-08-12 — https://threedots.tech/post/common-anti-patterns-in-go-web-applications/
  - Smółka, "Increasing Cohesion in Go with Generic Decorators", 2022-05-25 — https://threedots.tech/post/increasing-cohesion-in-go-with-generic-decorators/
  - Smółka, "Database Transactions in Go with Layered Architecture", 2024-09-05 — https://threedots.tech/post/database-transactions-in-go/
  - Wild Workouts @8ecfcdf, 2026-08-27 — https://github.com/ThreeDotsLabs/wild-workouts-go-ddd-example
- Mat Ryer, "How I write HTTP services in Go after 13 years", Grafana, Feb 2024 — https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/
- Alex Edwards:
  - "11 tips for structuring your Go projects", 2025-01-22 — https://alexedwards.net/blog/11-tips-for-structuring-your-go-projects
  - "The fat service pattern", 2022-08-08 — https://alexedwards.net/blog/the-fat-service-pattern
- Dave Cheney, "Practical Go" (QCon China), 2019-04 — https://dave.cheney.net/practical-go/presentations/qcon-china.html
- Kat Zień, GopherCon 2018 talk; JetBrains interview, 2023-04-11 — https://blog.jetbrains.com/go/2023/04/11/catching-up-with-kat-zien-on-the-structure-of-go-apps-in-2023/
- Robert C. Martin, "The Clean Architecture", 2012-08-13 — https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html ; *Clean Architecture* (2017), ch. 34 by Simon Brown
- Cockburn & Garrido de Paz, *Hexagonal Architecture Explained* (2024; preview text) — https://gist.github.com/zkdiff/794f9d44b86ee49b2f0e4ce766a6981d
- Kamil Grzybek, "Modular Monolith: Integration Styles", 2020-07-26 — https://www.kamilgrzybek.com/blog/posts/modular-monolith-integration-styles
- Michael Stack, *Event-Driven Architecture in Golang* (Packt 2022), Chapter12/baskets — https://github.com/PacktPublishing/Event-Driven-Architecture-in-Golang
- Jimmy Bogard, "Vertical Slice Architecture", 2018-04-19 — https://www.jimmybogard.com/vertical-slice-architecture/
- Vlad Khononov, *Learning Domain-Driven Design* (O'Reilly 2021), ch. 10
- Code read from the module cache:
  - River v0.51.2 `worker.go` (`AddWorker`) and `job_complete_tx.go`
  - go-arch-lint v1.19.1 `docs/syntax` (released 2026-10-29)
  - oapi-codegen v2.8.2 configuration (`include-tags`)
  - caarlos0/env v11.4.2 (`ParseWithOptions`)
  - `go help` (Go 1.27.4) on `internal` directories
- evrone go-clean-template v1.19.3, 2026-11-11 — https://github.com/evrone/go-clean-template

Downloaded sources are in /tmp/claude-0/-root-lead-new--localenv/6b41f19b-ec42-4595-8dc3-68f60461b2ed/scratchpad/research-go-arch-schools/. Nothing in the plugin was modified.
