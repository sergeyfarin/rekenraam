package db

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strings"

	"rekenraam/backend/internal/exact"
)

// Split and reverse split (slice 5, T-122). Every long lot open on the
// effective date, at the operation's same-day replay slot, has its remaining
// quantity multiplied by numerator/denominator exactly. Remaining basis is
// unchanged, so aggregate basis is conserved and only per-share basis moves.
// The journal posts the aggregate signed quantity delta to the holding account
// and its opposite to commodity_trading in the security commodity; no cash is
// posted. Fractions the security's scale cannot represent are refused, never
// rounded: whole-share brokers settle them with a separate cash-in-lieu event.
//
// History that changes how many shares a split multiplied (a quantity
// correction, backdated acquisition or reversal before it) replays the split
// and appends a revision; when the revision's aggregate delta differs from its
// predecessor, the difference e posts as an adjustment journal dated to the
// split (H +e, T -e), linked to the split operation and the revision (T-129).

var (
	// ErrSplitNoEligibleHoldings means no lot is open on the effective date, so
	// the split would change nothing. Zero-delta splits need a journal-free
	// operation path that does not exist yet.
	ErrSplitNoEligibleHoldings = errors.New("no holdings are open on the split effective date")
	// ErrSplitFractionUnrepresentable means a lot's split quantity has more
	// decimal places than the security permits.
	ErrSplitFractionUnrepresentable = errors.New("split quantity is not representable at the security's quantity scale")
	// ErrSplitPositionChanged means the position changed between planning the
	// split journal and committing it; the planned quantity delta is stale.
	ErrSplitPositionChanged = errors.New("the position changed after the split was planned")
)

// CreateSplitParams are the sourced split terms. The ratio must already be
// positive, in lowest terms and different from 1:1.
type CreateSplitParams struct {
	BookID             int64
	AccountID          int64
	CommodityID        int64
	EffectiveOn        string
	RatioNumerator     int64
	RatioDenominator   int64
	SourceEvidenceJSON string
	// ExpectedDelta is the aggregate quantity delta the journal posts. The
	// writer recomputes it inside its transaction and refuses a mismatch.
	ExpectedDeltaValue exact.Coefficient
	ExpectedDeltaScale int
	// ReplacesOperationID is the split a replacement supersedes. Its plan
	// replays without that split, at its correction root's same-day slot.
	ReplacesOperationID int64
}

// SplitLotEffect is one lot's exact quantity change. New and delta share a
// scale, which is never narrower than the lot's previous remaining scale.
type SplitLotEffect struct {
	LotID           int64
	CostCommodityID int64
	BeforeValue     exact.Coefficient
	BeforeScale     int
	AfterValue      exact.Coefficient
	AfterScale      int
	DeltaValue      exact.Coefficient
	DeltaScale      int
}

// SplitPlan is the split's effect at its replay slot. Replayed reports that
// a later depletion exists, so dependent disposals are replayed too.
type SplitPlan struct {
	Effects    []SplitLotEffect
	DeltaValue exact.Coefficient
	DeltaScale int
	Replayed   bool
}

type splitPositionPlan struct {
	costCommodityID int64
	effects         []SplitLotEffect
	intents         []InvestmentReplayIntent
	projection      *InvestmentReplayProjection
}

type splitWritePlan struct {
	SplitPlan
	positions []splitPositionPlan
}

// splitLotQuantity multiplies an exact quantity by a positive rational,
// widening the scale only as far as needed and never past ceiling.
func splitLotQuantity(value exact.Coefficient, scale int, numerator, denominator int64, ceiling int) (exact.Coefficient, int, error) {
	if value.Sign() <= 0 || numerator <= 0 || denominator <= 0 || scale < 0 {
		return "", 0, fmt.Errorf("%w: split lot quantity and ratio must be positive", ErrInvalidDisposalParams)
	}
	product := new(big.Int).Mul(value.BigInt(), big.NewInt(numerator))
	divisor := big.NewInt(denominator)
	for next := scale; next <= ceiling; next++ {
		widened := new(big.Int).Mul(product, exact.Pow10(next-scale))
		quotient, remainder := new(big.Int).QuoRem(widened, divisor, new(big.Int))
		if remainder.Sign() != 0 {
			continue
		}
		result, err := exact.FromBig(quotient)
		if err != nil {
			return "", 0, err
		}
		return result, next, nil
	}
	return "", 0, ErrSplitFractionUnrepresentable
}

// splitQuantityCeilingTx is the deepest quantity scale the security permits
// on the effective date: its own limit, capped by its commodity kind.
func splitQuantityCeilingTx(ctx context.Context, tx *sql.Tx, bookID, commodityID int64, date string) (int, error) {
	var kind string
	var ceiling int
	err := tx.QueryRowContext(ctx, `
		SELECT c.kind, cv.max_quantity_scale FROM commodities c
		JOIN commodity_versions cv ON cv.commodity_id = c.id
		WHERE c.id = ? AND c.book_id = ? AND cv.id = (
			SELECT asof.id FROM commodity_versions asof
			WHERE asof.commodity_id = c.id AND asof.effective_from <= ?
			ORDER BY asof.effective_from DESC, asof.version_seq DESC LIMIT 1)`,
		commodityID, bookID, date).Scan(&kind, &ceiling)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: split commodity %d has no version on %s", ErrInvalidDisposalParams, commodityID, date)
	}
	if err != nil {
		return 0, fmt.Errorf("read split quantity scale: %w", err)
	}
	return min(ceiling, exact.MaxScaleForCommodityKind(kind)), nil
}

// splitEffectsForPositionTx reads the position's current projection and
// derives each eligible lot's split quantity without writing anything.
func splitEffectsForPositionTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64,
	date string, numerator, denominator int64) ([]SplitLotEffect, error) {
	ceiling, err := splitQuantityCeilingTx(ctx, tx, bookID, commodityID, date)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, remaining_quantity_value, remaining_quantity_scale, basis_knowledge
		FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ?
			AND position_side = 'long' AND status = 'open' AND opened_on <= ?
		ORDER BY id`, bookID, accountID, commodityID, costCommodityID, date)
	if err != nil {
		return nil, fmt.Errorf("read split eligible lots: %w", err)
	}
	defer rows.Close()
	var effects []SplitLotEffect
	for rows.Next() {
		effect := SplitLotEffect{CostCommodityID: costCommodityID}
		var knowledge string
		if err := rows.Scan(&effect.LotID, &effect.BeforeValue, &effect.BeforeScale, &knowledge); err != nil {
			return nil, fmt.Errorf("scan split eligible lot: %w", err)
		}
		if effect.BeforeValue.Sign() == 0 {
			continue
		}
		if knowledge != InvestmentBasisKnown {
			return nil, ErrUnknownInvestmentBasis
		}
		effect.AfterValue, effect.AfterScale, err = splitLotQuantity(effect.BeforeValue, effect.BeforeScale, numerator, denominator, ceiling)
		if err != nil {
			return nil, fmt.Errorf("lot %d: %w", effect.LotID, err)
		}
		delta := exact.ScaledIntFromCoefficient(effect.AfterValue, effect.AfterScale)
		delta.SubScaled(exact.ScaledIntFromCoefficient(effect.BeforeValue, effect.BeforeScale))
		delta.Align(effect.AfterScale)
		if effect.DeltaValue, err = delta.Coefficient(); err != nil {
			return nil, err
		}
		effect.DeltaScale = delta.Scale()
		effects = append(effects, effect)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate split eligible lots: %w", err)
	}
	return effects, nil
}

// applySplitEffectsTx installs the split quantities in the lot projection.
// Remaining basis is untouched: a split never moves basis between lots.
func applySplitEffectsTx(ctx context.Context, tx *sql.Tx, bookID int64, effects []SplitLotEffect,
	at string, actorUserID, auditEventID int64) error {
	for _, effect := range effects {
		result, err := tx.ExecContext(ctx, `UPDATE investment_lot_state
			SET remaining_quantity_value = ?, remaining_quantity_scale = ?,
				updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = NULLIF(?, 0)
			WHERE lot_id = ? AND book_id = ? AND status = 'open'`,
			effect.AfterValue, effect.AfterScale, at, actorUserID, auditEventID, effect.LotID, bookID)
		if err != nil {
			return fmt.Errorf("apply split to lot %d: %w", effect.LotID, err)
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return fmt.Errorf("%w: split lot %d is not open", ErrInvalidDisposalParams, effect.LotID)
		}
	}
	return nil
}

func sumSplitEffects(effects []SplitLotEffect) *exact.ScaledInt {
	total := exact.NewScaledInt()
	for _, effect := range effects {
		total.AddCoefficient(effect.DeltaValue, effect.DeltaScale)
	}
	return total
}

// planSplitTx computes the split at its slot for every cost-currency position
// of the holding. When a later depletion exists it replays the position with
// the split inserted; the caller persists that replay. operationID is zero for
// a plan that has no operation yet; it then sorts after every same-day event,
// exactly where the committed operation (the newest ID) will sort. A
// replacement always replays, without the split it replaces and at that
// split's correction-root slot, which is what replay sees once the inverse
// journal has made the replaced split ineffective.
func planSplitTx(ctx context.Context, tx *sql.Tx, params CreateSplitParams, operationID, auditEventID, actorUserID int64, at string) (splitWritePlan, error) {
	if params.BookID <= 0 || params.AccountID <= 0 || params.CommodityID <= 0 ||
		!isDisposalCalendarDate(params.EffectiveOn) || params.RatioNumerator <= 0 ||
		params.RatioDenominator <= 0 || params.RatioNumerator == params.RatioDenominator {
		return splitWritePlan{}, fmt.Errorf("%w: split terms are incomplete", ErrInvalidDisposalParams)
	}
	latest, err := latestPositionRewriteDateTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID)
	if err != nil {
		return splitWritePlan{}, err
	}
	replacing := params.ReplacesOperationID > 0
	plan := splitWritePlan{SplitPlan: SplitPlan{Replayed: replacing || latest != "" && params.EffectiveOn < latest}}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT cost_commodity_id FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side = 'long'
		ORDER BY cost_commodity_id`, params.BookID, params.AccountID, params.CommodityID)
	if err != nil {
		return splitWritePlan{}, fmt.Errorf("read split position currencies: %w", err)
	}
	var costIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return splitWritePlan{}, fmt.Errorf("scan split position currency: %w", err)
		}
		costIDs = append(costIDs, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return splitWritePlan{}, fmt.Errorf("read split position currencies: %w", err)
	}
	orderID := operationID
	if orderID <= 0 {
		orderID = math.MaxInt64
	}
	if replacing {
		roots, err := investmentReplayOrderOperationIDsQuery(ctx, tx, params.BookID)
		if err != nil {
			return splitWritePlan{}, err
		}
		if orderID = roots[params.ReplacesOperationID]; orderID <= 0 {
			return splitWritePlan{}, fmt.Errorf("%w: replaced split %d has no correction root", ErrInvalidDisposalParams, params.ReplacesOperationID)
		}
	}
	for _, costID := range costIDs {
		position := splitPositionPlan{costCommodityID: costID}
		if !plan.Replayed {
			position.effects, err = splitEffectsForPositionTx(ctx, tx, params.BookID, params.AccountID,
				params.CommodityID, costID, params.EffectiveOn, params.RatioNumerator, params.RatioDenominator)
			if err != nil {
				return splitWritePlan{}, err
			}
		} else {
			intents, err := investmentReplayIntentsQuery(ctx, tx, params.BookID, params.AccountID, params.CommodityID, costID, "long")
			if err != nil {
				return splitWritePlan{}, err
			}
			intents = slices.DeleteFunc(intents, func(intent InvestmentReplayIntent) bool {
				return replacing && intent.OperationID == params.ReplacesOperationID
			})
			intents = append(intents, InvestmentReplayIntent{
				OperationID: operationID, OrderOperationID: orderID, OperationKind: "split",
				EffectSeq: 1, EventDate: params.EffectiveOn, Kind: "split",
				RatioNumerator: params.RatioNumerator, RatioDenominator: params.RatioDenominator,
				SplitIsSubject: true, AuditEventID: auditEventID, CreatedByUserID: actorUserID, CreatedAt: at,
			})
			projection, err := simulateInvestmentReplayTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID, costID, intents)
			if err != nil {
				// The command's own split failing is its own refusal, not a
				// dependency of some other operation.
				var dependency *InvestmentReplayDependencyError
				if errors.As(err, &dependency) && dependency.OperationID == operationID && dependency.DecisionID == 0 &&
					!errors.Is(err, ErrInvestmentCorrectionDependency) {
					return splitWritePlan{}, dependency.Cause
				}
				return splitWritePlan{}, err
			}
			for _, split := range projection.Splits {
				if split.Subject {
					position.effects = split.Effects
				}
			}
			position.intents, position.projection = intents, &projection
		}
		plan.Effects = append(plan.Effects, position.effects...)
		plan.positions = append(plan.positions, position)
	}
	if len(plan.Effects) == 0 {
		return splitWritePlan{}, ErrSplitNoEligibleHoldings
	}
	total := sumSplitEffects(plan.Effects)
	if plan.DeltaValue, err = total.Coefficient(); err != nil {
		return splitWritePlan{}, err
	}
	plan.DeltaScale = total.Scale()
	return plan, nil
}

// PlanSplit computes the split's per-lot effects and journal quantity delta
// at its replay slot in a transaction that is always rolled back.
func (r *InvestmentRepository) PlanSplit(ctx context.Context, params CreateSplitParams, actorUserID int64) (SplitPlan, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return SplitPlan{}, fmt.Errorf("begin split plan: %w", err)
	}
	defer rollbackTx(ctx, tx)
	plan, err := planSplitTx(ctx, tx, params, 0, 0, actorUserID, "1970-01-01T00:00:00Z")
	if err != nil {
		return SplitPlan{}, err
	}
	return plan.SplitPlan, nil
}

// CreateSplit commits the split journal, sourced terms, per-lot effects, any
// dependent replay, checkpoint invalidation and audit together.
func (r *InvestmentRepository) CreateSplit(ctx context.Context, journal CreateTransactionParams, params CreateSplitParams) (TransactionRecord, SplitPlan, error) {
	return r.createSplit(ctx, journal, params, false)
}

// SimulateSplit runs the complete split writer, then rolls back. Only
// pre-existing checkpoint refs and the gain-impact change set escape.
func (r *InvestmentRepository) SimulateSplit(ctx context.Context, journal CreateTransactionParams, params CreateSplitParams) (SimulatedInvestmentWrite, error) {
	transaction, _, err := r.createSplit(ctx, journal, params, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func (r *InvestmentRepository) createSplit(ctx context.Context, journal CreateTransactionParams, params CreateSplitParams, preview bool) (TransactionRecord, SplitPlan, error) {
	write := executeInvestmentWriteTx[SplitPlan]
	if preview {
		write = previewInvestmentWriteTx[SplitPlan]
	}
	return write(ctx, r.database, journal, func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (SplitPlan, error) {
		return writeSplitEffectsTx(ctx, tx, params, transaction, auditEventID, journal.ActorUserID, journal.CreatedAt)
	}, nil)
}

// writeSplitEffectsTx records a split whose journal the enclosing writer just
// posted: it recomputes the plan at the slot, refuses a stale journal delta,
// then writes the sourced terms, per-lot effects and any dependent replay.
func writeSplitEffectsTx(ctx context.Context, tx *sql.Tx, params CreateSplitParams, transaction TransactionRecord,
	auditEventID, actorUserID int64, createdAt string) (SplitPlan, error) {
	operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
	if err != nil {
		return SplitPlan{}, err
	}
	plan, err := planSplitTx(ctx, tx, params, operationID, auditEventID, actorUserID, createdAt)
	if err != nil {
		return SplitPlan{}, err
	}
	// Recomputed at the slot inside this write: a position that moved since
	// the journal was planned must not commit a stale holding delta.
	if exact.ScaledIntFromCoefficient(plan.DeltaValue, plan.DeltaScale).Cmp(
		exact.ScaledIntFromCoefficient(params.ExpectedDeltaValue, params.ExpectedDeltaScale)) != 0 {
		return SplitPlan{}, ErrSplitPositionChanged
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_split_facts
		(operation_id, book_id, account_id, commodity_id, effective_on, ratio_numerator,
		 ratio_denominator, source_evidence_json, created_audit_event_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, operationID, params.BookID, params.AccountID,
		params.CommodityID, params.EffectiveOn, params.RatioNumerator, params.RatioDenominator,
		params.SourceEvidenceJSON, auditEventID); err != nil {
		return SplitPlan{}, fmt.Errorf("record split fact: %w", err)
	}
	for _, effect := range plan.Effects {
		result, err := tx.ExecContext(ctx, `INSERT INTO investment_lot_events (
			book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
			cost_basis_value, cost_basis_scale, metadata_json, created_at, created_by_user_id, created_audit_event_id
		) VALUES (?, ?, 'split_adjustment', ?, ?, ?, ?, '0', 0, ?, ?, ?, ?)`,
			params.BookID, effect.LotID, transaction.ID, params.EffectiveOn, effect.DeltaValue,
			effect.DeltaScale, fmt.Sprintf(`{"ratio_numerator":%d,"ratio_denominator":%d}`,
				params.RatioNumerator, params.RatioDenominator),
			createdAt, actorUserID, auditEventID)
		if err != nil {
			return SplitPlan{}, fmt.Errorf("record split lot event: %w", err)
		}
		eventID, err := result.LastInsertId()
		if err != nil {
			return SplitPlan{}, fmt.Errorf("read split lot event id: %w", err)
		}
		if err := linkLotEffectTx(ctx, tx, operationID, eventID); err != nil {
			return SplitPlan{}, err
		}
	}
	for _, position := range plan.positions {
		if position.projection == nil {
			if err := applySplitEffectsTx(ctx, tx, params.BookID, position.effects,
				createdAt, actorUserID, auditEventID); err != nil {
				return SplitPlan{}, err
			}
			continue
		}
		if err := persistInvestmentReplayProjectionTx(ctx, tx, params.BookID, params.AccountID,
			params.CommodityID, position.costCommodityID, operationID, auditEventID,
			actorUserID, createdAt, position.intents, *position.projection); err != nil {
			return SplitPlan{}, err
		}
	}
	return plan.SplitPlan, nil
}

// effectiveSplitEffectsQuery returns a split's current per-lot effects for
// one cost currency: its latest replay revision, or else its original events.
func effectiveSplitEffectsQuery(ctx context.Context, reader queryer, bookID, operationID, costCommodityID int64) ([]SplitLotEffect, int64, int, error) {
	var revisionID int64
	var revisionSeq int
	rows, err := reader.QueryContext(ctx, `SELECT id, revision_seq FROM latest_investment_split_revisions
		WHERE book_id = ? AND operation_id = ? AND cost_commodity_id = ?`, bookID, operationID, costCommodityID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read latest split revision: %w", err)
	}
	if rows.Next() {
		if err := rows.Scan(&revisionID, &revisionSeq); err != nil {
			rows.Close()
			return nil, 0, 0, fmt.Errorf("scan latest split revision: %w", err)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, 0, 0, fmt.Errorf("read latest split revision: %w", err)
	}
	query := `SELECT e.lot_id, e.quantity_value, e.quantity_scale
		FROM investment_operation_lot_effects x
		JOIN investment_lot_events e ON e.id = x.lot_event_id AND e.event_kind = 'split_adjustment'
		JOIN investment_lots l ON l.id = e.lot_id
		WHERE e.book_id = ? AND x.operation_id = ? AND l.cost_commodity_id = ?
		ORDER BY e.lot_id`
	args := []any{bookID, operationID, costCommodityID}
	if revisionID > 0 {
		query = `SELECT lot_id, quantity_delta_value, quantity_delta_scale
			FROM investment_split_revision_effects WHERE book_id = ? AND revision_id = ?
			ORDER BY lot_id`
		args = []any{bookID, revisionID}
	}
	effects, err := reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read effective split effects: %w", err)
	}
	defer effects.Close()
	var result []SplitLotEffect
	for effects.Next() {
		effect := SplitLotEffect{CostCommodityID: costCommodityID}
		if err := effects.Scan(&effect.LotID, &effect.DeltaValue, &effect.DeltaScale); err != nil {
			return nil, 0, 0, fmt.Errorf("scan effective split effect: %w", err)
		}
		result = append(result, effect)
	}
	if err := effects.Err(); err != nil {
		return nil, 0, 0, fmt.Errorf("iterate effective split effects: %w", err)
	}
	return result, revisionID, revisionSeq, nil
}

func canonicalSplitEffects(effects []SplitLotEffect) string {
	sorted := slices.Clone(effects)
	slices.SortFunc(sorted, func(a, b SplitLotEffect) int { return cmp.Compare(a.LotID, b.LotID) })
	var canonical strings.Builder
	for _, effect := range sorted {
		fmt.Fprintf(&canonical, "%d:%s;", effect.LotID,
			exact.ScaledIntFromCoefficient(effect.DeltaValue, effect.DeltaScale).Normalized().String())
	}
	return canonical.String()
}

// persistSplitRevisionTx appends an effective revision only when replay moved
// a split's per-lot effects; an unchanged replay adds no audit noise. A
// revision whose effects move a different aggregate quantity than the current
// ones posts the difference as an adjustment journal, so the split's linked
// journals always sum to its effective effects (T-129).
func persistSplitRevisionTx(ctx context.Context, tx *sql.Tx, bookID, costCommodityID, causedByOperationID, auditEventID, actorUserID int64,
	createdAt string, split InvestmentReplaySplit) error {
	current, priorID, priorSeq, err := effectiveSplitEffectsQuery(ctx, tx, bookID, split.OperationID, costCommodityID)
	if err != nil {
		return err
	}
	if canonicalSplitEffects(current) == canonicalSplitEffects(split.Effects) {
		return nil
	}
	if priorSeq == 0 {
		priorSeq = 1
	}
	delta := sumSplitEffects(split.Effects)
	delta.SubScaled(sumSplitEffects(current))
	var adjustmentVersionID int64
	if delta.Sign() != 0 {
		if adjustmentVersionID, err = postSplitAdjustmentJournalTx(ctx, tx, bookID, split.OperationID,
			delta.Normalized(), auditEventID, actorUserID, createdAt); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO investment_split_revisions (
		book_id, operation_id, cost_commodity_id, revision_seq, caused_by_operation_id,
		supersedes_revision_id, created_at, created_audit_event_id, adjustment_transaction_version_id
	) VALUES (?, ?, ?, ?, ?, NULLIF(?, 0), ?, ?, NULLIF(?, 0))`, bookID, split.OperationID, costCommodityID,
		priorSeq+1, causedByOperationID, priorID, createdAt, auditEventID, adjustmentVersionID)
	if err != nil {
		return fmt.Errorf("append split revision for operation %d: %w", split.OperationID, err)
	}
	revisionID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read split revision id: %w", err)
	}
	for index, effect := range split.Effects {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_split_revision_effects (
			book_id, revision_id, effect_seq, lot_id, quantity_delta_value, quantity_delta_scale
		) VALUES (?, ?, ?, ?, ?, ?)`, bookID, revisionID, index+1, effect.LotID,
			effect.DeltaValue, effect.DeltaScale); err != nil {
			return fmt.Errorf("append split revision effect: %w", err)
		}
	}
	return nil
}

// postSplitAdjustmentJournalTx posts a split's revised quantity delta e as a
// journal dated to the split: H +e to the holding and T -e to
// commodity_trading, in the security, under the enclosing command's audit
// event. The journal is linked to the split operation as 'split_adjustment';
// the investment writer applies the reconciliation guard to it after the
// domain effects (splitAdjustmentCheckpointCandidatesTx).
func postSplitAdjustmentJournalTx(ctx context.Context, tx *sql.Tx, bookID, splitOperationID int64, delta *exact.ScaledInt,
	auditEventID, actorUserID int64, createdAt string) (int64, error) {
	var accountID, commodityID, tradingID int64
	var effectiveOn string
	if err := tx.QueryRowContext(ctx, `SELECT f.account_id, f.commodity_id, f.effective_on, trading.id
		FROM investment_split_facts f
		JOIN accounts trading ON trading.book_id = f.book_id AND trading.system_role = 'commodity_trading'
		WHERE f.operation_id = ? AND f.book_id = ?`, splitOperationID, bookID).Scan(
		&accountID, &commodityID, &effectiveOn, &tradingID); err != nil {
		return 0, fmt.Errorf("read split %d adjustment accounts: %w", splitOperationID, err)
	}
	value, err := delta.Coefficient()
	if err != nil {
		return 0, err
	}
	posting := func(key string, account int64, quantity exact.Coefficient) PostingSpec {
		return PostingSpec{LineKey: key, AccountID: account, QuantityValue: quantity, QuantityScale: delta.Scale(),
			CommodityID: commodityID, ReconciliationStatus: "uncleared", MetadataJSON: "{}"}
	}
	record, err := insertTransactionWithAuditEventTx(ctx, tx, CreateTransactionParams{
		BookID: bookID, ActorUserID: actorUserID, CreatedAt: createdAt,
		Spec: TransactionSpec{
			Status: "posted", TransactionKind: "investment", TransactionDate: effectiveOn,
			MetadataJSON: fmt.Sprintf(`{"split_adjustment_of_operation_id":%d}`, splitOperationID),
			JournalEntries: []JournalEntrySpec{{EntryDate: effectiveOn, EntryKind: "investment", MetadataJSON: "{}",
				Postings: []PostingSpec{
					posting("split-adjustment-holding", accountID, value),
					posting("split-adjustment-trading", tradingID, value.Negated()),
				}}},
		},
	}, auditEventID)
	if err != nil {
		return 0, fmt.Errorf("post split %d adjustment journal: %w", splitOperationID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
		(book_id, operation_id, transaction_version_id, link_seq, role)
		SELECT ?, ?, ?, COALESCE(MAX(link_seq), 0) + 1, 'split_adjustment'
		FROM investment_operation_journal_links WHERE operation_id = ?`,
		bookID, splitOperationID, record.VersionID, splitOperationID); err != nil {
		return 0, fmt.Errorf("link split %d adjustment journal: %w", splitOperationID, err)
	}
	return record.VersionID, nil
}
