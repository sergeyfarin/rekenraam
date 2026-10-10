package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// Share exchange (#177, operation plan "Compound corporate actions"). Every
// long lot of one instrument open in a holding account at the slot becomes a
// lot of another instrument in the destination holding account (the same one,
// or the new instrument's own when the source's default instrument refuses
// it): its remaining quantity times
// numerator/denominator exactly, its remaining basis, cost currency and
// knowledge unchanged, its original acquisition date kept on the link. The
// fact is an investment_transfer_facts row of kind 'exchange', so replay,
// the dependency closure and link revisions treat each link as a fixed
// source-lot depletion whose carried basis opens the destination lot.
//
// The journal posts only the two securities: H_old -Q, T_old +Q and
// H_new +Q', T_new -Q'. Basis never leaves commodity_trading in the cost
// currency, so nothing is realized and a later sale's clearing residual is
// its operational gain against the carried basis.

var (
	// ErrShareExchangeNoHoldings means no long lot of the old instrument is
	// open in the account at the exchange slot.
	ErrShareExchangeNoHoldings = errors.New("no holdings of the exchanged instrument are open on the effective date")
	// ErrShareExchangeShortPosition refuses an exchange while the old
	// instrument has an open short lot in the account: a short obligation's
	// exchange is outside this kind.
	ErrShareExchangeShortPosition = fmt.Errorf("%w: the exchanged instrument has an open short position in this account", ErrPositionSideConflict)
	// ErrShareExchangePositionChanged means a holding moved between planning
	// the exchange journal and committing it.
	ErrShareExchangePositionChanged = errors.New("the position changed after the exchange was planned")
	// ErrShareExchangeIncomplete means replay left units of the old instrument
	// open at the exchange's slot, so the exchange no longer covers the whole
	// holding (for example after a backdated purchase).
	ErrShareExchangeIncomplete = errors.New("the share exchange no longer covers the whole holding")
)

// CreateShareExchangeParams are the sourced exchange terms. The ratio is new
// units per old unit, positive and in lowest terms.
type CreateShareExchangeParams struct {
	BookID                 int64
	AccountID              int64
	DestinationAccountID   int64
	CommodityID            int64
	DestinationCommodityID int64
	EffectiveOn            string
	RatioNumerator         int64
	RatioDenominator       int64
	SourceEvidenceJSON     string
	// Expected totals are what the journal posts. The writer recomputes them
	// inside its transaction and refuses a mismatch.
	ExpectedSourceQuantityValue      exact.Coefficient
	ExpectedSourceQuantityScale      int
	ExpectedDestinationQuantityValue exact.Coefficient
	ExpectedDestinationQuantityScale int
}

// ShareExchangeLink is one source lot's move. DestinationLotID is zero in a
// plan or preview.
type ShareExchangeLink struct {
	SourceLotID              int64
	DestinationLotID         int64
	CostCommodityID          int64
	SourceQuantityValue      exact.Coefficient
	SourceQuantityScale      int
	DestinationQuantityValue exact.Coefficient
	DestinationQuantityScale int
	CarriedBasisValue        int64
	CarriedBasisScale        int
	BasisKnowledge           string // unknown leaves CarriedBasis unused
	OriginalDateKnowledge    string
	OriginalAcquiredOn       string
}

// ShareExchangePlan is every link and the two journal totals.
type ShareExchangePlan struct {
	Links                    []ShareExchangeLink
	SourceQuantityValue      exact.Coefficient
	SourceQuantityScale      int
	DestinationQuantityValue exact.Coefficient
	DestinationQuantityScale int
}

func validShareExchangeParams(params CreateShareExchangeParams) bool {
	return params.BookID > 0 && params.AccountID > 0 && params.DestinationAccountID > 0 && params.CommodityID > 0 &&
		params.DestinationCommodityID > 0 && params.DestinationCommodityID != params.CommodityID &&
		isDisposalCalendarDate(params.EffectiveOn) && params.RatioNumerator > 0 && params.RatioDenominator > 0
}

// planShareExchangeTx reads the holding at the slot and derives every link
// without writing. Entry is in date order until #179: an exchange behind a
// later depletion of either instrument in the account is refused.
func planShareExchangeTx(ctx context.Context, tx *sql.Tx, params CreateShareExchangeParams) (ShareExchangePlan, error) {
	if !validShareExchangeParams(params) {
		return ShareExchangePlan{}, fmt.Errorf("%w: share exchange terms are incomplete", ErrInvalidDisposalParams)
	}
	for _, position := range [][2]int64{{params.AccountID, params.CommodityID}, {params.DestinationAccountID, params.DestinationCommodityID}} {
		backdated, err := positionRewrittenAfterTx(ctx, tx, params.BookID, position[0], position[1], params.EffectiveOn)
		if err != nil {
			return ShareExchangePlan{}, err
		}
		if backdated {
			return ShareExchangePlan{}, fmt.Errorf("%w: a share exchange cannot yet be dated behind a later depletion", ErrOutOfOrderPositionEvent)
		}
	}
	var shortLots int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side = 'short' AND status = 'open'`,
		params.BookID, params.AccountID, params.CommodityID).Scan(&shortLots); err != nil {
		return ShareExchangePlan{}, fmt.Errorf("read share exchange short lots: %w", err)
	}
	if shortLots > 0 {
		return ShareExchangePlan{}, ErrShareExchangeShortPosition
	}
	ceiling, err := splitQuantityCeilingTx(ctx, tx, params.BookID, params.DestinationCommodityID, params.EffectiveOn)
	if err != nil {
		return ShareExchangePlan{}, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, opened_on, cost_commodity_id, remaining_quantity_value, remaining_quantity_scale,
			remaining_cost_basis_value, remaining_cost_basis_scale, basis_knowledge
		FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side = 'long'
			AND status = 'open' AND opened_on <= ?
		ORDER BY cost_commodity_id, opened_on, id`, params.BookID, params.AccountID, params.CommodityID, params.EffectiveOn)
	if err != nil {
		return ShareExchangePlan{}, fmt.Errorf("read share exchange lots: %w", err)
	}
	type sourceLot struct {
		link     ShareExchangeLink
		openedOn string
	}
	var sources []sourceLot
	for rows.Next() {
		var source sourceLot
		var basis sql.NullInt64
		var basisScale sql.NullInt64
		link := &source.link
		if err := rows.Scan(&link.SourceLotID, &source.openedOn, &link.CostCommodityID,
			&link.SourceQuantityValue, &link.SourceQuantityScale, &basis, &basisScale, &link.BasisKnowledge); err != nil {
			rows.Close()
			return ShareExchangePlan{}, fmt.Errorf("scan share exchange lot: %w", err)
		}
		switch {
		case link.BasisKnowledge == InvestmentBasisKnown && basis.Valid && basisScale.Valid:
			link.CarriedBasisValue, link.CarriedBasisScale = basis.Int64, int(basisScale.Int64)
		case link.BasisKnowledge == InvestmentBasisUnknown && !basis.Valid:
		default:
			rows.Close()
			return ShareExchangePlan{}, fmt.Errorf("%w: lot %d has an invalid basis knowledge/amount pair", ErrInvalidDisposalParams, link.SourceLotID)
		}
		if link.SourceQuantityValue.Sign() > 0 {
			sources = append(sources, source)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return ShareExchangePlan{}, fmt.Errorf("read share exchange lots: %w", err)
	}
	if len(sources) == 0 {
		return ShareExchangePlan{}, ErrShareExchangeNoHoldings
	}
	plan := ShareExchangePlan{}
	sourceTotal, destinationTotal := exact.NewScaledInt(), exact.NewScaledInt()
	for _, source := range sources {
		link := source.link
		link.DestinationQuantityValue, link.DestinationQuantityScale, err = splitLotQuantity(link.SourceQuantityValue,
			link.SourceQuantityScale, params.RatioNumerator, params.RatioDenominator, ceiling)
		if err != nil {
			return ShareExchangePlan{}, fmt.Errorf("lot %d: %w", link.SourceLotID, err)
		}
		if link.OriginalDateKnowledge, link.OriginalAcquiredOn, err = internalTransferOriginalDateTx(ctx, tx,
			link.SourceLotID, source.openedOn); err != nil {
			return ShareExchangePlan{}, err
		}
		sourceTotal.AddCoefficient(link.SourceQuantityValue, link.SourceQuantityScale)
		destinationTotal.AddCoefficient(link.DestinationQuantityValue, link.DestinationQuantityScale)
		plan.Links = append(plan.Links, link)
	}
	if plan.SourceQuantityValue, err = sourceTotal.Coefficient(); err != nil {
		return ShareExchangePlan{}, err
	}
	if plan.DestinationQuantityValue, err = destinationTotal.Coefficient(); err != nil {
		return ShareExchangePlan{}, err
	}
	plan.SourceQuantityScale, plan.DestinationQuantityScale = sourceTotal.Scale(), destinationTotal.Scale()
	return plan, nil
}

// PlanShareExchange computes the exchange's links and journal totals in a
// transaction that is always rolled back.
func (r *InvestmentRepository) PlanShareExchange(ctx context.Context, params CreateShareExchangeParams) (ShareExchangePlan, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ShareExchangePlan{}, fmt.Errorf("begin share exchange plan: %w", err)
	}
	defer rollbackTx(ctx, tx)
	return planShareExchangeTx(ctx, tx, params)
}

// CreateShareExchange commits the journal, the exchange fact, every source
// depletion, destination lot and link, checkpoint invalidation and audit
// together.
func (r *InvestmentRepository) CreateShareExchange(ctx context.Context, journal CreateTransactionParams, params CreateShareExchangeParams) (TransactionRecord, ShareExchangePlan, error) {
	return r.createShareExchange(ctx, journal, params, false)
}

// SimulateShareExchange runs the complete writer, then rolls back. The links
// are what the same commit would write; destination lot IDs are cleared
// because they were never durable.
func (r *InvestmentRepository) SimulateShareExchange(ctx context.Context, journal CreateTransactionParams, params CreateShareExchangeParams) (SimulatedInvestmentWrite, ShareExchangePlan, error) {
	transaction, plan, err := r.createShareExchange(ctx, journal, params, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, ShareExchangePlan{}, err
	}
	for index := range plan.Links {
		plan.Links[index].DestinationLotID = 0
	}
	return simulatedInvestmentWrite(transaction), plan, nil
}

func (r *InvestmentRepository) createShareExchange(ctx context.Context, journal CreateTransactionParams, params CreateShareExchangeParams, preview bool) (TransactionRecord, ShareExchangePlan, error) {
	write := executeInvestmentWriteTx[ShareExchangePlan]
	if preview {
		write = previewInvestmentWriteTx[ShareExchangePlan]
	}
	return write(ctx, r.database, journal, func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (ShareExchangePlan, error) {
		return writeShareExchangeTx(ctx, tx, params, transaction, journal, auditEventID)
	}, nil)
}

// writeShareExchangeTx records an exchange whose journal the enclosing writer
// just posted: it recomputes the plan, refuses stale journal totals, then
// moves every lot.
func writeShareExchangeTx(ctx context.Context, tx *sql.Tx, params CreateShareExchangeParams, transaction TransactionRecord,
	journal CreateTransactionParams, auditEventID int64) (ShareExchangePlan, error) {
	plan, err := planShareExchangeTx(ctx, tx, params)
	if err != nil {
		return ShareExchangePlan{}, err
	}
	same := func(value exact.Coefficient, scale int, expectedValue exact.Coefficient, expectedScale int) bool {
		return exact.ScaledIntFromCoefficient(value, scale).Cmp(exact.ScaledIntFromCoefficient(expectedValue, expectedScale)) == 0
	}
	if !same(plan.SourceQuantityValue, plan.SourceQuantityScale, params.ExpectedSourceQuantityValue, params.ExpectedSourceQuantityScale) ||
		!same(plan.DestinationQuantityValue, plan.DestinationQuantityScale,
			params.ExpectedDestinationQuantityValue, params.ExpectedDestinationQuantityScale) {
		return ShareExchangePlan{}, ErrShareExchangePositionChanged
	}
	operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
	if err != nil {
		return ShareExchangePlan{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_facts
		(operation_id, book_id, transfer_kind, effective_on, commodity_id, source_account_id,
		 destination_account_id, source_evidence_json, created_audit_event_id,
		 destination_commodity_id, ratio_numerator, ratio_denominator)
		VALUES (?, ?, 'exchange', ?, ?, ?, ?, ?, ?, ?, ?, ?)`, operationID, params.BookID, params.EffectiveOn,
		params.CommodityID, params.AccountID, params.DestinationAccountID, params.SourceEvidenceJSON, auditEventID,
		params.DestinationCommodityID, params.RatioNumerator, params.RatioDenominator); err != nil {
		return ShareExchangePlan{}, fmt.Errorf("record share exchange fact: %w", err)
	}
	// Every source depletion precedes every destination opening in effect
	// order, which is where replay applies the grouped exchange intent.
	depletions := make([]LotDisposalRecord, len(plan.Links))
	allocationScales := make(map[int64]int)
	for index, link := range plan.Links {
		depletionParams := shareExchangeDepletionParams(params, link.CostCommodityID, transaction.ID, journal)
		scale, cached := allocationScales[link.CostCommodityID]
		if !cached {
			if scale, err = positionBasisAllocationScaleTx(ctx, tx, depletionParams); err != nil {
				return ShareExchangePlan{}, err
			}
			allocationScales[link.CostCommodityID] = scale
		}
		depletion, err := disposeLotTx(ctx, tx, depletionParams, link.SourceLotID,
			link.SourceQuantityValue, link.SourceQuantityScale, auditEventID, scale)
		if err != nil {
			return ShareExchangePlan{}, err
		}
		if err := linkLotEffectTx(ctx, tx, operationID, depletion.EventID); err != nil {
			return ShareExchangePlan{}, err
		}
		depletions[index] = depletion
	}
	for index := range plan.Links {
		link, depletion := &plan.Links[index], depletions[index]
		// The full depletion carries the lot's remaining basis at the
		// position's allocation scale; that is the basis the link records.
		link.BasisKnowledge = normalizedBasisKnowledge(depletion.BasisKnowledge)
		link.CarriedBasisValue, link.CarriedBasisScale = depletion.CostBasisValue, depletion.CostBasisScale
		openedValue, openedScale := link.CarriedBasisValue, link.CarriedBasisScale
		if link.BasisKnowledge == InvestmentBasisUnknown {
			link.CarriedBasisValue, link.CarriedBasisScale, openedValue, openedScale = 0, 0, 0, 0
		}
		destination, err := createLotWithAuditTx(ctx, tx, CreateInvestmentLotParams{
			BookID: params.BookID, AccountID: params.DestinationAccountID, CommodityID: params.DestinationCommodityID,
			OpenedOn: params.EffectiveOn, SourceTransactionID: transaction.ID,
			QuantityValue: link.DestinationQuantityValue, QuantityScale: link.DestinationQuantityScale,
			CostBasisValue: openedValue, CostBasisScale: openedScale, CostCommodityID: link.CostCommodityID,
			OpeningBasisKnowledge: link.BasisKnowledge, MetadataJSON: `{"source":"share_exchange"}`,
			EventKind: "transfer_in", CreatedAt: journal.CreatedAt, CreatedByUserID: journal.ActorUserID,
		}, auditEventID, false)
		if err != nil {
			return ShareExchangePlan{}, err
		}
		link.DestinationLotID = destination.ID
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_lot_links
			(operation_id, link_seq, source_lot_id, destination_lot_id, quantity_value, quantity_scale,
			 basis_knowledge, carried_basis_value, carried_basis_scale, cost_commodity_id,
			 original_date_knowledge, original_acquired_on, source_evidence_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
			operationID, index+1, link.SourceLotID, destination.ID, link.SourceQuantityValue, link.SourceQuantityScale,
			link.BasisKnowledge, nullableBasisValue(link.CarriedBasisValue, link.BasisKnowledge),
			nullableBasisScale(link.CarriedBasisScale, link.BasisKnowledge), link.CostCommodityID,
			link.OriginalDateKnowledge, link.OriginalAcquiredOn, params.SourceEvidenceJSON); err != nil {
			return ShareExchangePlan{}, fmt.Errorf("link share exchange lots: %w", err)
		}
	}
	for costCommodityID := range allocationScales {
		depletionParams := shareExchangeDepletionParams(params, costCommodityID, transaction.ID, journal)
		if err := requirePositionBasisRangeQueryTx(ctx, tx, params.BookID, params.AccountID,
			params.CommodityID, costCommodityID, true, PositionSideLong); err != nil {
			return ShareExchangePlan{}, err
		}
		if err := keepPositionMethodFamilyTx(ctx, tx, depletionParams, auditEventID); err != nil {
			return ShareExchangePlan{}, err
		}
	}
	return plan, nil
}

func shareExchangeDepletionParams(params CreateShareExchangeParams, costCommodityID, transactionID int64, journal CreateTransactionParams) DisposeLotsParams {
	return DisposeLotsParams{BookID: params.BookID, AccountID: params.AccountID,
		CommodityID: params.CommodityID, CostCommodityID: costCommodityID,
		TransactionID: transactionID, EventDate: params.EffectiveOn, EventKind: "transfer_out",
		MetadataJSON: params.SourceEvidenceJSON, CreatedAt: journal.CreatedAt, ActorUserID: journal.ActorUserID,
		// Unknown basis moves as unknown: the destination inherits it.
		AdmitUnknownBasis: true}
}

// keepPositionMethodFamilyTx leaves a position's method-family lock as it
// was, releasing it when nothing remains open. An exchange takes every lot
// whole, so unlike a selected-lot transfer it makes no method choice.
func keepPositionMethodFamilyTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, auditEventID int64) error {
	var family string
	err := tx.QueryRowContext(ctx, `SELECT method_family FROM investment_position_basis_state
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND position_side = ?`,
		params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID,
		positionSideOrLong(params.PositionSide)).Scan(&family)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read exchanged position basis method: %w", err)
	}
	method := "specific_lot"
	if family == "average_cost" {
		method = "average_cost"
	}
	return updatePositionMethodFamilyTx(ctx, tx, params, method, auditEventID)
}

// requireShareExchangeCompleteTx is replay's whole-holding rule: after an
// exchange's depletions no lot of the position opened on or before the
// exchange date may remain open.
func requireShareExchangeCompleteTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64, date string) error {
	var open int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ?
			AND position_side = 'long' AND status = 'open' AND opened_on <= ?`,
		bookID, accountID, commodityID, costCommodityID, date).Scan(&open); err != nil {
		return fmt.Errorf("check share exchange coverage: %w", err)
	}
	if open > 0 {
		return ErrShareExchangeIncomplete
	}
	return nil
}
