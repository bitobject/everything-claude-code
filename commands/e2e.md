---
description: Write, run or stabilize Playwright E2E tests for web/ against the production Next.js build and the real Go API, with axe accessibility checks and SEO HTML smoke tests; test data goes through the API. Delegates to the e2e-runner agent.
---

# /ecc:e2e

Delegate to the **e2e-runner** agent with the request below. It follows the rules in its definition: production build
(`next start`), real Go API and database, data created through the typed API client, user-facing locators, no sleeps,
lead-form journeys with and without JavaScript (asserting the kept input with `toHaveValue`), axe with the WCAG 2.2
tags on key pages, raw-HTML checks for SEO routes.

## Arguments

$ARGUMENTS can be:

- a journey to cover, e.g. `lead form on a city landing without JavaScript`, `lawyer accepts a lead in the cabinet`
- `run [spec path] [--project=desktop|mobile]` to run existing tests and report
- `flaky <spec path>` to reproduce with `--repeat-each=10`, find the cause and fix it
- `seo [route]` to add or run the YandexBot raw-HTML and status-code checks for a route type

With no arguments: run the whole suite and report.

## Preconditions (the agent checks them first)

1. The Go API is reachable at `E2E_API_URL` (default `http://localhost:8080`), migrated and seeded through
   the API. If not, it tells you how to start it; it never seeds the database directly.
2. A production build exists (`web/.next/BUILD_ID`) and was built against that API (`API_INTERNAL_URL`) without
   analytics counter ids. If not: `pnpm build` in `web/`. A failing build goes to `/ecc:build-fix`.
3. Cabinet journeys: a seeded test lawyer and a way to get its session through the API. Login is by email, Yandex ID,
   VK ID or phone number and the flow is not decided yet: the agent uses what the contract defines for tests, asks
   when it defines nothing, and never drives Yandex ID or VK ID pages.

## What you get back

```
## E2E: <scope>
Run: <passed> passed · <failed> failed · <flaky> flaky (desktop, mobile) · <duration>
Failed: <file:line> "<test title>" [project]
  <expected vs observed> · Trace: <path>
  Cause: <probable cause in the app or the test>
  Fix proposed: <file:line and change>
axe: <violations per scanned page>
New/changed tests: <files>
```

New tests land in `web/e2e/*.spec.ts`; shared fixtures (`api` client, `makeAxeBuilder`) in `web/e2e/fixtures.ts`.
The agent does not weaken assertions, add sleeps or `skip` tests to get green; a quarantined test uses
`test.fixme(true, '<issue link>')`.

Related: `/ecc:build-fix` (build fails), `typescript-reviewer` agent (review new tests and app fixes),
`seo-specialist` agent (deeper SEO audit than the smoke tests).
