package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	CheckCheckpointIntegrity    = "checkpoint_integrity"
	CheckAccountVersionCoverage = "account_version_coverage"
	CheckSQLiteIntegrity        = "sqlite_integrity"
	CheckAttachments            = "attachments"
)

// Result statuses.
const (
	SelfCheckPassed        = "passed"
	SelfCheckFailed        = "failed"
	SelfCheckNotApplicable = "not_applicable"
)

// SelfCheckResult is one check's verdict, in the terms a person needs: what was
// checked, what was found, where to look, and what to do about it.
type SelfCheckResult struct {
	CheckID      string
	Status       string
	FindingCount int64
	Sample       []int64
	Summary      string
	// Explanation and NextStep are not stored: they belong to the check, not to
	// a particular run, and freezing them into old rows would let history
	// contradict the current wording.
	Explanation string
	NextStep    string
}

// SelfCheckRun is a whole run.
type SelfCheckRun struct {
	ID               int64
	Trigger          string
	Status           string
	FailedCheckCount int64
	StartedAt        string
	FinishedAt       string
	Results          []SelfCheckResult
}

type SelfCheckService struct {
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
		explanation: "Open investment lots must account for exactly what the holding account holds, and no lot may have negative or over-consumed remaining quantity.",
		nextStep:    "Compare the named account's holdings with its lots. A mismatch means gains and cost basis are being computed from a position the ledger does not agree with.",
	},
	CheckCheckpointIntegrity: {
		explanation: "An active reconciliation checkpoint must still add up to the statement balance it recorded, from postings that are still current.",
		nextStep:    "Re-reconcile the named account. A checkpoint over superseded postings is asserting a balance the ledger no longer supports.",
	},
	CheckAccountVersionCoverage: {
		explanation: "Every posting must have a version of its account that was in effect on the posting's date.",
		nextStep:    "The app refuses to create these, so a non-zero count means rows arrived from outside it — a backfill, a manual repair, or a restored and patched database. Exports have been quietly falling back to each account's earliest version for these rows.",
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
		return SelfCheckRun{}, err
	}

	var failed int64
	stored := make([]db.SelfCheckResultRecord, 0, len(results))
	for _, result := range results {
		if result.Status == SelfCheckFailed {
			failed++
		}
		sample, marshalErr := json.Marshal(emptyIfNilIDs(result.Sample))
		if marshalErr != nil {
			return SelfCheckRun{}, fmt.Errorf("encode self-check sample: %w", marshalErr)
		}
		stored = append(stored, db.SelfCheckResultRecord{
			RunID:        runRecord.ID,
			CheckID:      result.CheckID,
			Status:       result.Status,
			FindingCount: result.FindingCount,
			SampleJSON:   string(sample),
			Summary:      result.Summary,
		})
	}

	status := SelfCheckPassed
	if failed > 0 {
		status = SelfCheckFailed
	}
	finishedAt := s.now().UTC().Format(time.RFC3339)
	if err := s.repository.SaveSelfCheckResults(ctx, runRecord.ID, status, failed, finishedAt, stored); err != nil {
		return SelfCheckRun{}, err
	}

	return SelfCheckRun{
		ID:               runRecord.ID,
		Trigger:          trigger,
		Status:           status,
		FailedCheckCount: failed,
		StartedAt:        startedAt,
		FinishedAt:       finishedAt,
		Results:          results,
	}, nil
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
		ID:               runRecord.ID,
		Trigger:          runRecord.Trigger,
		Status:           runRecord.Status,
		FailedCheckCount: runRecord.FailedCheckCount,
		StartedAt:        runRecord.StartedAt,
		FinishedAt:       runRecord.FinishedAt.String,
	}
	for _, record := range resultRecords {
		var sample []int64
		_ = json.Unmarshal([]byte(record.SampleJSON), &sample)
		narrative := checkNarratives[record.CheckID]
		run.Results = append(run.Results, SelfCheckResult{
			CheckID:      record.CheckID,
			Status:       record.Status,
			FindingCount: record.FindingCount,
			Sample:       sample,
			Summary:      record.Summary,
			Explanation:  narrative.explanation,
			NextStep:     narrative.nextStep,
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

	lots, err := s.lotReconciliationCheck(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	checkpoints, err := s.checkpointIntegrityCheck(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	integrity, err := s.sqliteIntegrityCheck(ctx)
	if err != nil {
		return nil, err
	}

	results := append(balances, structural, lots, checkpoints, coverage, integrity, s.attachmentsCheck())
	for index := range results {
		narrative := checkNarratives[results[index].CheckID]
		results[index].Explanation = narrative.explanation
		results[index].NextStep = narrative.nextStep
	}
	return results, nil
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
	sort.Slice(bookResult.Sample, func(i, j int) bool { return bookResult.Sample[i] < bookResult.Sample[j] })
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
	sort.Slice(result.Sample, func(i, j int) bool { return result.Sample[i] < result.Sample[j] })
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

	result := SelfCheckResult{CheckID: CheckLotReconciliation, Status: SelfCheckPassed, Summary: "lots account for exactly what the holdings hold"}
	if len(lots) == 0 {
		result.Summary = "no investment lots in this book"
		return result, nil
	}

	type position struct {
		accountID   int64
		commodityID int64
	}

	remaining := map[position]*exact.ScaledInt{}
	var summaries []string
	var negative, overConsumed int64

	for _, lot := range lots {
		remainingValue := exact.ScaledIntFromCoefficient(lot.RemainingQuantityValue, lot.RemainingQuantityScale)
		originalValue := exact.ScaledIntFromCoefficient(lot.QuantityValue, lot.QuantityScale)

		if remainingValue.Sign() < 0 {
			negative++
			result.Sample = appendCapped(result.Sample, lot.LotID)
		}
		if remainingValue.Cmp(originalValue) > 0 {
			overConsumed++
			result.Sample = appendCapped(result.Sample, lot.LotID)
		}
		if lot.Status != "open" {
			continue
		}
		key := position{accountID: lot.AccountID, commodityID: lot.CommodityID}
		if remaining[key] == nil {
			remaining[key] = exact.NewScaledInt()
		}
		remaining[key].AddScaled(remainingValue)
	}

	holdings := map[position]*exact.ScaledInt{}
	err = s.repository.StreamPostedPostings(ctx, snapshot, BookID, func(record db.SelfCheckPostingRecord) error {
		key := position{accountID: record.AccountID, commodityID: record.CommodityID}
		if _, tracked := remaining[key]; !tracked {
			return nil
		}
		if holdings[key] == nil {
			holdings[key] = exact.NewScaledInt()
		}
		holdings[key].AddCoefficient(record.QuantityValue, record.QuantityScale)
		return nil
	})
	if err != nil {
		return SelfCheckResult{}, err
	}

	var mismatched int64
	for key, lotTotal := range remaining {
		holding := holdings[key]
		if holding == nil {
			holding = exact.NewScaledInt()
		}
		if lotTotal.Cmp(holding) != 0 {
			mismatched++
			result.Sample = appendCapped(result.Sample, key.accountID)
		}
	}

	if negative > 0 {
		summaries = append(summaries, fmt.Sprintf("%d lots have negative remaining quantity", negative))
	}
	if overConsumed > 0 {
		summaries = append(summaries, fmt.Sprintf("%d lots have more remaining than they ever held", overConsumed))
	}
	if mismatched > 0 {
		summaries = append(summaries, fmt.Sprintf("%d holdings disagree with their open lots", mismatched))
	}
	if len(summaries) > 0 {
		result.Status = SelfCheckFailed
		result.FindingCount = negative + overConsumed + mismatched
		result.Summary = joinSummaries(summaries)
		sort.Slice(result.Sample, func(i, j int) bool { return result.Sample[i] < result.Sample[j] })
	}

	return result, nil
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
		total := exact.NewScaledInt()
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
		sort.Slice(result.Sample, func(i, j int) bool { return result.Sample[i] < result.Sample[j] })
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

func appendCapped(sample []int64, id int64) []int64 {
	for _, existing := range sample {
		if existing == id {
			return sample
		}
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
		joined := summaries[0]
		for _, summary := range summaries[1:] {
			joined += "; " + summary
		}
		return joined
	}
}
