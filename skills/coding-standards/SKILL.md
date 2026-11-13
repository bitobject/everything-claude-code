---
name: coding-standards
description: TypeScript and React coding standards for web/ (Next.js 16, React 19, TypeScript 6.0, zod 4) covering strict tsconfig, no any, discriminated unions, zod at trust boundaries, naming, immutability, error handling, hydration safety, accessibility basics, UI components (shadcn/ui on Base UI) and the lint/format/test toolchain (typed ESLint, boundaries, jsx-a11y). Use when writing or reviewing .ts/.tsx files under web/. Go standards live in the golang-patterns skill.
---

# TypeScript and React standards (web/)

**Go code is out of scope here: see the `golang-patterns` skill.** Next.js architecture (caching, Server Actions,
API clients) is in `nextjs-patterns`; SEO in `seo`. Versions come from `STACK.md`.

## Principles

- Readable over clever. Small modules with one job; a component that needs a scroll bar to read is two components.
- Types come from the contract. `lib/api/schema.ts` is generated; never re-declare an API shape by hand.
- Make illegal states unrepresentable (discriminated unions), then let the compiler enforce it.
- Validate untrusted input once, at the boundary, and work with typed values inside.
- No business rules in `web/`: if you are computing a price, a permission or an eligibility, it belongs in Go.

## Toolchain

```bash
pnpm typecheck        # next typegen && tsc --noEmit
pnpm lint             # eslint . (`next lint` no longer exists; `next build` does not lint)
pnpm format           # prettier --check .
pnpm test             # vitest run
pnpm knip             # unused files, exports and dependencies
pnpm e2e              # playwright test (see the e2e-runner agent)
pnpm check            # all of the above except e2e, then pnpm build: the web's definition of done
```

```jsonc
// web/package.json (excerpt)
"packageManager": "pnpm@12.16.1",
"devEngines": {
  // pnpm refuses to install or run scripts on another Node or pnpm (ERR_PNPM_BAD_RUNTIME_VERSION);
  // it does not enforce the project's own "engines"
  "runtime": { "name": "node", "version": "^26.13.1", "onFail": "error" },
  "packageManager": { "name": "pnpm", "version": "^12.16.1", "onFail": "error" }
},
"scripts": {
  "check": "pnpm typecheck && pnpm lint && pnpm format && pnpm test && pnpm knip && pnpm build"
}
```

Dependencies are exact versions from `STACK.md` §4 (no `^`), including the helpers (Vite, jsdom, Testing Library,
`@tailwindcss/postcss`, `@axe-core/playwright`). Generators (`create-next-app`, `shadcn`) write ranges: pin them.

### ESLint: boundaries, typed rules, accessibility

```js
// web/eslint.config.mjs
import { defineConfig, globalIgnores } from 'eslint/config'
import nextVitals from 'eslint-config-next/core-web-vitals'
import nextTs from 'eslint-config-next/typescript'
import prettier from 'eslint-config-prettier/flat'
import boundaries from 'eslint-plugin-boundaries'
import jsxA11y from 'eslint-plugin-jsx-a11y-x'
import tseslint from 'typescript-eslint'

export default defineConfig([
  ...nextVitals,
  ...nextTs,

  // Type-aware rules (typescript-eslint strictTypeChecked) on everything tsc checks.
  {
    files: ['**/*.{ts,tsx,mts}'],
    extends: [tseslint.configs.strictTypeChecked],
    languageOptions: { parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname } },
    rules: {
      '@typescript-eslint/switch-exhaustiveness-check': ['error', { considerDefaultExhaustiveForUnions: false }],
      '@typescript-eslint/consistent-type-imports': 'error',
      'no-console': ['error', { allow: ['warn', 'error'] }],
    },
  },

  // Accessibility: the full recommended set from the ESLint 10 fork. eslint-config-next enables only six
  // jsx-a11y rules, as warnings; turn those off so each problem is reported once.
  {
    files: ['**/*.{jsx,tsx}'],
    ...jsxA11y.configs.recommended,
    rules: {
      ...jsxA11y.configs.recommended.rules,
      // shadcn's <Label> and <Input> count as label and control at their call sites
      'jsx-a11y-x/label-has-associated-control': [
        'error',
        { labelComponents: ['Label'], controlComponents: ['Input'] },
      ],
    },
  },
  {
    rules: {
      'jsx-a11y/alt-text': 'off',
      'jsx-a11y/aria-props': 'off',
      'jsx-a11y/aria-proptypes': 'off',
      'jsx-a11y/aria-unsupported-elements': 'off',
      'jsx-a11y/role-has-required-aria-props': 'off',
      'jsx-a11y/role-supports-aria-props': 'off',
    },
  },
  // The generated Label primitive receives htmlFor through props; its call sites are checked instead.
  { files: ['src/components/ui/label.tsx'], rules: { 'jsx-a11y-x/label-has-associated-control': 'off' } },

  // Frontend boundaries: app → features → components/lib; no feature imports another feature;
  // the server API client only from a feature's data.ts / actions.ts.
  {
    files: ['src/**/*.{ts,tsx}'],
    plugins: { boundaries },
    settings: {
      'boundaries/elements': [
        { type: 'app', pattern: 'src/app' },
        { type: 'feature', pattern: 'src/features/*', capture: ['feature'] },
        { type: 'components', pattern: 'src/components' },
        { type: 'api', pattern: 'src/lib/api' },
        { type: 'lib', pattern: 'src/lib' },
      ],
      'boundaries/files': [
        { category: 'server-entry', pattern: 'src/features/*/{data,actions}.ts' },
        { category: 'api-server', pattern: 'src/lib/api/server.ts' },
        { category: 'proxy', pattern: 'src/proxy.ts' },
      ],
    },
    rules: {
      'boundaries/dependencies': [
        'error',
        {
          default: 'disallow',
          policies: [
            {
              from: { element: { type: 'app' } },
              allow: { to: { element: { types: { anyOf: ['feature', 'components', 'api', 'lib'] } } } },
            },
            {
              from: { element: { type: 'feature' } },
              allow: { to: { element: { types: { anyOf: ['components', 'api', 'lib'] } } } },
            },
            { from: { element: { type: 'components' } }, allow: { to: { element: { type: 'lib' } } } },
            { from: { element: { type: 'api' } }, allow: { to: { element: { type: 'lib' } } } },
            {
              from: { file: { categories: 'proxy' } },
              allow: { to: { element: { types: { anyOf: ['api', 'lib'] } } } },
            },
            {
              disallow: { to: { file: { categories: 'api-server' } } },
              message: 'Only features/*/data.ts and actions.ts call Go through lib/api/server.ts',
            },
            { from: { file: { categories: 'server-entry' } }, allow: { to: { file: { categories: 'api-server' } } } },
          ],
        },
      ],
    },
  },

  // Configuration is read in one place: lib/env.server.ts (server) and lib/env.public.ts (NEXT_PUBLIC_*).
  {
    files: ['src/**/*.{ts,tsx}'],
    ignores: ['src/lib/env.server.ts', 'src/lib/env.public.ts'],
    rules: {
      'no-restricted-properties': [
        'error',
        {
          object: 'process',
          property: 'env',
          message: 'Read configuration through lib/env.server.ts or lib/env.public.ts.',
        },
      ],
    },
  },

  // Playwright fixtures receive a function named `use`; it is not a React hook.
  { files: ['e2e/**/*.ts'], rules: { 'react-hooks/rules-of-hooks': 'off' } },

  prettier, // last among style-affecting configs: turns off rules Prettier owns
  globalIgnores([
    '.next/**',
    'out/**',
    'build/**',
    'next-env.d.ts',
    'src/lib/api/schema.ts',
    'playwright-report/**',
    'test-results/**',
  ]),
])
```

- **`strictTypeChecked`** (typescript-eslint, typed with `projectService`) covers `any`, `!`, `@ts-ignore`
  (only `@ts-expect-error` with a 10+ character reason), floating and misused promises, unnecessary conditions and
  assertions, unsafe `any` flows and **deprecated APIs** (`no-deprecated`: it flags, for example, React's
  `FormEvent` and TanStack's `prefetchQuery`). `switch-exhaustiveness-check` and `consistent-type-imports` are added
  explicitly; no preset enables them.
- **jsx-a11y**: `eslint-config-next` enables only six jsx-a11y rules, all as warnings (labels and `<div onClick>`
  pass). `eslint-plugin-jsx-a11y-x` is the maintained fork that supports ESLint 10 (the original's peer range stops at
  ESLint 9); its `recommended` set runs as errors.
- **Boundaries** (`eslint-plugin-boundaries` v7, `boundaries/dependencies`): `app → features → components/lib`, no
  feature-to-feature imports (captured feature names), the server API client only from `features/*/data.ts` and
  `actions.ts`. It resolves `@/` through the TypeScript resolver `eslint-config-next` registers. Chosen over
  `import/no-restricted-paths`: that rule needs one zone per feature to forbid feature-to-feature imports, and
  `eslint-plugin-import` 2.32 does not declare ESLint 10 support.
- `eslint-config-next` also enables `react-hooks` 7's React Compiler rules (`purity`, `refs`, `set-state-in-effect`,
  `immutability`) as errors. Never disable `rules-of-hooks` or `exhaustive-deps` inline without a written reason;
  `e2e/` turns `rules-of-hooks` off only because Playwright fixtures receive a function named `use`.
- Typed linting needs about as much memory as `tsc` (0.6–0.85 GB on a small app). On a small machine run lint, tests
  and the build one after another, not in parallel.
- knip 6 parses with oxc's raw transfer, which reserves a large buffer up front; on a machine short of memory it
  fails with `RangeError: Array buffer allocation failed`. `KNIP_DISABLE_RAW_TRANSFER=1` turns that off. `knip.json`
  ignores the generated `src/lib/api/schema.ts`.

**TypeScript version.** `typescript` is pinned to `6.0.3` (exact, plain package, no aliases). TypeScript 7.0
(native) is deferred until typescript-eslint (inside `eslint-config-next`, peer `typescript <6.1.0`) and
openapi-typescript support it: 7.0 ships no JavaScript API and both tools need one. Do not upgrade it as part of
another change. openapi-typescript 7.13 declares peer `typescript ^5.x`, so pnpm prints a peer-dependency warning
with 6.0.3; that warning is expected.

## tsconfig

Keep everything Next writes (`jsx`, `module`, `moduleResolution: "bundler"`, `plugins`, `include` of `.next/types`)
and add strictness:

```jsonc
{
  "compilerOptions": {
    "strict": true,                      // default since TS 6.0; keep it explicit
    "noUncheckedIndexedAccess": true,    // arr[i] and record[k] are T | undefined
    "noImplicitOverride": true,
    "noImplicitReturns": true,
    "noFallthroughCasesInSwitch": true,
    "paths": { "@/*": ["./src/*"] }      // relative to tsconfig; `baseUrl` is deprecated in TS 6.0
  }
}
```

TS 6.0 also defaults `types` to `[]`: when globals such as `vi` disappear, list their packages in `types`. Other 6.0
deprecations (`moduleResolution: "node"` and `"classic"`, `target: "es5"`, `downlevelIteration`) are fixed by
migrating, not by `"ignoreDeprecations": "6.0"`. Never relax a flag to make an error go away; never set
`typescript.ignoreBuildErrors`.

`next build` type-checks with the project's own `tsc` and the whole tsconfig project, so unit tests, e2e specs and
config files must compile too. Vitest strips types without checking them: a test can pass in Vitest and still fail
the build.

Not used: `exactOptionalPropertyTypes` (it fights the optional fields openapi-typescript generates).

## Types

- **No `any`.** Use `unknown` and narrow (`typeof`, `instanceof`, `in`, a zod schema). Generic code uses type
  parameters, not `any`.
- **No non-null `!`.** Check and handle the `undefined` case; with `noUncheckedIndexedAccess` it is often real.
- **`as` only where the compiler cannot know** (a narrowed DOM node, a value just validated), never to silence a
  mismatch. `as const` and `satisfies` are fine: `satisfies` checks a value without widening it.
- **`@ts-expect-error` with a reason**, never `@ts-ignore`.
- **Literal unions, not `enum`.** TS enums emit runtime code and do not match the string unions openapi-typescript
  generates.
- **Explicit return types on exported functions** in `lib/` and `features/` (data functions, actions, helpers).
  Components and local callbacks may infer.
- **`type` aliases by default**; `interface` only for declaration merging (`declare global { interface Window … }`).
- **Alias generated types, do not copy them:**

```ts
import type { components, paths } from '@/lib/api/schema'

export type Lawyer = components['schemas']['Lawyer']
export type LeadStatus = components['schemas']['LeadStatus'] // a string-literal union from the contract
type LeadsQuery = paths['/v1/me/leads']['get']['parameters']['query']
```

### Discriminated unions

Model every value with variants as a union with a literal tag, and switch exhaustively (`switch-exhaustiveness-check`
reports a missing case; `assertNever` also guards at runtime).

```ts
export type LoadState<T> =
  | { status: 'loading' }
  | { status: 'failed'; message: string }
  | { status: 'ready'; data: T }

export function assertNever(value: never): never {
  throw new Error(`Unhandled variant: ${JSON.stringify(value)}`)
}

function statusLabel(status: LeadStatus): string {
  switch (status) {
    case 'new':
      return 'Новая'
    case 'accepted':
      return 'Принята'
    case 'closed':
      return 'Закрыта'
    default:
      return assertNever(status) // a new status in the contract breaks the build here, not in production
  }
}
```

Never model variants as optional fields (`{ data?: T; error?: string; loading?: boolean }`): it allows
`loading` with `error` and data at once.

## zod 4 at trust boundaries

Validate where data enters with no compile-time guarantee:

| Boundary | Example |
|---|---|
| Form input in Server Actions | `LeadInput.safeParse(Object.fromEntries(formData))` |
| URL `searchParams`, route params that are not from `generateStaticParams` | `SearchQuery.safeParse(await searchParams)` |
| Environment variables | `serverEnv()` in `lib/env.server.ts` |
| Webhooks and Route Handler bodies | the revalidate webhook |
| `localStorage`, `postMessage`, third-party APIs | parse before use |

Do **not** write zod schemas that mirror Go API responses: the generated types are the contract, and Go's handler
tests validate responses against the spec. A second hand-written copy drifts.

```ts
import * as z from 'zod'

export const SearchQuery = z.object({
  q: z.string().trim().max(100).default(''),
  page: z.coerce.number().int().min(1).max(500).default(1),
})
export type SearchQuery = z.infer<typeof SearchQuery>

const parsed = SearchQuery.safeParse(rawParams)
if (!parsed.success) {
  const { fieldErrors } = z.flattenError(parsed.error) // { formErrors: string[], fieldErrors: { q?: string[] } }
}
```

zod 4 idioms (the zod 3 forms are deprecated or removed): `z.email()`, `z.url()`, `z.uuid()` instead of
`z.string().email()`; `{ error: '…' }` instead of `message`, `invalid_type_error`, `required_error`;
`z.flattenError(err)` / `z.treeifyError(err)` instead of `err.flatten()` / `err.format()`; prefer `z.strictObject` /
`z.looseObject` (`.strict()` / `.passthrough()` still work and are not deprecated, only discouraged); `.extend()`
instead of `.merge()`; `z.record` takes two arguments; `z.stringbool()` for boolean env vars. Use `safeParse` for
user input (expected failures) and `parse` where failure means misconfiguration (env). Never `z.strictObject` for
`FormData`: it also carries React's `$ACTION_*` fields and anti-spam fields.

## Naming

| Thing | Convention | Example |
|---|---|---|
| Files and folders | kebab-case | `lead-form.tsx`, `use-debounced-value.ts` |
| Route segments | lowercase Latin transliteration | `app/(public)/[city]/[topic]` |
| Components, types | PascalCase, no `I`/`T` prefixes | `LeadForm`, `LeadFormState` |
| Hooks | `use` + noun/verb | `useLeadFilters` |
| Server Actions | verb + noun | `submitLead`, `acceptLead` |
| Event props / handlers | `onX` props, `handleX` implementations | `onSelect`, `handleSubmit` |
| Booleans | `is/has/can/should` | `isPending`, `hasReviews` |
| Module constants | UPPER_SNAKE only for true constants | `LANDINGS_PER_SITEMAP` |
| Tests | `*.test.ts(x)` next to the code; Playwright `e2e/*.spec.ts` | `lead-form.test.tsx` |

Name by domain, not by type (`features/leads/`, not `components/forms/`). UI copy is Russian; identifiers, comments
and commit messages are English.

## Immutability

- Never mutate props, state, cached data or API objects. Return new values: `{ ...lead, status: 'accepted' }`,
  `[...items, item]`, `items.filter(...)`, `[...items].sort(compare)` (`sort` mutates in place).
- Mark inputs `readonly` / `ReadonlyArray<T>` in shared helpers; it documents and enforces the contract.
- Update state from the previous value with the functional form: `setCount((c) => c + 1)`.
- Mutating a local accumulator inside one function is fine; mutating anything that escapes it is not.

## Error handling

- **Expected failures are values**: a validation error, a 404, a 409 from Go. Return them in a discriminated
  union (`LeadFormState`) or call `notFound()`; the UI renders them.
- **Unexpected failures throw**: network errors, 5xx, broken invariants. Throw an `Error` subclass (`ApiError`) with
  context; `error.tsx` or a `catchError` boundary renders the fallback.
- `catch (error: unknown)` and narrow before use. Never swallow (`catch {}`), never `throw 'string'`.
- Do not show raw API or exception messages to users; map them to Russian copy. Server error messages reach the
  client only as a digest in production anyway.
- No `console.log` in committed code. Log on the server with context (route, status, request id), never personal
  data (names, phones, case details) or tokens.
- Do not wrap `redirect()`/`notFound()` in `try/catch` without rethrowing: they work by throwing.

## React

- Function components only. Hooks at the top level, never conditional; custom hooks start with `use`.
- **Derive, don't sync.** Values computed from props/state are computed during render, not copied into state by
  `useEffect`. Effects are for synchronizing with external systems, and every subscription has a cleanup.
- No data fetching in `useEffect`: Server Components for page data, `$api.useQuery` in the cabinet.
- Keys are stable ids from the API, never array indexes for lists that change.
- Forms: `<form action={…}>` with `useActionState`; `useOptimistic` for optimistic UI; `useFormStatus` for a nested
  submit button.
- `useMemo`/`useCallback`/`memo` only with a measured reason or to keep a dependency stable.
- `dangerouslySetInnerHTML` only through the `JsonLd` helper. Rich text from the API is rendered as React elements or
  sanitized on the Go side, never passed through raw.
- **Hydration safety.** Server and client must render the same markup: no `Date.now()`, `Math.random()`,
  `typeof window` branches or locale-dependent formatting in render. Format with a fixed locale and time zone:
  `new Intl.DateTimeFormat('ru-RU', { timeZone: 'Europe/Moscow', dateStyle: 'long' })`. Browser-only values go in an
  effect or a Client Component that renders after mount. Never nest block elements in `<p>`.

## Accessibility basics

- `<html lang="ru">`. Semantic landmarks: one `<main>`, `<nav>`, `<header>`, `<footer>`; headings in order, one `<h1>`.
- A `<button>` acts, an `<a href>` navigates. Never `<div onClick>`.
- Every input has a `<label htmlFor>`; errors are linked with `aria-describedby` and announced (`role="alert"` or
  `aria-live="polite"`); invalid fields get `aria-invalid`.
- Content images have meaningful Russian `alt`; decorative images `alt=""`.
- Keyboard: everything reachable with Tab in visual order, visible `:focus-visible` styles, focus moved into
  dialogs and back on close, Escape closes them.
- Contrast at least WCAG AA (4.5:1 for body text); never color as the only signal; touch targets at least 24×24 CSS px
  (WCAG 2.2); do not disable zoom.
- jsx-a11y-x (lint) and axe in Playwright with the `wcag22aa` tags (see the `e2e-runner` agent) catch the mechanical
  part; focus order, dialogs and copy still need a human look.

## UI components

- **shadcn/ui on Base UI** (the shadcn default since July 2026; React Aria Components was the alternative). Set up with
  `pnpm dlx shadcn@4.23.0 init -b base -p nova`, add components with `shadcn add <name>`. The CLI copies them into
  `src/components/ui/`: from then on they are our code, reviewed and linted like any other. It also adds
  `@base-ui/react`, `class-variance-authority`, `cn`, `lucide-react`, `tw-animate-css` and `shadcn` itself (for
  `shadcn/tailwind.css`) with `^` ranges (pin them), and rewrites `globals.css` and the root layout's font with only
  the `latin` subset (add `cyrillic`).
- Design tokens (colors, radii, fonts) are CSS variables in `src/app/globals.css` under Tailwind 4 `@theme`; components
  use the generated utilities (`bg-primary`, `rounded-md`), never raw color values.
- Feature components compose `components/ui`; `components/ui` never imports a feature (boundaries lint).

## Tests

- Vitest 5 + Testing Library for Client Components, hooks and pure helpers; query by role and label, as users do.
- Async Server Components are not supported by Vitest: cover them with Playwright.
- `actions.ts` and data modules import `server-only`; mock them (`vi.mock('../actions')`) in component tests.
- Keep logic testable without Next: an async container loads and passes props to a synchronous view; parsing, problem
  mapping and flows are pure functions (`lib/api/problem.ts`, `features/leads/submit.ts`). Details: `tdd-workflow`.
- No tests against a mocked Go API shape you typed by hand: build fixtures from the generated types and mock HTTP with
  MSW 3 + openapi-msw handlers typed from `paths`, so contract changes break tests at compile time.

## Checklist

- [ ] `pnpm check` passes (typecheck, lint with boundaries + typed rules + jsx-a11y, format, tests, knip, build)
- [ ] No unexplained `as`, no `enum` (lint covers `any`, `!`, `@ts-ignore`, promises, exhaustiveness)
- [ ] API types aliased from `schema.ts`; no hand-written DTOs; `schema.ts` untouched
- [ ] Variants are discriminated unions with exhaustive switches
- [ ] Untrusted input parsed with zod 4 at the boundary
- [ ] No mutation of props/state/API data
- [ ] Errors: expected → values, unexpected → thrown `Error`; nothing swallowed; no personal data in logs
- [ ] Hydration-safe render; fixed locale and time zone for dates
- [ ] Focus order, dialogs, contrast and copy checked by a person (lint covers labels, roles, alt text)
