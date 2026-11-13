<!-- Independent reviewer's report, 2026-11-13: Frontend material vs the pinned package versions.
     Raw model output, kept for its sources and detail. Which claims were re-verified is stated in
     ../2026-11-13-best-practices-review.md (sections marked [проверено] / [опровергнуто]). -->

# Frontend material vs. pinned versions: review report

Nothing was compiled or run. "Correct" means the claim matches the published package's `.d.ts` files, its source, or the docs that `next@16.4.6` ships in `dist/docs`. I unpacked the tarballs under `/tmp/claude-0/-root-lead-new--localenv/6b41f19b-ec42-4595-8dc3-68f60461b2ed/scratchpad/research-frontend-correctness/pkg-<name>-<version>/package/`.

Abbreviations used below:
- **Packages:** `N:` is the next@16.4.6 package dir. `D:` is `N:dist/docs/01-app/`.
- **Our files:** NP = skills/nextjs-patterns/SKILL.md, SEO = skills/seo/SKILL.md, CS = skills/coding-standards/SKILL.md, OC = skills/openapi-contract-first/SKILL.md, RT = rules/typescript.md, TR = agents/typescript-reviewer.md, BER = agents/build-error-resolver.md, E2E = agents/e2e-runner.md.

## 1. Verdict

Largely yes. Every API, config key, file convention, import path, CLI flag and message id the material relies on exists in the pinned versions, for Next 16.4.6 and for the other packages. The Cache Components / `ensureStatic` model is described accurately, including subtle points such as the request-time render for HTML-limited bots, whose list includes Yandex.

An agent following the material should produce code that type-checks. That is still unverified, and types that depend on the contract (for example `error.code`) are assumed.

Three behaviours are wrong at runtime:
- the canonical lead form wipes the visitor's input after every invalid submit;
- the LCP-image advice leaves the image `loading="lazy"`;
- forwarding `(await cookies()).toString()` re-encodes cookie values.

There are also five low-severity inaccuracies.

## 2. Claims table

| Claim | Where | Status | Evidence |
|---|---|---|---|
| All 14 pinned versions exist and are npm `latest`. TS 6.0.3 is the newest 6.x; `latest` is 7.0.2 | STACK:142-144 | correct | registry dist-tags, 2026-11-13 |
| TS 7 has no usable JS API. typescript-eslint 8.72.2 has peer `>=4.8.4 <6.1.0`. openapi-typescript 7.13.0 has peer `^5.x` and uses the TS factory API | STACK:146-149; CS:55-59; RT:43 | correct (nuance: 7.0.2's main entry exports only `version`; its new API lives under `typescript/unstable/*`) | typescript@7.0.2 `package.json` exports; peer fields in both packages |
| TS 7 crash messages `reading 'Cjs'` and `reading 'createKeywordTypeNode'` | BER:59-60 | correct | typescript-estree `dist/create-program/shared.js:63-72`; openapi-typescript `dist/lib/ts.mjs:11` |
| `cacheComponents` and `partialPrefetching` are top-level options; leaving `partialPrefetching` unset logs a warning | NP:51-52 | correct | `D:…/partialPrefetching.md`; `N:dist/server/config-shared.d.ts:1721,1732` |
| `remotePatterns: [new URL(…)]` | NP:53 | correct | `D:03-api-reference/02-components/image.md:541` |
| `dynamic`, `dynamicParams`, `revalidate`, `fetchCache` exports are rejected with Cache Components; `unstable_cache` belongs to the old model | NP:58-59; TR:53-54; BER:78 | correct (the exact error text is documented only for `dynamicParams`) | `route-segment-config/index.md` (v16.0.0 row); `migrating-to-cache-components.md:648-652` |
| `ensureStatic` (16.4): its values, scope (layouts plus `generateMetadata`), what it forbids, ≥1 complete `generateStaticParams` set, unlisted params wait for the full page, child cannot weaken the level, error texts | NP:264-300; SEO:27; BER:71-75 | correct | `D:…/ensureStatic.md`; `keeping-pages-static.md`; `N:dist/build/segment-config/app/app-segment-config.js:165-168` |
| `cacheLife` presets, the implicit `default` profile, dynamic-hole thresholds | NP:201-205 | correct (also: `stale` under 30 s is excluded) | `N:cache.d.ts`; `cacheLife.md:139-147,266-270` |
| `cacheTag(...tags)`: ≤128 tags per call, ≤256 chars, case-sensitive | NP:119,207 | correct | `use-cache/cache-tag.d.ts`; `cacheTag.md:103` |
| `revalidateTag(tag,'max')` serves stale while revalidating; `{expire:0}` blocks; the one-argument form is deprecated | NP:211-217; RT:22; TR:150 | correct (the one-argument form is also a TS compile error) | `spec-extension/revalidate.d.ts`; `revalidateTag.md`; `version-16.md:424-436` |
| `updateTag` works only in Server Actions; the error text matches | NP:213,217; BER:79 | correct | `revalidate.js`; `updateTag.md` |
| `cookies()` inside `'use cache'` raises `next-request-in-use-cache`; return `data`, not a `Response`; arguments and closures form the cache key | NP:130-143 | correct | `use-cache.md:79-105,157-198,239-241` |
| Message ids (`blocking-prerender-*`, `static-route-*`, `empty-generate-static-params`, `generate-static-params`, `missing-suspense-with-csr-bailout`, `react-hydration-error`) and the `blocking-route` overlay insight | NP:208,310-313; BER:69-87 | correct (runtime data read outside Suspense maps to `-runtime`, not `-dynamic`) | URLs grepped from `N:dist`; `08-caching.md:138` |
| `generateStaticParams` must return ≥1 item with Cache Components (pages) | NP:267,485; RT:24 | correct | `generate-static-params.md:309-311` |
| HTML-limited bots get a request-time render, the default list matches Yandex, and plain Googlebot gets the static page | NP:301-302; SEO:27-34 | correct | `N:dist/shared/lib/router/utils/html-bots.js`; `N:dist/build/templates/app-page-runtime.js:360-366` |
| `notFound()` returns 404 before streaming or during prerender, and 200 + noindex after streaming starts | NP:293,427; SEO:264 | correct | `not-found.md`; `loading.md` "Status codes" |
| `proxy.ts` with `export function proxy`: Node only, no `runtime` export, no `'use cache'` or fetch caching, matcher caveats, Server Actions can slip past the matcher | NP:398-419; RT:18; TR:126 | correct | `16-proxy.md:31-39`; `proxy.md:60,77,251-257` |
| `error.tsx` props `{error: unknown; retry}`; `global-error` cannot export metadata; `catchError` from `next/error` | NP:425-429; RT:25 | correct (`retry` and `catchError` stable since 16.3) | `error.md`; `catchError.md`; `N:error.d.ts` |
| `next typegen && tsc --noEmit`; global `PageProps`; `next lint` removed and `next build` no longer lints; codemod name; `next analyze --output`; `--debug-prerender`; `--debug-build-paths` | NP:78,470; CS:22-23; BER:41-42,51,95; TR:127 | correct | `06-cli/next.md`; `version-16.md:1096-1110` |
| `eslint.config.mjs` imports; the `/core-web-vitals` and `/typescript` exports are arrays; `eslint-config-prettier/flat` | CS:31-49 | correct | eslint-config-next `dist/*.d.ts`; `03-eslint.md` |
| ESLint 10 needs eslint-config-next ≥16.4; drop the `settings.react.version` workaround; plugin peer warnings are expected | BER:96,101-102 | correct | `03-eslint.md:83-85`; react 7.37.5, jsx-a11y 6.10.2 and import 2.32.0 peers stop at `^9` |
| "eslint-config-next already brings react-hooks and jsx-a11y" | CS:52 | partly wrong | P7 |
| `images.qualities` defaults to `[75]`; `priority` is deprecated; the LCP image only needs `fetchPriority="high"` | NP:459-461; SEO:277; RT:26; TR:110 | incomplete | P2 |
| `Inter` with the `cyrillic` subset; Tailwind 4 `@theme inline` | NP:465-466 | correct | `@next/font` google `index.d.ts:7097`; `font.md:845-851` |
| Metadata fields (`metadataBase`, title template, `openGraph`, `verification.yandex`, `canonical`, `robots`, `pagination`); file-based metadata wins | SEO:71-88,256-257 | correct | `metadata-interface.d.ts`; `generate-metadata.md:114,756-777,927-943` |
| `generateSitemaps` URLs, no sitemap index, `id: Promise<string>` | SEO:110-131 | correct | `generate-sitemaps.md`; `next-metadata-route-loader.js:239-300` |
| "`[]` from `generateSitemaps` fails the build" | SEO:121 | likely wrong | P5 |
| robots `other` field (since 16.3) for `Clean-param`; `sitemap` accepts an array | SEO:151-171 | correct | `metadata-interface.d.ts:547-576`; `robots.md:122-204` |
| Trailing-slash redirect is 308; `permanent` gives 308/307; `redirect()` after streaming becomes a meta refresh | SEO:48,266-270 | correct | `N:dist/lib/load-custom-routes.js:564-571`; `redirect.md:12` |
| JSON-LD escaping of `<`; schema-dts `Graph`, `WithContext`, `Thing` | SEO:184-226 | correct | `json-ld.md:11-34`; schema-dts `dist/schema.d.ts:4-10` |
| `useReportWebVitals` from `next/web-vitals`; `next/script` `afterInteractive` | SEO:282,344 | correct | `dist/client/web-vitals.d.ts` |
| openapi-fetch: default `createClient<paths>`, `Client` type, `{data,error,response}` union, `params`, `headers`; `generate:api -o` | NP:84-127; OC:18,135; E2E:81-91 | correct | openapi-fetch `dist/index.d.mts:1-30,95-110,334-335`; openapi-typescript `bin/cli.js:17` |
| openapi-react-query `useQuery`/`useMutation(method, path, …)` with keys `[method, path, init]` | NP:157-193 | correct (the key is `[method, path]` when `init` is undefined) | openapi-react-query `dist/index.mjs:5,20,61` |
| TanStack v5 provider pattern; `isPending` | NP:165-192 | correct (identical to Next's TanStack guide) | `client-side-data-fetching/tanstack-query.md:28-39` |
| zod 4 idioms | CS:167-171; NP:227-443 | correct (`.strict()` is only discouraged, not deprecated) | zod `v4/classic/schemas.d.ts:495-507,798`; `v4/core/api.d.ts:8-10` |
| `useActionState` returns `[state, action, isPending]`; `useOptimistic`, `useFormStatus` | NP:368; CS:219 | correct | @types/react 19.3.0 `index.d.ts:1982-1991` |
| "`$ACTION_*` keys are stripped" | NP:349 | wrong | P4 |
| The lead form keeps input on an invalid submit (implied by the e2e test name) | NP:373-386; E2E:136 | wrong | P1 |
| Forward `(await cookies()).toString()` to Go | NP:139,392 | risky | P3 |
| TS 6.0 defaults (`strict`, `types: []`) and deprecations (`baseUrl`, `node10`, `es5`, `downlevelIteration`) | CS:69-80; BER:52-54 | correct, except "`classic` removed" | P8 |
| Multi-instance setup: `cacheHandlers` with `refreshTags`, encryption key, `deploymentId`; `.env*` live in `web/` | NP:248-250,455 | correct | `cacheHandlers.md:212-329`; `self-hosting.md:163-247` |
| Playwright 1.65 config and CLI; `networkidle` discouraged; `request.storageState({path})`; AxeBuilder API | E2E:24-192 | correct | playwright `lib/program.js:202-224`; `types/test.d.ts:1327`; playwright-core `types.d.ts:3457,20676`; @axe-core `dist/index.d.mts:17-34` |
| Vitest cannot test async Server Components | CS:245 | correct | `testing/vitest.md:9` |
| "`ensureStatic` + `partialPrefetching` (Next 16.4)" give complete HTML for on-demand pages | STACK:130 | imprecise | P6 |

## 3. Problems

**P1. The lead form loses everything the visitor typed after an invalid submit.** Severity **high**, since this is the money form. Confidence high.
- **What we say:** the `useActionState` form at NP:373-386 returns `invalid` and shows the errors, and the e2e test at E2E:136 is named "…without losing input".
- **What is true:** React 19 resets all uncontrolled fields after a function action finishes without throwing, and returning a validation error counts. The inputs have no `defaultValue`, so they come back empty. Without JS the server render is empty too. The test never asserts the values, so it passes anyway.
- **Evidence:** `pkg-react-dom-19.3.1/package/cjs/react-dom-client.development.js` `startHostTransition` calls `requestFormReset$1(formFiber)` before `action(formData)`. react.dev's `<form>` page says: "Returning a validation error instead of throwing also triggers this reset". The React 19.3 post adds: "`onReset` now fires when React automatically resets a form after a Server Action".
- **Fix:** add `values` to the `invalid` and `rejected` states and render `defaultValue={state.values?.name}`. Add `await expect(page.getByLabel('Имя')).toHaveValue('А')` to the test.

**P2. The LCP-image advice leaves the image lazy-loaded.** Severity medium. Confidence high.
- **What we say:** give the single LCP image `fetchPriority="high"`.
- **What is true:** next/image still emits `loading="lazy"`, because only `priority` or `preload` disable lazy loading. Next's own LCP warning asks for `loading="eager"`.
- **Evidence:** `N:dist/shared/lib/get-img-props.js:236,446,382-385`; `image.md:266-294`.
- **Fix:** use `loading="eager"` together with `fetchPriority="high"`. Use `preload` only when the image must be discovered from `<head>`; it must not be combined with `loading` or `fetchPriority`. Update NP:460, SEO:277, RT:26 and TR:110.

**P3. `(await cookies()).toString()` is not the original Cookie header.** Severity medium. Confidence high on the mechanism; the impact depends on the token alphabet.
- **What is true:** values are decoded with `decodeURIComponent` on parse, and values that fail to decode are silently dropped. `toString()` then re-encodes them with `encodeURIComponent`. So `s=ab+/==` reaches Go as `s=ab%2B%2F%3D%3D`, and Go does not decode cookie values.
- **Evidence:** `N:dist/compiled/@edge-runtime/cookies/index.js:48-65,247-249`.
- **Fix:** forward `(await headers()).get('cookie')` unchanged, or only the session cookie taken from that raw header. Alternatively require URL-safe tokens on the Go side.

**P4. `$ACTION_*` keys are not stripped.** Severity low. Confidence high.
- **What is true:** with JS enabled, the FormData includes the hidden `$ACTION_*` inputs. The code works only because `z.object` drops unknown keys.
- **Evidence:** `D:02-guides/forms.md:70`.
- **Fix:** reword the comment, and warn never to use `z.strictObject` for FormData.

**P5. `generateSitemaps` returning `[]` probably does not fail the build.** Severity low. Confidence medium; this comes from reading the source, not from a run.
- **What is true:** a sitemap is compiled into a GET route handler. `isRoutePPREnabled` is true only for `APP_PAGE` routes, and the empty-params error requires it. With `[]` there are simply no chunks, and every `/…/sitemap/N.xml` returns 404.
- **Evidence:** `N:dist/build/utils.js:758`; `N:dist/build/static-paths/app.js:513-524`; `next-metadata-route-loader.js:258-273`.
- **Fix:** drop the reason, or handle zero chunks explicitly.

**P6. STACK:130 credits `partialPrefetching` with something it doesn't do.** Severity low. Confidence high.
- **What is true:** `partialPrefetching` arrived in 16.3, and `ensureStatic='navigation'` "works with Cache Components alone". Without `ensureStatic`, Partial Prefetching is exactly what serves a fallback shell for unlisted params.
- **Evidence:** `partialPrefetching.md` version history; `ensureStatic.md`; `incremental-static-regeneration-cache-components.md`.
- **Fix:** "`ensureStatic='navigation'` (16.4) guarantees complete HTML. `partialPrefetching: true` (16.3) is set because it is part of the recommended model and leaving it unset logs a warning."

**P7. eslint-config-next's lint coverage is not what CS:52 implies.** Severity low to medium. Confidence high.
- **What is true:** only six `jsx-a11y` rules are enabled, all as `warn`, so labels and `<div onClick>` are not linted. Separately, eslint-plugin-react-hooks 7's `recommended` set (spread by eslint-config-next) enables React Compiler rules such as `purity`, `refs`, `set-state-in-effect` and `immutability` as errors.
- **Evidence:** eslint-config-next `dist/index.js` rules block; react-hooks 7.1.1 `cjs/…production.js:55246-55264`.
- **Fix:** enable the `jsx-a11y` rules you want in the rules block (the plugin is already registered by eslint-config-next), and mention the compiler rules.

**P8. Minor wording.** Severity low. Confidence high.
- In TS 6.0, `moduleResolution: classic` is deprecated, not removed (BER:53). Evidence: `typescript.js:130044-130123`.
- zod's `.strict()` is not deprecated (CS:169-170).

## 4. Gaps

- **Node and pnpm are not pinned, and the tools disagree on floors.** Vitest 5.1 needs Node `^22.12 || ^24 || >=26`. @axe-core/playwright 4.14 needs ≥22.12. ESLint 10 needs `^20.19 || ^22.13 || >=24`. Current LTS lines are 24.22.3 and 26.13.1; pnpm is at 12.18.0. Pin Node 24 or 26 through `engines`/`devEngines`.
- **Supporting packages are unpinned.**
  - Not in STACK §4: @axe-core/playwright 4.14.0 (E2E:10 points to STACK for it), eslint-config-prettier 10.1.8, @tailwindcss/postcss 4.3.5, server-only 0.0.1.
  - Vitest 5 has a `vite` peer (^6.4–^8; 8.3.8 is latest) and also needs @vitejs/plugin-react 6.3.0, jsdom 31.0.0 and @testing-library/react 16.3.3.
  - The `@/` alias in Vitest needs Vite 8's `resolve.tsconfigPaths: true` or vite-tsconfig-paths.
  - Tailwind 4 needs `postcss.config.mjs` with `@tailwindcss/postcss` and `@import 'tailwindcss'`.
- **`next build` type-checks with the project's `tsc` CLI by default** (`experimental.useTypeScriptCli: true`). It therefore checks every file in tsconfig, e2e and unit tests included, without Next's code frames (`N:dist/server/config-shared.js:263`; `useTypeScriptCli.md`). Also, Next writes `"strict": false` into tsconfig if the key is missing, so the explicit `strict` in CS is essential.
- **The bot list includes `Chrome-Lighthouse` and `Google-InspectionTool`.** PageSpeed lab data and URL Inspection therefore see the request-time render, not the static HTML. Add a 404 e2e test using the Yandex UA.
- **Reverse proxy and Server Actions.** Next compares `Origin` with `x-forwarded-host`/`host`, so the proxy must forward the public host or `serverActions.allowedOrigins` must be set. Actions are capped at 1 MB by default (`serverActions.md:13,40`).
- **Unknown slugs.** With Cache Components, `dynamicParams=false` is gone, and under `ensureStatic` every random URL triggers a blocking on-demand render plus a Go call. Consider a slug check in Proxy, as `loading.md` suggests.
- **Navigation keeps state.** Cache Components keep visited routes mounted in a hidden `<Activity>`, so form state such as "sent" persists on back-navigation (`cacheComponents.md`; `preserving-ui-state.md`).
- **Newer APIs are not mentioned.** `instant` (16.3), `navigation()`/`prefetch()` (16.4), React 19.3 `use(browser())`, and `'use cache: remote'` for multi-instance setups. Next also ships version-exact docs plus an `AGENTS.md` in `node_modules/next/dist/docs`; point agents there.
- **Security and supply chain.** Require next ≥16.4.5: the Nov 3 advisories include shell poisoning when `partialPrefetching` is unset. prettier 3.10.1 was published on the day the versions were checked, so consider pnpm `minimumReleaseAge`.

## 5. Sources

- registry.npmjs.org: dist-tags, package metadata and tarballs (npm, Inc.), accessed 2026-11-13. Publish dates: next / eslint-config-next 16.4.6 on 2026-11-09; react 19.3.1 on 2026-10-14; typescript 6.0.3 on 2026-04-16 and 7.0.2 on 2026-07-08; typescript-eslint 8.72.2 on 2026-11-09; openapi-typescript / openapi-fetch / openapi-react-query on 2026-02-11; zod 4.6.7 on 2026-10-12; vitest 5.1.0 on 2026-11-02; Playwright 1.65.0 on 2026-11-05; tailwindcss 4.3.5 on 2026-11-06; eslint 10.14.0 on 2026-10-30; prettier 3.10.1 on 2026-11-12; schema-dts 2.1.0 on 2026-10-02.
- next@16.4.6 bundled docs, `dist/docs` (Vercel), 2026-11-09.
- https://nextjs.org/blog/next-16-4 "Next.js 16.4" (Vercel), 2026-10-06.
- https://nextjs.org/blog/november-2026-security-release "November 2026 Security Release" (Vercel), 2026-11-03.
- https://react.dev/reference/react-dom/components/form "&lt;form&gt;" (React team), accessed 2026-11-13.
- https://react.dev/blog/2026/09/09/react-19-3 "React 19.3" (React team), 2026-09-09.
- https://nodejs.org/dist/index.json, the Node.js release index (OpenJS Foundation), accessed 2026-11-13.
- https://nextjs.org/docs/app/guides/migrating-to-cache-components "Migrating to Cache Components" (Vercel), accessed 2026-11-13.
