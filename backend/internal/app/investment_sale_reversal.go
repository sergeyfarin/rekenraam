package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/db"
)

var (
	ErrInvestmentSaleNotFound         = errors.New("investment sale operation not found")
	ErrInvestmentSaleAlreadyCorrected = errors.New("investment sale already corrected")
	ErrInvestmentImportedSale         = errors.New("imported sale requires source correction")
	ErrInvestmentSaleChanged          = errors.New("investment sale changed")
)

type ReverseInvestmentSaleInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	OriginType             string
	OperationID            int64
	TransactionID          int64
	Reason                 string
	ReconciliationOverride bool
	// GainImpactAcknowledgement echoes the preview token for the committed
	// disposal gain changes the user accepted (T-126).
	GainImpactAcknowledgement string
}

// ReverseSale posts a new inverse transaction and removes the original sale
// from the effective long-position replay. Imported fills require a committed
// source identity, which remains bound to the original operation.
func (s *InvestmentService) ReverseSale(ctx context.Context, input ReverseInvestmentSaleInput) (Transaction, error) {
	operation, params, err := s.prepareSaleReversalWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.ReverseSale(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapReverseSaleError(err)
	}
	return toTransaction(record), nil
}

// ReverseSaleReconciliationImpact runs the actual reversal writer and its
// dependent replay in a rolled-back transaction rather than planning the
// inverse journal alone, so impossible replays and gain changes surface (T-126).
func (s *InvestmentService) ReverseSaleReconciliationImpact(ctx context.Context, input ReverseInvestmentSaleInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, params, err := s.prepareSaleReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewSaleReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapReverseSaleError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareSaleReversalWrite(ctx context.Context, input ReverseInvestmentSaleInput) (db.SaleOperationRecord, db.CreateTransactionParams, error) {
	operation, planned, err := s.reverseSalePlan(ctx, input)
	if err != nil {
		return db.SaleOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, planned, nil)
	if err != nil {
		return db.SaleOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = planned.ChangeReason
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	return operation, params, nil
}

func (s *InvestmentService) reverseSalePlan(ctx context.Context, input ReverseInvestmentSaleInput) (db.SaleOperationRecord, CreateTransactionInput, error) {
	if input.OwnerUserID <= 0 || (input.OperationID <= 0 && input.TransactionID <= 0) {
		return db.SaleOperationRecord{}, CreateTransactionInput{}, ValidationError{Message: "owner and sale id are required"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return db.SaleOperationRecord{}, CreateTransactionInput{}, ValidationError{Message: "sale reversal reason is required"}
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil {
		return db.SaleOperationRecord{}, CreateTransactionInput{}, err
	}
	var operation db.SaleOperationRecord
	if input.TransactionID > 0 {
		operation, err = s.repository.SaleOperationByTransactionID(ctx, BookID, input.TransactionID)
	} else {
		operation, err = s.repository.SaleOperationByID(ctx, BookID, input.OperationID)
	}
	if err != nil {
		return db.SaleOperationRecord{}, CreateTransactionInput{}, mapReverseSaleError(err)
	}
	if operation.AlreadyCorrected {
		return db.SaleOperationRecord{}, CreateTransactionInput{}, ErrInvestmentSaleAlreadyCorrected
	}
	if operation.ImportedLineage {
		linked, err := s.repository.HasCommittedImportSource(ctx, BookID, operation.OperationID)
		if err != nil {
			return db.SaleOperationRecord{}, CreateTransactionInput{}, err
		}
		if !linked {
			return db.SaleOperationRecord{}, CreateTransactionInput{}, ErrInvestmentImportedSale
		}
	}
	original, err := s.transactionService.Transaction(ctx, operation.TransactionID)
	if err != nil {
		return db.SaleOperationRecord{}, CreateTransactionInput{}, err
	}
	if original.VersionID != operation.CurrentVersionID || original.Status != "posted" || original.TransactionKind != "investment" || original.DeletedAt != "" || original.TransactionDate != operation.EventDate {
		return db.SaleOperationRecord{}, CreateTransactionInput{}, ErrInvestmentSaleChanged
	}
	spec := invertedInvestmentTransactionSpec(original)
	spec.InvestmentOperationKind = "reversal"
	return operation, CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, OriginType: defaultString(input.OriginType, "browser_api"),
		Operation: "investment.sale.reverse", CorrectionOfTransactionID: &operation.TransactionID,
		Spec: spec, ChangeReason: reason, ReconciliationOverride: input.ReconciliationOverride,
	}, nil
}

func invertedInvestmentTransactionSpec(original Transaction) TransactionInput {
	spec := transactionInputFromTransaction(original)
	spec.ExternalRefHint = ""
	for entryIndex := range spec.JournalEntries {
		for postingIndex := range spec.JournalEntries[entryIndex].Postings {
			posting := &spec.JournalEntries[entryIndex].Postings[postingIndex]
			posting.QuantityValue = posting.QuantityValue.Negated()
		}
	}
	return spec
}

func mapReverseSaleError(err error) error {
	// Restoring sold units can change a later split's multiplied quantity or a
	// later transfer's carried basis; name that operation instead of a 500.
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) {
		return InvestmentSaleDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrInvestmentSaleNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrInvestmentSaleAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentImportedCorrection):
		return ErrInvestmentImportedSale
	case errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrInvestmentSaleChanged
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("reverse investment sale: %w", mapTransactionDBError(err))
	}
}
