package app

import (
	"context"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
)

// Native correction of named shorts (#175). A short sale is corrected like a
// buy — its inverse retires the opening and the short position replays — and
// a cover like a sale. Each command is fenced to its own operation kind and
// replays only the short side.

var (
	ErrInvestmentShortNotFound         = errors.New("short sale or cover operation not found")
	ErrInvestmentShortAlreadyCorrected = errors.New("short sale or cover already corrected")
	ErrInvestmentShortChanged          = errors.New("short sale or cover changed")
)

// ErrInvestmentShortDependency refuses a short entry or correction that
// leaves a later cover too few owed units; the error names that cover.
var ErrInvestmentShortDependency = errors.New("a later short cover depends on these borrowed units")

type InvestmentShortDependencyError struct {
	OperationID int64
	DecisionID  int64
}

func (e InvestmentShortDependencyError) Error() string {
	return fmt.Sprintf("%s: later operation %d decision %d", ErrInvestmentShortDependency, e.OperationID, e.DecisionID)
}

func (e InvestmentShortDependencyError) Unwrap() error { return ErrInvestmentShortDependency }

// shortDependency restates the long-family dependency errors the shared
// writers return for a short position, so the message names a cover.
func shortDependency(err error) error {
	var buy InvestmentBuyDependencyError
	var sale InvestmentSaleDependencyError
	switch {
	case errors.As(err, &buy):
		return InvestmentShortDependencyError{OperationID: buy.OperationID, DecisionID: buy.DecisionID}
	case errors.As(err, &sale):
		return InvestmentShortDependencyError{OperationID: sale.OperationID, DecisionID: sale.DecisionID}
	case errors.Is(err, ErrInvestmentBuyDependency), errors.Is(err, ErrInvestmentSaleDependency):
		return ErrInvestmentShortDependency
	}
	return err
}

var shortCoverCorrectionFamily = disposalCorrectionFamily{kind: "short_cover", noun: "short cover",
	reverseOperation: "investment.short_cover.reverse", notFound: ErrInvestmentShortNotFound,
	alreadyCorrected: ErrInvestmentShortAlreadyCorrected, changed: ErrInvestmentShortChanged}

// ReverseShortCover restores the short lots a cover closed and replays the
// short position.
func (s *InvestmentService) ReverseShortCover(ctx context.Context, input ReverseInvestmentSaleInput) (Transaction, error) {
	transaction, err := s.reverseDisposal(ctx, input, shortCoverCorrectionFamily)
	return transaction, shortDependency(err)
}

func (s *InvestmentService) ReverseShortCoverReconciliationImpact(ctx context.Context, input ReverseInvestmentSaleInput) (ReconciliationImpact, error) {
	impact, err := s.reverseDisposalReconciliationImpact(ctx, input, shortCoverCorrectionFamily)
	return impact, shortDependency(err)
}

// ReplaceShortCover corrects a cover's date, quantity, amount, charges or
// method and replays every later cover of the position.
func (s *InvestmentService) ReplaceShortCover(ctx context.Context, input ReplaceInvestmentSaleInput) (ReplaceInvestmentSaleResult, error) {
	result, err := s.replaceDisposal(ctx, input, "browser_api", "investment.short_cover.replace", nil, shortCoverCorrectionFamily)
	return result, shortDependency(err)
}

func (s *InvestmentService) ReplaceShortCoverReconciliationImpact(ctx context.Context, input ReplaceInvestmentSaleInput) (ReconciliationImpact, error) {
	impact, err := s.replaceDisposalReconciliationImpact(ctx, input, "investment.short_cover.replace", shortCoverCorrectionFamily)
	return impact, shortDependency(err)
}

// ReverseShortSale retires a short opening; a later cover that needed its
// units is named and nothing is written.
func (s *InvestmentService) ReverseShortSale(ctx context.Context, input ReverseInvestmentBuyInput) (Transaction, error) {
	operation, params, err := s.prepareShortSaleReversalWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.ReverseBuy(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapShortSaleCorrectionError(err)
	}
	return toTransaction(record), nil
}

func (s *InvestmentService) ReverseShortSaleReconciliationImpact(ctx context.Context, input ReverseInvestmentBuyInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, params, err := s.prepareShortSaleReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewBuyReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapShortSaleCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

// ReplaceShortSale corrects an opening and replays the later covers of its
// short position (and of its new position when it moved).
func (s *InvestmentService) ReplaceShortSale(ctx context.Context, input ReplaceInvestmentBuyInput) (ReplaceInvestmentBuyResult, error) {
	prepared, err := s.prepareAcquisitionReplacementWrite(ctx, input, "browser_api", "investment.short_sale.replace", "short_sale")
	if err != nil {
		return ReplaceInvestmentBuyResult{}, err
	}
	record, err := s.repository.ReplaceBuyWithPostWrite(ctx, prepared.Operation, prepared.Inverse, prepared.Replacement, prepared.Lot, nil)
	if err != nil {
		return ReplaceInvestmentBuyResult{}, mapShortSaleCorrectionError(err)
	}
	lotID := record.Lot.ID
	return ReplaceInvestmentBuyResult{
		Inverse:                toTransaction(record.Inverse),
		Replacement:            InvestmentTradeResult{Transaction: toTransaction(record.Replacement), LotID: &lotID},
		CorrectedTransactionID: prepared.Operation.TransactionID,
	}, nil
}

func (s *InvestmentService) ReplaceShortSaleReconciliationImpact(ctx context.Context, input ReplaceInvestmentBuyInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	prepared, err := s.prepareAcquisitionReplacementWrite(ctx, input, "browser_api", "investment.short_sale.replace", "short_sale")
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.SimulateBuyReplacement(ctx, prepared.Operation, prepared.Inverse, prepared.Replacement, prepared.Lot)
	if err != nil {
		return ReconciliationImpact{}, mapShortSaleCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) shortSaleCorrectionPlan(ctx context.Context, input ReplaceInvestmentBuyInput) (opening db.BuyOperationRecord, planned CreateTransactionInput, err error) {
	return s.acquisitionCorrectionPlan(ctx, acquisitionCorrectionRequest{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		TransactionID: input.TransactionID, Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
		OperationKind: "short_sale", Operation: "investment.short_sale.replace", Noun: "short sale",
	})
}

func (s *InvestmentService) prepareShortSaleReversalWrite(ctx context.Context, input ReverseInvestmentBuyInput) (db.BuyOperationRecord, db.CreateTransactionParams, error) {
	operation, planned, err := s.shortSaleCorrectionPlan(ctx, ReplaceInvestmentBuyInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		TransactionID: input.TransactionID, Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
	})
	if err != nil {
		return db.BuyOperationRecord{}, db.CreateTransactionParams{}, err
	}
	planned.Operation = "investment.short_sale.reverse"
	planned.Spec.InvestmentOperationKind = "reversal"
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

// mapShortSaleCorrectionError names a later cover the corrected opening can
// no longer satisfy, like a buy correction names a later sale.
func mapShortSaleCorrectionError(err error) error {
	mapped := shortDependency(mapBuyReplacementError(err))
	switch {
	case errors.Is(mapped, ErrInvestmentBuyNotFound):
		return ErrInvestmentShortNotFound
	case errors.Is(mapped, ErrInvestmentBuyAlreadyCorrected):
		return ErrInvestmentShortAlreadyCorrected
	case errors.Is(mapped, ErrInvestmentBuyChanged):
		return ErrInvestmentShortChanged
	}
	return mapped
}
