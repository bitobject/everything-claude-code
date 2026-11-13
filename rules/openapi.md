---
paths:
  - "api/**"
---
# OpenAPI contract

Detail: skill `openapi-contract-first`.

- `api/openapi.yaml` (OpenAPI 3.1, the subset the generators handle) is the source of truth for
  HTTP. Change it first, then regenerate **both** sides in the same change:
  `cd backend && go generate ./internal/gen/openapi/` and `cd web && pnpm generate:api`.
  `make api-lint` (vacuum) passes; CI runs `make api-breaking` (oasdiff against main).
- Every operation: `operationId` (camelCase verb+noun), one tag per backend module, `default`
  response `Problem` plus the specific error statuses, and `x-problem-codes` listing its codes by
  status. Problems: a `type` URI per code, a stable dotted `code`, `errors[]` with
  `pointer`/`parameter` and the rule, never the submitted values.
- Security is global (`security: [{session: []}]`); a public operation says `security: []`.
- Request bodies: `additionalProperties: false`, `required`, bounds on every field.
- Headers in the contract: `Location` on 201, `Retry-After` on 429 (declare 429 on public
  operations; 503 comes through `default`, and the server adds `Retry-After` to it too),
  `ETag`/`If-Match` where two users edit one resource. Retryable creates require
  `Idempotency-Key` (a quoted Structured Field String).
- No personal data in paths, query strings or header parameters; responses expose only what the
  caller may see.
- Lists: cursor pagination (`limit` with a `maximum`, `nextCursor`). Money: integer minor units +
  currency. Instants: `date-time` in UTC.
- Deploy order: a new request field ships in the backend before the web sends it.
- Breaking changes (removing/renaming, new required request fields or headers, narrowed enums,
  changed status codes or `operationId`, security added to a public operation) need a new version
  or an expand → migrate → contract plan. Ask the user before making one.
