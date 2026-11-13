<!-- Independent reviewer's report, 2026-11-13: Security, personal data, supply chain and operations.
     Raw model output, kept for its sources and detail. Which claims were re-verified is stated in
     ../2026-11-13-best-practices-review.md (sections marked [проверено] / [опровергнуто]). -->

# Review: security, personal data, supply chain and operations

I read the files on 2026-11-12/13. Some of them changed while I was reading (tool pinning; STACK §4 now pins Node 26.13.1 and pnpm 12.16.1), so line numbers are approximate. **Assumptions:** the SEO pages and the cabinet share one origin, as STACK says. An nginx-type reverse proxy sits in front. The CI platform isn't chosen yet, so the GitHub-specific advice applies only if you pick GitHub. The legal points are an engineer's reading of the primary texts; a Russian data-protection lawyer must confirm them.

## 1. Verdict

The core application-security patterns are current and correct:
- object-level authorization in use cases, with 404s and owner filters in SQL;
- opaque server-side sessions;
- `http.CrossOriginProtection` behind a same-origin `/api`;
- Next.js treated as an untrusted rendering tier (no database, Server Actions treated as public endpoints, proxy.ts not an auth boundary);
- versions that already include the Oct–Nov 2026 Next.js and React fixes.

The weak points are elsewhere:
- **The lead form.** It has no consent capture and no anti-bot design, and the way the client IP reaches Go can be spoofed.
- **The CSP rule contradicts the rendering model.** We require nonces, which don't work on prerendered pages.
- **Operations are mostly missing.** There is no audit trail, backup/PITR, readiness probe or drain, error tracking, npm update policy, real secret scanning or CI hardening.
- **152-FZ coverage stops at "store in Russia, keep it out of logs".** The 2025–2026 obligations that change code are absent.

## 2. Confirmed

- BOLA defence in use cases, 404 for other users' IDs, owner filter in queries, negative tests [3][7].
- Opaque CSPRNG session IDs; HttpOnly/Secure/SameSite; rotation on login and privilege change; revocation [1 §7][4].
- OTP rules: 6+ digits, ≤5 min, per-code and per-phone attempt limits, constant-time compare [1 6.5.4/6.5.5/6.6.3].
- `CrossOriginProtection` is used within its real semantics (Sec-Fetch-Site/Origin; safe methods and header-less requests pass), plus "no state change on GET" [17].
- proxy.ts is never the auth boundary, which is the lesson of CVE-2025-29927 [12].
- Next has no DB credentials and no business rules; Go authorizes every call; `server-only`; no per-user data in shared caches [7]. This also contains the damage from the RSC and Next remote-code-execution bugs (RCEs) [10][11].
- Multi-instance setup: shared `cacheHandlers` with `refreshTags`, `NEXT_SERVER_ACTIONS_ENCRYPTION_KEY`, `deploymentId` [8].
- Versions are current: Next 16.4.6 includes the 3 Nov 2026 cache-leak and self-hosting fixes (patched in 16.4.5); React 19.3.1 includes the 14 Oct 2026 RSC RCE fix; Go 1.27.4, x/vuln 1.9.0 and OTel Go 1.47.0 are current [10][11].
- Body caps, server timeouts, sqlc-only SQL, allow-listed ORDER BY, statement_timeout [3].
- govulncheck (reachability-based) and gosec in the gate; `os.OpenRoot` for paths from input [17].
- The DSN parse error is deliberately not wrapped; pgx's password redaction is only "best effort" [18].
- The 14-day release-age policy for tool binaries, with a security exception, is sound; Problem 13 extends it to app dependencies.

## 3. Problems

**P1. Nonce CSP contradicts prerendering.**
- **Where:** security-review/SKILL.md "Headers and transport" (l.101) and the security-reviewer checklist.
- **We say:** "CSP (nonce-based for scripts)".
- **Evidence:**
  - Next.js docs: nonces require every page to be dynamically rendered, static optimization and ISR are disabled, and "PPR is incompatible with nonce-based CSP" [6].
  - Experimental SRI covers external chunks but not the inline `self.__next_f` scripts [6][13].
  - Our Metrica loader is inline, and Yandex says inline code needs a nonce or hash [14].
- **Severity/confidence:** high / high. An agent that follows the rule either makes thousands of SEO pages dynamic or ships a broken CSP.
- **Change:** split the policy.
  - Public pages: a static CSP from next.config `headers()` or the proxy: `script-src 'self' 'unsafe-inline' https://mc.yandex.ru https://yastatic.net; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; connect-src 'self' https://mc.yandex.ru`. Roll it out Report-Only first.
  - Cabinet (already dynamic): nonce plus `'strict-dynamic'` set in proxy.ts.
  - Move the Metrica loader into an external file.

**P2. The lead form has no consent capture.**
- **Where:** nextjs-patterns "Server Actions and forms" (`LeadInput`, `submitLead`), the reference `CreateLeadRequest`, and the "Personal data" section of security-review.
- **Evidence:**
  - 152-FZ Art. 9(1), in force since 1 Sep 2025 (156-FZ): consent must be "оформлено отдельно от иных информации и (или) документов" [19][20].
  - Fine: 300–700k RUB under 13.11(2) KoAP.
  - Passing leads to lawyers is a transfer to third parties.
- **Severity/confidence:** high / high on the requirement; medium on the form and UX details (lawyer must confirm the legal basis and wording).
- **Change:**
  - Add a `consents` field to the contract: document id and version per purpose (processing, transfer to lawyers, marketing).
  - Use unticked checkboxes that link to the separate consent text, and a privacy-policy link on every form (Art. 18.1(2)).
  - Go stores an append-only consent record (version hash, time, form, IP).
  - Support withdrawal: stop processing and destroy within 30 days (Art. 21(5)).

**P3. The client IP used for rate limiting can be spoofed.**
- **Where:** nextjs-patterns l.392–393: "Forward `x-forwarded-for` … if Go rate-limits by IP". Nothing defines trusted proxies, and Go is also reachable from browsers via `/api`.
- **Evidence:**
  - MDN: use only addresses added by trusted proxies, read from the right; if the server can be reached any other way, "no part" of the header is trustworthy [16].
  - otelhttp v0.72.0 takes the leftmost X-Forwarded-For value as `client.address` [18].
- **Severity/confidence:** medium-high / high. Lead-spam and SMS limits become bypassable.
- **Change:**
  - The proxy overwrites (not appends) one header with `$remote_addr`.
  - One function, `platform/httpserver.ClientIP`, trusts that header only from configured proxy and Next CIDRs, otherwise uses `RemoteAddr`.
  - Never use telemetry IPs for decisions.

**P4. Metrica runs inside the authenticated area.**
- **Where:** seo/SKILL.md l.305: "render once in the root layout". The root layout wraps `(cabinet)` and `/login`.
- **Why it matters:**
  - A third-party script gets same-origin `fetch('/api/…')` with the lawyer's cookies (HttpOnly doesn't prevent that) and sees DOM containing clients' phones and questions.
  - Webvisor is configured per counter, so "lead-form pages only" can't be expressed with this snippet.
- **Evidence:** ASVS 14.2.3 (no sensitive data to trackers) [1][14].
- **Severity/confidence:** medium / high.
- **Change:**
  - Load Metrica only in the `(public)` layout, behind a consent switch.
  - Mark form fields with `ym-disable-keys` / `ym-hide-content`.
  - No personal data in goals or params; no third-party scripts in the cabinet.

**P5. The secret check is blind and noisy.**
- **Where:** verification-loop "Always" and security-reviewer step 2: `git diff HEAD | grep -nEi '(secret|token|…)'`.
- **Problem:**
  - It skips untracked files, which is exactly where a new `.env.prod` or config file lands.
  - It misses DSNs (`postgres://u:p@`), JWTs and cloud keys.
  - It flags every `csrfToken`.
- **Evidence:** OWASP recommends pre-commit secret detection [5].
- **Severity/confidence:** medium / high.
- **Change:** gitleaks 8.30 (now in maintenance mode) or its successor Betterleaks (Mar 2026, reads `.gitleaks.toml`) [30]. Run it on staged changes pre-commit, over full history in CI, and with `dir` for untracked files.

**P6. Validation errors echo the submitted values.**
- **Where:** reference main.go l.79–86 and `BadRequestWriter`: `Detail: err.Error()`.
- **Evidence (verified in source):**
  - kin-openapi v0.153.0 appends "Schema:" and "Value:" dumps to `SchemaError.Error()`.
  - For OpenAPI 3.1, `DisableSchemaErrorDetails()` only returns `ErrSchemaErrorDetailsWithJSONSchema2020`.
  - jsonschema v6 `pattern` messages quote the value [18].
- **Effect:** a mistyped phone number comes back in the problem body, and into every log that records problems. This breaks our own rule "not in error messages".
- **Severity/confidence:** low-medium / high.
- **Change:** a fixed `detail` plus `errors[{pointer, keyword}]` built from `JSONPointer()`/`SchemaField`; add a test that a submitted value never appears in a response.

**P7. The reviewer drops findings without an exploit.**
- **Where:** security-reviewer.md l.90: "No theoretical findings without a path to exploit".
- **Problem:** missing consent, audit trail, retention or backups are OWASP Top 10:2025 A09 or 152-FZ findings with no exploit, so they get suppressed [2].
- **Severity/confidence:** medium / medium.
- **Change:** add a separate "compliance and hardening" section to the report.

**P8. One database role for runtime and migrations.**
- **Where:** `app migrate` reuses `DATABASE_URL`; postgres-patterns has no grants. "A module reads only its own schema" is a convention, not enforced.
- **Severity/confidence:** medium / high.
- **Change:**
  - An owner role for goose and River migrations.
  - A runtime role with DML only on module schemas.
  - INSERT-only rights on audit tables.
  - SCRAM authentication (PostgreSQL 18 deprecates MD5 [31]); `sslmode=verify-full` when the database is off-host.

**P9. Liveness only, no drain.**
- **Where:** reference `GET /healthz`; `httpserver.Run` calls `Shutdown` immediately on SIGTERM.
- **Severity/confidence:** medium / high. Leads get dropped during deploys.
- **Change:**
  - `/livez` checks the process only; `/readyz` pings the pool with a ≤1 s timeout [33].
  - On SIGTERM: fail readiness, wait at least the load balancer's deregistration delay, then call `Shutdown`.
  - River stops softly first, with a timeout.
  - Give Next 10–30 s to drain [8].

**P10. The auth model is undecided and inconsistent.**
- **Where:** security-review assumes OTP and one-time links, e2e-runner logs in with email and password, and STACK has no row for it.
- **Evidence:** 149-FZ Art. 8(10) limits sign-in to a Russian mobile number, ESIA, EBS or a Russian-owned system; Google, Apple, GitHub and Telegram sign-in are excluded. Fines of 500–700k RUB for companies since 7 Jul 2026, per secondary sources; lawyer to confirm [23].
- **Severity/confidence:** medium / high.
- **Change:** add a STACK row covering:
  - phone OTP through a Russian SMS provider;
  - a `__Host-` prefixed session cookie (ASVS 3.3.3);
  - stated idle and absolute timeouts (OWASP: 15–30 min idle for low-risk apps [4]);
  - re-authentication before a phone change (7.5.1), plus a session list and revoke-all (7.5.2).

**P11. Minor issues.**
- **`unset` is over-claimed.** The reference config comment says the DSN is "removed from the environment once read". `os.Unsetenv` doesn't touch `/proc/<pid>/environ` or the orchestrator spec, and OWASP prefers files over environment variables [5]. Use caarlos0/env's `,file` option (it exists in v11.4.2).
- **Missing API headers.** The skill promises `nosniff`, but the reference sets neither `nosniff` nor `Cache-Control: no-store` on `/v1/*`. ASVS 14.3.2 calls for no-store; one proxy cache rule could serve lawyer A's leads to lawyer B. On logout, add `Clear-Site-Data` and `queryClient.clear()` [4].
- **SSRF check timing.** "Resolve and reject private ranges" must happen in `net.Dialer.Control`, otherwise DNS rebinding gets through.

## 4. Gaps

1. **152-FZ duties that change code (now; lawyer to confirm)** [19][21][22][24]:
   - **Processor inventory:** a maintained list of processors that feeds the RKN notifications: processing (Art. 22) and a separate one for cross-border transfer (Art. 12(3)).
   - **Localization:** every store that holds personal data stays in Russia, including backups, logs, traces, analytics, the error tracker and CI artifacts (Art. 18(5), tightened 1 Jul 2025).
   - **Hosting:** use a host from RKN's hosting registry (mandatory since 1 Feb 2025).
   - **Breaches:** detection plus a runbook to notify RKN within 24 h and 72 h (Art. 21(3.1)).
   - **Data subject requests:** answered in 10 working days (Art. 20).
   - **Destruction evidence:** an act plus an event-log extract, kept 3 years (RKN Order 179).
   - **Disclosure log:** which lawyer received which lead (Art. 20(3) requires telling recipients about corrections).
   - **Special categories:** health or criminal matters in free-text questions fall under Art. 10.
   - **Protection level and threat model** under PP 1119 / FSTEC Order 21.
   - **Stakes:** leak fines of 3–15M RUB by volume; repeat leaks 1–3% of revenue up to 500M (420-FZ); criminal Art. 272.1.
2. **Cookie and analytics consent (now; contested).** Practitioners treat Metrica cookies as personal data. Build Metrica behind a consent switch so either legal outcome is a config change.
3. **Anti-automation for lead and OTP flows (now)** [3][15]:
   - per-IP limits at the edge;
   - honeypot field plus minimum fill time;
   - Yandex SmartCaptcha (invisible), verified in Go with the client IP. Tokens are single-use and valid 5 minutes. Yandex advises treating non-200 responses as "ok", so pair that fail-open with stricter limits.
   - business-flow limits in use cases, backed by PostgreSQL (OWASP API6): leads per phone per day, OTP per number and prefix, a global SMS budget, kill switches. No Redis needed.
4. **Audit trail and security events (now)** [1 V16][2 A09]:
   - an append-only table, written in the same transaction as the action;
   - events: logins, OTP, session revocation, lead viewed, contact revealed, export, role change, consent withdrawal, deletion;
   - alerts on mass views and failed logins;
   - logs kept in separate storage in Russia.
5. **Fail-closed auth in the contract (now)** [3]:
   - Declare `securitySchemes` and a global `security`; public operations say `security: []` explicitly.
   - kin-openapi refuses requests with `ErrAuthenticationServiceMissing` when no AuthenticationFunc is set, so "forgot auth" becomes impossible [18].
   - The proxy exposes only cabinet paths under `/api` (OWASP API9).
6. **Stronger factors.** Passkeys or TOTP for staff now and for lawyers later (ASVS 6.6.1, L2); go-webauthn is at v0.18.2.
7. **npm and Go dependency policy (now)** [25][26][28][35]:
   - commit `pnpm-workspace.yaml` with `minimumReleaseAge` of 3–7 days (the pnpm 11+ default is 1 day) and an `allowBuilds` allow-list (sharp); keep the `strictDepBuilds` and `blockExoticSubdeps` defaults; set `trustPolicy: no-downgrade`;
   - Renovate with release age and grouping, security updates exempt;
   - a ≤48 h patch SLA for Next, React and sharp criticals. There were four critical RCEs between Dec 2025 and Oct 2026, and self-hosters get no provider-side mitigation;
   - osv-scanner over go.mod and pnpm-lock in CI;
   - review new Go modules (the Go proxy served a typosquat for 3 years); a Russian GOPROXY mirror only with GOSUMDB left on.
   - Lessons from the TanStack compromise (May 2026): the malicious packages had valid SLSA provenance; a 7-day cooldown would have blocked them; the payload stole `~/.claude/projects/*.jsonl` and planted hooks in `.claude/settings.json`.
8. **Agent and CI hygiene (now)** [27][29][30][34]:
   - committed `.claude/settings.json` with `permissions.deny` for `.env*`, secrets and dumps;
   - CODEOWNERS on `.claude/**`, hooks and CI configuration;
   - Actions pinned by SHA, checked with zizmor;
   - `permissions: {}` by default; no `pull_request_target` with a fork checkout; no caches shared with release jobs;
   - OIDC deploys pinned to workflow and branch;
   - (In March 2026 the trivy-action tags were force-pushed to malicious commits.)
9. **Next.js self-host hardening (now)** [8][10]:
   - the proxy drops unknown `Host` headers and sets `X-Forwarded-Host` itself;
   - the Next container runs non-root with a read-only filesystem, and egress only to Go and the OTel collector;
   - restrict the image optimizer or pre-optimize images (critical sharp RCE in Oct 2026); avoid runtime `next/og` (RCE in Sep 2026);
   - `X-Accel-Buffering: no` for streaming;
   - one Next instance until a shared cache store (for example Valkey) exists.
10. **Backups and PITR (now, before the first real lead)** [31]: WAL archiving plus pgBackRest 2.60, WAL-G or Barman, or managed PostgreSQL with PITR in Russia. Backups encrypted and off-host, RPO/RTO stated, quarterly restore drills.
11. **Observability (now)** [9][32]:
   - otelhttp, otelpgx (query parameters off), otelriver;
   - trace IDs in slog via otelslog;
   - RED metrics, an SLO on lead submission and a "no leads for N minutes" alert;
   - treat traces as personal-data stores, because otelhttp records IP and user agent;
   - self-hosted GlitchTip (needs only PostgreSQL and 256–512 MB RAM), wired to Next's `onRequestError`, with PII scrubbing.
12. **Kill switches now; feature flags later.**
13. **Later:**
   - a separate cabinet origin;
   - column encryption with a blind index for phone and question (ask an infosec consultant about GOST crypto);
   - SBOM;
   - DAST against staging;
   - Semgrep for TypeScript.

## 5. Over-engineering

- Nonce CSP everywhere (P1): server rendering on every SEO hit and no CDN caching, for pages that hold no user data.
- Don't adopt self-hosted Sentry. It needs Kafka, ClickHouse and 16–32 GB of RAM [32]; GlitchTip is enough at this size.
- Don't build SLSA provenance pipelines now. TanStack showed provenance doesn't prove the code is safe; lockfile, cooldown and a build-script allow-list buy more per hour.
- Otherwise our material under-specifies far more than it over-prescribes.

## 6. Sources

1. OWASP ASVS 5.0.0 — https://asvs.owasp.org/ ; chapters https://github.com/OWASP/ASVS/tree/master/5.0/en — OWASP, 30 May 2025
2. OWASP Top 10:2025 — https://top10.owasp.org/2025 — OWASP, RC Nov 2025, final early 2026
3. OWASP API Security Top 10 2023 (still current) — https://owasp.org/API-Security/editions/2023/en/0x11-t10/
4. Session Management Cheat Sheet — https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html — OWASP
5. Secrets Management Cheat Sheet — https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html — OWASP
6. Next.js CSP guide — https://nextjs.org/docs/app/guides/content-security-policy — Vercel, 2026-03-20
7. Next.js Data Security — https://nextjs.org/docs/app/guides/data-security — Vercel, 2026-10-06
8. Next.js Self-Hosting — https://nextjs.org/docs/app/guides/self-hosting — Vercel, 2026-10-15
9. Next.js OpenTelemetry — https://nextjs.org/docs/app/guides/open-telemetry (2026-10-22) and instrumentation `onRequestError` (2026-06-09)
10. Next.js advisories (GHSA-2464-fhr4-w9xw 14 Oct 2026; GHSA-vcvr-r3jv-pc5j 22 Sep 2026; GHSA-rhh4-g77r-r9qj, GHSA-pc58-hwjq-p385 3 Nov 2026; GHSA-4342-x723-ch2f 29 Aug 2025) — https://github.com/vercel/next.js/security/advisories
11. React advisories (GHSA-xqc2-xf4c-x3h3, 14 Oct 2026) — https://github.com/facebook/react/security/advisories ; CVE-2025-55182 — https://react.dev/blog/2025/12/03/critical-security-vulnerability-in-react-server-components — React team, 3 Dec 2025
12. CVE-2025-29927 — https://nextjs.org/blog/cve-2025-29927 — Vercel, Mar 2025
13. "Strict CSP meets prerendered HTML" — https://dev.to/tonalmathew/strict-csp-meets-prerendered-html-a-nextjs-app-router-deep-dive-18b9 — one author, 2026
14. Yandex Metrica with CSP — https://yandex.ru/support/metrica/code/install-counter-csp.html ; Webvisor settings — https://yandex.ru/support/metrica/webvisor/settings.html
15. SmartCaptcha validation — https://yandex.cloud/en/docs/smartcaptcha/concepts/validation — Yandex Cloud, 2026-08-27
16. X-Forwarded-For — https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/X-Forwarded-For — MDN
17. `go doc net/http.CrossOriginProtection`, `os.Root` (Go 1.27.4); https://go.dev/doc/go1.27
18. Source code read locally: kin-openapi v0.153.0 (`openapi3/schema_error.go`, `openapi3filter/validate_request.go`), jsonschema v6.0.3 (`kind/kind.go`), otelhttp v0.72.0 (`internal/semconv/server.go`), pgx v5.13.0 (`pgconn/errors.go`), caarlos0/env v11.4.2 README
19. 152-FZ, Arts. 9, 12, 18, 20, 21, 22 (current consolidated text) — https://www.consultant.ru/document/cons_doc_LAW_61801/
20. Consent rules from 1 Sep 2025 — https://www.garant.ru/article/1862510/ — Garant (sponsored article), Sep 2025
21. 420-FZ/421-FZ fines — https://www.advgazeta.ru/ag-expert/advices/borbu-s-utechkoy-personalnykh-dannykh-usilili/ — Advokatskaya Gazeta, 18 Jun 2025
22. Localization from 1 Jul 2025 and hosting registry — https://www.comnews.ru/digital-economy/content/241730/2025-10-13/2025-w42/1016/granica-zamke-osobennosti-raboty-personalnymi-dannymi-rossii — ComNews, 13 Oct 2025
23. Fines for login via foreign services from 7 Jul 2026 — https://ppt.ru/art/internet/shtrafy-za-avtorizatsiyu-na-saytakh-cherez-inostrannye-servisy — ppt.ru, 2026
24. RKN Order No. 179 of 28.10.2022 — https://legalacts.ru/doc/prikaz-roskomnadzora-ot-28102022-n-179-ob-utverzhdenii-trebovanii/
25. pnpm — https://pnpm.io/supply-chain-security ; https://pnpm.io/blog/releases/11.0 (28 Apr 2026) ; https://pnpm.io/blog/releases/12.0 (26 Aug 2026)
26. TanStack compromise — https://snyk.io/blog/tanstack-npm-packages-compromised — Snyk, 11 May 2026
27. Trivy compromise — https://www.wiz.io/blog/trivy-compromised-teampcp-supply-chain-attack — Wiz, Mar 2026
28. Go module proxy typosquat — https://socket.dev/blog/malicious-package-exploits-go-module-proxy-caching-for-persistence — Socket, Feb 2025
29. GitHub SHA-pinning policy — https://github.blog/changelog/2025-08-15-github-actions-policy-now-supports-blocking-and-sha-pinning-actions/ — 15 Aug 2025
30. zizmor 1.32 — https://docs.zizmor.sh/ ; Betterleaks — https://www.helpnetsecurity.com/2026/03/19/betterleaks-open-source-secrets-scanner/ — 19 Mar 2026
31. PostgreSQL 18 release notes — https://www.postgresql.org/docs/release/18.0/ ; pgBackRest 2.60.0 (18 Oct 2026)
32. GlitchTip install — https://glitchtip.com/documentation/install ; Sentry self-hosted — https://develop.sentry.dev/self-hosted/
33. Kubernetes probes — https://kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/
34. Claude Code settings (`permissions.deny`) — https://code.claude.com/docs/en/settings — Anthropic, 2026
35. Go module mirrors in Russia — GitVerse (ComNews, 16 Jun 2026) https://www.comnews.ru/content/245852/2026-06-16/2026-w25/1018/gitverse-obespechivaet-stabilnyy-dostup-k-paketnym-khranilischam-populyarnykh-yazykov-programmirovaniya ; Yandex Cloud Registry https://yandex.cloud/en/docs/cloud-registry/operations/go/pull

Downloads went only to /tmp/claude-0/-root-lead-new--localenv/6b41f19b-ec42-4595-8dc3-68f60461b2ed/scratchpad/research-security-ops/; nothing in the plugin was modified.
