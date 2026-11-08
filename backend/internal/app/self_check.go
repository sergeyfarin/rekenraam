package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// The ledger self-check: does this book still hold together?
//
// Read-only and diagnostic, deliberately. It reports, names, and explains; it
// never repairs. A check that could change a balance to make itself pass would
// be a different feature, and the wrong one — a book that has gone wrong needs
// a person to look at it, not a program to tidy the evidence away.
//
// Every total here is folded in Go through exact.ScaledInt. Coefficients are
// strings in SQLite, so SUM() over them would be a float in disguise
// (ledger-invariants).

// Check identifiers, stable because a screen, a history, and a support
// conversation all name them.
const (
	CheckEntryBalance           = "entry_balance"
	CheckTransactionBalance     = "transaction_balance"
	CheckBookBalance            = "book_balance"
	CheckVersionIntegrity       = "version_integrity"
	CheckLotReconciliation      = "lot_reconciliation"
	CheckCommodityPositionSign  = "commodity_position_sign"
	CheckCheckpointIntegrity    = "checkpoint_integrity"
	CheckAccountVersionCoverage = "account_version_coverage"
	CheckInvestmentFoundation   = "investment_foundation"
	CheckInvestmentAttribution  = "investment_attribution"
	CheckInvestmentReplay       = "investment_replay_equivalence"
	CheckPriceDependencies      = "price_dependency_integrity"
	CheckSQLiteIntegrity        = "sqlite_integrity"
	CheckAttachments            = "attachments"
)

// Result and run statuses (#313, #389; data-portability-plan.md, "Self-check
// outcomes"). Completed results/runs take failed over warning over incomplete
// over passed; errored means execution itself did not finish. Warning and
// unavailable counts stay visible even alongside an integrity failure.
const (
	SelfCheckPassed        = "passed"
	SelfCheckWarning       = "warning"
	SelfCheckIncomplete    = "incomplete"
	SelfCheckFailed        = "failed"
	SelfCheckErrored       = "errored"
	SelfCheckNotApplicable = "not_applicable"
)

// SelfCheckResult is one check's verdict, in the terms a person needs: what was
// checked, what was found, where to look, and what to do about it.
type SelfCheckResult struct {
	CheckID      string
	Status       string
	FindingCount int64
	Sample       []int64
	// UnavailableCount and UnavailableSample name what the check could not
	// verify because the information is unknown — never counted as findings.
	UnavailableCount  int64
	UnavailableSample []int64
	WarningCount      int64
	WarningSample     []SelfCheckWarningSample
	Summary           string
	// Explanation and NextStep are not stored: they belong to the check, not to
	// a particular run, and freezing them into old rows would let history
	// contradict the current wording.
	Explanation string
	NextStep    string
}

// Warning samples identify advisory attribution contradictions, never
// integrity findings. Reason codes are stable; the UI translates their copy.
type SelfCheckWarningSample struct {
	TransactionID int64  `json:"transaction_id"`
	Reason        string `json:"reason"`
}

// SelfCheckRun is a whole run.
type SelfCheckRun struct {
	ID               int64
	Trigger          string
	Status           string
	FailedCheckCount int64
	// IncompleteCheckCount counts results with unavailable coverage, including
	// failed ones, so a failed run still discloses what it could not verify.
	IncompleteCheckCount int64
	WarningCheckCount    int64
	ErrorSummary         string
	StartedAt            string
	FinishedAt           string
	Results              []SelfCheckResult
}

type SelfCheckService struct {
	settings   *SettingsService
	repository *db.SelfCheckRepository
	now        func() time.Time
}

func NewSelfCheckService(repository *db.SelfCheckRepository) *SelfCheckService {
	return &SelfCheckService{repository: repository, now: time.Now}
}

func (s *SelfCheckService) SetNowForTest(now func() time.Time) { s.now = now }

// checkNarrative is the fixed wording for a check: what it means and what to do
// when it fails. Kept beside the code that produces the finding so the two
// cannot drift.
type checkNarrative struct {
	explanation string
	nextStep    string
}

var checkNarratives = map[string]checkNarrative{
	CheckEntryBalance: {
		explanation: "Every journal entry must sum to zero within each commodity. This is what double entry means here.",
		nextStep:    "Open the named transactions and look at their postings. An entry that does not balance was not written by this app's normal paths.",
	},
	CheckTransactionBalance: {
		explanation: "Each posted transaction must sum to zero within each commodity across all of its entries.",
		nextStep:    "Open the named transactions. If their individual entries balance but the transaction does not, the entries were changed independently.",
	},
	CheckBookBalance: {
		explanation: "Across the whole book, every commodity's postings must sum to zero — the sum of all balanced entries is balanced.",
		nextStep:    "If the entry and transaction checks passed and this one did not, the difference is arithmetic, not structure: report it, do not edit around it.",
	},
	CheckVersionIntegrity: {
		explanation: "Transactions, their versions, entries, and postings must all belong to each other: a posting's entry must be part of the same version, and a transaction must have at least one version.",
		nextStep:    "The named rows are structurally orphaned. They cannot be fixed from the app; keep a backup before anyone tries anything else.",
	},
	CheckLotReconciliation: {
		explanation: "Open investment lots must account for exactly what the holding account holds. Every original disposal and replay revision must allocate its full quantity, basis and signed proceeds, with positive allocated quantities and nonnegative basis. Unknown projected basis is missing information, not damage: basis reconciliation for those lots is unavailable, so the check is incomplete rather than passed, while quantities and every other comparison are still checked.",
		nextStep:    "Inspect the named disposal snapshot when allocation totals disagree; self-check does not repair its evidence. For a position mismatch, compare the named account's holdings with its lots. For inconsistent projected basis, preserve the named lot's source evidence before investigating its projection. Unknown basis needs no repair: record the basis from your broker's statement through a basis resolution when you have it, and verification becomes complete.",
	},
	CheckCommodityPositionSign: {
		explanation: "A negative number of countable units needs an explicit explanation: it may be an out-of-order entry, an error, or a short position. Open named short lots explain a holding's negative quantity up to the units they owe on each date; any further negative coin or share quantity is unclassified and needs review. Negative money is normal. The commodity-trading clearing account is excluded because it carries the other half of each movement.",
		nextStep:    "Look at the named accounts' dated registers for that commodity. A later acquisition can restore today's balance without correcting an earlier negative position. Usually a disposal was entered before the acquisition that covers it, or one was entered twice.",
	},
	CheckCheckpointIntegrity: {
		explanation: "An active reconciliation checkpoint must still add up to the statement balance it recorded, from postings that are still current.",
		nextStep:    "Re-reconcile the named account. A checkpoint over superseded postings is asserting a balance the ledger no longer supports.",
	},
	CheckAccountVersionCoverage: {
		explanation: "Every posting must have a version of its account that was in effect on the posting's date.",
		nextStep:    "The app refuses to create these, so a non-zero count means rows arrived from outside it — a backfill, a manual repair, or a restored and patched database. Exports have been quietly falling back to each account's earliest version for these rows.",
	},
	CheckInvestmentFoundation: {
		explanation: "Named investment operations must link their posted versions, source lots, effects and journal-backed cash components. Disposal decisions and each shared cost-currency clearing posting must independently conserve their attributed proceeds. Saved gains runs must retain their original input/output digests and matching descriptors; they are checked as documents, never recomputed against current history. A completed setup also needs its external investment transfer equity account and commission default.",
		nextStep:    "Review the named operation, disposal decision, journal, lot or saved gains run and the setup accounts. Preserve the original rows before correcting any mismatch or missing link.",
	},
	CheckInvestmentAttribution: {
		explanation: "Payment-in-lieu attribution is checked against historical short quantities, and fractional-disposal parents against current explanatory context. Dividend payer attribution is compared with retained provider claims; a different known payer warns, while an unassigned payer has unavailable support. Manual unassigned cash remains visible in income coverage without asserting share entitlement. None of these diagnostics authorizes cash or indicates financial corruption.",
		nextStep:    "Open the named payment and compare its attribution with the broker statement. The diagnostic never changes cash or source evidence. A saved self-check describes its recorded snapshot; rerun after a correction.",
	},
	CheckInvestmentReplay: {
		explanation: "Commands replay only the investment positions they affect. This check replays every long and short position from its effective trades, transfers, splits and elections in a rolled-back simulation and compares lot balances, each disposal's allocations, split effects, carried transfer basis and the method lock with what is stored, exactly and per cost currency. A stored result that history no longer produces means a command skipped a replay it needed.",
		nextStep:    "Note the named lots, disposal decisions or operations and keep a backup. Do not edit the projection by hand: re-entering a correction through the app replays the position, and a refused replay names the operation history no longer supports.",
	},
	CheckPriceDependencies: {
		explanation: "A derived price records which observations it was computed from twice: readably in its derivation, and as indexed links that voiding follows. The two must agree, and a derived price still in use must not depend on a voided observation.",
		nextStep:    "Void the named derived observations: a voided source means their value came from a retired number. A disagreement between derivation and links means rows were written outside the app; keep a backup before anything else.",
	},
	CheckSQLiteIntegrity: {
		explanation: "The database file itself must pass SQLite's integrity_check and foreign_key_check.",
		nextStep:    "Stop the app and restore the most recent verified backup. A failure here is the file, not the bookkeeping.",
	},
	CheckAttachments: {
		explanation: "Attachments are stored outside the database, so their integrity is a separate question from the ledger's.",
		nextStep:    "Nothing to do: attachment storage is not implemented yet. This slot exists so that when it is, the check appears here rather than being remembered later.",
	},
}

// RunSelfCheck executes every check against one snapshot and records the
// outcome. The run record is the only thing it writes.
func (s *SelfCheckService) RunSelfCheck(ctx context.Context, trigger string) (SelfCheckRun, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return SelfCheckRun{}, referenceErr
	}

	if trigger != "manual" && trigger != "scheduled" {
		return SelfCheckRun{}, ValidationError{Message: "self-check trigger must be manual or scheduled"}
	}

	startedAt := s.now().UTC().Format(time.RFC3339)
	runRecord, err := s.repository.CreateSelfCheckRun(ctx, BookID, trigger, startedAt)
	if err != nil {
		return SelfCheckRun{}, err
	}

	results, err := s.executeChecks(ctx)
	if err != nil {
		return SelfCheckRun{}, s.recordRunError(ctx, runRecord.ID, err)
	}

	stored := make([]db.SelfCheckResultRecord, 0, len(results))
	for index := range results {
		result := &results[index]
		result.Status = resultStatus(*result)
		sample, marshalErr := json.Marshal(emptyIfNilIDs(result.Sample))
		if marshalErr != nil {
			return SelfCheckRun{}, s.recordRunError(ctx, runRecord.ID, fmt.Errorf("encode self-check sample: %w", marshalErr))
		}
		unavailableSample, marshalErr := json.Marshal(emptyIfNilIDs(result.UnavailableSample))
		if marshalErr != nil {
			return SelfCheckRun{}, s.recordRunError(ctx, runRecord.ID, fmt.Errorf("encode self-check unavailable sample: %w", marshalErr))
		}
		warnings := result.WarningSample
		if warnings == nil {
			warnings = []SelfCheckWarningSample{}
		}
		warningSample, marshalErr := json.Marshal(warnings)
		if marshalErr != nil {
			return SelfCheckRun{}, s.recordRunError(ctx, runRecord.ID, marshalErr)
		}
		stored = append(stored, db.SelfCheckResultRecord{
			RunID:                 runRecord.ID,
			CheckID:               result.CheckID,
			Status:                result.Status,
			FindingCount:          result.FindingCount,
			SampleJSON:            string(sample),
			UnavailableCount:      result.UnavailableCount,
			UnavailableSampleJSON: string(unavailableSample),
			WarningCount:          result.WarningCount,
			WarningSampleJSON:     string(warningSample),
			Summary:               result.Summary,
		})
	}

	status, failed, warning, incomplete := aggregateSelfCheck(results)
	finishedAt := s.now().UTC().Format(time.RFC3339)
	if err := s.repository.SaveSelfCheckResults(ctx, runRecord.ID, status, failed, incomplete, finishedAt, stored); err != nil {
		return SelfCheckRun{}, s.recordRunError(ctx, runRecord.ID, err)
	}

	return SelfCheckRun{
		ID:                   runRecord.ID,
		Trigger:              trigger,
		Status:               status,
		FailedCheckCount:     failed,
		IncompleteCheckCount: incomplete,
		WarningCheckCount:    warning,
		StartedAt:            startedAt,
		FinishedAt:           finishedAt,
		Results:              results,
	}, nil
}

// resultStatus derives a completed check's status from its counts, so a check
// cannot report unavailable coverage as passed, or call a finding incomplete.
// not_applicable is kept: it is a reserved slot, not a measurement.
func resultStatus(result SelfCheckResult) string {
	switch {
	case result.Status == SelfCheckNotApplicable:
		return SelfCheckNotApplicable
	case result.Status == SelfCheckFailed || result.FindingCount > 0:
		return SelfCheckFailed
	case result.WarningCount > 0:
		return SelfCheckWarning
	case result.UnavailableCount > 0:
		return SelfCheckIncomplete
	default:
		return SelfCheckPassed
	}
}

// aggregateSelfCheck applies the run precedence: any failed result fails the
// run, otherwise warnings precede incomplete coverage. The incomplete
// count includes failed results that also have unavailable coverage.
func aggregateSelfCheck(results []SelfCheckResult) (string, int64, int64, int64) {
	var failed, warning, incomplete int64
	for _, result := range results {
		if result.Status == SelfCheckFailed {
			failed++
		}
		if result.UnavailableCount > 0 {
			incomplete++
		}
		if result.WarningCount > 0 {
			warning++
		}
	}
	switch {
	case failed > 0:
		return SelfCheckFailed, failed, warning, incomplete
	case warning > 0:
		return SelfCheckWarning, failed, warning, incomplete
	case incomplete > 0:
		return SelfCheckIncomplete, failed, warning, incomplete
	default:
		return SelfCheckPassed, failed, warning, incomplete
	}
}

func (s *SelfCheckService) recordRunError(ctx context.Context, runID int64, runErr error) error {
	finishedAt := s.now().UTC().Format(time.RFC3339)
	// The request may have been cancelled, but the durable run must still stop
	// saying "running". WithoutCancel preserves values while allowing this
	// small bookkeeping write to finish.
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.repository.MarkSelfCheckRunErrored(recordCtx, runID, finishedAt, runErr.Error()); err != nil {
		return errors.Join(runErr, err)
	}
	return runErr
}

// RecoverInterruptedRuns closes out any run left `running` by a process that
// did not survive to finish it. T-71 made every in-process error path durably
// finish the run it started, but that code only runs if the process is still
// alive to run it — a crash, kill, or power loss between CreateSelfCheckRun
// and any later write leaves the row `running` forever, exactly as invisible
// to LatestSelfCheck as before T-71. Call this once at startup, before
// anything can start a new run: a run cannot outlive the process that started
// it, so every `running` row a fresh process finds is stale by definition.
func (s *SelfCheckService) RecoverInterruptedRuns(ctx context.Context) (int64, error) {
	finishedAt := s.now().UTC().Format(time.RFC3339)
	return s.repository.RecoverInterruptedSelfCheckRuns(ctx, BookID, finishedAt,
		"self-check did not finish: the process running it stopped before it could record a result")
}

// LatestSelfCheck returns the last finished run, or a zero run when the book has
// never been checked. "Never checked" is a legitimate answer and is not an
// error: it is what a screen says before the first nightly backup chains one.
func (s *SelfCheckService) LatestSelfCheck(ctx context.Context) (SelfCheckRun, bool, error) {
	runRecord, resultRecords, err := s.repository.LatestSelfCheckRun(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return SelfCheckRun{}, false, nil
		}
		return SelfCheckRun{}, false, err
	}

	run := SelfCheckRun{
		ID:                   runRecord.ID,
		Trigger:              runRecord.Trigger,
		Status:               runRecord.Status,
		FailedCheckCount:     runRecord.FailedCheckCount,
		IncompleteCheckCount: runRecord.IncompleteCheckCount,
		WarningCheckCount:    runRecord.WarningCheckCount,
		ErrorSummary:         runRecord.ErrorSummary,
		StartedAt:            runRecord.StartedAt,
		FinishedAt:           runRecord.FinishedAt.String,
	}
	for _, record := range resultRecords {
		var sample, unavailableSample []int64
		var warningSample []SelfCheckWarningSample
		_ = json.Unmarshal([]byte(record.SampleJSON), &sample)
		_ = json.Unmarshal([]byte(record.UnavailableSampleJSON), &unavailableSample)
		_ = json.Unmarshal([]byte(record.WarningSampleJSON), &warningSample)
		narrative := checkNarratives[record.CheckID]
		run.Results = append(run.Results, SelfCheckResult{
			CheckID:           record.CheckID,
			Status:            record.Status,
			FindingCount:      record.FindingCount,
			Sample:            sample,
			UnavailableCount:  record.UnavailableCount,
			UnavailableSample: unavailableSample,
			WarningCount:      record.WarningCount,
			WarningSample:     warningSample,
			Summary:           record.Summary,
			Explanation:       narrative.explanation,
			NextStep:          narrative.nextStep,
		})
	}

	return run, true, nil
}

func (s *SelfCheckService) executeChecks(ctx context.Context) ([]SelfCheckResult, error) {
	snapshot, err := s.repository.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = snapshot.Rollback() }()

	balances, err := s.balanceChecks(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	structural, err := s.versionIntegrityCheck(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	coverage, err := s.accountVersionCoverageCheck(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	investmentFoundation, err := s.investmentFoundationCheck(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	attribution, err := s.investmentAttributionCheck(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	lots, err := s.lotReconciliationCheck(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	positions, err := s.commodityPositionSignCheck(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	checkpoints, err := s.checkpointIntegrityCheck(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	prices, err := s.priceDependencyCheck(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	// Replay runs in short write transactions of its own; release the read
	// snapshot first so it never holds a connection the replay waits on.
	_ = snapshot.Rollback()
	replay, err := s.investmentReplayCheck(ctx)
	if err != nil {
		return nil, err
	}

	integrity, err := s.sqliteIntegrityCheck(ctx)
	if err != nil {
		return nil, err
	}

	results := append(balances, structural, lots, positions, checkpoints, coverage, investmentFoundation, attribution, replay, prices, integrity, s.attachmentsCheck())
	for index := range results {
		narrative := checkNarratives[results[index].CheckID]
		results[index].Explanation = narrative.explanation
		results[index].NextStep = narrative.nextStep
	}
	return results, nil
}

// investmentReplayCheck is T-134's verifier: every long and short position's stored
// projection must equal its replay from effective intents.
func (s *SelfCheckService) investmentReplayCheck(ctx context.Context) (SelfCheckResult, error) {
	equivalence, err := s.repository.InvestmentReplayEquivalence(ctx, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}
	result := SelfCheckResult{CheckID: CheckInvestmentReplay, Status: SelfCheckPassed,
		Summary: fmt.Sprintf("%d investment positions replay to their stored lots, disposals, splits and transfers",
			equivalence.Positions)}
	if len(equivalence.Mismatches) > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount = int64(len(equivalence.Mismatches))
		counts := make(map[string]int)
		var kinds []string
		var references []string
		for _, mismatch := range equivalence.Mismatches {
			if counts[mismatch.Kind] == 0 {
				kinds = append(kinds, mismatch.Kind)
			}
			counts[mismatch.Kind]++
			if len(result.Sample) < db.SelfCheckSampleLimit && mismatch.ReferenceID > 0 {
				result.Sample = append(result.Sample, mismatch.ReferenceID)
				references = append(references, fmt.Sprintf("%s #%d", replayReferenceLabel(mismatch.Kind), mismatch.ReferenceID))
			}
		}
		var parts []string
		for _, kind := range kinds {
			parts = append(parts, fmt.Sprintf("%d %s", counts[kind], replayMismatchLabel(kind)))
		}
		result.Summary = "replay disagrees with the stored projection: " + strings.Join(parts, "; ")
		if len(references) > 0 {
			result.Summary += " (" + strings.Join(references, ", ") + ")"
		}
	}
	return result, nil
}

func replayMismatchLabel(kind string) string {
	switch kind {
	case db.ReplayMismatchLotState:
		return "lot balances"
	case db.ReplayMismatchDisposal:
		return "disposal allocations"
	case db.ReplayMismatchSplit:
		return "split effects"
	case db.ReplayMismatchTransferLink:
		return "transfers carrying stale basis"
	case db.ReplayMismatchCapitalReturn:
		return "returns of capital with stale effects"
	case db.ReplayMismatchMethodFamily:
		return "method locks"
	default:
		return "positions whose history no longer replays"
	}
}

func replayReferenceLabel(kind string) string {
	switch kind {
	case db.ReplayMismatchLotState:
		return "lot"
	case db.ReplayMismatchDisposal:
		return "decision"
	default:
		return "operation"
	}
}

func (s *SelfCheckService) investmentFoundationCheck(ctx context.Context, snapshot *sql.Tx) (SelfCheckResult, error) {
	result := SelfCheckResult{CheckID: CheckInvestmentFoundation, Status: SelfCheckPassed, Summary: "investment operations, source facts, proceeds, setup defaults and saved gains documents are consistent"}
	checks := []struct {
		label string
		query string
	}{
		{"completed setup missing transfer equity account", `
			SELECT b.id FROM books b JOIN setup_steps step ON step.step_key = 'system_accounts'
			WHERE b.id = ? AND step.completed_at IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM accounts a JOIN ` + db.CurrentReferenceVersions(ctx, "account") + ` av ON av.account_id = a.id
				WHERE a.book_id = b.id AND a.system_role = 'external_investment_transfer_equity'
				  AND av.account_class = 'equity' AND av.account_kind = 'equity')`},
		{"completed setup missing commission default", `
			SELECT b.id FROM books b JOIN setup_steps step ON step.step_key = 'system_accounts'
			WHERE b.id = ? AND step.completed_at IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM investment_fee_policies p
				JOIN investment_fee_policy_versions v ON v.policy_id = p.id
				WHERE p.book_id = b.id AND p.account_id IS NULL AND p.charge_kind = 'commission'
				  AND v.version_seq = 1 AND v.treatment = 'clearing_included')`},
		{"journal-backed operation missing a posted version link", `
			SELECT o.id FROM investment_operations o WHERE o.book_id = ?
			-- Investment operations require a posted journal. The declared
			-- exemption for basis resolutions (#168): a known-zero basis resolution has no bridge to
			-- post, and neither does the reversal of one. Their facts and their
			-- absence of journals are checked with the resolution below.
			AND NOT EXISTS (SELECT 1 FROM investment_basis_resolutions zero
				WHERE zero.basis_value = '0' AND (zero.operation_id = o.id
					OR (o.operation_kind = 'reversal' AND zero.operation_id = o.correction_of_operation_id)))
			-- #146: a sourced split with no original lot effects starts at
			-- zero delta. Split journal/effect conservation below still requires
			-- every nonzero replay effect to have a posted adjustment journal.
			AND NOT (o.operation_kind = 'split' AND EXISTS (
				SELECT 1 FROM investment_split_facts f WHERE f.operation_id = o.id AND f.book_id = o.book_id)
				AND NOT EXISTS (SELECT 1 FROM investment_operation_lot_effects x WHERE x.operation_id = o.id))
            -- #196: reversing a sourced split can also be journal-free.
            -- Its predecessor's journals, including this inverse if any,
            -- must still net to the now-ineffective split's zero effects.
            AND NOT (o.operation_kind = 'reversal' AND EXISTS (
              SELECT 1 FROM investment_split_facts f WHERE f.operation_id = o.correction_of_operation_id AND f.book_id = o.book_id))
			AND NOT EXISTS (SELECT 1 FROM investment_operation_journal_links l
				JOIN transaction_versions v ON v.id = l.transaction_version_id
				JOIN transactions t ON t.id = v.transaction_id
				WHERE l.operation_id = o.id AND l.book_id = o.book_id
				AND v.book_id = o.book_id AND v.status = 'posted'
				AND t.book_id = o.book_id AND t.deleted_at IS NULL)`},
		{"trade has a cash, expense or charge-clearing posting without a source component", `
			SELECT DISTINCT o.id FROM investment_operations o
			JOIN investment_operation_journal_links l ON l.operation_id = o.id AND l.book_id = o.book_id
				AND l.role = 'primary'
			JOIN posting_versions pv ON pv.transaction_version_id = l.transaction_version_id
			JOIN posting_lines pl ON pl.id = pv.posting_line_id
			JOIN accounts a ON a.id = pv.account_id AND a.book_id = o.book_id
			JOIN commodities commodity ON commodity.id = pv.commodity_id AND commodity.book_id = o.book_id
			WHERE o.book_id = ? AND o.operation_kind IN ('buy', 'sell')
			AND commodity.kind = 'currency'
			AND (a.system_role IS NULL OR pl.line_key LIKE 'investment-charge-%')
			AND NOT EXISTS (SELECT 1 FROM investment_operation_components c
				WHERE c.operation_id = o.id AND c.posting_version_id = pv.id)`},
		{"payment in lieu has missing, malformed or cross-book attribution facts", `
			SELECT o.id FROM investment_operations o WHERE o.book_id = ?
			AND (o.operation_kind = 'payment_in_lieu' OR (o.operation_kind = 'reversal' AND EXISTS (
				SELECT 1 FROM investment_operations predecessor WHERE predecessor.id = o.correction_of_operation_id AND predecessor.operation_kind = 'payment_in_lieu')))
			AND NOT EXISTS (SELECT 1 FROM valid_investment_payment_in_lieu_facts f WHERE f.operation_id = o.id AND f.book_id = o.book_id)`},
		{"dividend has missing, malformed or inconsistent payment-shape facts", `
			SELECT o.id FROM investment_operations o WHERE o.book_id=?
			AND (o.operation_kind='dividend' OR (o.operation_kind='reversal' AND EXISTS (
				SELECT 1 FROM investment_operations predecessor WHERE predecessor.id=o.correction_of_operation_id AND predecessor.operation_kind='dividend')))
			AND NOT EXISTS (SELECT 1 FROM valid_investment_dividend_facts f WHERE f.operation_id=o.id AND f.book_id=o.book_id)`},
		{"cash payment claimed by multiple provider events", `
			SELECT identity.root_operation_id FROM investment_cash_payment_identity identity
			JOIN investment_event_suggestions claim ON claim.book_id=identity.book_id
				AND (claim.generated_transaction_id=identity.transaction_id OR claim.linked_transaction_id=identity.transaction_id)
			WHERE identity.book_id=? GROUP BY identity.root_operation_id HAVING COUNT(DISTINCT claim.id)>1`},
		{"operation journal link is not posted in its book", `
			SELECT l.id FROM investment_operation_journal_links l
			JOIN transaction_versions v ON v.id = l.transaction_version_id
			JOIN transactions t ON t.id = v.transaction_id
			WHERE l.book_id = ? AND (v.book_id <> l.book_id OR v.status <> 'posted'
				OR t.book_id <> l.book_id OR t.deleted_at IS NOT NULL)`},
		{"operation lot missing immutable source facts", `
			SELECT DISTINCT l.id FROM investment_lots l
			JOIN transaction_versions v ON v.transaction_id = l.source_transaction_id
			JOIN investment_operation_journal_links link ON link.transaction_version_id = v.id
			JOIN investment_operations o ON o.id = link.operation_id
			WHERE l.book_id = ? AND link.book_id = l.book_id
			AND l.operation_id IS NOT o.id`},
		{"operation lot event missing effect link", `
			SELECT DISTINCT e.id FROM investment_lot_events e
			JOIN transaction_versions v ON v.transaction_id = e.transaction_id
			JOIN investment_operation_journal_links link ON link.transaction_version_id = v.id
			JOIN investment_operations o ON o.id = link.operation_id
			WHERE e.book_id = ? AND link.book_id = e.book_id
			AND NOT EXISTS (SELECT 1 FROM investment_operation_lot_effects x
				WHERE x.lot_event_id = e.id AND x.operation_id = o.id)`},
		{"external transfer missing typed source or matching lot", `
			SELECT o.id FROM investment_operations o WHERE o.book_id = ?
			AND o.operation_kind = 'external_transfer_in'
			AND NOT EXISTS (
				SELECT 1 FROM investment_transfer_facts f
				JOIN investment_transfer_lot_links x ON x.operation_id = f.operation_id
				JOIN investment_lots l ON l.id = x.destination_lot_id
				WHERE f.operation_id = o.id AND f.transfer_kind = 'external_in'
				AND f.effective_on = o.event_date AND f.destination_account_id = l.account_id
				AND f.commodity_id = l.commodity_id
				AND EXISTS (SELECT 1 FROM investment_operation_journal_links link
					JOIN transaction_versions v ON v.id = link.transaction_version_id
					WHERE link.operation_id = o.id AND v.transaction_id = l.source_transaction_id)
				AND x.basis_knowledge = l.opening_basis_knowledge AND x.quantity_value = l.quantity_value
				AND x.quantity_scale = l.quantity_scale
				AND x.carried_basis_value IS l.cost_basis_value
				AND x.carried_basis_scale IS l.cost_basis_scale
				AND x.cost_commodity_id = l.cost_commodity_id)`},
		{"internal transfer missing linked source and destination effects", `
			SELECT o.id FROM investment_operations o WHERE o.book_id = ?
			AND o.operation_kind = 'internal_transfer'
			AND (NOT EXISTS (SELECT 1 FROM investment_transfer_facts f
				JOIN investment_transfer_lot_links x ON x.operation_id = f.operation_id
				WHERE f.operation_id = o.id AND f.transfer_kind = 'internal')
			OR EXISTS (SELECT 1 FROM investment_transfer_lot_links x
				JOIN investment_transfer_facts f ON f.operation_id = x.operation_id
				WHERE f.operation_id = o.id AND x.source_lot_id IS NOT NULL AND NOT EXISTS (
					SELECT 1 FROM investment_lots source JOIN investment_lots destination
						ON destination.id = x.destination_lot_id
					JOIN investment_lot_events source_event ON source_event.lot_id = source.id
						AND source_event.event_kind = 'transfer_out'
					JOIN investment_operation_lot_effects source_effect ON source_effect.lot_event_id = source_event.id
						AND source_effect.operation_id = o.id
					JOIN investment_lot_events destination_event ON destination_event.lot_id = destination.id
						AND destination_event.event_kind = 'transfer_in'
					JOIN investment_operation_lot_effects destination_effect ON destination_effect.lot_event_id = destination_event.id
						AND destination_effect.operation_id = o.id
					WHERE source.id = x.source_lot_id AND source.book_id = f.book_id
						AND source.account_id = f.source_account_id AND destination.account_id = f.destination_account_id
						AND source.commodity_id = f.commodity_id AND destination.commodity_id = f.commodity_id
						AND EXISTS (SELECT 1 FROM investment_operation_journal_links link
							JOIN transaction_versions v ON v.id = link.transaction_version_id
							WHERE link.operation_id = o.id
							AND v.transaction_id = destination.source_transaction_id
							AND v.transaction_id = source_event.transaction_id
							AND v.transaction_id = destination_event.transaction_id)
						AND x.quantity_value = destination.quantity_value AND x.quantity_scale = destination.quantity_scale
						AND x.basis_knowledge = destination.opening_basis_knowledge
						AND x.carried_basis_value IS destination.cost_basis_value
						AND x.carried_basis_scale IS destination.cost_basis_scale
						AND x.cost_commodity_id = source.cost_commodity_id
						AND x.cost_commodity_id = destination.cost_commodity_id
						AND source_event.quantity_value = '-' || x.quantity_value
						AND source_event.quantity_scale = x.quantity_scale
						AND source_event.basis_knowledge = x.basis_knowledge
						AND source_event.cost_basis_scale IS x.carried_basis_scale
						AND (x.basis_knowledge = 'unknown' OR source_event.cost_basis_value = '-' || x.carried_basis_value
							OR (source_event.cost_basis_value = '0' AND x.carried_basis_value = '0'))
				))
			-- A pooled_lot transfer (T-135) has one link and one destination
			-- lot opened by its transfer_in effect; every transfer_out effect
			-- takes from its source position. Totals are folded in Go.
			OR EXISTS (SELECT 1 FROM investment_transfer_facts f WHERE f.operation_id = o.id
				AND f.destination_lineage = 'pooled_lot' AND (
					(SELECT COUNT(*) FROM investment_transfer_lot_links x WHERE x.operation_id = f.operation_id) <> 1
					OR NOT EXISTS (SELECT 1 FROM investment_transfer_lot_links x
						JOIN investment_lots destination ON destination.id = x.destination_lot_id
						JOIN investment_lot_events destination_event ON destination_event.lot_id = destination.id
							AND destination_event.event_kind = 'transfer_in'
						JOIN investment_operation_lot_effects destination_effect
							ON destination_effect.lot_event_id = destination_event.id AND destination_effect.operation_id = o.id
						WHERE x.operation_id = f.operation_id AND x.source_lot_id IS NULL
							AND destination.account_id = f.destination_account_id AND destination.commodity_id = f.commodity_id
							AND destination.cost_commodity_id = x.cost_commodity_id
							AND x.quantity_value = destination.quantity_value AND x.quantity_scale = destination.quantity_scale
							AND x.basis_knowledge = destination.opening_basis_knowledge
							AND x.carried_basis_value IS destination.cost_basis_value
							AND x.carried_basis_scale IS destination.cost_basis_scale
							AND EXISTS (SELECT 1 FROM investment_operation_journal_links link
								JOIN transaction_versions v ON v.id = link.transaction_version_id
								WHERE link.operation_id = o.id AND v.transaction_id = destination.source_transaction_id
								AND v.transaction_id = destination_event.transaction_id))
					OR NOT EXISTS (SELECT 1 FROM investment_operation_lot_effects x
						JOIN investment_lot_events e ON e.id = x.lot_event_id AND e.event_kind = 'transfer_out'
						WHERE x.operation_id = o.id)
					OR EXISTS (SELECT 1 FROM investment_operation_lot_effects x
						JOIN investment_lot_events e ON e.id = x.lot_event_id AND e.event_kind = 'transfer_out'
						JOIN investment_lots source ON source.id = e.lot_id
						JOIN investment_transfer_lot_links link ON link.operation_id = f.operation_id
						WHERE x.operation_id = o.id AND (source.account_id <> f.source_account_id
							OR source.commodity_id <> f.commodity_id OR source.cost_commodity_id <> link.cost_commodity_id))))
			)`},
		// Each exchange link moves one source lot whole into one destination
		// lot of the new instrument in the destination holding, carrying its basis
		// and knowledge, through effects on the exchange's own journal (#177).
		// The ratio and journal totals are folded in Go.
		{"share exchange missing its fact or linked whole-lot effects", `
			SELECT o.id FROM investment_operations o WHERE o.book_id = ?
			AND o.operation_kind IN ('share_exchange', 'cash_and_stock_merger')
			AND (NOT EXISTS (SELECT 1 FROM investment_transfer_facts f
				JOIN investment_transfer_lot_links x ON x.operation_id = f.operation_id
				WHERE f.operation_id = o.id AND f.transfer_kind = 'exchange' AND f.effective_on = o.event_date)
			OR EXISTS (SELECT 1 FROM investment_transfer_lot_links x
				JOIN investment_transfer_facts f ON f.operation_id = x.operation_id
				WHERE f.operation_id = o.id AND NOT EXISTS (
					SELECT 1 FROM investment_lots source JOIN investment_lots destination
						ON destination.id = x.destination_lot_id
					JOIN investment_lot_events source_event ON source_event.lot_id = source.id
						AND source_event.event_kind = 'transfer_out'
					JOIN investment_operation_lot_effects source_effect ON source_effect.lot_event_id = source_event.id
						AND source_effect.operation_id = o.id
					JOIN investment_lot_events destination_event ON destination_event.lot_id = destination.id
						AND destination_event.event_kind = 'transfer_in'
					JOIN investment_operation_lot_effects destination_effect ON destination_effect.lot_event_id = destination_event.id
						AND destination_effect.operation_id = o.id
					WHERE source.id = x.source_lot_id AND source.book_id = f.book_id
						AND source.account_id = f.source_account_id AND destination.account_id = f.destination_account_id
						AND source.commodity_id = f.commodity_id AND destination.commodity_id = f.destination_commodity_id
						AND source.position_side = 'long' AND destination.position_side = 'long'
						AND destination.opened_on = f.effective_on AND source_event.event_date = f.effective_on
						AND EXISTS (SELECT 1 FROM investment_operation_journal_links link
							JOIN transaction_versions v ON v.id = link.transaction_version_id
							WHERE link.operation_id = o.id AND link.role = 'primary'
							AND v.transaction_id = destination.source_transaction_id
							AND v.transaction_id = source_event.transaction_id
							AND v.transaction_id = destination_event.transaction_id)
						AND x.basis_knowledge = destination.opening_basis_knowledge
						AND x.carried_basis_value IS destination.cost_basis_value
						AND x.carried_basis_scale IS destination.cost_basis_scale
						AND x.cost_commodity_id = source.cost_commodity_id
						AND x.cost_commodity_id = destination.cost_commodity_id
						AND source_event.quantity_value = '-' || x.quantity_value
						AND source_event.quantity_scale = x.quantity_scale
						AND source_event.basis_knowledge = x.basis_knowledge
						AND source_event.cost_basis_scale IS x.carried_basis_scale
						AND (x.basis_knowledge = 'unknown' OR source_event.cost_basis_value = '-' || x.carried_basis_value
							OR (source_event.cost_basis_value = '0' AND x.carried_basis_value = '0'))))
			-- Every effect the exchange made is one of its links' events.
			OR EXISTS (SELECT 1 FROM investment_operation_lot_effects effect
				JOIN investment_lot_events e ON e.id = effect.lot_event_id
				WHERE effect.operation_id = o.id AND NOT EXISTS (
					SELECT 1 FROM investment_transfer_lot_links x
					WHERE x.operation_id = o.id AND ((e.event_kind = 'transfer_out' AND x.source_lot_id = e.lot_id)
						OR (e.event_kind = 'transfer_in' AND x.destination_lot_id = e.lot_id)
						-- A merger's cash realizes on the exchanged lot (#181).
						OR (e.event_kind = 'disposal' AND o.operation_kind = 'cash_and_stock_merger'
							AND x.source_lot_id = e.lot_id)))))`},
		// A cash-and-stock merger records its election and payable date, and
		// one quantity-0 cash decision on the old position whose allocations
		// realize each link's lot once, through quantity-0 disposal effects on
		// the effective date (#181). Amounts are checked by the allocation,
		// clearing and lot reconciliation checks.
		{"cash-and-stock merger missing its election, cash decision or per-lot realization", `
			SELECT o.id FROM investment_operations o WHERE o.book_id = ?
			AND o.operation_kind = 'cash_and_stock_merger'
			AND (NOT EXISTS (SELECT 1 FROM investment_transfer_facts f
				JOIN investment_operation_dates payable ON payable.operation_id = o.id AND payable.date_role = 'payable'
					AND payable.date_seq = 1 AND payable.event_date >= f.effective_on
				WHERE f.operation_id = o.id AND f.transfer_kind = 'exchange' AND f.cash_basis_fraction_value IS NOT NULL)
			OR (SELECT COUNT(*) FROM investment_disposal_decisions d JOIN investment_transfer_facts f ON f.operation_id = o.id
				WHERE d.operation_id = o.id AND d.quantity_value = '0' AND d.position_side = 'long'
					AND d.account_id = f.source_account_id AND d.commodity_id = f.commodity_id
					AND d.event_date = f.effective_on) <> 1
			OR EXISTS (SELECT 1 FROM investment_disposal_decisions d WHERE d.operation_id = o.id AND d.quantity_value <> '0')
			OR (SELECT COUNT(*) FROM investment_disposal_allocations a
				JOIN investment_disposal_decisions d ON d.id = a.decision_id WHERE d.operation_id = o.id)
				<> (SELECT COUNT(*) FROM investment_transfer_lot_links x WHERE x.operation_id = o.id)
			OR EXISTS (SELECT 1 FROM investment_transfer_lot_links x
				JOIN investment_transfer_facts f ON f.operation_id = x.operation_id
				WHERE x.operation_id = o.id AND NOT EXISTS (
					SELECT 1 FROM investment_disposal_allocations a
					JOIN investment_disposal_decisions d ON d.id = a.decision_id AND d.operation_id = o.id
					JOIN investment_lot_events e ON e.id = a.lot_event_id AND e.lot_id = x.source_lot_id
					JOIN investment_operation_lot_effects effect ON effect.lot_event_id = e.id AND effect.operation_id = o.id
					WHERE a.lot_id = x.source_lot_id AND a.quantity_value = '0' AND e.event_kind = 'disposal'
						AND e.quantity_value = '0' AND e.event_date = f.effective_on)))`},
		// Each spin-off link reduces one parent lot's basis, keeping its units,
		// by exactly the basis that opens one new lot of the distributed
		// instrument, through effects on the spin-off's own journal (#180).
		// The ratio and journal total are folded in Go.
		{"spin-off missing its fact or linked basis effects", `
			SELECT o.id FROM investment_operations o WHERE o.book_id = ?
			AND o.operation_kind = 'spin_off'
			AND (NOT EXISTS (SELECT 1 FROM investment_transfer_facts f
				JOIN investment_transfer_lot_links x ON x.operation_id = f.operation_id
				WHERE f.operation_id = o.id AND f.transfer_kind = 'spin_off' AND f.effective_on = o.event_date)
			OR EXISTS (SELECT 1 FROM investment_transfer_lot_links x
				JOIN investment_transfer_facts f ON f.operation_id = x.operation_id
				WHERE f.operation_id = o.id AND NOT EXISTS (
					SELECT 1 FROM investment_lots source JOIN investment_lots destination
						ON destination.id = x.destination_lot_id
					JOIN investment_lot_events source_event ON source_event.lot_id = source.id
						AND source_event.event_kind = 'basis_reduction'
					JOIN investment_operation_lot_effects source_effect ON source_effect.lot_event_id = source_event.id
						AND source_effect.operation_id = o.id
					JOIN investment_lot_events destination_event ON destination_event.lot_id = destination.id
						AND destination_event.event_kind = 'transfer_in'
					JOIN investment_operation_lot_effects destination_effect ON destination_effect.lot_event_id = destination_event.id
						AND destination_effect.operation_id = o.id
					WHERE source.id = x.source_lot_id AND source.book_id = f.book_id
						AND source.account_id = f.source_account_id AND destination.account_id = f.destination_account_id
						AND source.commodity_id = f.commodity_id AND destination.commodity_id = f.destination_commodity_id
						AND source.position_side = 'long' AND destination.position_side = 'long'
						AND source.opened_on <= f.effective_on
						AND destination.opened_on = f.effective_on AND source_event.event_date = f.effective_on
						AND EXISTS (SELECT 1 FROM investment_operation_journal_links link
							JOIN transaction_versions v ON v.id = link.transaction_version_id
							WHERE link.operation_id = o.id AND link.role = 'primary'
							AND v.transaction_id = destination.source_transaction_id
							AND v.transaction_id = source_event.transaction_id
							AND v.transaction_id = destination_event.transaction_id)
						AND x.basis_knowledge = destination.opening_basis_knowledge
						AND x.carried_basis_value IS destination.cost_basis_value
						AND x.carried_basis_scale IS destination.cost_basis_scale
						AND x.cost_commodity_id = source.cost_commodity_id
						AND x.cost_commodity_id = destination.cost_commodity_id
						AND source_event.quantity_value = '0'
						AND source_event.basis_knowledge = x.basis_knowledge
						AND source_event.cost_basis_scale IS x.carried_basis_scale
						AND (x.basis_knowledge = 'unknown' OR source_event.cost_basis_value = '-' || x.carried_basis_value
							OR (source_event.cost_basis_value = '0' AND x.carried_basis_value = '0'))))
			-- Every effect the spin-off made is one of its links' events.
			OR EXISTS (SELECT 1 FROM investment_operation_lot_effects effect
				JOIN investment_lot_events e ON e.id = effect.lot_event_id
				WHERE effect.operation_id = o.id AND NOT EXISTS (
					SELECT 1 FROM investment_transfer_lot_links x
					WHERE x.operation_id = o.id AND ((e.event_kind = 'basis_reduction' AND x.source_lot_id = e.lot_id)
						OR (e.event_kind = 'transfer_in' AND x.destination_lot_id = e.lot_id)))))`},
		{"outbound transfer missing linked source effects", `
			SELECT o.id FROM investment_operations o WHERE o.book_id = ?
			AND o.operation_kind = 'external_transfer_out'
			AND (NOT EXISTS (SELECT 1 FROM investment_transfer_facts f
				JOIN investment_transfer_lot_links x ON x.operation_id = f.operation_id
				WHERE f.operation_id = o.id AND f.transfer_kind = 'external_out')
			OR EXISTS (SELECT 1 FROM investment_transfer_lot_links x
				JOIN investment_transfer_facts f ON f.operation_id = x.operation_id
				WHERE f.operation_id = o.id AND (x.destination_lot_id IS NOT NULL
					OR NOT EXISTS (
					SELECT 1 FROM investment_lots source
					JOIN investment_lot_events source_event ON source_event.lot_id = source.id
						AND source_event.event_kind = 'transfer_out'
					JOIN investment_operation_lot_effects source_effect ON source_effect.lot_event_id = source_event.id
						AND source_effect.operation_id = o.id
					WHERE source.id = x.source_lot_id AND source.book_id = f.book_id
						AND source.account_id = f.source_account_id AND source.commodity_id = f.commodity_id
						AND source.cost_commodity_id = x.cost_commodity_id
						AND EXISTS (SELECT 1 FROM investment_operation_journal_links link
							JOIN transaction_versions v ON v.id = link.transaction_version_id
							WHERE link.operation_id = o.id AND link.role = 'primary'
							AND v.transaction_id = source_event.transaction_id)
						AND source_event.quantity_value = '-' || x.quantity_value
						AND source_event.quantity_scale = x.quantity_scale
						AND source_event.basis_knowledge = x.basis_knowledge
						AND source_event.cost_basis_scale IS x.carried_basis_scale
						AND (x.basis_knowledge = 'unknown' OR source_event.cost_basis_value = '-' || x.carried_basis_value
							OR (source_event.cost_basis_value = '0' AND x.carried_basis_value = '0')))))
			-- Every depletion the operation made is one of its links.
			OR EXISTS (SELECT 1 FROM investment_operation_lot_effects effect
				JOIN investment_lot_events e ON e.id = effect.lot_event_id
				WHERE effect.operation_id = o.id AND (e.event_kind <> 'transfer_out' OR NOT EXISTS (
					SELECT 1 FROM investment_transfer_lot_links x
					WHERE x.operation_id = o.id AND x.source_lot_id = e.lot_id))))`},
		// A sourced resolution's primary journal is exactly its omitted inbound
		// bridge: trading +b and equity -b in the cost currency, at the
		// transfer date, and nothing else (T-145). A known zero has no primary
		// journal at all (#168). The pinned facts are checked against the
		// still-unknown link it resolves.
		{"basis resolution provenance has broken lineage or audit attribution", `
			SELECT p.resolution_operation_id FROM investment_basis_resolution_provenance p
			LEFT JOIN investment_basis_resolutions r ON r.operation_id = p.resolution_operation_id
			LEFT JOIN audit_events a ON a.id = p.created_audit_event_id
			WHERE p.book_id = ? AND (r.operation_id IS NULL OR r.book_id <> p.book_id
				OR a.id IS NULL OR a.book_id <> p.book_id OR a.actor_user_id IS NULL
				OR a.operation <> 'investment.basis_resolution.provenance' OR length(trim(a.reason)) = 0
				OR p.revision_seq <> (SELECT COUNT(*) FROM investment_basis_resolution_provenance prior
					WHERE prior.resolution_operation_id = p.resolution_operation_id AND prior.revision_seq <= p.revision_seq)
				OR NOT json_valid(p.source_evidence_json))`},
		{"basis resolution disagrees with its bridge or pinned transfer", `
			SELECT o.id FROM investment_operations o
			WHERE o.book_id = ? AND o.operation_kind = 'basis_resolution'
			AND (NOT EXISTS (SELECT 1 FROM investment_basis_resolutions r
				JOIN investment_transfer_lot_links x ON x.operation_id = r.transfer_operation_id AND x.link_seq = r.link_seq
				JOIN investment_transfer_facts f ON f.operation_id = x.operation_id
				WHERE r.operation_id = o.id AND x.basis_knowledge = 'unknown' AND f.transfer_kind = 'external_in'
					AND x.destination_lot_id = r.lot_id AND x.quantity_value = r.quantity_value
					AND x.quantity_scale = r.quantity_scale AND x.cost_commodity_id = r.cost_commodity_id
					AND f.effective_on = o.event_date)
			OR (SELECT COUNT(*) FROM investment_operation_journal_links l
				JOIN posting_versions pv ON pv.transaction_version_id = l.transaction_version_id
				WHERE l.operation_id = o.id AND l.role = 'primary')
				<> CASE WHEN EXISTS (SELECT 1 FROM investment_basis_resolutions zero
					WHERE zero.operation_id = o.id AND zero.basis_value = '0') THEN 0 ELSE 2 END
			OR (NOT EXISTS (SELECT 1 FROM investment_basis_resolutions zero
				WHERE zero.operation_id = o.id AND zero.basis_value = '0')
			AND NOT EXISTS (SELECT 1 FROM investment_basis_resolutions r
				JOIN investment_operation_journal_links l ON l.operation_id = r.operation_id AND l.role = 'primary'
				JOIN transaction_versions v ON v.id = l.transaction_version_id
				JOIN posting_versions trading ON trading.transaction_version_id = l.transaction_version_id
				JOIN accounts ta ON ta.id = trading.account_id AND ta.system_role = 'commodity_trading'
				JOIN posting_versions equity ON equity.transaction_version_id = l.transaction_version_id
				JOIN accounts ea ON ea.id = equity.account_id AND ea.system_role = 'external_investment_transfer_equity'
				WHERE r.operation_id = o.id AND v.transaction_date = o.event_date
					AND trading.commodity_id = r.cost_commodity_id AND equity.commodity_id = r.cost_commodity_id
					AND trading.quantity_value = r.basis_value AND trading.quantity_scale = r.basis_scale
					AND equity.quantity_value = '-' || r.basis_value AND equity.quantity_scale = r.basis_scale)))`},
		// The reversal of a journal-free known zero has nothing to invert, and
		// a zero successor's only journal is its predecessor's inverse (#168).
		{"known-zero basis resolution or its reversal posts a journal", `
			SELECT o.id FROM investment_operations o
			JOIN investment_basis_resolutions zero ON zero.operation_id IN (o.id, o.correction_of_operation_id)
			WHERE o.book_id = ? AND zero.basis_value = '0'
				AND (o.operation_kind = 'reversal' OR zero.operation_id = o.id)
				AND EXISTS (SELECT 1 FROM investment_operation_journal_links l
					WHERE l.operation_id = o.id AND l.role <> 'reversal')`},
		// A link has at most one effective resolution, and a replacement
		// corrects a resolution of the same link (#168).
		{"transfer link has more than one effective basis resolution or a misdirected correction", `
			SELECT r.operation_id FROM investment_basis_resolutions r
			JOIN investment_operations o ON o.id = r.operation_id
			WHERE r.book_id = ?
			AND ((EXISTS (SELECT 1 FROM effective_investment_operations e WHERE e.id = r.operation_id)
				AND EXISTS (SELECT 1 FROM investment_basis_resolutions other
					JOIN effective_investment_operations e ON e.id = other.operation_id
					WHERE other.transfer_operation_id = r.transfer_operation_id AND other.link_seq = r.link_seq
						AND other.operation_id <> r.operation_id))
			OR (o.correction_of_operation_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM investment_basis_resolutions prior
				WHERE prior.operation_id = o.correction_of_operation_id
					AND prior.transfer_operation_id = r.transfer_operation_id AND prior.link_seq = r.link_seq)))`},
		{"transfer basis allocation disagrees with its source depletions", `
			SELECT f.operation_id FROM investment_transfer_facts f
			WHERE f.book_id = ? AND f.transfer_kind IN ('internal', 'external_out')
			AND EXISTS (SELECT 1 FROM investment_operation_lot_effects x
				JOIN investment_lot_events e ON e.id = x.lot_event_id AND e.event_kind = 'transfer_out'
				WHERE x.operation_id = f.operation_id
				AND (e.cost_basis_method IS 'average_cost') <> (f.basis_allocation = 'average_cost_pool'))`},
		{"return of capital missing its facts or matching basis-reduction events", `
			SELECT o.id FROM investment_operations o WHERE o.book_id = ?
			AND o.operation_kind = 'return_of_capital'
			AND (NOT EXISTS (SELECT 1 FROM investment_capital_return_facts f
				JOIN investment_capital_return_effects e ON e.operation_id = f.operation_id
				WHERE f.operation_id = o.id AND f.payment_on = o.event_date)
			OR EXISTS (SELECT 1 FROM investment_capital_return_effects e
				JOIN investment_lot_events ev ON ev.id = e.lot_event_id
				WHERE e.operation_id = o.id AND (ev.event_kind <> 'basis_reduction' OR ev.quantity_value <> '0'
					OR NOT EXISTS (SELECT 1 FROM investment_operation_lot_effects x
						WHERE x.operation_id = o.id AND x.lot_event_id = ev.id)))
			OR EXISTS (SELECT 1 FROM investment_operation_lot_effects x
				JOIN investment_lot_events ev ON ev.id = x.lot_event_id
				WHERE x.operation_id = o.id AND NOT EXISTS (SELECT 1 FROM investment_capital_return_effects e
					WHERE e.operation_id = o.id AND e.lot_event_id = ev.id)))`},
		{"cash in lieu has missing, malformed or cross-book parent facts", `
            SELECT o.id FROM effective_investment_operations o WHERE o.book_id = ? AND o.operation_kind = 'cash_in_lieu'
            AND NOT EXISTS (SELECT 1 FROM valid_investment_cash_in_lieu_facts f WHERE f.operation_id = o.id AND f.book_id = o.book_id)`},
		{"split missing its sourced ratio or lot effects", `
			SELECT o.id FROM investment_operations o WHERE o.book_id = ?
			AND o.operation_kind = 'split'
			AND (NOT EXISTS (SELECT 1 FROM investment_split_facts f
				WHERE f.operation_id = o.id AND f.book_id = o.book_id AND f.effective_on = o.event_date)
			OR EXISTS (SELECT 1 FROM investment_operation_lot_effects x
				JOIN investment_lot_events e ON e.id = x.lot_event_id
				JOIN investment_lots l ON l.id = e.lot_id
				JOIN investment_split_facts f ON f.operation_id = o.id
				WHERE x.operation_id = o.id AND (e.event_date <> f.effective_on
					OR l.account_id <> f.account_id OR l.commodity_id <> f.commodity_id))
			OR EXISTS (SELECT 1 FROM investment_operation_lot_effects x
				JOIN investment_lot_events e ON e.id = x.lot_event_id
				WHERE x.operation_id = o.id AND e.event_kind <> 'split_adjustment'))`},
		{"split adjustment journal not recorded by a split revision on the split date", `
			SELECT link.id FROM investment_operation_journal_links link
			WHERE link.book_id = ? AND link.role = 'split_adjustment'
			AND NOT EXISTS (SELECT 1 FROM investment_split_revisions r
				JOIN investment_split_facts f ON f.operation_id = r.operation_id
				JOIN transaction_versions v ON v.id = r.adjustment_transaction_version_id
				WHERE r.adjustment_transaction_version_id = link.transaction_version_id
					AND r.operation_id = link.operation_id AND v.transaction_date = f.effective_on
					AND NOT EXISTS (SELECT 1 FROM journal_entries je
						WHERE je.transaction_version_id = v.id AND je.entry_date <> f.effective_on))`},
		{"posted long disposal missing proceeds decision", `
			SELECT o.id FROM investment_operations o
			WHERE o.book_id = ? AND o.operation_kind IN ('sell', 'write_off', 'cash_in_lieu', 'cash_and_stock_merger')
			AND NOT EXISTS (SELECT 1 FROM investment_disposal_decisions d
				WHERE d.operation_id = o.id AND d.position_side = 'long')`},
		{"short sale missing its short lot", `
			SELECT o.id FROM investment_operations o
			WHERE o.book_id = ? AND o.operation_kind = 'short_sale'
			AND (NOT EXISTS (SELECT 1 FROM investment_lots l
				WHERE l.operation_id = o.id AND l.position_side = 'short' AND l.opened_on = o.event_date)
			OR EXISTS (SELECT 1 FROM investment_lots l WHERE l.operation_id = o.id AND l.position_side <> 'short'))`},
		{"short cover missing its short decision", `
			SELECT o.id FROM investment_operations o
			WHERE o.book_id = ? AND o.operation_kind = 'short_cover'
			AND (NOT EXISTS (SELECT 1 FROM investment_disposal_decisions d
				WHERE d.operation_id = o.id AND d.position_side = 'short')
			OR EXISTS (SELECT 1 FROM investment_disposal_decisions d
				WHERE d.operation_id = o.id AND d.position_side <> 'short'))`},
		{"short lot or decision outside a short operation", `
			SELECT o.id FROM investment_operations o
			WHERE o.book_id = ? AND (
				(o.operation_kind <> 'short_sale' AND EXISTS (SELECT 1 FROM investment_lots l
					WHERE l.operation_id = o.id AND l.position_side = 'short'))
				OR (o.operation_kind <> 'short_cover' AND EXISTS (SELECT 1 FROM investment_disposal_decisions d
					WHERE d.operation_id = o.id AND d.position_side = 'short')))`},
		{"disposal allocation consumes a lot of the other side", `
			SELECT d.operation_id FROM investment_disposal_decisions d
			JOIN investment_disposal_allocations a ON a.decision_id = d.id
			JOIN investment_lots l ON l.id = a.lot_id
			WHERE d.book_id = ? AND l.position_side <> d.position_side`},
		{"disposal decision has no matching operation journal link", `
			SELECT d.id FROM investment_disposal_decisions d
			JOIN investment_operations o ON o.id = d.operation_id
			WHERE d.book_id = ? AND o.operation_kind IN ('sell', 'write_off', 'cash_in_lieu', 'short_cover', 'cash_and_stock_merger')
			AND NOT EXISTS (SELECT 1 FROM investment_operation_journal_links l
				WHERE l.operation_id = d.operation_id AND l.book_id = d.book_id
				AND l.transaction_version_id = d.transaction_version_id AND l.role <> 'reversal')`},
	}
	var summaries []string
	var sampleReferences []string
	var invalidRuns int64
	if err := db.StreamGainsRuns(ctx, snapshot, BookID, func(run db.GainsRunRecord) error {
		if !db.ValidGainsRunDocument(run) {
			invalidRuns++
			if len(result.Sample) < db.SelfCheckSampleLimit {
				result.Sample = append(result.Sample, run.ID)
				sampleReferences = append(sampleReferences, fmt.Sprintf("gains run #%d", run.ID))
			}
		}
		return nil
	}); err != nil {
		return SelfCheckResult{}, err
	}
	if invalidRuns > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount += invalidRuns
		summaries = append(summaries, fmt.Sprintf("%d saved gains runs failed document integrity", invalidRuns))
	}
	var invalidReturnsRuns int64
	if err := db.StreamReturnsRuns(ctx, snapshot, BookID, func(run db.ReturnsRunRecord) error {
		if !db.ValidReturnsRunDocument(run) {
			invalidReturnsRuns++
			if len(result.Sample) < db.SelfCheckSampleLimit {
				result.Sample = append(result.Sample, run.ID)
				sampleReferences = append(sampleReferences, fmt.Sprintf("returns run #%d", run.ID))
			}
		}
		return nil
	}); err != nil {
		return SelfCheckResult{}, err
	}
	if invalidReturnsRuns > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount += invalidReturnsRuns
		summaries = append(summaries, fmt.Sprintf("%d saved returns runs failed document integrity", invalidReturnsRuns))
	}

	for _, check := range checks {
		anomaly, err := s.repository.CountStructuralAnomaly(ctx, snapshot, check.query, BookID)
		if err != nil {
			return SelfCheckResult{}, err
		}
		if anomaly.Count == 0 {
			continue
		}
		result.Status = SelfCheckFailed
		result.FindingCount += anomaly.Count
		kind := "operation"
		switch check.label {
		case "completed setup missing transfer equity account", "completed setup missing commission default":
			kind = "book"
		case "operation journal link is not posted in its book",
			"split adjustment journal not recorded by a split revision on the split date":
			kind = "journal link"
		case "operation lot missing immutable source facts":
			kind = "lot"
		case "operation lot event missing effect link":
			kind = "lot event"
		case "disposal decision has no matching operation journal link":
			kind = "decision"
		}
		for _, id := range anomaly.Sample {
			if len(result.Sample) >= db.SelfCheckSampleLimit {
				break
			}
			result.Sample = append(result.Sample, id)
			sampleReferences = append(sampleReferences, fmt.Sprintf("%s #%d", kind, id))
		}
		summaries = append(summaries, fmt.Sprintf("%d %s", anomaly.Count, check.label))
	}
	var componentMismatch int64
	err := s.repository.StreamInvestmentComponents(ctx, snapshot, BookID, func(component db.SelfCheckInvestmentComponentRecord) error {
		amount := exact.ScaledIntFromCoefficient(component.AmountValue, component.AmountScale)
		needsPosting := component.ComponentKind == "net_settlement" && amount.Sign() != 0 ||
			component.ComponentKind == "charge" && (component.SeparatelyPaid || component.ChargeTreatment.String == "separately_expensed")
		bad := needsPosting != component.PostingID.Valid
		if needsPosting && component.PostingID.Valid {
			expected := amount
			if component.ComponentKind == "charge" {
				expected = amount.Negated()
			}
			posted, parseErr := exact.Parse(component.PostingValue.String)
			if parseErr != nil {
				return fmt.Errorf("parse posting for investment component %d: %w", component.ComponentID, parseErr)
			}
			bad = component.PostingBookID.Int64 != BookID || !component.VersionLinked ||
				component.PostingCommodity.Int64 != component.CommodityID ||
				component.PostingDate.String != component.AmountDate ||
				expected.Cmp(exact.ScaledIntFromCoefficient(posted, int(component.PostingScale.Int64))) != 0
			if component.ComponentKind == "net_settlement" {
				bad = bad || !component.CashAccountID.Valid || component.PostingAccountID.Int64 != component.CashAccountID.Int64
			} else if component.ChargeTreatment.String == "separately_expensed" {
				bad = bad || !component.ChargeAccountID.Valid || component.PostingAccountID.Int64 != component.ChargeAccountID.Int64
			} else {
				bad = bad || component.PostingRole.String != "commodity_trading"
			}
		}
		if bad {
			componentMismatch++
			if len(result.Sample) < db.SelfCheckSampleLimit {
				result.Sample = append(result.Sample, component.ComponentID)
				sampleReferences = append(sampleReferences, fmt.Sprintf("component #%d", component.ComponentID))
			}
		}
		return nil
	})
	if err != nil {
		return SelfCheckResult{}, err
	}
	if componentMismatch > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount += componentMismatch
		summaries = append(summaries, fmt.Sprintf("%d source components disagree with posted journal legs", componentMismatch))
	}
	// A split's primary journal plus its adjustments must post exactly what its
	// effective lot effects moved; otherwise the holding and its lots disagree
	// in a way the aggregate holding check can net away across splits (T-129).
	splitDeltas, err := s.repository.SelfCheckSplitDeltas(ctx, snapshot, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}
	var splitMismatch int64
	for _, split := range splitDeltas {
		if split.Journal.Cmp(split.Effects) == 0 {
			continue
		}
		splitMismatch++
		if len(result.Sample) < db.SelfCheckSampleLimit {
			result.Sample = append(result.Sample, split.OperationID)
			sampleReferences = append(sampleReferences, fmt.Sprintf("operation #%d", split.OperationID))
		}
	}
	if splitMismatch > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount += splitMismatch
		summaries = append(summaries, fmt.Sprintf("%d splits post a different journal delta than their effective lot effects", splitMismatch))
	}
	// Every committed and revised depletion set of a pooled_lot transfer must
	// take exactly the quantity and basis its one destination lot carries.
	pooledSets, err := s.repository.SelfCheckPooledTransferSets(ctx, snapshot, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}
	var pooledMismatch int64
	for _, set := range pooledSets {
		// Quantity is conserved whatever the knowledge; basis only when known.
		knowledgeAgrees := set.LinkUnknown == (set.UnknownDepletions > 0)
		basisAgrees := set.LinkUnknown || set.DepletedBasis.Cmp(set.LinkBasis) == 0
		if set.Depletions > 0 && set.DepletedQuantity.Cmp(set.LinkQuantity) == 0 && knowledgeAgrees && basisAgrees {
			continue
		}
		pooledMismatch++
		if len(result.Sample) < db.SelfCheckSampleLimit {
			result.Sample = append(result.Sample, set.OperationID)
			sampleReferences = append(sampleReferences, fmt.Sprintf("operation #%d", set.OperationID))
		}
	}
	if pooledMismatch > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount += pooledMismatch
		summaries = append(summaries, fmt.Sprintf("%d pooled transfer depletion sets do not carry their link's quantity and basis", pooledMismatch))
	}
	// A share exchange must hold source quantity times its ratio in every
	// destination lot and post exactly those totals in both instruments (#177);
	// a spin-off posts only its new instrument (#180).
	exchanges, err := s.repository.SelfCheckShareExchanges(ctx, snapshot, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}
	var exchangeMismatch int64
	for _, exchange := range exchanges {
		if exchange.Agrees() {
			continue
		}
		exchangeMismatch++
		if len(result.Sample) < db.SelfCheckSampleLimit {
			result.Sample = append(result.Sample, exchange.OperationID)
			sampleReferences = append(sampleReferences, fmt.Sprintf("operation #%d", exchange.OperationID))
		}
	}
	if exchangeMismatch > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount += exchangeMismatch
		summaries = append(summaries, fmt.Sprintf("%d share exchanges or spin-offs do not convert their lots at the recorded ratio or post a different journal", exchangeMismatch))
	}
	// An outbound transfer's bridge must post to the transfer equity account
	// exactly the basis its links carried out, in each cost currency.
	bridges, err := s.repository.SelfCheckTransferBridges(ctx, snapshot, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}
	var bridgeMismatch int64
	for _, bridge := range bridges {
		// An unknown outbound posts no bridge until its basis is resolved.
		if bridge.Unknown && bridge.Bridged.Sign() == 0 || !bridge.Unknown && bridge.Carried.Cmp(bridge.Bridged) == 0 {
			continue
		}
		bridgeMismatch++
		if len(result.Sample) < db.SelfCheckSampleLimit {
			result.Sample = append(result.Sample, bridge.OperationID)
			sampleReferences = append(sampleReferences, fmt.Sprintf("operation #%d", bridge.OperationID))
		}
	}
	if bridgeMismatch > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount += bridgeMismatch
		summaries = append(summaries, fmt.Sprintf("%d outbound transfers bridge a different basis than their links carried", bridgeMismatch))
	}
	capitalReturns, err := s.repository.SelfCheckCapitalReturns(ctx, snapshot, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}
	var capitalReturnMismatch int64
	for _, capitalReturn := range capitalReturns {
		if !capitalReturn.Mismatched && capitalReturn.Allocated.Cmp(capitalReturn.Amount) == 0 {
			continue
		}
		capitalReturnMismatch++
		if len(result.Sample) < db.SelfCheckSampleLimit {
			result.Sample = append(result.Sample, capitalReturn.OperationID)
			sampleReferences = append(sampleReferences, fmt.Sprintf("operation #%d", capitalReturn.OperationID))
		}
	}
	if capitalReturnMismatch > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount += capitalReturnMismatch
		summaries = append(summaries, fmt.Sprintf("%d returns of capital do not conserve their receipt across lot effects", capitalReturnMismatch))
	}
	type clearingKey struct {
		operationID, versionID, currencyID int64
	}
	var group clearingKey
	var decisionID int64
	var proceeds, clearing *exact.ScaledInt
	var proceedsMismatch int64
	finishGroup := func() {
		if decisionID == 0 || proceeds.Cmp(clearing.Negated()) == 0 {
			return
		}
		proceedsMismatch++
		if len(result.Sample) < db.SelfCheckSampleLimit {
			result.Sample = append(result.Sample, decisionID)
			sampleReferences = append(sampleReferences, fmt.Sprintf("decision #%d (operation #%d, version #%d, currency #%d)",
				decisionID, group.operationID, group.versionID, group.currencyID))
		}
	}
	err = s.repository.StreamDisposalClearing(ctx, snapshot, BookID, func(record db.SelfCheckDisposalClearingRecord) error {
		key := clearingKey{record.OperationID, record.TransactionVersionID, record.CostCommodityID}
		if key != group {
			finishGroup()
			group = key
			decisionID = record.DecisionID
			proceeds = exact.NewScaledInt()
			clearing = exact.NewScaledInt()
		}
		if record.IsPosting {
			clearing.AddCoefficient(record.AmountValue, record.AmountScale)
		} else {
			proceeds.AddCoefficient(record.AmountValue, record.AmountScale)
		}
		return nil
	})
	if err != nil {
		return SelfCheckResult{}, err
	}
	finishGroup()
	if proceedsMismatch > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount += proceedsMismatch
		summaries = append(summaries, fmt.Sprintf("%d disposal proceeds disagree with posted clearing", proceedsMismatch))
	}
	var attributionEntity int64
	var attributionPosting, attributionBad bool
	var expectedAttribution, allocatedAttribution *exact.ScaledInt
	var attributionMismatch int64
	finishAttribution := func() {
		if attributionEntity == 0 || !attributionBad && expectedAttribution.Cmp(allocatedAttribution) == 0 {
			return
		}
		attributionMismatch++
		if len(result.Sample) < db.SelfCheckSampleLimit {
			kind := "decision"
			if attributionPosting {
				kind = "posting"
			}
			result.Sample = append(result.Sample, attributionEntity)
			sampleReferences = append(sampleReferences, fmt.Sprintf("%s #%d", kind, attributionEntity))
		}
	}
	err = s.repository.StreamDisposalClearingAllocations(ctx, snapshot, BookID, func(record db.SelfCheckDisposalClearingAllocationRecord) error {
		if !record.IsAllocation {
			finishAttribution()
			attributionEntity, attributionPosting, attributionBad = record.EntityID, record.IsPosting, false
			expectedAttribution = exact.ScaledIntFromCoefficient(record.AmountValue, record.AmountScale)
			if record.IsPosting {
				expectedAttribution = expectedAttribution.Negated()
			}
			allocatedAttribution = exact.NewScaledInt()
		} else {
			allocatedAttribution.AddCoefficient(record.AmountValue, record.AmountScale)
			attributionBad = attributionBad || !record.ValidRelationship
		}
		return nil
	})
	if err != nil {
		return SelfCheckResult{}, err
	}
	finishAttribution()
	if attributionMismatch > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount += attributionMismatch
		summaries = append(summaries, fmt.Sprintf("%d disposal clearing attribution mismatches", attributionMismatch))
	}
	if result.Status == SelfCheckFailed {
		if len(result.Sample) > db.SelfCheckSampleLimit {
			result.Sample = result.Sample[:db.SelfCheckSampleLimit]
		}
		result.Summary = joinSummaries(summaries)
		if len(sampleReferences) > 0 {
			result.Summary += "; sample references: " + strings.Join(sampleReferences, ", ")
		}
	}
	return result, nil
}

// balanceChecks folds every posted posting once and answers three questions
// from it: does each entry balance, does each transaction, and does the book.
func (s *SelfCheckService) balanceChecks(ctx context.Context, snapshot *sql.Tx) ([]SelfCheckResult, error) {
	type key struct {
		id          int64
		commodityID int64
	}

	entries := map[key]*exact.ScaledInt{}
	transactions := map[key]*exact.ScaledInt{}
	book := map[int64]*exact.ScaledInt{}

	err := s.repository.StreamPostedPostings(ctx, snapshot, BookID, func(record db.SelfCheckPostingRecord) error {
		entryKey := key{id: record.JournalEntryID, commodityID: record.CommodityID}
		if entries[entryKey] == nil {
			entries[entryKey] = exact.NewScaledInt()
		}
		entries[entryKey].AddCoefficient(record.QuantityValue, record.QuantityScale)

		transactionKey := key{id: record.TransactionID, commodityID: record.CommodityID}
		if transactions[transactionKey] == nil {
			transactions[transactionKey] = exact.NewScaledInt()
		}
		transactions[transactionKey].AddCoefficient(record.QuantityValue, record.QuantityScale)

		if book[record.CommodityID] == nil {
			book[record.CommodityID] = exact.NewScaledInt()
		}
		book[record.CommodityID].AddCoefficient(record.QuantityValue, record.QuantityScale)
		return nil
	})
	if err != nil {
		return nil, err
	}

	entryResult := imbalanceResult(CheckEntryBalance, entries, "journal entries do not balance", func(k key) int64 { return k.id })
	transactionResult := imbalanceResult(CheckTransactionBalance, transactions, "transactions do not balance", func(k key) int64 { return k.id })

	bookResult := SelfCheckResult{CheckID: CheckBookBalance, Status: SelfCheckPassed, Summary: "every commodity sums to zero across the book"}
	for commodityID, total := range book {
		if total.Sign() != 0 {
			bookResult.Status = SelfCheckFailed
			bookResult.FindingCount++
			if len(bookResult.Sample) < db.SelfCheckSampleLimit {
				bookResult.Sample = append(bookResult.Sample, commodityID)
			}
		}
	}
	slices.Sort(bookResult.Sample)
	if bookResult.Status == SelfCheckFailed {
		bookResult.Summary = fmt.Sprintf("%d commodities do not sum to zero across the book", bookResult.FindingCount)
	}

	return []SelfCheckResult{entryResult, transactionResult, bookResult}, nil
}

func imbalanceResult[K comparable](checkID string, totals map[K]*exact.ScaledInt, failureLabel string, idOf func(K) int64) SelfCheckResult {
	result := SelfCheckResult{CheckID: checkID, Status: SelfCheckPassed}

	seen := map[int64]bool{}
	for key, total := range totals {
		if total.Sign() == 0 {
			continue
		}
		id := idOf(key)
		if seen[id] {
			continue
		}
		seen[id] = true
		result.FindingCount++
		result.Sample = append(result.Sample, id)
	}
	slices.Sort(result.Sample)
	if len(result.Sample) > db.SelfCheckSampleLimit {
		result.Sample = result.Sample[:db.SelfCheckSampleLimit]
	}

	if result.FindingCount > 0 {
		result.Status = SelfCheckFailed
		result.Summary = fmt.Sprintf("%d %s", result.FindingCount, failureLabel)
	} else {
		result.Summary = "all balance per commodity"
	}
	return result
}

// versionIntegrityCheck asks whether the ledger's rows still belong to each
// other. These are counts of rows that should not exist, which SQL answers
// well — unlike money, which it must never sum here.
func (s *SelfCheckService) versionIntegrityCheck(ctx context.Context, snapshot *sql.Tx) (SelfCheckResult, error) {
	result := SelfCheckResult{CheckID: CheckVersionIntegrity, Status: SelfCheckPassed, Summary: "versions, entries, and postings all belong to each other"}

	queries := []struct {
		label string
		query string
	}{
		{
			label: "transactions with no version",
			query: `
				SELECT t.id FROM transactions t
				WHERE t.book_id = ? AND NOT EXISTS (
					SELECT 1 FROM transaction_versions tv WHERE tv.transaction_id = t.id
				)`,
		},
		{
			label: "postings whose entry belongs to another version",
			query: `
				SELECT pv.id
				FROM posting_versions pv
				JOIN journal_entries je ON je.id = pv.journal_entry_id
				WHERE pv.book_id = ? AND pv.transaction_version_id <> je.transaction_version_id`,
		},
		{
			label: "postings whose line belongs to another transaction",
			query: `
				SELECT pv.id
				FROM posting_versions pv
				JOIN posting_lines pl ON pl.id = pv.posting_line_id
				JOIN transaction_versions tv ON tv.id = pv.transaction_version_id
				WHERE pv.book_id = ? AND pl.transaction_id <> tv.transaction_id`,
		},
	}

	var summaries []string
	for _, check := range queries {
		anomaly, err := s.repository.CountStructuralAnomaly(ctx, snapshot, check.query, BookID)
		if err != nil {
			return SelfCheckResult{}, err
		}
		if anomaly.Count == 0 {
			continue
		}
		result.Status = SelfCheckFailed
		result.FindingCount += anomaly.Count
		result.Sample = append(result.Sample, anomaly.Sample...)
		summaries = append(summaries, fmt.Sprintf("%d %s", anomaly.Count, check.label))
	}

	if result.Status == SelfCheckFailed {
		if len(result.Sample) > db.SelfCheckSampleLimit {
			result.Sample = result.Sample[:db.SelfCheckSampleLimit]
		}
		result.Summary = joinSummaries(summaries)
	}
	return result, nil
}

// priceDependencyCheck keeps the derived-price index honest (#104). The void
// cascade follows price_observation_dependencies, not derivation_json, so a
// link the derivation names but the index lacks is a void that silently stops
// short; an active derived price over a voided source is that miss already
// happened.
func (s *SelfCheckService) priceDependencyCheck(ctx context.Context, snapshot *sql.Tx) (SelfCheckResult, error) {
	result := SelfCheckResult{CheckID: CheckPriceDependencies, Status: SelfCheckPassed,
		Summary: "every derived price's links match its derivation, and none in use depends on a voided observation"}
	checks := []struct {
		label string
		query string
	}{
		{
			label: "derived prices in use that depend on a voided observation",
			query: `
				SELECT DISTINCT dependency.observation_id
				FROM price_observation_dependencies dependency
				JOIN price_observations derived ON derived.id = dependency.observation_id
				JOIN price_observations source ON source.id = dependency.source_observation_id
				WHERE dependency.book_id = ?1
					AND derived.voided_at IS NULL AND source.voided_at IS NOT NULL
				ORDER BY dependency.observation_id`,
		},
		{
			// The same leg rule the write path and migration 0005 index by.
			label: "derived prices whose links disagree with their derivation",
			query: `
				WITH derived AS MATERIALIZED (
					SELECT id, derivation_json FROM price_observations
					WHERE book_id = ?1 AND is_derived = 1
						AND CASE WHEN json_valid(derivation_json) THEN json_type(derivation_json, '$.legs') END = 'array'
				), named AS MATERIALIZED (
					SELECT DISTINCT d.id AS observation_id, json_extract(leg.value, '$.observation_id') AS source_id
					FROM derived d
					JOIN json_each(d.derivation_json, '$.legs') leg
					WHERE CASE WHEN leg.type = 'object' THEN json_type(leg.value, '$.observation_id') END = 'integer'
				)
				SELECT named.observation_id FROM named
				WHERE NOT EXISTS (
					SELECT 1 FROM price_observation_dependencies dependency
					WHERE dependency.observation_id = named.observation_id
						AND dependency.source_observation_id = named.source_id
				)
				UNION
				SELECT dependency.observation_id FROM price_observation_dependencies dependency
				WHERE dependency.book_id = ?1 AND NOT EXISTS (
					SELECT 1 FROM named
					WHERE named.observation_id = dependency.observation_id
						AND named.source_id = dependency.source_observation_id
				)
				ORDER BY 1`,
		},
	}
	var summaries []string
	for _, check := range checks {
		anomaly, err := s.repository.CountStructuralAnomaly(ctx, snapshot, check.query, BookID)
		if err != nil {
			return SelfCheckResult{}, err
		}
		if anomaly.Count == 0 {
			continue
		}
		result.Status = SelfCheckFailed
		result.FindingCount += anomaly.Count
		result.Sample = append(result.Sample, anomaly.Sample...)
		summaries = append(summaries, fmt.Sprintf("%d %s", anomaly.Count, check.label))
	}
	if result.Status == SelfCheckFailed {
		if len(result.Sample) > db.SelfCheckSampleLimit {
			result.Sample = result.Sample[:db.SelfCheckSampleLimit]
		}
		result.Summary = joinSummaries(summaries)
	}
	return result, nil
}

// accountVersionCoverageCheck is the counter behind slice 1's export fallback.
// The fallback exists so a gap can never drop a posting and unbalance an
// export; this is what stops it covering for corrupt data in silence.
func (s *SelfCheckService) accountVersionCoverageCheck(ctx context.Context, snapshot *sql.Tx) (SelfCheckResult, error) {
	anomaly, err := s.repository.CountStructuralAnomaly(ctx, snapshot, `
		SELECT pv.id
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		WHERE tv.book_id = ?
			AND tv.status = 'posted'
			AND t.deleted_at IS NULL
			AND NOT EXISTS (
				SELECT 1 FROM account_versions av
				WHERE av.account_id = pv.account_id AND av.effective_from <= je.entry_date
			)
	`, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}

	result := SelfCheckResult{
		CheckID:      CheckAccountVersionCoverage,
		Status:       SelfCheckPassed,
		FindingCount: anomaly.Count,
		Sample:       anomaly.Sample,
		Summary:      "every posting has an account version in effect on its date",
	}
	if anomaly.Count > 0 {
		result.Status = SelfCheckFailed
		result.Summary = fmt.Sprintf("%d postings predate every version of their own account", anomaly.Count)
	}
	return result, nil
}

// lotReconciliationCheck compares what the lots say a position is with what the
// ledger says it is. A disagreement means cost basis and gains are being
// computed against a position the ledger does not hold.
func (s *SelfCheckService) lotReconciliationCheck(ctx context.Context, snapshot *sql.Tx) (SelfCheckResult, error) {
	lots, err := s.repository.SelfCheckLots(ctx, snapshot, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}
	missing := SelfCheckResult{CheckID: CheckLotReconciliation, Status: SelfCheckFailed, Summary: "lots are missing their current state projection"}
	for _, lot := range lots {
		if lot.MissingProjection {
			missing.FindingCount++
			missing.Sample = appendCapped(missing.Sample, lot.LotID)
		}
	}
	if missing.FindingCount > 0 {
		return withUnavailableBasis(missing, lots), nil
	}
	// Do not derive a projection from damaged allocation evidence. Report the
	// source snapshot and leave the immutable history untouched for inspection.
	allocationCheck, err := s.disposalAllocationCheck(ctx, snapshot)
	if err != nil {
		return SelfCheckResult{}, err
	}
	if allocationCheck.Status == SelfCheckFailed {
		return withUnavailableBasis(allocationCheck, lots), nil
	}

	events, err := s.repository.SelfCheckLotEvents(ctx, snapshot, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}

	result := SelfCheckResult{CheckID: CheckLotReconciliation, Status: SelfCheckPassed, Summary: "lots account for exactly what the holdings hold"}

	type position struct {
		accountID   int64
		commodityID int64
	}
	// Long basis and short opening proceeds are separate amounts; a holding
	// that was short and later long must not net one against the other.
	type basisPosition struct {
		accountID       int64
		commodityID     int64
		costCommodityID int64
		positionSide    string
	}

	remaining := map[position]*exact.ScaledInt{}
	remainingBasis := map[basisPosition]*exact.ScaledInt{}
	unknownBasis := map[basisPosition]bool{}
	eventQuantity := map[int64]*exact.ScaledInt{}
	eventBasis := map[basisPosition]*exact.ScaledInt{}
	var summaries []string
	var negative, overConsumed, negativeBasis, invalidBasis int64

	// A forward split legitimately leaves a lot holding more units than it
	// opened with; the ceiling is the opening plus every effective increase.
	splitIncrease := map[int64]*exact.ScaledInt{}
	for _, event := range events {
		if event.IsSplit && event.QuantityValue.Sign() > 0 {
			if splitIncrease[event.LotID] == nil {
				splitIncrease[event.LotID] = exact.NewScaledInt()
			}
			splitIncrease[event.LotID].AddCoefficient(event.QuantityValue, event.QuantityScale)
		}
	}

	for _, lot := range lots {
		remainingValue := exact.ScaledIntFromCoefficient(lot.RemainingQuantityValue, lot.RemainingQuantityScale)
		originalValue := exact.ScaledIntFromCoefficient(lot.QuantityValue, lot.QuantityScale)
		if increase := splitIncrease[lot.LotID]; increase != nil {
			originalValue.AddScaled(increase)
		}

		if remainingValue.Sign() < 0 {
			negative++
			result.Sample = appendCapped(result.Sample, lot.LotID)
		}
		if remainingValue.Cmp(originalValue) > 0 {
			overConsumed++
			result.Sample = appendCapped(result.Sample, lot.LotID)
		}
		if !lot.InvalidBasisProjection && lot.BasisKnowledge == db.InvestmentBasisKnown && lot.RemainingCostBasisValue < 0 {
			negativeBasis++
			result.Sample = appendCapped(result.Sample, lot.LotID)
		}
		key := position{accountID: lot.AccountID, commodityID: lot.CommodityID}
		if remaining[key] == nil {
			remaining[key] = exact.NewScaledInt()
		}
		// A short lot's remaining units are owed, so the holding carries them
		// as a negative balance.
		if lot.PositionSide == db.PositionSideShort {
			remaining[key].SubScaled(remainingValue)
		} else {
			remaining[key].AddScaled(remainingValue)
		}
		basisKey := basisPosition{accountID: lot.AccountID, commodityID: lot.CommodityID, costCommodityID: lot.CostCommodityID, positionSide: lot.PositionSide}
		if lot.InvalidBasisProjection {
			invalidBasis++
			unknownBasis[basisKey] = true
			result.Sample = appendCapped(result.Sample, lot.LotID)
			continue
		}
		if lot.BasisKnowledge == db.InvestmentBasisUnknown {
			// Supported missing information, not a finding (#313): counted as
			// unavailable coverage below, while every quantity comparison for
			// this lot and its position stays active.
			unknownBasis[basisKey] = true
			continue
		}
		if remainingBasis[basisKey] == nil {
			remainingBasis[basisKey] = exact.NewScaledInt()
		}
		remainingBasis[basisKey].AddInt64(lot.RemainingCostBasisValue, lot.RemainingCostBasisScale)
	}
	lotSide := make(map[int64]string, len(lots))
	for _, lot := range lots {
		lotSide[lot.LotID] = lot.PositionSide
	}
	for _, event := range events {
		if eventQuantity[event.LotID] == nil {
			eventQuantity[event.LotID] = exact.NewScaledInt()
		}
		eventQuantity[event.LotID].AddCoefficient(event.QuantityValue, event.QuantityScale)
		key := basisPosition{accountID: event.AccountID, commodityID: event.CommodityID, costCommodityID: event.CostCommodityID, positionSide: lotSide[event.LotID]}
		if event.BasisKnowledge == db.InvestmentBasisUnknown {
			unknownBasis[key] = true
			continue
		}
		if eventBasis[key] == nil {
			eventBasis[key] = exact.NewScaledInt()
		}
		eventBasis[key].AddInt64(event.CostBasisValue, event.CostBasisScale)
	}

	holdings := map[position]*exact.ScaledInt{}
	err = s.repository.StreamPostedInvestmentPostings(ctx, snapshot, BookID, func(record db.SelfCheckPostingRecord) error {
		key := position{accountID: record.AccountID, commodityID: record.CommodityID}
		if holdings[key] == nil {
			holdings[key] = exact.NewScaledInt()
		}
		holdings[key].AddCoefficient(record.QuantityValue, record.QuantityScale)
		return nil
	})
	if err != nil {
		return SelfCheckResult{}, err
	}

	var mismatched, eventQuantityMismatch, basisMismatch int64
	allPositions := map[position]bool{}
	for key := range remaining {
		allPositions[key] = true
	}
	for key := range holdings {
		allPositions[key] = true
	}
	for key := range allPositions {
		lotTotal := remaining[key]
		if lotTotal == nil {
			lotTotal = exact.NewScaledInt()
		}
		holding := holdings[key]
		if holding == nil {
			holding = exact.NewScaledInt()
		}
		if lotTotal.Cmp(holding) != 0 {
			mismatched++
			result.Sample = appendCapped(result.Sample, key.accountID)
		}
	}
	for _, lot := range lots {
		expected := eventQuantity[lot.LotID]
		if expected == nil {
			expected = exact.NewScaledInt()
		}
		actual := exact.ScaledIntFromCoefficient(lot.RemainingQuantityValue, lot.RemainingQuantityScale)
		if expected.Cmp(actual) != 0 {
			eventQuantityMismatch++
			result.Sample = appendCapped(result.Sample, lot.LotID)
		}
	}
	allBasisPositions := map[basisPosition]bool{}
	for key := range remainingBasis {
		allBasisPositions[key] = true
	}
	for key := range eventBasis {
		allBasisPositions[key] = true
	}
	for key := range allBasisPositions {
		if unknownBasis[key] {
			continue
		}
		projected := remainingBasis[key]
		if projected == nil {
			projected = exact.NewScaledInt()
		}
		fromEvents := eventBasis[key]
		if fromEvents == nil {
			fromEvents = exact.NewScaledInt()
		}
		if projected.Cmp(fromEvents) != 0 {
			basisMismatch++
			result.Sample = appendCapped(result.Sample, key.accountID)
		}
	}

	if negative > 0 {
		summaries = append(summaries, fmt.Sprintf("%d lots have negative remaining quantity", negative))
	}
	if invalidBasis > 0 {
		summaries = append(summaries, fmt.Sprintf("%d lots have inconsistent projected basis knowledge and amounts", invalidBasis))
	}
	if overConsumed > 0 {
		summaries = append(summaries, fmt.Sprintf("%d lots have more remaining than they ever held", overConsumed))
	}
	if negativeBasis > 0 {
		summaries = append(summaries, fmt.Sprintf("%d lots have negative remaining basis", negativeBasis))
	}
	if mismatched > 0 {
		summaries = append(summaries, fmt.Sprintf("%d holdings disagree with their current lot projection", mismatched))
	}
	if eventQuantityMismatch > 0 {
		summaries = append(summaries, fmt.Sprintf("%d lots disagree with their quantity events", eventQuantityMismatch))
	}
	if basisMismatch > 0 {
		summaries = append(summaries, fmt.Sprintf("%d positions disagree with their basis events", basisMismatch))
	}
	if len(summaries) > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount = negative + overConsumed + negativeBasis + mismatched + eventQuantityMismatch + basisMismatch + invalidBasis
		result.Summary = joinSummaries(summaries)
		slices.Sort(result.Sample)
	}
	if len(lots) == 0 && len(holdings) == 0 {
		result.Summary = "no investment lots or posted holding positions in this book"
	}

	return withUnavailableBasis(result, lots), nil
}

// withUnavailableBasis discloses every lot — open or closed — whose projected
// basis is supported unknown information. It never removes or downgrades a
// finding: on a failed result the unavailable scope is reported beside it,
// and on an otherwise clean result it makes the check incomplete, not passed.
// A lot with inconsistent knowledge and amounts is damage, counted elsewhere.
func withUnavailableBasis(result SelfCheckResult, lots []db.SelfCheckLotRecord) SelfCheckResult {
	for _, lot := range lots {
		if lot.MissingProjection || lot.InvalidBasisProjection || lot.BasisKnowledge != db.InvestmentBasisUnknown {
			continue
		}
		result.UnavailableCount++
		result.UnavailableSample = appendCapped(result.UnavailableSample, lot.LotID)
	}
	if result.UnavailableCount == 0 {
		return result
	}
	slices.Sort(result.UnavailableSample)
	unavailable := fmt.Sprintf("%d lots have unknown projected basis; basis reconciliation is unavailable", result.UnavailableCount)
	if result.Status == SelfCheckFailed {
		result.Summary = joinSummaries([]string{result.Summary, unavailable})
	} else {
		result.Status = SelfCheckIncomplete
		result.Summary = unavailable
	}
	return result
}

// disposalAllocationCheck checks the original snapshot and every replay
// revision independently. Revision basis replaces the original basis, while
// the decision's quantity and signed proceeds remain the conserved facts.
// Quantity and proceeds are checked whatever the basis knowledge. A known
// total must equal its all-known allocations; an unknown total is unresolved
// information only when at least one allocation is unknown, so an invented
// unknown (or a known total over unknown allocations) is damage.
func (s *SelfCheckService) disposalAllocationCheck(ctx context.Context, snapshot *sql.Tx) (SelfCheckResult, error) {
	result := SelfCheckResult{CheckID: CheckLotReconciliation, Status: SelfCheckPassed}
	var decisionID, revisionID int64
	var expectedQuantity, expectedBasis, expectedProceeds *exact.ScaledInt
	var quantity, basis, proceeds *exact.ScaledInt
	var allocationCount, unknownAllocations int
	var expectedUnknown, invalid bool
	var references []string
	finishSet := func() {
		if decisionID == 0 {
			return
		}
		basisConserved := unknownAllocations > 0 && expectedUnknown ||
			unknownAllocations == 0 && !expectedUnknown && basis.Cmp(expectedBasis) == 0
		if allocationCount > 0 && !invalid && quantity.Cmp(expectedQuantity) == 0 &&
			basisConserved && proceeds.Cmp(expectedProceeds) == 0 {
			return
		}
		result.Status = SelfCheckFailed
		result.FindingCount++
		if len(result.Sample) < db.SelfCheckSampleLimit {
			result.Sample = append(result.Sample, decisionID)
			if revisionID == 0 {
				references = append(references, fmt.Sprintf("decision #%d original", decisionID))
			} else {
				references = append(references, fmt.Sprintf("decision #%d revision #%d", decisionID, revisionID))
			}
		}
	}
	err := s.repository.StreamDisposalAllocationSets(ctx, snapshot, BookID, func(record db.SelfCheckDisposalAllocationRecord) error {
		known, value, ok := selfCheckDisposalBasis(record)
		if !record.IsAllocation {
			finishSet()
			decisionID, revisionID = record.DecisionID, record.RevisionID
			expectedQuantity = exact.ScaledIntFromCoefficient(record.QuantityValue, record.QuantityScale)
			expectedBasis = value
			expectedUnknown = !known
			expectedProceeds = exact.ScaledIntFromCoefficient(record.ProceedsValue, record.ProceedsScale)
			quantity, basis, proceeds = exact.NewScaledInt(), exact.NewScaledInt(), exact.NewScaledInt()
			allocationCount, unknownAllocations = 0, 0
			invalid = !ok || !selfCheckDisposalQuantity(record)
			return nil
		}
		allocationCount++
		invalid = invalid || !ok || !selfCheckDisposalQuantity(record)
		quantity.AddCoefficient(record.QuantityValue, record.QuantityScale)
		if known {
			basis.AddScaled(value)
		} else {
			unknownAllocations++
		}
		proceeds.AddCoefficient(record.ProceedsValue, record.ProceedsScale)
		return nil
	})
	if err != nil {
		return SelfCheckResult{}, err
	}
	finishSet()
	if result.FindingCount > 0 {
		result.Summary = fmt.Sprintf("%d disposal allocation sets disagree with quantity, basis or proceeds; sample references: %s",
			result.FindingCount, strings.Join(references, ", "))
	}
	return result, nil
}

// selfCheckDisposalQuantity is a disposal's unit rule: positive, except that
// a cash-and-stock merger's cash decision disposes exactly none (#181).
func selfCheckDisposalQuantity(record db.SelfCheckDisposalAllocationRecord) bool {
	if record.BasisOnly {
		return record.QuantityValue.Sign() == 0
	}
	return record.QuantityValue.Sign() > 0
}

// selfCheckDisposalBasis reads one stored basis knowledge/amount tuple. ok is
// false for an inconsistent pair, an unparseable amount or a negative known
// basis; an unknown basis is valid with no value.
func selfCheckDisposalBasis(record db.SelfCheckDisposalAllocationRecord) (known bool, value *exact.ScaledInt, ok bool) {
	switch {
	case record.BasisKnowledge == db.InvestmentBasisUnknown && !record.BasisValue.Valid && !record.BasisScale.Valid:
		return false, nil, true
	case record.BasisKnowledge == db.InvestmentBasisKnown && record.BasisValue.Valid && record.BasisScale.Valid:
		coefficient, err := exact.Parse(record.BasisValue.String)
		if err != nil || coefficient.Sign() < 0 {
			return true, exact.NewScaledInt(), false
		}
		return true, exact.ScaledIntFromCoefficient(coefficient, int(record.BasisScale.Int64)), true
	default:
		return false, nil, false
	}
}

// checkpointIntegrityCheck asks whether a reconciliation still means what it
// claimed. Two ways it can stop meaning it: the arithmetic no longer adds up,
// or the postings underneath it were superseded without the checkpoint being
// invalidated (the T-53 case).
func (s *SelfCheckService) checkpointIntegrityCheck(ctx context.Context, snapshot *sql.Tx) (SelfCheckResult, error) {
	checkpoints, err := s.repository.SelfCheckActiveCheckpoints(ctx, snapshot, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}

	result := SelfCheckResult{CheckID: CheckCheckpointIntegrity, Status: SelfCheckPassed, Summary: "every active checkpoint still adds up"}
	if len(checkpoints) == 0 {
		result.Summary = "no active reconciliation checkpoints"
		return result, nil
	}

	var mismatched, stale int64
	for _, checkpoint := range checkpoints {
		total := exact.ScaledIntFromCoefficient(checkpoint.StartingBalanceValue, checkpoint.StartingBalanceScale)
		for _, posting := range checkpoint.Postings {
			total.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
		}
		statement := exact.ScaledIntFromCoefficient(checkpoint.StatementBalanceValue, checkpoint.StatementBalanceScale)
		if total.Cmp(statement) != 0 {
			mismatched++
			result.Sample = appendCapped(result.Sample, checkpoint.CheckpointID)
		}
		if checkpoint.StalePostingCount > 0 {
			stale++
			result.Sample = appendCapped(result.Sample, checkpoint.CheckpointID)
		}
	}

	var summaries []string
	if mismatched > 0 {
		summaries = append(summaries, fmt.Sprintf("%d checkpoints no longer sum to their statement balance", mismatched))
	}
	if stale > 0 {
		summaries = append(summaries, fmt.Sprintf("%d checkpoints are active over superseded postings", stale))
	}
	if len(summaries) > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount = mismatched + stale
		result.Summary = joinSummaries(summaries)
		slices.Sort(result.Sample)
	}

	return result, nil
}

func (s *SelfCheckService) sqliteIntegrityCheck(ctx context.Context) (SelfCheckResult, error) {
	outcome, err := s.repository.SQLiteIntegrity(ctx)
	if err != nil {
		return SelfCheckResult{}, err
	}
	if outcome == "ok" {
		return SelfCheckResult{CheckID: CheckSQLiteIntegrity, Status: SelfCheckPassed, Summary: "integrity_check and foreign_key_check both pass"}, nil
	}
	return SelfCheckResult{
		CheckID:      CheckSQLiteIntegrity,
		Status:       SelfCheckFailed,
		FindingCount: 1,
		Summary:      outcome,
	}, nil
}

// attachmentsCheck is the reserved slot. It reports not_applicable rather than
// passing, because passing a check nobody ran is how a coverage claim gets made
// by accident (R14a fills this).
func (s *SelfCheckService) attachmentsCheck() SelfCheckResult {
	return SelfCheckResult{
		CheckID: CheckAttachments,
		Status:  SelfCheckNotApplicable,
		Summary: "attachment storage is not implemented yet, so there is nothing to verify",
	}
}

// commodityPositionSignCheck answers a question the lot reconciliation cannot:
// the lot check compares holdings against lots, so it only sees accounts the
// investment subledger manages. A crypto wallet has no lots at all, and an
// ordinary asset account can be handed an instrument without one, so both can
// be driven below zero by an ordinary balanced entry — a sale recorded before
// the purchase that covers it, or a sale recorded twice. Nothing refuses it at
// the write, because refusing would also refuse a statement imported out of
// order. The entry and book still balance, so this dated diagnostic and the
// net-worth warning must flag the unclassified position.
func (s *SelfCheckService) commodityPositionSignCheck(ctx context.Context, snapshot *sql.Tx) (SelfCheckResult, error) {
	type position struct {
		accountID   int64
		commodityID int64
	}
	balances := map[position]*exact.ScaledInt{}
	negative := map[position]bool{}
	touched := map[position]bool{}
	// Named short lots explain a negative balance up to the units they owe
	// on that date (#173); only the excess stays unclassified.
	shortEvents, err := s.repository.SelfCheckShortLotQuantityEvents(ctx, snapshot, BookID)
	if err != nil {
		return SelfCheckResult{}, err
	}
	owed := map[position]*exact.ScaledInt{}
	nextShort := 0
	date := ""
	checkDate := func() {
		for nextShort < len(shortEvents) && shortEvents[nextShort].EventDate <= date {
			event := shortEvents[nextShort]
			key := position{accountID: event.AccountID, commodityID: event.CommodityID}
			if owed[key] == nil {
				owed[key] = exact.NewScaledInt()
			}
			owed[key].AddCoefficient(event.QuantityValue, event.QuantityScale)
			nextShort++
		}
		for key := range touched {
			explained := exact.NewScaledInt()
			if owed[key] != nil {
				explained = owed[key].Negated()
			}
			if balances[key].Cmp(explained) < 0 {
				negative[key] = true
			}
			delete(touched, key)
		}
	}
	if err := s.repository.StreamPostedNonCurrencyPostings(ctx, snapshot, BookID, func(record db.SelfCheckCommodityPositionRecord) error {
		if date != "" && date != record.EntryDate {
			checkDate()
		}
		date = record.EntryDate
		key := position{accountID: record.AccountID, commodityID: record.CommodityID}
		if balances[key] == nil {
			balances[key] = exact.NewScaledInt()
		}
		balances[key].AddCoefficient(record.QuantityValue, record.QuantityScale)
		touched[key] = true
		return nil
	}); err != nil {
		return SelfCheckResult{}, err
	}
	checkDate()

	result := SelfCheckResult{CheckID: CheckCommodityPositionSign, Status: SelfCheckPassed, Summary: "no dated account position holds an unclassified negative quantity of a countable commodity"}
	keys := make([]position, 0, len(negative))
	for key := range negative {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].accountID != keys[j].accountID {
			return keys[i].accountID < keys[j].accountID
		}
		return keys[i].commodityID < keys[j].commodityID
	})
	for _, key := range keys {
		result.Sample = appendCapped(result.Sample, key.accountID)
	}
	if len(keys) > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount = int64(len(keys))
		result.Summary = fmt.Sprintf("%d account positions held an unclassified negative quantity of a countable commodity on at least one date", len(keys))
	}
	return result, nil
}

func appendCapped(sample []int64, id int64) []int64 {
	if slices.Contains(sample, id) {
		return sample
	}
	if len(sample) >= db.SelfCheckSampleLimit {
		return sample
	}
	return append(sample, id)
}

func joinSummaries(summaries []string) string {
	switch len(summaries) {
	case 0:
		return ""
	case 1:
		return summaries[0]
	default:
		var joined strings.Builder
		joined.WriteString(summaries[0])
		for _, summary := range summaries[1:] {
			joined.WriteString("; " + summary)
		}
		return joined.String()
	}
}

func (s *SelfCheckService) SetSettings(settings *SettingsService) { s.settings = settings }
