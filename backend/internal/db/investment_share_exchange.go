package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"

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
	// ReplacesOperationID is the exchange a replacement corrects (#179): the
	// plan replays without it, at its correction-root slot.
	ReplacesOperationID int64
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

// shareExchangeSubject is what a replayed plan records the exchange as: the
// command's operation, journal and audit event. It is zero in a plan that has
// no operation yet, which then sorts after every same-day event, exactly where
// the committed operation (the newest ID) will sort.
type shareExchangeSubject struct {
	OperationID   int64
	TransactionID int64
	AuditEventID  int64
	ActorUserID   int64
	CreatedAt     string
}

// shareExchangeWritePlan is the plan plus how the writer must record it.
type shareExchangeWritePlan struct {
	ShareExchangePlan
	// Replayed means the source depletions are the ones replay produced at
	// the exchange's slot (#179); Depletions holds them in link order.
	Replayed   bool
	Depletions []LotDisposalRecord
	// DestinationBackdated means the new lots open behind a later event of the
	// destination holding, so they open by replay admission and it replays.
	DestinationBackdated bool
}

// planShareExchangeTx derives every link without writing. An exchange in date
// order reads today's holding. One dated behind a later depletion of the old
// instrument, or a replacement, replays the holding with the exchange as its
// subject at the slot, so it takes the whole position open then (#179).
func planShareExchangeTx(ctx context.Context, tx *sql.Tx, params CreateShareExchangeParams, subject shareExchangeSubject) (shareExchangeWritePlan, error) {
	if !validShareExchangeParams(params) {
		return shareExchangeWritePlan{}, fmt.Errorf("%w: share exchange terms are incomplete", ErrInvalidDisposalParams)
	}
	replacing := params.ReplacesOperationID > 0
	sourceBackdated, err := positionRewrittenAfterTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID, params.EffectiveOn)
	if err != nil {
		return shareExchangeWritePlan{}, err
	}
	destinationBackdated, err := positionRewrittenAfterTx(ctx, tx, params.BookID, params.DestinationAccountID,
		params.DestinationCommodityID, params.EffectiveOn)
	if err != nil {
		return shareExchangeWritePlan{}, err
	}
	var shortLots int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side = 'short' AND status = 'open'`,
		params.BookID, params.AccountID, params.CommodityID).Scan(&shortLots); err != nil {
		return shareExchangeWritePlan{}, fmt.Errorf("read share exchange short lots: %w", err)
	}
	if shortLots > 0 {
		return shareExchangeWritePlan{}, ErrShareExchangeShortPosition
	}
	ceiling, err := splitQuantityCeilingTx(ctx, tx, params.BookID, params.DestinationCommodityID, params.EffectiveOn)
	if err != nil {
		return shareExchangeWritePlan{}, err
	}
	plan := shareExchangeWritePlan{Replayed: replacing || sourceBackdated, DestinationBackdated: replacing || destinationBackdated}
	type sourceLot struct {
		link     ShareExchangeLink
		openedOn string
	}
	var sources []sourceLot
	if plan.Replayed {
		if plan.Depletions, err = shareExchangeSubjectDepletionsTx(ctx, tx, params, subject); err != nil {
			return shareExchangeWritePlan{}, err
		}
		for _, depletion := range plan.Depletions {
			lot, err := investmentLotByIDTx(ctx, tx, params.BookID, depletion.LotID)
			if err != nil {
				return shareExchangeWritePlan{}, err
			}
			link := ShareExchangeLink{SourceLotID: depletion.LotID, CostCommodityID: lot.CostCommodityID,
				SourceQuantityValue: depletion.QuantityValue, SourceQuantityScale: depletion.QuantityScale,
				BasisKnowledge: normalizedBasisKnowledge(depletion.BasisKnowledge)}
			if link.BasisKnowledge == InvestmentBasisKnown {
				link.CarriedBasisValue, link.CarriedBasisScale = depletion.CostBasisValue, depletion.CostBasisScale
			}
			sources = append(sources, sourceLot{link: link, openedOn: lot.OpenedOn})
		}
	} else {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, opened_on, cost_commodity_id, remaining_quantity_value, remaining_quantity_scale,
				remaining_cost_basis_value, remaining_cost_basis_scale, basis_knowledge
			FROM current_investment_lots
			WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side = 'long'
				AND status = 'open' AND opened_on <= ?
			ORDER BY cost_commodity_id, opened_on, id`, params.BookID, params.AccountID, params.CommodityID, params.EffectiveOn)
		if err != nil {
			return shareExchangeWritePlan{}, fmt.Errorf("read share exchange lots: %w", err)
		}
		for rows.Next() {
			var source sourceLot
			var basis sql.NullInt64
			var basisScale sql.NullInt64
			link := &source.link
			if err := rows.Scan(&link.SourceLotID, &source.openedOn, &link.CostCommodityID,
				&link.SourceQuantityValue, &link.SourceQuantityScale, &basis, &basisScale, &link.BasisKnowledge); err != nil {
				rows.Close()
				return shareExchangeWritePlan{}, fmt.Errorf("scan share exchange lot: %w", err)
			}
			switch {
			case link.BasisKnowledge == InvestmentBasisKnown && basis.Valid && basisScale.Valid:
				link.CarriedBasisValue, link.CarriedBasisScale = basis.Int64, int(basisScale.Int64)
			case link.BasisKnowledge == InvestmentBasisUnknown && !basis.Valid:
			default:
				rows.Close()
				return shareExchangeWritePlan{}, fmt.Errorf("%w: lot %d has an invalid basis knowledge/amount pair", ErrInvalidDisposalParams, link.SourceLotID)
			}
			if link.SourceQuantityValue.Sign() > 0 {
				sources = append(sources, source)
			}
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return shareExchangeWritePlan{}, fmt.Errorf("read share exchange lots: %w", err)
		}
	}
	if len(sources) == 0 {
		return shareExchangeWritePlan{}, ErrShareExchangeNoHoldings
	}
	sourceTotal, destinationTotal := exact.NewScaledInt(), exact.NewScaledInt()
	for _, source := range sources {
		link := source.link
		link.DestinationQuantityValue, link.DestinationQuantityScale, err = splitLotQuantity(link.SourceQuantityValue,
			link.SourceQuantityScale, params.RatioNumerator, params.RatioDenominator, ceiling)
		if err != nil {
			return shareExchangeWritePlan{}, fmt.Errorf("lot %d: %w", link.SourceLotID, err)
		}
		if link.OriginalDateKnowledge, link.OriginalAcquiredOn, err = internalTransferOriginalDateTx(ctx, tx,
			link.SourceLotID, source.openedOn); err != nil {
			return shareExchangeWritePlan{}, err
		}
		sourceTotal.AddCoefficient(link.SourceQuantityValue, link.SourceQuantityScale)
		destinationTotal.AddCoefficient(link.DestinationQuantityValue, link.DestinationQuantityScale)
		plan.Links = append(plan.Links, link)
	}
	if plan.SourceQuantityValue, err = sourceTotal.Coefficient(); err != nil {
		return shareExchangeWritePlan{}, err
	}
	if plan.DestinationQuantityValue, err = destinationTotal.Coefficient(); err != nil {
		return shareExchangeWritePlan{}, err
	}
	plan.SourceQuantityScale, plan.DestinationQuantityScale = sourceTotal.Scale(), destinationTotal.Scale()
	return plan, nil
}

// shareExchangeSubjectDepletionsTx replays each cost-currency position of the
// old holding with the exchange as the subject at its slot: the replaced
// exchange's correction-root slot for a replacement, else the exchange's own.
// The depletions are what the exchange takes at that slot; a later decision
// the emptied holding cannot satisfy is a named dependency.
func shareExchangeSubjectDepletionsTx(ctx context.Context, tx *sql.Tx, params CreateShareExchangeParams,
	subject shareExchangeSubject) ([]LotDisposalRecord, error) {
	costIDs, err := holdingCostCommodityIDsTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID)
	if err != nil {
		return nil, err
	}
	orderID := subject.OperationID
	if orderID <= 0 {
		orderID = math.MaxInt64
	}
	if params.ReplacesOperationID > 0 {
		roots, err := investmentReplayOrderOperationIDsQuery(ctx, tx, params.BookID)
		if err != nil {
			return nil, err
		}
		if orderID = roots[params.ReplacesOperationID]; orderID <= 0 {
			return nil, fmt.Errorf("%w: replaced exchange %d has no correction root", ErrInvalidDisposalParams, params.ReplacesOperationID)
		}
	}
	var depletions []LotDisposalRecord
	for _, costID := range costIDs {
		intents, err := investmentReplayIntentsQuery(ctx, tx, params.BookID, params.AccountID, params.CommodityID, costID, "long")
		if err != nil {
			return nil, err
		}
		// Before its inverse posts, the replaced exchange is still effective.
		intents = slices.DeleteFunc(intents, func(intent InvestmentReplayIntent) bool {
			return params.ReplacesOperationID > 0 && intent.OperationID == params.ReplacesOperationID
		})
		intents = append(intents, InvestmentReplayIntent{
			OperationID: subject.OperationID, OrderOperationID: orderID, OperationKind: "share_exchange",
			EffectSeq: 1, EventDate: params.EffectiveOn, Kind: "exchange_out", TransferIsSubject: true,
			TransactionID: subject.TransactionID, AuditEventID: subject.AuditEventID,
			CreatedByUserID: subject.ActorUserID, CreatedAt: subject.CreatedAt,
		})
		projection, err := simulateInvestmentReplayTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID, costID, intents)
		if err != nil {
			return nil, err
		}
		depletions = append(depletions, projection.SubjectTransferOut...)
	}
	return depletions, nil
}

// holdingCostCommodityIDsTx lists the cost currencies a holding has ever
// held one instrument in, long.
func holdingCostCommodityIDsTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT cost_commodity_id FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side = 'long'
		ORDER BY cost_commodity_id`, bookID, accountID, commodityID)
	if err != nil {
		return nil, fmt.Errorf("read holding cost currencies: %w", err)
	}
	var costIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan holding cost currency: %w", err)
		}
		costIDs = append(costIDs, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("read holding cost currencies: %w", err)
	}
	return costIDs, nil
}

// replayHoldingsTx replays every long cost-currency position of each holding
// (account, instrument) once, in order, from its committed effective intents.
func replayHoldingsTx(ctx context.Context, tx *sql.Tx, bookID int64, holdings [][2]int64,
	operationID, auditEventID, actorUserID int64, createdAt string) error {
	seen := make(map[[2]int64]bool, len(holdings))
	for _, holding := range holdings {
		if seen[holding] {
			continue
		}
		seen[holding] = true
		costIDs, err := holdingCostCommodityIDsTx(ctx, tx, bookID, holding[0], holding[1])
		if err != nil {
			return err
		}
		for _, costID := range costIDs {
			if err := replayCorrectedPositionTx(ctx, tx, bookID, investmentReplayPositionKey{holding[0], holding[1], costID},
				operationID, auditEventID, actorUserID, createdAt); err != nil {
				return err
			}
		}
	}
	return nil
}

// takeShareExchangeHoldingTx is the subject exchange's replay step: it takes
// every long lot of the position open at the slot, whole.
func takeShareExchangeHoldingTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, auditEventID int64, allocationScale int) ([]LotDisposalRecord, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, remaining_quantity_value, remaining_quantity_scale
		FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ?
			AND position_side = 'long' AND status = 'open' AND opened_on <= ?
		ORDER BY opened_on, id`, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID, params.EventDate)
	if err != nil {
		return nil, fmt.Errorf("read share exchange holding: %w", err)
	}
	var allocations []LotAllocation
	for rows.Next() {
		var allocation LotAllocation
		if err := rows.Scan(&allocation.LotID, &allocation.QuantityValue, &allocation.QuantityScale); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan share exchange holding: %w", err)
		}
		if allocation.QuantityValue.Sign() > 0 {
			allocations = append(allocations, allocation)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("read share exchange holding: %w", err)
	}
	moved := make([]LotDisposalRecord, 0, len(allocations))
	for _, allocation := range allocations {
		depletion, err := disposeLotTx(ctx, tx, params, allocation.LotID, allocation.QuantityValue,
			allocation.QuantityScale, auditEventID, allocationScale)
		if err != nil {
			return nil, err
		}
		moved = append(moved, depletion)
	}
	return moved, nil
}

// PlanShareExchange computes the exchange's links and journal totals in a
// transaction that is always rolled back.
// A replayed plan's simulated lot events name actorUserID and no audit event.
func (r *InvestmentRepository) PlanShareExchange(ctx context.Context, params CreateShareExchangeParams, actorUserID int64) (ShareExchangePlan, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ShareExchangePlan{}, fmt.Errorf("begin share exchange plan: %w", err)
	}
	defer rollbackTx(ctx, tx)
	plan, err := planShareExchangeTx(ctx, tx, params, shareExchangeSubject{ActorUserID: actorUserID, CreatedAt: "1970-01-01T00:00:00Z"})
	if err != nil {
		return ShareExchangePlan{}, err
	}
	return plan.ShareExchangePlan, nil
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
// moves every lot. A replayed plan records the depletions replay produced at
// the slot, opens the new lots by replay admission and replays both holdings,
// so every later decision is revised or named as a dependency (#179).
func writeShareExchangeTx(ctx context.Context, tx *sql.Tx, params CreateShareExchangeParams, transaction TransactionRecord,
	journal CreateTransactionParams, auditEventID int64) (ShareExchangePlan, error) {
	operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
	if err != nil {
		return ShareExchangePlan{}, err
	}
	plan, err := planShareExchangeTx(ctx, tx, params, shareExchangeSubject{OperationID: operationID,
		TransactionID: transaction.ID, AuditEventID: auditEventID, ActorUserID: journal.ActorUserID, CreatedAt: journal.CreatedAt})
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
		var depletion LotDisposalRecord
		if plan.Replayed {
			depletion = plan.Depletions[index]
			if depletion.EventID, err = insertShareExchangeDepletionTx(ctx, tx, params, transaction.ID, journal,
				auditEventID, depletion); err != nil {
				return ShareExchangePlan{}, err
			}
		} else {
			depletionParams := shareExchangeDepletionParams(params, link.CostCommodityID, transaction.ID, journal)
			scale, cached := allocationScales[link.CostCommodityID]
			if !cached {
				if scale, err = positionBasisAllocationScaleTx(ctx, tx, depletionParams); err != nil {
					return ShareExchangePlan{}, err
				}
				allocationScales[link.CostCommodityID] = scale
			}
			if depletion, err = disposeLotTx(ctx, tx, depletionParams, link.SourceLotID,
				link.SourceQuantityValue, link.SourceQuantityScale, auditEventID, scale); err != nil {
				return ShareExchangePlan{}, err
			}
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
		}, auditEventID, plan.DestinationBackdated)
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
	// The old holding first, so a dependency the emptied holding breaks is
	// named before one the new lots change.
	var replayed [][2]int64
	if plan.Replayed {
		replayed = append(replayed, [2]int64{params.AccountID, params.CommodityID})
	}
	if plan.DestinationBackdated {
		replayed = append(replayed, [2]int64{params.DestinationAccountID, params.DestinationCommodityID})
	}
	if err := replayHoldingsTx(ctx, tx, params.BookID, replayed, operationID, auditEventID,
		journal.ActorUserID, journal.CreatedAt); err != nil {
		return ShareExchangePlan{}, err
	}
	return plan.ShareExchangePlan, nil
}

// insertShareExchangeDepletionTx records a depletion replay produced at the
// exchange's slot as its transfer_out lot event. The projection is installed
// by the replay that follows, not here.
func insertShareExchangeDepletionTx(ctx context.Context, tx *sql.Tx, params CreateShareExchangeParams, transactionID int64,
	journal CreateTransactionParams, auditEventID int64, depletion LotDisposalRecord) (int64, error) {
	knowledge := normalizedBasisKnowledge(depletion.BasisKnowledge)
	result, err := tx.ExecContext(ctx, `INSERT INTO investment_lot_events (
		book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
		cost_basis_value, cost_basis_scale, metadata_json,
		created_at, created_by_user_id, created_audit_event_id, basis_knowledge
	) VALUES (?, ?, 'transfer_out', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		params.BookID, depletion.LotID, transactionID, params.EffectiveOn,
		depletion.QuantityValue.Negated(), depletion.QuantityScale,
		nullableBasisValue(-depletion.CostBasisValue, knowledge), nullableBasisScale(depletion.CostBasisScale, knowledge),
		params.SourceEvidenceJSON, journal.CreatedAt, journal.ActorUserID, auditEventID, knowledge)
	if err != nil {
		return 0, fmt.Errorf("record share exchange depletion: %w", err)
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read share exchange depletion id: %w", err)
	}
	return eventID, nil
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

// ShareExchangeTerms are a committed exchange's fact and its effective links
// (#178): replay may since have revised a link's carried basis.
type ShareExchangeTerms struct {
	AccountID              int64
	DestinationAccountID   int64
	CommodityID            int64
	DestinationCommodityID int64
	EffectiveOn            string
	RatioNumerator         int64
	RatioDenominator       int64
	SourceEvidenceJSON     string
	Plan                   ShareExchangePlan
}

// ShareExchangeTermsByOperation reads an exchange for the transaction detail.
// Each destination quantity is the destination lot's opening quantity, which
// is the link quantity times the ratio (validate-and-ship item 31).
func (r *InvestmentRepository) ShareExchangeTermsByOperation(ctx context.Context, bookID, operationID int64) (ShareExchangeTerms, error) {
	var terms ShareExchangeTerms
	err := r.database.QueryRowContext(ctx, `SELECT source_account_id, destination_account_id, commodity_id,
		destination_commodity_id, effective_on, ratio_numerator, ratio_denominator, source_evidence_json
		FROM investment_transfer_facts WHERE book_id = ? AND operation_id = ? AND transfer_kind = 'exchange'`,
		bookID, operationID).Scan(&terms.AccountID, &terms.DestinationAccountID, &terms.CommodityID,
		&terms.DestinationCommodityID, &terms.EffectiveOn, &terms.RatioNumerator, &terms.RatioDenominator,
		&terms.SourceEvidenceJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ShareExchangeTerms{}, ErrNotFound
	}
	if err != nil {
		return ShareExchangeTerms{}, fmt.Errorf("read share exchange terms: %w", err)
	}
	rows, err := r.database.QueryContext(ctx, `SELECT link.source_lot_id, link.destination_lot_id, link.cost_commodity_id,
		link.quantity_value, link.quantity_scale, lot.quantity_value, lot.quantity_scale,
		link.basis_knowledge, link.carried_basis_value, link.carried_basis_scale,
		link.original_date_knowledge, COALESCE(link.original_acquired_on, '')
		FROM effective_investment_transfer_links link
		JOIN investment_lots lot ON lot.id = link.destination_lot_id
		WHERE link.operation_id = ? ORDER BY link.link_seq`, operationID)
	if err != nil {
		return ShareExchangeTerms{}, fmt.Errorf("read share exchange links: %w", err)
	}
	defer rows.Close()
	sourceTotal, destinationTotal := exact.NewScaledInt(), exact.NewScaledInt()
	for rows.Next() {
		var link ShareExchangeLink
		var basis, basisScale sql.NullInt64
		if err := rows.Scan(&link.SourceLotID, &link.DestinationLotID, &link.CostCommodityID,
			&link.SourceQuantityValue, &link.SourceQuantityScale, &link.DestinationQuantityValue, &link.DestinationQuantityScale,
			&link.BasisKnowledge, &basis, &basisScale, &link.OriginalDateKnowledge, &link.OriginalAcquiredOn); err != nil {
			return ShareExchangeTerms{}, fmt.Errorf("scan share exchange link: %w", err)
		}
		link.BasisKnowledge = normalizedBasisKnowledge(link.BasisKnowledge)
		if link.BasisKnowledge == InvestmentBasisKnown {
			if !basis.Valid || !basisScale.Valid {
				return ShareExchangeTerms{}, fmt.Errorf("share exchange link of lot %d has a known basis without an amount", link.SourceLotID)
			}
			link.CarriedBasisValue, link.CarriedBasisScale = basis.Int64, int(basisScale.Int64)
		}
		sourceTotal.AddCoefficient(link.SourceQuantityValue, link.SourceQuantityScale)
		destinationTotal.AddCoefficient(link.DestinationQuantityValue, link.DestinationQuantityScale)
		terms.Plan.Links = append(terms.Plan.Links, link)
	}
	if err := rows.Err(); err != nil {
		return ShareExchangeTerms{}, fmt.Errorf("read share exchange links: %w", err)
	}
	if terms.Plan.SourceQuantityValue, err = sourceTotal.Coefficient(); err != nil {
		return ShareExchangeTerms{}, err
	}
	if terms.Plan.DestinationQuantityValue, err = destinationTotal.Coefficient(); err != nil {
		return ShareExchangeTerms{}, err
	}
	terms.Plan.SourceQuantityScale, terms.Plan.DestinationQuantityScale = sourceTotal.Scale(), destinationTotal.Scale()
	return terms, nil
}
