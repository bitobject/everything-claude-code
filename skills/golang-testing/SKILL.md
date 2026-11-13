---
name: golang-testing
description: How to test the Go backend layer by layer - entity (table-driven, cmp.Diff, fuzzing), usecase (the in-memory store fake and the store contract suite it shares with PostgreSQL), repository/postgres (real PostgreSQL via pgtest, one database per test, the application role), delivery/httpapi (real use cases through the production API stack, every response checked against the OpenAPI contract and its problem codes), workers (golden JSON of job args, cancel or retry), end-to-end API tests on PostgreSQL, architecture tests, testing/synctest for time, b.Loop benchmarks, golden files. Use when writing, fixing or reviewing Go tests.
---

# Go testing

Binding decisions live in `STACK.md` (testing pyramid, versions). This skill shows how to apply
them; every example is from `../go-clean-architecture/reference/backend/`. The TDD loop that
drives these tests is in the `tdd-workflow` skill.

## Rules

- Test behavior through the exported API (`package foo_test`): tests of private functions break
  on every refactor and prove nothing about callers.
- Stdlib `testing` + `go-cmp`: `if got != want { t.Errorf(...) }` and `cmp.Diff(want, got)`, no
  assertion library. Messages: `Func(args) = got, want want`; diffs say `(-want +got)`.
- `t.Fatal` when the test cannot continue, `t.Error` to report and keep checking.
- `t.Context()` for every call that takes a context. It is cancelled just before the `Cleanup`
  functions run, so cleanups use `context.WithoutCancel(t.Context())` (with a timeout when they
  wait on something).
- Always `-race`. `-shuffle=on` runs in CI (`make ci`): it disables the test cache, so the local
  loop leaves it out.
- **PostgreSQL tests**: with `-short` they skip (the fast local loop, `make test` without
  `TEST_DATABASE_URL`); without `-short` and without `TEST_DATABASE_URL` they fail. CI sets the
  URL, so a missing database can't produce a green build.
- Mark helpers with `t.Helper()` so failures point at the caller.
- One behavior per test or per table row; name it after the behavior
  (`TestCreateLeadKeyReusedWithAnotherBody`).
- Test data is generated: phone numbers like `+79991234567`, never real client data.

## Which test for which layer

| Layer | Test | Doubles | Where in the reference |
|---|---|---|---|
| `entity` | pure unit, table-driven, fuzz for parsers and value objects | none | `leads/entity/lead_test.go` |
| `usecase` | the use case on the in-memory store, fixed clock and IDs | `usecasetest.Store` | `leads/usecase/usecase_test.go` |
| storage ports | **one contract suite**, run on the fake and on PostgreSQL | none on PostgreSQL | `usecasetest.RunStoreContract` |
| `repository/postgres` | the suite + what only PostgreSQL shows (events in the transaction, locks) | none: real database, real migrations | `leads/repository/postgres/repository_test.go` |
| `delivery/httpapi` | requests through the production API stack, real use cases, fake storage | `usecasetest.Store` | `leads/delivery/httpapi/handler_test.go` |
| `internal/api` | requests the contract rejects: 400/401/404/405/413/415 as problems | a server that must not be reached | `internal/api/api_test.go` |
| `delivery/worker`, thin workers | golden JSON of the args, cancel vs retry, idempotency on PostgreSQL | a fake facade or the fake store | `leads/delivery/worker`, `notifications` |
| `cmd/api`, `cmd/worker` | end to end on PostgreSQL through what `main` wires | none | `cmd/api/api_test.go`, `cmd/worker/worker_test.go` |
| architecture | struct tags, `Restore*` callers, SQL schemas, River migrations | none | `internal/archtest`, `migrations/river_test.go` |
| browser journeys | Playwright from `web/` (`e2e-runner` agent) | none | — |

Most tests go to `entity` and `usecase`: fast, and they cover the business rules. Adapter tests
prove the translation (SQL, HTTP, JSON) and nothing else.

## entity: table-driven

```go
func TestParsePhone(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{name: "valid", in: "+79991234567"},
		{name: "missing plus", in: "79991234567", wantErr: entity.ErrInvalidPhone},
		{name: "too short", in: "+7999123456", wantErr: entity.ErrInvalidPhone},
		{name: "other country", in: "+19991234567", wantErr: entity.ErrInvalidPhone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := entity.ParsePhone(tt.in)
			if !errors.Is(err, tt.wantErr) { // errors.Is(nil, nil) is true
				t.Fatalf("ParsePhone(%q) error = %v, want %v", tt.in, err, tt.wantErr)
			}
			if err == nil && p.E164() != tt.in { // String() masks the number
				t.Errorf("ParsePhone(%q).E164() = %q, want %q", tt.in, p.E164(), tt.in)
			}
		})
	}
}
```

- Aggregates keep fields unexported: compare what callers observe (getters, a view struct), not
  the private layout (`cmp.Diff` on unexported fields panics without options).
- Pass time and IDs in (`NewLead(id, …, now)`), so the test needs no clock and no fake.
- Cover the edges the rule names: bounds (`3` and `200` runes), Unicode (count runes, not bytes),
  trimming, zero values refused, every state transition and every forbidden one.
- Personal data masking is a behavior: `fmt.Sprint`, `%+v` and a slog JSON line of a `Phone`
  never contain the number (`TestPhoneIsMaskedWhenPrinted`).
- Check `apperr.KindOf(err)` when the kind matters to callers (HTTP status, retry).

Fuzz anything that parses untrusted input, asserting properties, not exact outputs:

```go
func FuzzParsePhone(f *testing.F) {
	f.Add("+79991234567")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		p, err := entity.ParsePhone(s)
		if err == nil && p.E164() != s {
			t.Errorf("valid phone %q round-trips as %q", s, p.E164())
		}
	})
}
```

Run `go test -fuzz=FuzzParsePhone -fuzztime=30s ./internal/leads/entity`; commit the failing
inputs Go writes to `testdata/fuzz/`, they become regression cases.

## usecase: the in-memory store

`usecase/usecasetest` (only test files import it; go-arch-lint fails production code that does):
- `Store` implements every storage port of the module (`UnitOfWork`, the repositories, the query
  port, the purge port). It keeps values, not pointers, so a change survives only if the port
  saves it. `Do` runs on a copy of the state that replaces it only when `fn` returns nil: a
  failed unit of work stores nothing, like a rollback. `Create` refuses a taken ID, like a
  primary key. `Fail(err)` makes every call fail, like a database that is down. `Events()`
  returns the events of committed units of work.
- `FixedClock{T}`; IDs are injected as `func() uuid.UUID` (production passes `uuid.NewV7`).

```go
func TestCreateLeadStoresAndPublishes(t *testing.T) {
	store := usecasetest.NewStore()
	got, err := newCreateLead(store, usecasetest.Now).Handle(t.Context(), input("key-0123456789abcdef"))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	want := usecase.LeadView{ID: leadID, Topic: "Divorce", City: "Kazan", Status: "new", CreatedAt: usecasetest.Now}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Handle() mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]usecase.Event{contract.LeadCreated{LeadID: leadID}}, store.Events()); diff != "" {
		t.Errorf("published events mismatch (-want +got):\n%s", diff)
	}
}
```

What to test in a use case:
- the output compared with `cmp.Diff` against a full `want`;
- every error branch: invalid input never reaches storage; storage errors are wrapped
  (`errors.Is(err, boom)` after `store.Fail(boom)`); domain errors keep their kind;
- what must happen together (the lead and its event) and that nothing happens when a later step
  fails;
- idempotency and limits (`TestCreateLeadRepeatedKeyReturnsTheFirstLead`,
  `TestCreateLeadLimitPerPhone`), one row per rule.

### The store contract suite

A fake that drifts from PostgreSQL makes use-case tests lie. `usecasetest.RunStoreContract(t,
newStore)` is the behavior the use cases rely on: create and get, a taken ID fails, not found,
`Update` saves the whole aggregate and leaves it unchanged when `fn` fails, a failed unit of work
stores nothing and frees its idempotency key, the claim semantics, the purge, the read side. The
fake runs it in `usecase_test.go`, the adapter in `repository_test.go`:

```go
func TestStoreContract(t *testing.T) { // repository/postgres
	usecasetest.RunStoreContract(t, func(t *testing.T) usecasetest.StoreUnderTest {
		return postgres.NewStore(pgtest.New(t, migrations.FS), events.NewBus(nil))
	})
}
```

A new port method gets a subtest in the suite before either implementation is written.

## repository/postgres: real PostgreSQL

Never fake the database in adapter tests: the SQL, constraints, locks, privileges and error codes
are what you are testing.

`pgtest.New(t, migrations.FS)` (`internal/platform/postgres/pgtest`):
- `TEST_DATABASE_URL` is the schema owner's DSN on a PostgreSQL 18 server (CI: a service
  container; locally: docker compose or a local server). The role needs `CREATEDB`; the README
  of the reference has the one-time role setup.
- A template database is migrated once per set of migrations (its name is a hash of the files,
  built under a temporary name and renamed, under an advisory lock, so parallel packages share
  it and a stale one is never used); each test gets a clone, dropped `WITH (FORCE)` at the end.
- The pool switches to the application role (`SET ROLE app_runtime`), so a missing `GRANT` fails
  a test, not production.
- No `TRUNCATE`, no shared fixtures; tests that own their database can use `t.Parallel()`.

Cover, per adapter:
- the store contract suite;
- events and jobs exist only when the transaction commits
  (`TestPublishInsertsJobsInTheTransaction` counts the subscriber's rows in `river.river_job`
  after a commit and after a rollback);
- every constraint you map → the right `apperr` kind; corrupt stored data → an internal error;
- concurrency where it matters:

```go
func TestConcurrentAssignHasOneWinner(t *testing.T) {
	leads := postgres.NewStore(pgtest.New(t, migrations.FS), events.NewBus(nil)).Leads()
	l := usecasetest.NewLead(t, "+79991234567", usecasetest.Now)
	if err := leads.Create(t.Context(), l); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = leads.Update(t.Context(), l.ID(), func(l *entity.Lead) error { return l.Assign() })
		})
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, entity.ErrNotAssignable):
			t.Errorf("Update() unexpected error: %v", err)
		}
	}
	if won != 1 { // FOR NO KEY UPDATE serializes; the losers see the assigned state
		t.Errorf("successful assigns = %d, want 1", won)
	}
}
```

- keyset pagination: page boundaries, ties on the sort key, the empty last page.

## delivery/httpapi: the production stack and the contract

`apitest.Serve(t, si, req)` (`internal/api/apitest`) runs the request through what `cmd/api`
mounts (body limit, request validation against `api/openapi.yaml`, the strict handler, the error
writer) and `apitest.CheckContract` checks the response: its schema for that status and media
type and, for a problem, that its `code` is listed in the operation's `x-problem-codes` for that
status. The handler gets the real use cases on the fake store:

```go
func newServer(store *usecasetest.Store) openapi.StrictServerInterface {
	return httpapi.New(httpapi.UseCases{
		Create: usecase.NewCreateLead(store, usecasetest.FixedClock{T: usecasetest.Now}, uuid.NewV7),
		Get:    usecase.NewGetLead(store.Queries()),
		Assign: usecase.NewAssignLead(store.Leads()),
	})
}

rec := apitest.Serve(t, newServer(usecasetest.NewStore()), createRequest(t, body))
```

Cover, per operation:
- success: status, body, headers the contract declares (`Location`, `Retry-After`);
- each error the use case can return → its status and problem code (`TestErrorsBecomeProblems`;
  an internal error shows no internal text);
- invalid requests → 400 `request.invalid` with `errors[]` of `{pointer|parameter, code}`, and
  **never the submitted values** (`TestInvalidRequestsNeverEchoInput` searches the body for them);
- secured operations without the session cookie → 401 (`TestSecuredOperationsNeedTheSessionCookie`).

Requests that never reach a handler (unknown path 404, wrong method 405 with `Allow`, too large
413, wrong media type 415, malformed JSON) are tested once for the whole API in
`internal/api/api_test.go`.

## Workers and jobs

- **Job args JSON is pinned** (`TestLeadCreatedArgsJSON`): marshal to a golden string and back.
  Change the golden only by adding fields: queued jobs must still decode after a deploy.
- **Outcome**: a job whose subject is gone is cancelled (`errors.AsType[*rivertype.JobCancelError]`),
  other errors are returned for a retry (`TestWorkCancelsWhenTheLeadIsGone`,
  `TestWorkRetriesOtherErrors`).
- **Idempotency** on PostgreSQL: run `Work` twice for one job, expect one effect
  (`TestWorkRecordsOneMessagePerLead`).
- Call `Work` directly with `&river.Job[Args]{JobRow: &rivertype.JobRow{}, Args: …}`; no River
  client is needed.
- **Wiring** of `cmd/worker` on PostgreSQL: build the client with the production function,
  subscribe to `river.EventKindJobCompleted`, start it and wait (with a timeout) for the job you
  expect (`TestPeriodicPurgeRunsOnStart`).

## End to end on PostgreSQL

`cmd/api/api_test.go` builds the handler with the same `newAPI` function `main` uses, on
`pgtest.New`, and runs a scenario: create, repeat with the same `Idempotency-Key` (same body
back), assign with the session cookie, and exactly one subscriber job in `river.river_job`.
Every response goes through `apitest.CheckContract`. One or two such tests per binary: they
prove the wiring, the layer tests prove the rules.

## Architecture tests

- `internal/archtest`: no struct tags in `entity`/`usecase` (test-support directories excepted),
  `entity.Restore*` called only from `repository/`, and every module's `queries/*.sql` names only
  its own schema (not another module's, not `river`). A new module is found automatically.
- `migrations/river_test.go`: the `*_river_*.sql` files are River's migrations for the River
  version in `go.mod`, unchanged.
- go-arch-lint, depguard and these tests are checked against deliberate violations when they are
  changed (the reference README lists the ones that were).

## Time, timers and concurrency: testing/synctest

Never `time.Sleep` in a test to wait for something. Run time-dependent code in a bubble: time
is fake, starts at 2000-01-01 00:00 UTC and advances only when every goroutine in the bubble is
durably blocked, so a 5-minute timeout takes microseconds and is exact.

```go
func TestSendTimesOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Go 1.27: NewTestServer uses an in-memory network that works inside a bubble,
		// and registers its own Cleanup.
		srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(time.Minute): // a provider that hangs
			case <-r.Context().Done():
			}
		}))
		c := sms.New(srv.Client(), "http://sms.example", 5*time.Second) // any host reaches srv

		start := time.Now()
		err := c.Send(t.Context(), "+79991234567", "hi")

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Send() error = %v, want deadline exceeded", err)
		}
		if got := time.Since(start); got != 5*time.Second {
			t.Errorf("Send() returned after %v, want exactly 5s", got)
		}
	})
}
```

- `synctest.Sleep(d)` (Go 1.27) = `time.Sleep(d)` + `synctest.Wait()`: advance the clock, then
  let the code under test settle before asserting. `synctest.Wait()` alone waits until every
  other goroutine in the bubble is durably blocked.
- Durably blocking: bubble channels and `select` on them, `time.Sleep`, `sync.WaitGroup.Wait`,
  `sync.Cond.Wait`. Not: mutexes, real network I/O, syscalls. So PostgreSQL or a loopback server
  cannot run inside a bubble; use `httptest.NewTestServer` (in-memory) and fakes.
- Use cases take a `Clock` port: a fixed fake is simpler than a bubble when only "now" matters.

Other concurrency rules:
- `t.Fatal`/`t.FailNow` only from the test goroutine (vet `testinggoroutine` flags it).
- Start goroutines with `wg.Go(func() { ... })` (Go 1.25) and wait before asserting.
- `t.Parallel()` for tests that own their state; not with `t.Setenv` or `t.Chdir`.

## Golden files

For large, stable outputs (generated documents, rendered emails) and for job args (above), not
for JSON API responses (the contract check covers those). Pattern:
`var update = flag.Bool("update", false, "rewrite golden files")`; the test writes
`testdata/<t.Name()>.golden` when `*update` is set, then always reads it and compares with
`cmp.Diff(string(want), string(got))`. Regenerate with `go test ./pkg -run TestName -update` and
review the golden diff like code. Make output deterministic first (sorted keys, fixed time and
IDs). Debug output that should not be committed goes to `t.ArtifactDir()` (kept with
`-artifacts`).

## Benchmarks

```go
func BenchmarkParsePhone(b *testing.B) {
	for b.Loop() { // Go 1.24: setup above the loop is excluded; results are kept alive
		if _, err := entity.ParsePhone("+79991234567"); err != nil {
			b.Fatal(err)
		}
	}
}
```

Run `go test -run='^$' -bench=. -benchmem -count=10 ./pkg > new.txt` and compare with
`benchstat old.txt new.txt` (`golang.org/x/perf/cmd/benchstat`). Benchmark hot paths only, to
answer a question.

## Commands

```bash
make test                                              # -race; adds -short when TEST_DATABASE_URL is unset
TEST_DATABASE_URL=postgres://app_owner:…@localhost:5432/postgres?sslmode=disable make test
go test -race -short ./...                             # the fast loop, no database
go test -race -run 'TestCreateLead/key' ./internal/leads/usecase
go test -count=1 ./internal/leads/repository/postgres  # bypass the test cache
go test -race -count=20 -run TestFlaky ./pkg           # reproduce a flake
go test -shuffle=1731422400 ./pkg                      # replay the order a CI failure reported
go test -fuzz=FuzzParsePhone -fuzztime=30s ./internal/leads/entity
go test -coverprofile=cover.out ./... && go tool cover -func=cover.out
```

`go test` runs a subset of `go vet` (including `printf` and, in Go 1.27, `stdversion`), so a vet
failure stops the test run: fix it, do not disable it.

## Coverage is a signal, not a target

An uncovered branch in `entity` or `usecase` is usually a missing behavior test: ask which rule it
protects and test that. Never chase a percentage; tests written for a number assert nothing.
Exclude generated code (`sqlcgen/`, `gen/openapi/`) when reading numbers; see `/ecc:test-coverage`.

## Anti-patterns

| Anti-pattern | Why it hurts | Do instead |
|---|---|---|
| Mocking pgx, `sqlcgen.Queries` or `http.Client` | Tests the mock; SQL, constraints and locking stay untested | Real PostgreSQL in adapter tests; the fake store in use-case tests |
| A fake without the contract suite | It drifts from PostgreSQL and use-case tests pass on behavior production doesn't have | `RunStoreContract` on both |
| Faking the use cases in handler tests | Tests a copy of the use case's behavior | Real use cases on the fake store |
| Asserting call sequences (`EXPECT().Times(1)`) | Breaks on every refactor; says nothing about outcome | Assert returned values and store state |
| `time.Sleep` to wait for goroutines or timers | Slow and flaky | `testing/synctest`, channels, `wg.Wait()`, River's event subscription |
| Asserting on log output | Logs are not behavior | Assert returned errors, state, published events, jobs |
| Testing unexported functions | Couples tests to structure | Test through the exported API |
| Shared database state, fixed IDs across tests, `TRUNCATE` between tests | Order-dependent, cannot run in parallel | `pgtest.New(t, …)` per test, `uuid.NewV7()` |
| `t.Skip` when the database is missing | Green build that tested nothing | Fail; skip only under `-short` |
| Tests as the database owner | A missing `GRANT` passes in tests and fails in production | `pgtest` connects as the application role |
| `context.Background()` in test bodies | Calls outlive the test | `t.Context()`; `context.WithoutCancel(t.Context())` in cleanups |
| `t.Fatal` inside a spawned goroutine | Does not stop the test; vet flags it | Report back to the test goroutine |
| `reflect.DeepEqual` and `%v` dumps | Unreadable failures | `cmp.Diff(want, got)` |
| Writing tests to reach a coverage number | Assertion-free tests | One behavior per test |
