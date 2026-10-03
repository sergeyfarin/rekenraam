package db

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"

	"rekenraam/backend/internal/exact"
)

// InvestmentReplayIntent is an immutable economic input from the effective
// end of a correction chain. Superseded facts remain stored for audit but are
// excluded from current replay. A disposal carries its elected method and any
// explicit specific-lot choice, never the lots selected by FIFO/LIFO/average.
type InvestmentReplayIntent struct {
	OperationID int64
	// OrderOperationID is the root operation's original same-day slot. A
	// replacement inherits that slot so a later same-day sale still follows
	// the corrected acquisition when replay sorts the effective intents.
	OrderOperationID int64
	OperationKind    string
	EffectSeq        int
	EventDate        string
	Kind             string // opening, disposal, transfer_out, pooled_transfer_out or split
	LotID            int64  // opening or transfer_out
	DecisionID       int64  // disposal only
	// QuantityValue is the opening, disposal or transfer quantity. For a split
	// it is the signed holding delta the split's effective effects moved in
	// this cost currency, which replay must reproduce exactly.
	QuantityValue    exact.Coefficient
	QuantityScale    int
	RatioNumerator   int64 // split only
	RatioDenominator int64 // split only
	// SplitIsSubject marks the split the current command is creating. It has
	// no recorded effects yet, so replay reports its effects instead of
	// checking them.
	SplitIsSubject  bool
	AmountValue     exact.Coefficient // opening consideration or disposal proceeds
	AmountScale     int
	CostBasisMethod string
	DecisionSource  DisposalDecisionSource
	SpecificLots    []LotAllocation
	// PooledLinks are a pooled transfer's committed per-lot carried amounts,
	// in link order. Replay must reproduce them exactly.
	PooledLinks     []InvestmentReplayTransferLink
	TransactionID   int64
	AuditEventID    int64
	CreatedByUserID int64
	CreatedAt       string
}

// InvestmentReplayTransferLink is one source lot's committed depletion in a
// pooled internal transfer.
type InvestmentReplayTransferLink struct {
	LotID          int64
	QuantityValue  exact.Coefficient
	QuantityScale  int
	CostBasisValue exact.Coefficient
	CostBasisScale int
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
			   AND e.event_kind IN ('acquisition', 'reinvested_dividend', 'transfer_in')
			 ORDER BY x.effect_seq LIMIT 1)
		FROM investment_lot_facts f JOIN investment_operations o ON o.id = f.operation_id
		WHERE f.book_id = ? AND f.account_id = ? AND f.commodity_id = ?
			AND f.cost_commodity_id = ? AND f.position_side = ?
			AND o.correction_mode IS NOT 'reverse'
			AND NOT EXISTS (SELECT 1 FROM investment_operations successor
				WHERE successor.correction_of_operation_id = o.id)
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
			AND o.correction_mode IS NOT 'reverse'
			AND NOT EXISTS (SELECT 1 FROM investment_operations successor
				WHERE successor.correction_of_operation_id = o.id)
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
	transfers, err := reader.QueryContext(ctx, `
		SELECT f.operation_id, o.operation_kind, f.effective_on, x.source_lot_id,
			x.quantity_value, x.quantity_scale, x.carried_basis_value, x.carried_basis_scale,
			e.transaction_id, e.created_audit_event_id, e.created_by_user_id, e.created_at,
			effect.effect_seq, f.basis_allocation
		FROM investment_transfer_facts f
		JOIN investment_transfer_lot_links x ON x.operation_id = f.operation_id
		JOIN investment_operations o ON o.id = f.operation_id
		JOIN investment_operation_lot_effects effect ON effect.operation_id = f.operation_id
		JOIN investment_lot_events e ON e.id = effect.lot_event_id
			AND e.lot_id = x.source_lot_id AND e.event_kind = 'transfer_out'
		WHERE f.book_id = ? AND f.source_account_id = ? AND f.commodity_id = ?
			AND f.transfer_kind = 'internal' AND x.cost_commodity_id = ?
			AND o.correction_mode IS NOT 'reverse'
			AND NOT EXISTS (SELECT 1 FROM investment_operations successor
				WHERE successor.correction_of_operation_id = o.id)
		ORDER BY f.operation_id, x.link_seq
	`, bookID, accountID, commodityID, costCommodityID)
	if err != nil {
		return nil, fmt.Errorf("read replay transfer depletions: %w", err)
	}
	// A pooled transfer is one intent: replay depletes the pool once for its
	// total quantity, then compares every link it produced.
	pooled := make(map[int64]int)
	for transfers.Next() {
		var intent InvestmentReplayIntent
		var basis sql.NullString
		var basisScale sql.NullInt64
		var allocation sql.NullString
		if err := transfers.Scan(&intent.OperationID, &intent.OperationKind, &intent.EventDate,
			&intent.LotID, &intent.QuantityValue, &intent.QuantityScale, &basis, &basisScale,
			&intent.TransactionID, &intent.AuditEventID, &intent.CreatedByUserID,
			&intent.CreatedAt, &intent.EffectSeq, &allocation); err != nil {
			transfers.Close()
			return nil, fmt.Errorf("scan replay transfer depletion: %w", err)
		}
		if !basis.Valid || !basisScale.Valid || intent.EffectSeq <= 0 {
			transfers.Close()
			return nil, fmt.Errorf("%w: transfer operation %d lacks known basis or an effect", ErrInvalidDisposalParams, intent.OperationID)
		}
		intent.AmountValue = exact.Coefficient(basis.String)
		intent.AmountScale = int(basisScale.Int64)
		intent.Kind = "transfer_out"
		if allocation.String != InternalTransferAverageCostPool {
			intents = append(intents, intent)
			continue
		}
		link := InvestmentReplayTransferLink{LotID: intent.LotID, QuantityValue: intent.QuantityValue,
			QuantityScale: intent.QuantityScale, CostBasisValue: intent.AmountValue, CostBasisScale: intent.AmountScale}
		index, exists := pooled[intent.OperationID]
		if !exists {
			intent.Kind = "pooled_transfer_out"
			intent.LotID = 0
			intent.AmountValue, intent.AmountScale = "", 0
			pooled[intent.OperationID] = len(intents)
			intents = append(intents, intent)
			index = len(intents) - 1
		} else {
			total := exact.ScaledIntFromCoefficient(intents[index].QuantityValue, intents[index].QuantityScale)
			total.AddCoefficient(intent.QuantityValue, intent.QuantityScale)
			quantity, err := total.Coefficient()
			if err != nil {
				transfers.Close()
				return nil, fmt.Errorf("total pooled transfer %d quantity: %w", intent.OperationID, err)
			}
			intents[index].QuantityValue, intents[index].QuantityScale = quantity, total.Scale()
			intents[index].EffectSeq = min(intents[index].EffectSeq, intent.EffectSeq)
		}
		intents[index].PooledLinks = append(intents[index].PooledLinks, link)
	}
	if err := transfers.Err(); err != nil {
		transfers.Close()
		return nil, fmt.Errorf("iterate replay transfer depletions: %w", err)
	}
	if err := transfers.Close(); err != nil {
		return nil, fmt.Errorf("close replay transfer depletions: %w", err)
	}

	splits, err := investmentReplaySplitIntentsQuery(ctx, reader, bookID, accountID, commodityID, costCommodityID)
	if err != nil {
		return nil, err
	}
	intents = append(intents, splits...)

	selected, err := reader.QueryContext(ctx, `
		SELECT a.decision_id, a.lot_id, a.quantity_value, a.quantity_scale,
			source.operation_id
		FROM investment_disposal_allocations a
		JOIN investment_disposal_decisions d ON d.id = a.decision_id
		JOIN investment_operations o ON o.id = d.operation_id
		JOIN investment_lot_facts source ON source.lot_id = a.lot_id
		WHERE d.book_id = ? AND d.account_id = ? AND d.commodity_id = ?
			AND d.cost_commodity_id = ? AND d.position_side = ? AND d.cost_basis_method = 'specific_lot'
			AND o.correction_mode IS NOT 'reverse'
			AND NOT EXISTS (SELECT 1 FROM investment_operations successor
				WHERE successor.correction_of_operation_id = o.id)
		ORDER BY a.decision_id, a.allocation_seq
	`, bookID, accountID, commodityID, costCommodityID, side)
	if err != nil {
		return nil, fmt.Errorf("read replay specific-lot choices: %w", err)
	}
	type specificChoice struct {
		Allocation        LotAllocation
		SourceOperationID int64
	}
	choices := make(map[int64][]specificChoice)
	for selected.Next() {
		var decisionID int64
		var choice specificChoice
		if err := selected.Scan(&decisionID, &choice.Allocation.LotID,
			&choice.Allocation.QuantityValue, &choice.Allocation.QuantityScale,
			&choice.SourceOperationID); err != nil {
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
	orderIDs, err := investmentReplayOrderOperationIDsQuery(ctx, reader, bookID)
	if err != nil {
		return nil, err
	}
	for index := range intents {
		orderID, ok := orderIDs[intents[index].OperationID]
		if !ok {
			return nil, fmt.Errorf("replay operation %d has no correction root", intents[index].OperationID)
		}
		intents[index].OrderOperationID = orderID
	}
	// A specific-lot election names the acquisition the user chose. A
	// correction gives that acquisition a new lot row, so its effective replay
	// choice follows the opening's correction root while the original election
	// and allocation retain their source lot ID.
	effectiveLotByRoot := make(map[int64]int64)
	for _, intent := range intents {
		if intent.Kind != "opening" {
			continue
		}
		if previous, exists := effectiveLotByRoot[intent.OrderOperationID]; exists && previous != intent.LotID {
			effectiveLotByRoot[intent.OrderOperationID] = 0
		} else if !exists {
			effectiveLotByRoot[intent.OrderOperationID] = intent.LotID
		}
	}
	for index := range intents {
		if intents[index].Kind != "disposal" || intents[index].CostBasisMethod != "specific_lot" {
			continue
		}
		for _, choice := range choices[intents[index].DecisionID] {
			rootID, ok := orderIDs[choice.SourceOperationID]
			if !ok {
				return nil, fmt.Errorf("replay specific-lot source operation %d has no correction root", choice.SourceOperationID)
			}
			allocation := choice.Allocation
			if effectiveLotID := effectiveLotByRoot[rootID]; effectiveLotID > 0 {
				allocation.LotID = effectiveLotID
			}
			intents[index].SpecificLots = append(intents[index].SpecificLots, allocation)
		}
		if len(intents[index].SpecificLots) == 0 {
			return nil, fmt.Errorf("replay specific-lot decision %d has no elected lots", intents[index].DecisionID)
		}
	}
	sortInvestmentReplayIntents(intents)
	return intents, nil
}

// investmentReplaySplitIntentsQuery reads the effective splits of a holding.
// Each carries the delta its effective effects moved in this cost currency;
// a replay that would move a different quantity is a named dependency.
func investmentReplaySplitIntentsQuery(ctx context.Context, reader queryer, bookID, accountID, commodityID, costCommodityID int64) ([]InvestmentReplayIntent, error) {
	rows, err := reader.QueryContext(ctx, `
		SELECT f.operation_id, o.operation_kind, f.effective_on, f.ratio_numerator, f.ratio_denominator,
			f.created_audit_event_id, e.transaction_id, e.created_by_user_id, e.created_at
		FROM investment_split_facts f
		JOIN investment_operations o ON o.id = f.operation_id
		JOIN investment_lot_events e ON e.id = (SELECT x.lot_event_id FROM investment_operation_lot_effects x
			WHERE x.operation_id = f.operation_id ORDER BY x.effect_seq LIMIT 1)
		WHERE f.book_id = ? AND f.account_id = ? AND f.commodity_id = ?
			AND o.correction_mode IS NOT 'reverse'
			AND NOT EXISTS (SELECT 1 FROM investment_operations successor
				WHERE successor.correction_of_operation_id = o.id)
		ORDER BY f.operation_id`, bookID, accountID, commodityID)
	if err != nil {
		return nil, fmt.Errorf("read replay splits: %w", err)
	}
	var intents []InvestmentReplayIntent
	for rows.Next() {
		intent := InvestmentReplayIntent{Kind: "split", EffectSeq: 1}
		var transactionID sql.NullInt64
		if err := rows.Scan(&intent.OperationID, &intent.OperationKind, &intent.EventDate,
			&intent.RatioNumerator, &intent.RatioDenominator, &intent.AuditEventID,
			&transactionID, &intent.CreatedByUserID, &intent.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan replay split: %w", err)
		}
		intent.TransactionID = transactionID.Int64
		intents = append(intents, intent)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate replay splits: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close replay splits: %w", err)
	}
	for index := range intents {
		effects, _, _, err := effectiveSplitEffectsQuery(ctx, reader, bookID, intents[index].OperationID, costCommodityID)
		if err != nil {
			return nil, err
		}
		recorded := sumSplitEffects(effects)
		if intents[index].QuantityValue, err = recorded.Coefficient(); err != nil {
			return nil, err
		}
		intents[index].QuantityScale = recorded.Scale()
	}
	return intents, nil
}

func sortInvestmentReplayIntents(intents []InvestmentReplayIntent) {
	slices.SortFunc(intents, func(a, b InvestmentReplayIntent) int {
		if order := cmp.Compare(a.EventDate, b.EventDate); order != 0 {
			return order
		}
		orderID := func(intent InvestmentReplayIntent) int64 {
			if intent.OrderOperationID > 0 {
				return intent.OrderOperationID
			}
			return intent.OperationID
		}
		if order := cmp.Compare(orderID(a), orderID(b)); order != 0 {
			return order
		}
		return cmp.Compare(a.EffectSeq, b.EffectSeq)
	})
}

func investmentReplayOrderOperationIDsQuery(ctx context.Context, reader queryer, bookID int64) (map[int64]int64, error) {
	rows, err := reader.QueryContext(ctx, `WITH RECURSIVE roots(operation_id, root_id) AS (
		SELECT id, id FROM investment_operations
		WHERE book_id = ? AND correction_of_operation_id IS NULL
		UNION ALL
		SELECT child.id, roots.root_id
		FROM investment_operations child
		JOIN roots ON roots.operation_id = child.correction_of_operation_id
		WHERE child.book_id = ?
	)
	SELECT operation_id, root_id FROM roots`, bookID, bookID)
	if err != nil {
		return nil, fmt.Errorf("read investment replay correction roots: %w", err)
	}
	defer rows.Close()
	orderIDs := make(map[int64]int64)
	for rows.Next() {
		var operationID, rootID int64
		if err := rows.Scan(&operationID, &rootID); err != nil {
			return nil, fmt.Errorf("scan investment replay correction root: %w", err)
		}
		orderIDs[operationID] = rootID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investment replay correction roots: %w", err)
	}
	return orderIDs, nil
}
