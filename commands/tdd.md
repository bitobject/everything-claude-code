---
description: Implement a feature, a plan step or a bug fix test-first via the tdd-guide agent - Go inside out (entity, use case on the fake store, repository on real PostgreSQL with the store contract suite, contract + delivery through the production API stack), web with Vitest/Testing Library and Playwright journeys.
---

# /ecc:tdd

Delegate to the **tdd-guide** agent with the requirement given as arguments: a plan step
(`docs/plans/<slug>.md step <n>`), a feature, a bug report, or a refactoring goal. The agent
follows the `tdd-workflow` skill.

## What happens

1. **Plan.** With a plan step, the agent reads `docs/plans/<slug>.md` (it must say
   `Status: approved`), implements only that step and its acceptance criteria, and ticks the
   step's box in the same change. Without a plan, the requirement is restated as observable
   behaviors mapped to layers; anything non-trivial goes to `/ecc:plan` first.
   Ambiguities are asked about, not guessed.
2. **Red → green → refactor per behavior**, inside out:
   - `entity`: table-driven test of the rule → implement the rule.
   - `usecase`: test the use case on the fake store (`usecasetest.Store`, fixed clock and IDs) →
     implement it as its own type; new ports are declared here, with a subtest in the store
     contract suite.
   - `repository/postgres`: migration → sqlc query → the contract suite and adapter tests on
     PostgreSQL (`pgtest`) → adapter.
   - contract + `delivery/httpapi`: edit `api/openapi.yaml` (`make api-lint`), generate both
     sides, test through `apitest.Serve` with the real use cases (responses checked against the
     contract) → handler and wiring. They merge together: the generated interface needs the
     handler.
   - web: Vitest + Testing Library for components and forms; journeys through Playwright
     (`/ecc:e2e`).
   Every red run is shown and must fail for the intended reason.
3. **Finish**: `go test -race ./...` with `TEST_DATABASE_URL` set when storage, workers or wiring
   changed (and the web suite when touched), then the quality gate (`/ecc:verify`).

## Example

```text
/ecc:tdd closing an assigned lead records closed_at and asks the client for feedback
```

```text
Behaviors
  1. assigned lead → closed, closed_at = now (UTC)
  2. new or closed lead → 409 lead.not_closable
  3. LeadClosed v1 published in the same transaction (notifications asks for feedback)
  4. POST /v1/leads/{leadId}/close → 200 Lead | 404 | 409 problem+json

RED      entity   TestLeadClose/new_lead_cannot_close           want ErrNotClosable, got <nil>
GREEN    entity   Lead.Close
RED      usecase  TestCloseLeadPublishesLeadClosed              events (-want +got): -LeadClosed
GREEN    usecase  CloseLead.Handle: Update + Publish inside UnitOfWork
RED      postgres TestStoreContract/update_saves_the_whole_aggregate   closed_at = <nil>
GREEN    postgres 20261113120000_leads_closed_at.sql, UpdateLead sets closed_at, adapter
RED      httpapi  TestCloseLead/conflict                        status 500 (stub), want 409
GREEN    httpapi  Handler.CloseLead, x-problem-codes 409 lead.not_closable
REFACTOR shared status parsing in the adapter

TEST_DATABASE_URL=… go test -race ./...   ok
```

## Rules

- No production code without a failing test that requires it.
- One behavior per test; assert outcomes, not calls; no sleeps, no shared database state.
- Coverage is read afterwards as a hint for missing behaviors (`/ecc:test-coverage`), never as a
  target.

## Related

`/ecc:go-test` (run or repair tests) · `/ecc:go-build` (build errors) · `/ecc:e2e` (Playwright) ·
skills `tdd-workflow`, `golang-testing`, `postgres-patterns`.
