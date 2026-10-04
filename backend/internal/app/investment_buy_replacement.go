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
	// GainImpactAcknowledgement echoes the preview token for the committed
	// disposal gain changes the user accepted (T-126).
	GainImpactAcknowledgement string
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
		return fmt.Sprintf("investment buy cannot satisfy later transfer or split operation %d", e.OperationID)
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
	prepared, err := s.prepareBuyReplacementWrite(ctx, input, originType, operationCode)
	if err != nil {
		return ReplaceInvestmentBuyResult{}, err
	}
	record, err := s.repository.ReplaceBuyWithPostWrite(ctx, prepared.Operation, prepared.Inverse, prepared.Replacement, prepared.Lot, postWrite)
	if err != nil {
		return ReplaceInvestmentBuyResult{}, mapBuyReplacementError(err)
	}
	lotID := record.Lot.ID
	return ReplaceInvestmentBuyResult{
		Inverse:                toTransaction(record.Inverse),
		Replacement:            InvestmentTradeResult{Transaction: toTransaction(record.Replacement), LotID: &lotID},
		CorrectedTransactionID: prepared.Operation.TransactionID,
	}, nil
}

type preparedBuyReplacementWrite struct {
	Operation   db.BuyOperationRecord
	Inverse     db.CreateTransactionParams
	Replacement db.CreateTransactionParams
	Lot         db.CreateInvestmentLotParams
}

// Preview and commit freeze identical journal, source-economics and lot facts.
func (s *InvestmentService) prepareBuyReplacementWrite(ctx context.Context, input ReplaceInvestmentBuyInput,
	originType, operationCode string,
) (preparedBuyReplacementWrite, error) {
	operation, inversePlan, err := s.buyReplacementPlan(ctx, input)
	if err != nil {
		return preparedBuyReplacementWrite{}, err
	}
	replacement := input.Replacement
	replacement.OwnerUserID = input.OwnerUserID
	replacement.AuthSessionID = input.AuthSessionID
	replacement.RequestID = input.RequestID
	replacement.OriginType = originType
	replacement.Operation = operationCode
	replacement.ChangeReason = inversePlan.ChangeReason
	replacement.ReconciliationOverride = input.ReconciliationOverride
	if err := validateBuyReplacementTrade(replacement); err != nil {
		return preparedBuyReplacementWrite{}, err
	}
	inversePlan.OriginType = originType
	inversePlan.Operation = operationCode
	inversePlan.ReconciliationOverride = input.ReconciliationOverride
	inverseParams, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, inversePlan, nil)
	if err != nil {
		return preparedBuyReplacementWrite{}, err
	}
	replacementParams, lotParams, err := s.prepareBuyWrite(ctx, replacement)
	if err != nil {
		return preparedBuyReplacementWrite{}, err
	}
	replacementParams.CorrectionOfTransactionID = inverseParams.CorrectionOfTransactionID
	replacementParams.InvestmentCorrectionOfOperationID = operation.OperationID
	replacementParams.InvestmentCorrectionMode = "replace"
	replacementParams.InvestmentCorrectionReason = inversePlan.ChangeReason
	replacementParams.CreatedAt = inverseParams.CreatedAt
	// The compound command's first journal carries the disclosure policy.
	inverseParams.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	replacementParams.GainImpact = nil
	return preparedBuyReplacementWrite{Operation: operation, Inverse: inverseParams, Replacement: replacementParams, Lot: lotParams}, nil
}

func (s *InvestmentService) ReplaceBuyReconciliationImpact(ctx context.Context, input ReplaceInvestmentBuyInput) (ReconciliationImpact, error) {
	// A preview can examine reconciled periods without the owner authorizing a
	// write. Simulation and all checkpoint changes are rolled back together.
	input.ReconciliationOverride = true
	prepared, err := s.prepareBuyReplacementWrite(ctx, input, "browser_api", "investment.buy.replace")
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.SimulateBuyReplacement(ctx, prepared.Operation, prepared.Inverse, prepared.Replacement, prepared.Lot)
	if err != nil {
		return ReconciliationImpact{}, mapBuyReplacementError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) buyReplacementPlan(ctx context.Context, input ReplaceInvestmentBuyInput) (db.BuyOperationRecord, CreateTransactionInput, error) {
	return s.acquisitionCorrectionPlan(ctx, acquisitionCorrectionRequest{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		TransactionID: input.TransactionID, Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
		OperationKind: "buy", Operation: "investment.buy.replace", Noun: "buy",
	})
}

// acquisitionCorrectionRequest names the posted acquisition a reversal or
// replacement starts from. OperationKind fences each command to its own
// family: a buy command never corrects a reinvested dividend, and vice versa.
type acquisitionCorrectionRequest struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride bool
	OperationKind          string
	Operation              string
	Noun                   string
}

// acquisitionCorrectionPlan pins the effective acquisition and plans its
// exact inverse journal. The writer rechecks the same facts in its transaction.
func (s *InvestmentService) acquisitionCorrectionPlan(ctx context.Context, input acquisitionCorrectionRequest) (db.BuyOperationRecord, CreateTransactionInput, error) {
	if input.OwnerUserID <= 0 || input.TransactionID <= 0 {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, ValidationError{Message: "owner and " + input.Noun + " id are required"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, ValidationError{Message: input.Noun + " correction reason is required"}
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, err
	}
	notFound, alreadyCorrected, changed := ErrInvestmentBuyNotFound, ErrInvestmentBuyAlreadyCorrected, ErrInvestmentBuyChanged
	if input.OperationKind == "reinvested_dividend" {
		notFound, alreadyCorrected, changed = ErrInvestmentReinvestmentNotFound, ErrInvestmentReinvestmentAlreadyCorrected, ErrInvestmentReinvestmentChanged
	}
	operation, err := s.repository.BuyOperationByTransactionID(ctx, BookID, input.TransactionID)
	if errors.Is(err, db.ErrNotFound) || (err == nil && operation.OperationKind != input.OperationKind) {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, notFound
	}
	if err != nil {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, mapBuyReplacementError(err)
	}
	if operation.AlreadyCorrected {
		return db.BuyOperationRecord{}, CreateTransactionInput{}, alreadyCorrected
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
		return db.BuyOperationRecord{}, CreateTransactionInput{}, changed
	}
	return operation, CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, OriginType: "browser_api",
		Operation: input.Operation, CorrectionOfTransactionID: &operation.TransactionID,
		Spec: invertedInvestmentTransactionSpec(original), ChangeReason: reason,
		ReconciliationOverride: input.ReconciliationOverride,
	}, nil
}

// validateBuyReplacementTrade requires explicit economic elections. The trade
// date, holding account, instrument and cost currency may change (T-116): the
// writer replays the source and the new position together.
func validateBuyReplacementTrade(replacement InvestmentTradeInput) error {
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
