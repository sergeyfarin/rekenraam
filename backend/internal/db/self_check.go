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

// SelfCheckDisposalClearingRecord streams each decision and each matching
// clearing posting once per operation/version/currency group. The caller
// sums and compares coefficients with exact arithmetic, never SQLite SUM.
type SelfCheckDisposalClearingRecord struct {
	OperationID          int64
	TransactionVersionID int64
	CostCommodityID      int64
	IsPosting            bool
	DecisionID           int64
	AmountValue          exact.Coefficient
	AmountScale          int
}

// Each allocation set starts with its immutable decision/revision totals,
// followed by its allocation rows. A header with no following rows represents
// missing evidence, rather than an inner join silently excluding that set.
type SelfCheckDisposalAllocationRecord struct {
	DecisionID    int64
	RevisionID    int64
	RevisionSeq   int
	IsAllocation  bool
	QuantityValue exact.Coefficient
	QuantityScale int
	BasisValue    exact.Coefficient
	BasisScale    int
	ProceedsValue exact.Coefficient
	ProceedsScale int
}

func (r *SelfCheckRepository) StreamDisposalAllocationSets(ctx context.Context, transaction *sql.Tx, bookID int64, visit func(SelfCheckDisposalAllocationRecord) error) error {
	rows, err := transaction.QueryContext(ctx, `
		WITH decisions AS (
			SELECT * FROM investment_disposal_decisions WHERE book_id = ?
		), revisions AS (
			SELECT revision.* FROM investment_disposal_revisions revision
			JOIN decisions d ON d.id = revision.decision_id AND d.book_id = revision.book_id
		)
		SELECT d.id AS decision_id, 0 AS revision_id, 1 AS revision_seq, 0 AS is_allocation, 0 AS allocation_seq,
			d.quantity_value, d.quantity_scale, d.disposed_basis_value, d.disposed_basis_scale, d.proceeds_value, d.proceeds_scale
		FROM decisions d
		UNION ALL
		SELECT d.id, 0, 1, 1, a.allocation_seq,
			a.quantity_value, a.quantity_scale, a.cost_basis_value, a.cost_basis_scale, a.proceeds_value, a.proceeds_scale
		FROM decisions d JOIN investment_disposal_allocations a ON a.decision_id = d.id AND a.book_id = d.book_id
		UNION ALL
		SELECT d.id, revision.id, revision.revision_seq, 0, 0,
			d.quantity_value, d.quantity_scale, revision.disposed_basis_value, revision.disposed_basis_scale, d.proceeds_value, d.proceeds_scale
		FROM revisions revision JOIN decisions d ON d.id = revision.decision_id
		UNION ALL
		SELECT revision.decision_id, revision.id, revision.revision_seq, 1, a.allocation_seq,
			a.quantity_value, a.quantity_scale, a.cost_basis_value, a.cost_basis_scale, a.proceeds_value, a.proceeds_scale
		FROM revisions revision JOIN investment_disposal_revision_allocations a
			ON a.revision_id = revision.id AND a.book_id = revision.book_id
		ORDER BY decision_id, revision_seq, is_allocation, allocation_seq
	`, bookID)
	if err != nil {
		return fmt.Errorf("read disposal allocation sets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var record SelfCheckDisposalAllocationRecord
		var allocationSeq int
		if err := rows.Scan(&record.DecisionID, &record.RevisionID, &record.RevisionSeq, &record.IsAllocation, &allocationSeq,
			&record.QuantityValue, &record.QuantityScale, &record.BasisValue, &record.BasisScale, &record.ProceedsValue, &record.ProceedsScale); err != nil {
			return fmt.Errorf("scan disposal allocation set: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate disposal allocation sets: %w", err)
	}
	return nil
}

// SelfCheckInvestmentComponentRecord joins each source fact to its optional
// posted journal leg. Amount equality is checked by the application with
// exact scaled arithmetic.
type SelfCheckInvestmentComponentRecord struct {
	ComponentID      int64
	ComponentKind    string
	ChargeTreatment  sql.NullString
	SeparatelyPaid   bool
	AmountValue      exact.Coefficient
	AmountScale      int
	AmountDate       string
	CommodityID      int64
	CashAccountID    sql.NullInt64
	ChargeAccountID  sql.NullInt64
	PostingID        sql.NullInt64
	PostingBookID    sql.NullInt64
	PostingAccountID sql.NullInt64
	PostingRole      sql.NullString
	PostingCommodity sql.NullInt64
	PostingValue     sql.NullString
	PostingScale     sql.NullInt64
	PostingDate      sql.NullString
	VersionLinked    bool
}

func (r *SelfCheckRepository) StreamInvestmentComponents(ctx context.Context, transaction *sql.Tx, bookID int64, visit func(SelfCheckInvestmentComponentRecord) error) error {
	rows, err := transaction.QueryContext(ctx, `
		SELECT c.id, c.component_kind, c.charge_treatment, c.separately_paid,
			c.amount_value, c.amount_scale, c.amount_date, c.commodity_id,
			c.cash_account_id, c.charge_account_id,
			pv.id, pv.book_id, pv.account_id, a.system_role, pv.commodity_id,
			pv.quantity_value, pv.quantity_scale, je.entry_date,
			EXISTS (SELECT 1 FROM investment_operation_journal_links l
				WHERE l.operation_id = c.operation_id AND l.transaction_version_id = pv.transaction_version_id)
		FROM investment_operation_components c
		LEFT JOIN posting_versions pv ON pv.id = c.posting_version_id
		LEFT JOIN journal_entries je ON je.id = pv.journal_entry_id
		LEFT JOIN accounts a ON a.id = pv.account_id
		WHERE c.book_id = ? ORDER BY c.id
	`, bookID)
	if err != nil {
		return fmt.Errorf("read investment components for self-check: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var record SelfCheckInvestmentComponentRecord
		if err := rows.Scan(&record.ComponentID, &record.ComponentKind, &record.ChargeTreatment,
			&record.SeparatelyPaid, &record.AmountValue, &record.AmountScale, &record.AmountDate,
			&record.CommodityID, &record.CashAccountID, &record.ChargeAccountID,
			&record.PostingID, &record.PostingBookID, &record.PostingAccountID,
			&record.PostingRole, &record.PostingCommodity, &record.PostingValue,
			&record.PostingScale, &record.PostingDate, &record.VersionLinked); err != nil {
			return fmt.Errorf("scan investment component for self-check: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate investment components for self-check: %w", err)
	}
	return nil
}

// StreamDisposalClearing checks aggregate sell/write-off economics, including
// multiple decisions sharing clearing legs. The separate clearing-allocation
// stream checks individual decisions and posting portions independently.
func (r *SelfCheckRepository) StreamDisposalClearing(ctx context.Context, transaction *sql.Tx, bookID int64, visit func(SelfCheckDisposalClearingRecord) error) error {
	rows, err := transaction.QueryContext(ctx, `
		WITH decisions AS (
			SELECT d.* FROM investment_disposal_decisions d
			JOIN investment_operations o ON o.id = d.operation_id AND o.book_id = d.book_id
			WHERE d.book_id = ? AND o.operation_kind IN ('sell', 'write_off')
		), clearing_groups AS (
			SELECT DISTINCT book_id, operation_id, transaction_version_id, cost_commodity_id
			FROM decisions
		)
		SELECT operation_id, transaction_version_id, cost_commodity_id,
			0 AS is_posting, id AS decision_id, proceeds_value AS amount_value, proceeds_scale AS amount_scale
		FROM decisions
		UNION ALL
		SELECT g.operation_id, g.transaction_version_id, g.cost_commodity_id,
			1, 0, pv.quantity_value, pv.quantity_scale
		FROM clearing_groups g
		JOIN posting_versions pv ON pv.transaction_version_id = g.transaction_version_id
			AND pv.book_id = g.book_id AND pv.commodity_id = g.cost_commodity_id
		JOIN accounts a ON a.id = pv.account_id AND a.book_id = g.book_id
			AND a.system_role = 'commodity_trading'
		ORDER BY operation_id, transaction_version_id, cost_commodity_id, is_posting, decision_id
	`, bookID)
	if err != nil {
		return fmt.Errorf("read disposal clearing: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var record SelfCheckDisposalClearingRecord
		if err := rows.Scan(&record.OperationID, &record.TransactionVersionID, &record.CostCommodityID,
			&record.IsPosting, &record.DecisionID, &record.AmountValue, &record.AmountScale); err != nil {
			return fmt.Errorf("scan disposal clearing: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate disposal clearing: %w", err)
	}
	return nil
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

// StreamPostedInvestmentPostings visits posted security quantities held by
// security-holding accounts. Unlike filtering by lot keys, this also exposes a
// journal-only position after every lot has been closed or removed.
func (r *SelfCheckRepository) StreamPostedInvestmentPostings(ctx context.Context, transaction *sql.Tx, bookID int64, visit func(SelfCheckPostingRecord) error) error {
	rows, err := transaction.QueryContext(ctx, `
		SELECT pv.id, t.id, je.id, pv.account_id, pv.commodity_id, je.entry_date,
			pv.quantity_value, pv.quantity_scale
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		JOIN current_account_versions av ON av.account_id = pv.account_id
		JOIN commodities c ON c.id = pv.commodity_id
		WHERE tv.book_id = ? AND tv.status = 'posted' AND t.deleted_at IS NULL
			AND av.account_kind = 'security_holding' AND c.kind = 'security'
		ORDER BY t.id, je.id, pv.id
	`, bookID)
	if err != nil {
		return fmt.Errorf("read self-check investment postings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var record SelfCheckPostingRecord
		if err := rows.Scan(&record.PostingVersionID, &record.TransactionID, &record.JournalEntryID,
			&record.AccountID, &record.CommodityID, &record.EntryDate,
			&record.QuantityValue, &record.QuantityScale); err != nil {
			return fmt.Errorf("scan self-check investment posting: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate self-check investment postings: %w", err)
	}
	return nil
}

// SelfCheckCommodityPositionRecord is one posting of a commodity that exists in
// countable units rather than as money.
type SelfCheckCommodityPositionRecord struct {
	AccountID     int64
	CommodityID   int64
	EntryDate     string
	QuantityValue exact.Coefficient
	QuantityScale int
}

// StreamPostedNonCurrencyPostings visits every posted posting of a commodity
// whose kind is not a currency, skipping the commodity_trading clearing
// account. That account is the counterparty of every commodity movement and is
// negative by construction; every other account holds units it either has or
// does not.
func (r *SelfCheckRepository) StreamPostedNonCurrencyPostings(ctx context.Context, transaction *sql.Tx, bookID int64, visit func(SelfCheckCommodityPositionRecord) error) error {
	rows, err := transaction.QueryContext(ctx, `
		SELECT pv.account_id, pv.commodity_id, je.entry_date, pv.quantity_value, pv.quantity_scale
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		JOIN accounts a ON a.id = pv.account_id
		JOIN commodities c ON c.id = pv.commodity_id
		WHERE tv.book_id = ? AND tv.status = 'posted' AND t.deleted_at IS NULL
			AND c.kind <> 'currency'
			AND (a.system_role IS NULL OR a.system_role <> 'commodity_trading')
		ORDER BY je.entry_date, pv.id
	`, bookID)
	if err != nil {
		return fmt.Errorf("read self-check commodity positions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var record SelfCheckCommodityPositionRecord
		if err := rows.Scan(&record.AccountID, &record.CommodityID, &record.EntryDate, &record.QuantityValue, &record.QuantityScale); err != nil {
			return fmt.Errorf("scan self-check commodity position: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate self-check commodity positions: %w", err)
	}
	return nil
}

// SelfCheckLotRecord is one investment lot's current standing.
type SelfCheckLotRecord struct {
	InvalidBasisProjection  bool
	BasisKnowledge          string
	MissingProjection       bool
	LotID                   int64
	AccountID               int64
	CommodityID             int64
	Status                  string
	QuantityValue           exact.Coefficient
	QuantityScale           int
	RemainingQuantityValue  exact.Coefficient
	RemainingQuantityScale  int
	CostBasisValue          int64
	CostBasisScale          int
	RemainingCostBasisValue int64
	RemainingCostBasisScale int
	CostCommodityID         int64
}

func (r *SelfCheckRepository) SelfCheckLots(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckLotRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT lot.id, lot.account_id, lot.commodity_id, COALESCE(state.status, ''),
		lot.quantity_value, lot.quantity_scale, COALESCE(state.remaining_quantity_value, '0'), COALESCE(state.remaining_quantity_scale, 0),
		lot.cost_basis_value, lot.cost_basis_scale, state.remaining_cost_basis_value,
		state.remaining_cost_basis_scale, lot.cost_commodity_id, state.lot_id IS NULL, COALESCE(state.basis_knowledge, '')
		FROM investment_lots lot LEFT JOIN investment_lot_state state ON state.lot_id = lot.id AND state.book_id = lot.book_id
		WHERE lot.book_id = ? ORDER BY lot.account_id, lot.commodity_id, lot.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check lots: %w", err)
	}
	defer rows.Close()

	var lots []SelfCheckLotRecord
	for rows.Next() {
		var lot SelfCheckLotRecord
		var basisValue, basisScale sql.NullInt64
		if err := rows.Scan(&lot.LotID, &lot.AccountID, &lot.CommodityID, &lot.Status,
			&lot.QuantityValue, &lot.QuantityScale,
			&lot.RemainingQuantityValue, &lot.RemainingQuantityScale,
			&lot.CostBasisValue, &lot.CostBasisScale,
			&basisValue, &basisScale, &lot.CostCommodityID, &lot.MissingProjection, &lot.BasisKnowledge); err != nil {
			return nil, fmt.Errorf("scan self-check lot: %w", err)
		}
		if !lot.MissingProjection {
			lot.RemainingCostBasisValue, lot.RemainingCostBasisScale, err = projectedBasis(basisValue, basisScale, lot.BasisKnowledge)
			if err != nil {
				lot.InvalidBasisProjection = true
			}
		}
		lots = append(lots, lot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate self-check lots: %w", err)
	}
	return lots, nil
}

type SelfCheckLotEventRecord struct {
	LotID           int64
	AccountID       int64
	CommodityID     int64
	CostCommodityID int64
	QuantityValue   exact.Coefficient
	QuantityScale   int
	CostBasisValue  int64
	CostBasisScale  int
}

func (r *SelfCheckRepository) SelfCheckLotEvents(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckLotEventRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT le.lot_id, l.account_id, l.commodity_id, l.cost_commodity_id,
			le.quantity_value, le.quantity_scale, le.cost_basis_value, le.cost_basis_scale
		FROM investment_lot_events le
		JOIN current_investment_lots l ON l.id = le.lot_id
		WHERE le.book_id = ?
			AND NOT EXISTS (
				SELECT 1 FROM investment_operation_lot_effects effect
				JOIN investment_operations successor ON successor.correction_of_operation_id = effect.operation_id
				WHERE effect.lot_event_id = le.id
			)
			AND NOT EXISTS (
				SELECT 1 FROM investment_disposal_allocations original
				JOIN investment_disposal_revisions revision ON revision.decision_id = original.decision_id
				WHERE original.lot_event_id = le.id
			)
		ORDER BY le.lot_id, le.event_date, le.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check lot events: %w", err)
	}
	var events []SelfCheckLotEventRecord
	for rows.Next() {
		var event SelfCheckLotEventRecord
		if err := rows.Scan(&event.LotID, &event.AccountID, &event.CommodityID, &event.CostCommodityID,
			&event.QuantityValue, &event.QuantityScale, &event.CostBasisValue, &event.CostBasisScale); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan self-check lot event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate self-check lot events: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close self-check lot events: %w", err)
	}
	// Replayed disposal events remain in history. Reconcile the current lot
	// projection against the latest effective allocation set instead.
	revised, err := transaction.QueryContext(ctx, `
		SELECT allocation.lot_id, l.account_id, l.commodity_id, l.cost_commodity_id,
			allocation.quantity_value, allocation.quantity_scale,
			allocation.cost_basis_value, allocation.cost_basis_scale
		FROM investment_disposal_revisions revision
		JOIN investment_disposal_decisions decision ON decision.id = revision.decision_id
		JOIN investment_disposal_revision_allocations allocation ON allocation.revision_id = revision.id
		JOIN current_investment_lots l ON l.id = allocation.lot_id
		WHERE revision.book_id = ? AND revision.revision_seq = (
			SELECT MAX(latest.revision_seq) FROM investment_disposal_revisions latest
			WHERE latest.decision_id = revision.decision_id)
			AND NOT EXISTS (SELECT 1 FROM investment_operations successor
				WHERE successor.correction_of_operation_id = decision.operation_id)
		ORDER BY revision.decision_id, allocation.allocation_seq`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read effective self-check allocations: %w", err)
	}
	for revised.Next() {
		var event SelfCheckLotEventRecord
		var quantity exact.Coefficient
		var basis int64
		if err := revised.Scan(&event.LotID, &event.AccountID, &event.CommodityID,
			&event.CostCommodityID, &quantity, &event.QuantityScale,
			&basis, &event.CostBasisScale); err != nil {
			revised.Close()
			return nil, fmt.Errorf("scan effective self-check allocation: %w", err)
		}
		if basis < 0 || quantity.Sign() <= 0 {
			revised.Close()
			return nil, fmt.Errorf("invalid effective self-check allocation for lot %d", event.LotID)
		}
		event.QuantityValue = quantity.Negated()
		event.CostBasisValue = -basis
		events = append(events, event)
	}
	if err := revised.Err(); err != nil {
		revised.Close()
		return nil, fmt.Errorf("iterate effective self-check allocations: %w", err)
	}
	if err := revised.Close(); err != nil {
		return nil, fmt.Errorf("close effective self-check allocations: %w", err)
	}
	return events, nil
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
	ErrorSummary     string
	StartedAt        string
	FinishedAt       sql.NullString
	CreatedAt        string
}

func (r *SelfCheckRepository) MarkSelfCheckRunErrored(ctx context.Context, runID int64, finishedAt string, summary string) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE self_check_runs
		SET status = 'errored', error_summary = ?, finished_at = ?
		WHERE id = ? AND status = 'running'
	`, summary, finishedAt, runID)
	if err != nil {
		return fmt.Errorf("mark self-check run errored: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read errored self-check rows affected: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("mark self-check run errored: updated %d rows", updated)
	}
	return nil
}

// RecoverInterruptedSelfCheckRuns closes out every run this book still shows
// as "running". A run's own process is the only thing ever meant to move it
// out of that state, so any row still in it when a new process starts belongs
// to one that did not survive to finish — a kill, a crash, a power loss
// between CreateSelfCheckRun and any later write, none of which the T-71
// in-process error handling can reach because that code never regains control.
func (r *SelfCheckRepository) RecoverInterruptedSelfCheckRuns(ctx context.Context, bookID int64, finishedAt string, summary string) (int64, error) {
	result, err := r.database.ExecContext(ctx, `
		UPDATE self_check_runs
		SET status = 'errored', error_summary = ?, finished_at = ?
		WHERE book_id = ? AND status = 'running'
	`, summary, finishedAt, bookID)
	if err != nil {
		return 0, fmt.Errorf("recover interrupted self-check runs: %w", err)
	}
	recovered, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read recovered self-check rows affected: %w", err)
	}
	return recovered, nil
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
		SELECT id, book_id, trigger, status, failed_check_count, error_summary, started_at, finished_at, created_at
		FROM self_check_runs WHERE id = ?
	`, id).Scan(&run.ID, &run.BookID, &run.Trigger, &run.Status, &run.FailedCheckCount, &run.ErrorSummary,
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
		SELECT id, book_id, trigger, status, failed_check_count, error_summary, started_at, finished_at, created_at
		FROM self_check_runs
		WHERE book_id = ? AND status IN ('passed', 'failed', 'errored')
		ORDER BY started_at DESC, id DESC
		LIMIT 1
	`, bookID).Scan(&run.ID, &run.BookID, &run.Trigger, &run.Status, &run.FailedCheckCount, &run.ErrorSummary,
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
