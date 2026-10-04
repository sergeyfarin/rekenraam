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
