# Technical backlog — GitHub issue index

[GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues) is the live
tracker for open defects, technical debt, and actionable roadmap work. Issue
state, priority labels, discussion, and PR links belong there. This file keeps
the existing local IDs resolvable; it is not a second task list.

| Local ID | GitHub issue |
|---|---|
| G-08 | [#100 — Locale-aware amount entry](https://github.com/sergeyfarin/rekenraam/issues/100) |
| G-09 | [#106 — Exact FX display formatting](https://github.com/sergeyfarin/rekenraam/issues/106) |
| T-34 | [#111 — Investment provider events](https://github.com/sergeyfarin/rekenraam/issues/111) |
| T-48 | [#112 — TypeScript 7 upgrade](https://github.com/sergeyfarin/rekenraam/issues/112) |
| T-54 | [#104 — Derived-price dependencies](https://github.com/sergeyfarin/rekenraam/issues/104) |
| T-56 | [#109 — Post-merge audit](https://github.com/sergeyfarin/rekenraam/issues/109) |
| T-60 | [#110 — Shared report scenario](https://github.com/sergeyfarin/rekenraam/issues/110) |
| T-63 | [#107 — Account-version-gap message](https://github.com/sergeyfarin/rekenraam/issues/107) |
| T-72 | [#105 — Two-process SQLite tests](https://github.com/sergeyfarin/rekenraam/issues/105) |
| T-73 | [#108 — Large-book export and self-check](https://github.com/sergeyfarin/rekenraam/issues/108) |
| T-80 | [#102 — Translation catalog parity](https://github.com/sergeyfarin/rekenraam/issues/102) |
| T-87 | [#101 — Owner-local dates](https://github.com/sergeyfarin/rekenraam/issues/101) |
| T-108 | [#103 — Short sale and cover](https://github.com/sergeyfarin/rekenraam/issues/103) |
| T-112 | [#127 — Trading 212 fill taxonomy admission](https://github.com/sergeyfarin/rekenraam/issues/127) |
| T-113 | [#128 — SQLite fixture URI path escaping](https://github.com/sergeyfarin/rekenraam/issues/128) |
| T-114 | [#129 — Build shared replay gain disclosure and acknowledgement](https://github.com/sergeyfarin/rekenraam/issues/129) |
| T-115 | [#130 — Correct cash dividends and reinvested dividends](https://github.com/sergeyfarin/rekenraam/issues/130) |
| T-116 | [#131 — Correct trade date, holding account, instrument or cost currency](https://github.com/sergeyfarin/rekenraam/issues/131) |
| T-117 | [#132 — Admit backdated transfer-in and replay earlier disposals](https://github.com/sergeyfarin/rekenraam/issues/132) |
| T-118 | [#133 — Reverse or replace an investment write-off](https://github.com/sergeyfarin/rekenraam/issues/133) |
| T-119 | [#134 — Correct in-kind transfers across dependent positions](https://github.com/sergeyfarin/rekenraam/issues/134) |
| T-120 | [#135 — Group correction chains and reconcile their net balance impact](https://github.com/sergeyfarin/rekenraam/issues/135) |
| T-121 | [#136 — Verify provider execution cancellation before broader source revisions](https://github.com/sergeyfarin/rekenraam/issues/136) |
| T-122 | [#137 — Post manual stock splits and map Trading 212 split review](https://github.com/sergeyfarin/rekenraam/issues/137) |
| T-123 | [#138 — Carry pooled average-cost basis through internal transfers](https://github.com/sergeyfarin/rekenraam/issues/138) |
| T-124 | [#139 — Decide cross-position replay scope and consolidate effective investment reads](https://github.com/sergeyfarin/rekenraam/issues/139) |
| T-125 | [#140 — Measure and shorten the complete backend race gate](https://github.com/sergeyfarin/rekenraam/issues/140) |
| T-126 | [#141 — Roll out replay gain acknowledgement across existing commands](https://github.com/sergeyfarin/rekenraam/issues/141) |
| T-127 | [#142 — Make shared reconciliation previews report the full invalidation set](https://github.com/sergeyfarin/rekenraam/issues/142) |
| T-128 | [#143 — API and browser evidence for import gain review and Trading 212 source revisions](https://github.com/sergeyfarin/rekenraam/issues/143) |
| T-129 | [#144 — Correct or reverse posted splits and adjust their journal delta](https://github.com/sergeyfarin/rekenraam/issues/144) |
| T-130 | [#145 — Map verified Trading 212 split fills to the split command](https://github.com/sergeyfarin/rekenraam/issues/145) |
| T-131 | [#146 — Record zero-delta splits without a journal](https://github.com/sergeyfarin/rekenraam/issues/146) |
| T-132 | [#147 — Replay the cross-position dependency closure as one dated stream](https://github.com/sergeyfarin/rekenraam/issues/147) |
| T-133 | [#148 — Require an opening operation on every investment lot](https://github.com/sergeyfarin/rekenraam/issues/148) |
| T-134 | [#149 — Self-check full replay equivalence of investment projections](https://github.com/sergeyfarin/rekenraam/issues/149) |

T-75b [#99](https://github.com/sergeyfarin/rekenraam/issues/99) is closed for
its delivered long-buy/sale scope; the [2026-10-02 review](reviews/investment-review-2026-10-02.md)
maps the remaining correction families above. Local `T-nn` IDs and GitHub
`#nn` numbers are distinct identifiers; include both when discussing a ticket.

The [roadmap](roadmap.md) owns product order; its
[GitHub index](https://github.com/sergeyfarin/rekenraam/issues/120) links current
slices to issues. The [short-horizon queue](todo.md) points to immediate work,
and [implemented](implemented.md) records what ships.

The [pre-migration backlog snapshot](reviews/open-backlog-before-github-2026-09-27.md)
preserves the detailed acceptance context and the T-42–T-47 ID collision
mapping. Closed-item evidence remains in the
[July](reviews/resolved-backlog-2026-07.md) and
[September](reviews/resolved-backlog-2026-09-27.md) resolution records.

For new actionable work, create an issue with the next unambiguous `T-nn`,
`G-nn`, or `S-nn` ID as appropriate and link it here if that ID is referenced
in the repo. Record durable decisions in requirements, conventions, ADRs, or
plans as applicable; issue comments do not replace those documents. Close an
issue after the change and its validation land, and update the shipped-feature
ledger or roadmap when their boundaries change.
