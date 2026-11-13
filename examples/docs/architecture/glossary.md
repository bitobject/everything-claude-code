# Glossary

> Starting point from the ecc plugin: the terms known so far, per module. Modules other than
> `leads` are *proposed* (see `subdomains.md`). Confirm or correct the terms with the team.

Code is in English; users and the business speak Russian. Each concept has one name everywhere:
Go identifiers, SQL, the OpenAPI contract, the web code and the plans. Two names for one concept,
or one name for two concepts inside a module, is a bug in this glossary.

When a module is created, its section moves into `backend/internal/<module>/CLAUDE.md` under
"Glossary" and is replaced here by a link. From then on the module's `CLAUDE.md` is the only
source, and Claude Code loads it whenever Claude works in that module. Terms that mean different
things in two modules are listed in `context-map.md`.

## leads

| Term (code) | Russian | Meaning |
|---|---|---|
| Lead | заявка, лид | A request for legal help left by a client: phone, legal topic, city. Lifecycle: new → assigned → closed. |
| Client | клиент | The person who left the lead. |
| Assignment | назначение лида | The lead is given to one lawyer; only a new lead can be assigned. |
| Routing | распределение | Choosing which lawyers may receive a lead: by city and topic; further rules are open. |

## billing

| Term (code) | Russian | Meaning |
|---|---|---|
| Sale | продажа лида | A lead assigned to a lawyer for its price, paid from the lawyer's balance (open: `subdomains.md`, question 2). |
| Price | цена лида | Depends on the city and the legal topic; a sale keeps a snapshot of the price. Who owns the price list is open. |
| Balance | баланс | The lawyer's prepaid money, an integer number of kopecks. |
| Top-up | пополнение | A payment through the provider that increases the balance. |
| Debit | списание | Money taken from the balance, for example for a lead. |

## lawyers

| Term (code) | Russian | Meaning |
|---|---|---|
| Lawyer | юрист | A professional who buys leads and works in the cabinet. |
| Profile | анкета | What is known about a lawyer: legal topics, cities, contacts. |
| Cabinet | личный кабинет | The part of the web where a logged-in lawyer works. |

## catalog

| Term (code) | Russian | Meaning |
|---|---|---|
| City | город | A city with its own public pages; leads and lawyers refer to it by ID. |
| Topic | тема, категория права | An area of law (for example divorce); drives pages, routing and price. |
| Landing page | посадочная страница | A public page for a city, a topic, or a city and a topic. |

## identity

| Term (code) | Russian | Meaning |
|---|---|---|
| User | пользователь | Someone who can log in: a lawyer or staff. |
| Staff | сотрудник | Our operator with extra permissions. |
| Session | сессия | A server-side login session, referenced by an HttpOnly cookie. |

## notifications

| Term (code) | Russian | Meaning |
|---|---|---|
| Notification | уведомление | A message to a person, sent as an SMS. |
