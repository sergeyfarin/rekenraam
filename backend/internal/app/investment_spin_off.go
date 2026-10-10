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
	// ErrSpinOffNoHoldings means nothing of the parent instrument was held in
	// the account on the effective date.
	ErrSpinOffNoHoldings = db.ErrSpinOffNoHoldings
	// ErrSpinOffFraction means a lot's new quantity needs more decimal places
	// than the new instrument permits. Cash in lieu arrives with #181; the
	// distribution is never rounded to fit.
	ErrSpinOffFraction = errors.New("the spin-off would leave a fraction the new instrument cannot record exactly")
	// ErrSpinOffChanged means a holding moved between planning and
	// committing; preview again.
	ErrSpinOffChanged = db.ErrSpinOffPositionChanged
)

// SpinOffInput records a spin-off (#180): every long lot of CommodityID in
// HoldingAccountID keeps its units and moves BasisFraction of its remaining
// basis to a new lot of DestinationCommodityID in DestinationHoldingAccountID,
// RatioNumerator new units for every RatioDenominator parent units. The
// fraction is an exact decimal strictly between 0 and 1, such as 0.141 for an
// issuer's 14.1 % allocation. The destination defaults to the same account.
type SpinOffInput struct {
	OwnerUserID                 int64
	AuthSessionID               int64
	RequestID                   string
	EffectiveOn                 string
	HoldingAccountID            int64
	DestinationHoldingAccountID int64
	CommodityID                 int64
	DestinationCommodityID      int64
	RatioNumerator              int64
	RatioDenominator            int64
	BasisFractionValue          exact.Coefficient
	BasisFractionScale          int
	SourceEvidenceJSON          string
	Memo                        string
	ChangeReason                string
	ReconciliationOverride      bool
}

// SpinOffLink is one parent lot's distribution. DestinationLotID is zero in a
// preview; RemainingBasis is the parent's basis after the spin-off and is
// known only in a preview or result.
type SpinOffLink = db.SpinOffLink

// SpinOffPlan is the stored terms, every link and the exact totals.
type SpinOffPlan struct {
	RatioNumerator           int64
	RatioDenominator         int64
	BasisFractionValue       exact.Coefficient
	BasisFractionScale       int
	Links                    []SpinOffLink
	DestinationQuantityValue exact.Coefficient
	DestinationQuantityScale int
	BasisTotals              []SpinOffBasisTotal
}

// SpinOffBasisTotal is what a spin-off divides in one cost currency. Any
// unknown lot leaves both basis totals unknown, as a position's basis is;
// UnknownLots counts them. RemainingBasis is zero when not recorded.
type SpinOffBasisTotal struct {
	CostCommodityID          int64
	SourceQuantityValue      exact.Coefficient
	SourceQuantityScale      int
	DestinationQuantityValue exact.Coefficient
	DestinationQuantityScale int
	BasisKnowledge           string
	AllocatedBasisValue      exact.Coefficient
	AllocatedBasisScale      int
	RemainingBasisValue      exact.Coefficient
	RemainingBasisScale      int
	UnknownLots              int
}

type SpinOffPreview struct {
	Plan   SpinOffPlan
	Impact ReconciliationImpact
}

type SpinOffResult struct {
	Transaction Transaction
	Plan        SpinOffPlan
}

// prepareSpinOffWrite plans the spin-off journal: two legs in the new
// instrument, so the basis never leaves commodity_trading.
func (s *InvestmentService) prepareSpinOffWrite(ctx context.Context, input SpinOffInput) (db.CreateTransactionParams, db.CreateSpinOffParams, error) {
	fail := func(err error) (db.CreateTransactionParams, db.CreateSpinOffParams, error) {
		return db.CreateTransactionParams{}, db.CreateSpinOffParams{}, err
	}
	if input.OwnerUserID <= 0 {
		return fail(ValidationError{Message: "owner user is required"})
	}
	date, err := cleanRequiredDate(input.EffectiveOn, "spin-off effective date")
	if err != nil {
		return fail(err)
	}
	if input.RatioNumerator <= 0 || input.RatioDenominator <= 0 ||
		input.RatioNumerator > investmentSplitRatioMax || input.RatioDenominator > investmentSplitRatioMax {
		return fail(ValidationError{Message: "spin-off ratio must be two positive whole numbers"})
	}
	if input.CommodityID <= 0 || input.DestinationCommodityID <= 0 || input.CommodityID == input.DestinationCommodityID {
		return fail(ValidationError{Message: "a spin-off needs two different instruments"})
	}
	fractionValue, fractionScale, err := cleanSpinOffFraction(input.BasisFractionValue, input.BasisFractionScale)
	if err != nil {
		return fail(err)
	}
	divisor := new(big.Int).GCD(nil, nil, big.NewInt(input.RatioNumerator), big.NewInt(input.RatioDenominator)).Int64()
	numerator, denominator := input.RatioNumerator/divisor, input.RatioDenominator/divisor
	destinationAccountID := input.DestinationHoldingAccountID
	if destinationAccountID == 0 {
		destinationAccountID = input.HoldingAccountID
	}
	dependencies := newAccountRuleDependencies()
	if _, err := s.accountInRole(ctx, input.HoldingAccountID, date, holdingRole, dependencies); err != nil {
		return fail(err)
	}
	if destinationAccountID != input.HoldingAccountID {
		if _, err := s.accountInRole(ctx, destinationAccountID, date, holdingRole, dependencies); err != nil {
			return fail(err)
		}
	}
	if err := s.requireCommodityKind(ctx, input.CommodityID, date, "parent instrument", false); err != nil {
		return fail(err)
	}
	if err := s.requireCommodityKind(ctx, input.DestinationCommodityID, date, "new instrument", false); err != nil {
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
	spinOff := db.CreateSpinOffParams{
		BookID: BookID, AccountID: input.HoldingAccountID, DestinationAccountID: destinationAccountID,
		CommodityID: input.CommodityID, DestinationCommodityID: input.DestinationCommodityID, EffectiveOn: date,
		RatioNumerator: numerator, RatioDenominator: denominator,
		BasisFractionValue: fractionValue, BasisFractionScale: fractionScale, SourceEvidenceJSON: evidence,
	}
	planned, err := s.repository.PlanSpinOff(ctx, spinOff)
	if err != nil {
		return fail(mapSpinOffError(err))
	}
	spinOff.ExpectedDestinationQuantityValue, spinOff.ExpectedDestinationQuantityScale = planned.DestinationQuantityValue, planned.DestinationQuantityScale
	create := CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, OriginType: "browser_api",
		Operation: "investment.spin_off", ChangeReason: input.ChangeReason,
		ReconciliationOverride: input.ReconciliationOverride,
		Spec: TransactionInput{
			Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "spin_off",
			TransactionDate: date, Description: memo,
			JournalEntries: []JournalEntryInput{{EntryDate: date, EntryKind: "investment", Memo: memo,
				Postings: []PostingInput{
					{AccountID: destinationAccountID, CommodityID: input.DestinationCommodityID,
						QuantityValue: planned.DestinationQuantityValue, QuantityScale: planned.DestinationQuantityScale, Memo: memo},
					{AccountID: tradingID, CommodityID: input.DestinationCommodityID,
						QuantityValue: planned.DestinationQuantityValue.Negated(), QuantityScale: planned.DestinationQuantityScale, Memo: memo},
				}}},
		},
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, create, dependencies)
	if err != nil {
		return fail(err)
	}
	return journal, spinOff, nil
}

// cleanSpinOffFraction returns the fraction in lowest decimal terms; it must
// lie strictly between 0 and 1 and need at most 12 decimal places.
func cleanSpinOffFraction(value exact.Coefficient, scale int) (exact.Coefficient, int, error) {
	invalid := ValidationError{Message: "basis fraction must be a decimal greater than 0 and less than 1, with at most 12 decimal places"}
	if value == "" || scale < 0 || scale > 24 {
		return "", 0, invalid
	}
	if _, err := exact.Parse(string(value)); err != nil {
		return "", 0, invalid
	}
	normalized := exact.ScaledIntFromCoefficient(value, scale).Normalized()
	coefficient, err := normalized.Coefficient()
	if err != nil || !validSpinOffFraction(coefficient, normalized.Scale()) {
		return "", 0, invalid
	}
	return coefficient, normalized.Scale(), nil
}

func validSpinOffFraction(value exact.Coefficient, scale int) bool {
	return scale >= 1 && scale <= 12 && value.Sign() > 0 && value.BigInt().Cmp(exact.Pow10(scale)) < 0
}

// PreviewSpinOff runs the complete spin-off writer, including checkpoint
// invalidation, then rolls back.
func (s *InvestmentService) PreviewSpinOff(ctx context.Context, input SpinOffInput) (SpinOffPreview, error) {
	input.ReconciliationOverride = true
	journal, spinOff, err := s.prepareSpinOffWrite(ctx, input)
	if err != nil {
		return SpinOffPreview{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, plan, err := s.repository.SimulateSpinOff(ctx, journal, spinOff)
	if err != nil {
		return SpinOffPreview{}, mapSpinOffError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return SpinOffPreview{}, err
	}
	out, err := spinOffPlanOf(spinOff.RatioNumerator, spinOff.RatioDenominator, spinOff.BasisFractionValue,
		spinOff.BasisFractionScale, plan, true)
	if err != nil {
		return SpinOffPreview{}, err
	}
	return SpinOffPreview{Plan: out, Impact: impact}, nil
}

func (s *InvestmentService) PreviewSpinOffReconciliationImpact(ctx context.Context, input SpinOffInput) (ReconciliationImpact, error) {
	preview, err := s.PreviewSpinOff(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return preview.Impact, nil
}

// SpinOff commits the journal, the spin-off fact, every parent basis
// reduction, new lot and link under one audit event.
func (s *InvestmentService) SpinOff(ctx context.Context, input SpinOffInput) (SpinOffResult, error) {
	journal, spinOff, err := s.prepareSpinOffWrite(ctx, input)
	if err != nil {
		return SpinOffResult{}, err
	}
	// An in-order spin-off changes no committed disposal.
	journal.GainImpact = gainImpactPolicy("")
	transaction, plan, err := s.repository.CreateSpinOff(ctx, journal, spinOff)
	if err != nil {
		return SpinOffResult{}, mapSpinOffError(err)
	}
	out, err := spinOffPlanOf(spinOff.RatioNumerator, spinOff.RatioDenominator, spinOff.BasisFractionValue,
		spinOff.BasisFractionScale, plan, true)
	if err != nil {
		return SpinOffResult{}, err
	}
	return SpinOffResult{Transaction: toTransaction(transaction), Plan: out}, nil
}

// spinOffPlanOf totals the links per cost currency, in first-seen order,
// exactly. withRemaining totals the parent's remaining basis too, which only a
// plan or result knows.
func spinOffPlanOf(numerator, denominator int64, fractionValue exact.Coefficient, fractionScale int,
	plan db.SpinOffPlan, withRemaining bool) (SpinOffPlan, error) {
	out := SpinOffPlan{RatioNumerator: numerator, RatioDenominator: denominator,
		BasisFractionValue: fractionValue, BasisFractionScale: fractionScale, Links: plan.Links,
		DestinationQuantityValue: plan.DestinationQuantityValue, DestinationQuantityScale: plan.DestinationQuantityScale}
	if out.Links == nil {
		out.Links = []SpinOffLink{}
	}
	type sums struct {
		source, destination, allocated, remaining *exact.ScaledInt
		unknown                                   int
	}
	order := []int64{}
	byCurrency := map[int64]*sums{}
	for _, link := range out.Links {
		current, ok := byCurrency[link.CostCommodityID]
		if !ok {
			current = &sums{source: exact.NewScaledInt(), destination: exact.NewScaledInt(),
				allocated: exact.NewScaledInt(), remaining: exact.NewScaledInt()}
			byCurrency[link.CostCommodityID] = current
			order = append(order, link.CostCommodityID)
		}
		current.source.AddCoefficient(link.SourceQuantityValue, link.SourceQuantityScale)
		current.destination.AddCoefficient(link.DestinationQuantityValue, link.DestinationQuantityScale)
		if link.BasisKnowledge == db.InvestmentBasisUnknown {
			current.unknown++
			continue
		}
		current.allocated.AddInt64(link.AllocatedBasisValue, link.AllocatedBasisScale)
		current.remaining.AddInt64(link.RemainingBasisValue, link.RemainingBasisScale)
	}
	for _, currencyID := range order {
		current := byCurrency[currencyID]
		total := SpinOffBasisTotal{CostCommodityID: currencyID, UnknownLots: current.unknown,
			SourceQuantityScale: current.source.Scale(), DestinationQuantityScale: current.destination.Scale(),
			BasisKnowledge: db.InvestmentBasisKnown}
		var err error
		if total.SourceQuantityValue, err = current.source.Coefficient(); err != nil {
			return SpinOffPlan{}, err
		}
		if total.DestinationQuantityValue, err = current.destination.Coefficient(); err != nil {
			return SpinOffPlan{}, err
		}
		if current.unknown > 0 {
			total.BasisKnowledge = db.InvestmentBasisUnknown
		} else {
			if total.AllocatedBasisValue, err = current.allocated.Coefficient(); err != nil {
				return SpinOffPlan{}, err
			}
			total.AllocatedBasisScale = current.allocated.Scale()
			if withRemaining {
				if total.RemainingBasisValue, err = current.remaining.Coefficient(); err != nil {
					return SpinOffPlan{}, err
				}
				total.RemainingBasisScale = current.remaining.Scale()
			}
		}
		out.BasisTotals = append(out.BasisTotals, total)
	}
	if out.BasisTotals == nil {
		out.BasisTotals = []SpinOffBasisTotal{}
	}
	return out, nil
}

func mapSpinOffError(err error) error {
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) {
		return InvestmentTransferDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	switch {
	case errors.Is(err, db.ErrInvestmentCorrectionDependency):
		return ErrInvestmentTransferDependency
	case errors.Is(err, db.ErrSplitFractionUnrepresentable):
		return ErrSpinOffFraction
	case errors.Is(err, db.ErrSpinOffNoHoldings), errors.Is(err, db.ErrSpinOffPositionChanged),
		errors.Is(err, db.ErrOutOfOrderPositionEvent), errors.Is(err, db.ErrPositionSideConflict):
		return err
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("spin-off: %w", mapTransactionDBError(err))
	}
}
