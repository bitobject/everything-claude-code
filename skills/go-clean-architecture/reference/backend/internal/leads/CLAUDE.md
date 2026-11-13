# leads module

## Purpose
Captures requests for legal help from the public forms (leads) and tracks each lead until a
lawyer takes it. A module of its own: the lead's lifecycle and the anti-spam rules change at their
own pace, and other modules react to its events (notifications now; routing and billing later).
Subdomain: core (`docs/architecture/subdomains.md`).

## Profile
full: the lead has a lifecycle (`new` → `assigned` → `closed`) and several entry points (HTTP, a
periodic job, the facade other modules read through).

## Glossary
- **Lead** (заявка, лид) — one request from a visitor: phone, legal topic, city; not an account.
- **Assign** (назначить) — give a new lead to a lawyer; only from `new`.
- **Idempotency key** (ключ идемпотентности) — the form's random key per submission; a retry with
  it returns the first lead instead of creating another one.

## Public API
- Facade: `contract.Reader` (`contract/contract.go`), implemented by `delivery/facade`, built in
  `cmd` — other modules read a lead by ID (the notifications worker).
- DTOs: `contract.Lead` (no phone); error `contract.ErrLeadNotFound`.
- What callers must know: `Status` may get new values; treat an unknown one like `new`.

## Events
- Published (`contract/contract.go`): `LeadCreated` v1 (`leads.lead_created.v1`) — in the
  transaction that stores a new lead, not on an idempotent replay; it carries the lead ID only.
- Consumed: none.

## Invariants
- A lead is valid from creation: phone `+7` and 10 digits, topic 3–200 and city 2–100 characters
  after trimming — `entity.NewLead`, `entity.Parse*`; `TestParsePhone`, `FuzzParsePhone`,
  `TestNewLeadRefusesZeroValues`.
- Only a new lead can be assigned — `entity.Lead.Assign`; `TestLeadAssign`, `TestAssignLead`.
- One Idempotency-Key, one lead, for 24 hours; the key with another body is 422 —
  `usecase.CreateLead`; `TestCreateLeadRepeatedKeyReturnsTheFirstLead`,
  `TestCreateLeadKeyReusedWithAnotherBody`.
- At most 3 leads per phone in 24 hours (approximate under concurrency) — `usecase.CreateLead`;
  `TestCreateLeadLimitPerPhone`, `TestCreateLeadRateLimited`.
- The phone never leaves the module: not in responses, events, errors or logs (`entity.Phone`
  masks itself) — `TestPhoneIsMaskedWhenPrinted`, `TestInvalidRequestsNeverEchoInput`.

## Data
- Schema `leads`; tables: `leads` — one lead; `idempotency_keys` — one key, a fingerprint of its
  request and the lead it created.
- Personal data: `leads.phone`; `idempotency_keys.fingerprint` (SHA-256 of phone, topic and city:
  trying every phone number reverses it). `topic` is free text and may hold personal details.
- Retention: `idempotency_keys` rows are deleted 24 hours after creation by the hourly job
  `leads.purge_idempotency_keys` (`delivery/worker`). Leads: not decided yet.

## Depends on
- none: only `platform` and `kernel`. Other modules reach leads through `contract` alone.

## Commands
- Tests: `cd backend && go test -race ./internal/leads/...`.
- `repository/postgres` tests need PostgreSQL 18 at `TEST_DATABASE_URL` (a role with CREATEDB;
  `pgtest` gives each test its own database); without it they fail, `-short` skips them.
- Boundaries: `cd backend && bin/go-arch-lint check`. Done: `/ecc:verify`.
