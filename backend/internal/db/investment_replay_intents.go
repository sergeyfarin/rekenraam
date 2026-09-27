package db

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"

	"rekenraam/backend/internal/exact"
)

// InvestmentReplayIntent is an immutable economic input to a future position
// rebuild. In particular, a disposal carries its elected method and any
// explicit specific-lot choice, never the lots selected by FIFO/LIFO/average.
type InvestmentReplayIntent struct {
	OperationID     int64
	OperationKind   string
	EffectSeq       int
	EventDate       string
	Kind            string // opening or disposal
	LotID           int64  // opening only
	DecisionID      int64  // disposal only
	QuantityValue   exact.Coefficient
	QuantityScale   int
	AmountValue     exact.Coefficient // opening consideration or disposal proceeds
	AmountScale     int
	CostBasisMethod string
	DecisionSource  DisposalDecisionSource
	SpecificLots    []LotAllocation
	TransactionID   int64
	AuditEventID    int64
	CreatedByUserID int64
	CreatedAt       string
}

func (r *InvestmentRepository) ListInvestmentReplayIntents(ctx context.Context, bookID, accountID, commodityID, costCommodityID int64, side string) ([]InvestmentReplayIntent, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin replay intent snapshot: %w", err)
	}
	defer rollbackTx(ctx, tx)
	intents, err := investmentReplayIntentsQuery(ctx, tx, bookID, accountID, commodityID, costCommodityID, side)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("close replay intent snapshot: %w", err)
	}
	return intents, nil
}

func investmentReplayIntentsQuery(ctx context.Context, reader queryer, bookID, accountID, commodityID, costCommodityID int64, side string) ([]InvestmentReplayIntent, error) {
	if bookID <= 0 || accountID <= 0 || commodityID <= 0 || costCommodityID <= 0 || side != "long" {
		return nil, fmt.Errorf("%w: replay requires a long position and its exact book, account, instrument and cost currency", ErrInvalidDisposalParams)
	}
	openings, err := reader.QueryContext(ctx, `
		SELECT f.lot_id, f.operation_id, o.operation_kind, f.opened_on,
			f.quantity_value, f.quantity_scale, f.consideration_value, f.consideration_scale,
			(SELECT x.effect_seq FROM investment_operation_lot_effects x
			 JOIN investment_lot_events e ON e.id = x.lot_event_id
			 WHERE x.operation_id = f.operation_id AND e.lot_id = f.lot_id
			   AND e.event_kind IN ('acquisition', 'reinvested_dividend')
			 ORDER BY x.effect_seq LIMIT 1)
		FROM investment_lot_facts f JOIN investment_operations o ON o.id = f.operation_id
		WHERE f.book_id = ? AND f.account_id = ? AND f.commodity_id = ?
			AND f.cost_commodity_id = ? AND f.position_side = ?
	`, bookID, accountID, commodityID, costCommodityID, side)
	if err != nil {
		return nil, fmt.Errorf("read replay openings: %w", err)
	}
	intents := make([]InvestmentReplayIntent, 0)
	for openings.Next() {
		var intent InvestmentReplayIntent
		var seq sql.NullInt64
		if err := openings.Scan(&intent.LotID, &intent.OperationID, &intent.OperationKind, &intent.EventDate,
			&intent.QuantityValue, &intent.QuantityScale, &intent.AmountValue, &intent.AmountScale, &seq); err != nil {
			openings.Close()
			return nil, fmt.Errorf("scan replay opening: %w", err)
		}
		if !seq.Valid || seq.Int64 <= 0 {
			openings.Close()
			return nil, fmt.Errorf("replay opening lot %d has no immutable operation effect", intent.LotID)
		}
		intent.EffectSeq = int(seq.Int64)
		intent.Kind = "opening"
		intents = append(intents, intent)
	}
	if err := openings.Err(); err != nil {
		openings.Close()
		return nil, fmt.Errorf("iterate replay openings: %w", err)
	}
	if err := openings.Close(); err != nil {
		return nil, fmt.Errorf("close replay openings: %w", err)
	}

	disposals, err := reader.QueryContext(ctx, `
		SELECT d.id, d.operation_id, o.operation_kind, d.event_date,
			d.quantity_value, d.quantity_scale, d.proceeds_value, d.proceeds_scale,
			d.cost_basis_method, d.resolution_tier, d.account_version_id,
			d.profile_id, d.profile_version_id, d.source_effective_from,
			d.source_recorded_at, d.transaction_id, d.created_audit_event_id,
			d.created_by_user_id, d.created_at,
			(SELECT MIN(x.effect_seq) FROM investment_disposal_allocations a
			 JOIN investment_operation_lot_effects x ON x.lot_event_id = a.lot_event_id
			 WHERE a.decision_id = d.id AND x.operation_id = d.operation_id)
		FROM investment_disposal_decisions d JOIN investment_operations o ON o.id = d.operation_id
		WHERE d.book_id = ? AND d.account_id = ? AND d.commodity_id = ?
			AND d.cost_commodity_id = ? AND d.position_side = ?
	`, bookID, accountID, commodityID, costCommodityID, side)
	if err != nil {
		return nil, fmt.Errorf("read replay disposals: %w", err)
	}
	for disposals.Next() {
		var intent InvestmentReplayIntent
		var seq sql.NullInt64
		var accountVersionID, profileID, profileVersionID sql.NullInt64
		var effectiveFrom, recordedAt sql.NullString
		if err := disposals.Scan(&intent.DecisionID, &intent.OperationID, &intent.OperationKind, &intent.EventDate,
			&intent.QuantityValue, &intent.QuantityScale, &intent.AmountValue, &intent.AmountScale,
			&intent.CostBasisMethod, &intent.DecisionSource.ResolutionTier, &accountVersionID,
			&profileID, &profileVersionID, &effectiveFrom, &recordedAt,
			&intent.TransactionID, &intent.AuditEventID, &intent.CreatedByUserID,
			&intent.CreatedAt, &seq); err != nil {
			disposals.Close()
			return nil, fmt.Errorf("scan replay disposal: %w", err)
		}
		if !seq.Valid || seq.Int64 <= 0 {
			disposals.Close()
			return nil, fmt.Errorf("replay disposal decision %d has no immutable operation effect", intent.DecisionID)
		}
		intent.EffectSeq = int(seq.Int64)
		intent.Kind = "disposal"
		intent.DecisionSource.AccountVersionID = accountVersionID.Int64
		intent.DecisionSource.ProfileID = profileID.Int64
		intent.DecisionSource.ProfileVersionID = profileVersionID.Int64
		intent.DecisionSource.SourceEffectiveFrom = effectiveFrom.String
		intent.DecisionSource.SourceRecordedAt = recordedAt.String
		intents = append(intents, intent)
	}
	if err := disposals.Err(); err != nil {
		disposals.Close()
		return nil, fmt.Errorf("iterate replay disposals: %w", err)
	}
	if err := disposals.Close(); err != nil {
		return nil, fmt.Errorf("close replay disposals: %w", err)
	}

	selected, err := reader.QueryContext(ctx, `
		SELECT a.decision_id, a.lot_id, a.quantity_value, a.quantity_scale
		FROM investment_disposal_allocations a
		JOIN investment_disposal_decisions d ON d.id = a.decision_id
		WHERE d.book_id = ? AND d.account_id = ? AND d.commodity_id = ?
			AND d.cost_commodity_id = ? AND d.position_side = ? AND d.cost_basis_method = 'specific_lot'
		ORDER BY a.decision_id, a.allocation_seq
	`, bookID, accountID, commodityID, costCommodityID, side)
	if err != nil {
		return nil, fmt.Errorf("read replay specific-lot choices: %w", err)
	}
	choices := make(map[int64][]LotAllocation)
	for selected.Next() {
		var decisionID int64
		var choice LotAllocation
		if err := selected.Scan(&decisionID, &choice.LotID, &choice.QuantityValue, &choice.QuantityScale); err != nil {
			selected.Close()
			return nil, fmt.Errorf("scan replay specific-lot choice: %w", err)
		}
		choices[decisionID] = append(choices[decisionID], choice)
	}
	if err := selected.Err(); err != nil {
		selected.Close()
		return nil, fmt.Errorf("iterate replay specific-lot choices: %w", err)
	}
	if err := selected.Close(); err != nil {
		return nil, fmt.Errorf("close replay specific-lot choices: %w", err)
	}
	for index := range intents {
		if intents[index].Kind == "disposal" && intents[index].CostBasisMethod == "specific_lot" {
			intents[index].SpecificLots = choices[intents[index].DecisionID]
			if len(intents[index].SpecificLots) == 0 {
				return nil, fmt.Errorf("replay specific-lot decision %d has no elected lots", intents[index].DecisionID)
			}
		}
	}
	sortInvestmentReplayIntents(intents)
	return intents, nil
}

func sortInvestmentReplayIntents(intents []InvestmentReplayIntent) {
	slices.SortFunc(intents, func(a, b InvestmentReplayIntent) int {
		if order := cmp.Compare(a.EventDate, b.EventDate); order != 0 {
			return order
		}
		if order := cmp.Compare(a.OperationID, b.OperationID); order != 0 {
			return order
		}
		return cmp.Compare(a.EffectSeq, b.EffectSeq)
	})
}
