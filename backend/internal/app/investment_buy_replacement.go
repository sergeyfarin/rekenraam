package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/db"
)

var (
	ErrInvestmentBuyNotFound         = errors.New("investment buy operation not found")
	ErrInvestmentBuyAlreadyCorrected = errors.New("investment buy already corrected")
	ErrInvestmentImportedBuy         = errors.New("imported buy requires source correction")
	ErrInvestmentBuyChanged          = errors.New("investment buy changed")
	ErrInvestmentBuyDependency       = errors.New("investment buy cannot satisfy a later dependent operation")
)

type ReplaceInvestmentBuyInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride bool
	Replacement            InvestmentTradeInput
}

type ReplaceInvestmentBuyResult struct {
	Inverse                Transaction
	Replacement            InvestmentTradeResult
	CorrectedTransactionID int64
}

type InvestmentBuyDependencyError struct {
	OperationID int64
	DecisionID  int64
}

func (e InvestmentBuyDependencyError) Error() string {
	if e.DecisionID == 0 {
		return fmt.Sprintf("investment buy cannot satisfy later transfer operation %d", e.OperationID)
	}
	return fmt.Sprintf("investment buy cannot satisfy later operation %d disposal decision %d", e.OperationID, e.DecisionID)
}

func (e InvestmentBuyDependencyError) Unwrap() error { return ErrInvestmentBuyDependency }

// ReplaceBuy preserves the source journal and opening lot as history, then
// posts a compound correction and replays every affected long disposal.
func (s *InvestmentService) ReplaceBuy(ctx context.Context, input ReplaceInvestmentBuyInput) (ReplaceInvestmentBuyResult, error) {
	return s.replaceBuyWithPostWrite(ctx, input, nil)
}

func (s *InvestmentService) replaceBuyWithPostWrite(ctx context.Context, input ReplaceInvestmentBuyInput,
	postWrite func(*sql.Tx, int64, int64) error,
) (ReplaceInvestmentBuyResult, error) {
	return s.replaceBuyWithPostWriteOrigin(ctx, input, "browser_api", "investment.buy.replace", postWrite)
}

func (s *InvestmentService) replaceBuyWithPostWriteOrigin(ctx context.Context, input ReplaceInvestmentBuyInput,
	originType, operationCode string, postWrite func(*sql.Tx, int64, int64) error,
) (ReplaceInvestmentBuyResult, error) {
	operation, inversePlan, err := s.buyReplacementPlan(ctx, input)
	if err != nil {
		return ReplaceInvestmentBuyResult{}, err
	}
	replacement := input.Replacement
	replacement.OwnerUserID = input.OwnerUserID
	replacement.AuthSessionID = input.AuthSessionID
	replacement.RequestID = input.RequestID
	replacement.OriginType = originType
	replacement.Operation = operationCode
	replacement.ChangeReason = inversePlan.ChangeReason
	replacement.ReconciliationOverride = input.ReconciliationOverride
	if err := validateBuyReplacementTrade(replacement, operation); err != nil {
		return ReplaceInvestmentBuyResult{}, err
	}
	inversePlan.OriginType = originType
	inversePlan.Operation = operationCode
	inversePlan.ReconciliationOverride = input.ReconciliationOverride
	inverseParams, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, inversePlan, nil)
	if err != nil {
		return ReplaceInvestmentBuyResult{}, err
	}
	replacementParams, lotParams, err := s.prepareBuyWrite(ctx, replacement)
	if err != nil {
		return ReplaceInvestmentBuyResult{}, err
	}
	replacementParams.CorrectionOfTransactionID = inverseParams.CorrectionOfTransactionID
	replacementParams.InvestmentCorrectionOfOperationID = operation.OperationID
	replacementParams.InvestmentCorrectionMode = "replace"
	replacementParams.InvestmentCorrectionReason = inversePlan.ChangeReason
	replacementParams.CreatedAt = inverseParams.CreatedAt
	record, err := s.repository.ReplaceBuyWithPostWrite(ctx, operation, inverseParams, replacementParams, lotParams, postWrite)
	if err != nil {
		return ReplaceInvestmentBuyResult{}, mapBuyReplacementError(err)
	}
	lotID := record.Lot.ID
	return ReplaceInvestmentBuyResult{
		Inverse:                toTransaction(record.Inverse),
		Replacement:            InvestmentTradeResult{Transaction: toTransaction(record.Replacement), LotID: &lotID},
		CorrectedTransactionID: operation.TransactionID,
	}, nil
}

func (s *InvestmentService) ReplaceBuyReconciliationImpact(ctx context.Context, input ReplaceInvestmentBuyInput) (ReconciliationImpact, error) {
	operation, inversePlan, err := s.buyReplacementPlan(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	replacement := input.Replacement
	replacement.OwnerUserID = input.OwnerUserID
	if err := validateBuyReplacementTrade(replacement, operation); err != nil {
		return ReconciliationImpact{}, err
	}
	plan, err := s.buyPlan(ctx, replacement)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	inverseImpact, err := s.transactionService.investmentReconciliationImpactForCreate(ctx,
		CreateReconciliationImpactInput{OwnerUserID: input.OwnerUserID, Spec: inversePlan.Spec})
	if err != nil {
		return ReconciliationImpact{}, err
	}
	replacementImpact, err := s.transactionService.investmentReconciliationImpactForCreate(ctx,
		CreateReconciliationImpactInput{OwnerUserID: input.OwnerUserID, Spec: plan.Create.Spec})
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return mergeInvestmentCorrectionImpacts(inverseImpact, replacementImpact), nil
}

func (s *InvestmentService) buyReplacementPlan(ctx context.Context, input ReplaceInvestmentBuyInput) (db.BuyOperationRecord, CreateTransactionInput, error) {
	if input.OwnerUserID <= 0 || input.TransactionID <= 0 {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, ValidationError{Message: "owner and buy id are required"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, ValidationError{Message: "buy replacement reason is required"}
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, err
	}
	operation, err := s.repository.BuyOperationByTransactionID(ctx, BookID, input.TransactionID)
	if err != nil {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, mapBuyReplacementError(err)
	}
	if operation.AlreadyCorrected {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, ErrInvestmentBuyAlreadyCorrected
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
	original, err := s.transactionService.Transaction(ctx, operation.TransactionID)
	if err != nil {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, err
	}
	if original.VersionID != operation.CurrentVersionID || original.Status != "posted" ||
		original.TransactionKind != "investment" || original.DeletedAt != "" ||
		original.TransactionDate != operation.EventDate {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, ErrInvestmentBuyChanged
	}
	return operation, CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, OriginType: "browser_api",
		Operation: "investment.buy.replace", CorrectionOfTransactionID: &operation.TransactionID,
		Spec: invertedInvestmentTransactionSpec(original), ChangeReason: reason,
		ReconciliationOverride: input.ReconciliationOverride,
	}, nil
}

func validateBuyReplacementTrade(replacement InvestmentTradeInput, operation db.BuyOperationRecord) error {
	if replacement.TransactionDate != operation.EventDate ||
		replacement.HoldingAccountID != operation.AccountID ||
		replacement.CommodityID != operation.CommodityID ||
		replacement.CashCommodityID != operation.CostCommodityID {
		return ValidationError{Message: "replacement must keep the buy date, holding account, instrument and cost currency"}
	}
	for index, charge := range replacement.Charges {
		if charge.Treatment == "" {
			return ValidationError{Message: fmt.Sprintf("replacement charge %d treatment is required", index+1)}
		}
	}
	return nil
}

func mapBuyReplacementError(err error) error {
	var dependency *db.InvestmentReplayDependencyError
	if errors.Is(err, db.ErrInvestmentCorrectionDependency) && errors.As(err, &dependency) {
		return InvestmentBuyDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrInvestmentBuyNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrInvestmentBuyAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentImportedCorrection):
		return ErrInvestmentImportedBuy
	case errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrInvestmentBuyChanged
	case errors.Is(err, db.ErrInvestmentCorrectionDependency):
		return ErrInvestmentBuyDependency
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("replace investment buy: %w", mapTransactionDBError(err))
	}
}
