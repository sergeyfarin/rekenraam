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
	// BasisValue/Scale are NULL exactly when BasisKnowledge is unknown.
	BasisValue     sql.NullString
	BasisScale     sql.NullInt64
	BasisKnowledge string
	ProceedsValue  exact.Coefficient
	ProceedsScale  int
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
			d.quantity_value, d.quantity_scale, d.disposed_basis_value, d.disposed_basis_scale, d.basis_knowledge,
			d.proceeds_value, d.proceeds_scale
		FROM decisions d
		UNION ALL
		SELECT d.id, 0, 1, 1, a.allocation_seq,
			a.quantity_value, a.quantity_scale, a.cost_basis_value, a.cost_basis_scale, a.basis_knowledge,
			a.proceeds_value, a.proceeds_scale
		FROM decisions d JOIN investment_disposal_allocations a ON a.decision_id = d.id AND a.book_id = d.book_id
		UNION ALL
		SELECT d.id, revision.id, revision.revision_seq, 0, 0,
			d.quantity_value, d.quantity_scale, revision.disposed_basis_value, revision.disposed_basis_scale,
			revision.basis_knowledge, d.proceeds_value, d.proceeds_scale
		FROM revisions revision JOIN decisions d ON d.id = revision.decision_id
		UNION ALL
		SELECT revision.decision_id, revision.id, revision.revision_seq, 1, a.allocation_seq,
			a.quantity_value, a.quantity_scale, a.cost_basis_value, a.cost_basis_scale, a.basis_knowledge,
			a.proceeds_value, a.proceeds_scale
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
			&record.QuantityValue, &record.QuantityScale, &record.BasisValue, &record.BasisScale, &record.BasisKnowledge,
			&record.ProceedsValue, &record.ProceedsScale); err != nil {
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
			WHERE d.book_id = ? AND o.operation_kind IN ('sell', 'write_off', 'cash_in_lieu')
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
	// PositionSide is short for borrowed units, which the holding owes (#173).
	PositionSide            string
	OpeningBasisKnowledge   string
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
		state.remaining_cost_basis_scale, lot.cost_commodity_id, state.lot_id IS NULL, COALESCE(state.basis_knowledge, ''), lot.opening_basis_knowledge,
		lot.position_side
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
		var basisValue, basisScale, openingValue, openingScale sql.NullInt64
		if err := rows.Scan(&lot.LotID, &lot.AccountID, &lot.CommodityID, &lot.Status,
			&lot.QuantityValue, &lot.QuantityScale,
			&lot.RemainingQuantityValue, &lot.RemainingQuantityScale,
			&openingValue, &openingScale,
			&basisValue, &basisScale, &lot.CostCommodityID, &lot.MissingProjection, &lot.BasisKnowledge, &lot.OpeningBasisKnowledge, &lot.PositionSide); err != nil {
			return nil, fmt.Errorf("scan self-check lot: %w", err)
		}
		lot.CostBasisValue, lot.CostBasisScale, err = projectedBasis(openingValue, openingScale, lot.OpeningBasisKnowledge)
		if err != nil {
			lot.InvalidBasisProjection = true
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
	BasisKnowledge  string
	LotID           int64
	AccountID       int64
	CommodityID     int64
	CostCommodityID int64
	QuantityValue   exact.Coefficient
	QuantityScale   int
	CostBasisValue  int64
	CostBasisScale  int
	// IsSplit marks an effective split quantity change, which may raise a
	// lot above the quantity it opened with.
	IsSplit bool
}

func (r *SelfCheckRepository) SelfCheckLotEvents(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckLotEventRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT le.lot_id, l.account_id, l.commodity_id, l.cost_commodity_id,
			le.quantity_value, le.quantity_scale, le.cost_basis_value, le.cost_basis_scale,
			le.event_kind = 'split_adjustment', le.basis_knowledge
		FROM effective_investment_lot_events le
		JOIN current_investment_lots l ON l.id = le.lot_id
		WHERE le.book_id = ?
		ORDER BY le.lot_id, le.event_date, le.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check lot events: %w", err)
	}
	var events []SelfCheckLotEventRecord
	for rows.Next() {
		var event SelfCheckLotEventRecord
		var value, scale sql.NullInt64
		if err := rows.Scan(&event.LotID, &event.AccountID, &event.CommodityID, &event.CostCommodityID,
			&event.QuantityValue, &event.QuantityScale, &value, &scale, &event.IsSplit, &event.BasisKnowledge); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan self-check lot event: %w", err)
		}
		event.CostBasisValue, event.CostBasisScale, err = projectedBasis(value, scale, event.BasisKnowledge)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("read self-check event basis: %w", err)
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
			allocation.cost_basis_value, allocation.cost_basis_scale, allocation.basis_knowledge
		FROM latest_investment_disposal_revisions revision
		JOIN investment_disposal_decisions decision ON decision.id = revision.decision_id
		JOIN effective_investment_operations operation ON operation.id = decision.operation_id
		JOIN investment_disposal_revision_allocations allocation ON allocation.revision_id = revision.id
		JOIN current_investment_lots l ON l.id = allocation.lot_id
		WHERE revision.book_id = ?
		ORDER BY revision.decision_id, allocation.allocation_seq`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read effective self-check allocations: %w", err)
	}
	for revised.Next() {
		var event SelfCheckLotEventRecord
		var quantity exact.Coefficient
		var value, scale sql.NullInt64
		if err := revised.Scan(&event.LotID, &event.AccountID, &event.CommodityID,
			&event.CostCommodityID, &quantity, &event.QuantityScale,
			&value, &scale, &event.BasisKnowledge); err != nil {
			revised.Close()
			return nil, fmt.Errorf("scan effective self-check allocation: %w", err)
		}
		basis, basisScale, err := projectedBasis(value, scale, event.BasisKnowledge)
		if err != nil || basis < 0 || quantity.Sign() <= 0 {
			revised.Close()
			return nil, fmt.Errorf("invalid effective self-check allocation for lot %d", event.LotID)
		}
		event.QuantityValue = quantity.Negated()
		event.CostBasisValue, event.CostBasisScale = -basis, basisScale
		events = append(events, event)
	}
	if err := revised.Err(); err != nil {
		revised.Close()
		return nil, fmt.Errorf("iterate effective self-check allocations: %w", err)
	}
	if err := revised.Close(); err != nil {
		return nil, fmt.Errorf("close effective self-check allocations: %w", err)
	}
	// A replayed split's original events are superseded per cost currency by
	// its latest revision, exactly as replayed disposal allocations are.
	splits, err := transaction.QueryContext(ctx, `
		SELECT effect.lot_id, l.account_id, l.commodity_id, l.cost_commodity_id,
			effect.quantity_delta_value, effect.quantity_delta_scale
		FROM latest_investment_split_revisions revision
		JOIN effective_investment_operations operation ON operation.id = revision.operation_id
		JOIN investment_split_revision_effects effect ON effect.revision_id = revision.id
		JOIN current_investment_lots l ON l.id = effect.lot_id
		WHERE revision.book_id = ?
		ORDER BY revision.id, effect.effect_seq`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read effective self-check split effects: %w", err)
	}
	for splits.Next() {
		event := SelfCheckLotEventRecord{IsSplit: true}
		if err := splits.Scan(&event.LotID, &event.AccountID, &event.CommodityID,
			&event.CostCommodityID, &event.QuantityValue, &event.QuantityScale); err != nil {
			splits.Close()
			return nil, fmt.Errorf("scan effective self-check split effect: %w", err)
		}
		events = append(events, event)
	}
	if err := splits.Err(); err != nil {
		splits.Close()
		return nil, fmt.Errorf("iterate effective self-check split effects: %w", err)
	}
	if err := splits.Close(); err != nil {
		return nil, fmt.Errorf("close effective self-check split effects: %w", err)
	}
	// A revised internal transfer moves the same units at its latest carried
	// basis: out of its effective source lot (the corrected successor of the
	// original acquisition, when replaced) and into the destination (T-132).
	// A reversed transfer's revisions are evidence only (T-119).
	transfers, err := transaction.QueryContext(ctx, `
		SELECT revision.source_lot_id, source.account_id, source.commodity_id, source.cost_commodity_id,
			COALESCE(link.destination_lot_id, 0), COALESCE(destination.account_id, 0),
			COALESCE(destination.commodity_id, 0), COALESCE(destination.cost_commodity_id, 0),
			link.quantity_value, link.quantity_scale, revision.carried_basis_value, revision.carried_basis_scale,
			revision.basis_knowledge,
			-- A share exchange's destination holds the link's old units times
			-- its ratio (#177); every other destination holds the link's units.
			COALESCE(destination.quantity_value, link.quantity_value),
			COALESCE(destination.quantity_scale, link.quantity_scale),
			-- A spin-off's parent keeps its units and gives up only basis (#180).
			fact.transfer_kind = 'spin_off'
		FROM latest_investment_transfer_link_revisions revision
		JOIN effective_investment_operations operation ON operation.id = revision.operation_id
		JOIN investment_transfer_facts fact ON fact.operation_id = revision.operation_id
		JOIN investment_transfer_lot_links link ON link.operation_id = revision.operation_id
			AND link.link_seq = revision.link_seq
		JOIN current_investment_lots source ON source.id = revision.source_lot_id
		-- An outbound link (T-143) has no destination: only its source side.
		LEFT JOIN current_investment_lots destination ON destination.id = link.destination_lot_id
		WHERE revision.book_id = ?
		ORDER BY revision.operation_id, revision.link_seq`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read effective self-check transfer revisions: %w", err)
	}
	for transfers.Next() {
		var out, in SelfCheckLotEventRecord
		var quantity, inQuantity exact.Coefficient
		var scale, inScale int
		var basis sql.NullString
		var basisScale sql.NullInt64
		var knowledge string
		var spinOff bool
		if err := transfers.Scan(&out.LotID, &out.AccountID, &out.CommodityID, &out.CostCommodityID,
			&in.LotID, &in.AccountID, &in.CommodityID, &in.CostCommodityID,
			&quantity, &scale, &basis, &basisScale, &knowledge, &inQuantity, &inScale, &spinOff); err != nil {
			transfers.Close()
			return nil, fmt.Errorf("scan effective self-check transfer revision: %w", err)
		}
		value, valueScale, err := selfCheckRevisionBasis(basis, basisScale, knowledge)
		if err != nil || value < 0 || quantity.Sign() <= 0 || inQuantity.Sign() <= 0 {
			transfers.Close()
			return nil, fmt.Errorf("invalid effective self-check transfer revision for lot %d", in.LotID)
		}
		in.QuantityValue, in.QuantityScale, in.CostBasisValue, in.CostBasisScale = inQuantity, inScale, value, valueScale
		out.QuantityValue, out.QuantityScale, out.CostBasisValue, out.CostBasisScale = quantity.Negated(), scale, -value, valueScale
		if spinOff {
			out.QuantityValue, out.QuantityScale = "0", 0
		}
		in.BasisKnowledge, out.BasisKnowledge = knowledge, knowledge
		events = append(events, out)
		if in.LotID > 0 {
			events = append(events, in)
		}
	}
	if err := transfers.Err(); err != nil {
		transfers.Close()
		return nil, fmt.Errorf("iterate effective self-check transfer revisions: %w", err)
	}
	if err := transfers.Close(); err != nil {
		return nil, fmt.Errorf("close effective self-check transfer revisions: %w", err)
	}
	// A revised pooled_lot transfer takes its effective depletions out of the
	// source lots and its whole quantity into the one destination (T-135).
	pooled, err := transaction.QueryContext(ctx, `
		SELECT depletion.source_lot_id, source.account_id, source.commodity_id, source.cost_commodity_id,
			depletion.quantity_value, depletion.quantity_scale, depletion.cost_basis_value, depletion.cost_basis_scale,
			depletion.basis_knowledge, 0
		FROM latest_investment_transfer_link_revisions revision
		JOIN effective_investment_operations operation ON operation.id = revision.operation_id
		JOIN investment_transfer_link_revision_depletions depletion ON depletion.revision_id = revision.id
		JOIN current_investment_lots source ON source.id = depletion.source_lot_id
		WHERE revision.book_id = ? AND revision.source_lot_id IS NULL
		UNION ALL
		SELECT link.destination_lot_id, destination.account_id, destination.commodity_id, destination.cost_commodity_id,
			link.quantity_value, link.quantity_scale, revision.carried_basis_value, revision.carried_basis_scale,
			revision.basis_knowledge, 1
		FROM latest_investment_transfer_link_revisions revision
		JOIN effective_investment_operations operation ON operation.id = revision.operation_id
		JOIN investment_transfer_lot_links link ON link.operation_id = revision.operation_id
			AND link.link_seq = revision.link_seq
		JOIN current_investment_lots destination ON destination.id = link.destination_lot_id
		WHERE revision.book_id = ? AND revision.source_lot_id IS NULL`, bookID, bookID)
	if err != nil {
		return nil, fmt.Errorf("read effective self-check pooled transfer revisions: %w", err)
	}
	for pooled.Next() {
		var event SelfCheckLotEventRecord
		var quantity exact.Coefficient
		var basis sql.NullString
		var basisScale sql.NullInt64
		var incoming bool
		if err := pooled.Scan(&event.LotID, &event.AccountID, &event.CommodityID, &event.CostCommodityID,
			&quantity, &event.QuantityScale, &basis, &basisScale, &event.BasisKnowledge, &incoming); err != nil {
			pooled.Close()
			return nil, fmt.Errorf("scan effective self-check pooled transfer revision: %w", err)
		}
		value, valueScale, err := selfCheckRevisionBasis(basis, basisScale, event.BasisKnowledge)
		event.CostBasisScale = valueScale
		if err != nil || value < 0 || quantity.Sign() <= 0 {
			pooled.Close()
			return nil, fmt.Errorf("invalid effective self-check pooled transfer revision for lot %d", event.LotID)
		}
		event.QuantityValue, event.CostBasisValue = quantity, value
		if !incoming {
			event.QuantityValue, event.CostBasisValue = quantity.Negated(), -value
		}
		events = append(events, event)
	}
	if err := pooled.Err(); err != nil {
		pooled.Close()
		return nil, fmt.Errorf("iterate effective self-check pooled transfer revisions: %w", err)
	}
	if err := pooled.Close(); err != nil {
		return nil, fmt.Errorf("close effective self-check pooled transfer revisions: %w", err)
	}
	// A revised return of capital reduces each lot by its latest revision's
	// reduction; its original basis_reduction events are no longer effective
	// (T-148).
	reductions, err := transaction.QueryContext(ctx, `
		SELECT e.lot_id, l.account_id, l.commodity_id, l.cost_commodity_id, e.reduction_value, e.reduction_scale
		FROM latest_investment_capital_return_revisions revision
		JOIN effective_investment_operations operation ON operation.id = revision.operation_id
		JOIN investment_capital_return_revision_effects e ON e.revision_id = revision.id
		JOIN current_investment_lots l ON l.id = e.lot_id
		WHERE revision.book_id = ?`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read effective self-check capital return revisions: %w", err)
	}
	for reductions.Next() {
		var event SelfCheckLotEventRecord
		var reduction exact.Coefficient
		var scale int
		if err := reductions.Scan(&event.LotID, &event.AccountID, &event.CommodityID, &event.CostCommodityID,
			&reduction, &scale); err != nil {
			reductions.Close()
			return nil, fmt.Errorf("scan effective self-check capital return revision: %w", err)
		}
		value, err := exact.ScaledIntFromCoefficient(reduction, scale).Negated().Int64()
		if err != nil {
			reductions.Close()
			return nil, fmt.Errorf("invalid capital return revision reduction for lot %d", event.LotID)
		}
		event.QuantityValue, event.CostBasisValue, event.CostBasisScale = "0", value, scale
		events = append(events, event)
	}
	if err := errors.Join(reductions.Err(), reductions.Close()); err != nil {
		return nil, fmt.Errorf("read effective self-check capital return revisions: %w", err)
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
	// The balance the reconciling session started from — the previous
	// checkpoint's statement balance — which its cleared postings add to.
	StartingBalanceValue exact.Coefficient
	StartingBalanceScale int
	PostingCount         int64
	StalePostingCount    int64
	Postings             []SelfCheckPostingRecord
}

// SelfCheckActiveCheckpoints returns each active checkpoint with its
// snapshotted postings, plus how many of those postings are no longer part of
// the current version of their transaction.
//
// The second number is the T-53 case: a posting edited after being reconciled
// should have invalidated its checkpoint. A checkpoint still calling itself
// active over superseded postings is a claim the ledger no longer supports.
// Superseded means the posting line's financial facts changed — not merely
// that its transaction gained a version, which a description edit does.
func (r *SelfCheckRepository) SelfCheckActiveCheckpoints(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckCheckpointRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT c.id, c.account_id, c.commodity_id, c.statement_balance_value, c.statement_balance_scale,
			COALESCE(s.starting_balance_value, '0'), COALESCE(s.starting_balance_scale, 0)
		FROM reconciliation_checkpoints c
		LEFT JOIN reconciliation_sessions s ON s.id = c.session_id
		WHERE c.book_id = ? AND c.status = 'active'
		ORDER BY c.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check checkpoints: %w", err)
	}
	defer rows.Close()

	var checkpoints []SelfCheckCheckpointRecord
	for rows.Next() {
		var checkpoint SelfCheckCheckpointRecord
		if err := rows.Scan(&checkpoint.CheckpointID, &checkpoint.AccountID, &checkpoint.CommodityID,
			&checkpoint.StatementBalanceValue, &checkpoint.StatementBalanceScale,
			&checkpoint.StartingBalanceValue, &checkpoint.StartingBalanceScale); err != nil {
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
			-- Stale unless the posting line is still current, posted and not
			-- deleted, with the snapshot's account, commodity, date and
			-- quantity, at a position inside the checkpoint's boundary.
			CASE WHEN EXISTS (
				SELECT 1
				FROM posting_versions current_pv
				JOIN current_transaction_versions tv ON tv.id = current_pv.transaction_version_id
				JOIN transactions t ON t.id = tv.transaction_id
				JOIN journal_entries je ON je.id = current_pv.journal_entry_id
				WHERE current_pv.posting_line_id = snapshot_pv.posting_line_id
					AND tv.status = 'posted'
					AND t.deleted_at IS NULL
					AND current_pv.account_id = cp.account_id
					AND current_pv.commodity_id = cp.commodity_id
					AND je.entry_date = cp.entry_date
					AND current_pv.quantity_value = cp.quantity_value
					AND current_pv.quantity_scale = cp.quantity_scale
					AND (je.entry_date < c.statement_date
						OR (je.entry_date = c.statement_date
							AND current_pv.account_day_sequence <= c.statement_account_sequence))
			) THEN 0 ELSE 1 END AS stale
		FROM reconciliation_checkpoint_postings cp
		JOIN posting_versions snapshot_pv ON snapshot_pv.id = cp.posting_version_id
		JOIN reconciliation_checkpoints c ON c.id = cp.checkpoint_id
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

// SelfCheckSplitDelta compares what a split's linked journals posted to its
// holding with what its effective lot effects moved, across cost currencies.
type SelfCheckSplitDelta struct {
	OperationID int64
	Journal     *exact.ScaledInt
	Effects     *exact.ScaledInt
}

// SelfCheckSplitDeltas reads every split's primary and adjustment journal
// postings on its holding and security, plus the inverse journal of a
// reversal or replacement, and its effective per-lot effects: the latest
// revision per cost currency, else the original events, or nothing once the
// split is no longer effective (T-129).
func (r *SelfCheckRepository) SelfCheckSplitDeltas(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckSplitDelta, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT f.operation_id, pv.quantity_value, pv.quantity_scale,
			EXISTS (SELECT 1 FROM effective_investment_operations e WHERE e.id = f.operation_id)
		FROM investment_split_facts f
		LEFT JOIN investment_operation_journal_links link ON link.book_id = f.book_id
			AND ((link.operation_id = f.operation_id AND link.role IN ('primary', 'split_adjustment'))
				OR EXISTS (SELECT 1 FROM investment_operations successor
					WHERE successor.id = link.operation_id AND successor.correction_of_operation_id = f.operation_id
						AND ((successor.correction_mode = 'reverse' AND link.role = 'primary')
							OR (successor.correction_mode = 'replace' AND link.role = 'reversal'))))
		LEFT JOIN posting_versions pv ON pv.transaction_version_id = link.transaction_version_id
			AND pv.account_id = f.account_id AND pv.commodity_id = f.commodity_id
		WHERE f.book_id = ?
		ORDER BY f.operation_id`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check split journals: %w", err)
	}
	var deltas []SelfCheckSplitDelta
	effective := map[int64]bool{}
	for rows.Next() {
		var operationID int64
		var value sql.NullString
		var scale sql.NullInt64
		var isEffective bool
		if err := rows.Scan(&operationID, &value, &scale, &isEffective); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan self-check split journal: %w", err)
		}
		if len(deltas) == 0 || deltas[len(deltas)-1].OperationID != operationID {
			deltas = append(deltas, SelfCheckSplitDelta{OperationID: operationID,
				Journal: exact.NewScaledInt(), Effects: exact.NewScaledInt()})
			effective[operationID] = isEffective
		}
		if value.Valid {
			quantity, err := exact.Parse(value.String)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("parse self-check split journal posting: %w", err)
			}
			deltas[len(deltas)-1].Journal.AddCoefficient(quantity, int(scale.Int64))
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("read self-check split journals: %w", err)
	}
	for index := range deltas {
		if !effective[deltas[index].OperationID] {
			continue
		}
		currencies, err := transaction.QueryContext(ctx, `
			SELECT l.cost_commodity_id FROM investment_operation_lot_effects x
			JOIN investment_lot_events e ON e.id = x.lot_event_id
			JOIN investment_lots l ON l.id = e.lot_id WHERE x.operation_id = ?
			UNION SELECT cost_commodity_id FROM investment_split_revisions WHERE operation_id = ?`,
			deltas[index].OperationID, deltas[index].OperationID)
		if err != nil {
			return nil, fmt.Errorf("read self-check split currencies: %w", err)
		}
		var costIDs []int64
		for currencies.Next() {
			var id int64
			if err := currencies.Scan(&id); err != nil {
				currencies.Close()
				return nil, fmt.Errorf("scan self-check split currency: %w", err)
			}
			costIDs = append(costIDs, id)
		}
		if err := errors.Join(currencies.Err(), currencies.Close()); err != nil {
			return nil, fmt.Errorf("read self-check split currencies: %w", err)
		}
		for _, costID := range costIDs {
			effects, _, _, err := effectiveSplitEffectsQuery(ctx, transaction, bookID, deltas[index].OperationID, costID)
			if err != nil {
				return nil, err
			}
			deltas[index].Effects.AddScaled(sumSplitEffects(effects))
		}
	}
	return deltas, nil
}

// SelfCheckPooledTransferSet is one source depletion set of a pooled_lot
// internal transfer (T-135): the committed transfer_out events (RevisionID
// zero) or one revision's depletions, folded exactly, beside the quantity and
// basis the link carries for that set.
type SelfCheckPooledTransferSet struct {
	OperationID      int64
	RevisionID       int64
	LinkQuantity     *exact.ScaledInt
	LinkBasis        *exact.ScaledInt
	DepletedQuantity *exact.ScaledInt
	DepletedBasis    *exact.ScaledInt
	Depletions       int
	// LinkUnknown and UnknownDepletions carry knowledge: an unknown link must
	// have at least one unknown depletion, and a known link none (T-145).
	LinkUnknown       bool
	UnknownDepletions int
}

// SelfCheckPooledTransferSets audits every committed and revised depletion
// set of every pooled_lot transfer, not only the effective one.
func (r *SelfCheckRepository) SelfCheckPooledTransferSets(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckPooledTransferSet, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT link.operation_id, 0, link.quantity_value, link.quantity_scale,
			link.carried_basis_value, link.carried_basis_scale, link.basis_knowledge,
			e.quantity_value, e.quantity_scale, e.cost_basis_value, e.cost_basis_scale, e.basis_knowledge
		FROM investment_transfer_facts f
		JOIN investment_transfer_lot_links link ON link.operation_id = f.operation_id
		LEFT JOIN investment_operation_lot_effects effect ON effect.operation_id = f.operation_id
		LEFT JOIN investment_lot_events e ON e.id = effect.lot_event_id AND e.event_kind = 'transfer_out'
		WHERE f.book_id = ? AND f.destination_lineage = 'pooled_lot'
			AND (effect.operation_id IS NULL OR e.id IS NOT NULL)
		UNION ALL
		SELECT revision.operation_id, revision.id, link.quantity_value, link.quantity_scale,
			revision.carried_basis_value, revision.carried_basis_scale, revision.basis_knowledge,
			'-' || depletion.quantity_value, depletion.quantity_scale,
			CASE depletion.cost_basis_value WHEN '0' THEN '0' ELSE '-' || depletion.cost_basis_value END,
			depletion.cost_basis_scale, depletion.basis_knowledge
		FROM investment_transfer_link_revisions revision
		JOIN investment_transfer_lot_links link ON link.operation_id = revision.operation_id
			AND link.link_seq = revision.link_seq
		LEFT JOIN investment_transfer_link_revision_depletions depletion ON depletion.revision_id = revision.id
		WHERE revision.book_id = ? AND revision.source_lot_id IS NULL
		ORDER BY 1, 2`, bookID, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check pooled transfer sets: %w", err)
	}
	defer rows.Close()
	var sets []SelfCheckPooledTransferSet
	for rows.Next() {
		var operationID, revisionID int64
		var linkQuantity exact.Coefficient
		var linkQuantityScale int
		var linkBasis sql.NullString
		var linkBasisScale sql.NullInt64
		var quantity, basis sql.NullString
		var quantityScale, basisScale sql.NullInt64
		var linkKnowledge string
		var depletionKnowledge sql.NullString
		if err := rows.Scan(&operationID, &revisionID, &linkQuantity, &linkQuantityScale, &linkBasis, &linkBasisScale,
			&linkKnowledge, &quantity, &quantityScale, &basis, &basisScale, &depletionKnowledge); err != nil {
			return nil, fmt.Errorf("scan self-check pooled transfer set: %w", err)
		}
		if len(sets) == 0 || sets[len(sets)-1].OperationID != operationID || sets[len(sets)-1].RevisionID != revisionID {
			set := SelfCheckPooledTransferSet{OperationID: operationID, RevisionID: revisionID,
				LinkQuantity: exact.ScaledIntFromCoefficient(linkQuantity, linkQuantityScale),
				LinkBasis:    exact.NewScaledInt(), DepletedQuantity: exact.NewScaledInt(), DepletedBasis: exact.NewScaledInt(),
				LinkUnknown: linkKnowledge == InvestmentBasisUnknown}
			if linkBasis.Valid {
				set.LinkBasis.AddCoefficient(exact.Coefficient(linkBasis.String), int(linkBasisScale.Int64))
			}
			sets = append(sets, set)
		}
		if !quantity.Valid {
			continue
		}
		// Depletions leave the source, so their signed amounts are negative.
		set := &sets[len(sets)-1]
		set.Depletions++
		if depletionKnowledge.String == InvestmentBasisUnknown {
			set.UnknownDepletions++
		}
		set.DepletedQuantity.SubScaled(exact.ScaledIntFromCoefficient(exact.Coefficient(quantity.String), int(quantityScale.Int64)))
		if basis.Valid {
			set.DepletedBasis.SubScaled(exact.ScaledIntFromCoefficient(exact.Coefficient(basis.String), int(basisScale.Int64)))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate self-check pooled transfer sets: %w", err)
	}
	return sets, nil
}

// SelfCheckTransferBridge is one outbound transfer's carried basis in one
// cost currency: what its effective links carry and what its bridge journal
// plus every later bridge adjustment (T-143) post to the transfer equity
// account. They must agree exactly.
type SelfCheckTransferBridge struct {
	OperationID     int64
	CostCommodityID int64
	Carried         *exact.ScaledInt
	Bridged         *exact.ScaledInt
	// Unknown means a link carries unknown basis: no bridge may post until
	// the basis is resolved, so Bridged must be zero.
	Unknown bool
}

// SelfCheckTransferBridges folds every outbound transfer's link basis and
// bridge postings per cost currency. Coefficients are summed in Go.
func (r *SelfCheckRepository) SelfCheckTransferBridges(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckTransferBridge, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT f.operation_id, x.cost_commodity_id, 0, x.carried_basis_value, x.carried_basis_scale,
			x.basis_knowledge = 'unknown'
		FROM investment_transfer_facts f
		JOIN effective_investment_transfer_links x ON x.operation_id = f.operation_id
		WHERE f.book_id = ? AND f.transfer_kind = 'external_out'
		UNION ALL
		SELECT link.operation_id, pv.commodity_id, 1, pv.quantity_value, pv.quantity_scale, 0
		FROM investment_operation_journal_links link
		JOIN posting_versions pv ON pv.transaction_version_id = link.transaction_version_id
		JOIN accounts equity ON equity.id = pv.account_id
			AND equity.system_role = 'external_investment_transfer_equity'
		WHERE link.book_id = ? AND link.role = 'transfer_bridge'
		ORDER BY 1, 2`, bookID, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check transfer bridges: %w", err)
	}
	var bridges []SelfCheckTransferBridge
	for rows.Next() {
		var operationID, costID int64
		var bridged, unknown bool
		var value sql.NullString
		var scale sql.NullInt64
		if err := rows.Scan(&operationID, &costID, &bridged, &value, &scale, &unknown); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan self-check transfer bridge: %w", err)
		}
		if len(bridges) == 0 || bridges[len(bridges)-1].OperationID != operationID ||
			bridges[len(bridges)-1].CostCommodityID != costID {
			bridges = append(bridges, SelfCheckTransferBridge{OperationID: operationID, CostCommodityID: costID,
				Carried: exact.NewScaledInt(), Bridged: exact.NewScaledInt()})
		}
		if unknown {
			bridges[len(bridges)-1].Unknown = true
		}
		if !value.Valid {
			continue
		}
		amount, err := exact.Parse(value.String)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("parse self-check transfer bridge amount: %w", err)
		}
		if bridged {
			bridges[len(bridges)-1].Bridged.AddCoefficient(amount, int(scale.Int64))
		} else {
			bridges[len(bridges)-1].Carried.AddCoefficient(amount, int(scale.Int64))
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("read self-check transfer bridges: %w", err)
	}
	return bridges, nil
}

// SelfCheckCapitalReturn is one return of capital's receipt and the per-lot
// effects that must conserve it: allocations sum to the receipt, reduction
// plus excess equals each allocation, and each lot event takes exactly the
// reduction (T-146).
type SelfCheckCapitalReturn struct {
	OperationID int64
	Amount      *exact.ScaledInt
	Allocated   *exact.ScaledInt
	Mismatched  bool
}

func (r *SelfCheckRepository) SelfCheckCapitalReturns(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckCapitalReturn, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT f.operation_id, 0 AS revision_id, f.amount_value, f.amount_scale, COALESCE(e.allocated_value, '0'), COALESCE(e.allocated_scale, 0),
			COALESCE(e.reduction_value, '0'), COALESCE(e.reduction_scale, 0), COALESCE(e.excess_value, '0'), COALESCE(e.excess_scale, 0),
			COALESCE(ev.cost_basis_value, '0'), COALESCE(ev.cost_basis_scale, 0), e.lot_id IS NULL OR ev.id IS NULL
		FROM investment_capital_return_facts f
		LEFT JOIN investment_capital_return_effects e ON e.operation_id = f.operation_id
		LEFT JOIN investment_lot_events ev ON ev.id = e.lot_event_id
		WHERE f.book_id = ?
		UNION ALL
		-- A revised return has no lot events for its effects: the event check
		-- compares the reduction with itself.
		SELECT f.operation_id, revision.id AS revision_id, f.amount_value, f.amount_scale, COALESCE(e.allocated_value, '0'), COALESCE(e.allocated_scale, 0),
			COALESCE(e.reduction_value, '0'), COALESCE(e.reduction_scale, 0), COALESCE(e.excess_value, '0'), COALESCE(e.excess_scale, 0),
			CASE WHEN e.reduction_value = '0' THEN '0' ELSE '-' || COALESCE(e.reduction_value, '0') END, COALESCE(e.reduction_scale, 0), e.lot_id IS NULL
		FROM investment_capital_return_facts f
		JOIN investment_capital_return_revisions revision ON revision.operation_id = f.operation_id
		LEFT JOIN investment_capital_return_revision_effects e ON e.revision_id = revision.id
		WHERE f.book_id = ?
		ORDER BY 1, 2`, bookID, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check returns of capital: %w", err)
	}
	defer rows.Close()
	var returns []SelfCheckCapitalReturn
	var priorRevisionID int64
	for rows.Next() {
		var operationID, revisionID int64
		var missing bool
		var amount, allocated, reduction, excess, event exact.Coefficient
		var amountScale, allocatedScale, reductionScale, excessScale, eventScale int
		if err := rows.Scan(&operationID, &revisionID, &amount, &amountScale, &allocated, &allocatedScale,
			&reduction, &reductionScale, &excess, &excessScale, &event, &eventScale, &missing); err != nil {
			return nil, fmt.Errorf("scan self-check return of capital: %w", err)
		}
		if len(returns) == 0 || returns[len(returns)-1].OperationID != operationID || priorRevisionID != revisionID {
			priorRevisionID = revisionID
			returns = append(returns, SelfCheckCapitalReturn{OperationID: operationID,
				Amount: exact.ScaledIntFromCoefficient(amount, amountScale), Allocated: exact.NewScaledInt()})
		}
		current := &returns[len(returns)-1]
		current.Allocated.AddCoefficient(allocated, allocatedScale)
		parts := exact.ScaledIntFromCoefficient(reduction, reductionScale)
		parts.AddCoefficient(excess, excessScale)
		if missing || parts.Cmp(exact.ScaledIntFromCoefficient(allocated, allocatedScale)) != 0 ||
			exact.ScaledIntFromCoefficient(event, eventScale).Cmp(exact.ScaledIntFromCoefficient(reduction, reductionScale).Negated()) != 0 {
			current.Mismatched = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate self-check returns of capital: %w", err)
	}
	return returns, nil
}

// selfCheckRevisionBasis reads a revision's basis tuple: unknown has no
// amount, known must fit the event projection's int64.
func selfCheckRevisionBasis(value sql.NullString, scale sql.NullInt64, knowledge string) (int64, int, error) {
	if knowledge == InvestmentBasisUnknown && !value.Valid && !scale.Valid {
		return 0, 0, nil
	}
	if knowledge != InvestmentBasisKnown || !value.Valid || !scale.Valid {
		return 0, 0, fmt.Errorf("invalid revision basis knowledge/amount pair")
	}
	parsed, err := exact.Parse(value.String)
	if err != nil {
		return 0, 0, err
	}
	amount, err := exact.ScaledIntFromCoefficient(parsed, int(scale.Int64)).Int64()
	return amount, int(scale.Int64), err
}
