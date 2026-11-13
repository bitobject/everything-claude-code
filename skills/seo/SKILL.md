---
name: seo
description: Technical SEO for this project's Next.js 16 frontend, for Yandex and Google. Covers the Metadata API and generateMetadata, canonical URLs, sitemap.ts + generateSitemaps for thousands of city × legal-topic pages, robots.ts with Yandex Clean-param, JSON-LD typed with schema-dts (Organization, LegalService, BreadcrumbList, FAQPage, Article), internal linking, pagination, 404/410 and redirects, Core Web Vitals budgets, Yandex specifics (complete SSR HTML, Webmaster verification, regionality, Metrica without blocking render, no cloaking) and thin/duplicate programmatic content. Use when building or reviewing indexable pages, metadata, sitemaps, robots, structured data, redirects or analytics.
---

# Technical SEO (web/)

Rendering mechanics (`'use cache'`, `ensureStatic`, data functions) are in the `nextjs-patterns` skill; this skill
says what the HTML must contain and why. The market is Russia: Yandex and Google both matter, and Next serves
YandexBot a request-time render of the server HTML. Yandex decides by itself whether to run a page's JavaScript
(Webmaster default «На усмотрение робота Яндекса»), so the server HTML must already be complete.

## Principles

1. **One search intent per URL.** One city × topic page answers one query family; two URLs for the same intent
   compete and both lose.
2. **Complete HTML on the first response.** Title, canonical, H1, main text, links and JSON-LD are in the server
   HTML, not added by client JavaScript.
3. **Go decides what exists and what is indexable.** The API returns `indexable` per page from a business rule
   (enough lawyers, reviews, unique local content). Next renders that decision; it never invents its own threshold.
4. **One URL builder.** Links, canonicals, sitemaps, breadcrumbs and JSON-LD all call `lib/routes.ts`, so they can
   never disagree.
5. **Markup describes visible content only.** Structured data for content the user cannot see is spam.
6. **Measure in both consoles.** Yandex.Webmaster and Google Search Console, after every structural release.

## Rendering for crawlers

- SEO routes export `ensureStatic = 'navigation'` and read only `'use cache'`d data, so Googlebot and users get the
  prerendered page and unlisted params wait for a complete page instead of a fallback.
- Next renders a request-time page for HTML-limited bots (its default list matches YandexBot) so metadata lands in
  `<head>`. Content is identical; the render path differs. Every data source used at build must also be reachable at
  request time, or YandexBot gets a 500 while users see the page.
- Never put the H1, main text, FAQ, breadcrumbs or JSON-LD inside a client-only component, a `loading.tsx` boundary,
  `next/dynamic` with `ssr: false` or any `<Suspense>` boundary. Crawlers get a request-time render, and there a
  boundary sends its fallback first and the content later in a hidden streamed segment (`<div hidden id="S:…">`),
  even when the data is cached: verified with the YandexBot user agent. Suspense only around secondary parts, if
  at all.
- Do not override `htmlLimitedBots`; the default list includes Yandex.
- Verify the raw HTML the way Yandex sees it:

```bash
curl -sS -A 'Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)' \
  -o page.html -w '%{http_code}\n' https://example.ru/moskva/razvod
grep -c '<h1' page.html; grep -o '<link rel="canonical"[^>]*>' page.html; grep -c 'application/ld+json' page.html
grep -c '<div hidden id="S:' page.html   # streamed segments: expect 0
grep -c '<template id="B:' page.html      # Suspense fallbacks: 0, or 1 from the analytics counter (outside <main>)
```

Do the same with the Googlebot user agent (`Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)`):
Googlebot gets the prerendered page, YandexBot the request-time render, and both must be complete. The e2e suite
checks both (`e2e-runner`). Then use Yandex.Webmaster "Проверка ответа сервера" and Google "URL Inspection" for the
same URL.

## URLs and canonicals

- Lowercase Latin transliteration with hyphens (`/moskva/razvod`, `/sankt-peterburg/alimenty`). Slugs come from Go.
- No trailing slash: Next redirects `/x/` to `/x` (308) by default; keep it. Yandex may ignore `rel=canonical` for
  trailing-slash duplicates, so the redirect is what fixes them.
- Every indexable page sets `alternates.canonical` to its own clean path; `metadataBase` in the root layout turns it
  into an absolute URL. Canonical never carries tracking or filter parameters.
- Filter and sort combinations (`?sort=price`) are not indexable: `robots: { index: false, follow: true }` and a
  canonical to the unfiltered page. Indexable listings use path segments.
- Never canonicalize a page to a different page with different content; consolidate or redirect instead.

```ts
// src/lib/routes.ts — the only place that knows URL shapes (and, with typedRoutes, the only `as Route` cast)
import type { Route } from 'next'

export const cityPath = (city: string): Route => `/${city}` as Route
export const landingPath = (city: string, topic: string): Route => `/${city}/${topic}` as Route
export const landingListPath = (city: string, topic: string, page: number): Route =>
  page <= 1 ? landingPath(city, topic) : (`${landingPath(city, topic)}/stranica-${String(page)}` as Route)
```

## Metadata

```tsx
// src/app/layout.tsx (fragment)
import type { Metadata } from 'next'
import { serverEnv } from '@/lib/env.server'

export const metadata: Metadata = {
  metadataBase: new URL(serverEnv().SITE_URL),
  title: { default: 'Юридическая помощь — Бренд', template: '%s — Бренд' }, // template applies to child segments
  description: 'Консультации юристов и адвокатов в вашем городе.',
  openGraph: { type: 'website', locale: 'ru_RU', siteName: 'Бренд' },
  verification: { yandex: '<code from Yandex.Webmaster>', google: '<code from Search Console>' },
}
```

- `verification.yandex` renders `<meta name="yandex-verification" content="…">`. Keep it in the root layout
  permanently; removing it can cost the site its verified status in Webmaster.
- Pages use `generateMetadata` (example in `nextjs-patterns`) reading the same cached function as the page, so
  metadata and content come from one fetch and both are static.
- Title: unique per URL, primary phrase first, city in the title for local pages, about 60 characters.
  Description: unique, honest, about 120–160 characters. One `<h1>` per page that matches the title's intent.
- Non-indexable pages: `robots: { index: false, follow: true }` and absent from sitemaps. The cabinets' layout
  (`(private)/layout.tsx`, over `/account`, `/cabinet`, `/admin`): `robots: { index: false, follow: false }`.
- File-based metadata (`opengraph-image.tsx`, `icon.png`) overrides the `metadata` object; use one or the other per field.

## Programmatic pages (city × legal topic)

Thousands of near-identical pages are the main risk of this site. Google's spam policies name it directly: "doorway
abuse" includes "pages targeted at specific regions or cities that funnel users to one page", and "scaled content
abuse" is "many pages generated for the primary purpose of manipulating search rankings". Yandex's sanctions for
templated pages are «Малополезный контент» (it names "шаблонных методик (подстановок ключевых слов и фраз)") and
«SEO-тексты»; they appear in Webmaster under «Безопасность и нарушения». The exclusion status «Малоценная или
маловостребованная страница» (Страницы в поиске → Исключённые) is not a sanction: Yandex says such exclusions
«негативно не влияет на ранжирование».

- **Gate by data, in Go.** A pair is indexable only when it has real local substance: lawyers practising there,
  reviews, local prices, court and registry addresses, city-specific FAQ. The API returns `indexable`; non-indexable
  pairs either 404 or render with `noindex` and stay out of sitemaps and hub links.
- **Make every page different in content, not in a substituted city name.** A template that only swaps «в Москве» for
  «в Казани» is a doorway. Required unique blocks come from the API: lawyer list, local stats, local procedure notes.
- **Consolidate near-duplicates.** Two topics with the same intent (`razvod` vs `rastorzhenie-braka`) become one URL
  plus a 308 from the other.
- **Launch in waves.** Prerender the most-visited pairs with `generateStaticParams`, publish sitemaps in batches,
  and watch both consoles' excluded/indexed reports before adding the next batch.
- **No orphans.** Every indexable pair is reachable from its city hub and its topic hub through plain links.

## Sitemaps

Next builds no sitemap index for `generateSitemaps`: each chunk is served at `/<segment>/sitemap/<id>.xml`. List
the chunks in `robots.ts` (both engines read multiple `Sitemap:` lines) and submit them in both consoles.

```ts
// src/app/landings/sitemap.ts: programmatic pages in chunks (src/app/sitemap.ts lists home, hubs, static pages)
import type { MetadataRoute } from 'next'
import { countLandingSitemaps, listIndexableLandings } from '@/features/landing/data'
import { serverEnv } from '@/lib/env.server'
import { landingPath } from '@/lib/routes'

export async function generateSitemaps(): Promise<{ id: number }[]> {
  const chunks = await countLandingSitemaps() // cached; 0 is valid: the build passes and no chunk URL exists
  return Array.from({ length: chunks }, (_, id) => ({ id }))
}

export default async function sitemap({ id }: { id: Promise<string> }): Promise<MetadataRoute.Sitemap> {
  const site = serverEnv().SITE_URL
  const landings = await listIndexableLandings(Number(await id)) // cached, tagged 'landings'; Go serves chunk N
  return landings.map((l) => ({
    url: `${site}${landingPath(l.city, l.topic)}`,
    lastModified: l.updatedAt, // real change time from Go; new Date() is wrong and breaks prerender
  }))
}
```

- Go owns chunking (`GET /v1/seo/landing-sitemaps` → chunk count, `GET /v1/seo/landing-sitemaps/{chunk}` → items):
  a sitemap chunk needs random access by number; cursor pagination stays for app lists.
- Only canonical, indexable URLs that return 200. No redirects, no `noindex` pages, no parameters.
- The protocol limit is 50,000 URLs / 50 MB per file; chunks of 10,000 keep generation fast.
- `changeFrequency` and `priority` are optional hints; an accurate `lastModified` is worth more.

## robots.ts

```ts
// src/app/robots.ts
import type { MetadataRoute } from 'next'
import { countLandingSitemaps } from '@/features/landing/data'
import { serverEnv } from '@/lib/env.server'

const TRACKING_PARAMS = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_content', 'utm_term', 'yclid', 'gclid']

export default async function robots(): Promise<MetadataRoute.Robots> {
  const { SITE_URL, SITE_INDEXABLE } = serverEnv()
  if (!SITE_INDEXABLE) return { rules: { userAgent: '*', disallow: '/' } } // staging, previews

  const chunks = await countLandingSitemaps()
  return {
    rules: {
      userAgent: '*',
      allow: '/',
      disallow: ['/account/', '/cabinet/', '/admin/', '/login', '/api/', '/internal/'],
      other: { 'Clean-param': TRACKING_PARAMS.join('&') }, // Yandex; Google ignores unknown lines
    },
    sitemap: [
      `${SITE_URL}/sitemap.xml`,
      ...Array.from({ length: chunks }, (_, id) => `${SITE_URL}/landings/sitemap/${id}.xml`),
    ],
  }
}
```

- `other` (Next 16.3+) passes non-standard directives through verbatim. `Clean-param` is intersectional, so Yandex
  reads it from the `*` block; syntax `Clean-param: p1&p2 [path-prefix]`, at most 500 characters per rule. Yandex
  drops common `utm_*` automatically, but list what the site's ads and partners actually append (`yclid` is not on
  Yandex's automatic list).
- If you ever add a `User-agent: Yandex` group, Yandex ignores the `*` group entirely: repeat every rule there.
- Never disallow `/_next/`, CSS, JS or images; both engines render with them.
- `Disallow` works differently per engine. Google: it stops crawling, not indexing; to remove a URL, serve `noindex`
  or 404 and let it be crawled. Yandex: it removes disallowed pages from search (within about a week of the robot
  seeing the rule), and a meta `robots`/`X-Robots-Tag` on a disallowed page has no effect.
- Staging also needs HTTP auth; robots alone does not keep URLs out of the index.

## Structured data (JSON-LD with schema-dts)

```tsx
// src/lib/seo/json-ld.tsx
import type { Graph, Thing, WithContext } from 'schema-dts'

export function JsonLd({ data }: { data: WithContext<Thing> | Graph }) {
  return (
    <script
      type="application/ld+json"
      // escape "<" so text from the API cannot close the script element
      dangerouslySetInnerHTML={{ __html: JSON.stringify(data).replace(/</g, '\\u003c') }}
    />
  )
}
```

```ts
// src/features/landing/json-ld.ts
import type { Graph } from 'schema-dts'
import type { Landing } from './data'
import { cityPath, landingPath } from '@/lib/routes'

export function landingGraph(l: Landing, site: string): Graph {
  return {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          { '@type': 'ListItem', position: 1, name: 'Главная', item: site },
          { '@type': 'ListItem', position: 2, name: l.cityName, item: `${site}${cityPath(l.city)}` },
          { '@type': 'ListItem', position: 3, name: l.h1, item: `${site}${landingPath(l.city, l.topic)}` },
        ],
      },
      {
        '@type': 'FAQPage', // only when the same questions and answers are visible on the page
        mainEntity: l.faq.map((f) => ({
          '@type': 'Question',
          name: f.question,
          acceptedAnswer: { '@type': 'Answer', text: f.answer },
        })),
      },
    ],
  }
}
```

| Type | Where | Rules |
|---|---|---|
| `Organization` | home page | `@id: ${site}/#org`, name, url, logo, `sameAs` of official profiles; other nodes reference it by `@id` |
| `LegalService` | contacts / office pages | only for a real office with a visible address, phone and hours (`PostalAddress` with `postalCode`, `addressLocality`); `parentOrganization: { '@id': … }`. Never one per city without an office |
| `BreadcrumbList` | every page below home | must mirror the visible breadcrumbs; Yandex uses JSON-LD breadcrumbs in snippets |
| `FAQPage` | landings with a visible FAQ block | semantics only: Google removed FAQ rich results in 2026. Yandex's Q&A snippets use `QAPage` with one question per page; do not fake `QAPage` on multi-question pages |
| `Article` | guides | `headline`, `datePublished`, `dateModified` from Go, `author` (`Person` with a real profile URL), `publisher: { '@id': … }` |

- Type every object with schema-dts (`WithContext<LegalService>`, `Graph`); a typo then fails `tsc`.
- Render `<JsonLd>` in the Server Component that owns the data, inside the static output.
- Validate with Google Rich Results Test, validator.schema.org and the Yandex.Webmaster validator. Markup does not
  rank by itself; it changes snippets.

## Internal linking

- Hubs: home → city hubs → city × topic pages; topic hubs → the same topic in every indexable city.
- Each landing links to sibling topics in the same city and the same topic in nearby cities, with descriptive
  Russian anchors («Юрист по разводу в Казани»), never «подробнее».
- Visible breadcrumbs on every page below home, built from `lib/routes.ts`.
- Use `<Link href>` (renders `<a href>`). Navigation through `onClick={() => router.push(...)}` is invisible to crawlers.
- Link only to canonical, indexable URLs; a link to a redirect or a `noindex` page wastes crawl budget.

## Pagination

- Path segments (`/moskva/advokaty/stranica-2`), so each page is static; `?page=2` is request data. Numbered pages
  need a stable page-number endpoint in the API for public listings (cursor pagination stays for the cabinets).
- Every page is self-canonical (Google: don't use the first page as the canonical of the series) with a unique
  title («… — страница 2»).
- Link pages with plain `<a href>` (previous, next, numbers). `pagination: { previous, next }` in metadata renders
  `rel=prev/next`; Google no longer uses it, other engines may.
- No infinite scroll without paginated URLs behind it.

## Status codes, removals, redirects

| Situation | Response | How |
|---|---|---|
| Unknown slug / API 404 | 404 + `noindex` | `notFound()` during the static render. Inside a streamed boundary it becomes 200 + `noindex` (soft 404): keep the call outside streaming on SEO routes |
| Page removed for good, no successor | 410 | `proxy.ts` returns `new NextResponse(null, { status: 410 })` for a build-time list of paths; 404 is acceptable otherwise |
| Slug renamed, pages merged | 308 | `redirects()` in `next.config.ts` (`permanent: true`) or a build-time map in `proxy.ts`; both engines treat 308 as permanent |
| Temporarily moved | 307 | `permanent: false` |

- Never redirect from inside a streamed component: `redirect()` after streaming starts becomes a client-side meta
  refresh, not an HTTP redirect.
- No chains: point old URLs at the final target and update internal links and sitemaps in the same release.

## Core Web Vitals budgets

| Metric (p75, mobile) | Budget | Main levers here |
|---|---|---|
| LCP | ≤ 2.5 s | static HTML from cache; the LCP image via `next/image` with `loading="eager"` and `fetchPriority="high"` (`fetchPriority` alone still renders `loading="lazy"`) and correct `sizes`; fonts via `next/font` |
| INP | ≤ 200 ms | little client JS on SEO pages; no heavy work in input handlers; third-party scripts `afterInteractive`/`lazyOnload` |
| CLS | ≤ 0.1 | image dimensions; `next/font` fallback metrics; reserve space for banners, consent bars and async blocks |
| TTFB | ≤ 0.8 s | prerendered pages, cached data; no request-time work on SEO routes |

- Collect field data with `useReportWebVitals` (`next/web-vitals`) in a small Client Component and send it to
  analytics; lab tools alone miss real devices.
- Track first-load JS per route with `pnpm next analyze --output` and fail review when an SEO route grows without reason.

## Yandex specifics

- **Complete HTML.** See "Rendering for crawlers". YandexBot gets a request-time render of the same page. By default
  Yandex itself decides whether to execute JavaScript; its help recommends turning JavaScript rendering off
  (Индексирование → Рендеринг страниц JavaScript) for sites with SSR or prerendering.
- **Webmaster.** Verify with the meta tag above; add both `https://example.ru` and any mirror; submit the sitemap
  chunks; watch "Страницы в поиске → Исключённые" and "Диагностика" after releases.
- **Clean-param** for tracking parameters (robots.ts above).
- **Regionality.** Set the site region in Webmaster ("Представление в поиске → Региональность"): usually only one
  region is assigned. For several cities, add each real office to Yandex Business and publish its full address with
  postal code and city phone code as text (not an image) on the site; a Webmaster region request links to that
  contacts page. Region subdomains must carry content different from the main domain, or they are duplicates.
- **IndexNow** (optional): Yandex accepts change notifications at `https://yandex.com/indexnow`; Go knows when pages
  change and can send them after calling the revalidate webhook.
- **No cloaking.** Same content for bots and people. Never branch on `User-Agent` in application code. Next's
  bot-specific blocking metadata changes delivery, not content, and is fine.

### Yandex Metrica without blocking render

```tsx
// src/components/analytics/yandex-metrika.tsx — rendered by src/app/(public)/layout.tsx only
'use client'
import { Suspense, useEffect, useRef } from 'react'
import Script from 'next/script'
import { usePathname, useSearchParams } from 'next/navigation'
import { publicEnv } from '@/lib/env.public'

declare global {
  interface Window {
    ym?: (counterId: number, method: string, ...args: unknown[]) => void
  }
}

const COUNTER_ID = publicEnv.yandexMetrikaId // Number(process.env.NEXT_PUBLIC_YANDEX_METRIKA_ID ?? '') in lib/env.public.ts
// Paste verbatim from Metrica → Настройка → Код счётчика: the IIFE that defines window.ym and loads tag.js.
const LOADER = '/* Metrica loader */'

function PageHits() {
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const lastUrl = useRef<string | null>(null)

  useEffect(() => {
    const url = window.location.href
    if (lastUrl.current === null) {
      lastUrl.current = url // the landing hit is sent by the init script below
      return
    }
    if (url === lastUrl.current) return
    window.ym?.(COUNTER_ID, 'hit', url, { referer: lastUrl.current })
    lastUrl.current = url
  }, [pathname, searchParams])

  return null
}

export function YandexMetrika() {
  if (!Number.isInteger(COUNTER_ID) || COUNTER_ID <= 0) return null
  return (
    <>
      <Script id="yandex-metrika" strategy="afterInteractive">
        {`${LOADER}
ym(${String(COUNTER_ID)}, 'init', { defer: true, clickmap: true, trackLinks: true, accurateTrackBounce: true });
ym(${String(COUNTER_ID)}, 'hit', window.location.href);`}
      </Script>
      <Suspense fallback={null}>
        <PageHits />
      </Suspense>
    </>
  )
}
```

```tsx
// src/app/(public)/layout.tsx
import { YandexMetrika } from '@/components/analytics/yandex-metrika'

// Public pages only: the counter must never run in the cabinet, where clients' phones are on screen.
export default function PublicLayout({ children }: LayoutProps<'/'>) {
  return (
    <>
      {children}
      <YandexMetrika />
    </>
  )
}
```

- **Public pages only.** The root layout also wraps the cabinets (`/account`, `/cabinet`, `/admin`), where phone
  numbers are in the DOM and a third-party script could call `/api` with the user's cookie. Render the counter from
  `(public)/layout.tsx`, never from `app/layout.tsx`.
- **Counter settings** (Метрика → Настройка): keep «Использовать дополнительные настройки отслеживания» off (it sends
  contact details typed into forms to Metrica, hashed, for ad algorithms), and with it «Запоминать маскированные
  данные». On the Вебвизор tab keep «Записывать все поля» off, so field contents are recorded as asterisks.
- **Form fields** carry the CSS class `ym-disable-keys` (lead-form example in `nextjs-patterns`): if someone turns
  «Записывать все поля» on, Webvisor still records those fields as asterisks.
- `afterInteractive` loads the counter after hydration, so it never delays HTML or LCP (and the server HTML does
  not contain it).
- `defer: true` disables the automatic page view, as Metrica's SPA guide requires; client navigations send
  `hit` manually. The `<Suspense>` keeps `useSearchParams` from client-rendering the page.
- Webvisor records visitor sessions, including form interaction: enable it on lead-form pages only after a legal
  (152-ФЗ) decision.

## Review checklist

- [ ] Raw HTML (YandexBot and Googlebot UA) has status 200, `<title>`, canonical, one `<h1>`, main text, JSON-LD, and no
  streaming placeholders; no `<Suspense>` around the main content
- [ ] `ensureStatic = 'navigation'`, cached data only, no request data in `(public)` layouts
- [ ] Canonical built by `lib/routes.ts`, absolute, self-referencing, parameter-free
- [ ] `indexable` from Go drives `robots`, sitemap membership and hub links together
- [ ] Unique, data-backed content blocks; no city-name-swap templates
- [ ] Sitemaps list only 200 + canonical + indexable URLs with real `lastModified`; robots.ts lists every chunk
- [ ] Unknown slugs return a real 404; renames return 308 without chains
- [ ] JSON-LD typed with schema-dts, mirrors visible content, `<` escaped
- [ ] LCP image `loading="eager"` + `fetchPriority="high"`, dimensions set, fonts with the Cyrillic subset, third-party scripts deferred
- [ ] `verification.yandex` present; `Clean-param` covers the parameters ads append; staging is `Disallow: /` + auth
