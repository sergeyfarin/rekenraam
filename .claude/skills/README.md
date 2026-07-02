# Rekenraam Skill Library

Six core skills that let a junior/mid-level engineer or a smaller model
(Sonnet-class, Codex/ChatGPT) work on this project at the standard it was
built to. Each is a self-contained `SKILL.md` with hard rules, real file
references, exact commands, and this repo's known bug classes.

| Skill | Use when |
|---|---|
| `ledger-invariants` | Touching money, transactions, postings, reconciliation, investments, prices/FX — read **before** coding |
| `backend-slice` | Any Go change: endpoints, services, repositories, migrations, backend tests |
| `api-contract` | Adding/changing an `/api/v1` endpoint, schema, or error code |
| `frontend-screen` | Anything under `frontend/src` |
| `background-work` | Workers, schedulers, provider fetching, FX refresh, online import |
| `validate-and-ship` | Before every commit; running/debugging the app; deciding which doc to update |

Typical task = 2–3 skills. Example: "add a cashflow report endpoint + screen"
→ `backend-slice` + `api-contract` + `frontend-screen`, then
`validate-and-ship`. Anything financial also gets `ledger-invariants`.

## How to use with Claude Code (any model, including Sonnet)

Nothing to configure — skills in `.claude/skills/` are discovered
automatically and invoked when the task matches the description. To force one,
mention it: *"Apply the ledger-invariants skill, then add a void-checkpoint
endpoint."* With Sonnet or other smaller models, be explicit: name the 2–3
relevant skills in your first prompt rather than relying on auto-triggering.

Suggested prompt shape for a Sonnet session:

> Read `.claude/skills/backend-slice/SKILL.md` and
> `.claude/skills/api-contract/SKILL.md`, then implement <task>. Before
> committing, run through `.claude/skills/validate-and-ship/SKILL.md`.

## How to use with Codex / ChatGPT (or any non-Claude agent)

These tools don't read `.claude/skills/` on their own. Two options:

1. **Reference by path** (Codex CLI and other repo-aware agents): `AGENTS.md`
   points here; additionally paste into the task prompt:
   *"Follow `.claude/skills/<name>/SKILL.md` for this work."*
2. **Paste inline** (chat UIs without repo access): copy the relevant
   `SKILL.md` bodies into the system/first message. `ledger-invariants` +
   `validate-and-ship` are the two to always include for financial changes.

## Rules for maintaining this library

- Keep it small. A new skill must earn its place: recurring task shape, real
  failure modes, not derivable from a quick read of the code.
- When a convention changes in `docs/`, update the affected skill in the same
  change — skills that drift from `docs/conventions.md` are worse than none.
  The docs govern; skills summarize and point.
- When a new bug class ships, add it to the `validate-and-ship` review
  checklist (and `docs/backlog.md`) so it is checked forever after.
