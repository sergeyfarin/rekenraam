package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/db"
)

// T-119: an internal or external-in transfer is corrected by its own
// commands. A reversal posts the exact inverse journal and replays every
// position the transfer moved in one transaction; downstream positions follow
// through transfer propagation. The original journal, facts, links and lot
// events stay immutable evidence, changed gains need the exact
// acknowledgement and a reconciled balance needs the explicit override.

var (
	ErrInvestmentTransferNotFound         = errors.New("investment transfer operation not found")
	ErrInvestmentTransferAlreadyCorrected = errors.New("investment transfer already corrected")
	ErrInvestmentTransferChanged          = errors.New("investment transfer changed")
	ErrInvestmentImportedTransfer         = errors.New("transfer linked to an import row no longer names its committed source")
	ErrInvestmentTransferDependency       = errors.New("investment transfer correction cannot satisfy a dependent operation")
)

// InvestmentTransferDependencyError names the later operation a transfer
// correction would make impossible: a destination sale of units that would no
// longer arrive, or an onward transfer of a removed destination lot.
type InvestmentTransferDependencyError struct {
	OperationID int64
	DecisionID  int64
}

func (e InvestmentTransferDependencyError) Error() string {
	if e.DecisionID == 0 {
		return fmt.Sprintf("investment transfer correction cannot satisfy later operation %d", e.OperationID)
	}
	return fmt.Sprintf("investment transfer correction cannot satisfy later operation %d disposal decision %d", e.OperationID, e.DecisionID)
}

func (e InvestmentTransferDependencyError) Unwrap() error { return ErrInvestmentTransferDependency }

type ReverseInvestmentTransferInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
}

// ReverseTransfer removes a posted internal or external-in transfer: the
// source (internal) gets its units back, the destination loses the lots the
// transfer opened, and later decisions on both replay.
func (s *InvestmentService) ReverseTransfer(ctx context.Context, input ReverseInvestmentTransferInput) (Transaction, error) {
	operation, params, err := s.prepareTransferReversalWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.ReverseTransfer(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapTransferCorrectionError(err)
	}
	return toTransaction(record), nil
}

// ReverseTransferReconciliationImpact runs the reversal writer and rolls back.
func (s *InvestmentService) ReverseTransferReconciliationImpact(ctx context.Context, input ReverseInvestmentTransferInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, params, err := s.prepareTransferReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewTransferReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapTransferCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareTransferReversalWrite(ctx context.Context, input ReverseInvestmentTransferInput) (db.TransferOperationRecord, db.CreateTransactionParams, error) {
	operation, planned, err := s.transferCorrectionPlan(ctx, input.OwnerUserID, input.TransactionID, input.Reason)
	if err != nil {
		return db.TransferOperationRecord{}, db.CreateTransactionParams{}, err
	}
	planned.AuthSessionID, planned.RequestID = input.AuthSessionID, input.RequestID
	planned.ReconciliationOverride = input.ReconciliationOverride
	planned.Operation = "investment.transfer.reverse"
	planned.Spec.InvestmentOperationKind = "reversal"
	params, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, planned, nil)
	if err != nil {
		return db.TransferOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = planned.ChangeReason
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	return operation, params, nil
}

// transferCorrectionPlan pins the effective transfer and plans its exact
// inverse journal. The writer rechecks the same facts in its transaction.
func (s *InvestmentService) transferCorrectionPlan(ctx context.Context, ownerUserID, transactionID int64, reason string) (db.TransferOperationRecord, CreateTransactionInput, error) {
	if ownerUserID <= 0 || transactionID <= 0 {
		return db.TransferOperationRecord{}, CreateTransactionInput{}, ValidationError{Message: "owner and transfer id are required"}
	}
	if strings.TrimSpace(reason) == "" {
		return db.TransferOperationRecord{}, CreateTransactionInput{}, ValidationError{Message: "transfer correction reason is required"}
	}
	reason, err := cleanChangeReason(reason, "")
	if err != nil {
		return db.TransferOperationRecord{}, CreateTransactionInput{}, err
	}
	operation, err := s.repository.TransferOperationByTransactionID(ctx, BookID, transactionID)
	if err != nil {
		return db.TransferOperationRecord{}, CreateTransactionInput{}, mapTransferCorrectionError(err)
	}
	if operation.AlreadyCorrected {
		return db.TransferOperationRecord{}, CreateTransactionInput{}, ErrInvestmentTransferAlreadyCorrected
	}
	if operation.ImportedLineage {
		linked, err := s.repository.HasCommittedImportSource(ctx, BookID, operation.OperationID)
		if err != nil {
			return db.TransferOperationRecord{}, CreateTransactionInput{}, err
		}
		if !linked {
			return db.TransferOperationRecord{}, CreateTransactionInput{}, ErrInvestmentImportedTransfer
		}
	}
	original, err := s.transactionService.Transaction(ctx, operation.TransactionID)
	if err != nil {
		return db.TransferOperationRecord{}, CreateTransactionInput{}, err
	}
	if original.VersionID != operation.CurrentVersionID || original.Status != "posted" ||
		original.TransactionKind != "investment" || original.DeletedAt != "" ||
		original.TransactionDate != operation.EventDate {
		return db.TransferOperationRecord{}, CreateTransactionInput{}, ErrInvestmentTransferChanged
	}
	return operation, CreateTransactionInput{
		OwnerUserID: ownerUserID, OriginType: "browser_api",
		CorrectionOfTransactionID: &operation.TransactionID,
		Spec:                      invertedInvestmentTransactionSpec(original), ChangeReason: reason,
	}, nil
}

func mapTransferCorrectionError(err error) error {
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) {
		return InvestmentTransferDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrInvestmentTransferNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrInvestmentTransferAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentImportedCorrection):
		return ErrInvestmentImportedTransfer
	case errors.Is(err, db.ErrInvestmentTransferChanged), errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrInvestmentTransferChanged
	case errors.Is(err, db.ErrInvestmentCorrectionDependency):
		return ErrInvestmentTransferDependency
	case errors.Is(err, db.ErrOutOfOrderPositionEvent),
		errors.Is(err, db.ErrGainImpactAcknowledgementRequired),
		errors.Is(err, db.ErrGainImpactAcknowledgementStale):
		return err
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("correct investment transfer: %w", mapTransactionDBError(err))
	}
}

type ReplaceInvestmentTransferInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
	// Replacement is the full corrected internal transfer. Date, quantity or
	// lots, both accounts and destination lineage may change; the security
	// and its cost currency stay those of the transfer.
	Replacement InternalTransferInput
}

type ReplaceInvestmentTransferResult struct {
	Inverse                Transaction
	Replacement            InternalTransferResult
	CorrectedTransactionID int64
}

type preparedTransferReplacement struct {
	operation   db.TransferOperationRecord
	inverse     db.CreateTransactionParams
	replacement db.CreateTransactionParams
	transfer    db.CreateInternalTransferParams
}

// ReplaceTransfer posts the inverse and a corrected internal transfer at the
// replaced transfer's slot under one audit event, replaying every position
// either transfer moved. Re-recording a source_lots transfer from an
// average-cost source as one pooled lot is the remedy for a lineage refusal.
func (s *InvestmentService) ReplaceTransfer(ctx context.Context, input ReplaceInvestmentTransferInput) (ReplaceInvestmentTransferResult, error) {
	prepared, err := s.prepareTransferReplacement(ctx, input)
	if err != nil {
		return ReplaceInvestmentTransferResult{}, err
	}
	record, err := s.repository.ReplaceInternalTransfer(ctx, prepared.operation, prepared.inverse, prepared.replacement, prepared.transfer)
	if err != nil {
		return ReplaceInvestmentTransferResult{}, mapTransferReplacementError(err)
	}
	return ReplaceInvestmentTransferResult{
		Inverse: toTransaction(record.Inverse),
		Replacement: InternalTransferResult{Transaction: toTransaction(record.Replacement),
			Plan: toInternalTransferPlan(record.Result), DestinationLotIDs: record.Result.DestinationLotIDs},
		CorrectedTransactionID: prepared.operation.TransactionID,
	}, nil
}

// PreviewTransferReplacement runs the replacement writer and every replay,
// then rolls back: the plan the replacement would carry and its impact.
func (s *InvestmentService) PreviewTransferReplacement(ctx context.Context, input ReplaceInvestmentTransferInput) (InternalTransferPreview, error) {
	input.ReconciliationOverride = true
	prepared, err := s.prepareTransferReplacement(ctx, input)
	if err != nil {
		return InternalTransferPreview{}, err
	}
	simulated, result, err := s.repository.SimulateInternalTransferReplacement(ctx, prepared.operation,
		prepared.inverse, prepared.replacement, prepared.transfer)
	if err != nil {
		return InternalTransferPreview{}, mapTransferReplacementError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return InternalTransferPreview{}, err
	}
	return InternalTransferPreview{Plan: toInternalTransferPlan(result), Impact: impact}, nil
}

func (s *InvestmentService) prepareTransferReplacement(ctx context.Context, input ReplaceInvestmentTransferInput) (preparedTransferReplacement, error) {
	operation, inversePlan, err := s.transferCorrectionPlan(ctx, input.OwnerUserID, input.TransactionID, input.Reason)
	if err != nil {
		return preparedTransferReplacement{}, err
	}
	if operation.TransferKind != "internal" {
		return preparedTransferReplacement{}, ErrInvestmentTransferNotFound
	}
	replacement := input.Replacement
	replacement.OwnerUserID, replacement.AuthSessionID, replacement.RequestID =
		input.OwnerUserID, input.AuthSessionID, input.RequestID
	replacement.ChangeReason, replacement.ReconciliationOverride = inversePlan.ChangeReason, input.ReconciliationOverride
	if replacement.CommodityID != operation.CommodityID || replacement.CostCommodityID != operation.CostCommodityID {
		return preparedTransferReplacement{}, ValidationError{Message: "a transfer replacement keeps the security and its basis currency"}
	}
	plan, transfer, err := s.internalTransferPlan(ctx, replacement)
	if err != nil {
		return preparedTransferReplacement{}, err
	}
	inversePlan.AuthSessionID, inversePlan.RequestID = input.AuthSessionID, input.RequestID
	inversePlan.Operation = "investment.transfer.replace"
	inversePlan.ReconciliationOverride = input.ReconciliationOverride
	inversePlan.Spec.InvestmentOperationKind = ""
	inverse, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, inversePlan, nil)
	if err != nil {
		return preparedTransferReplacement{}, err
	}
	plan.Create.Operation = "investment.transfer.replace"
	replacementParams, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return preparedTransferReplacement{}, err
	}
	replacementParams.CorrectionOfTransactionID = inverse.CorrectionOfTransactionID
	replacementParams.InvestmentCorrectionOfOperationID = operation.OperationID
	replacementParams.InvestmentCorrectionMode = "replace"
	replacementParams.InvestmentCorrectionReason = inversePlan.ChangeReason
	replacementParams.CreatedAt = inverse.CreatedAt
	// The compound command's first journal carries the disclosure policy.
	inverse.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	replacementParams.GainImpact = nil
	return preparedTransferReplacement{operation: operation, inverse: inverse,
		replacement: replacementParams, transfer: transfer}, nil
}

// mapTransferReplacementError adds the replacement's own allocation refusals
// to the shared correction errors.
func mapTransferReplacementError(err error) error {
	var dependency *db.InvestmentReplayDependencyError
	switch {
	case errors.As(err, &dependency):
		return mapTransferCorrectionError(err)
	// The transfer was pinned before the write, so a missing lot inside it is
	// the replacement's own selection, not a missing transfer.
	case errors.Is(err, db.ErrInsufficientLots), errors.Is(err, db.ErrNotFound),
		errors.Is(err, db.ErrAverageCostTransferRequiresPoolAllocation),
		errors.Is(err, db.ErrPooledTransferRequiresAverageCost),
		errors.Is(err, db.ErrInvalidTransferDestinationLineage),
		errors.Is(err, db.ErrUnknownInvestmentBasis):
		return mapInternalTransferError(err)
	}
	return mapTransferCorrectionError(err)
}

type ReplaceInvestmentTransferInInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
	// Replacement is the full corrected external transfer in. Date, holding,
	// quantity, carried basis, original date and evidence may change; the
	// security and basis currency stay those of the transfer.
	Replacement ExternalTransferInInput
}

type ReplaceInvestmentTransferInResult struct {
	Inverse                Transaction
	Replacement            InvestmentTradeResult
	CorrectedTransactionID int64
}

type preparedTransferInReplacement struct {
	operation   db.TransferOperationRecord
	inverse     db.CreateTransactionParams
	replacement db.CreateTransactionParams
	transfer    db.CreateExternalTransferInParams
}

// ReplaceTransferIn posts the inverse (bridge included) and a corrected
// external transfer in under one audit event. The bridge difference is
// appended, never overwritten; the new lot takes the replaced transfer's
// same-day slot and both holdings replay.
func (s *InvestmentService) ReplaceTransferIn(ctx context.Context, input ReplaceInvestmentTransferInInput) (ReplaceInvestmentTransferInResult, error) {
	prepared, err := s.prepareTransferInReplacement(ctx, input)
	if err != nil {
		return ReplaceInvestmentTransferInResult{}, err
	}
	record, err := s.repository.ReplaceExternalTransferIn(ctx, prepared.operation, prepared.inverse,
		prepared.replacement, prepared.transfer)
	if err != nil {
		return ReplaceInvestmentTransferInResult{}, mapTransferCorrectionError(err)
	}
	lotID := record.Lot.ID
	return ReplaceInvestmentTransferInResult{
		Inverse:                toTransaction(record.Inverse),
		Replacement:            InvestmentTradeResult{Transaction: toTransaction(record.Replacement), LotID: &lotID},
		CorrectedTransactionID: prepared.operation.TransactionID,
	}, nil
}

// ReplaceTransferInReconciliationImpact runs the replacement writer and
// every replay, then rolls back.
func (s *InvestmentService) ReplaceTransferInReconciliationImpact(ctx context.Context, input ReplaceInvestmentTransferInInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	prepared, err := s.prepareTransferInReplacement(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.SimulateExternalTransferInReplacement(ctx, prepared.operation, prepared.inverse,
		prepared.replacement, prepared.transfer)
	if err != nil {
		return ReconciliationImpact{}, mapTransferCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareTransferInReplacement(ctx context.Context, input ReplaceInvestmentTransferInInput) (preparedTransferInReplacement, error) {
	operation, inversePlan, err := s.transferCorrectionPlan(ctx, input.OwnerUserID, input.TransactionID, input.Reason)
	if err != nil {
		return preparedTransferInReplacement{}, err
	}
	if operation.TransferKind != "external_in" {
		return preparedTransferInReplacement{}, ErrInvestmentTransferNotFound
	}
	replacement := input.Replacement
	replacement.OwnerUserID, replacement.AuthSessionID, replacement.RequestID =
		input.OwnerUserID, input.AuthSessionID, input.RequestID
	replacement.ChangeReason, replacement.ReconciliationOverride = inversePlan.ChangeReason, input.ReconciliationOverride
	if replacement.CommodityID != operation.CommodityID || replacement.CostCommodityID != operation.CostCommodityID {
		return preparedTransferInReplacement{}, ValidationError{Message: "a transfer replacement keeps the security and its basis currency"}
	}
	inversePlan.AuthSessionID, inversePlan.RequestID = input.AuthSessionID, input.RequestID
	inversePlan.Operation = "investment.transfer_in.replace"
	inversePlan.ReconciliationOverride = input.ReconciliationOverride
	inversePlan.Spec.InvestmentOperationKind = ""
	inverse, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, inversePlan, nil)
	if err != nil {
		return preparedTransferInReplacement{}, err
	}
	journal, transfer, err := s.externalTransferInWrite(ctx, replacement)
	if err != nil {
		return preparedTransferInReplacement{}, err
	}
	journal.Operation = "investment.transfer_in.replace"
	journal.CorrectionOfTransactionID = inverse.CorrectionOfTransactionID
	journal.InvestmentCorrectionOfOperationID = operation.OperationID
	journal.InvestmentCorrectionMode = "replace"
	journal.InvestmentCorrectionReason = inversePlan.ChangeReason
	journal.CreatedAt = inverse.CreatedAt
	transfer.Lot.CreatedAt = inverse.CreatedAt
	// The compound command's first journal carries the disclosure policy.
	inverse.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	journal.GainImpact = nil
	return preparedTransferInReplacement{operation: operation, inverse: inverse, replacement: journal, transfer: transfer}, nil
}
