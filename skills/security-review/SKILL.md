---
name: security-review
description: Secure-by-default patterns and checklists for the Go API and the Next.js frontend — authorization in use cases, login through Russian services only (email, Yandex ID, VK ID, phone) and sessions, CSRF, anti-spam for public forms (proxy limits, honeypot, SmartCaptcha verified in Go, business limits, kill switches, one ClientIP), input limits, SQL, SSRF, webhooks, secrets, personal data, money operations, headers and dependency scanning. Use when adding auth, endpoints, forms, uploads, payments, integrations or anything that handles personal data.
---

# Security review

The `security-reviewer` agent applies this skill to a diff. When writing code, use the patterns
below directly; they are cheaper than findings. Working code for several of them:
`${CLAUDE_PLUGIN_ROOT}/skills/go-clean-architecture/reference/backend/` (validator, problems, `ClientIP`, limits, masking).

## Authorization lives in use cases

```go
// usecase: the principal comes from the context (set by the auth middleware), the rule is here.
func (uc *GetLead) Handle(ctx context.Context, p auth.Principal, id uuid.UUID) (LeadView, error) {
	lead, err := uc.leads.Get(ctx, id)
	if err != nil {
		return LeadView{}, fmt.Errorf("get lead %s: %w", id, err)
	}
	if !lead.VisibleTo(p.LawyerID) { // entity rule: assigned lawyer or staff
		return LeadView{}, ErrLeadNotFound // 404, not 403: don't confirm the ID exists
	}
	return viewOf(lead), nil
}
```

- Every use case that reads or changes a resource checks the caller against *that* resource.
  The contract's security (the session cookie) authenticates; use cases authorize. UI hiding and
  Next.js `proxy.ts` are conveniences, not controls.
- Filter in the query for lists (`WHERE id = $1 AND assigned_lawyer_id = $2`), so a missing check
  can't leak rows.
- Test the negative: another user's ID returns 404; a staff-only operation without the role fails.

## Login and sessions

Users log in only through Russian services: **email, Yandex ID, VK ID, or a phone number**
(`${CLAUDE_PLUGIN_ROOT}/STACK.md`, "Login"). Decided (2026-11-13): email by a one-time code or link, no passwords;
clients, lawyers and staff all log in, each into their own cabinet (`/account`, `/cabinet`,
`/admin`); accounts that share a verified email are merged automatically; sessions last 7 days
idle and 30 days absolute. Email alone is accepted as a login method (the 149-FZ question is
closed). The identity module's ADR records these with the details below.

- **Yandex ID, VK ID**: OAuth 2.0 authorization code flow with PKCE, `state` checked on return,
  `nonce` where the provider issues ID tokens; the code is exchanged and the user info fetched by
  Go, server side; the provider's stable user ID is the key, an email is data. Never trust
  profile data sent by the browser.
- **Phone**: a code by SMS through a Russian provider: 6+ digits from `crypto/rand`, valid ≤ 5
  minutes, single use, an attempt limit per code and per phone, compared with
  `subtle.ConstantTimeCompare`; sends limited per number, per IP and by the daily SMS budget
  (anti-spam below).
- **Email**: a one-time code or link has the same rules as the SMS code; a link's token is
  single use, short-lived, consumed with a conditional update, and never logged.
- **Session**: an opaque random ID (`crypto/rand`), stored hashed, in the cookie
  `__Host-session` (`Secure; Path=/`, no `Domain`; `HttpOnly; SameSite=Lax`). The prefix makes the
  browser refuse it from a subdomain or over HTTP. Expiry: 7 days without activity, 30 days in
  any case; a new ID on login and on privilege change; revoked on logout.
- **Sensitive changes** (phone, email, linked providers) require logging in again just before
  the change, and revoke the user's other sessions. Users see their active sessions (device,
  time, rough location) and can end any of them.
- **Roles**: client, lawyer, staff. The session gives the use case a principal with the user ID
  and role; each use case checks both (a client sees only their own leads, a lawyer only the leads
  assigned to them, staff only what their role allows). A cabinet's URL prefix is routing, not
  authorization.
- **Automatic merge by email** (the chosen option, so it needs these safeguards): merge only on an
  email the provider marks as verified *and* that the existing account verified with our own code
  or link; never on an unverified or provider-unconfirmed email, never on a phone number from a
  provider profile. On every merge or new linked provider: email the account's address, log the
  event (user ID, provider, time; no email in the log), and let the user unlink a provider from
  the cabinet, which ends that provider's sessions. Staff accounts are never merged
  automatically: a staff role can't be reached by linking a provider.
- Login and recovery responses never reveal whether an account, a phone or an email exists.
- Never put tokens in `localStorage` or in URLs.
- **CSRF**: cookie-authenticated mutations go through `http.NewCrossOriginProtection().Handler(h)`
  (Go 1.25+, `Sec-Fetch-Site`/`Origin`). Next.js Server Actions keep their built-in origin check;
  the proxy passes the public host, `serverActions.allowedOrigins` only for real extra origins.

## Anti-spam for public forms

Layers, cheapest first; each one is required:
1. **The proxy** (nginx) limits requests per IP on the lead and code-sending endpoints
   (`limit_req`), and passes the client address in `X-Forwarded-For`
   (`$proxy_add_x_forwarded_for`).
2. **The form**: a honeypot field and a minimum fill time, checked in the Server Action, never
   sent to Go (`nextjs-patterns` skill). A bot that fills every field or submits in 300 ms gets the
   same "thanks" page as a person and nothing is created.
3. **Yandex SmartCaptcha** (built with the first real form): the form sends the widget's token; Go verifies it server side with
   the server key and the client IP through a `usecase` port (`HumanCheck`) implemented in
   `gateway/smartcaptcha`, with a short timeout, before the transaction. A retry of a request
   whose `Idempotency-Key` is already claimed returns the first response before the captcha is
   checked: the retry carries the same, already used token. Check the current validation
   endpoint and parameters in SmartCaptcha's documentation. Decided (2026-11-13):
   - a lead form submitted without JavaScript has no token: Go accepts it under stricter limits
     (per phone and per IP) and marks the lead unverified;
   - the provider is down: sending codes fails closed (an SMS costs money); lead forms fail open
     with every limit below still in force; both log a warning.
4. **Business limits in PostgreSQL**, in the use case: leads per phone per day (reference:
   `usecase.MaxLeadsPerPhone`, 429 `lead.rate_limited` with `Retry-After`), codes per number per
   hour and per day, a total SMS budget per day. Approximate under concurrency is acceptable for
   leads; the SMS budget is decremented with a conditional `UPDATE … WHERE remaining > 0`.
5. **Kill switches**: stopping lead intake or SMS sending (per channel) without a deploy: a row
   per switch in the module's settings, read with a short cache, changed only by staff, and every
   change logged with who and when. A stopped intake answers 503 with `Retry-After`.

**One client IP**: `httpserver.ClientIP` is the only code that reads `X-Forwarded-For`. It trusts
the header only when the direct peer is one of the configured proxy networks and returns the
rightmost address that isn't a proxy; anything else could be forged by the client. A request
from a Server Action reaches Go through Next.js (browser → nginx → Next.js → Go): Next.js passes
on the `X-Forwarded-For` it received from nginx unchanged, and Go's trusted networks include the
Next.js servers, so `ClientIP` finds the same client. Nothing else interprets `X-Forwarded-For`,
`X-Real-IP` or `Forwarded`, in Go or in Next.js.

## Input

- Bodies capped (`httpserver.LimitBody`, 1 MiB → 413), JSON shape enforced by the OpenAPI
  validator (`additionalProperties: false`, bounds on every field), invariants by entity
  constructors. Validation problems name the field and the rule, never the submitted value.
- SQL only through sqlc parameters. Dynamic `ORDER BY`/filters map from an allow-list of enum
  values to fixed SQL fragments; never interpolate input.
- Files: limit size before reading, detect type from content (`http.DetectContentType`), store
  under generated names outside any served directory, serve with `Content-Disposition: attachment`
  unless it's a known-safe image. Paths from input: `os.OpenRoot(dir)` + `root.Open(name)`.
- `os/exec`: fixed binary, input only as separate arguments, never through a shell.
- Web: React escapes by default; `dangerouslySetInnerHTML` only with sanitized HTML. JSON-LD:
  `JSON.stringify(data).replace(/</g, "\\u003c")` inside the script tag. Redirect targets from
  input are checked against an allow-list of internal paths.

## Outbound calls and webhooks

- `http.Client{Timeout: …}` with explicit transport timeouts; per-call `context.WithTimeout`;
  never inside a database transaction.
- Never fetch a URL supplied by a user. If a feature requires it: allow-list hosts, resolve and
  reject private/loopback/link-local addresses, disable redirects or re-check each hop.
- Incoming webhooks (payments, telephony): verify the HMAC signature over the raw body with
  `hmac.Equal`, check the timestamp window, process idempotently by the provider's event ID,
  respond 2xx only after the event is durably stored (a River job in the same transaction).

## Money

- Integer minor units in a named type; never floats. Every movement is a double-entry ledger
  posting in one transaction; balances derive from postings or are updated with a conditional
  `UPDATE … WHERE balance >= $amount` and checked rows-affected.
- Idempotency keys on every money-changing request and job; a unique constraint enforces it.
- Daily reconciliation against the provider; differences alert, never auto-correct.

## Personal data

- Collect the minimum; know where each field lives (module, table, column; the module's
  `CLAUDE.md` lists them) and who may read it.
- Store and process personal data of Russian users in Russia; no foreign SaaS (analytics, error
  trackers, LLM APIs, CDNs with logs) receives it without a legal basis and consent.
- Keep it out of logs, traces, metric labels, error messages, URLs, analytics events, job args,
  event payloads and third-party scripts. Log IDs instead. Mechanically: types that hold it mask
  themselves (`entity.Phone`), sloglint rejects keys like `phone` and `email`, vacuum rejects such
  parameter names, events and job args carry IDs only.
- Derived values count too: a hash of a phone number is reversible by trying every number
  (`idempotency_keys.fingerprint` is deleted after 24 hours).
- Mask on output where full values aren't needed (`+7******4567`); return contact details only to
  the party entitled to them (the assigned lawyer, after payment rules allow).
- Retention: delete or anonymize on schedule and on request; backups age out too.
- Development and tests use generated data only.

## Secrets and configuration

- Secrets come from the environment or a secret store, parsed in `cmd` only, as `config.Secret`
  (prints `[redacted]` in fmt and slog; `Reveal()` where the value is used). The `unset` tag
  removes the variable from the process environment once read, so child processes don't inherit
  it; the process's own `/proc/<pid>/environ` still shows it.
- A DSN parse error is never wrapped into a returned error: it may contain the password.
- Web: only `NEXT_PUBLIC_*` values reach the browser; never put a secret there. Server-only
  modules import `server-only` so a client import fails the build.
- `.env*` (except `.env.example`) ignored by git; rotate anything that was ever committed.

## Headers and transport (web)

`Strict-Transport-Security`, `X-Content-Type-Options: nosniff`,
`Referrer-Policy: strict-origin-when-cross-origin`, `Permissions-Policy`,
`frame-ancestors 'none'` (or `X-Frame-Options: DENY`). The Go API answers JSON only, every
response with `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`
(`httpserver.APIHeaders`): answers depend on the session, and one careless proxy cache rule would
serve lawyer A's leads to lawyer B.

`Content-Security-Policy`: a nonce-based policy needs every page rendered per request (Next.js:
"all pages must be dynamically rendered", incompatible with Partial Prerendering), so it can't
cover the static SEO pages. Don't turn static pages dynamic for it: which policy the public
pages and the cabinets get is an open decision (${CLAUDE_PLUGIN_ROOT}/STACK.md).

## Availability

Server timeouts (`ReadHeaderTimeout` and the rest), a deadline on every request
(`httpserver.Timeout`), body limits, the anti-spam layers above on public endpoints, pagination
limits with a maximum, `statement_timeout` and `transaction_timeout` in PostgreSQL, readiness that
fails while draining.

## Scanners (run in CI and in the review)

```bash
cd backend && go tool govulncheck ./...              # reachable vulnerable code
cd backend && bin/golangci-lint run ./...            # gosec, depguard, sloglint in the config
make api-lint                                        # the contract's security rules
cd web && pnpm audit --prod                          # npm advisories
```

Treat a reachable govulncheck finding as HIGH until triaged; upgrade rather than suppress.
