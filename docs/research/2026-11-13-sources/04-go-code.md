<!-- Independent reviewer's report, 2026-11-13: Go code-level practices, testing and linting.
     Raw model output, kept for its sources and detail. Which claims were re-verified is stated in
     ../2026-11-13-best-practices-review.md (sections marked [проверено] / [опровергнуто]). -->

**Scope.** Paths are relative to `/root/lead_new/.localenv/plugins/everything-claude-code/`, and `ref/` means `skills/go-clean-architecture/reference/backend/`. STACK.md, golang-patterns, go-reviewer and the Makefile changed while I was working (between 23:53 and 00:07, +04). The findings below match the files as of about 00:10 on 2026-11-13.

I checked claims against the Go 1.27.4 `api/*.txt` files, the spec, `go doc` and the stdlib source. I also read the module cache (pgx 5.13, kin-openapi 0.153, nethttp-middleware 1.3.1, oapi runtime 1.9, golangci-lint 2.15), the release notes and the lint configs of 7 projects. Nothing was built or run. The downloaded sources are in `/tmp/claude-0/-root-lead-new--localenv/6b41f19b-ec42-4595-8dc3-68f60461b2ed/scratchpad/research-go-code/`.

## 1. Verdict
At the idiom level the material is current and better than usual. Every Go-version claim in STACK.md §5 is correct for Go 1.27.4. The idioms match Google's style guide, Uber's guide and CodeReviewComments. The testing approach is what the Go team and Google recommend: stdlib plus go-cmp, hand-written fakes, real PostgreSQL through template databases, synctest, and contract validation.

The weak spots are where the reference handles live requests:
- Validation errors send personal data (phone numbers) back to the client.
- Nothing actually limits how long a handler runs.
- The error path hides 503s and corrupt-data cases, but logs client disconnects as errors.

Observability is a one-line decision with no mechanism behind it. Today the `…Context` logging calls carry no request or trace ID. The lint config is sound, but it lacks depguard, which every respected project I checked uses. It also lacks the linters that would enforce your own slog and testing rules.

## 2. Confirmed
- **apperr design.** Kinds, stable codes and a single mapping point form a "canonical error space". This matches Google's Best Practices on error handling and Ben Johnson's WTF Dial `error.go`.
- **Wrapping rules.** `%w` vs `%v`, no "failed to", no repeating what the callee already said, lowercase messages. Matches Google Best Practices ("Adding information to errors") and Uber ("Error Wrapping").
- **Handle errors once, log at the edge.** Matches Uber "Handle Errors Once" and Google "Logging errors".
- **Error APIs.** `errors.AsType` (Go 1.26) and pointer-receiver error types fit the Go 1.27 vet check on `%w` (both release notes).
- **Context rules.** Including `t.Context()` in tests and `context.Background()` in Cleanup (Google Decisions, "Contexts").
- **`main` → `run() error`, exit once.** Matches Uber "Exit in Main" and Mat Ryer (2024).
- **HTTP server basics.** Server timeouts, `MaxBytesReader`, `slog.NewLogLogger` for ErrorLog, re-panicking `ErrAbortHandler`, `CrossOriginProtection` (net/http docs; Go 1.25 notes).
- **Shutdown.** `WithoutCancel` plus a deadline, and the pool closed after the server (VictoriaMetrics, 2025).
- **DSN parse error not wrapped.** Correct: pgx calls its own password redaction "best effort" (`pgconn/errors.go`).
- **caarlos0/env v11.** Maintained (v11.4.2, 2026-11-02) and supports `required`, `unset` and `file`. kelseyhightower/envconfig (last release 2019) and cleanenv (2023) are stale; viper and koanf are heavier than needed.
- **Test style.** No assertion library, got/want, `(-want +got)`, `t.Helper`, `t.Fatal` only on the test goroutine (Go wiki TestComments; Google Decisions).
- **Test doubles.** Hand-written fakes, `usecasetest`/`pgtest` package names, `TestMain` for expensive setup (Google Best Practices).
- **Database tests.** A migrated template, `CREATE DATABASE … TEMPLATE` per test, and `TEST_DATABASE_URL` — the same design as pgtestdb. The testcontainers v0.45.0 calls used all exist (`Run`, `BasicWaitStrategies`, nil-safe `TerminateContainer`).
- **Time-dependent tests.** `synctest`, `synctest.Sleep` and `httptest.NewTestServer` (in-memory network, registers its own Cleanup, any host works) are described accurately (go doc 1.27; Go blog, Aug 2025).
- **`go fix -diff` as a gate.** It exits non-zero when there is a diff and skips generated files. The Go team built the modernizers because AI coding assistants keep writing old idioms (Go blog, Feb 2026), which is exactly this project's situation.
- **JSON v1/v2 guidance.** Matches the Go blog "A new Go API for JSON" (Nov 2026).
- **Generic methods.** The 1.27 caveat that they cannot implement interface methods is correct.

## 3. Problems
**P1 — Validation errors send personal data and the schema to the client. Severity high, confidence high.**
- Where: `ref/cmd/api/main.go:83` sends `Detail: err.Error()` for request-validator failures. `ref/internal/platform/httpserver/problem.go:69` does the same for decode errors.
- Evidence: for OpenAPI 3.1, kin-openapi uses the JSON Schema 2020-12 validator. kin-openapi's own `openapi3/schema_error.go` says that validator puts the submitted values into its messages, and `Error()` also appends full `Schema:` and `Value:` dumps.
- Effect: an `additionalProperties` or `required` failure on `POST /v1/leads` returns the whole request body, phone included. That breaks the contract's promise that the phone "is not returned" and the go-reviewer CRITICAL rule.
- Fix:
  - Build `detail` from `RequestError.Parameter.Name`, `SchemaError.JSONPointer()` and `SchemaField`, or use `Options.Options.WithCustomSchemaErrorFunc`.
  - Map `*http.MaxBytesError` to 413.
  - Add a handler test that sends an invalid body and asserts the phone is absent from the response.

**P2 — Nothing limits how long a handler runs. Severity medium, confidence high.**
- Where: `ref/.../httpserver/server.go:20-23` sets server timeouts and nothing else.
- Evidence: `WriteTimeout` does not cancel the handler. `r.Context()` ends only when the client disconnects, an HTTP/2 stream is cancelled, or the handler returns (`go doc net/http.Request.Context`; golang/go#59602). `startBackgroundRead` clears the read deadline, so `ReadTimeout` does not cancel it either.
- The reference `postgres.Open` also lacks the `statement_timeout` that postgres-patterns prescribes. Of that skill's "two clocks" bounding every query, neither exists in the reference.
- Effect: after 30 s the response can no longer be written, but the query keeps its goroutine and pooled connection.
- Fix: add a `context.WithTimeout` middleware inside `Recover`, map `DeadlineExceeded` to a 503/504 problem, and bring `postgres.Open` in line with postgres-patterns.

**P3 — The error writer logs the wrong things. Severity medium, confidence high.**
- Where: `problem.go:56-61` never logs non-internal kinds.
- Effect: `KindUnavailable` errors (deadlocks, lock or statement timeouts, returned as 503) leave no trace, although postgres-patterns wraps them with the comment "cause kept for logs". Client disconnects (`context.Canceled`) go the other way: they are logged at ERROR and answered with a 500.
- Fix: decide by status. Log everything ≥500, using WARN for Unavailable. Don't log 4xx. Log `Canceled` at DEBUG and don't answer it with a 500.

**P4 — Error kinds leak through `%w`. Severity medium, confidence high.**
- Where: `ref/internal/leads/repository/postgres/repository.go:89-96` wraps validation errors for *stored* rows with `%w`.
- Effect: a corrupt phone in the database surfaces as a 422 `lead.invalid_phone` on GET or assign, and because of P3 it is never logged.
- Related: `entity.NewLead` accepts the zero value `Phone{}` (`lead.go:73`). golang-patterns:15 claims unexported fields make invalid values "impossible to construct", but the zero value always compiles.
- Fix: translate errors in the adapter with a KindInternal sentinel, for example `fmt.Errorf("%w: stored lead %s: %v", errCorrupt, id, err)`. Add a rule that a kind is valid only in the layer that chose it. Reject `Phone{}` in `NewLead`.

**P5 — apperr sentinels are mutable globals. Severity low–medium, confidence medium.**
- Where: `ref/internal/kernel/apperr/apperr.go:44-55` exports `Kind`, `Code` and `Msg` on shared pointers.
- Effect: `errors.AsType` returns the sentinel itself. Translating `e.Msg` into Russian inside ErrorWriter, for example, would race across requests. Matching is by pointer only, so a re-created error with the same code doesn't match.
- Fix: unexported fields with `Kind()`, `Code()` and `Message()` accessors. Optionally add an `Is` method that compares `Code`, as WTF Dial does.

**P6 — `…Context` logging carries nothing. Severity medium, confidence high.**
- Where: golang-patterns:139 says the `…Context` variants make "trace/request IDs flow"; go-clean-architecture says to log "with request IDs".
- Evidence: `slog.JSONHandler.Handle(_ context.Context, …)` ignores the context, and no middleware puts a request ID into it.
- Fix: see G1.

**P7 — Recover drops the stack trace. Severity medium, confidence high.**
- Where: `server.go:69` logs only the panic value. Once the panic is recovered, net/http's own stack log never happens.
- Fix: add `"stack", debug.Stack()`.

**P8 — The reference fake breaks the skill's own rule. Severity low–medium, confidence high.**
- Where: `ref/internal/leads/usecase/leads_test.go:28-62`.
- Effect: the fake stores `*entity.Lead`, mutates it before checking `fn`'s error (so nothing is rolled back), shares the stored pointer and ignores `err` in `Update`. golang-testing's anti-pattern table requires copy-on-Update and restore-on-error, and agents will copy the reference.
- Fix: store values and copy in and out, like the skill's `fakeLeads`.

**P9 — Lint config versus the written guidance. Severity low, confidence high.**
- Where: `ref/.golangci.yml:36-77`.
- `time.Now` is banned outside `platform/clock`, but golang-patterns' own logging example needs `start := time.Now()`, and middleware and metrics need it too.
- `context.Background` is banned in golang-testing's `pgtest` helper, which is not a `_test` file.
- Test files are fully exempt, so nothing enforces `t.Context()` in tests.
- revive lists 9 rules without `enable-default-rules: true`. golangci-lint then replaces revive's default set (`pkg/golinters/revive`), dropping rules such as indent-error-flow, superfluous-else and unused-parameter.
- The `std-error-handling` preset hides unchecked `Close` on writers, which golang-patterns:144 says matters.
- `exhaustive.default-signifies-exhaustive: true` lets `StatusOf` turn a new Kind into a 500 silently.
- Fix:
  - Scope the `time.Now` ban with `path-except` to `internal/*/(entity|usecase)/`.
  - Exclude `*test` helper packages.
  - Add `usetesting` with `context-background` and `context-todo` set to true; both default to false in 2.15.
  - Set `revive.enable-default-rules: true`, as Prometheus does.
  - Add a test that every Kind maps to a status.

**P10 — Delivery tests don't use the production stack. Severity low–medium, confidence high.**
- Where: `handler_test.go:48-62` omits the validator, LimitBody, CSRF protection and Recover, although the skill says tests use "the same stack cmd/api builds". So P1's path is never checked against the contract.
- Fix: a single exported constructor that returns `http.Handler` (Mat Ryer's `NewServer` pattern), used by both `main` and the tests.

**P11 — Shutdown details. Severity low, confidence high.**
- Where: `main.go:49-50,109`.
- `stop()` is deferred, so a second SIGINT cannot force an exit.
- The `errors.Is(err, context.Canceled)` branch is dead code.
- There is no `srv.Close()` fallback.
- Since Go 1.26, `context.Cause(ctx)` reports which signal arrived; log it.

**P12 — Small inaccuracies. Severity low, confidence high.**
- golang-patterns:23 lists `maps.Keys` under 1.21. It arrived in 1.23 and returns `iter.Seq`, so the idiom is `slices.Sorted(maps.Keys(m))`.
- golang-patterns:100 says `time.After` "leaks timers less" since 1.23. Since 1.23 unreferenced timers are collected immediately, so it no longer leaks.
- golang-patterns:155 gives a 2–7 % PGO gain. go.dev/doc/pgo now says 2–14 %.
- golang-testing:372 uses `GetSwagger()`, which the generated code marks Deprecated (SA1019). The reference uses `GetSpec()`.
- golang-testing:258 calls `postgres.Migrate`, which the reference doesn't have.
- `-shuffle` reorders only top-level tests, not table rows (`testing.go`).
- golang-patterns:169 should use `go mod tidy -diff` (Go 1.23), and the Makefile has no tidy check.

**P13 — The edit hook formats differently from the gate. Severity low, confidence medium.**
- Where: `hooks/scripts/post-edit.sh:21` runs `goimports` without `-local`, while golangci-lint enforces `local-prefixes`. The `timeout` command on line 29 is not on stock macOS.
- Fix: run `bin/golangci-lint fmt "$file"`.

**P14 — "Fail, never skip" is contested. Severity low, confidence medium.**
- Where: golang-testing:22.
- It is right for CI. Locally, every `go test ./...` needs Docker and starts one container per adapter package, which is heavy on 2 CPUs and 3.8 GB.
- Suggestion: skip database tests under `-short` locally and fail in CI.

## 4. Gaps
**Now**
- **G1 — Observability needs actual wiring.**
  - Middleware that sets or reads the request ID and trace ID goes outermost. Anything that calls `r.WithContext` must sit outside otelhttp and the access log, because the generated inner mux sets `Pattern` on the copy.
  - A slog wrapper handler that adds `request_id`, `trace_id` and `span_id` from the context.
  - otelhttp 0.72 already takes `http.route` from `Request.Pattern` and names spans `{method} {route}`.
  - Set the propagator explicitly: `otel.SetTextMapPropagator(propagation.TraceContext{})`. The global default does nothing.
  - otelpgx 0.13 for query tracing and pool stats; rivercontrib/otelriver 0.12 for jobs.
  - An access log with route pattern, status and duration, never the query string.
  - RED metrics with no personal data in labels and bounded cardinality; never city × topic as labels.
  - OTel Go's logs API and SDK became stable in 1.47.0, but the otelslog bridge is still v0.21. Either write JSON to stdout or combine both with `slog.NewMultiHandler`.
- **G2 — Health checks and drain.** Separate `/livez` and `/readyz`, with readiness pinging the database. On SIGTERM, fail readiness, wait about one probe period, then call `Shutdown`.
- **G3 — Mechanical PII guards.** A `LogValue()` method on `Phone` and on a `Secret` type (see the stdlib `ExampleLogValuer_secret`), plus sloglint `forbidden-keys`.
- **G4 — Linters to add.** The point is to turn prose rules into checks that agents can't drift past.
  - **depguard.** Used by golangci-lint itself, Prometheus, Kubernetes, Grafana, Loki, OTel-Go and Traefik. Ban:
    - `google/uuid`: it is already importable as an indirect dependency, and go-arch-lint doesn't cover tests, platform or cmd.
    - `pkg/errors`, `x/exp/slices` and `x/exp/maps`.
    - `math/rand` and `io/ioutil`.
  - **sloglint** (as Prometheus does): no global logger, `context: scope`, snake_case keys, static messages.
  - **usetesting** (as OTel-Go does).
  - **govet nilness.**
  - Optional: errname, nilnil, recvcheck, nakedret, gochecknoinits, predeclared, bidichk.
- **G5 — A few Go-level API tests through the real wiring** (the handler constructor plus pgtest). They catch wiring and middleware-order bugs that fakes miss and Playwright finds late.
- **G6 — Setup traps.**
  - `go mod init` on Go 1.27 writes `go 1.26.0` (a Go 1.26 change), which breaks stdlib `uuid` through the stdversion check. Set `go 1.27` explicitly.
  - `-race` on Linux needs cgo and a C toolchain in the CI image.

**Later**
- **G7 — Diagnostics.** pprof (including `goroutineleak`) and `trace.FlightRecorder` on an internal listener only. `goleak` in `TestMain` for packages with goroutines.
- **G8 — Secrets.** Load secrets from files (caarlos0/env `,file` option) and log the config at startup with secrets redacted.
- **G9 — Scaling the test setup.** Key the template database by a hash of the migrations, as pgtestdb does. If mockery is ever added, use its `matryer` template rather than testify-style expectations.

## 5. Over-engineering
- **contextcheck:** noisy (Traefik disables it, "too many false-positive"), and forbidigo plus review already cover it.
- **rowserrcheck and sqlclosecheck:** they target `database/sql`; with sqlc's generated code excluded and pgx underneath, they have nothing to find.
- **intrange and copyloopvar:** they duplicate the `rangeint` and `forvar` modernizers that `go fix -diff` already gates.
- **perfsprint:** churn for little gain. Since Go 1.26, `fmt.Errorf("x")` allocates the same as `errors.New`.
- **The global `time.Now` ban and the PGO advice:** they cost more than they give for this service at this stage.

## 6. Sources
- Go 1.27, 1.26, 1.25 and 1.23 release notes — Go team — Aug 2026 / Feb 2026 / Aug 2025 / Aug 2024 — https://go.dev/doc/go1.27 (and /go1.26, /go1.25, /go1.23)
- "Using go fix to modernize Go code" — Alan Donovan, Go blog — Feb 2026 — https://go.dev/blog/gofix
- "A new Go API for JSON" — Go blog — Nov 2026 — https://go.dev/blog/jsonv2
- "Generic Methods" — Go blog — Aug 2026 — https://go.dev/blog/generic-methods
- "Goroutine Leak Profiles" — Go blog — Sep 2026 — https://go.dev/blog/goroutine-leak-profiles
- "Testing Time (and other asynchronicities)" — Go blog — Aug 2025 — https://go.dev/blog/testing-time
- Profile-guided optimization guide — Go team — https://go.dev/doc/pgo
- Google Go Style Guide (Decisions, Best Practices) — Google — https://google.github.io/styleguide/go/
- Go Test Comments and Code Review Comments — Go wiki — https://go.dev/wiki/TestComments, https://go.dev/wiki/CodeReviewComments
- Uber Go Style Guide — Uber — https://github.com/uber-go/guide/blob/master/style.md
- "How I write HTTP services in Go after 13 years" — Mat Ryer — Feb 2024 — https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/
- "Graceful Shutdown in Go: Practical Patterns" — Phuong Le, VictoriaMetrics — May 2025 — https://victoriametrics.com/blog/go-graceful-shutdown/
- "net/http: request context not canceled at WriteTimeout" — golang/go issue #59602 — https://github.com/golang/go/issues/59602
- WTF Dial `error.go` — Ben Johnson — https://github.com/benbjohnson/wtf/blob/main/error.go
- OpenTelemetry Go changelog (1.47.0: logs stable, 2026-10-02) — https://github.com/open-telemetry/opentelemetry-go/blob/main/CHANGELOG.md
- OpenTelemetry Go contrib changelog (otelhttp 0.72, 2026-10-03) — https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/CHANGELOG.md
- golangci-lint configs as of Nov 2026 — golangci-lint (https://github.com/golangci/golangci-lint/blob/main/.golangci.yml), Prometheus (https://github.com/prometheus/prometheus/blob/main/.golangci.yml), Kubernetes (`hack/golangci.yaml`), Grafana, OTel-Go, Traefik; golangci-lint v2.15.0 changelog and source (2026-10-16)
- Source read in the module cache: kin-openapi v0.153.0 (`openapi3/schema_error.go`, `schema_jsonschema_validator.go`), nethttp-middleware v1.3.1, pgx v5.13.0 `pgconn/errors.go`, Go 1.27.4 `net/http/server.go`, `log/slog`, `testing/synctest`
- caarlos0/env v11.4.2 (2026-11-02); testcontainers-go v0.45.0 (2026-11-05) — https://pkg.go.dev/github.com/caarlos0/env/v11, https://github.com/testcontainers/testcontainers-go
- pgtestdb — Peter Downs — v0.1.1 (Oct 2024) — https://github.com/peterldowns/pgtestdb
