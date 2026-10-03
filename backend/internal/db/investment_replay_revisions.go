package db

import (
	"context"
	"database/sql"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// persistInvestmentReplayProjectionTx appends the effective allocation sets
// and installs the current lot projection in the same transaction as the
// operation that caused replay. The original decision, allocations and lot
// events are immutable evidence of the first posting.
func persistInvestmentReplayProjectionTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID,
	causedByOperationID, auditEventID, actorUserID int64, createdAt string,
	intents []InvestmentReplayIntent, projection InvestmentReplayProjection) error {
	if bookID <= 0 || accountID <= 0 || commodityID <= 0 || costCommodityID <= 0 ||
		causedByOperationID <= 0 || auditEventID <= 0 || actorUserID <= 0 || createdAt == "" {
		return fmt.Errorf("%w: replay revision provenance or position is incomplete", ErrInvalidDisposalParams)
	}
	if projection.MethodFamily != "" && projection.MethodFamily != "individual_lot" && projection.MethodFamily != "average_cost" {
		return fmt.Errorf("%w: replay method family is invalid", ErrInvalidDisposalParams)
	}
	var lotCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM current_investment_lots WHERE book_id = ?
		AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND position_side = 'long'`,
		bookID, accountID, commodityID, costCommodityID).Scan(&lotCount); err != nil {
		return fmt.Errorf("count replay position lots: %w", err)
	}
	if lotCount != len(projection.Lots) {
		return fmt.Errorf("%w: replay lot state does not cover the whole position", ErrInvalidDisposalParams)
	}
	lotIDs := make(map[int64]bool, len(projection.Lots))
	for _, lot := range projection.Lots {
		if lot.LotID <= 0 || lotIDs[lot.LotID] || (lot.Status != "open" && lot.Status != "closed") ||
			lot.RemainingQuantityValue.Sign() < 0 || lot.RemainingCostBasisValue < 0 {
			return fmt.Errorf("%w: replay lot state is invalid", ErrInvalidDisposalParams)
		}
		lotIDs[lot.LotID] = true
	}
	decisions := make(map[int64]InvestmentReplayIntent)
	for _, intent := range intents {
		if intent.Kind == "disposal" {
			if intent.DecisionID <= 0 || decisions[intent.DecisionID].DecisionID != 0 {
				return fmt.Errorf("%w: duplicate replay disposal intent", ErrInvalidDisposalParams)
			}
			decisions[intent.DecisionID] = intent
		}
	}
	if len(decisions) != len(projection.Disposals) {
		return fmt.Errorf("%w: replay disposal output does not cover its intents", ErrInvalidDisposalParams)
	}
	seen := make(map[int64]bool, len(projection.Disposals))
	for _, disposal := range projection.Disposals {
		intent, exists := decisions[disposal.DecisionID]
		if !exists || seen[disposal.DecisionID] || len(disposal.Allocations) == 0 {
			return fmt.Errorf("%w: replay disposal output is missing or repeated", ErrInvalidDisposalParams)
		}
		seen[disposal.DecisionID] = true
		quantity, basis, proceeds := exact.NewScaledInt(), exact.NewScaledInt(), exact.NewScaledInt()
		for _, allocation := range disposal.Allocations {
			if !lotIDs[allocation.LotID] || allocation.QuantityValue.Sign() <= 0 ||
				allocation.CostBasisValue < 0 {
				return fmt.Errorf("%w: replay allocation is invalid", ErrInvalidDisposalParams)
			}
			quantity.AddCoefficient(allocation.QuantityValue, allocation.QuantityScale)
			basis.AddInt64(allocation.CostBasisValue, allocation.CostBasisScale)
			proceeds.AddInt64(allocation.ProceedsValue, allocation.ProceedsScale)
		}
		wantQuantity := exact.ScaledIntFromCoefficient(intent.QuantityValue, intent.QuantityScale)
		wantProceeds := exact.ScaledIntFromCoefficient(intent.AmountValue, intent.AmountScale)
		if quantity.Cmp(wantQuantity) != 0 || proceeds.Cmp(wantProceeds) != 0 {
			return fmt.Errorf("%w: replay allocation does not conserve quantity and proceeds for decision %d", ErrInvalidDisposalParams, disposal.DecisionID)
		}
		basisValue, err := basis.Coefficient()
		if err != nil {
			return fmt.Errorf("compute replay disposed basis: %w", err)
		}
		var priorID sql.NullInt64
		var priorSeq int
		err = tx.QueryRowContext(ctx, `SELECT id, revision_seq FROM investment_disposal_revisions
			WHERE book_id = ? AND decision_id = ? ORDER BY revision_seq DESC LIMIT 1`,
			bookID, disposal.DecisionID).Scan(&priorID, &priorSeq)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read current disposal revision: %w", err)
		}
		if err == sql.ErrNoRows {
			priorSeq = 1
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO investment_disposal_revisions (
			book_id, decision_id, revision_seq, caused_by_operation_id, supersedes_revision_id,
			disposed_basis_value, disposed_basis_scale, created_at, created_audit_event_id
		) SELECT ?, d.id, ?, ?, ?, ?, ?, ?, ? FROM investment_disposal_decisions d
			WHERE d.id = ? AND d.book_id = ? AND d.account_id = ? AND d.commodity_id = ?
				AND d.cost_commodity_id = ? AND d.position_side = 'long'`,
			bookID, priorSeq+1, causedByOperationID, priorID, basisValue, basis.Scale(),
			createdAt, auditEventID, disposal.DecisionID, bookID, accountID, commodityID, costCommodityID)
		if err != nil {
			return fmt.Errorf("append disposal revision for decision %d: %w", disposal.DecisionID, err)
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return fmt.Errorf("%w: replay decision %d is outside its position", ErrInvalidDisposalParams, disposal.DecisionID)
		}
		revisionID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("read disposal revision id: %w", err)
		}
		for index, allocation := range disposal.Allocations {
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_disposal_revision_allocations (
				book_id, revision_id, allocation_seq, lot_id, quantity_value, quantity_scale,
				cost_basis_value, cost_basis_scale, proceeds_value, proceeds_scale
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				bookID, revisionID, index+1, allocation.LotID, allocation.QuantityValue,
				allocation.QuantityScale, allocation.CostBasisValue, allocation.CostBasisScale,
				allocation.ProceedsValue, allocation.ProceedsScale); err != nil {
				return fmt.Errorf("append disposal revision allocation: %w", err)
			}
		}
	}
	splits := 0
	for _, intent := range intents {
		if intent.Kind == "split" {
			splits++
		}
	}
	if splits != len(projection.Splits) {
		return fmt.Errorf("%w: replay split output does not cover its intents", ErrInvalidDisposalParams)
	}
	for _, split := range projection.Splits {
		for _, effect := range split.Effects {
			if !lotIDs[effect.LotID] || effect.CostCommodityID != costCommodityID {
				return fmt.Errorf("%w: replay split effect is outside its position", ErrInvalidDisposalParams)
			}
		}
		if err := persistSplitRevisionTx(ctx, tx, bookID, costCommodityID, causedByOperationID,
			auditEventID, createdAt, split); err != nil {
			return err
		}
	}
	for _, lot := range projection.Lots {
		result, err := tx.ExecContext(ctx, `INSERT INTO investment_lot_state (status,
		remaining_quantity_value, remaining_quantity_scale, remaining_cost_basis_value, remaining_cost_basis_scale,
		updated_at, updated_by_user_id, updated_audit_event_id, lot_id, book_id)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, id, book_id FROM investment_lots
		WHERE id = ? AND book_id = ? AND account_id = ? AND commodity_id = ?
		AND cost_commodity_id = ? AND position_side = 'long'
		ON CONFLICT(lot_id) DO UPDATE SET status = excluded.status,
		remaining_quantity_value = excluded.remaining_quantity_value, remaining_quantity_scale = excluded.remaining_quantity_scale,
		remaining_cost_basis_value = excluded.remaining_cost_basis_value, remaining_cost_basis_scale = excluded.remaining_cost_basis_scale, basis_knowledge = 'known',
		updated_at = excluded.updated_at, updated_by_user_id = excluded.updated_by_user_id, updated_audit_event_id = excluded.updated_audit_event_id`,
			lot.Status, lot.RemainingQuantityValue, lot.RemainingQuantityScale, lot.RemainingCostBasisValue, lot.RemainingCostBasisScale,
			createdAt, actorUserID, auditEventID, lot.LotID, bookID, accountID, commodityID, costCommodityID)
		if err != nil {
			return fmt.Errorf("install replay lot projection: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return fmt.Errorf("%w: replay lot is outside its position", ErrInvalidDisposalParams)
		}
	}
	if projection.MethodFamily == "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM investment_position_basis_state
			WHERE book_id = ? AND account_id = ? AND commodity_id = ?
				AND cost_commodity_id = ? AND position_side = 'long'`,
			bookID, accountID, commodityID, costCommodityID); err != nil {
			return fmt.Errorf("clear replay method-family state: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_position_basis_state (
			book_id, account_id, commodity_id, cost_commodity_id, method_family,
			position_side, updated_at, updated_by_user_id, updated_audit_event_id
		) VALUES (?, ?, ?, ?, ?, 'long', ?, ?, ?)
		ON CONFLICT(book_id, account_id, commodity_id, cost_commodity_id, position_side)
		DO UPDATE SET method_family = excluded.method_family, updated_at = excluded.updated_at,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_audit_event_id = excluded.updated_audit_event_id`,
			bookID, accountID, commodityID, costCommodityID, projection.MethodFamily,
			createdAt, actorUserID, auditEventID); err != nil {
			return fmt.Errorf("install replay method-family state: %w", err)
		}
	}
	return nil
}
