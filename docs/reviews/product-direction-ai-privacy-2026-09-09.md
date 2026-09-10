# Rekenraam direction review — 9 September 2026

**Status: research and recommendations for owner consideration.** This is a
dated strategic review, not an accepted ADR or a replacement execution plan.
The current R10 learning M2 → M3 → M4 → R8 sequence remains authoritative.
Recommendations below that would change it require an explicit later decision
recorded in the governing documents.

**Recommendation:** continue Rekenraam, concentrate its promise, and test
adoption before broadening the suite. The strongest candidate is a user-owned
financial workspace for people with accounts and investments in several
currencies: reconcile the records, understand the next cash commitments, and
inspect how every reported number was obtained. Optional AI can help operate
that workspace. A generic finance chatbot or standalone MCP wrapper would be
a weaker direction for the assets already built.

The development effort already spent is not a reason to continue by itself.
The reusable ledger, import pipeline, reconciliation, investment subledger,
and operational safeguards are reasons: they reduce the remaining cost of
delivering this narrower outcome. Conversely, advanced implementation is not
evidence of adoption or willingness to pay.

**Evidence and limits**

Reviewed the active repository at `b53319e2`, including the product requirements,
conventions, architecture decisions, relevant ADRs, roadmap, TODO, backlog,
implemented ledger, competitor comparison, forecast plans and R10 acceptance
review. Sampled current code and contracts for authentication, reports,
forecasting and learning. Archived stacks are not treated as current.

Local capabilities below come from those documents and sampled source.
Existing test evidence is attributed to the recorded acceptance reviews; this
review did not rerun the application, audit every implementation, inspect
personal financial databases, or conduct user interviews. External facts were
checked against the linked primary sources on 2026-09-09. Competitor documentation
establishes advertised capability, not equivalent correctness or usability.
No market-size, adoption-rate, forecast-accuracy or revenue estimate is claimed.

**What is worth preserving**

| Existing asset | Product opportunity | Material limit |
|---|---|---|
| Exact multi-commodity journal, versioned financial records and guarded reconciliation | Establish a consistent record across accounts and explain changes | Balanced arithmetic does not prove that imports are complete or categories are correct |
| CSV profiles, preview-time rules, QIF and Trading 212 ingestion | Reduce repeated migration and monthly bookkeeping work | Coverage is bounded; XLSX/OFX and broader providers remain future work |
| Net worth, spending and cashflow reports with drill-down and optional reporting currency | Answer questions from deterministic calculations instead of asking an LLM to total rows | Different reports answer different questions; missing FX must remain visible |
| Recurring drafts and an accepted deterministic forecast | Separate recorded money from expected commitments and identify projected cash pressure | Not a bank available balance, credit-limit promise, market forecast or safe-to-spend guarantee |
| Investment lots and atomic trade creation | Connect brokerage cash and holdings in one record | T-75b correction workflows and T-76 disposal-policy provenance remain open |
| Verified scheduled backups, restore, exports and read-only self-check | Make ownership and continuity tangible | Core CSV/QIF exports are not a lossless export of every application feature; database backups and secret-key custody serve a different purpose |
| One Go binary, SQLite, responsive web and local authentication | Keep deployment and ongoing dependencies comparatively contained | Local installation and LAN HTTPS still require operator work; this is not browser-side encrypted synchronization |

Implementation anchors:
[reports](../../backend/internal/app/reports.go),
[forecast service](../../backend/internal/app/forecast.go),
[learning foundation](../../backend/internal/app/forecast_learning.go),
[implemented capabilities](../implemented.md),
[R10 acceptance](r10-core-acceptance-review-2026-09-07.md),
[export contract](../adrs/0011-ledger-export-contract.md), and
[investment boundaries](../adrs/0012-investment-ledger-subledger-and-reporting-boundaries.md).

**Where the existing strategy overreaches**

1. **A feature intersection is a positioning hypothesis, not a moat.**
   “Double-entry + investments + self-hosted web” describes the implementation
   well. It does not establish how often someone needs all three, how much
   inconvenience their present arrangement causes, or whether they will migrate.
   AI also makes that feature combination less expensive for competitors to
   assemble. The more defensible accumulation would be reliable migrations,
   understood failure cases, repeatable releases and a reputation earned through
   use. Those require continuing work and are not guaranteed advantages.

2. **“Daily driver first, differentiate later” can become endless parity work.**
   The roadmap compares Rekenraam with budgeting, forecasting, accounting and
   portfolio products at once. Matching every category would reproduce several
   teams' workloads. Reports, exports, CSV import, recurring entries and a useful
   forecast already exist. A tightly selected pilot can test a differentiated
   job before every desirable budget, receipt, crypto and return feature exists.
   Do not turn each competitor checkbox into a release requirement.

3. **The persona combines several markets.**
   An expat planning cash in two currencies is not automatically an active
   investor wanting tax-lot elections, a crypto accountant, or a former Money
   user migrating twenty years of history. Each has different import and support
   demands. “Cross-border” should initially mean accounts and cash obligations
   across currencies, not comprehensive jurisdiction-specific compliance.

4. **Investment integrity and investment completeness must remain distinct.**
   The immediate T-75a fence is valuable: it refuses unsafe generic mutations.
   That is not the same as giving an owner a complete way to correct a mistaken
   trade. T-75b is therefore an adoption issue for an investment-led pitch, not
   just technical debt. T-76 is already required before v0.1/schema freeze:
   durable provenance matters especially if the promise is explainable history.
   Do not present the current gains endpoint as reproducible historical or tax
   reporting. [Existing backlog](../backlog.md)

5. **Learning quality and ledger correctness are separate claims.**
   The R10 plan correctly keeps statistical estimates apart from facts. Exact
   arithmetic can still produce a poor prediction from incomplete or outdated
   history. Expat relocations, job changes and new accounts are particularly
   likely to make old spending patterns unrepresentative. Imported history
   increases model eligibility but also raises onboarding cost. The seasonal
   plan requires 36 complete months; many new users will not qualify.
   A transparent recent-average fallback can be a successful product result.
   [Learning contract](../plans/forecast-learning-plan.md)

6. **The “one-shot launch” framing risks delaying useful evidence.**
   A broad announcement deserves preparation. Private observations, limited
   pilots and a supported early release are different events. The repo's R2/R3/R5
   announcement prerequisites are already delivered; remaining gates should be
   explicit rather than extended whenever another feature becomes appealing.
   Keep the accepted token, signed-release and security gates; do not claim this
   review waives them. [Release gates](../roadmap.md)

7. **Longevity is not solved by open source alone.**
   Most owners cannot maintain a finance application just because its source is
   available. Recoverable data, documented formats, reproducible builds and
   supported upgrades are more immediate benefits. Today the conventions still
   say development databases are disposable, migrations may be rewritten, and
   there is no supported upgrade path. That is legitimate pre-release policy,
   but incompatible with inviting someone to keep their only durable financial
   record in the app. The v0.1 freeze and historical-upgrade tests are product
   work, not release cosmetics. [Lifecycle policy](../conventions.md)

**The competitive picture is less empty than the July comparison suggests**

| Verified observation | Implication for Rekenraam |
|---|---|
| Actual documents reconciliation and CSV/QIF/OFX/QFX/CAMT import. | Its missing reconciliation and QIF/OFX cells were inaccurate. Compete on the actual workflow and boundaries, not their supposed absence. |
| Actual offers optional end-to-end encryption for synchronized budget data, with separate caveats for bank-sync tokens and unencrypted local data. | “Keeps data private” is already a serious competitive axis; self-hosted server storage is a different trust model. |
| Actual's documentation still describes native multi-currency as unsupported, with an experimental rules workaround. | Proper source-currency accounting remains a concrete distinction worth testing. |
| PocketSmith has an official MCP integration covering financial data, budgets and forecasts. Its product page says access is opt-in and requested data then goes to the chosen AI provider. | Multi-currency finance plus AI access is not an unoccupied category. |
| Sure documents built-in or external assistants, local-model configuration and an MCP endpoint. | The Maybe successor should not be dismissed as merely an abandoned product's residue; optional local AI is not unique. |
| Lunch Money lists community MCP integrations on its developer page. | Users can already add AI to an established finance workflow without changing their core application. |
| Beancount.io offers MCP queries and ledger edits with previews and Git history. | Plain-text accounting plus an agent is a meaningful substitute for technical users. Beancount.io is a service; it is not synonymous with the Beancount project. |
| Portfolio Performance documents multi-currency accounts, transaction history, return analytics and portable files. | Keeping it beside Rekenraam may be rational. Do not assume every prospective user wants one app to replace all specialist tools. |

Primary sources, checked 2026-09-09:
[Actual reconciliation](https://actualbudget.org/docs/accounts/reconciliation/),
[Actual imports](https://actualbudget.org/docs/transactions/importing/),
[Actual encryption](https://actualbudget.org/docs/getting-started/sync/),
[Actual multi-currency](https://actualbudget.org/docs/budgeting/multi-currency/),
[PocketSmith MCP](https://developers.pocketsmith.com/docs/pocketsmith-mcp-server),
[PocketSmith AI disclosure](https://www.pocketsmith.com/ai/),
[Sure AI architecture](https://github.com/we-promise/sure/blob/main/docs/hosting/ai.md),
[Lunch Money developer tools](https://lunchmoney.app/developers),
[Beancount.io MCP](https://beancount.io/mcp), and
[Portfolio Performance](https://www.portfolio-performance.info/en/).

This is a targeted refresh, not a complete retest of the comparison matrix.
Claims such as “no competitor ships jurisdiction-aware gains,” “no OSS
equivalent,” and “users currently run Firefly + Ghostfolio + a spreadsheet”
should be treated as unproven hypotheses. The July migration-wave and price
claims also need fresh evidence before launch copy uses them. A vendor listing
a paid product does not prove willingness to pay for Rekenraam.

**AI changes the alternatives, but does not eliminate products**

There are at least three distinct uses of “AI manages my finance”:

- It explains recorded facts and helps formulate questions.
- It assists bookkeeping: mapping files, proposing categories, finding likely
  duplicates, or drafting recurring entries.
- It recommends or executes financial decisions.

The first two can fit Rekenraam's boundaries. The third adds a different product
promise; autonomous trades, payments and tax decisions are not an extension to
slip into this review.

A capable agent can help a user build a spreadsheet, generate a dashboard or
write a parser. It can also help maintain an existing accounting tool. Assume
those alternatives improve, rather than arguing that only Rekenraam can achieve
correctness. The buying question becomes: does Rekenraam reduce the user's
continuing work and uncertainty enough that assembling and maintaining an
alternative is unattractive?

A recurring financial record has work beyond the first script: duplicate
imports, transfer matching, amended statements, lifecycle corrections, stale
quotes, recovery, reproducible totals and years of upgrades. Rekenraam already
implements parts of this. That is useful accumulated work, not immunity from
competition.

One plausible future is that the user's preferred assistant becomes the main
interface while Rekenraam remains the record and calculation engine. Another is
that agents make existing plain-text tools sufficient for the most technical
users, while everyone else stays with bank-connected commercial apps. A third is
that privacy-conscious users mostly want clear screens and reliable import, with
no interest in chat. Test all three; do not require every customer to become an
AI user.

The strongest provisional positioning is:

> Keep a clear record of your money across currencies, on a system you control.
> Check it against your statements, understand upcoming commitments, and see
> where each number comes from. Use an assistant when you choose.

This is proposed positioning. It deliberately avoids promising automated tax
compliance, universal bank coverage, or complete investment corrections today.

**Privacy and AI should be independent choices**

| Mode | Data boundary | Status and product interpretation |
|---|---|---|
| Conventional UI with AI off | Ledger stays on the chosen Rekenraam host; enabled financial providers have their own network exchanges | The core must remain fully useful this way |
| Local statistical assistance | Calculations run in the Go process without an external model | R10 M1 foundation exists; M2–M4 and the public estimated-spending surface are still pending |
| Optional local language model | Selected data goes to a model endpoint the owner operates | Possible later; not implemented or a requirement for the core |
| Optional external assistant | Specifically authorized query results leave the host for the client/provider | Possible later; self-hosting does not make those disclosures local |
| Managed hosting | The app and readable financial records reside with the hosting operator | Separate business/deployment choice, not equivalent to end-to-end encrypted storage |

A privacy-conscious user is not necessarily a systems administrator. The
installation, update and restore journeys therefore deserve measurement alongside
finance features. Improve packaging and guidance within the accepted binary/web
shape first; there is no case here for reopening the archived native-app stack.

The current architecture computes on the server and defers SQLite database
encryption. TLS and encrypted provider credentials do not make the ledger opaque
to the host operator. Filesystem protection, device/storage encryption, backups,
and model disclosure answer different threats. Do not advertise “nothing leaves
your device” for a VPS deployment or when optional providers/assistants are enabled.
[Current deployment guidance](../deployment-security.md)

For future assistant access, let the owner grant a bounded account/date scope
and choose aggregates before raw transactions where useful. Explain what goes
to which endpoint and whether that endpoint is local or external. A local URL or
an “OpenAI-compatible” label alone is not evidence of local processing.
Merchant removal and aggregation reduce disclosure but do not make financial
data anonymous. Revocation stops future requests; it cannot retrieve copies
already sent.

These are proposed access requirements, not capabilities the app currently ships.

**Which customers to investigate first**

| Candidate | Recurring job | Why Rekenraam might win | Reason they may decline |
|---|---|---|---|
| Person with income, bills and savings in two or more currencies | Reconcile and plan cash without conflating currencies or transfers | Existing ledger, reporting and forecast fit | Import effort or lack of automatic bank coverage |
| Experienced Money/Quicken user | Preserve history and continue familiar financial review | Ownership, QIF, reconciliation and investments | Incomplete migration fidelity, correction gaps, installation burden |
| Privacy-conscious technical user | Maintain reliable records while choosing their own tools | Portable data and potentially bounded agent access | Actual or plain-text accounting plus an agent is already adequate |
| Investor primarily seeking performance analytics | Explain portfolio performance and allocations | Integrated brokerage cash and lots | Specialist tools may remain substantially better |
| Convenience-first budgeting user | Automatic categorization and household budgeting | Limited current fit | Single-user scope, self-hosting and BYO adapters create friction |

Start with the first group and recruit some members from the second and third.
Keep the investor-only and convenience-first groups as comparisons. “Household
finances” describes what the owner records; it must not imply shared logins,
partner accounts, advisor access or a multi-user product that does not exist.

**Product directions and their tradeoffs**

| Direction | Reuse of current work | Commercial possibility | Recommendation |
|---|---|---|---|
| Focused multi-currency finance workspace | Very high | Paid help, support or later convenience services if retention exists | Preferred first hypothesis |
| Broad Money/Quicken replacement | High, but many unfinished expectations | Large-looking category with substantial migration/support obligations | Keep as a long-term compatibility ambition, not the next release checklist |
| Finance engine accessed through API/MCP | High for calculations, less for UI | Developer sponsorship/support; uncertain standalone willingness to pay | Add access to the product after a concrete workflow wins |
| AI-first financial copilot | Moderate | Potential recurring usage, also direct incumbent competition | Do not pivot the whole app into this |
| Specialist investment/tax platform | High subledger reuse, major new domain burden | Could address expensive problems, but needs deeper expertise and support | Defer; no compliance claims |
| Migration/reconciliation utility alongside other apps | High import/export reuse | Paid migration assistance or a smaller standalone utility | Credible fallback if users value migration but do not retain the full app |

The last option is important. A failed all-in-one adoption test would not imply
all the work was wasted. Users might prefer Rekenraam to normalize and verify
records while keeping another budgeting or portfolio tool. That outcome should
be tested before attempting a broad platform rewrite.

**The best first assistant workflows**

Prioritize reducing bookkeeping effort and explaining an existing result:

1. **Import assistance.** Suggest column mappings, payee normalization and
   categories; show the source rows and the proposed interpretation in the
   existing staged review. Keep amount/date parsing and validation deterministic.
   Compare with reusable profiles and ordinary rules before adding a model.
2. **Explain a discrepancy.** Retrieve the report scope, reconciliation state,
   relevant entries and missing coverage. Distinguish an observed mismatch from
   a proposed cause. An LLM explanation must link back to the evidence.
3. **Explain cash commitments.** Ask which recorded or recurring events create
   a projected low balance in a particular currency. The existing R10 event
   details supply much of the foundation. Do not translate this into an
   unqualified “you can afford this.”
4. **Review proposed changes.** Later, a machine could prepare a recurring
   template or categorization proposal. The app must own review and commit.
   Existing recurring drafts demonstrate part of the pattern, but they do not
   create a general-purpose AI draft workflow automatically.

An evidence-rich monthly review could combine these without needing chat:
unreconciled accounts, import issues, known commitments and links to the entries
behind changes. This is a proposed summary experience, not an existing formal
period-close workflow.

**Where MCP fits**

An MCP server is an access mechanism for the same domain services, not a second
financial engine. Current authentication is browser sessions with CSRF;
personal-access tokens are an accepted announcement gate, not a shipped
capability. No active MCP implementation was found in the inspected API,
contracts and dependency manifest.
[Authentication](../../backend/internal/api/auth.go),
[OpenAPI](../../api/openapi/openapi.yaml),
[release gate](../roadmap.md).

If an experiment justifies it, the first adapter should expose a few read-only
questions: scoped balances, spending summaries, forecast explanations and
reconciliation status. Proposed tool contracts should carry:

- account and date scope, source currency and exact quoted quantities;
- the computation basis/version and current observation time;
- included/excluded data, known freshness and missing coverage;
- valuation assumptions and links or IDs for the underlying records;
- explicit pagination/completeness rather than a plausible partial total.

Reuse existing response fields and service logic; do not claim the current
reports already expose every proposed property or a durable historical snapshot.
Never ask the model to reconstruct canonical totals from a page of transactions.

Authorization should be enforced by the server on every query, including
aggregate queries and drill-down. A read-only tool list backed by an unrestricted
owner token is not a complete disclosure boundary. Imported descriptions and
documents are untrusted data, never instructions granting additional access.
Do not expose arbitrary SQL, shell execution, secrets, or full-database download
as convenience tools.

Keep writes out of the first experiment. A later proposal workflow needs a
persisted preview identity, expiry, a version/basis check at commit, explicit
owner approval, idempotency and the existing financial/audit guards. A model
claiming “the user approved” must not satisfy that approval. Investment proposals
would need their own subledger-aware lifecycle; they cannot bypass T-75b.

Do not build a separate chat UI, vector database or agent framework before
proving that these few operations materially outperform the existing UI and
existing-agent-plus-export baseline. A sanitized export can test some questions
before a network adapter exists, but is still a disclosure if sent externally.

**Recommended steering of the current roadmap**

The recommendation is concentration and a release boundary, not a rewrite.
The following are proposals; current accepted sequencing remains in force.

| Area | Proposed steering | Evidence or decision gate |
|---|---|---|
| R10 core | Preserve and demonstrate now | Already accepted; test whether people understand facts versus assumptions |
| R10 M2–M4 | Keep bounded local learning, its fallback and hardware/quality gates; start observational research alongside it | A more complex model ships only under its existing validation contract; measure eligibility and usefulness as well as error |
| R8 budgets | Plan a small per-currency planned-versus-actual workflow around the selected persona | Do not expand into every envelope, rollover and household model solely for parity |
| Import work | Improve repeated import/reconciliation friction discovered in pilots | Choose the next format/provider from repeated actual blockers; do not promise universal coverage |
| T-76, schema freeze and upgrades | Treat as trust/release obligations before durable v0.1 use | Existing T-76 and lifecycle policy; preserve all release gates |
| T-75b and R16 | Complete correction workflows before selling a broad investment replacement | Users must recover from realistic mistakes, not merely be refused unsafe edits |
| R11 price/FX UI | Consider pulling forward only the inspection/correction work needed to explain pilot results | Missing or stale observations may block the primary multi-currency job |
| R13 return analytics | Keep specialist-tool coexistence acceptable | Pull forward only if target users require it to retain the app |
| R17 crypto and R18 projections | Retain designs, avoid making breadth a prerequisite for initial evidence | Reordering accepted scope is a later owner decision; jurisdiction labels must not imply compliance |
| R14 receipts and wider connections | Defer broad expansion unless the chosen job repeatedly needs it | Each adds ingestion, privacy, backup and support obligations |
| Tokens / MCP | Fulfill the accepted token gate; separately test a small read-only adapter | MCP itself is not a release gate or validated business |
| Distribution | Measure installation and first successful reconciliation; prepare existing release assets | A good binary architecture does not establish easy onboarding |

A particularly valuable future comparison is a user-authored one-off forecast
scenario versus more elaborate seasonal modeling. A new rent or planned move
may matter more than the historical average. Scenarios need separate identity
and must not enter the ledger; they are explicitly out of current R10 scope.
Research the need first, rather than quietly inserting another implementation
into M2–M4.

**A bounded validation programme**

The following thresholds are proposed decision rules for a small study, not
market benchmarks or forecasts of success.

Recruit approximately eight people: four with recurring multi-currency needs,
two migrating from an older desktop tool, and two technical users already
comfortable with finance software and AI. Include people uninterested in AI and
people unwilling to self-host. Recruitment is future work; this review did not
contact anyone.

First observe a recent real task in their existing setup: importing a statement,
explaining a mismatch, checking a coming bill, or recording a brokerage event.
Ask what went wrong, how long the workaround took, what they currently pay and
what they refuse to disclose. Avoid “would you use an AI finance app?” as the
main evidence.

Run a paired task with the current Rekenraam flow and the person's best existing
alternative. For volunteers comfortable with AI, include an agent with a
sanitized export or existing competitor tools. Preserve their original records.
Under today's disposable-database policy, prototypes use reproducible copies
and must not become the sole record. Longitudinal reliance starts only with the
supported-release/upgrade boundary.

Useful proposed gates:

- At least five of eight finish a first scoped import and reconciliation in
  roughly 45 minutes, with any founder help explicitly recorded.
- At least four repeat the workflow independently on another statement and
  choose to continue over two monthly review cycles.
- No known financial mismatch is left unexplained. A balanced book is not a
  pass if it omitted transactions or the participant cannot identify its scope.
- Participants can distinguish recorded, recurring and estimated amounts
  without coaching. Confusion is a design failure even when calculations pass.
- For AI-assisted tasks, compare corrections, omitted records, time and
  disclosure against ordinary rules/UI. Faster prose alone is not a win.
- For forecasting, record eligible versus excluded data, sparse-history
  failures, relocation/regime changes and baseline comparison. Short-horizon
  backtests do not establish 90-day or annual accuracy.
- Test restore and one supported-version upgrade on a representative copy
  before claiming dependable continuity.
- For a commercial path, seek a few actual paid commitments for a clearly
  specified service and track support time. Expressions of interest or stars
  do not count as revenue evidence.

Record anonymous task outcomes with consent, not raw financial statements in
the repo or application logs. Eight participants cannot size the market; they
can expose a poor workflow or an incorrect persona cheaply.

Decide based on failure mode. If installation blocks use, address deployment
before adding finance features. If repeated import is the problem, improve the
pipeline. If only migration has value, investigate the utility/service option.
If the existing app plus an agent is just as good, make Rekenraam interoperate
or reduce ambition rather than adding a chatbot to recover attention.

**A product can survive without a large subscription business**

For personal utility, continued development is justified when the app reliably
solves the owner's actual work at an acceptable maintenance cost. Community
adoption and a business are separate objectives.

For open-source adoption, a narrowly documented use case, supported releases,
portable records and small useful integrations could be enough. Do not infer
funding from GitHub popularity or the availability of an AGPL license.

For a business, first test services compatible with the current ownership model:
fixed-scope migration/setup assistance and optional maintenance/support.
Sensitive statements can remain on the owner's machine; agree explicitly what
a support session reveals. Paid support must be priced against real hours,
provider breakage and release obligations. It may prove a viable small business
or an unattractive consultancy; that is unknown.

A useful recurring-value equation is:

> Revenue per customer minus payment/distribution costs, model or provider costs,
> and support/update effort valued at a sustainable rate.

A one-time migration fee does not establish a recurring subscription business.
A privacy-preserving local product may need no paid cloud AI at all. Managed
hosting could be tested later for people who want ownership/export but not
operations, while being candid about operator access. Full encrypted hosted
computation would be a different architecture, not a marketing adjustment.

Do not withhold export, restore or financial integrity to manufacture a paid
tier. Do not commit to lifetime support on the assumption that AI makes future
maintenance free. Broader licensing decisions require their own review; this
analysis changes none.

**Decisions this review proposes**

- Keep the Go/SvelteKit/SQLite application and its correctness boundaries.
- Lead with reconciled multi-currency records and cash planning; use the wider
  Money successor ambition to inform compatibility, not unlimited next-release
  scope.
- Preserve a complete non-AI experience. Keep local statistical assistance
  bounded and treat local/external language-model access as optional.
- Start task-based adoption research before adding more competitive breadth.
- Establish supported continuity before asking owners to rely on the app for
  irreplaceable records.
- Test read-only MCP only as a way to improve a specific workflow.
- Reconsider broad investment, crypto or tax positioning if the corresponding
  import, correction and provenance requirements cannot be supported.

None of these recommendations requires throwing away the application. Their
purpose is to spend the next development effort where it can establish a
repeatable user benefit—and to make changing direction possible before another
large set of features becomes a reason to postpone that evidence.
