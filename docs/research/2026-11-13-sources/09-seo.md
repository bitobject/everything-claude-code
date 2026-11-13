<!-- Independent reviewer's report, 2026-11-13: SEO for Yandex and Google.
     Raw model output, kept for its sources and detail. Which claims were re-verified is stated in
     ../2026-11-13-best-practices-review.md (sections marked [проверено] / [опровергнуто]). -->

## Review: technical and content SEO for Yandex and Google

### 1. Verdict
Our SEO material is better than most and mostly current. The Next.js 16.4 behaviour it relies on checks out against the docs and the framework source: `htmlLimitedBots` matches Yandex, `ensureStatic`, robots `other`, the `generateSitemaps` URL and Promise id, and `notFound()`/`redirect()` inside a stream. The Google and Yandex technical facts are also correct: FAQ rich results are gone, rel=prev/next is ignored, each paginated page is its own canonical, the doorway wording is verbatim, and Clean-param, 308 and regionality are right.

The gaps are in what both engines weigh most for a legal YMYL lead-gen site:
- There is no expertise or trust layer.
- There are no rules for review markup.
- Yandex's real sanctions and its behavioural ranking signals are missing. Meanwhile, a harmless Yandex indexing status is presented as the doorway penalty.
- Several technical points are wrong or untested: the LCP recipe leaves the image lazy-loaded, the security skill's nonce CSP contradicts static SEO pages, Metrica runs in the cabinet without consent, and the streamed request-time render that YandexBot actually receives is not tested.

The architecture does not need replacing; it needs these corrections and additions.

**Assumptions:** Next 16.4.6 renders bots the way current canary source does (I read the source; I could not run it). The 152-FZ points come from statute summaries, a ministry letter and practitioner columns, not from legal advice.

### 2. Confirmed
- The default `htmlLimitedBots` list matches `yandex`, and also `Google-InspectionTool`, `Chrome-Lighthouse` and Bingbot [1][2].
- With partial prerendering (PPR), these bots skip the prerendered page and get a request-time render, so "data must also work at request time" is correct (`shouldForceDynamicPPRRender`) [2].
- `ensureStatic='navigation'` (16.4) makes unlisted params wait for the complete page. Without it, 16.3+ serves the App Shell first [3][4].
- `notFound()` after streaming starts gives 200 + noindex. `redirect()` inside a stream becomes a meta refresh, which Yandex treats as a temporary redirect [6][29].
- Robots `other` (added in 16.3), the `generateSitemaps` URL and Promise id, `pagination` and `verification.yandex` all exist as described [5][7].
- Each paginated page is its own canonical; Google ignores prev/next [17]. The doorway and scaled-content quotes match the current text (updated 2026-08-28) [11].
- FAQ rich results stopped on 2026-05-07 and the docs were removed on 2026-06-15 [13].
- Clean-param syntax, the 500-character limit and placement anywhere in the file are correct. A `User-agent: Yandex` group does make Yandex ignore `*` [28].
- For trailing slashes Yandex wants a 301/308 redirect, says canonical "может игнорироваться", and lists 308 as permanent [29].
- Regionality: normally one region per site, offices registered in Yandex Business, a contacts-page link in the region request, and subdomains must differ from the main site [27].
- Yandex reads JSON-LD for BreadcrumbList; QAPage requires one question per page [30].
- Sitemaps: 50,000 URLs or 50 MB per file, accurate lastmod, Google ignores priority [18][31].
- IndexNow endpoint `yandex.com/indexnow`; Google does not participate [31][38]. Metrica on an SPA: `defer: true` plus manual `hit` [34]. Core Web Vitals: LCP 2.5 s, INP 200 ms, CLS 0.1 [19]. Escaping `<` in JSON-LD [10].

### 3. Problems

**P1 – high (confidence high).** `skills/security-review/SKILL.md` "Headers and transport" requires a nonce-based CSP, while STACK §3 and the `seo` skill require prerendered pages. Next's docs say that with nonces "all pages must be dynamically rendered… PPR is incompatible" [9]. Also, `ensureStatic='navigation'` fails on `headers()`.
*Fix:* use a static CSP on public routes (Next's experimental SRI, or a scoped `'unsafe-inline'`), and use nonces only in the cabinet. Move the Metrica init code into a static first-party JS file. Settle this with the security reviewer.

**P2 – medium (high).** The LCP row in `seo` "Core Web Vitals budgets" and `nextjs-patterns` "Images" both say to give the LCP image `fetchPriority="high"`. In Next 16, `isLazy = !priority && !preload && (loading==='lazy' || undefined)`, so `fetchPriority` alone still renders `loading="lazy"`, and Next warns at runtime to add `loading="eager"` [2][8].
*Fix:* the single LCP image gets `preload`, or `loading="eager"` with `fetchPriority="high"`.

**P3 – medium (high).** The first paragraph of `seo` "Programmatic pages" and the agent's first "High" bullet treat the status «Малоценная или маловостребованная» as the doorway penalty. Yandex says such exclusions are not violations and "негативно не влияет на ранжирование", and that it has no index quotas [24]. The sanctions that actually fit city templates are:
- «Малополезный контент», which explicitly covers "шаблонных методик (подстановок ключевых слов и фраз)" and auto-generated service catalogues with outdated data;
- «Использование SEO-текстов», which covers query spam and keyword lists in navigation [25].

*Fix:* name these two, monitor «Безопасность и нарушения», and treat low-demand exclusions as a demand signal.

**P4 – medium (medium; read from source, not run).** The `nextjs-patterns` "SEO routes" example wraps the main content in `<Suspense fallback={<LandingSkeleton/>}>` and claims `notFound()` there "runs during the static render". YandexBot never gets the static render. It gets a request-time render that streams, because `supportsDynamicResponse` makes `waitForAllReady` false [2]. With a cold cache, or after `revalidateTag(…, {expire:0})`, the raw HTML can contain the skeleton followed by hidden streamed segments. A `notFound()` inside the boundary would then be a soft 404; today it is avoided only because `generateMetadata` calls `notFound()` first.
*Fix:* no Suspense around SEO main content. Run the e2e tests and the agent's live checks (status, 404, content parity) under browser, Googlebot and YandexBot user agents, and assert there are no streaming placeholders (`<template id="B:`).

**P5 – low–medium (high).** The TTFB row in the `seo` CWV table says "no request-time work on SEO routes". That is false for YandexBot, Bingbot, Search Console's live test (`Google-InspectionTool`) and PageSpeed Insights (`Chrome-Lighthouse`): every hit is a server render [2].
*Fix:* size the Next server for crawler renders, watch response times in Yandex's «Статистика обхода», and trust field data over PageSpeed lab numbers.

**P6 – low–medium (high).** The agent's intro says "Yandex reads raw HTML strictly". In fact Yandex renders JavaScript at its own discretion by default, and its help says «Запретите рендеринг, если на сайте реализован SSR» [23].
*Fix:* add «Индексирование → Рендеринг страниц JavaScript» set to off to the setup steps.

**P7 – medium (high).** In `seo` "Yandex Metrica", the counter is mounted in the root layout, so it also runs in the cabinet, where leads' personal data is shown. This contradicts `security-review` ("not in… third-party scripts"). Metrica's automatic goals fire on `tel:` clicks, and the counter option «Использовать дополнительные настройки отслеживания» sends contact data typed into forms (hashed) to Yandex for ad algorithms [34].
*Fix:*
- Mount Metrica in the `(public)` layout only.
- Require these counter settings to be off: that option, «Запоминать маскированные данные», and Webvisor «Записывать все поля».
- Use `sendTitle:false` wherever page titles can contain personal data.
- Add `ym-disable-keys` to lead-form fields if Webvisor is ever enabled.

**P8 – medium (medium; the law is unsettled).** In the same section the counter loads unconditionally. Russia has no cookie-specific rule and Roskomnadzor has published none, but 2026 practitioner guidance treats analytics cookies as needing consent [37]. Since 2025-09-01, consent to personal-data processing must be a separate document (156-FZ) [36].
*Fix:* gate `ym init` on a consent state from day one, with the default decided by legal counsel. The banner must reserve its space (no CLS) and must not become the LCP element.

**P9 – low–medium (medium).** `seo` "Internal linking" has every landing link to the same topic in nearby cities with exact anchors («Юрист по разводу в Казани»). Across thousands of pages this is the keyword-list-in-navigation pattern that Yandex's SEO-text policy names [25].
*Fix:* a few contextual links with natural anchors, and no site-wide city × topic link grids.

**P10 – low (high).** The `seo` robots notes say "Disallow stops crawling, not indexing". That is Google-only: Yandex removes disallowed pages within about two weeks and ignores the meta tags of disallowed pages [24][29]. As a result, the cabinet's `noindex` is never read, because the cabinet is also disallowed.
*Fix:* state the rule per engine and pick one mechanism per path.

**P11 – low (high).** The agent's "Quality bar" bans meta keywords as folklore. Yandex's help still says `keywords` "может влиять на соответствие страницы поисковым запросам" [29].
*Fix:* say "optional, low value, never stuffed".

**P12 – low (medium).** For filter pages, `seo` "URLs and canonicals" combines `noindex` with a canonical to another page. These signals contradict each other [22]. Google's faceted-navigation guide prefers a robots.txt disallow, and calls canonical "less effective in the long term" [17]. Yandex offers Clean-param and a new «Настройка GET-параметров» tool (2025-12-12) [28].
*Fix:* sort parameters get Clean-param plus a canonical; filter parameters that are not needed in search get a Disallow pattern.

**P13 – low (high).** `seo` "Pagination": Yandex's only official statement (Platon Shchukin's blog, 2015) recommends a canonical to page 1 for pages with no traffic [33]. Self-canonical is still the better choice for discovery.
*Fix:* add a note that Yandex will likely exclude pages 2+ as low-demand, and that this is harmless.

**P14 – low (medium).** The `seo` robots.ts example calls the Go API to count sitemap chunks. When robots.txt returns 5xx, Google stops crawling for 12 hours [18].
*Fix:* keep robots.txt static and point it at a single sitemap-index route.

**P15 – low (high).** STACK.md §3 credits `partialPrefetching` with giving on-demand pages complete HTML. It only changes client-side prefetching; `ensureStatic='navigation'` is what produces the complete HTML [3][10].
*Fix:* also set `ensureStatic` in `(public)/layout.tsx`, so hubs and guides inherit it instead of relying on per-page exports.

### 4. Gaps
- **Expertise and trust layer – now.** Yandex's ЭПОС guidance names «юриспруденции» among high-stakes topics. Its Проксима quality metric uses author-authority signals for «юридические… услуги» [26]. Google gives YMYL topics "even more weight" for E-E-A-T, with trust first; the current Quality Rater Guidelines are the 2025-09-11 edition [19][21]. To add:
  - lawyer profiles with verifiable status; call someone «адвокат» only after a registry check;
  - a named reviewer and review date on guides;
  - legal entity details (наименование, адрес, ОГРН) and contacts;
  - a "how we work as an intermediary" block, which is Yandex's own example of good practice [25];
  - nothing that looks like a court or Госуслуги (Yandex's «Мимикрия» policy).
- **Review markup – now.** Google allows review markup only about a specific item, never on list or category pages, so never on city × topic landings. Star ratings for local businesses are allowed only for reviews about other businesses, so lawyer profiles qualify. No self-serving reviews, no imported reviews, and no undisclosed incentivised reviews (added 2026-07-24) [14][13].
- **Yandex behavioural signals – now.** Yandex trains ranking on how users interact with results. «Имитация действий пользователей» (faking user visits) is penalised site-wide [25][26]. Add it to the agent's "No manipulation" list, and track lead-submit goals.
- **Demand-aware, stable indexing – now.** Add Wordstat/Webmaster query demand to Go's `indexable` rule, plus thresholds with a margin so pages don't flip between index and noindex.
- **Yandex Webmaster setup – now:**
  - JavaScript rendering off;
  - the GET-parameter tool;
  - link Metrica to Webmaster and make an explicit decision on crawling pages found by the counter;
  - set crawl rate in Webmaster (Crawl-delay is ignored since 2018), and never emit Next's `host`/`crawlDelay`;
  - region «Россия» or none;
  - no regional subdomains without real offices (Yandex: «не рекомендуем»);
  - Turbo pages were closed on 2025-04-01 [23][28][27][31].
- **No GA4/GTM – now.** A Минцифры letter of 2025-05-12 calls Google Analytics a cross-border transfer. Since 2025-07-01, 23-FZ forbids collecting personal data through foreign databases, and fines rose from 2025-05-30 [35][36]. Search Console needs only verification; prefer a DNS Domain property.
- **Lead-gen UX policies – now.** Google's back-button-hijacking policy is enforced since 2026-06-15 [12], and Yandex's services feeds forbid popups [32]. Ban exit popups and history tricks, and audit chat/call-back widgets for INP.
- **WebSite structured data – now.** Add it to the home page for site names [19].
- **Yandex services features – later.** The rating for service aggregators and the «Надёжный бизнес» status (reply to reviews in Webmaster), and «Услуги» YML feeds of lawyer offers [32].
- **AI answers – later.** Google needs no special markup [19]. Yandex picks Алиса AI sources by the ЭПОС criteria; track Webmaster's «Видимость сайта в Алисе AI» [26][31].
- **Timing – later.** A Google core update has been rolling out since 2026-11-10 [20]. Avoid structural launches mid-rollout so effects can be attributed.

### 5. Over-engineering
- **FAQPage JSON-LD on every landing.** No engine uses it any more: Google retired the feature and dropped it from the Rich Results Test [13], and Yandex uses QAPage only for one question per page [30]. Keep the visible FAQ, drop the markup and its "must mirror visible content" checks across thousands of pages.
- **rel=prev/next through `pagination`.** Google ignores it and Yandex doesn't document it; plain links do the job.
- **Organization/LegalService JSON-LD aimed at Yandex.** Yandex documents JSON-LD only for breadcrumbs, Q&A and Mail [30]. Keep a minimal Organization + WebSite graph for Google.

### 6. Sources
1. Next.js, htmlLimitedBots (2025-10-03) https://nextjs.org/docs/app/api-reference/config/next-config-js/htmlLimitedBots
2. vercel/next.js canary source, read 2026-11-12: html-bots.ts, app-page-runtime.ts, app-render.tsx, get-img-props.ts https://github.com/vercel/next.js/tree/canary/packages/next/src
3. Next.js, ensureStatic (2026-10-06) https://nextjs.org/docs/app/api-reference/file-conventions/route-segment-config/ensureStatic
4. Next.js, ISR with Cache Components https://nextjs.org/docs/app/guides/incremental-static-regeneration-cache-components
5. Next.js, generateMetadata (2026-11-06) https://nextjs.org/docs/app/api-reference/functions/generate-metadata
6. Next.js, notFound (2026-11-06) https://nextjs.org/docs/app/api-reference/functions/not-found and redirect (2026-07-28) https://nextjs.org/docs/app/api-reference/functions/redirect
7. Next.js, robots (2026-05-01) https://nextjs.org/docs/app/api-reference/file-conventions/metadata/robots and generateSitemaps (2026-09-28) https://nextjs.org/docs/app/api-reference/functions/generate-sitemaps
8. Next.js, Image (16.4.6) https://nextjs.org/docs/app/api-reference/components/image
9. Next.js, Content Security Policy (2026-03-20) https://nextjs.org/docs/app/guides/content-security-policy
10. Next.js, partialPrefetching (2026-10-02) https://nextjs.org/docs/app/api-reference/config/next-config-js/partialPrefetching and JSON-LD https://nextjs.org/docs/app/guides/json-ld
11. Google, Spam policies (2026-08-28) https://developers.google.com/search/docs/essentials/spam-policies
12. Google, back button hijacking (2026-04-13) https://developers.google.com/search/blog/2026/04/back-button-hijacking
13. Google, Search Central documentation updates https://developers.google.com/search/updates
14. Google, Review snippet (2026-09-08) https://developers.google.com/search/docs/appearance/structured-data/review-snippet
15. Google, Robots meta tag (2026-03-24) https://developers.google.com/search/docs/crawling-indexing/robots-meta-tag
16. Google, JavaScript SEO basics (Dec 2025 changes) https://developers.google.com/search/docs/crawling-indexing/javascript/javascript-seo-basics
17. Google, Faceted navigation https://developers.google.com/search/docs/crawling-indexing/crawling-managing-faceted-navigation and Pagination https://developers.google.com/search/docs/specialty/ecommerce/pagination-and-incremental-page-loading
18. Google, Sitemaps https://developers.google.com/search/docs/crawling-indexing/sitemaps/build-sitemap and robots.txt spec https://developers.google.com/crawling/docs/robots-txt/robots-txt-spec
19. Google: Core Web Vitals https://developers.google.com/search/docs/appearance/core-web-vitals · Helpful content https://developers.google.com/search/docs/fundamentals/creating-helpful-content · AI features https://developers.google.com/search/docs/appearance/ai-features · Site names https://developers.google.com/search/docs/appearance/site-names
20. Google Search Status Dashboard https://status.search.google.com/products/rGHU1u87FJnkP6W2GwMi/history
21. Google, Quality Rater Guidelines PDF (Last-Modified 2025-09-11) https://guidelines.raterhub.com/searchqualityevaluatorguidelines.pdf; Search Engine Roundtable (2025-09-11) https://www.seroundtable.com/google-search-quality-raters-guidelines-update-40092.html
22. Search Engine Journal, Mueller on noindex + canonical (2018) https://www.searchenginejournal.com/google-dont-mix-noindex-relcanonical/262607/
23. Yandex, JavaScript indexing https://yandex.ru/support/webmaster/ru/yandex-indexing/rendering
24. Yandex, low-value/low-demand pages https://yandex.ru/support/webmaster/ru/site-indexing/low-demand
25. Yandex violations: https://yandex.ru/support/webmaster/ru/threat/useless-content · /threat/seo-text · /threat/user-actions-imitation · /threat/unfair-actions · /threat/mimicry
26. Yandex: ЭПОС https://yandex.ru/support/webmaster/ru/epos · search quality (Проксима) https://yandex.ru/support/webmaster/ru/search-quality · Алиса AI https://yandex.ru/support/webmaster/ru/alice
27. Yandex: regionality https://yandex.ru/support/webmaster/ru/site-geography/site-region · subdomains https://yandex.ru/support/webmaster/ru/yandex-indexing/subdomain
28. Yandex: https://yandex.ru/support/webmaster/ru/robot-workings/clean-param · /robot-workings/user-agent · /robot-workings/crawl-delay · /indexing-options/get-parameters
29. Yandex: https://yandex.ru/support/webmaster/ru/robot-workings/double · /robot-workings/canonical · /robot-workings/managing-redirects · /error-dictionary/http-codes · /controlling-robot/metatags · /robot-workings/unhelpful
30. Yandex: https://yandex.ru/support/webmaster/ru/json-ld/about · /supported-schemas/navigation-links · /supported-schemas/q-and-a · /supported-schemas/address-organization
31. Yandex: sitemap https://yandex.ru/support/webmaster/ru/controlling-robot/sitemap · IndexNow https://yandex.ru/support/webmaster/ru/indexing-options/index-now · Webmaster changelog https://yandex.ru/support/webmaster/ru/service/about
32. Yandex: https://yandex.ru/support/webmaster/ru/search-results/site-services · /service/rating · /service/reliable-business · /search-appearance/offices/about · /feed/executors
33. Yandex, Platon Shchukin blog, pagination (2015-12-29) https://yandex.ru/blog/platon/2878
34. Yandex Metrica: https://yandex.ru/support/metrica/ru/code/counter-spa-setup · /code/counter-initialize · /webvisor/settings · /general/counter-general · /general/auto-goals
35. Минцифры letter 12.05.2025 № П25-44929 https://rulaws.ru/acts/Pismo-Mintsifry-Rossii-ot-12.05.2025-N-P25-44929/
36. 156-FZ summary (Infostart) https://infostart.ru/journal/news/uchet-nalogi-pravo/s-1-sentyabrya-soglasie-na-obrabotku-personalnykh-dannykh-nuzhno-podpisyvat-otdelno_2466704/; 420-FZ fines (buh.ru) https://buh.ru/news/s-30-maya-shtraf-za-narushenie-sroka-podachi-uvedomleniya-v-roskomnadzor-sostavit-do-300-000-rubley.html
37. Practitioner columns: A. Stratulat, ppt.ru (2026-09-22) https://ppt.ru/columns/jurist-v-it/kuki-i-personalnye-dannye-chto-trebuet-152-fz; SberBusiness Live (2026-03-11) https://sberbusiness.live/publications/cookie
38. IndexNow FAQ https://www.indexnow.org/faq

The raw downloads (Yandex help pages as Markdown, Next.js source files) are in `/tmp/claude-0/-root-lead-new--localenv/6b41f19b-ec42-4595-8dc3-68f60461b2ed/scratchpad/research-seo/`.
