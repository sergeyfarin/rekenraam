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
}

// ReverseBuy terminates an effective long buy. The inverse journal and
// replayed lot projection commit together; an impossible later disposal
// leaves the original buy effective.
func (s *InvestmentService) ReverseBuy(ctx context.Context, input ReverseInvestmentBuyInput) (Transaction, error) {
	operation, planned, err := s.reverseBuyPlan(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	params, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, planned, nil)
	if err != nil {
		return Transaction{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = planned.ChangeReason
	record, err := s.repository.ReverseBuy(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapBuyReplacementError(err)
	}
	return toTransaction(record), nil
}

func (s *InvestmentService) ReverseBuyReconciliationImpact(ctx context.Context, input ReverseInvestmentBuyInput) (ReconciliationImpact, error) {
	operation, planned, err := s.reverseBuyPlan(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	if err := s.repository.SimulateBuyReversal(ctx, BookID, operation); err != nil {
		return ReconciliationImpact{}, mapBuyReplacementError(err)
	}
	return s.transactionService.investmentReconciliationImpactForCreate(ctx, CreateReconciliationImpactInput{
		OwnerUserID: input.OwnerUserID, Spec: planned.Spec,
	})
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
