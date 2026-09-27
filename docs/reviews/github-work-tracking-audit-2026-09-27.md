# GitHub work-tracking migration audit — 2026-09-27

**Status:** Migration completed on 2026-09-27. The 14 open local IDs map to
GitHub Issues [#99–#112](https://github.com/sergeyfarin/rekenraam/issues),
seven roadmap slices are [#113–#119](https://github.com/sergeyfarin/rekenraam/issues),
and [#120](https://github.com/sergeyfarin/rekenraam/issues/120) is the roadmap
index. `docs/backlog.md` now keeps only the ID-to-issue links.

This dated snapshot records the triage before moving live work from
`docs/backlog.md` to GitHub Issues. The repository had no GitHub issues when
checked. Keep the local live tracker until issue publication and link
verification succeed.

## Rechecked open IDs

| ID | Priority | Disposition / evidence |
|---|---|---|
| T-75b | P0 | Current R16 native correction gate; generic mutation fence and replay-read foundations are shipped, native command remains. |
| G-08 | P1 | Active locale-aware editable amount parsing gap; display formatting and silent-misparse guard already ship. |
| T-87 | P1 | Owner-local default date gap visible in transaction/investment/pricing forms using browser UTC dates. |
| T-80 | P1 | Five locales each still miss 36 app and 29 settings keys; native review is also pending. Current English catalogs contain 623 + 1,019 keys. |
| T-108 | P2 | Named short-sale and cover workflow after R16 transfer/basis operations; ordinary negative balances remain diagnostic. |
| T-54 | P2 | `derivation_json` and `json_each` remain in the pricing cascade; revisit before R17 enlarges the price graph. |
| T-72 | P2 conditional | Two-process test before supporting or recommending two instances on one database. No current multi-instance promise. |
| G-09 | P3 | FX display has private rounding/`Number` conversion; settle displayed-rate rounding and move to shared exact formatting. |
| T-63 | P3 | Misleading account-version-gap error; keep rejection, improve explanation. |
| T-73 | P3 | Add a bounded large-book export/self-check benchmark before release performance claims. |
| T-56 | P3 | Post-merge audit script/checklist remains useful but no current merge blocks work. |
| T-60 | P3 | Shared report test scenario is maintenance work; existing suites cover the behavior. |
| T-34 | Blocked | Provider-event producer belongs to R15 after IBKR and bank connection work and per-kind R16 rules. |
| T-48 | Blocked | Frontend remains on TypeScript 5.9.3; `openapi-typescript` 7.13.0 is the current release in the [upstream release list](https://github.com/openapi-ts/openapi-typescript/releases). Re-run generator checks when upstream support changes. |

The code checks sampled the current date defaults, editable amount helpers,
pricing dependency storage, investment lifecycle fence, and package versions.
This is a triage, not a proof that every open issue's proposed implementation
is optimal. The source acceptance context is preserved in
`docs/reviews/open-backlog-before-github-2026-09-27.md` and the linked plans.

## Roadmap tickets to publish

- One short roadmap issue linking the ordered Git roadmap and the current
  work. Keep the product sequence and durable decisions in Git.
- R16: runtime extraction (before the next large backend slice), T-75b
  correction, in-kind transfers/basis actions, T-108 short sale/cover, then
  compound corporate actions.
- R11: price/FX management UI after R16; R17: quote-provider registry and
  crypto instrument entry; R18: reproducible gains projections; R13: returns
  analytics after R18.
- R6 import depth, R15 connections, R14a attachments, and R7a entry
  convenience remain later. Do not create speculative per-adapter issues
  before their provider verification gates are met.
- Public-release work: personal-access tokens, signing/distribution, and
  adoption assets deserve separate issues when those gates are actively
  scheduled.

After issue creation, replace `docs/backlog.md` with a compact ID-to-issue
index, update `docs/todo.md` to link the current tickets, and make
`docs/README.md` describe GitHub Issues as the live work tracker. Retain the
dated closed-item records and this audit; do not bulk-publish closed history.
