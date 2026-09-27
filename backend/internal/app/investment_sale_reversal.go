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
}

// ReverseSale posts a new inverse transaction and removes the original sale
// from the effective long-position replay. Imported fills remain fenced until
// their source identity can be corrected in the same operation.
func (s *InvestmentService) ReverseSale(ctx context.Context, input ReverseInvestmentSaleInput) (Transaction, error) {
	operation, planned, err := s.reverseSalePlan(ctx, input)
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
	record, err := s.repository.ReverseSale(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapReverseSaleError(err)
	}
	return toTransaction(record), nil
}

func (s *InvestmentService) ReverseSaleReconciliationImpact(ctx context.Context, input ReverseInvestmentSaleInput) (ReconciliationImpact, error) {
	_, planned, err := s.reverseSalePlan(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return s.transactionService.investmentReconciliationImpactForCreate(ctx, CreateReconciliationImpactInput{
		OwnerUserID: input.OwnerUserID, Spec: planned.Spec,
	})
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
	if operation.Imported {
		return db.SaleOperationRecord{}, CreateTransactionInput{}, ErrInvestmentImportedSale
	}
	original, err := s.transactionService.Transaction(ctx, operation.TransactionID)
	if err != nil {
		return db.SaleOperationRecord{}, CreateTransactionInput{}, err
	}
	if original.VersionID != operation.CurrentVersionID || original.Status != "posted" || original.TransactionKind != "investment" || original.DeletedAt != "" || original.TransactionDate != operation.EventDate {
		return db.SaleOperationRecord{}, CreateTransactionInput{}, ErrInvestmentSaleChanged
	}
	spec := transactionInputFromTransaction(original)
	spec.InvestmentOperationKind = "reversal"
	spec.ExternalRefHint = ""
	for entryIndex := range spec.JournalEntries {
		for postingIndex := range spec.JournalEntries[entryIndex].Postings {
			posting := &spec.JournalEntries[entryIndex].Postings[postingIndex]
			posting.QuantityValue = posting.QuantityValue.Negated()
		}
	}
	return operation, CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, OriginType: defaultString(input.OriginType, "browser_api"),
		Operation: "investment.sale.reverse", CorrectionOfTransactionID: &operation.TransactionID,
		Spec: spec, ChangeReason: reason, ReconciliationOverride: input.ReconciliationOverride,
	}, nil
}

func mapReverseSaleError(err error) error {
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
