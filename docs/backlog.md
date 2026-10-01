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
| T-75b | [#99 — Investment-native correction](https://github.com/sergeyfarin/rekenraam/issues/99) |
| T-80 | [#102 — Translation catalog parity](https://github.com/sergeyfarin/rekenraam/issues/102) |
| T-87 | [#101 — Owner-local dates](https://github.com/sergeyfarin/rekenraam/issues/101) |
| T-108 | [#103 — Short sale and cover](https://github.com/sergeyfarin/rekenraam/issues/103) |
| T-112 | [#127 — Trading 212 fill taxonomy admission](https://github.com/sergeyfarin/rekenraam/issues/127) |
| T-113 | [#128 — SQLite fixture URI path escaping](https://github.com/sergeyfarin/rekenraam/issues/128) |

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
