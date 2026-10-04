# Local ID index

Resolves local `T-nn`/`G-nn` IDs cited in the repo to their GitHub issue.
Nothing else lives here: acceptance, state and priority are on
[GitHub](https://github.com/sergeyfarin/rekenraam/issues); order is in the
[roadmap](roadmap.md). Add a row when a new issue's ID is referenced in the
repo; new issues take the next unused `T-nn`, `G-nn` or `S-nn`. Local IDs and
GitHub numbers are distinct; cite both.

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
| T-135 | [#150 — Admit changed pooled-transfer lineage under replay](https://github.com/sergeyfarin/rekenraam/issues/150) |
| T-136 | [#151 — Label split adjustment journals in transaction lists and registers](https://github.com/sergeyfarin/rekenraam/issues/151) |
| T-137 | [#152 — Revised Trading 212 fills before the fetch cursor reachable from Refresh](https://github.com/sergeyfarin/rekenraam/issues/152) |

IDs not listed here were closed before the GitHub migration; see the
[pre-migration snapshot](reviews/open-backlog-before-github-2026-09-27.md)
(including the T-42–T-47 collision mapping) and the
[July](reviews/resolved-backlog-2026-07.md) and
[September](reviews/resolved-backlog-2026-09-27.md) resolution records.
