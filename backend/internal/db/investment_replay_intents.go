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
	Kind             string // opening, disposal, transfer_out, pooled_transfer_out, pooled_lot_transfer_out or split
	LotID            int64  // opening or transfer_out
	LinkSeq          int    // transfer_out: the link whose carried basis it produces
	// RecordedLotID is the source lot a transfer_out's current effective
	// depletion names; LotID is the lot replay depletes now, which follows the
	// acquisition's correction root (T-132).
	RecordedLotID int64
	// transferSource is the link's original source lot opening, which the
	// correction-root lineage is resolved from.
	transferSource transferSourceOpening
	DecisionID     int64 // disposal only
	// QuantityValue is the opening, disposal or transfer quantity. For a split
	// it is the signed holding delta the split's effective effects moved in
	// this cost currency; replay may move a different one, which persisting
	// posts as an adjustment journal (T-129).
	QuantityValue    exact.Coefficient
	QuantityScale    int
	RatioNumerator   int64 // split only
	RatioDenominator int64 // split only
	// SplitIsSubject marks the split the current command is creating. It has
	// no recorded effects yet, so replay reports its effects instead of
	// checking them.
	SplitIsSubject bool
	// TransferIsSubject marks the internal transfer a replacement command is
	// recording (T-119). It has no committed links yet, so replay reports its
	// source depletions instead of comparing them.
	TransferIsSubject bool
	AmountValue       exact.Coefficient // opening consideration or disposal proceeds
	AmountScale       int
	CostBasisMethod   string
	DecisionSource    DisposalDecisionSource
	SpecificLots      []LotAllocation
	// PooledLinks are a pooled transfer's committed per-lot carried amounts,
	// in link order. Replay must reproduce them exactly.
	PooledLinks []InvestmentReplayTransferLink
	// PooledDepletions are a pooled_lot transfer's effective source
	// depletions; AmountValue is its effective carried basis and the original
	// date fields its effective date. Replay may revise all three (T-135).
	PooledDepletions      []InvestmentReplayTransferLink
	OriginalDateKnowledge string
	OriginalAcquiredOn    string
	TransactionID         int64
	AuditEventID          int64
	CreatedByUserID       int64
	CreatedAt             string
}

// InvestmentReplayTransferLink is one source lot's committed depletion in a
// pooled internal transfer.
type InvestmentReplayTransferLink struct {
	LinkSeq        int
	LotID          int64
	RecordedLotID  int64
	transferSource transferSourceOpening
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
		SELECT l.id, l.operation_id, o.operation_kind, l.opened_on,
			l.quantity_value, l.quantity_scale,
			COALESCE(revision.carried_basis_value, l.cost_basis_value),
			COALESCE(revision.carried_basis_scale, l.cost_basis_scale),
			(SELECT x.effect_seq FROM investment_operation_lot_effects x
			 JOIN investment_lot_events e ON e.id = x.lot_event_id
			 WHERE x.operation_id = l.operation_id AND e.lot_id = l.id
			   AND e.event_kind IN ('acquisition', 'reinvested_dividend', 'transfer_in')
			 ORDER BY x.effect_seq LIMIT 1)
		FROM investment_lots l JOIN effective_investment_operations o ON o.id = l.operation_id
		-- An internal-transfer destination opens at its link's effective
		-- carried basis; the lot row keeps the first committed amount (T-132).
		LEFT JOIN effective_investment_transfer_links revision ON revision.destination_lot_id = l.id
		WHERE l.book_id = ? AND l.account_id = ? AND l.commodity_id = ?
			AND l.cost_commodity_id = ? AND l.position_side = ?
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
		FROM investment_disposal_decisions d JOIN effective_investment_operations o ON o.id = d.operation_id
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
	transfers, err := reader.QueryContext(ctx, `
		SELECT f.operation_id, o.operation_kind, f.effective_on, x.link_seq, x.source_lot_id,
			COALESCE(revision.source_lot_id, x.source_lot_id), src.operation_id, src.opened_on,
			x.quantity_value, x.quantity_scale,
			COALESCE(revision.carried_basis_value, x.carried_basis_value),
			COALESCE(revision.carried_basis_scale, x.carried_basis_scale),
			e.transaction_id, e.created_audit_event_id, e.created_by_user_id, e.created_at,
			effect.effect_seq, f.basis_allocation
		FROM investment_transfer_facts f
		JOIN investment_transfer_lot_links x ON x.operation_id = f.operation_id
		JOIN effective_investment_operations o ON o.id = f.operation_id
		JOIN investment_lots src ON src.id = x.source_lot_id
		JOIN investment_operation_lot_effects effect ON effect.operation_id = f.operation_id
		JOIN investment_lot_events e ON e.id = effect.lot_event_id
			AND e.lot_id = x.source_lot_id AND e.event_kind = 'transfer_out'
		LEFT JOIN latest_investment_transfer_link_revisions revision
			ON revision.operation_id = x.operation_id AND revision.link_seq = x.link_seq
		WHERE f.book_id = ? AND f.source_account_id = ? AND f.commodity_id = ?
			AND f.transfer_kind = 'internal' AND f.destination_lineage = 'source_lots'
			AND x.cost_commodity_id = ?
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
		var sourceOperationID sql.NullInt64
		if err := transfers.Scan(&intent.OperationID, &intent.OperationKind, &intent.EventDate,
			&intent.LinkSeq, &intent.transferSource.lotID, &intent.RecordedLotID, &sourceOperationID,
			&intent.transferSource.openedOn, &intent.QuantityValue, &intent.QuantityScale, &basis, &basisScale,
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
		intent.transferSource.operationID = sourceOperationID.Int64
		intent.LotID = intent.transferSource.lotID
		if allocation.String != InternalTransferAverageCostPool {
			intents = append(intents, intent)
			continue
		}
		link := InvestmentReplayTransferLink{LinkSeq: intent.LinkSeq, LotID: intent.LotID,
			RecordedLotID: intent.RecordedLotID, transferSource: intent.transferSource, QuantityValue: intent.QuantityValue,
			QuantityScale: intent.QuantityScale, CostBasisValue: intent.AmountValue, CostBasisScale: intent.AmountScale}
		index, exists := pooled[intent.OperationID]
		if !exists {
			intent.Kind = "pooled_transfer_out"
			intent.LotID, intent.LinkSeq, intent.RecordedLotID = 0, 0, 0
			intent.transferSource = transferSourceOpening{}
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

	pooledLots, err := investmentReplayPooledLotIntentsQuery(ctx, reader, bookID, accountID, commodityID, costCommodityID)
	if err != nil {
		return nil, err
	}
	intents = append(intents, pooledLots...)

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
		JOIN effective_investment_operations o ON o.id = d.operation_id
		JOIN investment_lots source ON source.id = a.lot_id AND source.operation_id IS NOT NULL
		WHERE d.book_id = ? AND d.account_id = ? AND d.commodity_id = ?
			AND d.cost_commodity_id = ? AND d.position_side = ? AND d.cost_basis_method = 'specific_lot'
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
	// A transfer depleted a specific acquisition. When that acquisition was
	// replaced, replay depletes its effective successor lot instead, exactly
	// as a specific-lot election does. The link's original acquisition date
	// orders the destination's FIFO/LIFO, so a successor opened on another
	// date is not followed: the transfer then fails as a named dependency.
	openedOn := make(map[int64]string)
	for _, intent := range intents {
		if intent.Kind == "opening" {
			openedOn[intent.LotID] = intent.EventDate
		}
	}
	effectiveSource := func(source transferSourceOpening) (int64, error) {
		if source.operationID <= 0 {
			return source.lotID, nil
		}
		rootID, ok := orderIDs[source.operationID]
		if !ok {
			return 0, fmt.Errorf("replay transfer source operation %d has no correction root", source.operationID)
		}
		if lotID := effectiveLotByRoot[rootID]; lotID > 0 && openedOn[lotID] == source.openedOn {
			return lotID, nil
		}
		return source.lotID, nil
	}
	for index := range intents {
		var err error
		switch intents[index].Kind {
		case "transfer_out":
			intents[index].LotID, err = effectiveSource(intents[index].transferSource)
		case "pooled_transfer_out":
			for link := range intents[index].PooledLinks {
				if err == nil {
					intents[index].PooledLinks[link].LotID, err = effectiveSource(intents[index].PooledLinks[link].transferSource)
				}
			}
		}
		if err != nil {
			return nil, err
		}
	}
	sortInvestmentReplayIntents(intents)
	return intents, nil
}

// transferSourceOpening is the original source lot of an internal transfer
// link and the operation and date that opened it.
type transferSourceOpening struct {
	lotID       int64
	operationID int64
	openedOn    string
}

// investmentReplayPooledLotIntentsQuery reads the pooled_lot transfers out of
// a holding (T-135). Each is one intent for the link's fixed quantity,
// carrying its effective basis, original date and source depletions: the
// latest revision's, or else the operation's committed transfer_out events.
func investmentReplayPooledLotIntentsQuery(ctx context.Context, reader queryer, bookID, accountID, commodityID, costCommodityID int64) ([]InvestmentReplayIntent, error) {
	rows, err := reader.QueryContext(ctx, `
		SELECT f.operation_id, o.operation_kind, f.effective_on, x.link_seq,
			x.quantity_value, x.quantity_scale, x.carried_basis_value, x.carried_basis_scale,
			x.original_date_knowledge, COALESCE(x.original_acquired_on, ''), COALESCE(x.revision_id, 0)
		FROM investment_transfer_facts f
		JOIN effective_investment_operations o ON o.id = f.operation_id
		JOIN effective_investment_transfer_links x ON x.operation_id = f.operation_id
		WHERE f.book_id = ? AND f.source_account_id = ? AND f.commodity_id = ?
			AND f.transfer_kind = 'internal' AND f.destination_lineage = 'pooled_lot'
			AND x.cost_commodity_id = ?
		ORDER BY f.operation_id`, bookID, accountID, commodityID, costCommodityID)
	if err != nil {
		return nil, fmt.Errorf("read replay pooled-lot transfers: %w", err)
	}
	var intents []InvestmentReplayIntent
	var revisions []int64
	for rows.Next() {
		intent := InvestmentReplayIntent{Kind: "pooled_lot_transfer_out"}
		var basis sql.NullString
		var basisScale sql.NullInt64
		var revisionID int64
		if err := rows.Scan(&intent.OperationID, &intent.OperationKind, &intent.EventDate, &intent.LinkSeq,
			&intent.QuantityValue, &intent.QuantityScale, &basis, &basisScale,
			&intent.OriginalDateKnowledge, &intent.OriginalAcquiredOn, &revisionID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan replay pooled-lot transfer: %w", err)
		}
		if !basis.Valid || !basisScale.Valid {
			rows.Close()
			return nil, fmt.Errorf("%w: transfer operation %d lacks known basis", ErrInvalidDisposalParams, intent.OperationID)
		}
		intent.AmountValue, intent.AmountScale = exact.Coefficient(basis.String), int(basisScale.Int64)
		intents = append(intents, intent)
		revisions = append(revisions, revisionID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate replay pooled-lot transfers: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close replay pooled-lot transfers: %w", err)
	}
	for index := range intents {
		intent := &intents[index]
		// The committed source events give the intent its slot and provenance
		// even after a revision replaced their amounts.
		events, err := reader.QueryContext(ctx, `
			SELECT effect.effect_seq, e.lot_id, e.quantity_value, e.quantity_scale,
				e.cost_basis_value, e.cost_basis_scale, e.transaction_id, e.created_audit_event_id,
				e.created_by_user_id, e.created_at
			FROM investment_operation_lot_effects effect
			JOIN investment_lot_events e ON e.id = effect.lot_event_id AND e.event_kind = 'transfer_out'
			WHERE effect.operation_id = ?
			ORDER BY effect.effect_seq`, intent.OperationID)
		if err != nil {
			return nil, fmt.Errorf("read replay pooled-lot depletions: %w", err)
		}
		for events.Next() {
			var depletion InvestmentReplayTransferLink
			var seq int
			var basis sql.NullString
			var basisScale sql.NullInt64
			var transactionID, auditEventID, userID int64
			var createdAt string
			if err := events.Scan(&seq, &depletion.LotID, &depletion.QuantityValue, &depletion.QuantityScale,
				&basis, &basisScale, &transactionID, &auditEventID, &userID, &createdAt); err != nil {
				events.Close()
				return nil, fmt.Errorf("scan replay pooled-lot depletion: %w", err)
			}
			if !basis.Valid || !basisScale.Valid {
				events.Close()
				return nil, fmt.Errorf("%w: transfer operation %d depletion lacks known basis", ErrInvalidDisposalParams, intent.OperationID)
			}
			depletion.QuantityValue = depletion.QuantityValue.Negated()
			depletion.CostBasisValue, depletion.CostBasisScale = exact.Coefficient(basis.String).Negated(), int(basisScale.Int64)
			if intent.EffectSeq == 0 {
				intent.EffectSeq, intent.TransactionID, intent.AuditEventID = seq, transactionID, auditEventID
				intent.CreatedByUserID, intent.CreatedAt = userID, createdAt
			}
			intent.PooledDepletions = append(intent.PooledDepletions, depletion)
		}
		if err := events.Err(); err != nil {
			events.Close()
			return nil, fmt.Errorf("iterate replay pooled-lot depletions: %w", err)
		}
		if err := events.Close(); err != nil {
			return nil, fmt.Errorf("close replay pooled-lot depletions: %w", err)
		}
		if intent.EffectSeq <= 0 {
			return nil, fmt.Errorf("replay pooled-lot transfer %d has no immutable source effect", intent.OperationID)
		}
		if revisions[index] > 0 {
			if intent.PooledDepletions, err = transferRevisionDepletionsQuery(ctx, reader, revisions[index]); err != nil {
				return nil, err
			}
		}
	}
	return intents, nil
}

// transferRevisionDepletionsQuery reads a pooled_lot revision's source
// depletions in order.
func transferRevisionDepletionsQuery(ctx context.Context, reader queryer, revisionID int64) ([]InvestmentReplayTransferLink, error) {
	rows, err := reader.QueryContext(ctx, `SELECT source_lot_id, quantity_value, quantity_scale,
		cost_basis_value, cost_basis_scale FROM investment_transfer_link_revision_depletions
		WHERE revision_id = ? ORDER BY depletion_seq`, revisionID)
	if err != nil {
		return nil, fmt.Errorf("read transfer revision depletions: %w", err)
	}
	var depletions []InvestmentReplayTransferLink
	for rows.Next() {
		var depletion InvestmentReplayTransferLink
		if err := rows.Scan(&depletion.LotID, &depletion.QuantityValue, &depletion.QuantityScale,
			&depletion.CostBasisValue, &depletion.CostBasisScale); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan transfer revision depletion: %w", err)
		}
		depletions = append(depletions, depletion)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate transfer revision depletions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close transfer revision depletions: %w", err)
	}
	if len(depletions) == 0 {
		return nil, fmt.Errorf("transfer link revision %d has no depletions", revisionID)
	}
	return depletions, nil
}

// investmentReplaySplitIntentsQuery reads the effective splits of a holding.
// Each carries the delta its effective effects moved in this cost currency.
func investmentReplaySplitIntentsQuery(ctx context.Context, reader queryer, bookID, accountID, commodityID, costCommodityID int64) ([]InvestmentReplayIntent, error) {
	rows, err := reader.QueryContext(ctx, `
		SELECT f.operation_id, o.operation_kind, f.effective_on, f.ratio_numerator, f.ratio_denominator,
			f.created_audit_event_id, e.transaction_id, e.created_by_user_id, e.created_at
		FROM investment_split_facts f
		JOIN effective_investment_operations o ON o.id = f.operation_id
		JOIN investment_lot_events e ON e.id = (SELECT x.lot_event_id FROM investment_operation_lot_effects x
			WHERE x.operation_id = f.operation_id ORDER BY x.effect_seq LIMIT 1)
		WHERE f.book_id = ? AND f.account_id = ? AND f.commodity_id = ?
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
	slices.SortFunc(intents, compareInvestmentReplayIntents)
}

// compareInvestmentReplayIntents is replay's causal order: date, then the
// correction root's same-day slot, then the operation's effect order. It
// orders intents of different positions too, so a transfer's source
// depletion precedes the destination lot it opens.
func compareInvestmentReplayIntents(a, b InvestmentReplayIntent) int {
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
