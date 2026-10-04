package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/db"
)

var (
	ErrInvestmentDividendNotFound         = errors.New("investment dividend operation not found")
	ErrInvestmentDividendAlreadyCorrected = errors.New("investment dividend already corrected")
	ErrInvestmentDividendChanged          = errors.New("investment dividend changed")
	ErrInvestmentImportedDividend         = errors.New("imported dividend is no longer linked to its committed source")
)

// A cash dividend opens no lot and replays nothing (T-115). Its reversal is
// one audited inverse journal; its replacement adds the corrected dividend
// under the same audit event. Both run the shared investment writer, so a
// reconciled cash, income or withholding balance needs the explicit override
// and the original journal stays posted history.

type ReverseInvestmentDividendInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride bool
}

type ReplaceInvestmentDividendInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride bool
	Replacement            DividendInput
}

type ReplaceInvestmentDividendResult struct {
	Inverse                Transaction
	Replacement            Transaction
	CorrectedTransactionID int64
}

func (s *InvestmentService) ReverseDividend(ctx context.Context, input ReverseInvestmentDividendInput) (Transaction, error) {
	operation, params, err := s.prepareDividendReversalWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.ReverseDividend(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapDividendCorrectionError(err)
	}
	return s.transactionService.enrichOne(ctx, toTransaction(record))
}

func (s *InvestmentService) ReverseDividendReconciliationImpact(ctx context.Context, input ReverseInvestmentDividendInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, params, err := s.prepareDividendReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewDividendReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapDividendCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareDividendReversalWrite(ctx context.Context, input ReverseInvestmentDividendInput) (db.DividendOperationRecord, db.CreateTransactionParams, error) {
	operation, _, planned, err := s.dividendCorrectionPlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
		input.TransactionID, input.Reason, input.ReconciliationOverride, "investment.dividend.reverse")
	if err != nil {
		return db.DividendOperationRecord{}, db.CreateTransactionParams{}, err
	}
	planned.Spec.InvestmentOperationKind = "reversal"
	params, err := s.transactionService.prepareCreateTransactionForWriteCarrying(ctx, planned, nil)
	if err != nil {
		return db.DividendOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = planned.ChangeReason
	return operation, params, nil
}

func (s *InvestmentService) ReplaceDividend(ctx context.Context, input ReplaceInvestmentDividendInput) (ReplaceInvestmentDividendResult, error) {
	operation, inverse, replacement, err := s.prepareDividendReplacementWrite(ctx, input)
	if err != nil {
		return ReplaceInvestmentDividendResult{}, err
	}
	record, err := s.repository.ReplaceDividend(ctx, operation, inverse, replacement)
	if err != nil {
		return ReplaceInvestmentDividendResult{}, mapDividendCorrectionError(err)
	}
	inverseTransaction, err := s.transactionService.enrichOne(ctx, toTransaction(record.Inverse))
	if err != nil {
		return ReplaceInvestmentDividendResult{}, err
	}
	replacementTransaction, err := s.transactionService.enrichOne(ctx, toTransaction(record.Replacement))
	if err != nil {
		return ReplaceInvestmentDividendResult{}, err
	}
	return ReplaceInvestmentDividendResult{Inverse: inverseTransaction, Replacement: replacementTransaction,
		CorrectedTransactionID: operation.TransactionID}, nil
}

func (s *InvestmentService) ReplaceDividendReconciliationImpact(ctx context.Context, input ReplaceInvestmentDividendInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, inverse, replacement, err := s.prepareDividendReplacementWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.SimulateDividendReplacement(ctx, operation, inverse, replacement)
	if err != nil {
		return ReconciliationImpact{}, mapDividendCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

// Preview and commit freeze identical journals and components.
func (s *InvestmentService) prepareDividendReplacementWrite(ctx context.Context, input ReplaceInvestmentDividendInput) (db.DividendOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, error) {
	const operationCode = "investment.dividend.replace"
	operation, original, inversePlan, err := s.dividendCorrectionPlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
		input.TransactionID, input.Reason, input.ReconciliationOverride, operationCode)
	if err != nil {
		return db.DividendOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{}, err
	}
	replacement := input.Replacement
	// dividendPlan writes the cash leg first; that posting names the original
	// cash account and currency.
	cash := original.JournalEntries[0].Postings[0]
	if replacement.TransactionDate != operation.EventDate || replacement.CashAccountID != cash.AccountID ||
		replacement.CashCommodityID != cash.CommodityID {
		return db.DividendOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{},
			ValidationError{Message: "replacement must keep the dividend date, cash account and currency"}
	}
	replacement.OwnerUserID = input.OwnerUserID
	replacement.AuthSessionID = input.AuthSessionID
	replacement.RequestID = input.RequestID
	replacement.OriginType = "browser_api"
	replacement.Operation = operationCode
	replacement.ChangeReason = inversePlan.ChangeReason
	replacement.ReconciliationOverride = input.ReconciliationOverride
	replacement.Status = "posted"
	replacement.ExternalRefHint = ""
	replacement.MetadataJSON = ""
	inverseParams, err := s.transactionService.prepareCreateTransactionForWriteCarrying(ctx, inversePlan, nil)
	if err != nil {
		return db.DividendOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{}, err
	}
	replacementParams, err := s.prepareDividendWrite(ctx, replacement)
	if err != nil {
		return db.DividendOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{}, err
	}
	replacementParams.CorrectionOfTransactionID = inverseParams.CorrectionOfTransactionID
	replacementParams.InvestmentCorrectionOfOperationID = operation.OperationID
	replacementParams.InvestmentCorrectionMode = "replace"
	replacementParams.InvestmentCorrectionReason = inversePlan.ChangeReason
	replacementParams.CreatedAt = inverseParams.CreatedAt
	return operation, inverseParams, replacementParams, nil
}

// dividendCorrectionPlan pins the effective dividend and plans its exact
// inverse. The writer rechecks the same facts inside its transaction.
func (s *InvestmentService) dividendCorrectionPlan(ctx context.Context, ownerUserID, authSessionID int64, requestID string,
	transactionID int64, reason string, override bool, operationCode string,
) (db.DividendOperationRecord, Transaction, CreateTransactionInput, error) {
	if ownerUserID <= 0 || transactionID <= 0 {
		return db.DividendOperationRecord{}, Transaction{}, CreateTransactionInput{}, ValidationError{Message: "owner and dividend id are required"}
	}
	if strings.TrimSpace(reason) == "" {
		return db.DividendOperationRecord{}, Transaction{}, CreateTransactionInput{}, ValidationError{Message: "dividend correction reason is required"}
	}
	cleanReason, err := cleanChangeReason(reason, "")
	if err != nil {
		return db.DividendOperationRecord{}, Transaction{}, CreateTransactionInput{}, err
	}
	operation, err := s.repository.DividendOperationByTransactionID(ctx, BookID, transactionID)
	if err != nil {
		return db.DividendOperationRecord{}, Transaction{}, CreateTransactionInput{}, mapDividendCorrectionError(err)
	}
	if operation.AlreadyCorrected {
		return db.DividendOperationRecord{}, Transaction{}, CreateTransactionInput{}, ErrInvestmentDividendAlreadyCorrected
	}
	if operation.ImportedLineage {
		linked, err := s.repository.HasCommittedImportSource(ctx, BookID, operation.OperationID)
		if err != nil {
			return db.DividendOperationRecord{}, Transaction{}, CreateTransactionInput{}, err
		}
		if !linked {
			return db.DividendOperationRecord{}, Transaction{}, CreateTransactionInput{}, ErrInvestmentImportedDividend
		}
	}
	original, err := s.transactionService.Transaction(ctx, operation.TransactionID)
	if err != nil {
		return db.DividendOperationRecord{}, Transaction{}, CreateTransactionInput{}, err
	}
	if original.VersionID != operation.CurrentVersionID || original.Status != "posted" ||
		original.TransactionKind != "investment" || original.DeletedAt != "" ||
		original.TransactionDate != operation.EventDate ||
		len(original.JournalEntries) == 0 || len(original.JournalEntries[0].Postings) == 0 {
		return db.DividendOperationRecord{}, Transaction{}, CreateTransactionInput{}, ErrInvestmentDividendChanged
	}
	return operation, original, CreateTransactionInput{
		OwnerUserID: ownerUserID, AuthSessionID: authSessionID, RequestID: requestID,
		OriginType: "browser_api", Operation: operationCode,
		CorrectionOfTransactionID: &operation.TransactionID,
		Spec:                      invertedInvestmentTransactionSpec(original), ChangeReason: cleanReason,
		ReconciliationOverride: override,
	}, nil
}

func mapDividendCorrectionError(err error) error {
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrInvestmentDividendNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrInvestmentDividendAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentImportedCorrection):
		return ErrInvestmentImportedDividend
	case errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrInvestmentDividendChanged
	case errors.Is(err, db.ErrInvalidDisposalParams):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("correct investment dividend: %w", mapTransactionDBError(err))
	}
}
