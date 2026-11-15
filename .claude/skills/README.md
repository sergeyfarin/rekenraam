# Rekenraam Skill Library

Eight skills that let any agent work on this project at the standard it was
built to:

- Two **task modes** say how a session runs from start to finish:
  `work-issue` (implement, then open a pull request) and `review-pr` (review
  only, change nothing).
- Six **area skills** hold the rules for a part of the codebase:
  `ledger-invariants`, `backend-slice`, `api-contract`, `frontend-screen`,
  `background-work`, `validate-and-ship`.

Which skill covers which work is in `AGENTS.md` → Task Modes and Task Skills,
because every harness loads that file. This file is for maintaining the
library.

## Layout

```
.claude/skills/<name>/SKILL.md      rules for every use of the skill
.claude/skills/<name>/<topic>.md    reference file, read when SKILL.md says so
.agents/skills -> ../.claude/skills symlink, so Codex finds the same files
```

Reference files today: `ledger-invariants/investments.md` (the investment
subledger contract) and `validate-and-ship/bug-classes/<area>.md` (the
catalogue of bugs this repo has shipped, split by area).

## How each harness finds them

| Harness | Instructions | Skills | Force one |
|---|---|---|---|
| Claude Code | `AGENTS.md` | `.claude/skills/` | `/skill-name` |
| Codex | `AGENTS.md` | `.agents/skills/` (the symlink) | `$skill-name` |
| OpenCode | `AGENTS.md` | `.claude/skills/` | `@skill-name` |

All three load only each skill's `name` and `description` up front, the
`SKILL.md` body when the skill is used, and a reference file only when the
body tells the agent to read it. Three consequences:

- The description decides whether a skill is used at all. It says what the
  skill covers and when to use it, in words a task prompt would contain.
- A reference file nobody is told to read is never read. `SKILL.md` names each
  one and the work that needs it.
- When a session should not depend on automatic selection, start the prompt
  with the task mode: `/work-issue 592`, `$work-issue 592` or
  `@work-issue 592`; `/review-pr 585` and its equivalents for a review. Name
  area skills too when the area is not obvious from the issue.

## Rules for maintaining this library

- Keep it small. A new skill must earn its place: a recurring task shape, real
  failure modes, not derivable from a quick read of the code.
- A task mode is a procedure and an area skill is a rule set. Keep them apart:
  a mode points at area skills, and no area rule is restated in a mode.
- Keep `SKILL.md` lean, about 250 lines at most. It holds what applies to
  every use of the skill. Detail for one family or one area goes in a
  reference file. A skill that grows by one appended paragraph per issue is
  turning into a changelog: `validate-and-ship` reached 1,000 lines that way
  before its catalogue moved to `bug-classes/`.
- Write the rule, then its named test. The issue number points at the story;
  the story itself belongs in the issue.
- Frontmatter is `name` and `description` only, and `name` equals the
  directory name. That subset is what all three harnesses read.
- Refer to files in another skill by their path from the repo root.
- When a convention changes in `docs/`, update the affected skill in the same
  change. The docs govern; skills summarize and point.
- When a new bug class ships, give it the next free number, add the entry to
  the matching `validate-and-ship/bug-classes/<area>.md` and its title to the
  list in `validate-and-ship/SKILL.md`, in the same change as the fix.
  Numbers are cited from code comments, so they are never reused or
  renumbered. File remaining work as a GitHub issue; `docs/backlog.md` keeps
  historical aliases only.
- `AGENTS.md` is the only repo-wide instruction file. Do not add
  harness-specific copies (`CLAUDE.md`, Copilot or Cursor instruction files);
  they drift from it.
