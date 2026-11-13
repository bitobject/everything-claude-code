# Subdomains

> Starting point from the ecc plugin. The types are the owner's classification; the Module and
> Profile columns are *proposed*. Confirm or correct them, answer the open questions, then remove
> this note.

<product> is lead generation for legal services in Russia: people who need a lawyer leave a request
(a lead) on the public pages; the platform routes each lead to suitable lawyers and sells it to one
of them; lawyers work with their leads in a cabinet.

| Subdomain | Type | Module (*proposed*) | Profile (*proposed*) |
|---|---|---|---|
| Lead routing: which lawyers may receive a lead | core | `leads` (question 1) | full |
| Lead sale: price, assignment to one lawyer, payment from the lawyer's balance | core | `leads` + `billing` (question 2) | full |
| Lawyer profiles: who the lawyer is, topics, cities, contacts | supporting | `lawyers` | question 3 |
| Content catalog for SEO pages: cities, legal topics, page texts | supporting | `catalog` (question 4) | thin |
| Identity and login | generic | `identity` (question 6) | thin |
| Payments | generic | a port of `billing`, implemented by an adapter for the provider | none: no module of its own |
| SMS | generic | `notifications` | thin |

What the type changes:
- **core**: where the product competes and the rules keep changing. Built in-house, full profile,
  the most thorough tests, an ADR for every hard-to-reverse decision.
- **supporting**: needed by the core, but nobody picks the product for it. Built simply: thin
  profile unless one of the full-profile criteria holds.
- **generic**: a solved problem. Adopted (a provider or a library) behind a port, never rebuilt.

Profile criteria: full if at least one holds — a lifecycle or state machine; money or legal
obligations; invariants across several fields or aggregates; several entry points (HTTP, jobs,
other modules); about 10 or more use cases. Thin otherwise. Each module states its profile and the
reason in its `CLAUDE.md`.

## Open questions

1. Does lead routing stay in `leads`, or become a module of its own when its rules grow (limits per
   lawyer, exclusivity, scoring)? Recommendation: keep it in `leads` until it needs its own language
   and tests; splitting a module later is cheaper than merging two.
2. A lead sale spans assignment (`leads`) and payment (`billing`). Options: (a) the money invariant
   lives in one module; (b) a saga on River jobs with idempotent steps and compensations; (c) a
   deliberate shared transaction. Decide when `billing` is designed, in an ADR. Until then, inside
   `Do`/`Update` a module uses only its own repositories and never calls another module.
3. Lawyer profiles: full or thin? Full if a profile has a lifecycle (moderation, verification,
   blocking).
4. Who owns the reference data that drives SEO pages, routing and prices (cities, legal topics):
   `catalog`, a dedicated schema, or a static list in code? Recommendation: `catalog`; the other
   modules keep only IDs, or read-only copies fed by its events.
5. `partners`: the plugin expects partner systems to create leads through the API (with an
   `Idempotency-Key`). Only a lead source feeding `leads`, or more (partner accounts, payouts)?
   Which type?
6. Login (decided in the plugin's `STACK.md`: only Russian services — email, Yandex ID, VK ID or a
   phone number). Still open: email by one-time code or by password; whether accounts that share
   a verified email are linked automatically or only from the cabinet after logging in; who logs
   in (lawyers and staff only, or clients too); whether email login alone satisfies 149-FZ (a
   question for the lawyer). Codes by SMS and email make `identity` depend on `notifications`.
