# Bug classes — Any bug fix, refactor or test

Part of the `validate-and-ship` review checklist. Read for every bug fix,
helper consolidation or new test. Every entry shipped as a real bug in this
repo. Numbers are stable IDs cited from code comments and docs; they are not
an order and are never reused.

## 11. Builder-output tests that never reach the real consumer

A test that checks a spec-building function's *return value* in isolation
(e.g. `buildTransactionSpec`) is not the same as proving it survives the
consumer's real validation. `EntryKind: "main"` sat in `buildTransactionSpec`
since the import feature's first commit — invalid per `entryKinds`, so every
`CommitImportBatch` call failed the instant it reached
`TransactionService.CreateTransaction` for real — undetected because no test
drove a staged row through the *actual* commit path against a real account
(T-22). When testing a function that produces input for another service, add
at least one test that calls the consumer for real, not just asserts on the
producer's output shape.

## 13. A duplicated helper is only as fixed as its least-visited copy

The decimal-comma 100x error has now shipped **three** times from the same
two-line pattern, `input.replace(/,/g, '')` before parsing, which reads `1,50`
as 150. Fixed on the import side (T-36), then in the transaction editor and
reconcile form (T-45), and it was *still live* in all three investment forms
four months later (T-47) because the survey that scoped T-45 treated those
forms as a later slice. The fix each time was correct; the **sweep** was what
failed. So: when fixing a helper that exists in more than one place, grep the
whole tree for the *pattern* before declaring it done, not just the copies the
current ticket names — and count what you find, because the T-47 survey said
two copies and there were seven. The same holds across a **pipeline**: #300
had the adapter apply the file's declared separator and commit re-read
rejected text with per-value detection. A value rejected at staging is
recorded on the row and refused at commit, never re-parsed
(`TestImportAuditRejectedConventionRowsAreFlaggedAndNeverCommitted`). Tests
repeat it: `10a03db7` made two self-check helpers assert the whole run and
missed four other helpers and six inline lists, so 114 call sites never
asserted checks outside a short named list, such as #389's
`investment_attribution` (#397). Diagnostics repeat it too: Prices judged a
quote by age while valuation also refuses it across conflicting splits, so
Home's "needs a price" link hid it (#476). A screen that says whether a
consumer can use something asks that consumer (`RateTable.DirectGap`), never a
restated rule. A healthy known-basis book asserts through
`requireKnownBasisSelfCheckPasses` (or `requireInvestmentSelfCheckPasses`,
which adds the investment oracles); never hand-list the checks that must pass.
Unknown-basis books use `requireHealthyUnresolvedBook`, and damage tests name
the check they expect to fail.

## 14. Consolidation that silently widens what is accepted

Retiring a private helper onto a shared one is a behaviour change unless
proven otherwise. The investment forms' parsers rejected a leading `-` only as
a *side effect* of running `/^\d+$/` over the concatenated coefficient;
`parseDecimalAmount` handles signs properly, so a like-for-like swap would
have started accepting negative share quantities (T-47). Ask of any such swap:
what did the old code reject *incidentally* that the new code accepts? Then
make the rejection explicit and named, and test it where the behaviour changed
— per call site, not once on the shared module.

Corollary: if the call sites are `.svelte` files, that test is impossible in
place. This project has **no component-test harness** (no `testing-library`,
no `jsdom`; vitest runs plain `.ts` only), so the validation has to be
extracted to a module first. That is a feature, not an obstacle — it is the
same reason G-02 existed.
