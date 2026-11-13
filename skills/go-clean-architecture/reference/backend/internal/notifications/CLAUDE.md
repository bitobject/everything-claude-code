# notifications module

## Purpose
Tells our operators about new leads: one message per lead in their feed (the reference stores the
message; a real one sends it by SMS or email through a provider). A module of its own: delivery
channels and their providers change independently of how leads work.
Subdomain: generic (`docs/architecture/subdomains.md`).

## Profile
thin: no lifecycle, one entry point (a job), no rules beyond formatting the message. One package
with its job, service code and sqlc queries (`notifications.go`, `queries/`, `sqlcgen/`).

## Glossary
- **Message** (сообщение) — what an operator sees about one lead; not a chat message.

## Public API
- none: no module calls notifications. It gets a `contract` package when one does.

## Events
- Published: none.
- Consumed: `leads.LeadCreated` v1 → job `notifications.lead_created` (`LeadCreatedArgs`,
  `notifications.go`) — records the message; idempotent by `lead_id`. `Subscribe` registers it;
  `cmd/api` calls it, so every process that publishes knows the subscriber.

## Invariants
- One message per lead, even when the job runs twice — `messages_lead_id_key` and
  `InsertMessage ... ON CONFLICT DO NOTHING`; `TestWorkRecordsOneMessagePerLead`.
- A job for a lead that doesn't exist is cancelled, not retried — `jobs.Outcome`;
  `TestWorkCancelsWhenTheLeadIsGone`, `TestWorkRetriesOtherErrors`.
- Job arguments hold IDs only and change only by adding fields — `TestLeadCreatedArgsJSON`.

## Data
- Schema `notifications`; tables: `messages` — one message about one lead.
- Personal data: none (the body has the topic and the city, never the phone).
- Retention: not decided yet.

## Depends on
- `leads` — sync: reads the lead through `leads/contract.Reader` (built in `cmd/worker`).
- `leads` — events: `LeadCreated` v1.

## Commands
- Tests: `cd backend && go test -race ./internal/notifications/...` (the message test needs
  PostgreSQL 18 at `TEST_DATABASE_URL`; `-short` skips it).
- Boundaries: `cd backend && bin/go-arch-lint check`. Done: `/ecc:verify`.
