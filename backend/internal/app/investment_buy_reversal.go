package app

import (
	"context"
	"strings"

	"rekenraam/backend/internal/db"
)

type ReverseInvestmentBuyInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride bool
	// GainImpactAcknowledgement echoes the preview token for the committed
	// disposal gain changes the user accepted (T-126).
	GainImpactAcknowledgement string
}

// ReverseBuy terminates an effective long buy. The inverse journal and
// replayed lot projection commit together; an impossible later disposal
// leaves the original buy effective.
func (s *InvestmentService) ReverseBuy(ctx context.Context, input ReverseInvestmentBuyInput) (Transaction, error) {
	operation, params, err := s.prepareBuyReversalWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.ReverseBuy(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapBuyReplacementError(err)
	}
	return toTransaction(record), nil
}

// ReverseBuyReconciliationImpact runs the actual reversal writer and its
// dependent replay in a rolled-back transaction (T-126).
func (s *InvestmentService) ReverseBuyReconciliationImpact(ctx context.Context, input ReverseInvestmentBuyInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, params, err := s.prepareBuyReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewBuyReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapBuyReplacementError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareBuyReversalWrite(ctx context.Context, input ReverseInvestmentBuyInput) (db.BuyOperationRecord, db.CreateTransactionParams, error) {
	operation, planned, err := s.reverseBuyPlan(ctx, input)
	if err != nil {
		return db.BuyOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, planned, nil)
	if err != nil {
		return db.BuyOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = planned.ChangeReason
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	return operation, params, nil
}

func (s *InvestmentService) reverseBuyPlan(ctx context.Context, input ReverseInvestmentBuyInput) (db.BuyOperationRecord, CreateTransactionInput, error) {
	if input.OwnerUserID <= 0 || input.TransactionID <= 0 {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, ValidationError{Message: "owner and buy id are required"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, ValidationError{Message: "buy reversal reason is required"}
	}
	operation, planned, err := s.buyReplacementPlan(ctx, ReplaceInvestmentBuyInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, TransactionID: input.TransactionID,
		Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
	})
	if err != nil {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, err
	}
	if operation.ImportedLineage {
		linked, err := s.repository.HasCommittedImportSource(ctx, BookID, operation.OperationID)
		if err != nil {
			return db.BuyOperationRecord{}, CreateTransactionInput{}, err
		}
		if !linked {
			return db.BuyOperationRecord{}, CreateTransactionInput{}, ErrInvestmentImportedBuy
		}
	}
	planned.Operation = "investment.buy.reverse"
	planned.Spec.InvestmentOperationKind = "reversal"
	return operation, planned, nil
}
