package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ExternalTransferInInput records an asset arriving from outside this book.
// Carried basis is a sourced, known amount (zero included), or explicitly
// unknown: then only the security legs post, and the lot opens with unknown
// basis until a sourced resolution (T-145).
type ExternalTransferInInput struct {
	OwnerUserID       int64
	AuthSessionID     int64
	RequestID         string
	EffectiveOn       string
	HoldingAccountID  int64
	CommodityID       int64
	QuantityValue     exact.Coefficient
	QuantityScale     int
	CarriedBasisValue int64
	CarriedBasisScale int
	// BasisKnowledge is known (the default) or unknown. Unknown basis takes
	// no amount; it never stands for zero.
	BasisKnowledge         string
	CostCommodityID        int64
	OriginalAcquiredOn     string
	SourceEvidenceJSON     string
	Memo                   string
	ChangeReason           string
	ReconciliationOverride bool
	// GainImpactAcknowledgement echoes the preview token when a transfer dated
	// behind later disposals revises their gains (T-117).
	GainImpactAcknowledgement string
}

func (s *InvestmentService) externalTransferInPlan(ctx context.Context, input ExternalTransferInInput) (investmentTransactionPlan, error) {
	if input.OwnerUserID <= 0 {
		return investmentTransactionPlan{}, ValidationError{Message: "owner user is required"}
	}
	date, err := cleanRequiredDate(input.EffectiveOn, "transfer effective date")
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	original, err := cleanOptionalDate(input.OriginalAcquiredOn, "original acquisition date")
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	if original != "" && original > date {
		return investmentTransactionPlan{}, ValidationError{Message: "original acquisition date cannot follow transfer effective date"}
	}
	if input.QuantityValue.Sign() <= 0 || input.QuantityScale < 0 || input.QuantityScale > exact.MaxCryptoScale {
		return investmentTransactionPlan{}, ValidationError{Message: "transfer quantity must be positive at a valid scale"}
	}
	knowledge := normalizedKnowledge(input.BasisKnowledge)
	switch {
	case knowledge != db.InvestmentBasisKnown && knowledge != db.InvestmentBasisUnknown:
		return investmentTransactionPlan{}, ValidationError{Message: "basis knowledge must be known or unknown"}
	case knowledge == db.InvestmentBasisUnknown && (input.CarriedBasisValue != 0 || input.CarriedBasisScale != 0):
		return investmentTransactionPlan{}, ValidationError{Message: "unknown carried basis cannot have an amount"}
	case input.CarriedBasisValue < 0 || input.CarriedBasisScale < 0 || input.CarriedBasisScale > 12:
		return investmentTransactionPlan{}, ValidationError{Message: "known carried basis must be nonnegative at a valid money scale"}
	}
	if input.CostCommodityID <= 0 {
		return investmentTransactionPlan{}, ValidationError{Message: "basis currency is required"}
	}
	dependencies := newAccountRuleDependencies()
	if _, err := s.accountInRole(ctx, input.HoldingAccountID, date, holdingRole, dependencies); err != nil {
		return investmentTransactionPlan{}, err
	}
	if err := s.requireCommodityKind(ctx, input.CommodityID, date, "transferred commodity", false); err != nil {
		return investmentTransactionPlan{}, err
	}
	if err := s.requireCommodityKind(ctx, input.CostCommodityID, date, "basis commodity", true); err != nil {
		return investmentTransactionPlan{}, err
	}
	tradingID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return investmentTransactionPlan{}, ValidationError{Message: "commodity trading system account is required"}
		}
		return investmentTransactionPlan{}, fmt.Errorf("resolve commodity trading account: %w", err)
	}
	equityID, err := s.repository.ExternalInvestmentTransferEquityAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return investmentTransactionPlan{}, ValidationError{Message: "external investment transfer equity account is required"}
		}
		return investmentTransactionPlan{}, fmt.Errorf("resolve external investment transfer equity account: %w", err)
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	evidence, err := cleanSizedJSONObject(input.SourceEvidenceJSON, "source evidence", investmentJSONMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	postings := []PostingInput{
		{AccountID: input.HoldingAccountID, CommodityID: input.CommodityID, QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale, Memo: memo},
		{AccountID: tradingID, CommodityID: input.CommodityID, QuantityValue: input.QuantityValue.Negated(), QuantityScale: input.QuantityScale, Memo: memo},
	}
	// Unknown basis posts no bridge; a known zero posts none either.
	if knowledge == db.InvestmentBasisKnown && input.CarriedBasisValue > 0 {
		postings = append(postings,
			PostingInput{AccountID: tradingID, CommodityID: input.CostCommodityID, QuantityValue: exact.New(input.CarriedBasisValue), QuantityScale: input.CarriedBasisScale, Memo: memo},
			PostingInput{AccountID: equityID, CommodityID: input.CostCommodityID, QuantityValue: exact.New(-input.CarriedBasisValue), QuantityScale: input.CarriedBasisScale, Memo: memo},
		)
	}
	return investmentTransactionPlan{
		Date: date, MetadataJSON: evidence, AccountRuleDependencies: dependencies,
		Create: CreateTransactionInput{
			OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
			RequestID: input.RequestID, OriginType: "browser_api",
			Operation: "investment.external_transfer_in", ChangeReason: input.ChangeReason,
			ReconciliationOverride: input.ReconciliationOverride,
			Spec: TransactionInput{
				Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "external_transfer_in",
				TransactionDate: date, Description: memo,
				JournalEntries: []JournalEntryInput{{EntryDate: date, EntryKind: "investment", Memo: memo, Postings: postings}},
			},
		},
	}, nil
}

// externalTransferInWrite freezes the journal and lot facts the preview and
// the commit both hand to the same writer.
func (s *InvestmentService) externalTransferInWrite(ctx context.Context, input ExternalTransferInInput) (db.CreateTransactionParams, db.CreateExternalTransferInParams, error) {
	plan, err := s.externalTransferInPlan(ctx, input)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateExternalTransferInParams{}, err
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateExternalTransferInParams{}, err
	}
	return journal, db.CreateExternalTransferInParams{
		Lot: db.CreateInvestmentLotParams{
			BookID: BookID, AccountID: input.HoldingAccountID, CommodityID: input.CommodityID,
			OpenedOn: plan.Date, QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale,
			CostBasisValue: input.CarriedBasisValue, CostBasisScale: input.CarriedBasisScale,
			OpeningBasisKnowledge: normalizedKnowledge(input.BasisKnowledge),
			CostCommodityID:       input.CostCommodityID, MetadataJSON: `{"source":"external_transfer_in"}`,
			CreatedAt: s.now().UTC().Format(time.RFC3339), CreatedByUserID: input.OwnerUserID,
		},
		OriginalAcquiredOn: strings.TrimSpace(input.OriginalAcquiredOn), SourceEvidenceJSON: plan.MetadataJSON,
	}, nil
}

// PreviewExternalTransferInReconciliationImpact runs the complete writer,
// including replay of later disposals, and rolls back.
func (s *InvestmentService) PreviewExternalTransferInReconciliationImpact(ctx context.Context, input ExternalTransferInInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	journal, transfer, err := s.externalTransferInWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, err := s.repository.SimulateExternalTransferIn(ctx, journal, transfer)
	if err != nil {
		return ReconciliationImpact{}, mapInvestmentOpeningWriteError(err, "external investment transfer")
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

// ExternalTransferIn records an inbound transfer with known or explicitly
// unknown basis. One dated behind
// later disposals replays them and needs the preview's gain acknowledgement
// when a committed gain changes (T-117).
func (s *InvestmentService) ExternalTransferIn(ctx context.Context, input ExternalTransferInInput) (InvestmentTradeResult, error) {
	journal, transfer, err := s.externalTransferInWrite(ctx, input)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	journal.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transaction, lot, err := s.repository.CreateExternalTransferIn(ctx, journal, transfer)
	if err != nil {
		return InvestmentTradeResult{}, mapInvestmentOpeningWriteError(err, "external investment transfer")
	}
	return InvestmentTradeResult{Transaction: toTransaction(transaction), LotID: &lot.ID}, nil
}
