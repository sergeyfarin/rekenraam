package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"

	"rekenraam/backend/internal/exact"
)

// Return of capital (slice 5, T-146). The cash receipt posts on the payment
// date (A +r, T −r in the receipt currency, which must be the position's cost
// currency: no implicit FX). The basis action applies at the effective date's
// replay slot to every long lot open then, per share: each lot's exact
// allocation r_i (truncated, with the final remainder on the last lot in
// stable lot order, so the allocations sum to r) reduces its known remaining
// basis by min(r_i, basis_i). The rest, r_i − reduction_i, is an unresolved
// excess: never negative basis, never income. Each effect is an immutable
// replay output with a basis_reduction lot event; history that changes them
// appends a revision of the whole effect set (T-148).

var (
	// ErrCapitalReturnNoHoldings means no lot is open on the effective date.
	ErrCapitalReturnNoHoldings = errors.New("no holdings are open on the return of capital effective date")
)

type CreateCapitalReturnParams struct {
	BookID             int64
	AccountID          int64
	CommodityID        int64
	CostCommodityID    int64
	CashAccountID      int64
	EffectiveOn        string
	PaymentOn          string
	AmountValue        exact.Coefficient
	AmountScale        int
	SourceEvidenceJSON string
}

// CapitalReturnEffect is one entitled lot's share of the receipt.
type CapitalReturnEffect struct {
	LotID                 int64
	EntitledQuantityValue exact.Coefficient
	EntitledQuantityScale int
	AllocatedValue        exact.Coefficient
	AllocatedScale        int
	ReductionValue        exact.Coefficient
	ReductionScale        int
	ExcessValue           exact.Coefficient
	ExcessScale           int
	// newBasis is the lot's remaining basis after the reduction.
	newBasis *exact.ScaledInt
}

type CapitalReturnResult struct {
	Effects []CapitalReturnEffect
}

// capitalReturnEffectsTx derives each entitled lot's allocation, reduction and
// excess from the position's current projection without writing anything.
func capitalReturnEffectsTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64,
	effectiveOn string, amount *exact.ScaledInt) ([]CapitalReturnEffect, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, remaining_quantity_value, remaining_quantity_scale, basis_knowledge,
			remaining_cost_basis_value, remaining_cost_basis_scale
		FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ?
			AND position_side = 'long' AND status = 'open' AND opened_on <= ?
		ORDER BY id`, bookID, accountID, commodityID, costCommodityID, effectiveOn)
	if err != nil {
		return nil, fmt.Errorf("read return of capital entitled lots: %w", err)
	}
	type entitled struct {
		effect CapitalReturnEffect
		basis  *exact.ScaledInt
	}
	var lots []entitled
	quantityScale := 0
	for rows.Next() {
		var lot entitled
		var knowledge string
		var basisValue, basisScale sql.NullInt64
		if err := rows.Scan(&lot.effect.LotID, &lot.effect.EntitledQuantityValue, &lot.effect.EntitledQuantityScale,
			&knowledge, &basisValue, &basisScale); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan return of capital entitled lot: %w", err)
		}
		if lot.effect.EntitledQuantityValue.Sign() <= 0 {
			continue
		}
		// An unknown basis cannot yield a definitive reduction/excess split.
		if knowledge != InvestmentBasisKnown || !basisValue.Valid || !basisScale.Valid {
			rows.Close()
			return nil, ErrUnknownInvestmentBasis
		}
		lot.basis = exact.ScaledIntFromInt64(basisValue.Int64, int(basisScale.Int64))
		quantityScale = max(quantityScale, lot.effect.EntitledQuantityScale)
		lots = append(lots, lot)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("read return of capital entitled lots: %w", err)
	}
	if len(lots) == 0 {
		return nil, ErrCapitalReturnNoHoldings
	}
	// Per-share allocation in integers: quantities aligned to one scale, the
	// receipt at its own scale, truncated, remainder on the last lot.
	quantities := make([]*big.Int, len(lots))
	total := new(big.Int)
	for index, lot := range lots {
		quantity := exact.ScaledIntFromCoefficient(lot.effect.EntitledQuantityValue, lot.effect.EntitledQuantityScale)
		quantity.Align(quantityScale)
		quantities[index] = quantity.BigInt()
		total.Add(total, quantities[index])
	}
	receipt := amount.BigInt()
	allocatedSoFar := new(big.Int)
	effects := make([]CapitalReturnEffect, 0, len(lots))
	for index, lot := range lots {
		share := new(big.Int)
		if index == len(lots)-1 {
			share.Sub(receipt, allocatedSoFar)
		} else {
			share.Mul(receipt, quantities[index])
			share.Quo(share, total)
			allocatedSoFar.Add(allocatedSoFar, share)
		}
		allocated := exact.ScaledIntFromBig(share, amount.Scale())
		reduction := allocated
		if lot.basis.Cmp(allocated) < 0 {
			reduction = lot.basis
		}
		excess := exact.ScaledIntFromBig(share, amount.Scale())
		excess.SubScaled(reduction)
		newBasis := exact.ScaledIntFromBig(lot.basis.BigInt(), lot.basis.Scale())
		newBasis.SubScaled(reduction)
		effect := lot.effect
		var err error
		if effect.AllocatedValue, err = allocated.Coefficient(); err != nil {
			return nil, err
		}
		effect.AllocatedScale = allocated.Scale()
		normalizedReduction := reduction.Normalized()
		if effect.ReductionValue, err = normalizedReduction.Coefficient(); err != nil {
			return nil, err
		}
		effect.ReductionScale = normalizedReduction.Scale()
		normalizedExcess := excess.Normalized()
		if effect.ExcessValue, err = normalizedExcess.Coefficient(); err != nil {
			return nil, err
		}
		effect.ExcessScale = normalizedExcess.Scale()
		effect.newBasis = newBasis
		effects = append(effects, effect)
	}
	return effects, nil
}

// applyCapitalReturnEffectsTx installs the reduced basis in the projection.
func applyCapitalReturnEffectsTx(ctx context.Context, tx *sql.Tx, bookID int64, effects []CapitalReturnEffect,
	at string, actorUserID, auditEventID int64) error {
	for _, effect := range effects {
		basis, err := effect.newBasis.Int64()
		if err != nil || basis < 0 {
			return ErrInvestmentBasisRange
		}
		result, err := tx.ExecContext(ctx, `UPDATE investment_lot_state
			SET remaining_cost_basis_value = ?, remaining_cost_basis_scale = ?,
				updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = NULLIF(?, 0)
			WHERE lot_id = ? AND book_id = ? AND status = 'open' AND basis_knowledge = 'known'`,
			basis, effect.newBasis.Scale(), at, actorUserID, auditEventID, effect.LotID, bookID)
		if err != nil {
			return fmt.Errorf("reduce lot %d basis: %w", effect.LotID, err)
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return fmt.Errorf("%w: return of capital lot %d is not open", ErrInvalidDisposalParams, effect.LotID)
		}
	}
	return nil
}

// sameCapitalReturnEffects reports whether replay reproduced the recorded
// per-lot effects exactly, compared by value.
func sameCapitalReturnEffects(replayed, recorded []CapitalReturnEffect) bool {
	if len(replayed) != len(recorded) {
		return false
	}
	equal := func(a exact.Coefficient, as int, b exact.Coefficient, bs int) bool {
		return exact.ScaledIntFromCoefficient(a, as).Cmp(exact.ScaledIntFromCoefficient(b, bs)) == 0
	}
	for index := range replayed {
		r, c := replayed[index], recorded[index]
		if r.LotID != c.LotID ||
			!equal(r.EntitledQuantityValue, r.EntitledQuantityScale, c.EntitledQuantityValue, c.EntitledQuantityScale) ||
			!equal(r.AllocatedValue, r.AllocatedScale, c.AllocatedValue, c.AllocatedScale) ||
			!equal(r.ReductionValue, r.ReductionScale, c.ReductionValue, c.ReductionScale) ||
			!equal(r.ExcessValue, r.ExcessScale, c.ExcessValue, c.ExcessScale) {
			return false
		}
	}
	return true
}

func (r *InvestmentRepository) CreateCapitalReturn(ctx context.Context, journal CreateTransactionParams, params CreateCapitalReturnParams) (TransactionRecord, CapitalReturnResult, error) {
	return r.createCapitalReturn(ctx, journal, params, false)
}

// SimulateCapitalReturn runs the complete writer, then rolls back.
func (r *InvestmentRepository) SimulateCapitalReturn(ctx context.Context, journal CreateTransactionParams, params CreateCapitalReturnParams) (SimulatedInvestmentWrite, CapitalReturnResult, error) {
	transaction, result, err := r.createCapitalReturn(ctx, journal, params, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, CapitalReturnResult{}, err
	}
	return simulatedInvestmentWrite(transaction), result, nil
}

func (r *InvestmentRepository) createCapitalReturn(ctx context.Context, journal CreateTransactionParams, params CreateCapitalReturnParams, preview bool) (TransactionRecord, CapitalReturnResult, error) {
	write := executeInvestmentWriteTx[CapitalReturnResult]
	if preview {
		write = previewInvestmentWriteTx[CapitalReturnResult]
	}
	return write(ctx, r.database, journal,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (CapitalReturnResult, error) {
			return writeCapitalReturnTx(ctx, tx, journal, params, transaction, auditEventID)
		}, nil)
}

func writeCapitalReturnTx(ctx context.Context, tx *sql.Tx, journal CreateTransactionParams,
	params CreateCapitalReturnParams, transaction TransactionRecord, auditEventID int64,
) (CapitalReturnResult, error) {
	if params.BookID <= 0 || params.AccountID <= 0 || params.CommodityID <= 0 || params.CostCommodityID <= 0 ||
		params.CashAccountID <= 0 || !isDisposalCalendarDate(params.EffectiveOn) ||
		!isDisposalCalendarDate(params.PaymentOn) || params.EffectiveOn > params.PaymentOn ||
		params.AmountValue.Sign() <= 0 {
		return CapitalReturnResult{}, fmt.Errorf("%w: return of capital terms are incomplete", ErrInvalidDisposalParams)
	}
	// A return dated behind a later depletion is admitted through replay:
	// its effects come from the position at its own slot, and every later
	// decision replays under its recorded policy (T-148).
	latest, err := latestPositionRewriteDateTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID)
	if err != nil {
		return CapitalReturnResult{}, err
	}
	backdated := latest != "" && params.EffectiveOn < latest
	amount := exact.ScaledIntFromCoefficient(params.AmountValue, params.AmountScale)
	operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
	if err != nil {
		return CapitalReturnResult{}, err
	}
	var effects []CapitalReturnEffect
	if backdated {
		intents, err := investmentReplayIntentsQuery(ctx, tx, params.BookID, params.AccountID, params.CommodityID,
			params.CostCommodityID, "long")
		if err != nil {
			return CapitalReturnResult{}, err
		}
		intents = append(intents, InvestmentReplayIntent{Kind: "capital_return", OperationID: operationID,
			OrderOperationID: operationID, OperationKind: "return_of_capital", EventDate: params.EffectiveOn,
			EffectSeq: 1, AmountValue: params.AmountValue, AmountScale: params.AmountScale,
			CapitalReturnIsSubject: true, TransactionID: transaction.ID, AuditEventID: auditEventID,
			CreatedByUserID: journal.ActorUserID, CreatedAt: journal.CreatedAt})
		projection, err := simulateInvestmentReplayTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID,
			params.CostCommodityID, intents)
		if err != nil {
			return CapitalReturnResult{}, err
		}
		effects = projection.SubjectCapitalReturn
	} else if effects, err = capitalReturnEffectsTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID,
		params.CostCommodityID, params.EffectiveOn, amount); err != nil {
		return CapitalReturnResult{}, err
	}
	if len(effects) == 0 {
		return CapitalReturnResult{}, ErrCapitalReturnNoHoldings
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_capital_return_facts
		(operation_id, book_id, account_id, commodity_id, cost_commodity_id, cash_account_id,
		 effective_on, payment_on, amount_value, amount_scale, entitlement_rule, source_evidence_json,
		 created_audit_event_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open_lots_per_share', ?, ?)`,
		operationID, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID,
		params.CashAccountID, params.EffectiveOn, params.PaymentOn, params.AmountValue, params.AmountScale,
		params.SourceEvidenceJSON, auditEventID); err != nil {
		return CapitalReturnResult{}, fmt.Errorf("record return of capital fact: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_dates (operation_id, date_role, event_date)
		VALUES (?, 'effective', ?)`, operationID, params.EffectiveOn); err != nil {
		return CapitalReturnResult{}, fmt.Errorf("record return of capital effective date: %w", err)
	}
	for index, effect := range effects {
		reduction := exact.ScaledIntFromCoefficient(effect.ReductionValue, effect.ReductionScale).Negated()
		reductionValue, err := reduction.Coefficient()
		if err != nil {
			return CapitalReturnResult{}, err
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO investment_lot_events (
			book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
			cost_basis_value, cost_basis_scale, metadata_json, created_at, created_by_user_id, created_audit_event_id
		) VALUES (?, ?, 'basis_reduction', ?, ?, '0', 0, ?, ?, '{}', ?, ?, ?)`,
			params.BookID, effect.LotID, transaction.ID, params.EffectiveOn, reductionValue, reduction.Scale(),
			journal.CreatedAt, journal.ActorUserID, auditEventID)
		if err != nil {
			return CapitalReturnResult{}, fmt.Errorf("record basis reduction for lot %d: %w", effect.LotID, err)
		}
		eventID, err := result.LastInsertId()
		if err != nil {
			return CapitalReturnResult{}, fmt.Errorf("read basis reduction event id: %w", err)
		}
		if err := linkLotEffectTx(ctx, tx, operationID, eventID); err != nil {
			return CapitalReturnResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_capital_return_effects
			(operation_id, effect_seq, book_id, lot_id, lot_event_id,
			 entitled_quantity_value, entitled_quantity_scale, allocated_value, allocated_scale,
			 reduction_value, reduction_scale, excess_value, excess_scale)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			operationID, index+1, params.BookID, effect.LotID, eventID,
			effect.EntitledQuantityValue, effect.EntitledQuantityScale, effect.AllocatedValue, effect.AllocatedScale,
			effect.ReductionValue, effect.ReductionScale, effect.ExcessValue, effect.ExcessScale); err != nil {
			return CapitalReturnResult{}, fmt.Errorf("record return of capital effect: %w", err)
		}
	}
	if backdated {
		// The position replays with this return at its slot; later decisions
		// are revised and anything they carry on propagates.
		if err := replayCorrectedPositionTx(ctx, tx, params.BookID, investmentReplayPositionKey{
			params.AccountID, params.CommodityID, params.CostCommodityID},
			operationID, auditEventID, journal.ActorUserID, journal.CreatedAt); err != nil {
			return CapitalReturnResult{}, err
		}
		return CapitalReturnResult{Effects: effects}, nil
	}
	if err := applyCapitalReturnEffectsTx(ctx, tx, params.BookID, effects, journal.CreatedAt,
		journal.ActorUserID, auditEventID); err != nil {
		return CapitalReturnResult{}, err
	}
	if err := requirePositionBasisRangeTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID,
		params.CostCommodityID); err != nil {
		return CapitalReturnResult{}, err
	}
	return CapitalReturnResult{Effects: effects}, nil
}

// capitalReturnIntentsQuery reads the position's effective returns of capital
// as replay intents carrying their effective effects: the latest revision's
// (T-148), else the original commit's.
func capitalReturnIntentsQuery(ctx context.Context, reader queryer, bookID, accountID, commodityID, costCommodityID int64) ([]InvestmentReplayIntent, error) {
	rows, err := reader.QueryContext(ctx, `
		SELECT f.operation_id, o.operation_kind, f.effective_on, f.amount_value, f.amount_scale,
			ev.transaction_id, ev.created_audit_event_id, ev.created_by_user_id, ev.created_at
		FROM investment_capital_return_facts f
		JOIN effective_investment_operations o ON o.id = f.operation_id
		JOIN investment_capital_return_effects first ON first.operation_id = f.operation_id AND first.effect_seq = 1
		JOIN investment_lot_events ev ON ev.id = first.lot_event_id
		WHERE f.book_id = ? AND f.account_id = ? AND f.commodity_id = ? AND f.cost_commodity_id = ?
		ORDER BY f.operation_id`, bookID, accountID, commodityID, costCommodityID)
	if err != nil {
		return nil, fmt.Errorf("read replay returns of capital: %w", err)
	}
	var intents []InvestmentReplayIntent
	for rows.Next() {
		intent := InvestmentReplayIntent{Kind: "capital_return", EffectSeq: 1}
		if err := rows.Scan(&intent.OperationID, &intent.OperationKind, &intent.EventDate,
			&intent.AmountValue, &intent.AmountScale, &intent.TransactionID, &intent.AuditEventID,
			&intent.CreatedByUserID, &intent.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan replay return of capital: %w", err)
		}
		intents = append(intents, intent)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("read replay returns of capital: %w", err)
	}
	for index := range intents {
		effects, _, err := effectiveCapitalReturnEffectsQuery(ctx, reader, intents[index].OperationID)
		if err != nil {
			return nil, err
		}
		intents[index].CapitalReturnEffects = effects
	}
	return intents, nil
}

// effectiveCapitalReturnEffectsQuery returns a return of capital's current
// per-lot effects and the revision they come from (zero for the original).
func effectiveCapitalReturnEffectsQuery(ctx context.Context, reader queryer, operationID int64) ([]CapitalReturnEffect, int64, error) {
	var revisionID int64
	query := `SELECT lot_id, entitled_quantity_value, entitled_quantity_scale, allocated_value, allocated_scale,
		reduction_value, reduction_scale, excess_value, excess_scale
		FROM investment_capital_return_effects WHERE operation_id = ? ORDER BY effect_seq`
	args := []any{operationID}
	revisions, err := reader.QueryContext(ctx, `SELECT id FROM latest_investment_capital_return_revisions
		WHERE operation_id = ?`, operationID)
	if err != nil {
		return nil, 0, fmt.Errorf("read return of capital revision: %w", err)
	}
	for revisions.Next() {
		if err := revisions.Scan(&revisionID); err != nil {
			revisions.Close()
			return nil, 0, fmt.Errorf("scan return of capital revision: %w", err)
		}
	}
	if err := errors.Join(revisions.Err(), revisions.Close()); err != nil {
		return nil, 0, fmt.Errorf("read return of capital revision: %w", err)
	}
	if revisionID > 0 {
		query = `SELECT lot_id, entitled_quantity_value, entitled_quantity_scale, allocated_value, allocated_scale,
			reduction_value, reduction_scale, excess_value, excess_scale
			FROM investment_capital_return_revision_effects WHERE revision_id = ? ORDER BY effect_seq`
		args = []any{revisionID}
	}
	rows, err := reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("read return of capital effects: %w", err)
	}
	defer rows.Close()
	var effects []CapitalReturnEffect
	for rows.Next() {
		var effect CapitalReturnEffect
		if err := rows.Scan(&effect.LotID, &effect.EntitledQuantityValue, &effect.EntitledQuantityScale,
			&effect.AllocatedValue, &effect.AllocatedScale, &effect.ReductionValue, &effect.ReductionScale,
			&effect.ExcessValue, &effect.ExcessScale); err != nil {
			return nil, 0, fmt.Errorf("scan return of capital effect: %w", err)
		}
		effects = append(effects, effect)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate return of capital effects: %w", err)
	}
	return effects, revisionID, nil
}

// InvestmentReplayCapitalReturn is a return of capital whose replayed
// effects differ from its effective ones; persisting appends a revision.
type InvestmentReplayCapitalReturn struct {
	OperationID int64
	Effects     []CapitalReturnEffect
}

// persistCapitalReturnRevisionTx appends a revision of the whole effect set.
func persistCapitalReturnRevisionTx(ctx context.Context, tx *sql.Tx, bookID, causedByOperationID, auditEventID int64,
	createdAt string, revision InvestmentReplayCapitalReturn) error {
	_, priorID, err := effectiveCapitalReturnEffectsQuery(ctx, tx, revision.OperationID)
	if err != nil {
		return err
	}
	priorSeq := 1
	if priorID > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT revision_seq FROM investment_capital_return_revisions WHERE id = ?`,
			priorID).Scan(&priorSeq); err != nil {
			return fmt.Errorf("read return of capital revision sequence: %w", err)
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO investment_capital_return_revisions
		(book_id, operation_id, revision_seq, caused_by_operation_id, supersedes_revision_id, created_at, created_audit_event_id)
		VALUES (?, ?, ?, ?, NULLIF(?, 0), ?, ?)`, bookID, revision.OperationID, priorSeq+1, causedByOperationID,
		priorID, createdAt, auditEventID)
	if err != nil {
		return fmt.Errorf("append return of capital revision: %w", err)
	}
	revisionID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read return of capital revision id: %w", err)
	}
	for index, effect := range revision.Effects {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_capital_return_revision_effects
			(revision_id, effect_seq, book_id, lot_id, entitled_quantity_value, entitled_quantity_scale,
			 allocated_value, allocated_scale, reduction_value, reduction_scale, excess_value, excess_scale)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, revisionID, index+1, bookID, effect.LotID,
			effect.EntitledQuantityValue, effect.EntitledQuantityScale, effect.AllocatedValue, effect.AllocatedScale,
			effect.ReductionValue, effect.ReductionScale, effect.ExcessValue, effect.ExcessScale); err != nil {
			return fmt.Errorf("append return of capital revision effect: %w", err)
		}
	}
	return nil
}
