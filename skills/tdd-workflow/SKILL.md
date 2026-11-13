---
name: tdd-workflow
description: Test-driven development for this repo - Go backend (red, green, refactor per Clean Architecture layer, in plan steps: entity + use case on the fake store, storage with the store contract suite on PostgreSQL, contract + delivery through the production API stack) and Next.js web (Vitest + Testing Library, Playwright journeys). Use when adding a feature, implementing a plan step, fixing a bug or refactoring, before writing production code.
---

# TDD workflow

Write the test that states the behavior, watch it fail for the right reason, make it pass with
the smallest change, then clean up with the tests green. The goal is behavior that is pinned
down by tests, not a coverage number.

Test patterns per layer: `golang-testing` skill. Database specifics: `postgres-patterns`.
Architecture and layer order: `go-clean-architecture`. Browser journeys: `e2e-runner` agent.

## The loop

1. **Red.** Write one test for one behavior. Run it. Read the failure: it must fail because the
   behavior is missing (wrong value, missing error), not because of a typo or a broken setup.
   A compile error on a symbol you have not written yet is an acceptable first red; add a stub
   that returns zero values so the next run fails on the assertion.
2. **Green.** Write the least code that passes. Hard-coding is allowed while only one case
   exists; the next table row forces the general rule.
3. **Refactor.** Remove duplication, improve names, move code to the layer it belongs to. Run the
   tests after every step. Never change behavior and structure in the same step.
4. Repeat with the next behavior. Commit when a behavior is green and clean.

Skip TDD only for throwaway spikes (delete them afterwards), generated code, and pure wiring
in `cmd` (covered by higher-level tests).

## Go: one feature, layer by layer

Work inside out, in the steps a plan uses (`planner` agent): **entity + use case → storage →
contract + delivery + wiring → web**. Each step compiles, is green and can merge before the next
starts, so a failure always points at the layer you just touched. The contract is designed first
(in the plan) but lands with its handler: the generated server interface doesn't compile without
it. With a plan, implement one step and tick its box in the same change.

### 1a. Entity: the rule

```go
func TestLeadClose(t *testing.T) {
	tests := []struct {
		name    string
		from    entity.Status
		wantErr error
	}{
		{name: "assigned lead closes", from: entity.StatusAssigned},
		{name: "new lead cannot close", from: entity.StatusNew, wantErr: entity.ErrNotClosable},
		{name: "closed lead cannot close again", from: entity.StatusClosed, wantErr: entity.ErrNotClosable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := entity.RestoreLead(uuid.NewV7(), phone, topic, city, tt.from, at) // tests may restore; production only in repository/
			err := l.Close(closedAt)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Close() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && l.Status() != entity.StatusClosed {
				t.Errorf("Status() = %q, want closed", l.Status())
			}
		})
	}
}
```

Red: `l.Close undefined` → add `func (l *Lead) Close(at time.Time) error { return nil }` →
red on "new lead cannot close" → implement the rule → green. Invariants, state transitions and
value objects live here and are tested here, with no fakes at all.

### 1b. Use case: orchestration on the fake store

One type per use case; the test decides the ports it needs. It runs on `usecasetest.Store`, the
in-memory store whose behavior the store contract suite pins against PostgreSQL.

```go
func TestCloseLeadPublishesLeadClosed(t *testing.T) {
	store := usecasetest.NewStore()
	lead := assignedLead(t, store) // helper: Create + Assign through the store
	uc := usecase.NewCloseLead(store, usecasetest.FixedClock{T: usecasetest.Now})

	_, err := uc.Handle(t.Context(), lead.ID())

	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	want := []usecase.Event{contract.LeadClosed{LeadID: lead.ID()}}
	if diff := cmp.Diff(want, store.Events()); diff != "" {
		t.Errorf("events (-want +got):\n%s", diff)
	}
}
```

Add tests for each error branch: not found, the domain error kept (`errors.Is`,
`apperr.KindOf`), a storage failure wrapped (`store.Fail(boom)`), nothing stored and nothing
published when a later step fails. A new port method first gets a subtest in
`usecasetest.RunStoreContract`, then its fake implementation.

### 2. Storage

1. `make migration name=<what>` if the schema changes → query in `queries/*.sql` → `make generate`.
2. The store contract suite runs on PostgreSQL (`TestStoreContract` in `repository_test.go`, with
   `TEST_DATABASE_URL`); add adapter tests for what only PostgreSQL shows: each mapped constraint,
   locking, events and jobs only on commit.
3. Implement the adapter method until green.

Never skip this step because the fake is green: the suite is what makes the fake trustworthy.

### 3. Contract, delivery and wiring

1. Edit `api/openapi.yaml` (`make api-lint`), `go generate ./...`, regenerate the web client. The
   build fails: the handler lacks the new method. Add a stub that returns an error.
2. Test through `apitest.Serve` with the real use cases on the fake store: success with its
   headers, each error → status and problem `code` (listed in `x-problem-codes`), invalid input →
   400 without the submitted values. Replace the stub until green.
3. Wire in `cmd/api/wire.go` (constructors only); the end-to-end test on PostgreSQL covers the
   wiring.

Workers (`delivery/worker`, a thin module's job) follow the same idea: call `Work` with a fake of
what it calls and assert the outcome, the returned error (`jobs.Outcome`: cancel or retry) and
idempotency; pin the args with a golden-JSON test.

### 4. Check

`make check`, with `TEST_DATABASE_URL` set when storage, workers or wiring changed (say if the
PostgreSQL tests didn't run). CI runs `make ci` (shuffled order, the breaking-change gate). A
Playwright journey covers the feature end to end when users see it (see Web below).

## Bug fixes

1. Reproduce the bug with a failing test at the lowest layer that can show it (an entity rule
   bug gets an entity test, not an HTTP test).
2. Confirm it fails for the reported reason.
3. Fix with the smallest change; keep the test as the regression guard.
4. Ask whether the same mistake can exist elsewhere (other states, other modules) and add rows.

## Refactoring

- Start green. If the code has no tests, first write characterization tests through the public
  API that pin current behavior.
- Move in small steps, running the package tests after each.
- Tests that break on a pure refactor were testing structure: rewrite them against behavior.

## Web (Next.js)

Keep the business rules in Go; web tests prove parsing, rendering, interaction and the user journey. Write the
test first from the user's (or the contract's) point of view, as on the Go side.

| What | Tool | Notes |
|---|---|---|
| Pure logic: form parsing, problem mapping, submit flows, URL builders | Vitest | modules without Next imports; inject I/O (`send`) and pass fakes |
| What a form sends | Vitest + Ajv (JSON Schema 2020-12) | the schema comes from `api/openapi.yaml` itself (below) |
| Client Components, hooks | Vitest 5 + Testing Library | query by role and label; assert values and visible text, not component state |
| Cabinet components with data | Vitest + MSW 3 + openapi-msw | handlers typed from `paths` (below) |
| `proxy.ts` | Vitest + `next/experimental/testing/server` | `unstable_doesMiddlewareMatch` for the matcher, `getRedirectUrl` for the decisions |
| Synchronous views | Vitest | async Server Components stay thin containers that pass props to a view; cover them with Playwright |
| SEO output (metadata, canonical, JSON-LD, sitemap) | Vitest on the functions; Playwright on the raw HTML | indexable pages must have full content in the first response |
| Journeys (submit a lead, cabinet flows), accessibility | Playwright 1.65 + axe (`wcag22aa`) | real Go backend; `e2e-runner` agent (`/ecc:e2e`); lead forms also with JavaScript disabled |

```ts
// web/vitest.config.mts
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [react()],
  resolve: { tsconfigPaths: true }, // Vite 8: the @/ alias from tsconfig, no extra plugin
  test: { environment: 'jsdom', include: ['src/**/*.test.{ts,tsx}'] },
})
```

**The form sends what the contract accepts.** Compile the request schema from the contract and run the flow's
output through it, so a form field the contract lacks (or a phone format it rejects) fails a test, not production:

```ts
// web/src/features/leads/contract.test.ts (excerpt)
import Ajv2020 from 'ajv/dist/2020'
import { parse } from 'yaml'

const spec = parse(readFileSync(resolve(process.cwd(), '../api/openapi.yaml'), 'utf8')) as OpenApi
const ajv = new Ajv2020({ strict: true, allErrors: true })
const validateBody = ajv.compile(required(spec.components.schemas.CreateLeadRequest, 'CreateLeadRequest'))
const validateKey = ajv.compile(required(spec.components.parameters.IdempotencyKey, 'IdempotencyKey').schema)
// for each phone format the form accepts: the body passed to the fake `send` and the quoted key are valid,
// and a body with `name` or an unnormalized phone is rejected (proof that the validator bites)
```

The complete test, and the component tests below, are in `../../examples/web-lead-form.md` (relative to this
skill's directory).

**Mock the Go API at the HTTP level, typed from the contract:**

```ts
import { setupServer } from 'msw/node'
import { createOpenApiHttp } from 'openapi-msw'

const http = createOpenApiHttp<paths>({ baseUrl: 'http://localhost/api' })
const server = setupServer(
  http.get('/v1/leads/{leadId}', ({ params, response }) =>
    response(200).json({ id: params.leadId, topic: 'razvod', city: 'moskva', status, createdAt: '2026-11-13T10:00:00Z' }),
  ),
)
beforeAll(() => {
  server.listen({ onUnhandledFrame: 'error' }) // MSW 3 name; MSW 2 called it onUnhandledRequest
})
```

Gotchas (each one cost a failing run):
- **React 19 resets uncontrolled fields after every form action**, also one that returns an error. A test for "keeps
  the input" submits with a mocked action that returns an error state and asserts the input's `value`; it fails
  without `defaultValue={state.values…}`.
- A `'use server'` module imports `server-only`, which throws outside the server: mock it with
  `vi.mock('../actions', () => ({ submitLead }))` and create the mock with `vi.hoisted`.
- openapi-fetch keeps the `fetch` it was created with. A client built at module load, before `server.listen()`
  installs MSW, bypasses the mocks: in tests create it with `fetch: (request) => fetch(request)`. Under Node a
  relative base URL (`/api`) also needs an origin, so tests mock `@/lib/api/browser` with an absolute one.
- Vitest strips types without checking them, and `next build` type-checks the whole project, tests and e2e included:
  run `pnpm typecheck` before calling a test done.
- `catchError` (`next/error`) and `unstable_isUnrecognizedActionError` work in jsdom; mock `next/navigation` to
  simulate the deploy-skew error.

Run with `pnpm test` and `pnpm e2e`.

## Coverage

Coverage tells you where nobody looked; it does not tell you what is tested. After a feature,
read uncovered branches in `entity` and `usecase` and decide for each whether a behavior is
missing a test. There is no percentage gate. See `/ecc:test-coverage`.

## Done means

- Every new behavior and every bug fix has a test that failed before the change.
- Tests describe behavior in their names and assert outcomes, not calls.
- `make check` (with PostgreSQL when storage, workers or wiring changed) and `pnpm check` are
  green; nothing skipped except the PostgreSQL tests under `-short` in the local loop.
- The contract test covers each new or changed operation, including its error responses.
