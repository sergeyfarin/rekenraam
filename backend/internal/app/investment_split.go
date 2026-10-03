package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

var (
	// ErrInvestmentSplitNoHoldings means nothing was held on the split date.
	ErrInvestmentSplitNoHoldings = db.ErrSplitNoEligibleHoldings
	// ErrInvestmentSplitFraction means a lot's split quantity needs more
	// decimal places than the security permits; a cash-in-lieu event would
	// settle the fraction, and the split is never rounded to fit.
	ErrInvestmentSplitFraction = db.ErrSplitFractionUnrepresentable
	// ErrInvestmentSplitChanged means the position moved between planning and
	// committing; preview again.
	ErrInvestmentSplitChanged = db.ErrSplitPositionChanged
	// ErrInvestmentSplitDependency is the sentinel behind a named dependency.
	ErrInvestmentSplitDependency = errors.New("investment split cannot satisfy a later operation")
)

// investmentSplitRatioMax bounds each side of a split ratio. Real corporate
// actions are far smaller; the bound keeps the exact arithmetic predictable.
const investmentSplitRatioMax = 1_000_000_000

// InvestmentSplitDependencyError names the later operation a backdated split
// would make impossible, such as a sale of more units than a reverse split
// leaves.
type InvestmentSplitDependencyError struct {
	OperationID int64
	DecisionID  int64
}

func (e InvestmentSplitDependencyError) Error() string {
	if e.DecisionID == 0 {
		return fmt.Sprintf("investment split cannot satisfy later operation %d", e.OperationID)
	}
	return fmt.Sprintf("investment split cannot satisfy later operation %d disposal decision %d", e.OperationID, e.DecisionID)
}

func (e InvestmentSplitDependencyError) Unwrap() error { return ErrInvestmentSplitDependency }

// InvestmentSplitInput records a split or reverse split of one holding.
// Numerator new units replace denominator old units: 3-for-2 is 3/2, a
// 1-for-10 reverse split is 1/10.
type InvestmentSplitInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	OriginType                string
	Operation                 string
	EffectiveOn               string
	HoldingAccountID          int64
	CommodityID               int64
	RatioNumerator            int64
	RatioDenominator          int64
	SourceEvidenceJSON        string
	Memo                      string
	ChangeReason              string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
}

// InvestmentSplitLotEffect is one lot's exact quantity before and after.
type InvestmentSplitLotEffect struct {
	LotID           int64
	CostCommodityID int64
	BeforeValue     exact.Coefficient
	BeforeScale     int
	AfterValue      exact.Coefficient
	AfterScale      int
}

// InvestmentSplitPlan is the split's effect at its slot: per-lot changes, the
// aggregate holding delta the journal posts, and whether later disposals
// replay because the split is dated before them.
type InvestmentSplitPlan struct {
	RatioNumerator   int64
	RatioDenominator int64
	Effects          []InvestmentSplitLotEffect
	DeltaValue       exact.Coefficient
	DeltaScale       int
	Replayed         bool
}

// InvestmentSplitPreview is the plan plus the reconciliation and gain impact
// the writer would produce, computed by running it and rolling back.
type InvestmentSplitPreview struct {
	Plan   InvestmentSplitPlan
	Impact ReconciliationImpact
}

type InvestmentSplitResult struct {
	Transaction Transaction
	Plan        InvestmentSplitPlan
}

func (s *InvestmentService) prepareSplitWrite(ctx context.Context, input InvestmentSplitInput) (db.CreateTransactionParams, db.CreateSplitParams, InvestmentSplitPlan, error) {
	return s.prepareSplitWriteWith(ctx, input, 0)
}

// prepareSplitWriteWith plans a split journal. replacesOperationID names the
// split a replacement supersedes (T-129): the plan then replays without it.
func (s *InvestmentService) prepareSplitWriteWith(ctx context.Context, input InvestmentSplitInput, replacesOperationID int64) (db.CreateTransactionParams, db.CreateSplitParams, InvestmentSplitPlan, error) {
	fail := func(err error) (db.CreateTransactionParams, db.CreateSplitParams, InvestmentSplitPlan, error) {
		return db.CreateTransactionParams{}, db.CreateSplitParams{}, InvestmentSplitPlan{}, err
	}
	if input.OwnerUserID <= 0 {
		return fail(ValidationError{Message: "owner user is required"})
	}
	date, err := cleanRequiredDate(input.EffectiveOn, "split effective date")
	if err != nil {
		return fail(err)
	}
	if input.RatioNumerator <= 0 || input.RatioDenominator <= 0 ||
		input.RatioNumerator > investmentSplitRatioMax || input.RatioDenominator > investmentSplitRatioMax {
		return fail(ValidationError{Message: "split ratio must be two positive whole numbers"})
	}
	// 4-for-2 and 2-for-1 are the same corporate action; store lowest terms.
	divisor := new(big.Int).GCD(nil, nil, big.NewInt(input.RatioNumerator), big.NewInt(input.RatioDenominator)).Int64()
	numerator, denominator := input.RatioNumerator/divisor, input.RatioDenominator/divisor
	if numerator == denominator {
		return fail(ValidationError{Message: "a 1-for-1 split changes nothing"})
	}
	dependencies := newAccountRuleDependencies()
	if _, err := s.accountInRole(ctx, input.HoldingAccountID, date, holdingRole, dependencies); err != nil {
		return fail(err)
	}
	if err := s.requireCommodityKind(ctx, input.CommodityID, date, "split commodity", false); err != nil {
		return fail(err)
	}
	tradingID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fail(ValidationError{Message: "commodity trading system account is required"})
		}
		return fail(fmt.Errorf("resolve commodity trading account: %w", err))
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return fail(err)
	}
	evidence, err := cleanSizedJSONObject(input.SourceEvidenceJSON, "source evidence", investmentJSONMaxBytes)
	if err != nil {
		return fail(err)
	}
	split := db.CreateSplitParams{
		BookID: BookID, AccountID: input.HoldingAccountID, CommodityID: input.CommodityID,
		EffectiveOn: date, RatioNumerator: numerator, RatioDenominator: denominator,
		SourceEvidenceJSON: evidence, ReplacesOperationID: replacesOperationID,
	}
	planned, err := s.repository.PlanSplit(ctx, split, input.OwnerUserID)
	if err != nil {
		return fail(mapInvestmentSplitError(err))
	}
	split.ExpectedDeltaValue, split.ExpectedDeltaScale = planned.DeltaValue, planned.DeltaScale
	plan := investmentTransactionPlan{
		Date: date, MetadataJSON: evidence, AccountRuleDependencies: dependencies,
		Create: CreateTransactionInput{
			OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
			RequestID: input.RequestID, OriginType: defaultString(input.OriginType, "browser_api"),
			Operation: defaultString(input.Operation, "investment.split"), ChangeReason: input.ChangeReason,
			ReconciliationOverride: input.ReconciliationOverride,
			Spec: TransactionInput{
				Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "split",
				TransactionDate: date, Description: memo,
				JournalEntries: []JournalEntryInput{{EntryDate: date, EntryKind: "investment", Memo: memo,
					Postings: []PostingInput{
						{AccountID: input.HoldingAccountID, CommodityID: input.CommodityID,
							QuantityValue: planned.DeltaValue, QuantityScale: planned.DeltaScale, Memo: memo},
						{AccountID: tradingID, CommodityID: input.CommodityID,
							QuantityValue: planned.DeltaValue.Negated(), QuantityScale: planned.DeltaScale, Memo: memo},
					}}},
			},
		},
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return fail(err)
	}
	return journal, split, toInvestmentSplitPlan(numerator, denominator, planned), nil
}

// PreviewSplit plans the split and runs its complete writer, including any
// dependent replay, in a rolled-back transaction.
func (s *InvestmentService) PreviewSplit(ctx context.Context, input InvestmentSplitInput) (InvestmentSplitPreview, error) {
	input.ReconciliationOverride = true
	journal, split, plan, err := s.prepareSplitWrite(ctx, input)
	if err != nil {
		return InvestmentSplitPreview{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, err := s.repository.SimulateSplit(ctx, journal, split)
	if err != nil {
		return InvestmentSplitPreview{}, mapInvestmentSplitError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return InvestmentSplitPreview{}, err
	}
	return InvestmentSplitPreview{Plan: plan, Impact: impact}, nil
}

// Split commits the split journal, sourced ratio, per-lot effects and any
// dependent replay under one audit event. A backdated split that revises a
// committed disposal's gain needs the preview's acknowledgement (T-114).
func (s *InvestmentService) Split(ctx context.Context, input InvestmentSplitInput) (InvestmentSplitResult, error) {
	journal, split, _, err := s.prepareSplitWrite(ctx, input)
	if err != nil {
		return InvestmentSplitResult{}, err
	}
	journal.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transaction, committed, err := s.repository.CreateSplit(ctx, journal, split)
	if err != nil {
		return InvestmentSplitResult{}, mapInvestmentSplitError(err)
	}
	return InvestmentSplitResult{
		Transaction: toTransaction(transaction),
		Plan:        toInvestmentSplitPlan(split.RatioNumerator, split.RatioDenominator, committed),
	}, nil
}

func toInvestmentSplitPlan(numerator, denominator int64, plan db.SplitPlan) InvestmentSplitPlan {
	out := InvestmentSplitPlan{RatioNumerator: numerator, RatioDenominator: denominator,
		DeltaValue: plan.DeltaValue, DeltaScale: plan.DeltaScale, Replayed: plan.Replayed,
		Effects: make([]InvestmentSplitLotEffect, 0, len(plan.Effects))}
	for _, effect := range plan.Effects {
		out.Effects = append(out.Effects, InvestmentSplitLotEffect{
			LotID: effect.LotID, CostCommodityID: effect.CostCommodityID,
			BeforeValue: effect.BeforeValue, BeforeScale: effect.BeforeScale,
			AfterValue: effect.AfterValue, AfterScale: effect.AfterScale,
		})
	}
	return out
}

func mapInvestmentSplitError(err error) error {
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) {
		return InvestmentSplitDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	switch {
	case errors.Is(err, db.ErrSplitNoEligibleHoldings), errors.Is(err, db.ErrSplitFractionUnrepresentable),
		errors.Is(err, db.ErrSplitPositionChanged), errors.Is(err, db.ErrGainImpactAcknowledgementRequired),
		errors.Is(err, db.ErrGainImpactAcknowledgementStale):
		return err
	case errors.Is(err, db.ErrUnknownInvestmentBasis):
		return ValidationError{Message: "a holding with unresolved basis cannot be split yet"}
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("investment split: %w", mapTransactionDBError(err))
	}
}
