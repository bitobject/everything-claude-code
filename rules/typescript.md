---
paths:
  - "web/**/*.ts"
  - "web/**/*.tsx"
---
# web/ TypeScript rules

Details: skills `nextjs-patterns`, `seo`, `coding-standards`. Versions: `STACK.md`. Next.js API: the docs of the installed
version in `web/node_modules/next/dist/docs/`, not memory. Done means `pnpm check` passes.

## Architecture
- Next.js holds no business rules and never talks to PostgreSQL; it calls the Go API.
- `src/lib/api/schema.ts` is generated: never edit it, never hand-write API types. Change `api/openapi.yaml`,
  run `make generate`, alias `components['schemas']['X']`.
- Server code calls Go through `@/lib/api/server` (openapi-fetch): `api` for cached public reads,
  `apiFor(await sessionHeaders())` at request time, `apiFor(ctx.forwarded)` in Server Actions. The cabinets use `$api`
  from `@/lib/api/browser` (openapi-react-query). No raw `fetch` to the API, no axios.
- Boundaries (lint): `app → features → components/lib`; a feature never imports another feature; `lib/api/server.ts`
  only from `features/*/data.ts` and `actions.ts`; `process.env` only in `lib/env.server.ts`/`lib/env.public.ts`.
- Forward the session cookie verbatim (`sessionHeaders()`), never `(await cookies()).toString()`.
- Error bodies: parse with `asProblem()`, branch on `code`; a non-2xx body may not be a problem document.

## Next.js 16, not 14/15
- Lint rejects the old APIs and names the fix (the Next.js 16 block of `eslint.config.mjs`, config in
  `coding-standards`): `middleware.ts`, `unstable_cache`, `export const revalidate|dynamic|dynamicParams|fetchCache`,
  `fetch(…, { next: { revalidate } })`, `updateTag` outside `features/*/actions.ts`, `cookies()`/`headers()` inside
  `'use cache'`, `return []` from `generateStaticParams`, `next/image` `priority`, `as Route` outside `@/lib/routes`.
- `src/proxy.ts` exports `proxy`. Caching is Cache Components: `'use cache'` + `cacheLife()` + `cacheTag()`; a cached
  function gets request values as arguments and returns plain `data`, not the openapi-fetch result.
- `params`, `searchParams`, `cookies()`, `headers()` are async: `await` them. Type with the global `PageProps<'/route'>`.
- `revalidateTag(tag, 'max')` takes two arguments; `updateTag(tag)` works only in Server Actions.
- `generateStaticParams` returns at least one real param; unknown params call `notFound()`.
- `next lint` is gone: `pnpm exec eslint .`. `error.tsx` receives `{ error: unknown; retry }`.
- The single LCP image gets `loading="eager"` and `fetchPriority="high"`. `typedRoutes` is on: hrefs come from
  `@/lib/routes`.

## Forms and mutations
- Public forms are Server Actions (work without JS); cabinet interactions use `$api.useMutation` + invalidation.
  Each feature keeps its query keys and cache tags in `cache.ts`.
- Every action is built with next-safe-action from `@/lib/api/action-client` with an `.inputSchema(…)`:
  `actionClient` for public forms, `sessionActionClient` in the cabinets. No bare `'use server'` functions.
- A public form's action is a `.stateAction()` passed straight to React's `useActionState`; never next-safe-action's
  `useStateAction` (it dispatches through a client wrapper, so the form stops working without JS).
- React resets the form after every action: validation errors carry the typed `values` through
  `handleValidationErrorsShape` (annotate its return type), Go's answers return them in `data`; inputs render them
  as `defaultValue`.
- Request bodies are object literals of parsed fields (no field the contract lacks); `Idempotency-Key` is minted in
  the browser at submit or by the action, never during render.

## SEO routes (`app/(public)/**`)
- Content is Server Components; programmatic routes export `ensureStatic = 'navigation'`; no request data in these
  routes or their layouts.
- No `<Suspense>` or `loading.tsx` around the main content (H1, text, JSON-LD, the lead form): the page awaits its
  cached data. YandexBot gets a request-time render, where a boundary streams the content in a hidden segment.
  Suspense only around secondary parts.
- `generateMetadata` sets `alternates.canonical` from `@/lib/routes` and `robots: { index: false }` when the API
  returns `indexable: false`.
- `notFound()`/`redirect()` run in the page before anything streams.
- No `new Date()`/`Math.random()` in prerendered code; timestamps come from the API.
- JSON-LD only through the `JsonLd` helper (schema-dts types, `<` escaped), describing visible content.

## Cabinets (`app/(private)/**`)
- `/account` (clients), `/cabinet` (lawyers), `/admin` (staff): rendered at request time, nothing personal cached or
  prerendered, `noindex` from the `(private)` layout, no analytics.
- `proxy.ts` only redirects a request without the session cookie to `/login?next=…`; Go authorizes every call by role.
- Login is Go's: login screens call Go same-origin under `/api`; OAuth redirects and email links point to Go, never to
  a Next page.

## Client/server boundary
- `'use client'` only on interactive leaves; never import `@/lib/api/server` or `@/lib/env.server` from them.
- Only `NEXT_PUBLIC_*` reaches the browser, inlined at build time; never a secret.
- `useSearchParams()` only inside `<Suspense>`.
- Analytics and other third-party scripts only in `app/(public)/layout.tsx`: the root layout wraps the cabinets.

## Telemetry
- OpenTelemetry only in `src/instrumentation.ts` (`@vercel/otel`, export from `OTEL_EXPORTER_OTLP_*`);
  `propagateContextUrls` lists the Go API only.
- Never forward `traceparent` by hand: the server client's fetch carries the active span's context.
- No personal data in spans: query strings are stripped before export; custom spans get ids, never input values.

## TypeScript and zod
- `typescript` is pinned to 6.0.3; do not upgrade to 7 (typescript-eslint and openapi-typescript need the JS API).
- No `any`, non-null `!`, `@ts-ignore` or `enum`; model variants as discriminated unions with `assertNever`.
- zod 4: `import * as z from 'zod'`, `z.email()`, `{ error: '…' }`, `z.flattenError()`. Validate form data (the
  action's input schema), `searchParams`, env and webhooks, not Go responses.
- Format dates with a fixed `'ru-RU'` locale and `timeZone` so server and client render the same text.
- UI copy in Russian; identifiers and comments in English.
