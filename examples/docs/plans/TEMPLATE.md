<!-- Plan template (ecc plugin: examples/docs/plans/TEMPLATE.md). The planner agent (/ecc:plan)
writes docs/plans/<slug>.md in this shape; use it by hand for a plan written without the agent.
A small, low-risk change inside one module needs no plan file.

Lifecycle of the Status line:
  draft                    written, not yet approved: no code is written against it
  approved YYYY-MM-DD      the user said "go"; changes to the plan after this need their OK again
  done YYYY-MM-DD          every step has merged
  abandoned YYYY-MM-DD     with the reason
The approved plan is committed with its first step. Each later step ticks its box in the change
that implements it, so main always shows how far the plan has got, and the reviewer sees which
step a change claims. -->

# Plan: <what changes, in the user's words>

Status: draft
Request: <the request in one line, or a link to the issue>
Touches: <backend modules, web areas, the contract> · ADRs: <docs/adr/NNNN-<title>.md, or none>

## Goal
<One paragraph: what is different afterwards, for whom, observable from outside.>

## Acceptance criteria
<Each one a behavior that a test can check, and the layer whose test proves it. Numbered: steps
and reviews refer to them.>
- AC1. <Given …, when …, then …>. Test: <entity | usecase | repository | httpapi | web | e2e>.
- AC2. <…>

## Scope
- In: <what this plan changes>.
- Out: <what it deliberately leaves out, and where that goes: a later plan, an issue>.

## Open questions
<Each with options and a recommendation. While a question that changes the steps is open, the plan
stays a draft.>
- Q1. <question> Options: (a) <…>; (b) <…>. Recommendation: (a), because <…>.

## Decisions
<Answers to the questions and choices made while planning, with the reason and who decided. A
decision that is hard to reverse also gets an ADR in docs/adr/.>
- D1 (Q1). <decision> — <reason>; <who>, <YYYY-MM-DD>.

## Steps
<Ordered and test-first. Each step is one change that compiles, passes its tests and can merge to
main by itself, ideally in a day: about 100 changed lines, generated code aside. Inside a step the
order is contract → entity → use case → storage → delivery → wiring → web; a new API operation
merges in the same step as its handler, or the build breaks. Unfinished behavior stays unreachable
for users (the web entry point comes last) or behind a flag that a later step removes. Each step
passes /ecc:verify and is reviewed against this plan (/ecc:code-review) before it merges.>
- [ ] 1. <layer: files> — <what changes>. Test: <test name, and what it shows> (`<command>`).
  Covers: AC1.
- [ ] 2. <…>

## Risks
<Each with likelihood, impact and mitigation: a migration on a large table, a breaking contract
change, cache invalidation, concurrent updates of the same row, personal data.>
