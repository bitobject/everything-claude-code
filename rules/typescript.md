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
  `apiFor(await requestHeaders())` or `apiFor(await sessionHeaders())` at request time. The cabinet uses `$api` from
  `@/lib/api/browser` (openapi-react-query). No raw `fetch` to the API, no axios.
- Boundaries (lint): `app → features → components/lib`; a feature never imports another feature; `lib/api/server.ts`
  only from `features/*/data.ts` and `actions.ts`; `process.env` only in `lib/env.server.ts`/`lib/env.public.ts`.
- Forward the session cookie verbatim (`sessionHeaders()`), never `(await cookies()).toString()`.
- Error bodies: parse with `asProblem()`, branch on `code`; a non-2xx body may not be a problem document.

## Next.js 16, not 14/15
- `src/proxy.ts` with `export function proxy`; `middleware.ts` is the old name.
- `params`, `searchParams`, `cookies()`, `headers()` are async: `await` them. Type with the global `PageProps<'/route'>`.
- Caching is Cache Components: `'use cache'` + `cacheLife()` + `cacheTag()`. Never `unstable_cache`,
  `export const revalidate|dynamic|dynamicParams|fetchCache`, or `fetch(…, { next: { revalidate } })`.
- `revalidateTag(tag, 'max')` takes two arguments; `updateTag(tag)` works only in Server Actions.
- No `cookies()`/`headers()` inside `'use cache'`; cached functions return plain `data`, not the openapi-fetch result.
- `generateStaticParams` returns at least one item; unknown params call `notFound()`.
- `next lint` is gone: `pnpm exec eslint .`. `error.tsx` receives `{ error: unknown; retry }`.
- `next/image` `priority` is deprecated: the single LCP image gets `loading="eager"` and `fetchPriority="high"`
  (`fetchPriority` alone stays lazy).
- `typedRoutes` is on: build hrefs in `@/lib/routes`, the only place that casts to `Route`.

## Forms and mutations
- Public forms are Server Actions (work without JS); cabinet interactions use `$api.useMutation` + invalidation.
  Each feature keeps its query keys and cache tags in `cache.ts`.
- Every error state of a form action returns the submitted `values`; inputs render them as `defaultValue`.
- Request bodies are object literals of parsed fields (no field the contract lacks); `Idempotency-Key` is minted in
  the browser at submit or by the action, never during render.

## SEO routes (`app/(public)/**`)
- Content is Server Components; programmatic routes export `ensureStatic = 'navigation'`; no request data in these
  routes or their layouts.
- `generateMetadata` sets `alternates.canonical` from `@/lib/routes` and `robots: { index: false }` when the API
  returns `indexable: false`.
- `notFound()`/`redirect()` run before streaming, never inside a Suspense boundary over uncached data.
- No `new Date()`/`Math.random()` in prerendered code; timestamps come from the API.
- JSON-LD only through the `JsonLd` helper (schema-dts types, `<` escaped), describing visible content.

## Client/server boundary
- `'use client'` only on interactive leaves; never import `@/lib/api/server` or `@/lib/env.server` from them.
- Only `NEXT_PUBLIC_*` reaches the browser, inlined at build time; never a secret.
- `useSearchParams()` only inside `<Suspense>`.
- Analytics and other third-party scripts only in `app/(public)/layout.tsx`: the root layout wraps the cabinet.

## TypeScript and zod
- `typescript` is pinned to 6.0.3; do not upgrade to 7 (typescript-eslint and openapi-typescript need the JS API).
- No `any`, non-null `!`, `@ts-ignore` or `enum`; model variants as discriminated unions with `assertNever`.
- zod 4: `import * as z from 'zod'`, `z.email()`, `{ error: '…' }`, `z.flattenError()`. Validate form data,
  `searchParams`, env and webhooks, not Go responses.
- Format dates with a fixed `'ru-RU'` locale and `timeZone` so server and client render the same text.
- UI copy in Russian; identifiers and comments in English.
