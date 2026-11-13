<!-- Independent reviewer's report, 2026-11-13: HTTP API contract: OpenAPI 3.1 contract-first.
     Raw model output, kept for its sources and detail. Which claims were re-verified is stated in
     ../2026-11-13-best-practices-review.md (sections marked [проверено] / [опровергнуто]). -->

## 1. Verdict

Contract-first OpenAPI 3.1 with oapi-codegen's std-http and strict server is still the best fit for this product: server-side fetches for SEO pages, a cookie-authenticated cabinet, a future partner REST API, and a small team plus agents all working from one contract. The Go half is current: every pinned Go module was the latest on proxy.golang.org when checked on 2026-11-13, and the wiring follows the upstream docs. Three things fall short of current practice. First, openapi-typescript, openapi-fetch and openapi-react-query were put into maintenance mode on 2026-09-19, and the maintainer tells users to migrate. Second, the error model: `about:blank` plus `code` contradicts RFC 9457, there are no field-level errors, and with a 3.1 spec the validator's `err.Error()` copies request values (phone numbers included) into `detail`. Third, governance depends on judgment: no spec linter, no breaking-change diff, no mocks, and the handler tests don't run the validator that production runs.

Assumptions: I ran no generators (read-only task), so the TypeScript tool comparison comes from docs and registries. I read the nextjs-patterns form example as targeting the reference contract.

## 2. Confirmed

- **API-first:** both sides generated in the same change, and CI fails on any generation diff. This matches Zalando's "MUST follow API first principle" (2026).
- **The alternatives don't beat it here:**
  - Huma v2.40 generates OpenAPI 3.1 from Go code, so the spec becomes an output of the backend.
  - TypeSpec (1.0 GA, May 2025) is better for authoring at scale but adds a toolchain; worth it later.
  - ConnectRPC (CNCF sandbox, `buf breaking`, connect-query) has the best codegen, but RPC-style paths, POST by default and no problem+json or ETag semantics fit partner REST worse.
  - GraphQL buys nothing here.
- **3.1, not 3.2:** OpenAPI 3.2.0 shipped 2025-09-19, but kin-openapi supports it only partially and oapi-codegen v2 not at all.
- **Go versions are the latest:** oapi-codegen v2.8.2, runtime v1.9.0, nethttp-middleware v1.3.1, kin-openapi v0.153.0.
  - nethttp-middleware v1.3.1 keeps the validated body, so the strict handler binds exactly what was validated (this closes the case-insensitive double-decode bypass).
  - `LimitBody` sits before the validator, as the middleware's README requires.
- **ogen as a fallback only:** reasonable. It is active (v1.24.0, 2026-08-07) but has a smaller community.
- **Error mapping:** handlers return errors and one central mapper turns them into statuses, with internals hidden behind a 500 (RFC 9457 §4). The typed 4xx responses the strict server generates go unused, which is an acceptable trade-off.
- **Clients branch on a stable code, never on `detail`:** RFC 9457 §3.1.4; Google AIP-193 (reason + domain).
- **Pagination:** opaque cursor, bounded `limit`, `nextCursor` left out on the last page. Matches AIP-158 and Microsoft (nextLink never null).
- **No personal data in URLs; search uses POST with a body:** RFC 9110 §17.9.
- **Bounded fields, unknown fields rejected at runtime:** Microsoft ("fail … 400 if any JSON field name … not fully understood"); Zalando (servers "should reject … undefined fields").
- **Absent vs `null`:** `null` only when it means something. Matches Zalando; oapi-codegen 2.8 maps `type: [T, "null"]`.
- **Dates and money:** RFC 3339 instants in UTC; integer minor units plus ISO 4217 currency. Zalando uses a decimal amount and Google units + nanos, so this is a choice, not an error.
- **Enums:** clients must tolerate unknown values. Matches Microsoft "extensible enums" and Zalando.
- **Compatibility:** the breaking-change list and expand → migrate → contract match AIP-180.
- **`/v1` in the path:** matches AIP-185. Zalando and Microsoft version differently; there is no consensus.
- **Response validation in handler tests:** right place, since the oapi-codegen README says middleware can't validate responses.

## 3. Problems

**P1. The TypeScript client stack is in maintenance mode.** STACK §3 "API client" and §4; skill "Web client"; nextjs-patterns.
- **What we say:** pin openapi-typescript 7.13.0, openapi-fetch 0.17.0 and openapi-react-query 0.5.4.
- **Evidence:** pinned issue #2874 (maintainer, 2026-09-19) says the project is "in maintenance mode and won't be getting any more updates". There will be no security patches, "Please migrate to another project", and no handover.
  - All three packages ship from that repo; the last release was 2026-02-11.
  - The peer dependency is `typescript ^5.x`, and TypeScript 7 is unsupported (#2841).
  - Downloads still lead (about 55M/month) through inertia; @hey-api/openapi-ts is about 26M, orval about 13M, kubb about 2M.
- **Severity:** high. **Confidence:** high.
- **Change:** record this in STACK, then run a short spike on the reference contract comparing two replacements:
  - **@hey-api/openapi-ts:** fetch and Next.js clients, TanStack Query `queryOptions`/`mutationOptions`, a zod plugin. Self-reported users include Vercel, PayPal and AWS. It is still 0.x: the last stable release is 0.99.0 (2026-06-22), with only 1.0 prereleases since, so pin an exact version.
  - **orval 8.47:** semver-stable, weekly releases, fetch client, TanStack Query, zod and MSW mocks. Needs Node 22.18 or later.
  - Both name SDK functions after `operationId`, so it becomes part of the TypeScript API too.
  - Until then, the current packages keep working on TypeScript 6.0.

**P2. Validator errors echo request data.** `cmd/api/main.go` (`ErrorHandlerWithOpts`, `Detail: err.Error()`); skill "Errors".
- **Evidence:** for 3.1 documents, `openapi3filter` turns on the JSON Schema 2020-12 validator, and its messages contain values.
  - A pattern failure reads `'<value>' does not match pattern '<regex>'` (jsonschema v6).
  - `SchemaError.Error()` also appends `Schema:` and `Value:` dumps.
  - `DisableSchemaErrorDetails` now fails with `ErrSchemaErrorDetailsWithJSONSchema2020`.
  - `ErrorHandlerWithOpts` receives the full multi-line error.
- **Effect:** a malformed phone number, or any root-level failure such as an extra field (where Value is the whole body), is reflected into `detail` together with our regexes. That goes against RFC 9457 §3.1.4 and §4 and against the project's 152-FZ rule on personal data.
- **Severity:** high. **Confidence:** high (verified in the module source).
- **Change:**
  - Never serialize validator errors.
  - Map `MultiError` and `*SchemaError` to `errors: [{pointer: "#/phone", code: "pattern"}]` using `JSONPointer()` and `SchemaField`. Don't use `Reason`, which carries values in 2020-12 mode.
  - Set `Options.MultiError: true` so a form gets every field error at once.
  - Add a test that the response never contains the submitted value.
  - Apply the same fix to `BadRequestWriter`.

**P3. `about:blank` plus `code` contradicts RFC 9457.** Skill "Errors"; `problem.go`; the Problem schema.
- **Evidence:** RFC 9457 §3.1.1 says "Consumers MUST use the 'type' URI … as the problem type's primary identifier". §4.2.1 says `about:blank` means "no additional semantics beyond that of the HTTP status code". Zalando uses type URIs like `/problems/out-of-stock`.
- **Severity:** medium. **Confidence:** high.
- **Change:**
  - Give each code its own `type` URI (absolute, as the RFC recommends, or Zalando-style relative paths) with a fixed `title` per type; keep `code` as an alias.
  - Declare `type` with `format: uri-reference` and add `instance` or a trace id.
  - Tell clients that a non-2xx body may not be a problem document (a proxy 502, a ServeMux 404).
  - This is cheap now and a breaking change later.

**P4. Validator placement and status mapping.** `main.go` (`StdHTTPServerOptions.Middlewares`); the skill says undecodable input gets `request.malformed`.
- **Evidence:**
  - Generated wrappers bind path and query parameters before per-operation middlewares run. So bad parameters get `request.malformed`, while malformed JSON is caught first by the validator and gets `request.invalid`. That is the reverse of what the skill says.
  - Unknown `/v1` routes and 405 responses come from ServeMux as `text/plain`, not problem+json.
  - A body over 1 MiB becomes kin-openapi's "reading failed", so 400 instead of 413. A wrong Content-Type also gives 400 instead of 415.
  - std-http middlewares run last-to-first unless `compatibility.apply-chi-middleware-first-to-last` is set (the std-http template honours it). This is a trap once auth or logging middlewares are added.
- **Severity:** medium. **Confidence:** high.
- **Change:**
  - Wrap the whole subtree: `mux.Handle("/v1/", validate(routes))`, as the nethttp-middleware README wraps the router.
  - Map not-found to 404, `routers.ErrMethodNotAllowed` to 405, `*http.MaxBytesError` to 413 and media-type errors to 415, all as problem+json.
  - Set the ordering flag.
  - Use `DoNotValidateServers: true` instead of setting `spec.Servers = nil`.

**P5. Handler tests don't run the production stack.** `handler_test.go` (`serve`); golang-testing ("the same stack cmd/api builds").
- **Evidence:** the tests leave out the validator. As a result:
  - bounds, patterns and closed request bodies are never tested;
  - error codes differ between tests and production;
  - `BindJSONBody` falls back to case-insensitive decoding in tests.
  - Separately, golang-testing calls `GetSwagger()`, which the generated code marks deprecated.
- **Severity:** medium. **Confidence:** high.
- **Change:** build the `/v1` handler in one exported constructor used by both `main` and the tests, and add negative tests for each request schema.

**P6. Hand-written zod schemas drift from the contract.** nextjs-patterns "Server Actions and forms" vs the reference `CreateLeadRequest`.
- **Evidence:**
  - The example sends `name`, which the contract doesn't have. With `additionalProperties: false`, every submission would get a 400.
  - Its phone regex accepts `8 (999) 123-45-67`, which the contract's `^\+7[0-9]{10}$` rejects.
  - The city and topic bounds differ.
  - `tsc` doesn't catch any of this: excess-property checks apply only to object literals, and `parsed.data` is a variable.
- **Severity:** medium. **Confidence:** medium (the example may be meant as illustration only).
- **Change:**
  - Build request bodies as object literals from the parsed fields, after normalizing the phone number.
  - Generate zod from the contract (the P1 replacements can) and refine it for UI messages.
  - Add a test that sends the form's output through the validator.

**P7. Closed request bodies vs "adding an optional request field is non-breaking".** Skill "Schemas" and "Compatibility".
- **Evidence:**
  - This only holds if the backend deploys first; an older backend rejects the new field.
  - In JSON Schema 2020-12, `additionalProperties: false` ignores properties declared in `allOf` siblings, so composed schemas reject valid input.
  - Zalando: "API formats must not declare additionalProperties to be false", although servers should still reject undefined fields. Microsoft rejects unknown fields.
- **Severity:** low to medium. **Confidence:** medium (this is contested).
- **Change:**
  - Keep rejecting unknown fields.
  - Add a deploy-order rule: backend first. Cabinet browsers also keep running old bundles for a while.
  - Ban `additionalProperties: false` on schemas used in `allOf`; use `unevaluatedProperties: false` on the composed schema, and check how the generators handle it.

**P8. Idempotency is underspecified and not applied to leads.** Skill "Lists, retries, concurrency".
- **Evidence:** draft-ietf-httpapi-idempotency-key-header-08 (2026-10-22) is an active working-group draft, not an RFC. It specifies:
  - the value is a Structured Field String;
  - the resource publishes its expiry policy, and may use a payload fingerprint;
  - 400 when the key is required but missing;
  - 422 when a key is reused with a different payload;
  - 409 while the first request is still in flight.
- Leads feed billing, and a public form double-submits and retries, yet `createLead` takes no key.
- **Severity:** medium. **Confidence:** high.
- **Change:**
  - Write these rules into the skill, citing the source as a draft.
  - Scope keys per principal or partner, and store them in the same transaction as the state change.
  - Have the Server Action mint a key each time the form is rendered.

**P9. 3.1 support is young.** STACK §4 says "(OpenAPI 3.1)" without a caveat.
- **Evidence:** oapi-codegen's 3.1 support is described as "initial" and arrived in v2.8.0 (2026-07-17). kin-openapi changed its 3.1 semantics across v0.150–v0.153.
- **Severity:** low. **Confidence:** medium.
- **Change:** restrict the contract to a lint-enforced 3.1 subset: type arrays for null, `const`, `examples`. Avoid `prefixItems`, `if`/`then`/`else` and the `dependent*` keywords.

**P10. The reference contract breaks its own rules.** `api/openapi.yaml`.
- **Evidence:** no `tags` (the rules require one per module), no `securitySchemes`, no response headers, and a 201 without `Location`.
- **Severity:** low. **Confidence:** high.
- **Change:** this shows the rules aren't enforced; see G1.

## 4. Gaps

**Now**
- **G1. Spec lint in `make check`.** Options: vacuum v0.40 (Go binary, Spectral-compatible, OpenAPI 3.0–3.2, OWASP rules), Redocly CLI 2.78 or Spectral 6.17. Add custom rules for:
  - tags and operationId;
  - a `default` Problem response on every operation;
  - bounds on request fields;
  - parameter names that look like personal data;
  - casing;
  - the 3.1 subset from P9.
  
  Zalando pairs API-first with an automated linter.
- **G2. Breaking-change gate.** Run `oasdiff breaking` against `origin/main` and fail on ERR (v1.37.0, supports 3.1 and 3.2, has a GitHub Action). This replaces the code-reviewer's "Contract: compatible | BREAKING" judgment with a deterministic check.
- **G3. Field errors and declared codes.**
  - Use the RFC 9457 §3 `errors: [{detail, pointer}]` extension for 400 and 422.
  - List each operation's error codes in the contract so TypeScript can type `code` as an open union.
  - The lead form then maps server errors to fields.
- **G4. Auth in the contract.** Declare a cookie `securitySchemes` and document 401/403. Once security is declared, the validator needs an `AuthenticationFunc` (or `NoopAuthenticationFunc` if a separate middleware authenticates); otherwise it returns 401 for every secured call.
- **G5. Headers in the contract.** The strict server only writes headers declared in the contract: `ETag`, `If-Match`, `Location`, `Retry-After`. Also return `428 Precondition Required` (RFC 6585) when `If-Match` is missing.
- **G6. Rate-limit responses.** Declare `429` with `Retry-After` on public endpoints such as lead creation (spam target). Zalando: "MUST use code 429 with headers".

**Later**
- **G7. Mocks** for parallel frontend work: Prism 5.16 (supports 3.1) or MSW handlers generated by orval, kubb or Hey API.
- **G8. Schemathesis 4.33** (property-based testing from the spec) against a running backend in CI.
- **G9. Split the spec** into files bundled with `redocly bundle`, or move authoring to TypeSpec, once the YAML is too big for an agent to read comfortably.
- **G10. Deprecation signals:** `deprecated: true`, the `Deprecation` header (RFC 9745), `Sunset` (RFC 8594), and the oasdiff changelog.
- **G11. Separate partner contract:** its own audience (Zalando `x-audience`), versioning and auth.
- **G12. `output-options.nullable-type: true`** before the first PATCH endpoint, to tell absent from null.
- **G13. Cursor rules:** bind the cursor to the query's filters, and never let it act as an authorization token (AIP-158).
- **G14. HTTP `QUERY`** (RFC 10008, June 2026) for searches with a body, once OpenAPI 3.2 tooling supports it. Also watch oapi-codegen-exp, the v3 prototype built on libopenapi with 3.1/3.2 support.

## 5. Over-engineering

Little in this area.
- "Breaking changes go to a new path version" is heavier than needed for an internal API with one consumer. Default to expand → contract and keep new versions for the partner API.
- `httpserver.Problem` is a hand-written copy of the generated type, which STACK §1 calls a bug. It is harmless (`platform` can't import `gen`), but the exception should be documented.

## 6. Sources

| Source | Title | Author or organization | Date |
|---|---|---|---|
| https://github.com/openapi-ts/openapi-typescript/issues/2874 | "openapi-typescript in maintenance mode" | drwpow (maintainer) | 2026-09-19 |
| https://github.com/openapi-ts/openapi-typescript/issues/2841 | "openapi-typescript doesn't work with Typescript 7" | issue | 2026-07-09 |
| https://www.rfc-editor.org/rfc/rfc9457 | Problem Details for HTTP APIs | IETF | 2023-07 |
| https://www.rfc-editor.org/rfc/rfc9110 | HTTP Semantics | IETF | 2022-06 |
| https://www.rfc-editor.org/rfc/rfc10008 | The HTTP QUERY Method | IETF | 2026-06 |
| https://www.rfc-editor.org/rfc/rfc9745 | The Deprecation HTTP Response Header Field | IETF | 2025-03 |
| https://www.rfc-editor.org/rfc/rfc8594 | The Sunset HTTP Header Field | IETF | 2019-05 |
| https://www.rfc-editor.org/rfc/rfc6585 | Additional HTTP Status Codes (428) | IETF | 2012-04 |
| https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/ | The Idempotency-Key HTTP Header Field, draft -08 | IETF httpapi WG | 2026-10-22 |
| https://spec.openapis.org/oas/v3.2.0.html | OpenAPI Specification 3.2.0 | OpenAPI Initiative | 2025-09-19 |
| https://opensource.zalando.com/restful-api-guidelines/ | RESTful API Guidelines | Zalando | updated 2026-09-29 |
| https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md | Azure REST API Guidelines | Microsoft | updated 2026-10-22 |
| https://google.aip.dev/158, /155, /180, /185, /193 | API Improvement Proposals | Google | 2024–2025 changelogs |
| oapi-codegen v2.8.2 README, configuration schema, std-http and strict templates (module cache); https://github.com/oapi-codegen/oapi-codegen/releases | oapi-codegen | oapi-codegen | v2.8.0 2026-07-17; v2.8.2 2026-10-31 |
| kin-openapi v0.153.0 README/CHANGELOG, `openapi3filter/validate_request.go`, `openapi3/schema_error.go` | kin-openapi | getkin | 2026-11-07 |
| nethttp-middleware v1.3.1 `oapi_validate.go` and README | nethttp-middleware | oapi-codegen | 2026-10-23 |
| santhosh-tekuri/jsonschema v6.0.3 `kind/kind.go` | jsonschema | santhosh-tekuri | v6.0.3 |
| https://github.com/oapi-codegen/oapi-codegen-exp | V3 prototype | oapi-codegen | undated |
| https://github.com/ogen-go/ogen | ogen | ogen-go | v1.24.0, 2026-08-07 |
| https://huma.rocks/ | Huma | Daniel Taylor | v2.40.0, 2026-10-27 |
| https://typespec.io/blog/typespec-1-0-GA-release/ | TypeSpec 1.0 GA | Microsoft | 2025-05 |
| https://connectrpc.com/docs/web/get-requests-and-caching/ | Get requests and caching | ConnectRPC / Buf | undated |
| https://heyapi.dev/openapi-ts/get-started, `/plugins/tanstack-query`, `/clients/next-js`; https://github.com/hey-api/hey-api | Hey API docs and repo | Hey API | 0.99.0, 2026-06-22 |
| https://www.orval.dev/versions/v8 | Upgrading to v8 | Orval | npm 8.47.0, 2026-11-07 |
| https://github.com/oasdiff/oasdiff | oasdiff | oasdiff | v1.37.0, 2026-11-08 |
| https://github.com/daveshanley/vacuum | vacuum | pb33f | v0.40.0, 2026-11-12 |
| https://github.com/stoplightio/spectral | Spectral | Stoplight | 6.17.1, 2026-11-06 |
| https://github.com/stoplightio/prism | Prism | Stoplight | 5.16.0, 2026-07-17 |
| https://redocly.com/docs/cli | Redocly CLI | Redocly | 2.78.0, 2026-11-12 |
| https://pypi.org/project/schemathesis/ | Schemathesis | Schemathesis | 4.33.4, 2026-11-12 |
| registry.npmjs.org, api.npmjs.org (downloads 2026-10-13 to 11-11), proxy.golang.org | package registries | npm, Go | queried 2026-11-13 |
