package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ResolveTransferBasisInput resolves the unknown basis an external transfer in
// recorded (T-145). The basis is the transfer's total sourced basis in its own
// cost currency; it is never converted from another currency.
type ResolveTransferBasisInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	// TransactionID is the effective transfer-in transaction.
	TransactionID      int64
	BasisValue         int64
	BasisScale         int
	SourceEvidenceJSON string
	Reason             string
	// ReconciliationOverride lets the dated bridge invalidate affected
	// checkpoints; GainImpactAcknowledgement echoes the preview token for the
	// sales and transfers the resolution revises.
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
}

func (s *InvestmentService) resolveTransferBasisWrite(ctx context.Context, input ResolveTransferBasisInput) (db.CreateTransactionParams, db.ResolveTransferBasisParams, error) {
	if input.OwnerUserID <= 0 || input.TransactionID <= 0 {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "owner and transfer id are required"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "resolution reason is required"}
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, err
	}
	if input.BasisValue < 0 || input.BasisScale < 0 || input.BasisScale > 12 {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "resolved basis must be nonnegative at a valid money scale"}
	}
	if input.BasisValue == 0 {
		// A resolution posts its bridge; a known zero posts none, so it is
		// recorded by correcting the transfer with an explicit zero basis.
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "a zero basis is recorded by correcting the transfer, not by resolution"}
	}
	evidence, err := cleanSizedJSONObject(input.SourceEvidenceJSON, "source evidence", investmentJSONMaxBytes)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, err
	}
	operation, err := s.repository.TransferOperationByTransactionID(ctx, BookID, input.TransactionID)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, mapTransferCorrectionError(err)
	}
	if operation.TransferKind != "external_in" || operation.AlreadyCorrected {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ErrInvestmentTransferBasisNotUnknown
	}
	target, err := s.repository.UnknownTransferBasisForOperation(ctx, BookID, operation.OperationID)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, mapTransferCorrectionError(err)
	}
	tradingID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, fmt.Errorf("resolve commodity trading account: %w", err)
	}
	equityID, err := s.repository.ExternalInvestmentTransferEquityAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "external investment transfer equity account is required"}
		}
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, err
	}
	// The complete omitted inbound bridge, dated to the transfer.
	postings := []PostingInput{
		{AccountID: tradingID, CommodityID: target.CostCommodityID, QuantityValue: exact.New(input.BasisValue), QuantityScale: input.BasisScale},
		{AccountID: equityID, CommodityID: target.CostCommodityID, QuantityValue: exact.New(-input.BasisValue), QuantityScale: input.BasisScale},
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		OriginType: "browser_api", Operation: "investment.basis_resolution", ChangeReason: reason,
		ReconciliationOverride: input.ReconciliationOverride,
		Spec: TransactionInput{
			Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "basis_resolution",
			TransactionDate: target.EffectiveOn, Description: reason, MetadataJSON: evidence,
			JournalEntries: []JournalEntryInput{{EntryDate: target.EffectiveOn, EntryKind: "investment", Memo: reason, Postings: postings}},
		},
	}, nil)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, err
	}
	return journal, db.ResolveTransferBasisParams{BookID: BookID, TransferOperationID: operation.OperationID,
		BasisValue: exact.New(input.BasisValue), BasisScale: input.BasisScale, SourceEvidenceJSON: evidence}, nil
}

// ResolveTransferBasisImpact runs the complete resolution, replay and gain
// comparison, then rolls back, so a preview reports what the commit would do.
func (s *InvestmentService) ResolveTransferBasisImpact(ctx context.Context, input ResolveTransferBasisInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	journal, params, err := s.resolveTransferBasisWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, err := s.repository.SimulateTransferBasisResolution(ctx, journal, params)
	if err != nil {
		return ReconciliationImpact{}, mapTransferCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

// ResolveTransferBasis records the sourced basis of an unknown inbound
// transfer. Every sale and transfer its lot reached is revised by replay; the
// changed gains need the preview's acknowledgement.
func (s *InvestmentService) ResolveTransferBasis(ctx context.Context, input ResolveTransferBasisInput) (Transaction, error) {
	journal, params, err := s.resolveTransferBasisWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	journal.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transaction, err := s.repository.ResolveTransferBasis(ctx, journal, params)
	if err != nil {
		return Transaction{}, mapTransferCorrectionError(err)
	}
	return toTransaction(transaction), nil
}
