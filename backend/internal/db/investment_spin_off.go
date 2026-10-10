package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"

	"rekenraam/backend/internal/exact"
)

// Spin-off (#180, operation plan "Compound corporate actions"). Every long
// lot of the parent instrument open in a holding account at the slot keeps
// its units and gives up an exact fraction of its remaining basis to one new
// lot of the distributed instrument in the destination holding: the parent
// lot's remaining quantity times numerator/denominator, the allocated basis,
// its cost currency and knowledge, and its original acquisition date on the
// link. The allocation is the remaining basis times the fraction, truncated
// at the position's basis allocation scale; the parent keeps the remainder,
// so the reduction always equals the new lot's opening basis.
//
// The fact is an investment_transfer_facts row of kind 'spin_off'. Its links
// are fixed per parent lot: replay reduces each lot again at the slot, a
// changed allocation appends a link revision and the new position replays
// from it, and the dependency closure follows the spin-off edge.
//
// The journal posts only the new instrument: H_new +Q', T_new -Q'. The parent
// and the cost currency post nothing, so basis never leaves commodity_trading
// and nothing is realized.

var (
	// ErrSpinOffNoHoldings means no long lot of the parent is open in the
	// account at the spin-off slot.
	ErrSpinOffNoHoldings = errors.New("no holdings of the parent instrument are open on the effective date")
	// ErrSpinOffShortPosition refuses a spin-off while the parent has an open
	// short lot in the account.
	ErrSpinOffShortPosition = fmt.Errorf("%w: the parent instrument has an open short position in this account", ErrPositionSideConflict)
	// ErrSpinOffPositionChanged means a holding moved between planning the
	// spin-off journal and committing it.
	ErrSpinOffPositionChanged = errors.New("the position changed after the spin-off was planned")
	// ErrSpinOffIncomplete means replay left a parent lot open at the slot
	// that the spin-off did not entitle (for example after a backdated
	// purchase).
	ErrSpinOffIncomplete = errors.New("the spin-off no longer covers every parent lot open on its date")
	// ErrSpinOffEntitlementChanged means replay changed the units an entitled
	// parent lot holds at the slot (for example after a backdated sale), so the
	// fixed new lot no longer matches it.
	ErrSpinOffEntitlementChanged = errors.New("a parent lot entitled by the spin-off no longer holds the units it was entitled with")
)

// CreateSpinOffParams are the sourced spin-off terms. The ratio is new units
// per parent unit, positive and in lowest terms; the basis fraction is an
// exact decimal strictly between 0 and 1.
type CreateSpinOffParams struct {
	BookID                 int64
	AccountID              int64
	DestinationAccountID   int64
	CommodityID            int64
	DestinationCommodityID int64
	EffectiveOn            string
	RatioNumerator         int64
	RatioDenominator       int64
	BasisFractionValue     exact.Coefficient
	BasisFractionScale     int
	SourceEvidenceJSON     string
	// ReplacesOperationID is the spin-off a replacement corrects (#183): the
	// plan replays without it, at its correction-root slot.
	ReplacesOperationID int64
	// The expected total is what the journal posts. The writer recomputes it
	// inside its transaction and refuses a mismatch.
	ExpectedDestinationQuantityValue exact.Coefficient
	ExpectedDestinationQuantityScale int
}

// SpinOffLink is one parent lot's distribution. DestinationLotID is zero in
// a plan or preview. An unknown lot leaves both basis amounts unused.
type SpinOffLink struct {
	SourceLotID              int64
	DestinationLotID         int64
	CostCommodityID          int64
	SourceQuantityValue      exact.Coefficient
	SourceQuantityScale      int
	DestinationQuantityValue exact.Coefficient
	DestinationQuantityScale int
	BasisKnowledge           string
	// AllocatedBasis moves to the new lot; RemainingBasis stays on the parent.
	AllocatedBasisValue   int64
	AllocatedBasisScale   int
	RemainingBasisValue   int64
	RemainingBasisScale   int
	OriginalDateKnowledge string
	OriginalAcquiredOn    string
}

// SpinOffPlan is every link and the new-instrument total the journal posts.
type SpinOffPlan struct {
	Links                    []SpinOffLink
	DestinationQuantityValue exact.Coefficient
	DestinationQuantityScale int
}

func validSpinOffParams(params CreateSpinOffParams) bool {
	return params.BookID > 0 && params.AccountID > 0 && params.DestinationAccountID > 0 && params.CommodityID > 0 &&
		params.DestinationCommodityID > 0 && params.DestinationCommodityID != params.CommodityID &&
		isDisposalCalendarDate(params.EffectiveOn) && params.RatioNumerator > 0 && params.RatioDenominator > 0 &&
		validSpinOffFraction(params.BasisFractionValue, params.BasisFractionScale)
}

// validSpinOffFraction reports whether value/10^scale is strictly between 0
// and 1 with a scale the fact can store.
func validSpinOffFraction(value exact.Coefficient, scale int) bool {
	return scale >= 1 && scale <= 12 && value.Sign() > 0 && value.BigInt().Cmp(exact.Pow10(scale)) < 0
}

// spinOffAllocation is a lot's allocated basis: remaining × fraction,
// truncated at the allocation scale (never below the lot's own scale). The
// parent keeps remaining − allocated at the same scale.
func spinOffAllocation(remainingValue int64, remainingScale int, fractionValue exact.Coefficient, fractionScale,
	allocationScale int) (allocated, remaining *exact.ScaledInt, err error) {
	scale := max(allocationScale, remainingScale)
	basis := exact.ScaledIntFromInt64(remainingValue, remainingScale)
	basis.Align(scale)
	product := new(big.Int).Mul(basis.BigInt(), fractionValue.BigInt())
	product.Quo(product, exact.Pow10(fractionScale))
	allocated = exact.ScaledIntFromBig(product, scale)
	remaining = exact.ScaledIntFromBig(basis.BigInt(), scale)
	remaining.SubScaled(allocated)
	if _, err := allocated.Int64(); err != nil {
		return nil, nil, ErrInvestmentBasisRange
	}
	if _, err := remaining.Int64(); err != nil {
		return nil, nil, ErrInvestmentBasisRange
	}
	return allocated, remaining, nil
}

// spinOffLotTx is the spin-off's step for one entitled parent lot, shared by
// the writer and replay: the lot must be an open long lot of the position on
// or before the slot holding exactly quantity units. It installs the reduced
// basis in the projection and returns the link amounts. An unknown lot stays
// unknown and moves no amount.
func spinOffLotTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID, lotID int64,
	effectiveOn string, quantityValue exact.Coefficient, quantityScale int, fractionValue exact.Coefficient,
	fractionScale, allocationScale int, at string, actorUserID, auditEventID int64) (SpinOffLink, error) {
	var link SpinOffLink
	var basisValue, basisScale sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT remaining_quantity_value, remaining_quantity_scale,
			remaining_cost_basis_value, remaining_cost_basis_scale, basis_knowledge
		FROM current_investment_lots
		WHERE id = ? AND book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ?
			AND position_side = 'long' AND status = 'open' AND opened_on <= ?`,
		lotID, bookID, accountID, commodityID, costCommodityID, effectiveOn).Scan(&link.SourceQuantityValue,
		&link.SourceQuantityScale, &basisValue, &basisScale, &link.BasisKnowledge)
	if errors.Is(err, sql.ErrNoRows) {
		return SpinOffLink{}, ErrSpinOffEntitlementChanged
	}
	if err != nil {
		return SpinOffLink{}, fmt.Errorf("read spin-off parent lot: %w", err)
	}
	if exact.ScaledIntFromCoefficient(link.SourceQuantityValue, link.SourceQuantityScale).Cmp(
		exact.ScaledIntFromCoefficient(quantityValue, quantityScale)) != 0 {
		return SpinOffLink{}, ErrSpinOffEntitlementChanged
	}
	link.SourceLotID, link.CostCommodityID = lotID, costCommodityID
	link.SourceQuantityValue, link.SourceQuantityScale = quantityValue, quantityScale
	link.BasisKnowledge = normalizedBasisKnowledge(link.BasisKnowledge)
	if link.BasisKnowledge == InvestmentBasisUnknown {
		return link, nil
	}
	if !basisValue.Valid || !basisScale.Valid {
		return SpinOffLink{}, fmt.Errorf("%w: lot %d has a known basis without an amount", ErrInvalidDisposalParams, lotID)
	}
	allocated, remaining, err := spinOffAllocation(basisValue.Int64, int(basisScale.Int64), fractionValue,
		fractionScale, allocationScale)
	if err != nil {
		return SpinOffLink{}, err
	}
	link.AllocatedBasisValue, _ = allocated.Int64()
	link.RemainingBasisValue, _ = remaining.Int64()
	link.AllocatedBasisScale, link.RemainingBasisScale = allocated.Scale(), remaining.Scale()
	if _, err := tx.ExecContext(ctx, `UPDATE investment_lot_state
		SET remaining_cost_basis_value = ?, remaining_cost_basis_scale = ?,
			updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = NULLIF(?, 0)
		WHERE lot_id = ? AND book_id = ?`, link.RemainingBasisValue, link.RemainingBasisScale,
		at, actorUserID, auditEventID, lotID, bookID); err != nil {
		return SpinOffLink{}, fmt.Errorf("reduce spin-off parent lot %d basis: %w", lotID, err)
	}
	return link, nil
}

// requireSpinOffCompleteTx is replay's entitlement rule: after a spin-off's
// links every long lot of the position open at the slot must be one of them.
func requireSpinOffCompleteTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64,
	date string, entitled map[int64]bool) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ?
			AND position_side = 'long' AND status = 'open' AND opened_on <= ?`,
		bookID, accountID, commodityID, costCommodityID, date)
	if err != nil {
		return fmt.Errorf("check spin-off coverage: %w", err)
	}
	incomplete := false
	for rows.Next() {
		var lotID int64
		if err := rows.Scan(&lotID); err != nil {
			rows.Close()
			return fmt.Errorf("scan spin-off coverage: %w", err)
		}
		incomplete = incomplete || !entitled[lotID]
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("check spin-off coverage: %w", err)
	}
	if incomplete {
		return ErrSpinOffIncomplete
	}
	return nil
}

// spinOffWritePlan is the plan plus how the writer must record it.
type spinOffWritePlan struct {
	SpinOffPlan
	// Replayed means the parent reductions are the ones replay produced at the
	// spin-off's slot (#183); replay installs them in the projection.
	Replayed bool
	// DestinationBackdated means the new lots open behind a later event of the
	// destination holding, so they open by replay admission and it replays.
	DestinationBackdated bool
}

// planSpinOffTx derives every link without writing. A spin-off in date order
// reads today's holding. One dated behind a later rewrite of the parent, or a
// replacement, replays the parent with the spin-off as its subject at the
// slot, so it entitles every lot open then with the units it held (#183).
func planSpinOffTx(ctx context.Context, tx *sql.Tx, params CreateSpinOffParams, subject compoundActionSubject) (spinOffWritePlan, error) {
	if !validSpinOffParams(params) {
		return spinOffWritePlan{}, fmt.Errorf("%w: spin-off terms are incomplete", ErrInvalidDisposalParams)
	}
	replacing := params.ReplacesOperationID > 0
	parentBackdated, err := positionRewrittenAfterTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID, params.EffectiveOn)
	if err != nil {
		return spinOffWritePlan{}, err
	}
	destinationBackdated, err := positionRewrittenAfterTx(ctx, tx, params.BookID, params.DestinationAccountID,
		params.DestinationCommodityID, params.EffectiveOn)
	if err != nil {
		return spinOffWritePlan{}, err
	}
	var shortLots int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side = 'short' AND status = 'open'`,
		params.BookID, params.AccountID, params.CommodityID).Scan(&shortLots); err != nil {
		return spinOffWritePlan{}, fmt.Errorf("read spin-off short lots: %w", err)
	}
	if shortLots > 0 {
		return spinOffWritePlan{}, ErrSpinOffShortPosition
	}
	ceiling, err := splitQuantityCeilingTx(ctx, tx, params.BookID, params.DestinationCommodityID, params.EffectiveOn)
	if err != nil {
		return spinOffWritePlan{}, err
	}
	plan := spinOffWritePlan{Replayed: replacing || parentBackdated, DestinationBackdated: replacing || destinationBackdated}
	type parentLot struct {
		link     SpinOffLink
		openedOn string
	}
	var lots []parentLot
	if plan.Replayed {
		projections, err := replayCompoundActionSubjectTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID,
			params.ReplacesOperationID, subject, InvestmentReplayIntent{OperationKind: "spin_off",
				EventDate: params.EffectiveOn, Kind: "spin_off",
				BasisFractionValue: params.BasisFractionValue, BasisFractionScale: params.BasisFractionScale})
		if err != nil {
			return spinOffWritePlan{}, err
		}
		for _, projection := range projections {
			for _, link := range projection.SubjectSpinOff {
				lot, err := investmentLotByIDTx(ctx, tx, params.BookID, link.SourceLotID)
				if err != nil {
					return spinOffWritePlan{}, err
				}
				lots = append(lots, parentLot{link: link, openedOn: lot.OpenedOn})
			}
		}
	} else {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, cost_commodity_id, opened_on, remaining_quantity_value, remaining_quantity_scale,
				remaining_cost_basis_value, remaining_cost_basis_scale, basis_knowledge
			FROM current_investment_lots
			WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side = 'long'
				AND status = 'open' AND opened_on <= ?
			ORDER BY cost_commodity_id, opened_on, id`, params.BookID, params.AccountID, params.CommodityID, params.EffectiveOn)
		if err != nil {
			return spinOffWritePlan{}, fmt.Errorf("read spin-off parent lots: %w", err)
		}
		type openLot struct {
			parentLot
			basis, scale sql.NullInt64
		}
		var open []openLot
		for rows.Next() {
			var lot openLot
			link := &lot.link
			if err := rows.Scan(&link.SourceLotID, &link.CostCommodityID, &lot.openedOn, &link.SourceQuantityValue,
				&link.SourceQuantityScale, &lot.basis, &lot.scale, &link.BasisKnowledge); err != nil {
				rows.Close()
				return spinOffWritePlan{}, fmt.Errorf("scan spin-off parent lot: %w", err)
			}
			if link.SourceQuantityValue.Sign() > 0 {
				open = append(open, lot)
			}
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return spinOffWritePlan{}, fmt.Errorf("read spin-off parent lots: %w", err)
		}
		allocationScales := make(map[int64]int)
		for _, lot := range open {
			link := &lot.link
			scale, cached := allocationScales[link.CostCommodityID]
			if !cached {
				if scale, err = positionBasisAllocationScaleTx(ctx, tx, spinOffPositionParams(params, link.CostCommodityID)); err != nil {
					return spinOffWritePlan{}, err
				}
				allocationScales[link.CostCommodityID] = scale
			}
			link.BasisKnowledge = normalizedBasisKnowledge(link.BasisKnowledge)
			switch {
			case link.BasisKnowledge == InvestmentBasisKnown && lot.basis.Valid && lot.scale.Valid:
				allocated, remaining, err := spinOffAllocation(lot.basis.Int64, int(lot.scale.Int64),
					params.BasisFractionValue, params.BasisFractionScale, scale)
				if err != nil {
					return spinOffWritePlan{}, err
				}
				link.AllocatedBasisValue, _ = allocated.Int64()
				link.RemainingBasisValue, _ = remaining.Int64()
				link.AllocatedBasisScale, link.RemainingBasisScale = allocated.Scale(), remaining.Scale()
			case link.BasisKnowledge == InvestmentBasisUnknown && !lot.basis.Valid:
			default:
				return spinOffWritePlan{}, fmt.Errorf("%w: lot %d has an invalid basis knowledge/amount pair", ErrInvalidDisposalParams, link.SourceLotID)
			}
			lots = append(lots, lot.parentLot)
		}
	}
	if len(lots) == 0 {
		return spinOffWritePlan{}, ErrSpinOffNoHoldings
	}
	total := exact.NewScaledInt()
	for _, lot := range lots {
		link := lot.link
		if link.DestinationQuantityValue, link.DestinationQuantityScale, err = splitLotQuantity(link.SourceQuantityValue,
			link.SourceQuantityScale, params.RatioNumerator, params.RatioDenominator, ceiling); err != nil {
			return spinOffWritePlan{}, fmt.Errorf("lot %d: %w", link.SourceLotID, err)
		}
		if link.OriginalDateKnowledge, link.OriginalAcquiredOn, err = internalTransferOriginalDateTx(ctx, tx,
			link.SourceLotID, lot.openedOn); err != nil {
			return spinOffWritePlan{}, err
		}
		total.AddCoefficient(link.DestinationQuantityValue, link.DestinationQuantityScale)
		plan.Links = append(plan.Links, link)
	}
	if plan.DestinationQuantityValue, err = total.Coefficient(); err != nil {
		return spinOffWritePlan{}, err
	}
	plan.DestinationQuantityScale = total.Scale()
	return plan, nil
}

// spinOffHoldingTx is the subject spin-off's replay step: it entitles every
// long lot of the position open at the slot with the units it holds there
// and reduces each by the fraction.
func spinOffHoldingTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64,
	intent InvestmentReplayIntent, allocationScale int) ([]SpinOffLink, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, remaining_quantity_value, remaining_quantity_scale
		FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ?
			AND position_side = 'long' AND status = 'open' AND opened_on <= ?
		ORDER BY opened_on, id`, bookID, accountID, commodityID, costCommodityID, intent.EventDate)
	if err != nil {
		return nil, fmt.Errorf("read spin-off holding: %w", err)
	}
	var entitled []LotAllocation
	for rows.Next() {
		var lot LotAllocation
		if err := rows.Scan(&lot.LotID, &lot.QuantityValue, &lot.QuantityScale); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan spin-off holding: %w", err)
		}
		if lot.QuantityValue.Sign() > 0 {
			entitled = append(entitled, lot)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("read spin-off holding: %w", err)
	}
	links := make([]SpinOffLink, 0, len(entitled))
	for _, lot := range entitled {
		link, err := spinOffLotTx(ctx, tx, bookID, accountID, commodityID, costCommodityID, lot.LotID, intent.EventDate,
			lot.QuantityValue, lot.QuantityScale, intent.BasisFractionValue, intent.BasisFractionScale, allocationScale,
			intent.CreatedAt, intent.CreatedByUserID, intent.AuditEventID)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, nil
}

func spinOffPositionParams(params CreateSpinOffParams, costCommodityID int64) DisposeLotsParams {
	// Unknown lots stay in the position; the allocation scale resolves over
	// the known ones.
	return DisposeLotsParams{BookID: params.BookID, AccountID: params.AccountID, CommodityID: params.CommodityID,
		CostCommodityID: costCommodityID, EventDate: params.EffectiveOn, AdmitUnknownBasis: true}
}

// PlanSpinOff computes the spin-off's links and journal total in a
// transaction that is always rolled back.
// A replayed plan's simulated lot events name actorUserID and no audit event.
func (r *InvestmentRepository) PlanSpinOff(ctx context.Context, params CreateSpinOffParams, actorUserID int64) (SpinOffPlan, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return SpinOffPlan{}, fmt.Errorf("begin spin-off plan: %w", err)
	}
	defer rollbackTx(ctx, tx)
	plan, err := planSpinOffTx(ctx, tx, params, compoundActionSubject{ActorUserID: actorUserID, CreatedAt: "1970-01-01T00:00:00Z"})
	if err != nil {
		return SpinOffPlan{}, err
	}
	return plan.SpinOffPlan, nil
}

// CreateSpinOff commits the journal, the spin-off fact, every parent basis
// reduction, new lot and link, checkpoint invalidation and audit together.
func (r *InvestmentRepository) CreateSpinOff(ctx context.Context, journal CreateTransactionParams, params CreateSpinOffParams) (TransactionRecord, SpinOffPlan, error) {
	return r.createSpinOff(ctx, journal, params, false)
}

// SimulateSpinOff runs the complete writer, then rolls back. New lot IDs are
// cleared because they were never durable.
func (r *InvestmentRepository) SimulateSpinOff(ctx context.Context, journal CreateTransactionParams, params CreateSpinOffParams) (SimulatedInvestmentWrite, SpinOffPlan, error) {
	transaction, plan, err := r.createSpinOff(ctx, journal, params, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, SpinOffPlan{}, err
	}
	for index := range plan.Links {
		plan.Links[index].DestinationLotID = 0
	}
	return simulatedInvestmentWrite(transaction), plan, nil
}

func (r *InvestmentRepository) createSpinOff(ctx context.Context, journal CreateTransactionParams, params CreateSpinOffParams, preview bool) (TransactionRecord, SpinOffPlan, error) {
	write := executeInvestmentWriteTx[SpinOffPlan]
	if preview {
		write = previewInvestmentWriteTx[SpinOffPlan]
	}
	return write(ctx, r.database, journal, func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (SpinOffPlan, error) {
		return writeSpinOffTx(ctx, tx, params, transaction, journal, auditEventID)
	}, nil)
}

// writeSpinOffTx records a spin-off whose journal the enclosing writer just
// posted: it recomputes the plan, refuses a stale journal total, then reduces
// every parent lot and opens its new lot. A replayed plan records the
// reductions replay produced at the slot, opens the new lots by replay
// admission and replays both holdings, so every later decision is revised or
// named as a dependency (#183).
func writeSpinOffTx(ctx context.Context, tx *sql.Tx, params CreateSpinOffParams, transaction TransactionRecord,
	journal CreateTransactionParams, auditEventID int64) (SpinOffPlan, error) {
	operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
	if err != nil {
		return SpinOffPlan{}, err
	}
	plan, err := planSpinOffTx(ctx, tx, params, compoundActionSubject{OperationID: operationID,
		TransactionID: transaction.ID, AuditEventID: auditEventID, ActorUserID: journal.ActorUserID, CreatedAt: journal.CreatedAt})
	if err != nil {
		return SpinOffPlan{}, err
	}
	if exact.ScaledIntFromCoefficient(plan.DestinationQuantityValue, plan.DestinationQuantityScale).Cmp(
		exact.ScaledIntFromCoefficient(params.ExpectedDestinationQuantityValue, params.ExpectedDestinationQuantityScale)) != 0 {
		return SpinOffPlan{}, ErrSpinOffPositionChanged
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_facts
		(operation_id, book_id, transfer_kind, effective_on, commodity_id, source_account_id,
		 destination_account_id, source_evidence_json, created_audit_event_id,
		 destination_commodity_id, ratio_numerator, ratio_denominator, basis_fraction_value, basis_fraction_scale)
		VALUES (?, ?, 'spin_off', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, operationID, params.BookID, params.EffectiveOn,
		params.CommodityID, params.AccountID, params.DestinationAccountID, params.SourceEvidenceJSON, auditEventID,
		params.DestinationCommodityID, params.RatioNumerator, params.RatioDenominator,
		params.BasisFractionValue, params.BasisFractionScale); err != nil {
		return SpinOffPlan{}, fmt.Errorf("record spin-off fact: %w", err)
	}
	// Every parent reduction precedes every new lot in effect order, which is
	// where replay applies the grouped spin-off intent.
	allocationScales := make(map[int64]int)
	var costCommodityIDs []int64
	for index := range plan.Links {
		planned := plan.Links[index]
		scale, cached := allocationScales[planned.CostCommodityID]
		if !cached {
			if scale, err = positionBasisAllocationScaleTx(ctx, tx, spinOffPositionParams(params, planned.CostCommodityID)); err != nil {
				return SpinOffPlan{}, err
			}
			allocationScales[planned.CostCommodityID] = scale
			costCommodityIDs = append(costCommodityIDs, planned.CostCommodityID)
		}
		link := planned
		if !plan.Replayed {
			if link, err = spinOffLotTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID, planned.CostCommodityID,
				planned.SourceLotID, params.EffectiveOn, planned.SourceQuantityValue, planned.SourceQuantityScale,
				params.BasisFractionValue, params.BasisFractionScale, scale, journal.CreatedAt, journal.ActorUserID, auditEventID); err != nil {
				return SpinOffPlan{}, err
			}
		}
		reduction := exact.ScaledIntFromInt64(link.AllocatedBasisValue, link.AllocatedBasisScale).Negated()
		reductionValue, err := reduction.Coefficient()
		if err != nil {
			return SpinOffPlan{}, err
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO investment_lot_events (
			book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
			cost_basis_value, cost_basis_scale, metadata_json, created_at, created_by_user_id,
			created_audit_event_id, basis_knowledge
		) VALUES (?, ?, 'basis_reduction', ?, ?, '0', 0, ?, ?, ?, ?, ?, ?, ?)`,
			params.BookID, link.SourceLotID, transaction.ID, params.EffectiveOn,
			knownBasisText(reductionValue, link.BasisKnowledge), nullableBasisScale(link.AllocatedBasisScale, link.BasisKnowledge),
			params.SourceEvidenceJSON, journal.CreatedAt, journal.ActorUserID, auditEventID, link.BasisKnowledge)
		if err != nil {
			return SpinOffPlan{}, fmt.Errorf("record spin-off basis reduction for lot %d: %w", link.SourceLotID, err)
		}
		eventID, err := result.LastInsertId()
		if err != nil {
			return SpinOffPlan{}, fmt.Errorf("read spin-off basis reduction id: %w", err)
		}
		if err := linkLotEffectTx(ctx, tx, operationID, eventID); err != nil {
			return SpinOffPlan{}, err
		}
		link.DestinationQuantityValue, link.DestinationQuantityScale = planned.DestinationQuantityValue, planned.DestinationQuantityScale
		link.OriginalDateKnowledge, link.OriginalAcquiredOn = planned.OriginalDateKnowledge, planned.OriginalAcquiredOn
		plan.Links[index] = link
	}
	for index := range plan.Links {
		link := &plan.Links[index]
		destination, err := createLotWithAuditTx(ctx, tx, CreateInvestmentLotParams{
			BookID: params.BookID, AccountID: params.DestinationAccountID, CommodityID: params.DestinationCommodityID,
			OpenedOn: params.EffectiveOn, SourceTransactionID: transaction.ID,
			QuantityValue: link.DestinationQuantityValue, QuantityScale: link.DestinationQuantityScale,
			CostBasisValue: link.AllocatedBasisValue, CostBasisScale: link.AllocatedBasisScale,
			CostCommodityID: link.CostCommodityID, OpeningBasisKnowledge: link.BasisKnowledge,
			MetadataJSON: `{"source":"spin_off"}`, EventKind: "transfer_in",
			CreatedAt: journal.CreatedAt, CreatedByUserID: journal.ActorUserID,
		}, auditEventID, plan.DestinationBackdated)
		if err != nil {
			return SpinOffPlan{}, err
		}
		link.DestinationLotID = destination.ID
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_lot_links
			(operation_id, link_seq, source_lot_id, destination_lot_id, quantity_value, quantity_scale,
			 basis_knowledge, carried_basis_value, carried_basis_scale, cost_commodity_id,
			 original_date_knowledge, original_acquired_on, source_evidence_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
			operationID, index+1, link.SourceLotID, destination.ID, link.SourceQuantityValue, link.SourceQuantityScale,
			link.BasisKnowledge, nullableBasisValue(link.AllocatedBasisValue, link.BasisKnowledge),
			nullableBasisScale(link.AllocatedBasisScale, link.BasisKnowledge), link.CostCommodityID,
			link.OriginalDateKnowledge, link.OriginalAcquiredOn, params.SourceEvidenceJSON); err != nil {
			return SpinOffPlan{}, fmt.Errorf("link spin-off lots: %w", err)
		}
	}
	// The parent first, so a dependency its reduced basis breaks is named
	// before one the new lots change.
	var replayed [][2]int64
	if plan.Replayed {
		replayed = append(replayed, [2]int64{params.AccountID, params.CommodityID})
	}
	if plan.DestinationBackdated {
		replayed = append(replayed, [2]int64{params.DestinationAccountID, params.DestinationCommodityID})
	}
	if err := replayHoldingsTx(ctx, tx, params.BookID, replayed, operationID, auditEventID,
		journal.ActorUserID, journal.CreatedAt); err != nil {
		return SpinOffPlan{}, err
	}
	for _, costCommodityID := range costCommodityIDs {
		if err := requirePositionBasisRangeQueryTx(ctx, tx, params.BookID, params.AccountID,
			params.CommodityID, costCommodityID, true, PositionSideLong); err != nil {
			return SpinOffPlan{}, err
		}
	}
	return plan.SpinOffPlan, nil
}

// knownBasisText is a lot event's signed basis amount, NULL when unknown.
func knownBasisText(value exact.Coefficient, knowledge string) any {
	if normalizedBasisKnowledge(knowledge) == InvestmentBasisUnknown {
		return nil
	}
	return value
}

// SpinOffTerms are a committed spin-off's fact and its effective links: replay
// may since have revised a link's allocated basis. RemainingBasis is not
// recorded and stays zero here.
type SpinOffTerms struct {
	AccountID              int64
	DestinationAccountID   int64
	CommodityID            int64
	DestinationCommodityID int64
	EffectiveOn            string
	RatioNumerator         int64
	RatioDenominator       int64
	BasisFractionValue     exact.Coefficient
	BasisFractionScale     int
	SourceEvidenceJSON     string
	Plan                   SpinOffPlan
}

// SpinOffTermsByOperation reads a spin-off for the transaction detail. Each
// new quantity is the new lot's opening quantity (validate-and-ship item 31).
func (r *InvestmentRepository) SpinOffTermsByOperation(ctx context.Context, bookID, operationID int64) (SpinOffTerms, error) {
	var terms SpinOffTerms
	err := r.database.QueryRowContext(ctx, `SELECT source_account_id, destination_account_id, commodity_id,
		destination_commodity_id, effective_on, ratio_numerator, ratio_denominator,
		basis_fraction_value, basis_fraction_scale, source_evidence_json
		FROM investment_transfer_facts WHERE book_id = ? AND operation_id = ? AND transfer_kind = 'spin_off'`,
		bookID, operationID).Scan(&terms.AccountID, &terms.DestinationAccountID, &terms.CommodityID,
		&terms.DestinationCommodityID, &terms.EffectiveOn, &terms.RatioNumerator, &terms.RatioDenominator,
		&terms.BasisFractionValue, &terms.BasisFractionScale, &terms.SourceEvidenceJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return SpinOffTerms{}, ErrNotFound
	}
	if err != nil {
		return SpinOffTerms{}, fmt.Errorf("read spin-off terms: %w", err)
	}
	rows, err := r.database.QueryContext(ctx, `SELECT COALESCE(link.source_lot_id, 0), link.destination_lot_id,
		link.cost_commodity_id, link.quantity_value, link.quantity_scale, lot.quantity_value, lot.quantity_scale,
		link.basis_knowledge, link.carried_basis_value, link.carried_basis_scale,
		link.original_date_knowledge, COALESCE(link.original_acquired_on, '')
		FROM effective_investment_transfer_links link
		JOIN investment_lots lot ON lot.id = link.destination_lot_id
		WHERE link.operation_id = ? ORDER BY link.link_seq`, operationID)
	if err != nil {
		return SpinOffTerms{}, fmt.Errorf("read spin-off links: %w", err)
	}
	defer rows.Close()
	total := exact.NewScaledInt()
	for rows.Next() {
		var link SpinOffLink
		var basis, basisScale sql.NullInt64
		if err := rows.Scan(&link.SourceLotID, &link.DestinationLotID, &link.CostCommodityID,
			&link.SourceQuantityValue, &link.SourceQuantityScale, &link.DestinationQuantityValue, &link.DestinationQuantityScale,
			&link.BasisKnowledge, &basis, &basisScale, &link.OriginalDateKnowledge, &link.OriginalAcquiredOn); err != nil {
			return SpinOffTerms{}, fmt.Errorf("scan spin-off link: %w", err)
		}
		link.BasisKnowledge = normalizedBasisKnowledge(link.BasisKnowledge)
		if link.BasisKnowledge == InvestmentBasisKnown {
			if !basis.Valid || !basisScale.Valid {
				return SpinOffTerms{}, fmt.Errorf("spin-off link of lot %d has a known basis without an amount", link.SourceLotID)
			}
			link.AllocatedBasisValue, link.AllocatedBasisScale = basis.Int64, int(basisScale.Int64)
		}
		total.AddCoefficient(link.DestinationQuantityValue, link.DestinationQuantityScale)
		terms.Plan.Links = append(terms.Plan.Links, link)
	}
	if err := rows.Err(); err != nil {
		return SpinOffTerms{}, fmt.Errorf("read spin-off links: %w", err)
	}
	if terms.Plan.DestinationQuantityValue, err = total.Coefficient(); err != nil {
		return SpinOffTerms{}, err
	}
	terms.Plan.DestinationQuantityScale = total.Scale()
	return terms, nil
}
