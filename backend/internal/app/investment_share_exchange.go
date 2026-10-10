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
	// ErrShareExchangeNoHoldings means nothing of the old instrument was held
	// in the account on the effective date.
	ErrShareExchangeNoHoldings = db.ErrShareExchangeNoHoldings
	// ErrShareExchangeFraction means a lot's new quantity needs more decimal
	// places than the new instrument permits. Cash in lieu of exchange
	// fractions arrives with #181; the exchange is never rounded to fit.
	ErrShareExchangeFraction = errors.New("the exchange would leave a fraction the new instrument cannot record exactly")
	// ErrShareExchangeChanged means a holding moved between planning and
	// committing; preview again.
	ErrShareExchangeChanged = db.ErrShareExchangePositionChanged
)

// ShareExchangeInput records a share exchange (#177): the whole long holding
// of CommodityID in HoldingAccountID becomes DestinationCommodityID in
// DestinationHoldingAccountID, numerator new units for every denominator old
// units. A 1-for-2 fund merger is 1/2; a class conversion one-for-one is 1/1.
// The destination defaults to the same account; a holding whose default
// instrument is the old one hands the new units to the new instrument's own.
type ShareExchangeInput struct {
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
	SourceEvidenceJSON          string
	Memo                        string
	ChangeReason                string
	ReconciliationOverride      bool
	// GainImpactAcknowledgement is accepted for the shared contract; an
	// in-order exchange changes no committed disposal.
	GainImpactAcknowledgement string
}

// ShareExchangeLink is one source lot's move into its destination lot.
// DestinationLotID is zero in a preview.
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
	BasisKnowledge           string
	OriginalDateKnowledge    string
	OriginalAcquiredOn       string
}

// ShareExchangePlan is the stored ratio, every link and the exact totals the
// journal moves out of the old and into the new instrument.
type ShareExchangePlan struct {
	RatioNumerator           int64
	RatioDenominator         int64
	Links                    []ShareExchangeLink
	SourceQuantityValue      exact.Coefficient
	SourceQuantityScale      int
	DestinationQuantityValue exact.Coefficient
	DestinationQuantityScale int
}

type ShareExchangePreview struct {
	Plan   ShareExchangePlan
	Impact ReconciliationImpact
}

type ShareExchangeResult struct {
	Transaction Transaction
	Plan        ShareExchangePlan
}

func (s *InvestmentService) prepareShareExchangeWrite(ctx context.Context, input ShareExchangeInput) (db.CreateTransactionParams, db.CreateShareExchangeParams, error) {
	fail := func(err error) (db.CreateTransactionParams, db.CreateShareExchangeParams, error) {
		return db.CreateTransactionParams{}, db.CreateShareExchangeParams{}, err
	}
	if input.OwnerUserID <= 0 {
		return fail(ValidationError{Message: "owner user is required"})
	}
	date, err := cleanRequiredDate(input.EffectiveOn, "exchange effective date")
	if err != nil {
		return fail(err)
	}
	if input.RatioNumerator <= 0 || input.RatioDenominator <= 0 ||
		input.RatioNumerator > investmentSplitRatioMax || input.RatioDenominator > investmentSplitRatioMax {
		return fail(ValidationError{Message: "exchange ratio must be two positive whole numbers"})
	}
	if input.CommodityID <= 0 || input.DestinationCommodityID <= 0 || input.CommodityID == input.DestinationCommodityID {
		return fail(ValidationError{Message: "an exchange needs two different instruments"})
	}
	// 2-for-4 and 1-for-2 are the same terms; store lowest terms.
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
	if err := s.requireCommodityKind(ctx, input.CommodityID, date, "exchanged instrument", false); err != nil {
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
	exchange := db.CreateShareExchangeParams{
		BookID: BookID, AccountID: input.HoldingAccountID, DestinationAccountID: destinationAccountID,
		CommodityID: input.CommodityID, DestinationCommodityID: input.DestinationCommodityID, EffectiveOn: date,
		RatioNumerator: numerator, RatioDenominator: denominator, SourceEvidenceJSON: evidence,
	}
	planned, err := s.repository.PlanShareExchange(ctx, exchange)
	if err != nil {
		return fail(mapShareExchangeError(err))
	}
	exchange.ExpectedSourceQuantityValue, exchange.ExpectedSourceQuantityScale = planned.SourceQuantityValue, planned.SourceQuantityScale
	exchange.ExpectedDestinationQuantityValue, exchange.ExpectedDestinationQuantityScale = planned.DestinationQuantityValue, planned.DestinationQuantityScale
	// Four legs, each balanced in its own instrument; no cost-currency leg,
	// so the carried basis never leaves commodity_trading.
	plan := investmentTransactionPlan{
		Date: date, MetadataJSON: evidence, AccountRuleDependencies: dependencies,
		Create: CreateTransactionInput{
			OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
			RequestID: input.RequestID, OriginType: "browser_api",
			Operation: "investment.share_exchange", ChangeReason: input.ChangeReason,
			ReconciliationOverride: input.ReconciliationOverride,
			Spec: TransactionInput{
				Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "share_exchange",
				TransactionDate: date, Description: memo,
				JournalEntries: []JournalEntryInput{{EntryDate: date, EntryKind: "investment", Memo: memo,
					Postings: []PostingInput{
						{AccountID: input.HoldingAccountID, CommodityID: input.CommodityID,
							QuantityValue: planned.SourceQuantityValue.Negated(), QuantityScale: planned.SourceQuantityScale, Memo: memo},
						{AccountID: tradingID, CommodityID: input.CommodityID,
							QuantityValue: planned.SourceQuantityValue, QuantityScale: planned.SourceQuantityScale, Memo: memo},
						{AccountID: destinationAccountID, CommodityID: input.DestinationCommodityID,
							QuantityValue: planned.DestinationQuantityValue, QuantityScale: planned.DestinationQuantityScale, Memo: memo},
						{AccountID: tradingID, CommodityID: input.DestinationCommodityID,
							QuantityValue: planned.DestinationQuantityValue.Negated(), QuantityScale: planned.DestinationQuantityScale, Memo: memo},
					}}},
			},
		},
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return fail(err)
	}
	return journal, exchange, nil
}

// PreviewShareExchange runs the complete exchange writer, including checkpoint
// invalidation, then rolls back.
func (s *InvestmentService) PreviewShareExchange(ctx context.Context, input ShareExchangeInput) (ShareExchangePreview, error) {
	input.ReconciliationOverride = true
	journal, exchange, err := s.prepareShareExchangeWrite(ctx, input)
	if err != nil {
		return ShareExchangePreview{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, plan, err := s.repository.SimulateShareExchange(ctx, journal, exchange)
	if err != nil {
		return ShareExchangePreview{}, mapShareExchangeError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return ShareExchangePreview{}, err
	}
	return ShareExchangePreview{Plan: toShareExchangePlan(exchange, plan), Impact: impact}, nil
}

func (s *InvestmentService) PreviewShareExchangeReconciliationImpact(ctx context.Context, input ShareExchangeInput) (ReconciliationImpact, error) {
	preview, err := s.PreviewShareExchange(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return preview.Impact, nil
}

// ShareExchange commits the journal, the exchange fact, every source
// depletion, destination lot and link under one audit event.
func (s *InvestmentService) ShareExchange(ctx context.Context, input ShareExchangeInput) (ShareExchangeResult, error) {
	journal, exchange, err := s.prepareShareExchangeWrite(ctx, input)
	if err != nil {
		return ShareExchangeResult{}, err
	}
	journal.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transaction, plan, err := s.repository.CreateShareExchange(ctx, journal, exchange)
	if err != nil {
		return ShareExchangeResult{}, mapShareExchangeError(err)
	}
	return ShareExchangeResult{Transaction: toTransaction(transaction), Plan: toShareExchangePlan(exchange, plan)}, nil
}

func toShareExchangePlan(exchange db.CreateShareExchangeParams, plan db.ShareExchangePlan) ShareExchangePlan {
	out := ShareExchangePlan{RatioNumerator: exchange.RatioNumerator, RatioDenominator: exchange.RatioDenominator,
		SourceQuantityValue: plan.SourceQuantityValue, SourceQuantityScale: plan.SourceQuantityScale,
		DestinationQuantityValue: plan.DestinationQuantityValue, DestinationQuantityScale: plan.DestinationQuantityScale,
		Links: make([]ShareExchangeLink, 0, len(plan.Links))}
	for _, link := range plan.Links {
		out.Links = append(out.Links, ShareExchangeLink(link))
	}
	return out
}

func mapShareExchangeError(err error) error {
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) {
		return InvestmentTransferDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	switch {
	case errors.Is(err, db.ErrSplitFractionUnrepresentable):
		// Not wrapped: the split refusal shares the arithmetic, not the code.
		return ErrShareExchangeFraction
	case errors.Is(err, db.ErrShareExchangeNoHoldings), errors.Is(err, db.ErrShareExchangePositionChanged),
		errors.Is(err, db.ErrOutOfOrderPositionEvent), errors.Is(err, db.ErrPositionSideConflict),
		errors.Is(err, db.ErrGainImpactAcknowledgementRequired), errors.Is(err, db.ErrGainImpactAcknowledgementStale):
		return err
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("share exchange: %w", mapTransactionDBError(err))
	}
}
