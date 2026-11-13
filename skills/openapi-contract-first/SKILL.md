---
name: openapi-contract-first
description: Contract-first HTTP API between the Go backend and the Next.js frontend — designing operations and schemas in api/openapi.yaml (the OpenAPI 3.1 subset the generators handle), security in the contract, the error model (RFC 9457 problems with a type per code, errors[] with JSON Pointers, x-problem-codes per operation), headers (Location, Retry-After, ETag), pagination, Idempotency-Key, compatibility and deploy order, contract lint (vacuum) and the breaking-change gate (oasdiff), generating the Go strict server with oapi-codegen and the TypeScript client with openapi-typescript, request validation and contract tests. Use whenever an endpoint, request/response shape, header or error code is added or changed.
---

# OpenAPI contract first

`api/openapi.yaml` is the source of truth. Go handlers and TypeScript types are generated from
it; nobody hand-writes a type that mirrors the API. Working example:
`${CLAUDE_PLUGIN_ROOT}/skills/go-clean-architecture/reference/` (`api/openapi.yaml`, `api/vacuum.yaml`, the generator
config, `internal/api`, handlers and their contract tests).

## Workflow

1. **Design** the change in `api/openapi.yaml` (rules below), in the plan first when it is
   non-trivial. `make api-lint` (vacuum, `api/vacuum.yaml`) must pass.
2. **Generate** both sides, in the same change:
   - Go: `cd backend && go generate ./internal/gen/openapi/` → `openapi.gen.go`
   - Web: `cd web && pnpm generate:api` → `src/lib/api/schema.ts`
3. **Compile**: the Go build fails until `StrictServerInterface` is implemented; `tsc` fails where
   the web uses a changed shape. These errors are the to-do list. Because of the Go side, a
   contract change merges together with its handler.
4. **Implement** the handler in `delivery/httpapi`; test it through `apitest.Serve` (every
   response is checked against the contract and its problem codes).
5. **CI**: `make generate-check` (no drift between contract, server and client), `make api-lint`,
   and `make api-breaking` (oasdiff against `main`): a breaking change fails the build.

Never edit generated files; the `protect-generated` hook denies it.

## Designing operations

- Paths: plural nouns, kebab-case, versioned prefix: `/v1/leads/{leadId}`; `/v1/leads/{leadId}/assign`
  for a state transition that isn't a field update. Path params named `<thing>Id`.
- `operationId`: camelCase verb + noun (`createLead`, `listLeads`, `assignLead`). It becomes the Go
  method name; renaming it breaks the code.
- `tags`: one per backend module (`leads`, `lawyers`), declared in the top-level `tags`.
- Methods: GET reads (safe, cacheable), POST creates or triggers, PATCH partial update with
  explicit fields, PUT full replace (rare), DELETE removes. Never change state on GET.
- **Security in the contract**: `components.securitySchemes.session` (the `__Host-session`
  cookie) and a global `security: [{session: []}]`; a public operation says `security: []`. The
  request validator rejects a secured request without the session with 401 `auth.required`
  before any handler runs, so a forgotten check can't expose an operation. Who may do what is
  still decided in the use case.
- Personal data never goes in paths, query strings or header parameters (they end up in logs,
  history, analytics): search by phone is `POST /v1/leads/search` with a body. vacuum rejects
  such parameter names.

## Schemas

- `components/schemas` for everything reused; nouns (`Lead`, `CreateLeadRequest`), each with a
  `description`; properties camelCase.
- Request bodies: `additionalProperties: false`, explicit `required`, bounds on every string
  (`maxLength`, `pattern`, `enum`, `const` or `format`: vacuum checks it) and number
  (`minimum`/`maximum`), `maxItems` on arrays. The validator enforces them before your code runs.
- Responses: list `required` honestly; optional fields are absent, not `null`, unless `null`
  means something (then `type: [string, "null"]`).
- Formats: `uuid` for IDs, `date-time` (RFC 3339, UTC) for instants, `date` for calendar dates.
  Money: `{ "amount": 150000, "currency": "RUB" }` with integer minor units, never floats.
- Enums: lowercase snake_case strings. Clients must treat an unknown value like a documented
  default (say which in the description); the web does.
- Don't expose internal structure: no database IDs of other modules, no internal statuses the
  client can't act on, no personal data the caller isn't entitled to (`Lead` has no phone).
- **The 3.1 subset**: oapi-codegen's 3.1 support is initial (since v2.8.0), so the contract uses
  only what both generators handle. vacuum rejects `prefixItems`, `if`/`then`/`else`,
  `dependentSchemas`/`dependentRequired`, and `additionalProperties: false` inside an `allOf`
  member (in JSON Schema 2020-12 it rejects the sibling schemas' properties; close the composed
  schema with `unevaluatedProperties: false` instead).

## Errors (RFC 9457)

Every operation has `default: { $ref: "#/components/responses/Problem" }` plus the statuses a
client should handle, and lists its codes by status in `x-problem-codes`:

```yaml
x-problem-codes:
  "400": [request.invalid]
  "413": [request.too_large]
  "415": [request.unsupported_media_type]
  "422": [lead.invalid_phone, lead.invalid_topic, lead.invalid_city, idempotency.key_reused]
  "429": [lead.rate_limited]
  "500": [internal]
  "503": [unavailable, timeout]
```

```json
{ "type": "https://example.com/problems/request.invalid", "title": "the request does not match the API contract",
  "status": 400, "code": "request.invalid", "requestId": "0193…",
  "errors": [{ "pointer": "#/phone", "code": "pattern" }, { "parameter": "Idempotency-Key", "code": "required" }] }
```

- `type` is a URI per code (`https://example.com/problems/<code>`; RFC 9457 §3.1.1: `about:blank`
  means "nothing beyond the status"), `title` is fixed per type, `code` is stable and dotted
  (`<module>.<reason>`); clients branch on `code` (or `type`), never on `title` or `detail`.
- `errors[]` (400 only) names where and which rule: `pointer` (RFC 6901, as a URI fragment; `#`
  is the body as a whole) or `parameter`, and `code` (`required`, `pattern`, `maxLength`,
  `additionalProperties`, `format`, `malformed`). **Never the submitted values**: kin-openapi's
  messages contain them (and the schema), so they are never copied into a response or a log.
- `detail` is safe to show and never contains submitted values; `requestId` matches the logs.
- The backend writes problems in one place: `platform/httpserver.ErrorWriter` from `apperr` kinds
  (handlers just return errors) and `internal/api` for requests that don't match the contract.
  Statuses: 400 `request.invalid`, 401 `auth.required`, 404 `request.not_found` (no such
  operation) or a domain not-found, 405 `request.method_not_allowed` (with `Allow`), 413
  `request.too_large`, 415 `request.unsupported_media_type`, 422 domain validation, 409 domain
  conflicts, 429 limits, 500 `internal`, 503 `unavailable` and `timeout` (the server adds
  `Retry-After`).
- A new code under an existing status is compatible: the list in `x-problem-codes` can grow, and
  clients treat an unknown code like its status. A 502 or 504 from a proxy may not be a problem
  document at all: clients parse defensively.
- `apitest.CheckContract` fails a test when a problem's code isn't listed for its status.

## Headers

- `201` with `Location` (the new resource's path), declared and `required` in the contract; the
  strict server writes only declared headers.
- `429` on every public operation, with `Retry-After` (seconds): the response `RetryableProblem`
  declares it. `503` (`unavailable`, `timeout`) arrives through `default`; the server sends
  `Retry-After` with it too, so clients may read it when present.
- Optimistic concurrency where two users edit one resource: `ETag` on GET, `If-Match` on PATCH,
  `412` on mismatch.

## Lists

- Cursor pagination for application lists: `GET /v1/leads?limit=50&cursor=<opaque>` →
  `{ "items": [...], "nextCursor": "..." }` (absent on the last page). `limit` has a `maximum`.
- Public SEO listings (numbered pages that must be linkable and crawlable) and sitemap chunks use
  page numbers: `GET /v1/seo/pages?page=3&pageSize=50` → `{ "items": [...], "totalPages": 12 }`,
  backed by keyset queries or precomputed ranges, never `OFFSET` on large tables.

## Idempotency-Key

Operations a client may retry and that must not run twice (public lead forms, partner
integrations, money movements) require an `Idempotency-Key` header, per
draft-ietf-httpapi-idempotency-key-header-08:
- The value is a Structured Field String, so it is quoted: `Idempotency-Key: "6f1c0e5a-…"`;
  the contract's pattern is `^"[A-Za-z0-9_-]{16,64}"$`. The handler strips the quotes.
- The client makes a new random key per submission (a form: in the browser after hydration, never
  during a static render) and keeps it for that submission's retries.
- Same key and same request within 24 hours → the first response again (201 with the same body);
  same key with a different request → 422 `idempotency.key_reused`; a concurrent request with the
  key waits for the first one. A missing key is 400 `request.invalid`.
- Storage, the fingerprint and the purge: `postgres-patterns`, "Idempotency-Key storage".

## Compatibility and deploy order

Non-breaking: adding an operation, an optional request field, a response field, a new error code
under an existing status. Breaking: removing or renaming anything, making a request field or
header required, narrowing types or enums, changing a status code or `operationId`, changing the
meaning of a field, adding security to a public operation. `make api-breaking` (oasdiff, ERR level)
fails on these; it doesn't judge meaning, so a review still does. Breaking changes need the
user's yes, then a new path version or expand → migrate → contract: add the new field, move
clients, remove the old one in a later release.

Deploy order: request bodies are closed (`additionalProperties: false`), so a new request field
ships in the backend first and the web sends it only after that deploy. A new response field the
web needs: backend first as well. Removing something: the web stops using it first.

## Generation setup (verified with oapi-codegen v2.8.2)

`backend/internal/gen/openapi/oapi-codegen.yaml`:

```yaml
package: openapi
output: openapi.gen.go
generate:
  models: true
  std-http-server: true   # net/http ServeMux with Go 1.22 patterns; no router dependency
  strict-server: true     # typed request/response objects; handlers return (Response, error)
  embedded-spec: true     # GetSpec() for the request validator and contract tests
compatibility:
  always-prefix-enum-values: true          # LeadStatusNew, not New
  apply-chi-middleware-first-to-last: true # per-operation middlewares run in the listed order
output-options:
  type-mapping:            # format: uuid → stdlib uuid.UUID (Go 1.27), not google/uuid
    string:
      formats:
        uuid: { type: uuid.UUID, import: uuid }
```

`generate.go` in the same directory:
`//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../../api/openapi.yaml`
with the tool pinned by `go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2`.

## Request validation and wiring

`internal/api.New(si, log)` (reference `internal/api/api.go`) is the `/v1/` handler:
- the request validator (`nethttp-middleware` v1.3.1) covers the whole `/v1/` subtree, with
  `DoNotValidateServers`, `MultiError: true` (every field error at once) and an
  `AuthenticationFunc` for the session scheme;
- its errors become problems that never echo input: 404 and 405 for unknown routes and methods,
  413 for a body over the limit, 415 for a media type the operation doesn't accept, 401 without
  the session, otherwise 400 `request.invalid` with `errors[]`;
- the strict handler's request errors (a path parameter that isn't a UUID, malformed JSON) are 400
  with `errors[]` too; its response errors go to `httpserver.ErrorWriter`;
- `si` embeds every module's handler: `struct{ *leadshttp.Handler; *lawyershttp.Handler }`
  (`cmd/api/wire.go`).
`cmd/api` mounts it behind the middleware chain (golang-patterns skill, "HTTP server").

## Web client (openapi-typescript + openapi-fetch)

The client stack stays as is for now (${CLAUDE_PLUGIN_ROOT}/STACK.md: openapi-typescript, openapi-fetch,
openapi-react-query; in maintenance mode since 2026-09-19, the replacement is an open decision).
- `web/src/lib/api/schema.ts`: generated, never edited.
- `web/src/lib/api/server.ts`: `createClient<paths>({ baseUrl: <internal Go URL> })` for Server
  Components and Server Actions; imports `server-only`; sends a deadline and a request ID; forwards
  only the session cookie, taken from the raw `Cookie` header (`(await headers()).get('cookie')`),
  never `(await cookies()).toString()`, which re-encodes values.
- `web/src/lib/api/browser.ts`: same-origin client with the `/api` prefix for the cabinets; the
  reverse proxy forwards `/api/*` to Go, so the HttpOnly cookie and `http.CrossOriginProtection`
  work without CORS.
- Error bodies are read with `asProblem()` (`lib/api/problem.ts`, `nextjs-patterns`): a proxy's
  502 or 504 isn't a problem document, so nothing assumes the shape.

```ts
const { data, error } = await api.GET("/v1/leads/{leadId}", { params: { path: { leadId } } });
// error is the typed Problem: branch on error.code, never on detail
```

Details and caching: the `nextjs-patterns` skill.

## Review checklist

`make api-lint` already enforces: operationId and tags, a `default` problem and `x-problem-codes`
on every operation, security by default, bounded request strings, camelCase properties, no
personal data in parameter names, the 3.1 subset. `make api-breaking` catches structural breaking
changes. The reviewer checks what tools can't:
- [ ] The operation and schemas mean what the use case does; names follow the conventions.
- [ ] Every code the handler can return is in `x-problem-codes` (the contract tests cover the
      paths they exercise; read the use case for the others).
- [ ] Responses expose only what the caller may see; authorization happens in the use case.
- [ ] Retryable creates have `Idempotency-Key`; public operations declare 429.
- [ ] Compatibility of meaning (a field whose meaning changed is breaking even if oasdiff is
      quiet); the deploy order is in the plan.
- [ ] Go and TS regenerated in the same change; the handler tests go through `apitest.Serve`.
