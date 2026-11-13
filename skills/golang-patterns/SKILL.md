---
name: golang-patterns
description: Idiomatic modern Go (1.22–1.27) for this backend — naming, errors, context, concurrency, interfaces, generics, iterators, JSON, slog logging with request IDs and redaction, configuration and secrets, the HTTP server, resources, performance, linters and tooling. Use when writing or reviewing any Go code; for where code belongs (layers, ports, modules) use go-clean-architecture.
---

# Go patterns (Go 1.27)

Structure (layers, ports, modules, transactions): the `go-clean-architecture` skill.
Tests: the `golang-testing` skill. Database: `postgres-patterns`. Versions: `${CLAUDE_PLUGIN_ROOT}/STACK.md`.
Working code: `${CLAUDE_PLUGIN_ROOT}/skills/go-clean-architecture/reference/backend/`.

## Principles

- Clear beats clever. Code is read far more than written; optimize for the reader who has no context.
- A little copying beats a little dependency. Standard library first; a new module is a decision.
- Make the zero value useful, or make it impossible to construct (unexported fields + constructor).
- Errors are values: handle each one once, with context.
- Don't communicate by sharing memory; share memory by communicating — but a mutex is fine.

## Use the current language and library

| Since | Write | Not |
|---|---|---|
| 1.21 | `min(a, b)`, `max`, `clear(m)`, `slices.Sort`, `slices.Contains`, `maps.Copy`, `maps.Clone` | hand-written loops and helpers |
| 1.22 | `for i := range n`; per-iteration loop variables | `for i := 0; i < n; i++`; `v := v` copies |
| 1.22 | `mux.HandleFunc("POST /v1/leads/{id}/assign", h)`, `r.PathValue("id")` | third-party routers |
| 1.23 | iterators: `func All() iter.Seq2[K, V]`, `maps.Keys`, `maps.Values`, `slices.Collect`, `slices.Backward` | callbacks with ad-hoc stop flags, index loops run backwards |
| 1.24 | `t.Context()`, `b.Loop()`, `os.OpenRoot` for user-influenced paths, json `omitzero`, `slog.DiscardHandler` | `context.Background()` in tests, `b.N`, `filepath.Join` + prefix checks |
| 1.24 | `tool` directive in `go.mod` (`go get -tool`) | `tools.go` with blank imports |
| 1.25 | `wg.Go(func(){…})`, `testing/synctest`, `http.NewCrossOriginProtection()` | `wg.Add(1); go …; defer wg.Done()`, sleeping tests, hand-made CSRF |
| 1.26 | `errors.AsType[*apperr.Error](err)`, `new(expr)` for optional pointer fields, `slog.NewMultiHandler` | `var e *T; errors.As(err, &e)`, `func ptr[T any](v T) *T` helpers |
| 1.26 | `go fix ./...` modernizers (`go fix -diff ./...` is part of `make check`) | manual idiom upgrades |
| 1.27 | stdlib `uuid.NewV7()`, `encoding/json/v2` options, `strings.CutLast`, `httptest.NewTestServer`, `synctest.Sleep` | `github.com/google/uuid` (depguard bans it), `strings.LastIndex` slicing |
| 1.27 | generic methods `func (s *Set[T]) Map[U any](…)` — they cannot satisfy interface methods | free functions only |

## Naming

- Packages: short, lowercase, one word, named for what they provide (`apperr`, `clock`, `httpapi`).
  No `util`, `common`, `helpers`, `models`, `types`. A package name never repeats in its exports:
  `clock.System`, not `clock.SystemClock`. Same-named packages of different modules are imported
  with aliases `<module><layer>` (`leadspg`, `leadsuc`).
- Initialisms keep their case: `ID`, `URL`, `HTTP`, `JSON` (`LeadID`, `parseURL`).
- Getters have no `Get`: `lead.Status()`; setters (rare in entities) are `SetX`.
- Receivers: one or two letters, consistent across methods (`func (l *Lead)`), never `this`/`self`.
- Errors: sentinels `ErrNotFound`, types `ValidationError`; messages lowercase, no punctuation,
  no "failed to" (`"insert lead: %w"`).
- Interfaces: role nouns (`LeadRepository`, `Clock`); one-method interfaces may use `-er` (`Notifier`).
- Short names for short scopes (`i`, `r`, `ctx`), descriptive names for package-level identifiers.

## Errors

```go
lead, err := repo.Get(ctx, id)
if err != nil {
	return LeadView{}, fmt.Errorf("get lead %s: %w", id, err) // context = what + which
}
```

- Wrap with `%w` when callers may need the cause, `%v`/`%s` to deliberately hide it (crossing a
  trust boundary, or turning a stored-data error into an internal one). Add the identifier you
  have; don't repeat what the callee already said.
- Compare with `errors.Is`; extract with `errors.AsType[*T](err)` (1.26). Never `err == ErrX` on a
  wrapped error, never `strings.Contains(err.Error(), …)`.
- Error types use pointer receivers; wrap `*T` only when `*T` (not `T`) implements `error`;
  `go vet` (1.27) flags `%w` with a pointer to a value-receiver error type.
- Handle once: either return it (wrapped) or log it and stop. Logging and returning duplicates noise.
- `errors.Join` for independent failures (closing several resources, validating many fields).
- An ignored error is explicit and explained: `_ = f.Close() // read-only file; nothing to recover`.
- `panic` only for programmer errors at startup (impossible config, `regexp.MustCompile` of a
  constant). Never across a package boundary as control flow.

## Context

- `ctx context.Context` is the first parameter of every function that does I/O or may block.
- Never store a context in a struct; never pass `nil`; `context.Background()` only in `main`
  (forbidigo allows it in `cmd/` and tests only).
- Derive deadlines at the edge (`context.WithTimeout`, the `httpserver.Timeout` middleware), and
  `defer cancel()` immediately.
- `context.WithoutCancel(ctx)` for work that must outlive the request or the signal (shutdown,
  `river.Client.Start`), always with a deadline of its own; `context.AfterFunc` to tie cleanup
  to cancellation; `context.Cause(ctx)` to report why.
- In tests `t.Context()` is cancelled before the `t.Cleanup` functions run: cleanups use
  `context.WithoutCancel(t.Context())` with a timeout.
- Values in context: request-scoped metadata only (request ID, authenticated principal, trace).
  Unexported key types; typed accessor functions (`logging.WithRequestID`). Never dependencies,
  never transactions.

## Concurrency

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(8)                         // bound the fan-out
for _, id := range ids {
	g.Go(func() error { return notify(ctx, id) }) // per-iteration id since 1.22
}
if err := g.Wait(); err != nil {
	return fmt.Errorf("notify lawyers: %w", err)
}
```

- Every goroutine has an owner and an exit path (context cancellation, closed channel). If you
  can't say when it stops, don't start it.
- Prefer `errgroup` (errors + cancellation) or `wg.Go` (1.25) over hand-rolled `Add/Done`.
- The sender closes a channel; receivers never close. Unbuffered by default; a buffer size is a
  claim you can justify.
- Protect shared state with a mutex next to the fields it guards; `defer mu.Unlock()`.
  `atomic.Int64`, `atomic.Bool` for flags and counters; `sync.OnceValue` for lazy init.
- `select` with `ctx.Done()` in every blocking loop. Since 1.23 an unreferenced timer is collected
  even if never stopped, so `time.After` in a loop no longer leaks; a `time.NewTimer` reused with
  `Reset` is still clearer in a hot loop.
- Run tests with `-race`. Test timeouts and tickers with `testing/synctest`, not sleeps.
- Diagnose leaks with the `goroutineleak` pprof profile (GA in 1.27).

## Interfaces and types

- Accept interfaces, return concrete types. Declare an interface where it is consumed, with only
  the methods that consumer calls.
- No interface until there are two implementations or a boundary (a port, a test fake at a layer
  boundary). Don't mock what you own inside a layer.
- Constructors `NewX(deps…) *X` take required dependencies positionally. Functional options only
  for many genuinely optional settings; config structs are often clearer.
- Distinct value types for values that must not swap places (`entity.Topic`, `entity.City`
  instead of two `string` parameters).
- Embedding is for composing behavior, not for "inheritance"; don't embed a type just to save
  typing if it leaks its whole method set into your API.
- Generics for algorithms and containers (`Set[T]`, `events.Subscribe[E]`), not for
  repositories, services or DI. If an interface works, prefer the interface.

## Data

- JSON: `encoding/json` is backed by json/v2 since 1.27 (faster unmarshal; error text differs).
  Use `omitzero` instead of `omitempty` for structs and time. In new code needing strictness
  (duplicate keys, invalid UTF-8 rejected by default) use `encoding/json/v2`. Entities have no
  tags: JSON shapes come from the OpenAPI contract. River job args are the exception (stored as
  JSON for weeks: tags, fields only added, a golden test).
- Time: store and compute in UTC; compare with `Equal`; durations typed (`time.Duration`), never
  bare ints with unit comments. Times that go to PostgreSQL are truncated to microseconds
  (`clock.System`).
- Money: integer minor units (kopecks) in a named type; never `float64`.
- Slices: preallocate when the size is known (`make([]T, 0, n)`); return `nil` for "none" unless
  the JSON contract needs `[]`.

## Logging (`log/slog`)

```go
log.InfoContext(ctx, "lead assigned", "lead_id", id, "lawyer_id", lawyerID, "elapsed_ms", time.Since(start).Milliseconds())
```

- One logger per process, `logging.New(os.Stdout, level)`: JSON lines, wrapped in
  `logging.ContextHandler`, which adds `request_id` from the context to every record. Without such
  a wrapper the `…Context` methods carry nothing. When tracing is added, the same handler adds
  `trace_id` and `span_id` (`trace.SpanContextFromContext(ctx)`).
- Always the `…Context` method when a context is in scope; static messages; key-value pairs only;
  `snake_case` keys; IDs and enums only. sloglint enforces all of this (`.golangci.yml`) and
  rejects the keys `phone`, `email`, `name`, `address`, `password`, `token`, `secret`, `dsn`,
  `body`.
- Personal data masks itself: a type holding it implements `String`, `GoString` where needed,
  and `LogValue` (`entity.Phone` prints `+7******4567`). Secrets are `config.Secret` (prints
  `[redacted]`; `Reveal()` at the single place that needs the value).
- Never log full request bodies or query strings (the access log writes the path without the
  query). The logger is injected, not global.
- What gets logged where: the HTTP access log (method, path, status, duration), errors by status
  in `httpserver.ErrorWriter` (unknown errors ERROR; 5xx from `apperr` and timeouts WARN; a client
  that went away DEBUG; 4xx not at all), job failures in River's logger.

## Observability

OpenTelemetry, in the reference (`platform/telemetry`, `httpserver/observe.go`,
`postgres.NewTracer`, `jobs.Middleware`):
- **Setup first.** `telemetry.Setup(ctx, "api"|"worker", log)` runs in `cmd` before the pool and
  the River client: instrumentations bind to the global providers when they are built. It sets the
  W3C `TraceContext` propagator, a resource (`service.name` from code, overridden by
  `OTEL_SERVICE_NAME`/`OTEL_RESOURCE_ATTRIBUTES`), and OTLP/HTTP exporters only for the signals
  whose endpoint is set (`OTEL_EXPORTER_OTLP_ENDPOINT` or per signal). Without an endpoint spans
  are still created, so log lines and jobs carry trace IDs. `defer telemetry.Flush(...)` runs last.
- **HTTP.** `httpserver.Observe` wraps the `/v1` chain, outermost; probes stay outside. Span names
  and `http.route` come from the route template (`Route` around the API's mux, `LabelRoute` where
  the validator answers): never `r.URL.Path`, so IDs never become metric series. A request that
  matches no operation is named by its method alone. `server.address`/`server.port` are fixed
  (the Host header is the client's), and `client.address` comes from `ClientIP`.
- **SQL.** `postgres.DefaultConfig` sets `NewTracer()`: a client span per query only inside a
  traced request or job (River's polling has no parent, so no spans), named by the sqlc query
  (`CreateLead`); the statement text has only placeholders. Never
  `otelpgx.WithIncludeQueryParameters`: arguments are phone numbers. `Open` records pool stats.
- **Jobs.** Every River client gets `Middleware: jobs.Middleware()` (otelriver): the inserting
  span's `traceparent` goes into the job's metadata, and `river.work/<kind>` links back to it.
- **Logs.** `logging.ContextHandler` adds `trace_id` and `span_id` next to `request_id`; a log line
  and its trace find each other by `trace_id`.
- **Attributes.** Labels and span attributes carry IDs and templates, never phone numbers, names or
  query strings with them. Business spans in use cases are not needed: the request, SQL and job
  spans already show where time goes; inner layers don't import OpenTelemetry.
- **Tests.** `telemetrytest.Install(t)` (before building the handler or client; no
  `t.Parallel`) records spans and metrics; see `observe_test.go`, `postgres_test.go`,
  `cmd/api/api_test.go` (the job's `traceparent`) and `cmd/worker/worker_test.go` (the link).
- Check the current versions before upgrading (`go list -m -versions`): otel 1.47.0, otelhttp
  0.72.0, otelpgx 0.13.0, otelriver 0.12.0 in the reference.

## Configuration and secrets

- `caarlos0/env/v11` into a `cfg` struct in `cmd/<binary>`: `required` for what must be set,
  `envDefault` for safe defaults, typed fields (`time.Duration`, `slog.Level`).
- DSNs and keys are `config.Secret` with `unset`: the variable is removed from the process
  environment once read, so child processes don't inherit it. The process's own
  `/proc/<pid>/environ` still shows what it started with.
- No config files read by inner layers: `cmd` passes plain values to constructors.

## HTTP server

`platform/httpserver` (reference):
- `http.Server` with `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` (30 s), `IdleTimeout`,
  `MaxHeaderBytes`. `WriteTimeout` doesn't cancel the handler's context, so the `Timeout`
  middleware gives every request a deadline shorter than it; `DeadlineExceeded` becomes 503
  `timeout`.
- Middleware order on `/v1/`, outermost first (`cmd/api/wire.go` `newHandler`): `Observe` →
  `RequestID` → `APIHeaders` (`Cache-Control: no-store`, `X-Content-Type-Options: nosniff` on every
  answer) → `AccessLog` → `Recover` (logs the stack, re-panics `http.ErrAbortHandler`) → `Timeout` →
  `LimitBody` (1 MiB) → `http.NewCrossOriginProtection` → the validated API.
- `/livez` (process up) and `/readyz` (database answers within 1 s, not draining). On SIGTERM:
  fail readiness, wait `SHUTDOWN_DRAIN_DELAY` for the load balancer to notice, then `Shutdown`
  with a grace period.
- The client IP for limits and captcha checks comes only from `httpserver.ClientIP`, which trusts
  `X-Forwarded-For` only from the proxies' addresses (security-review skill).

## Resources and I/O

- `defer rows.Close()` / `defer resp.Body.Close()` right after the error check.
- For writes, the `Close` error matters: `if err := f.Close(); err != nil { return … }`.
- `http.Client` always with a `Timeout` (or per-request context deadline); never the zero client
  for outbound calls. Reuse clients; they pool connections.
- Paths built from input: `os.OpenRoot(dir)` then `root.Open(name)` (1.24): it refuses `..` escapes.
- Randomness for tokens and IDs: `crypto/rand` (`rand.Text()`); `math/rand/v2` only for
  non-security jitter (depguard bans `math/rand`).

## Performance (measure first)

- Benchmarks with `for b.Loop() {}`; profiles with `go test -cpuprofile`/`-memprofile` and pprof.
- Hot paths: preallocate, `strings.Builder`, avoid `fmt.Sprintf` in loops, avoid conversions
  between `[]byte` and `string` in loops.
- `sync.Pool` only after a profile shows allocation pressure. PGO: commit `default.pgo` from
  production profiles; the Go team reports 2–14 % on CPU-bound programs.
- N+1 queries are the usual culprit, not Go: batch with `= ANY($1)`.

## Tooling

`make check` is the definition of done (verification-loop skill); the pieces:

```bash
gofmt -l .                                # no output = formatted
go vet ./...                              # runs in go test too (incl. stdversion since 1.27)
bin/golangci-lint run ./...               # .golangci.yml; LINT_FLAGS=--concurrency=1 on small machines
bin/go-arch-lint check                    # the dependency rule
go fix -diff ./...                        # modernizers; empty diff expected
go test -race ./...                       # CI adds -shuffle=on (it disables the test cache)
go tool govulncheck ./...
go mod tidy -diff                         # go.mod and go.sum are tidy
```

`${CLAUDE_PLUGIN_ROOT}/skills/go-clean-architecture/reference/backend/.golangci.yml` (v2 format, 0 issues on the reference
with golangci-lint 2.15.0) is the starting configuration; set `goimports.local-prefixes` to the
module path. What it adds beyond the standard set, and why:
- depguard: `google/uuid`, `pkg/errors`, `x/exp/slices`, `x/exp/maps`, `math/rand`, `io/ioutil`
  banned everywhere; `net/...`, `database/...`, `os` and `log` banned in `entity` and `usecase`
  (go-arch-lint can't see standard packages);
- forbidigo: `fmt.Print*`, `log.Print*/Fatal*/Panic*`, `context.Background/TODO` outside `cmd/` and
  tests, `time.Now` in `entity` and `usecase` (the Clock port);
- sloglint (above), usetesting (`t.Context`, `t.TempDir`, `t.Setenv`), govet `nilness`, errorlint,
  `exhaustive` with `default-signifies-exhaustive: false` (a new enum value breaks the build until
  every switch handles it), nolintlint (a `//nolint` names the linter and the reason), revive with
  its default rules kept.
- Off on purpose: contextcheck (noisy), rowserrcheck and sqlclosecheck (nothing to find with pgx +
  sqlc), copyloopvar and intrange (`go fix` does it), perfsprint (taste).

## Anti-patterns

- `util`/`common` packages; package-level mutable variables; `init()` with side effects.
- Naked returns in functions longer than a few lines; named results used as variables.
- `interface{}`/`any` parameters where a concrete type or a small interface fits.
- Returning interfaces from constructors; interfaces with one implementation and no boundary.
- Goroutines without cancellation; `time.Sleep` for synchronization.
- Ignored errors, `log.Fatal` outside `main`, `panic` as control flow.
- Pointer to an interface; pointer receivers mixed with value receivers on one type without reason.
- Reflection-based mapping/DI; `reflect` in business code.
- A secret or personal data in a log line, an error message, a URL or a metric label.
