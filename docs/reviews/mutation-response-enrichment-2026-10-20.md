# Investment and reconciliation response enrichment — 2026-10-20

Delivery for [#370](https://github.com/sergeyfarin/rekenraam/issues/370), found
by [#349](https://github.com/sergeyfarin/rekenraam/issues/349); parent
[#260](https://github.com/sergeyfarin/rekenraam/issues/260).

## Result and source evidence

Mutation responses now carry the same posting account/commodity display fields
as canonical transaction reads, including inverse/replacement pairs and bulk
reconciliation-status changes. The journal, lot, correction, reconciliation,
pricing and import-identity writers are unchanged. Empty tag arrays and explicit
null display/unknown-knowledge fields retain their contracts. Journal-free
operations still return the supported null transaction.

Baseline: `e2fa0b0f478f32096b52d414a466f3f84418ad66`. The final source SHA,
installed format checkers, exact selected tests and counts are recorded in
[summary.json](evidence/mutation-response-enrichment-2026-10-20/summary.json).
The later publication commit adds this report, evidence and current-state docs.

The expanded committed-source audit records **1,108 actual HTTP responses**,
**165 of 332 declared operations**, 809 operation/status/test/error rows and
all eight schema sensitivity controls. **All fifteen original #370 failure rows
are repaired, and the #370 exception is removed from the runner.** The only
remaining failure is the known setup-owner HTTP 413 declaration under #350.
There are zero untracked discrepancies. The remaining **167 operations** and
unobserved declared statuses remain explicitly unverified.

- [Producer inventory](evidence/mutation-response-enrichment-2026-10-20/producers.csv)
  records 48 enrichment boundaries and four internal receipt/pure-conversion
  dispositions. Aliases and existing paths are explained below.
- [Complete operation inventory](evidence/mutation-response-enrichment-2026-10-20/operations.csv)
  retains declared, observed and unverified statuses.
- [Response/status/test/shape matrix](evidence/mutation-response-enrichment-2026-10-20/responses.csv)
  records every captured response group, exact consumer and schema disposition.
- [Schema controls](evidence/mutation-response-enrichment-2026-10-20/controls.csv)
  preserve independent requiredness, nullability, exact-number, PATCH and error
  payload/schema mutations from #349.

## Consumer inventory and implementation

`TransactionService.enrichPostings` remains the sole display owner: one bulk
account lookup and one bulk commodity lookup, independent of posting count.
Singles use the existing `enrichOne`. `enrichRecords` converts all returned
journal records and passes them through that same owner together. It does not
write or reload a transaction, calculate amounts, or call per-posting services.

| Family / consumers | Disposition |
|---|---|
| Buy, sell, short sale, short cover, reinvested dividend | Enriched at the public service response boundary. |
| Write-off, cash in lieu | Public wrappers enrich the shared disposal receipt; reversal/replacement share the hydrated sale-correction family. |
| Ordinary cash dividend, withholding, payment in lieu | Existing dividend/payment paths already enrich; separate withholding shares dividend writer/enrichment. Dividend/payment replacement pairs now batch both journals instead of performing four lookups. |
| Buy/sale/short/reinvestment reversal and replacement | All single inverse journals and both replacement journals are enriched. Short-cover correction shares disposal correction. Source-linked buy/sale corrections share the corrected pair boundaries. |
| Internal, external inbound and external outbound transfers | Entry, shared reversal and replacement responses are enriched. New outbound replacement/reversal HTTP coverage supplements the prior transfer journeys. |
| Basis resolution, reversal, replacement and provenance-only correction | Enrich the returned journal set before optional conversion; zero journals remain nil, with zero display queries. Existing provenance-only audit/source semantics remain. |
| Split, operation-addressed split correction, journal-free split | Single and compound result journals are enriched; operation identity, nullable transaction and plan are unchanged. |
| Return of capital, exchange, spin-off, cash-and-stock merger | Entry, reversal and replacement responses all use the same owner. Calculation/plan conversions and guards are unchanged. |
| Reconciliation posting-status batch | All returned transactions are enriched together; no per-transaction metadata lookup loop. Session/checkpoint DTOs contain no transaction posting display payload and need no enrichment. |
| Trading 212, ordinary investment CSV and crypto CSV internal buy/sell receipts | Deliberately consume only the atomic writer's durable IDs. Their private `buyWithPostWrite` / `sellWithPostWrite` paths do not request optional display data after commit. Public Buy/Sell/WriteOff/CashInLieu boundaries supply it for UI consumers. |
| Import source correction and dividend repair | Transaction-returning owner-review/source-correction results use existing or newly batched enrichment. Commit identity/source revision callbacks still run inside their original writer transaction. |
| Ordinary transaction CRUD, lifecycle, ordering, list/deleted/register reads | Existing owners already enrich; no production change required. Preview planners and input validation use pure conversion without pretending to return posted response DTOs. |
| Provider suggestion acceptance and workers | They return suggestion/status/identity data, not a posting response; existing financial writer, retry and source-marker behavior remains. |

The inventory covers direct conversions and delegating wrappers. A raw
`toTransaction` remaining inside the private import receipt/validation path is
intentional; it is not a missing public DTO boundary.

## Named regression and failure evidence

| Control | Before / independent expectation | Final result |
|---|---|---|
| `TestMutationResponseEnrichmentBuyMatchesCanonicalRead` | Baseline mutation posting `account_class` is empty, failing the independent five-class assertion; canonical GET contains display facts. | Four actual buy postings retain exact quantity strings and valid USD/instrument codes; wire display fields match GET. |
| `TestMutationResponseEnrichmentReconciliationBatchMatchesCanonicalReads` | Baseline batch returns empty classes on both touched transactions. | Both transactions' display facts, tags and exact coefficients match canonical reads. |
| Shared HTTP mutation assertion in `doInvestmentRequest` | Every successful transaction-shaped POST result is compared independently with the real canonical GET. Existing financial assertions still consume the untouched body. | Entry, reversal, replacement, transfer, structural action, null-basis and correction journeys pass; display fields are present even when null, and tags remain arrays. |
| `TestTransactionResponseEnrichmentBatchesCompoundAndRepeatedJournals` | Independent native authorizer observes the summary read columns, not a mock repository. | Two and 100 journals each perform one account and one commodity batch read. A zero journal performs neither lookup and stays nil. |
| `TestMutationResponseEnrichmentFailurePreservesCommittedBuyWithoutRetry` | Baseline returns incomplete HTTP 201 rather than reaching either denied post-commit display lookup. | Denying the account or commodity lookup after COMMIT returns the sanitized 500 envelope. Exactly one transaction and one lot remain; canonical GET succeeds after removing the denial. |
| `TestMutationResponseEnrichmentFailurePreservesCompoundCorrection` | A response failure must not rerun either half or imply writer rollback. | One correction audit, source/inverse/replacement transactions and the corrected remaining quantity of three persist exactly once; both result journals are readable after restoring metadata access. |
| `TestInvestmentImportWriterDoesNotDependOnResponseEnrichment` | A candidate placing enrichment inside the private import writer failed after the receipt committed. This caught an introduced regression before delivery, not a separate baseline defect. | Public-only trade enrichment keeps the real internal writer callback/receipt successful when optional post-commit metadata reads are denied. The receipt transaction ID and single committed journal agree. |
| `TestMutationResponseEnrichmentExternalOutReplacementAndReversal` | Inventory required outbound replacement/reversal coverage beyond the original entry test. | Real entry, corrected outbound pair and reversal all compare with canonical reads. |
| `TestCapitalReturnReplacementAPIPreviewMatchesCommitAndFencesKind` (#372) | Its new checkpoint-list assertion fails before the small fix: nil/null. | The correction context emits [] with override false; incoming request bound-set validation is unchanged. |
| Expanded schema audit (#371) | The faulted real replace-buy response is HTTP 500 with the shared error envelope, but the operation has no declared 500. | The operation and generated response union now declare it; actual fault response validates. |

## Error and financial semantics

Enrichment happens after the existing domain writer commits, as with ordinary
TransactionService writes. If a response lookup fails, the service returns the
existing sanitized INTERNAL_ERROR; it does not return an incomplete successful
DTO, claim rollback, silently retry, or append another financial operation.
The caller refreshes canonical transactions/correction-chain reads before
choosing another mutation. A newly created transaction can be recovered through
the normal transaction list. No new committed-write receipt/idempotency protocol
is introduced. This existing ambiguous-error boundary is explicitly documented
on the reproduced replace-buy 500 response (#371).

Private import receipts are different consumers: they need committed IDs to
settle staged-row/source identity, not optional UI metadata. Keeping enrichment
at the public trade boundary avoids creating a post-commit import failure or
cleanup attempt from a display lookup. The callback/identity remains inside the
same writer transaction. The import regression exercises that real callback;
it is not claimed as an exhaustive fault matrix for every provider path.

## Small findings

- **#371 fixed:** reproduced replace-buy 500 declaration and generated client.
- **#372 fixed:** explicit empty override checkpoint array only in the
  capital-return read-side correction terms. Request decoding/guard distinctions
  between missing and explicitly reviewed sets are untouched.
- **#350 remains open:** the independent capped-JSON HTTP 413 contract scope.

No further independent audit or unresolved defect was identified within #370's
bounded consumer inventory. No owner/provider prerequisite blocks closure.
#260 remains open for its separately tracked work and release prerequisites.

## Validation and limits

Final validation:

| Command / evidence | Result |
|---|---|
| `./scripts/test-backend.sh` | Format, vet and all native race packages pass: app 222.803s, API 133.862s, command 76.289s; unchanged DB/other packages use valid cached results. |
| Focused service/API controls | Query-bound, import receipt, buy/reconciliation, compound failure and outbound correction cases pass; capital-return checkpoint assertion has recorded failing-before/passing-after evidence. |
| `./scripts/test-frontend.sh` | Generated client, zero Svelte errors/warnings, 66 catalog cases and 691 unit cases pass. |
| Committed-source schema runner at `864e37efd4fb134e8a48edc9d0da5616f7f33325` | 1,108 responses, 165 operations, eight controls, zero untracked discrepancies; only #350 remains. |
| Source/evidence review | Matrix counts/dispositions, local report links and `git diff --check` pass. |

The final full backend gate follows all implementation/test repairs. An earlier
run caught a test fixture incorrectly expecting EUR instead of its configured
USD; that assertion was corrected, without changing production behavior or
weakening the actual metadata/canonical-read checks. Exact reference-book, subledger,
reconciliation, source/import-identity and self-check assertions remain in the
existing suites; no financial writer assertions are weakened.

This is synthetic, provider-free evidence. It does not certify every operation,
status, error interleaving or live-provider wire format. The retained #349 format
limits still apply: installed checker names are recorded, and RFC 3339 date-time
checking is absent in this environment. There is no repeated browser, release,
large-book or owner-deployment claim for this response repair. Remote CI is
pending at push, not a remote-green claim.
