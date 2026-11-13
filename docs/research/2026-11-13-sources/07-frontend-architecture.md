<!-- Independent reviewer's report, 2026-11-13: Frontend architecture: Next.js App Router at scale.
     Raw model output, kept for its sources and detail. Which claims were re-verified is stated in
     ../2026-11-13-best-practices-review.md (sections marked [проверено] / [опровергнуто]). -->

# Frontend architecture review (Next.js App Router), ecc plugin

## 1. Verdict
The rendering and data-flow basics are current and well chosen. Server Components are the default. Go is the only business authority, called through an internal URL with the cookie forwarded. `'use cache'` data functions sit over the remote API and are invalidated by tag webhooks. TanStack Query is kept to the cabinet, and API types are generated from the contract. This matches Next.js 16.4's own guidance ("External HTTP API, zero trust") and the pattern Next.js Commerce uses.

The weak side is what keeps a large frontend written by agents consistent:
- Boundaries rest on goodwill: there is nothing like go-arch-lint for `web/`.
- There is no type-aware lint and almost no accessibility lint.
- The forms example has a bug that loses user data.
- The cabinet has no server-provided-data pattern and no rule for choosing a mutation path.
- There is no decision on a UI system.
- The agent guidance ignores the version-matched docs and the AGENTS.md block that Next.js now ships.
- The TypeScript 7 stance is stricter than Microsoft and Next now recommend.

## 2. Confirmed
- Go owns the rules; Next forwards cookies and does not authorize; `proxy.ts` is not the auth boundary. Source: Next "Data Security" (External HTTP APIs, zero trust, auditing `proxy.ts`).
- Feature folders outside `app/`, a shared `components/` and `lib/`, folders named by domain. Sources: Bulletproof React; Next "Project structure" ("store project files outside app").
- `'use client'` on leaves, providers only in the cabinet layout, `server-only` on modules that hold secrets. Source: Next "Server and Client Components" and "Data Security".
- `'use cache'` + `cacheLife` + tags over a remote API, a webhook calling `revalidateTag(tag,'max')`, `updateTag` in actions. Sources: Next "Client-side data fetching" (mutation table); vercel/commerce `lib/shopify`.
- TanStack Query only where a browser cache is needed; a QueryClient per request on the server, a singleton in the browser, `staleTime` > 0. Source: Next's TanStack Query guide.
- The browser calls Go same-origin under `/api`; server code uses the internal URL. Sources: Next "Backend for Frontend" (proxying); midday uses `API_INTERNAL_URL` the same way.
- Expected errors are returned as values; unexpected ones are thrown to `error.tsx`. Source: Next "Error handling".
- Async Server Components are covered by E2E tests; Playwright runs against the production build. Source: Next testing guides.
- `strict`, `noUncheckedIndexedAccess`, `noImplicitOverride`. Source: Total TypeScript cheat sheet.
- ESLint + Prettier now, no React Compiler, no monorepo. Source: create-next-app 16.4 defaults ("ESLint, No React Compiler"). Typed-lint and boundaries plugins exist only for ESLint, and Oxlint's JS plugins are still alpha.
- eslint-config-next 16.4.6 already applies the `recommended` preset of `react-hooks` v7, which includes the compiler-era rules (checked in the package source).

## 3. Problems

**P1. The lead form clears the user's input on every failed submit.** Severity high · confidence high
- Where: `skills/nextjs-patterns/SKILL.md`, "Server Actions and forms" (`submitLead`/`LeadForm`); `agents/e2e-runner.md`, test "form marks an invalid name without losing input".
- What we say: the inputs are uncontrolled and have no `defaultValue`; the `invalid`, `rejected` and `failed` states return no values; the test asserts `toBeVisible()`, not the value.
- Evidence: React resets uncontrolled fields when a form action completes without throwing, and "returning a validation error instead of throwing also triggers this reset". The fix React gives is to return the submitted values and pass them as `defaultValue`, which also works before hydration. https://react.dev/reference/react-dom/components/form, https://react.dev/reference/react/useActionState
- Change: return string `values` in every non-success state and render them as `defaultValue`. Assert `toHaveValue` both with and without JavaScript. Alternatively use Conform: `submission.reply()` keeps the values, and `hideFields` drops the ones you don't want echoed.

**P2. Frontend boundaries are not enforced.** Severity high · confidence high
- Where: `STACK.md` §1/§3; the `nextjs-patterns` "Layout of web/" section; the eslint config in `coding-standards`.
- The backend is "enforced by go-arch-lint, not by goodwill". The web has only `server-only` plus prose in the reviewer agent.
- Evidence:
  - Bulletproof React forbids imports between features and enforces the direction shared → features → app with `import/no-restricted-paths`. https://github.com/alan2207/bulletproof-react/blob/master/docs/project-structure.md
  - eslint-plugin-boundaries v7 provides `boundaries/dependencies` policies. https://www.jsboundaries.dev/
  - OSS monorepos rely on package boundaries; next-forge also runs `turbo boundaries`. Formbricks bans raw `process.env` and `unstable_cache` through lint.
- Change: add eslint-plugin-boundaries, or `import/no-restricted-paths` (already installed with eslint-config-next). Proposed element types and rules:
  - `app` may import `feature`, `components` and `lib`.
  - `feature` may import `components` and `lib`; features never import each other and are composed in `app/`.
  - `components` may not import `lib/api`.
  - `lib/api/server.ts` and `lib/env.server.ts` may only be imported from `data.ts`, `actions.ts`, `route.ts` and pages/layouts.
  - No barrel files inside features.
  - `no-restricted-syntax` on `process.env` outside `lib/env*`.

**P3. The nonce-based CSP conflicts with static SEO pages.** Severity high · confidence high
- Where: `skills/security-review/SKILL.md`, "Headers and transport (web)" ("nonce-based for scripts"), versus `STACK.md` §3 Rendering.
- Evidence: with nonces "all pages must be dynamically rendered", and Partial Prerendering (PPR) is incompatible. Hash-based SRI (experimental) keeps static generation. https://nextjs.org/docs/app/guides/content-security-policy
- Change: use an SRI/hash-based or nonce-free CSP for prerendered routes, with nonces only on dynamic cabinet routes if needed. Agree this with the security reviewer.

**P4. No type-aware or accessibility lint; the reviewer agent does by hand what rules would do.** Severity medium · confidence high
- Where: `coding-standards`, `eslint.config.mjs`; `typescript-reviewer`, "HIGH: type safety/accessibility". We also claim eslint-config-next "already brings jsx-a11y".
- Evidence:
  - `eslint-config-next/typescript` is the non-type-checked `recommended` set. typescript-eslint "strongly recommend[s]" typed linting with `projectService`. https://typescript-eslint.io/getting-started/typed-linting/
  - eslint-config-next 16.4.6 turns on only 6 jsx-a11y rules, all as warnings (package source).
- Change:
  - Adopt `strictTypeChecked`, at minimum `no-floating-promises`, `no-misused-promises`, `switch-exhaustiveness-check` (makes `assertNever` optional) and `no-unnecessary-condition`.
  - Enable jsx-a11y `recommended`. The plugin's last release was 2024-10 and its peer range stops at ESLint 9; the fork `eslint-plugin-jsx-a11y-x` (0.2.0) supports ESLint 10.
  - Enforce kebab-case file names with `check-file`.
  - Optionally turn on `verbatimModuleSyntax`.
  - Then cut the reviewer's checklist down.

**P5. "TypeScript 7 deferred, no aliases" blocks the official migration path.** Severity medium · confidence high
- Where: `STACK.md` §4; `coding-standards` ("TypeScript version"); `rules/typescript.md`; `typescript-reviewer`, which flags npm aliases as HIGH.
- Evidence:
  - The TS 7.0 announcement (2026-07-08) recommends `"typescript": "npm:@typescript/typescript6@^6.0.2"` for tools that need the API, such as typescript-eslint, with TS 7 used for `tsc`. It reports 8–12× faster full builds. https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/
  - Next 16.4's `next build` runs the project-local `tsc` CLI and supports TS 7. https://nextjs.org/docs/app/api-reference/config/typescript
- Change: run TS 6 and TS 7 side by side as Microsoft describes. Check that `pnpm generate:api` still works under the alias, then update the reviewer rule.

**P6. The cabinet shows its data only after hydration.** Severity medium · confidence high
- Where: `nextjs-patterns`, "Cabinet (browser → Go API)". Prefetching is mentioned but no recipe is given.
- Evidence:
  - Next's TanStack guide (server-provided data): start `prefetchQuery` without awaiting it, dehydrate pending queries, wrap in `<HydrationBoundary>`, and override `queryFn` on the server "because the… relative URL only resolves in the browser". https://nextjs.org/docs/app/guides/client-side-data-fetching/tanstack-query
  - openapi-react-query 0.5.4 builds keys as `[method, path, init]` with no `baseUrl`, so `prefetchQuery($apiServer.queryOptions(...))` fills the same key the browser reads.
  - midday does exactly this, with a `server-only` client, `cache(makeQueryClient)` and an 8-second SSR timeout.
- Change: add a server-prefetch recipe to the skill. Retry only on 5xx, and handle 401 in one central place.

**P7. Two mutation paths with no rule for choosing, and no shared definition of cache keys.** Severity medium · confidence medium-high
- Where: `STACK.md` §3 ("Forms: Server Actions") versus `$api.useMutation` in the `nextjs-patterns` cabinet section.
- Evidence:
  - Server Actions are dispatched "one at a time per client" (https://nextjs.org/docs/app/guides/server-actions). That suits forms but not rapid cabinet actions.
  - Next's guide says to keep "the query key and server tag in the same cache contract". Its own example repo, next-spa-patterns, has a `lib/*-cache.ts` file per data type holding `{queryKey, tag}`.
- Change: write the rule:
  - Progressively enhanced or public forms use a Server Action.
  - Cabinet interactions use `$api.useMutation` plus key invalidation, plus `router.refresh()` when the screen has server-rendered parts.
  - Each feature gets a `cache.ts` holding its query keys and cache tags.

**P8. The Go API's RFC 9457 errors cannot reach individual fields.** Severity medium · confidence high
- Where: `submitLead` maps every 4xx to a single `code`; the Problem schema in `openapi-contract-first` has no field location.
- Evidence: RFC 9457 §3 shows a 422 with `errors: [{detail, pointer}]`, using JSON Pointer (https://www.rfc-editor.org/rfc/rfc9457.html). Conform's `reply({fieldErrors, formErrors})` maps onto this directly.
- Change: the contract adds `errors[]: {pointer, code}`. The web has one `problem-to-form-state.ts` that maps `/phone` to the `phone` field and each code to Russian copy. Field errors should be typed by the input's keys, not `Record<string, …>`.

**P9. Hand-written zod request schemas copy the contract.** Severity low-medium · confidence medium
- Where: `LeadInput`. `STACK.md` itself says "Hand-written DTOs that mirror the API are a bug" and promises "zod for client-side checks", but the example only validates on the server.
- Change: either generate request schemas (Hey API's zod plugin: zod v4, `requests: true`, https://heyapi.dev/docs/openapi/typescript/plugins/zod) and add Russian messages on top, or keep only the native `required`/`type` checks and rely on P8.

**P10. The agent guidance ignores Next's version-matched docs.** Severity medium · confidence high
- Where: `nextjs-patterns` (488 lines restating APIs); `examples/CLAUDE.md`.
- Evidence:
  - Next 16.2+ ships its docs in `node_modules/next/dist/docs/`. create-next-app 16.4 creates AGENTS.md by default, and `next dev` keeps its block up to date. https://nextjs.org/docs/app/guides/ai-agents
  - Vercel's evals: the AGENTS.md docs index reached 100%; skills reached 53% (79% with explicit instructions); the skill was never invoked in 56% of cases. https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals
  - Next's own example repo (next-spa-patterns) and formbricks use a `CLAUDE.md` that contains `@AGENTS.md`.
- Change:
  - Add `web/CLAUDE.md` containing `@AGENTS.md`.
  - Keep project decisions in the always-loaded `rules/typescript.md`.
  - Shrink `nextjs-patterns` to our own choices, linking to the bundled doc paths.
  - Consider `next-devtools-mcp`, whose `get_compilation_issues` and `compile_route` check compilation without a full build (https://nextjs.org/docs/app/guides/mcp).

**P11. The post-edit hook and the gate type-check badly.** Severity medium · confidence high
- Where: `hooks/scripts/post-edit.sh` (TypeScript block); `verification-loop` (`pnpm exec tsc --noEmit`).
- Problems:
  - It runs a whole-project `tsc` on every edit.
  - If it hits the 90-second timeout, it reports nothing.
  - It filters errors to the edited file, so importers broken by an exported-type change stay invisible.
  - It skips `next typegen`. Next's docs say plain `tsc --noEmit` doesn't validate route types (https://nextjs.org/docs/app/api-reference/cli/next).
- Change: use `pnpm typecheck` in the gate. In the hook, run `tsc --incremental` (or TS 7), report new errors in any file (capped), run typegen when a route file is added, and run `eslint --fix` on the edited file. Do the full check at Stop.

**P12. The axe scans skip WCAG 2.2.** Severity low · confidence medium-high
- Where: `e2e-runner` fixtures, `withTags([... 'wcag21aa'])`, while `coding-standards` targets WCAG 2.2 (24×24 touch targets).
- Evidence: axe 4.14's `target-size` rule is tagged `wcag22aa`, and WCAG 2.2 rules are off by default.
- Change: add `'wcag22aa'` and confirm `target-size` actually runs.

## 4. Gaps

**Now (from day one)**
- **UI system.** No primitives or tokens are chosen, but the cabinet will need dialogs, menus, selects and comboboxes. Use shadcn/ui on Base UI (shadcn's default since 2026-07; Radix is still supported, https://ui.shadcn.com/docs/changelog/2026-07-base-ui-default), or React Aria Components. Define tokens in Tailwind 4 `@theme`, keep the primitives in `components/ui`, and add `prettier-plugin-tailwindcss`.
- **A Server Action wrapper.** Either an in-house `defineAction(schema, handler)` or next-safe-action 8 (used by dub, midday and formbricks), so every action validates input, forwards the cookie/IP, maps the Problem response and returns typed state.
- **Test seams:**
  - An async container that passes props to a synchronous view, so the view is unit-testable.
  - Pure parse and Problem-mapping functions that test without Next mocks.
  - MSW 3 + openapi-msw 2.1, with handlers typed from `paths`, for cabinet components.
  - `React.cache` around per-request reads such as `getMe`.
- **Server API client resilience.** `lib/api/server.ts` has no timeout; add `AbortSignal.timeout` (midday uses 8 seconds) and request-id/`traceparent` headers.
- **Version skew on the lead form.** Server Action IDs rotate on every deployment, so a page opened before a deploy gets "Failed to find Server Action". Next's docs advise showing a retry path; the form should keep its values when this happens.
- **knip in pre-PR checks and CI.** Its Next plugin knows the App Router and `proxy` entry files.
- **`typedRoutes: true`**, which complements `lib/routes.ts`.

**Later (as the codebase or team grows)**
- Vitest Browser Mode with the Playwright provider and `vitest-browser-react`, for cabinet interactions.
- Playwright `toHaveScreenshot` for the 2–3 landing templates, in a pinned Docker image.
- pnpm workspaces + Turborepo (`turbo boundaries` is still experimental), only when a second JS app or shared package appears.
- `nuqs` for cabinet filter state in the URL.
- Oxlint as a fast pre-pass; its type-aware mode needs TS 7.
- React Compiler.
- Bundle budgets in CI (`next analyze --output` diff).
- The experimental `@next/playwright` `instant()` checks.

## 5. Over-engineering
- A whole-project `tsc` after every edit (see P11).
- Long manual review checklists for rules a linter can check: naming, `any`, floating promises, accessibility. They cost tokens and are unreliable.
- Running both the desktop and the mobile project for every E2E spec. Use mobile for public pages only.
- Things not to adopt:
  - Full FSD layers: they need renamed `_app`/`_pages` layers and re-exports, and add a lot of ceremony for a small team.
  - The taint APIs: still experimental, and cabinet data flows browser → Go anyway.
  - `exactOptionalPropertyTypes`: it fights the optional fields openapi-typescript generates.
  - Storybook or a monorepo now.

## 6. Sources
- Next.js 16.4.6 docs, read from the copy bundled in the package (Vercel, 2026): /docs/app/guides/{data-security, client-side-data-fetching/tanstack-query, server-actions, content-security-policy, ai-agents, mcp, backend-for-frontend}, /docs/app/api-reference/{config/typescript, config/eslint, cli/next, cli/create-next-app}, at https://nextjs.org/docs
- React `<form>` and `useActionState` docs — React team, current: https://react.dev/reference/react-dom/components/form, https://react.dev/reference/react/useActionState
- RFC 9457 — IETF, 2023-07: https://www.rfc-editor.org/rfc/rfc9457.html
- Announcing TypeScript 7.0 — Microsoft, 2026-07-08; Announcing TypeScript 6.0 — Microsoft, 2026-03-23: https://devblogs.microsoft.com/typescript/
- "AGENTS.md outperforms skills in our agent evals" — Jude Gao, Vercel, 2026-01-27: https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals
- typescript-eslint typed linting (v8.72.2, 2026): https://typescript-eslint.io/getting-started/typed-linting/
- Bulletproof React project structure — Alan Alickovic: https://github.com/alan2207/bulletproof-react/blob/master/docs/project-structure.md
- Feature-Sliced Design with Next.js (©2025): https://feature-sliced.design/docs/guides/tech/with-nextjs
- eslint-plugin-boundaries 7.4.3 — Javier Brea, 2026: https://www.jsboundaries.dev/
- Turborepo boundaries — Vercel, updated 2026-11-11: https://turborepo.dev/docs/reference/boundaries
- shadcn/ui changelog, July 2026: https://ui.shadcn.com/docs/changelog/2026-07-base-ui-default
- Vitest Browser Mode (v5.1): https://vitest.dev/guide/browser/
- Oxlint type-aware linting and JS plugins — Oxc, 2026: https://oxc.rs/docs/guide/usage/linter/type-aware.html
- Conform Next.js integration and v1 upgrade notes — Edmund Hung: https://conform.guide/integration/nextjs
- TSConfig cheat sheet — Matt Pocock, 2024-04-23: https://www.totaltypescript.com/tsconfig-cheat-sheet
- axe-core rule descriptions (4.14) — Deque: https://github.com/dequelabs/axe-core/blob/develop/doc/rule-descriptions.md
- Knip Next.js plugin: https://knip.dev/reference/plugins/next
- npm registry: versions and peer dependencies checked 2026-11-12.

## Open-source App Router codebases examined

| Repository | Structure | Data access and boundaries |
|---|---|---|
| [midday](https://github.com/midday-ai/midday) | Turborepo; `apps/dashboard` (Next 16.2) and `apps/api` (Hono + tRPC); about 30 packages; Biome | Prefetch in Server Components through a `server-only` tRPC client, then `HydrationBoundary`; internal API URL; next-safe-action; nuqs; boundaries are package boundaries |
| [formbricks](https://github.com/formbricks/formbricks) | Turborepo; `apps/web` (Next 16.4): `app/` routes, `modules/<feature>` (pages, actions, lib), `lib/<entity>/service.ts`; ADRs, AGENTS.md | next-safe-action wrapper; TanStack Query; react-hook-form; lint bans raw `process.env` and `unstable_cache`; typed lint introduced gradually |
| [dub.link (formerly dub)](https://github.com/dubinc/dub.link) | pnpm + Turborepo; `apps/web` (Next 15.5) organised by type (`lib/api`, `lib/actions`, `lib/swr`, `ui/<domain>`) | Route handlers, next-safe-action, SWR; no lint rules on imports inside the app |
| [next-forge](https://github.com/vercel/next-forge) | Turborepo template: apps (app, web, api, …) and packages (auth, design-system, seo, …); Biome via Ultracite | Package `exports` plus `turbo boundaries` |
| [vercel/commerce](https://github.com/vercel/commerce) | Single app: `app/`, `components/<domain>`, `lib/shopify` | One data module with `'use cache'` + tags over a remote API, plus a revalidate webhook (uses older experimental flags) |
| [next-spa-patterns](https://github.com/nextjs/next-spa-patterns) | Route colocation (`app/<x>/[id]/{data,…-cache,…-view}`) | `lib/*-cache.ts` files holding query key + tag; `CLAUDE.md` contains `@AGENTS.md` |
| [bulletproof-react (nextjs-app)](https://github.com/alan2207/bulletproof-react) | `features/*`, `app/**/_components`, shared `lib/` | `import/no-restricted-paths` zones; `check-file` naming |
| [documenso](https://github.com/documenso/documenso) | Moved off Next.js to React Router 7 + Hono | Separate `packages/lib/server-only/*` and `client-only` folders |
| [cal.diy (formerly cal.com)](https://github.com/calcom/cal.diy) | MIT fork of cal.com with enterprise code removed; Yarn + Turborepo; Biome | tRPC + Prisma (internals not examined) |
| [papermark](https://github.com/papermark/papermark) | Mixes Pages Router and App Router; folders by type | Not a useful App Router reference |

**Assumptions:** nothing was run or installed. I read the Next.js 16.4.6 docs from the copy bundled in the package. Claude Code honours `@` imports in CLAUDE.md, as the next-spa-patterns and formbricks repos rely on. I did not test whether openapi-typescript works under the `typescript` npm alias.
