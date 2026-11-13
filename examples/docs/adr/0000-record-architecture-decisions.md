---
status: accepted
date: {YYYY-MM-DD: the day /ecc:setup-project copied this file}
decision-makers: {project owner}
---

# Record architecture decisions in this repository with MADR 4.0

## Context and Problem Statement

People and coding agents change this code base every day. A decision that lives only in a chat,
in the ecc plugin or in someone's head gets lost or contradicted. Where are decisions recorded, in
which format, and which source wins when this project and the plugin's defaults disagree?

## Considered Options

* ADRs in this repository (`docs/adr/`, MADR 4.0), with the ecc plugin's `STACK.md` as the baseline
* The ecc plugin's `STACK.md` only
* No written record (chat, commit messages)

## Decision Outcome

Chosen option: "ADRs in this repository, with `STACK.md` as the baseline", because a decision has
to be versioned and reviewed together with the code that implements it, and this project must be
able to depart from the plugin's defaults on purpose.

* One file per decision: `docs/adr/NNNN-<title-with-dashes>.md`, from `docs/adr/TEMPLATE.md`.
* Status: `proposed` (drafted, usually by the `architect` agent) → `accepted` (a person approved
  it) or `rejected`; later `deprecated` or `superseded by ADR-NNNN`. An accepted ADR is not
  rewritten: a new ADR supersedes it.
* The ecc plugin's `STACK.md` holds the default decisions. Where an accepted ADR here says
  otherwise, the ADR wins for this project.
* An ADR is written for decisions that are hard to reverse: module boundaries and data ownership,
  consistency between modules, a new dependency or piece of infrastructure, the shape of the
  public contract or of events. Small, reversible choices go to the "Decisions" section of the plan
  in `docs/plans/`.

### Consequences

* Good, because a decision is reviewed in the same change as the code that implements it.
* Good, because agents read the decision instead of guessing it.
* Bad, because writing ADRs takes time; that is why small, reversible choices stay in plans.

### Confirmation

`/ecc:code-review` reports a hard-to-reverse change that has no ADR. The `architect` agent drafts
the ADR together with the design.

## More Information

* MADR 4.0.0: <https://adr.github.io/madr/>
* Architecture overview: `docs/architecture/README.md`
