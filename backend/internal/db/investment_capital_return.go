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
	// ErrCapitalReturnEntitlementUnavailable means an explicitly entitled lot
	// is not an open lot of the position on the effective date.
	ErrCapitalReturnEntitlementUnavailable = errors.New("an entitled lot is not open in this position on the return of capital effective date")
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
	// EntitledLotIDs, when set, names the lots the corporate action entitles
	// (explicit_lots, T-148); each takes its whole remaining quantity. Empty
	// applies the per-share rule to every lot open on the effective date.
	EntitledLotIDs []int64
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
	effectiveOn string, amount *exact.ScaledInt, entitledLotIDs []int64) ([]CapitalReturnEffect, error) {
	named := make(map[int64]bool, len(entitledLotIDs))
	for _, lotID := range entitledLotIDs {
		named[lotID] = true
	}
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
		if lot.effect.EntitledQuantityValue.Sign() <= 0 || (len(named) > 0 && !named[lot.effect.LotID]) {
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
	if len(named) > 0 && len(lots) != len(named) {
		return nil, ErrCapitalReturnEntitlementUnavailable
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
			CapitalReturnIsSubject: true, CapitalReturnEntitledLots: params.EntitledLotIDs,
			TransactionID: transaction.ID, AuditEventID: auditEventID,
			CreatedByUserID: journal.ActorUserID, CreatedAt: journal.CreatedAt})
		projection, err := simulateInvestmentReplayTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID,
			params.CostCommodityID, intents)
		if err != nil {
			return CapitalReturnResult{}, err
		}
		effects = projection.SubjectCapitalReturn
	} else if effects, err = capitalReturnEffectsTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID,
		params.CostCommodityID, params.EffectiveOn, amount, params.EntitledLotIDs); err != nil {
		return CapitalReturnResult{}, err
	}
	if len(effects) == 0 {
		return CapitalReturnResult{}, ErrCapitalReturnNoHoldings
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_capital_return_facts
		(operation_id, book_id, account_id, commodity_id, cost_commodity_id, cash_account_id,
		 effective_on, payment_on, amount_value, amount_scale, entitlement_rule, source_evidence_json,
		 created_audit_event_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		operationID, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID,
		params.CashAccountID, params.EffectiveOn, params.PaymentOn, params.AmountValue, params.AmountScale,
		capitalReturnEntitlementRule(params.EntitledLotIDs), params.SourceEvidenceJSON, auditEventID); err != nil {
		return CapitalReturnResult{}, fmt.Errorf("record return of capital fact: %w", err)
	}
	if len(params.EntitledLotIDs) > 0 {
		quantities := make(map[int64]CapitalReturnEffect, len(effects))
		for _, effect := range effects {
			quantities[effect.LotID] = effect
		}
		for index, lotID := range params.EntitledLotIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_capital_return_entitlements
				(operation_id, entitlement_seq, book_id, lot_id, quantity_value, quantity_scale)
				VALUES (?, ?, ?, ?, ?, ?)`, operationID, index+1, params.BookID, lotID,
				quantities[lotID].EntitledQuantityValue, quantities[lotID].EntitledQuantityScale); err != nil {
				return CapitalReturnResult{}, fmt.Errorf("record return of capital entitlement: %w", err)
			}
		}
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
			ev.transaction_id, ev.created_audit_event_id, ev.created_by_user_id, ev.created_at,
			f.entitlement_rule = 'explicit_lots'
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
	var explicit []bool
	for rows.Next() {
		intent := InvestmentReplayIntent{Kind: "capital_return", EffectSeq: 1}
		var isExplicit bool
		if err := rows.Scan(&intent.OperationID, &intent.OperationKind, &intent.EventDate,
			&intent.AmountValue, &intent.AmountScale, &intent.TransactionID, &intent.AuditEventID,
			&intent.CreatedByUserID, &intent.CreatedAt, &isExplicit); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan replay return of capital: %w", err)
		}
		intents = append(intents, intent)
		explicit = append(explicit, isExplicit)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("read replay returns of capital: %w", err)
	}
	for index := range intents {
		if explicit[index] {
			if intents[index].capitalReturnEntitlementSources, err = capitalReturnEntitlementsQuery(ctx, reader,
				intents[index].OperationID); err != nil {
				return nil, err
			}
		}
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

// CapitalReturnOperationRecord pins a posted return of capital and its
// holding for correction (T-148). A correction rechecks it inside the write.
type CapitalReturnOperationRecord struct {
	OperationID          int64
	TransactionID        int64
	TransactionVersionID int64
	CurrentVersionID     int64
	EventDate            string
	AccountID            int64
	CommodityID          int64
	CostCommodityID      int64
	AlreadyCorrected     bool
	ImportedLineage      bool
}

func (r *InvestmentRepository) CapitalReturnOperationByTransactionID(ctx context.Context, bookID, transactionID int64) (CapitalReturnOperationRecord, error) {
	return capitalReturnOperationByTransactionIDQuery(ctx, r.database, bookID, transactionID)
}

func capitalReturnOperationByTransactionIDQuery(ctx context.Context, reader saleOperationReader, bookID, transactionID int64) (CapitalReturnOperationRecord, error) {
	var record CapitalReturnOperationRecord
	var corrected int
	err := reader.QueryRowContext(ctx, `SELECT o.id, linked_version.transaction_id, link.transaction_version_id,
		current.id, o.event_date, f.account_id, f.commodity_id, f.cost_commodity_id,
		EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id)
		FROM investment_operations o
		JOIN investment_capital_return_facts f ON f.operation_id = o.id
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
		WHERE o.book_id = ? AND linked_version.transaction_id = ? AND o.operation_kind = 'return_of_capital'`,
		bookID, transactionID).Scan(&record.OperationID, &record.TransactionID, &record.TransactionVersionID,
		&record.CurrentVersionID, &record.EventDate, &record.AccountID, &record.CommodityID, &record.CostCommodityID,
		&corrected)
	if errors.Is(err, sql.ErrNoRows) {
		return CapitalReturnOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return CapitalReturnOperationRecord{}, fmt.Errorf("read return of capital operation: %w", err)
	}
	record.AlreadyCorrected = corrected != 0
	if record.ImportedLineage, err = investmentOperationHasImportedLineageQuery(ctx, reader, bookID, record.OperationID); err != nil {
		return CapitalReturnOperationRecord{}, err
	}
	return record, nil
}

func checkCapitalReturnOperationForCorrectionTx(ctx context.Context, tx *sql.Tx, bookID int64, expected CapitalReturnOperationRecord) error {
	current, err := capitalReturnOperationByTransactionIDQuery(ctx, tx, bookID, expected.TransactionID)
	if err != nil {
		return err
	}
	if current.AlreadyCorrected {
		return ErrInvestmentOperationAlreadyCorrected
	}
	if current.ImportedLineage {
		linked, err := investmentOperationHasCommittedSourceQuery(ctx, tx, bookID, current.OperationID)
		if err != nil {
			return err
		}
		if !linked {
			return ErrInvestmentImportedCorrection
		}
	}
	if current != expected {
		return ErrInvestmentSaleChanged
	}
	return checkInvestmentSourceJournalTx(ctx, tx, bookID, current.TransactionID,
		current.EventDate, current.TransactionVersionID, current.CurrentVersionID)
}

// ReverseCapitalReturn posts the exact inverse of the receipt as a reversal
// operation, which removes the basis action from effective history, and
// replays the holding so its lots get their basis back (T-148).
func (r *InvestmentRepository) ReverseCapitalReturn(ctx context.Context, params CreateTransactionParams, expected CapitalReturnOperationRecord) (TransactionRecord, error) {
	return r.reverseCapitalReturn(ctx, params, expected, false)
}

// PreviewCapitalReturnReversal runs the reversal writer and its replay, then
// rolls back.
func (r *InvestmentRepository) PreviewCapitalReturnReversal(ctx context.Context, params CreateTransactionParams, expected CapitalReturnOperationRecord) (SimulatedInvestmentWrite, error) {
	transaction, err := r.reverseCapitalReturn(ctx, params, expected, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func (r *InvestmentRepository) reverseCapitalReturn(ctx context.Context, params CreateTransactionParams, expected CapitalReturnOperationRecord, preview bool) (TransactionRecord, error) {
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 || expected.AccountID <= 0 ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.TransactionKind != "investment" ||
		params.Spec.Status != "posted" || params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" || params.InvestmentCorrectionReason == "" ||
		!params.CorrectionOfTransactionID.Valid || params.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return TransactionRecord{}, fmt.Errorf("%w: return of capital reversal is incomplete", ErrInvalidDisposalParams)
	}
	guard := func(tx *sql.Tx) error {
		return checkCapitalReturnOperationForCorrectionTx(ctx, tx, params.BookID, expected)
	}
	effect := func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
		operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, replayCorrectedPositionTx(ctx, tx, params.BookID, investmentReplayPositionKey{
			expected.AccountID, expected.CommodityID, expected.CostCommodityID},
			operationID, auditEventID, params.ActorUserID, params.CreatedAt)
	}
	var transaction TransactionRecord
	var err error
	if preview {
		transaction, _, err = previewInvestmentWriteWithGuardTx(ctx, r.database, params, guard, effect)
	} else {
		transaction, _, err = executeInvestmentWriteWithGuardTx(ctx, r.database, params, guard, effect, nil)
	}
	return transaction, err
}

func capitalReturnEntitlementRule(entitledLotIDs []int64) string {
	if len(entitledLotIDs) > 0 {
		return "explicit_lots"
	}
	return "open_lots_per_share"
}

// capitalReturnEntitlementsQuery reads the lots an explicit_lots return of
// capital names, with the opening each came from, so replay can follow a
// corrected acquisition to its successor lot.
func capitalReturnEntitlementsQuery(ctx context.Context, reader queryer, operationID int64) ([]transferSourceOpening, error) {
	rows, err := reader.QueryContext(ctx, `SELECT e.lot_id, COALESCE(l.operation_id, 0), l.opened_on
		FROM investment_capital_return_entitlements e JOIN investment_lots l ON l.id = e.lot_id
		WHERE e.operation_id = ? ORDER BY e.entitlement_seq`, operationID)
	if err != nil {
		return nil, fmt.Errorf("read return of capital entitlements: %w", err)
	}
	defer rows.Close()
	var sources []transferSourceOpening
	for rows.Next() {
		var source transferSourceOpening
		if err := rows.Scan(&source.lotID, &source.operationID, &source.openedOn); err != nil {
			return nil, fmt.Errorf("scan return of capital entitlement: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate return of capital entitlements: %w", err)
	}
	return sources, nil
}
