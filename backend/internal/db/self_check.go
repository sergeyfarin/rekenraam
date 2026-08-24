package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/exact"
)

// SelfCheckRepository reads through the read-only pool and writes only its own
// run records through the writer. The asymmetry is the point: a check that
// could reach a ledger table would be a different feature, and a far more
// dangerous one.
type SelfCheckRepository struct {
	database *sql.DB
	readOnly *sql.DB
}

func NewSelfCheckRepository(database *sql.DB, readOnly *sql.DB) *SelfCheckRepository {
	return &SelfCheckRepository{database: database, readOnly: readOnly}
}

// Snapshot begins the read transaction every check shares, so a run describes
// one state of the book rather than a smear across several.
func (r *SelfCheckRepository) Snapshot(ctx context.Context) (*sql.Tx, error) {
	transaction, err := r.readOnly.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin self-check snapshot: %w", err)
	}
	return transaction, nil
}

// SelfCheckPostingRecord is one posting as the balance checks see it.
type SelfCheckPostingRecord struct {
	PostingVersionID int64
	TransactionID    int64
	JournalEntryID   int64
	AccountID        int64
	CommodityID      int64
	EntryDate        string
	QuantityValue    exact.Coefficient
	QuantityScale    int
}

// StreamPostedPostings visits every posting of the current version of every
// posted, non-deleted transaction. Coefficients come back as strings; every
// total built from them is folded in Go.
func (r *SelfCheckRepository) StreamPostedPostings(ctx context.Context, transaction *sql.Tx, bookID int64, visit func(SelfCheckPostingRecord) error) error {
	rows, err := transaction.QueryContext(ctx, `
		SELECT pv.id, t.id, je.id, pv.account_id, pv.commodity_id, je.entry_date,
			pv.quantity_value, pv.quantity_scale
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		WHERE tv.book_id = ? AND tv.status = 'posted' AND t.deleted_at IS NULL
		ORDER BY t.id, je.id, pv.id
	`, bookID)
	if err != nil {
		return fmt.Errorf("read self-check postings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var record SelfCheckPostingRecord
		if err := rows.Scan(&record.PostingVersionID, &record.TransactionID, &record.JournalEntryID,
			&record.AccountID, &record.CommodityID, &record.EntryDate,
			&record.QuantityValue, &record.QuantityScale); err != nil {
			return fmt.Errorf("scan self-check posting: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate self-check postings: %w", err)
	}
	return nil
}

// SelfCheckLotRecord is one investment lot's current standing.
type SelfCheckLotRecord struct {
	LotID                  int64
	AccountID              int64
	CommodityID            int64
	Status                 string
	QuantityValue          exact.Coefficient
	QuantityScale          int
	RemainingQuantityValue exact.Coefficient
	RemainingQuantityScale int
}

func (r *SelfCheckRepository) SelfCheckLots(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckLotRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT id, account_id, commodity_id, status,
			quantity_value, quantity_scale,
			remaining_quantity_value, remaining_quantity_scale
		FROM investment_lots
		WHERE book_id = ?
		ORDER BY account_id, commodity_id, id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check lots: %w", err)
	}
	defer rows.Close()

	var lots []SelfCheckLotRecord
	for rows.Next() {
		var lot SelfCheckLotRecord
		if err := rows.Scan(&lot.LotID, &lot.AccountID, &lot.CommodityID, &lot.Status,
			&lot.QuantityValue, &lot.QuantityScale,
			&lot.RemainingQuantityValue, &lot.RemainingQuantityScale); err != nil {
			return nil, fmt.Errorf("scan self-check lot: %w", err)
		}
		lots = append(lots, lot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate self-check lots: %w", err)
	}
	return lots, nil
}

// SelfCheckCheckpointRecord is an active reconciliation checkpoint and the
// postings it was built from, as snapshotted at the time.
type SelfCheckCheckpointRecord struct {
	CheckpointID          int64
	AccountID             int64
	CommodityID           int64
	StatementBalanceValue exact.Coefficient
	StatementBalanceScale int
	PostingCount          int64
	StalePostingCount     int64
	Postings              []SelfCheckPostingRecord
}

// SelfCheckActiveCheckpoints returns each active checkpoint with its
// snapshotted postings, plus how many of those postings are no longer part of
// the current version of their transaction.
//
// The second number is the T-53 case: a posting edited after being reconciled
// should have invalidated its checkpoint. A checkpoint still calling itself
// active over superseded postings is a claim the ledger no longer supports.
func (r *SelfCheckRepository) SelfCheckActiveCheckpoints(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckCheckpointRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT id, account_id, commodity_id, statement_balance_value, statement_balance_scale
		FROM reconciliation_checkpoints
		WHERE book_id = ? AND status = 'active'
		ORDER BY id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check checkpoints: %w", err)
	}
	defer rows.Close()

	var checkpoints []SelfCheckCheckpointRecord
	for rows.Next() {
		var checkpoint SelfCheckCheckpointRecord
		if err := rows.Scan(&checkpoint.CheckpointID, &checkpoint.AccountID, &checkpoint.CommodityID,
			&checkpoint.StatementBalanceValue, &checkpoint.StatementBalanceScale); err != nil {
			return nil, fmt.Errorf("scan self-check checkpoint: %w", err)
		}
		checkpoints = append(checkpoints, checkpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate self-check checkpoints: %w", err)
	}

	for index := range checkpoints {
		postings, stale, err := r.checkpointPostings(ctx, transaction, checkpoints[index].CheckpointID)
		if err != nil {
			return nil, err
		}
		checkpoints[index].Postings = postings
		checkpoints[index].PostingCount = int64(len(postings))
		checkpoints[index].StalePostingCount = stale
	}

	return checkpoints, nil
}

func (r *SelfCheckRepository) checkpointPostings(ctx context.Context, transaction *sql.Tx, checkpointID int64) ([]SelfCheckPostingRecord, int64, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT
			cp.posting_version_id,
			cp.account_id,
			cp.commodity_id,
			cp.entry_date,
			cp.quantity_value,
			cp.quantity_scale,
			CASE WHEN EXISTS (
				SELECT 1
				FROM posting_versions pv
				JOIN current_transaction_versions tv ON tv.id = pv.transaction_version_id
				WHERE pv.id = cp.posting_version_id
			) THEN 0 ELSE 1 END AS stale
		FROM reconciliation_checkpoint_postings cp
		WHERE cp.checkpoint_id = ?
		ORDER BY cp.posting_version_id
	`, checkpointID)
	if err != nil {
		return nil, 0, fmt.Errorf("read checkpoint postings: %w", err)
	}
	defer rows.Close()

	var postings []SelfCheckPostingRecord
	var stale int64
	for rows.Next() {
		var posting SelfCheckPostingRecord
		var isStale int64
		if err := rows.Scan(&posting.PostingVersionID, &posting.AccountID, &posting.CommodityID,
			&posting.EntryDate, &posting.QuantityValue, &posting.QuantityScale, &isStale); err != nil {
			return nil, 0, fmt.Errorf("scan checkpoint posting: %w", err)
		}
		postings = append(postings, posting)
		stale += isStale
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate checkpoint postings: %w", err)
	}

	return postings, stale, nil
}

// StructuralAnomaly is one structural problem and a sample of where it is.
type StructuralAnomaly struct {
	Count  int64
	Sample []int64
}

// CountStructuralAnomaly runs one of the version-integrity queries. These are
// counts of rows that should not exist, which SQL answers well — unlike money,
// which it must never sum here.
func (r *SelfCheckRepository) CountStructuralAnomaly(ctx context.Context, transaction *sql.Tx, query string, args ...any) (StructuralAnomaly, error) {
	rows, err := transaction.QueryContext(ctx, query, args...)
	if err != nil {
		return StructuralAnomaly{}, fmt.Errorf("run structural check: %w", err)
	}
	defer rows.Close()

	anomaly := StructuralAnomaly{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return StructuralAnomaly{}, fmt.Errorf("scan structural check: %w", err)
		}
		anomaly.Count++
		if len(anomaly.Sample) < SelfCheckSampleLimit {
			anomaly.Sample = append(anomaly.Sample, id)
		}
	}
	if err := rows.Err(); err != nil {
		return StructuralAnomaly{}, fmt.Errorf("iterate structural check: %w", err)
	}

	return anomaly, nil
}

// SelfCheckSampleLimit caps how many offending ids a result carries. Enough to
// go and look; not so many that a broken book produces a wall of numbers.
const SelfCheckSampleLimit = 20

// SQLiteIntegrity runs the two PRAGMA checks against the live database through
// the read pool.
func (r *SelfCheckRepository) SQLiteIntegrity(ctx context.Context) (string, error) {
	var result string
	if err := r.readOnly.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return "", fmt.Errorf("run integrity_check: %w", err)
	}
	if !strings.EqualFold(result, "ok") {
		return result, nil
	}

	rows, err := r.readOnly.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return "", fmt.Errorf("run foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table sql.NullString
		var rowid sql.NullInt64
		var parent sql.NullString
		var constraintIndex sql.NullInt64
		if err := rows.Scan(&table, &rowid, &parent, &constraintIndex); err != nil {
			return "", fmt.Errorf("scan foreign_key_check: %w", err)
		}
		return fmt.Sprintf("foreign key violation in %s referencing %s", table.String, parent.String), nil
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate foreign_key_check: %w", err)
	}

	return "ok", nil
}

// --- Run records: the only thing the self-check writes ---

type SelfCheckRunRecord struct {
	ID               int64
	BookID           int64
	Trigger          string
	Status           string
	FailedCheckCount int64
	StartedAt        string
	FinishedAt       sql.NullString
	CreatedAt        string
}

type SelfCheckResultRecord struct {
	RunID        int64
	CheckID      string
	Status       string
	FindingCount int64
	SampleJSON   string
	Summary      string
}

func (r *SelfCheckRepository) CreateSelfCheckRun(ctx context.Context, bookID int64, trigger string, now string) (SelfCheckRunRecord, error) {
	result, err := r.database.ExecContext(ctx, `
		INSERT INTO self_check_runs (book_id, trigger, status, started_at, created_at)
		VALUES (?, ?, 'running', ?, ?)
	`, bookID, trigger, now, now)
	if err != nil {
		return SelfCheckRunRecord{}, fmt.Errorf("create self-check run: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return SelfCheckRunRecord{}, fmt.Errorf("read self-check run id: %w", err)
	}
	return r.SelfCheckRunByID(ctx, id)
}

func (r *SelfCheckRepository) SaveSelfCheckResults(ctx context.Context, runID int64, status string, failedCount int64, finishedAt string, results []SelfCheckResultRecord) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin self-check results: %w", err)
	}
	defer rollbackTx(ctx, tx)

	for _, result := range results {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO self_check_results (run_id, check_id, status, finding_count, sample_json, summary)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(run_id, check_id) DO UPDATE SET
				status = excluded.status,
				finding_count = excluded.finding_count,
				sample_json = excluded.sample_json,
				summary = excluded.summary
		`, runID, result.CheckID, result.Status, result.FindingCount, result.SampleJSON, result.Summary); err != nil {
			return fmt.Errorf("save self-check result: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE self_check_runs SET status = ?, failed_check_count = ?, finished_at = ? WHERE id = ?
	`, status, failedCount, finishedAt, runID); err != nil {
		return fmt.Errorf("finish self-check run: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit self-check results: %w", err)
	}
	return nil
}

func (r *SelfCheckRepository) SelfCheckRunByID(ctx context.Context, id int64) (SelfCheckRunRecord, error) {
	var run SelfCheckRunRecord
	err := r.database.QueryRowContext(ctx, `
		SELECT id, book_id, trigger, status, failed_check_count, started_at, finished_at, created_at
		FROM self_check_runs WHERE id = ?
	`, id).Scan(&run.ID, &run.BookID, &run.Trigger, &run.Status, &run.FailedCheckCount,
		&run.StartedAt, &run.FinishedAt, &run.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SelfCheckRunRecord{}, ErrNotFound
	}
	if err != nil {
		return SelfCheckRunRecord{}, fmt.Errorf("read self-check run: %w", err)
	}
	return run, nil
}

// LatestSelfCheckRun returns the most recent finished run, which is what a
// screen shows without making anyone wait for a new one.
func (r *SelfCheckRepository) LatestSelfCheckRun(ctx context.Context, bookID int64) (SelfCheckRunRecord, []SelfCheckResultRecord, error) {
	var run SelfCheckRunRecord
	err := r.database.QueryRowContext(ctx, `
		SELECT id, book_id, trigger, status, failed_check_count, started_at, finished_at, created_at
		FROM self_check_runs
		WHERE book_id = ? AND status IN ('passed', 'failed')
		ORDER BY started_at DESC, id DESC
		LIMIT 1
	`, bookID).Scan(&run.ID, &run.BookID, &run.Trigger, &run.Status, &run.FailedCheckCount,
		&run.StartedAt, &run.FinishedAt, &run.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SelfCheckRunRecord{}, nil, ErrNotFound
	}
	if err != nil {
		return SelfCheckRunRecord{}, nil, fmt.Errorf("read latest self-check run: %w", err)
	}

	results, err := r.SelfCheckResults(ctx, run.ID)
	if err != nil {
		return SelfCheckRunRecord{}, nil, err
	}
	return run, results, nil
}

func (r *SelfCheckRepository) SelfCheckResults(ctx context.Context, runID int64) ([]SelfCheckResultRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT run_id, check_id, status, finding_count, sample_json, summary
		FROM self_check_results
		WHERE run_id = ?
		ORDER BY id
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("read self-check results: %w", err)
	}
	defer rows.Close()

	var results []SelfCheckResultRecord
	for rows.Next() {
		var result SelfCheckResultRecord
		if err := rows.Scan(&result.RunID, &result.CheckID, &result.Status, &result.FindingCount,
			&result.SampleJSON, &result.Summary); err != nil {
			return nil, fmt.Errorf("scan self-check result: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate self-check results: %w", err)
	}
	return results, nil
}
