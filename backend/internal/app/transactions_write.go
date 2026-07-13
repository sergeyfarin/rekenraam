package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
)

func (s *TransactionService) CreateTransaction(ctx context.Context, input CreateTransactionInput) (Transaction, error) {
	params, err := s.prepareCreateTransactionForWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}

	record, err := s.repository.CreateTransaction(ctx, params)
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}

	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) prepareCreateTransactionForWrite(ctx context.Context, input CreateTransactionInput) (db.CreateTransactionParams, error) {
	params, err := s.prepareCreateTransaction(ctx, input)
	if err != nil {
		return db.CreateTransactionParams{}, err
	}

	refs, err := s.reconciliationInvalidationRefsFromSpec(ctx, params.Spec)
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	if len(refs) > 0 && !input.ReconciliationOverride {
		return db.CreateTransactionParams{}, ErrReconciliationOverrideRequired
	}
	if input.ReconciliationOverride {
		params.InvalidateCheckpointRefs = refs
		params.InvalidateCheckpointReason = params.ChangeReason
	}

	return params, nil
}

func (s *TransactionService) createTransactionRecordInTx(ctx context.Context, tx *sql.Tx, params db.CreateTransactionParams) (db.TransactionRecord, error) {
	record, err := s.repository.CreateTransactionInTx(ctx, tx, params)
	if err != nil {
		return db.TransactionRecord{}, mapTransactionDBError(err)
	}
	return record, nil
}

func (s *TransactionService) prepareCreateTransaction(ctx context.Context, input CreateTransactionInput) (db.CreateTransactionParams, error) {
	if input.OwnerUserID <= 0 {
		return db.CreateTransactionParams{}, ValidationError{Message: "owner user is required"}
	}

	now := s.now().UTC()
	spec, err := s.cleanTransactionSpec(ctx, input.Spec, cleanTransactionOptions{DefaultStatus: "posted"})
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "created transaction")
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	operation := strings.TrimSpace(input.Operation)
	if operation == "" {
		operation = "transaction.create"
	}

	return db.CreateTransactionParams{
		BookID:                    BookID,
		CorrectionOfTransactionID: nullableInt64(input.CorrectionOfTransactionID),
		ActorUserID:               input.OwnerUserID,
		AuthSessionID:             input.AuthSessionID,
		RequestID:                 input.RequestID,
		OriginType:                input.OriginType,
		Operation:                 operation,
		Spec:                      spec,
		CreatedAt:                 now.Format(time.RFC3339),
		ChangeReason:              changeReason,
	}, nil
}

func (s *TransactionService) UpdateTransaction(ctx context.Context, input UpdateTransactionInput) (Transaction, error) {
	if input.OwnerUserID <= 0 {
		return Transaction{}, ValidationError{Message: "owner user is required"}
	}
	if input.TransactionID <= 0 {
		return Transaction{}, ValidationError{Message: "transaction id is required"}
	}

	current, err := s.repository.TransactionByID(ctx, BookID, input.TransactionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Transaction{}, ErrTransactionNotFound
		}
		return Transaction{}, fmt.Errorf("read transaction: %w", err)
	}
	if current.Status == "voided" {
		return Transaction{}, ErrTransactionVoided
	}

	// The transaction's current lifecycle state decides the final effective
	// status, not whatever the request body's Status field claims: a draft
	// stays a draft unless explicitly promoted, and anything already posted
	// stays posted. Pinning this via ForcedStatus (rather than overwriting
	// spec.Status after the fact) ensures cleanTransactionSpec's balance
	// validation — which only runs for status=="posted" — checks the status
	// that will actually be persisted.
	promotingDraft := current.Status == "draft" && input.AllowDraftPromotion
	forcedStatus := "posted"
	if current.Status == "draft" && !promotingDraft {
		forcedStatus = "draft"
	}
	spec, err := s.cleanTransactionSpec(ctx, input.Spec, cleanTransactionOptions{
		ForcedStatus:     forcedStatus,
		ExistingLineKeys: lineKeySet(current),
		ExistingPostings: existingPostingStateSet(current),
	})
	if err != nil {
		return Transaction{}, err
	}

	var invalidationRefs []db.CheckpointInvalidationRef
	if promotingDraft {
		// Promotion changes no posting's own fields, so the diff-based guard
		// below would find nothing to flag even though every posting is
		// entering the ledger for the first time. Guard on the postings'
		// already-assigned real positions instead — the same period-scoped
		// check used for unvoid/restore, since promotion is structurally the
		// same "posting enters a reconciled period" case.
		invalidationRefs, err = s.periodScopedRefsFromRecord(ctx, current)
	} else {
		invalidationRefs, err = s.reconciliationInvalidationRefs(ctx, current, spec)
	}
	if err != nil {
		return Transaction{}, err
	}
	if len(invalidationRefs) > 0 && !input.ReconciliationOverride {
		return Transaction{}, ErrReconciliationOverrideRequired
	}

	now := s.now().UTC()
	changeReason, err := cleanChangeReason(input.ChangeReason, "updated transaction")
	if err != nil {
		return Transaction{}, err
	}
	operation := strings.TrimSpace(input.Operation)
	if operation == "" {
		operation = "transaction.update"
	}

	record, err := s.repository.UpdateTransaction(ctx, db.UpdateTransactionParams{
		BookID:                     BookID,
		TransactionID:              input.TransactionID,
		ActorUserID:                input.OwnerUserID,
		AuthSessionID:              input.AuthSessionID,
		RequestID:                  input.RequestID,
		OriginType:                 input.OriginType,
		Operation:                  operation,
		Spec:                       spec,
		RecordedAt:                 now.Format(time.RFC3339),
		ChangeReason:               changeReason,
		InvalidateCheckpointRefs:   invalidationRefs,
		InvalidateCheckpointReason: changeReason,
	})
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}

	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) PostTransaction(ctx context.Context, input PostTransactionInput) (Transaction, error) {
	if input.OwnerUserID <= 0 {
		return Transaction{}, ValidationError{Message: "owner user is required"}
	}
	current, err := s.Transaction(ctx, input.TransactionID)
	if err != nil {
		return Transaction{}, err
	}
	if current.Status == "voided" {
		return Transaction{}, ErrTransactionVoided
	}
	if current.Status == "posted" {
		return current, nil
	}

	spec := transactionInputFromTransaction(current)
	spec.Status = "posted"
	return s.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID:            input.OwnerUserID,
		AuthSessionID:          input.AuthSessionID,
		RequestID:              input.RequestID,
		OriginType:             input.OriginType,
		Operation:              "transaction.post",
		TransactionID:          input.TransactionID,
		Spec:                   spec,
		ChangeReason:           input.ChangeReason,
		AllowDraftPromotion:    true,
		ReconciliationOverride: input.ReconciliationOverride,
	})
}

func (s *TransactionService) VoidTransaction(ctx context.Context, input VoidTransactionInput) (Transaction, error) {
	if input.OwnerUserID <= 0 {
		return Transaction{}, ValidationError{Message: "owner user is required"}
	}
	if input.TransactionID <= 0 {
		return Transaction{}, ValidationError{Message: "transaction id is required"}
	}

	now := s.now().UTC()
	changeReason, err := cleanChangeReason(input.ChangeReason, "")
	if err != nil {
		return Transaction{}, err
	}
	if changeReason == "" {
		return Transaction{}, ValidationError{Message: "change reason is required"}
	}
	current, err := s.repository.TransactionByID(ctx, BookID, input.TransactionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Transaction{}, ErrTransactionNotFound
		}
		return Transaction{}, fmt.Errorf("read transaction: %w", err)
	}
	if current.Status == "draft" {
		// A draft has no ledger footprint to void, and voiding one anyway would
		// leave it permanently un-hard-deletable (DeleteDraftTransaction refuses
		// any transaction with a durable posted/voided version) with no valid
		// lifecycle path back out: unvoid would restore it to draft, but by then
		// a stale "voided" version already blocks hard delete forever. Post or
		// delete the draft instead.
		return Transaction{}, ErrTransactionDraftNotVoidable
	}
	invalidationRefs, err := s.periodScopedRefsFromRecord(ctx, current)
	if err != nil {
		return Transaction{}, err
	}
	if len(invalidationRefs) > 0 && !input.ReconciliationOverride {
		return Transaction{}, ErrReconciliationOverrideRequired
	}

	record, err := s.repository.VoidTransaction(ctx, db.VoidTransactionParams{
		BookID:                     BookID,
		TransactionID:              input.TransactionID,
		ActorUserID:                input.OwnerUserID,
		AuthSessionID:              input.AuthSessionID,
		RequestID:                  input.RequestID,
		OriginType:                 input.OriginType,
		Operation:                  "transaction.void",
		RecordedAt:                 now.Format(time.RFC3339),
		ChangeReason:               changeReason,
		InvalidateCheckpointRefs:   invalidationRefs,
		InvalidateCheckpointReason: changeReason,
	})
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}

	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) UnvoidTransaction(ctx context.Context, input TransactionLifecycleInput) (Transaction, error) {
	current, changeReason, now, err := s.prepareLifecycleChange(ctx, input, true)
	if err != nil {
		return Transaction{}, err
	}
	if current.DeletedAt != "" {
		return Transaction{}, ErrTransactionDeleted
	}
	if current.Status != "voided" {
		return current, nil
	}
	refs, err := s.periodScopedRefsFromTransaction(ctx, current)
	if err != nil {
		return Transaction{}, err
	}
	if len(refs) > 0 && !input.ReconciliationOverride {
		return Transaction{}, ErrReconciliationOverrideRequired
	}
	record, err := s.repository.UnvoidTransaction(ctx, db.TransactionLifecycleParams{
		BookID: BookID, TransactionID: input.TransactionID, ActorUserID: input.OwnerUserID,
		AuthSessionID: input.AuthSessionID, RequestID: input.RequestID, OriginType: input.OriginType,
		Operation: "transaction.unvoid", RecordedAt: now, ChangeReason: changeReason,
		InvalidateCheckpointRefs: refs, InvalidateCheckpointReason: changeReason,
	})
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}
	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) SoftDeleteTransaction(ctx context.Context, input TransactionLifecycleInput) (Transaction, error) {
	return s.setTransactionDeleted(ctx, input, true)
}

func (s *TransactionService) RestoreTransaction(ctx context.Context, input TransactionLifecycleInput) (Transaction, error) {
	return s.setTransactionDeleted(ctx, input, false)
}

func (s *TransactionService) setTransactionDeleted(ctx context.Context, input TransactionLifecycleInput, deleted bool) (Transaction, error) {
	current, changeReason, now, err := s.prepareLifecycleChange(ctx, input, true)
	if err != nil {
		return Transaction{}, err
	}
	if (current.DeletedAt != "") == deleted {
		return current, nil
	}
	if deleted && current.Status == "draft" {
		return Transaction{}, ErrTransactionPosted
	}
	refs, err := s.periodScopedRefsFromTransaction(ctx, current)
	if err != nil {
		return Transaction{}, err
	}
	// Both directions of this flag are guarded: soft-deleting removes postings
	// from a reconciled period, and restoring puts them back — either changes
	// what a completed reconciliation reflects, symmetric with unvoid's guard.
	if len(refs) > 0 && !input.ReconciliationOverride {
		return Transaction{}, ErrReconciliationOverrideRequired
	}
	operation := "transaction.restore"
	if deleted {
		operation = "transaction.soft_delete"
	}
	record, err := s.repository.SetTransactionDeleted(ctx, db.SetTransactionDeletedParams{
		TransactionLifecycleParams: db.TransactionLifecycleParams{
			BookID: BookID, TransactionID: input.TransactionID, ActorUserID: input.OwnerUserID,
			AuthSessionID: input.AuthSessionID, RequestID: input.RequestID, OriginType: input.OriginType,
			Operation: operation, RecordedAt: now, ChangeReason: changeReason,
			InvalidateCheckpointRefs: refs, InvalidateCheckpointReason: changeReason,
		}, Deleted: deleted,
	})
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}
	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) prepareLifecycleChange(ctx context.Context, input TransactionLifecycleInput, requireReason bool) (Transaction, string, string, error) {
	if input.OwnerUserID <= 0 {
		return Transaction{}, "", "", ValidationError{Message: "owner user is required"}
	}
	if input.TransactionID <= 0 {
		return Transaction{}, "", "", ValidationError{Message: "transaction id is required"}
	}
	record, err := s.repository.TransactionByIDIncludingDeleted(ctx, BookID, input.TransactionID)
	if err != nil {
		return Transaction{}, "", "", mapTransactionDBError(err)
	}
	reason, err := cleanChangeReason(input.ChangeReason, "")
	if err != nil {
		return Transaction{}, "", "", err
	}
	if requireReason && reason == "" {
		return Transaction{}, "", "", ValidationError{Message: "change reason is required"}
	}
	return toTransaction(record), reason, s.now().UTC().Format(time.RFC3339), nil
}

func (s *TransactionService) DeleteDraftTransaction(ctx context.Context, input DeleteDraftTransactionInput) error {
	if input.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}
	if input.TransactionID <= 0 {
		return ValidationError{Message: "transaction id is required"}
	}

	if err := s.repository.DeleteDraftTransaction(ctx, db.DeleteDraftTransactionParams{
		BookID:        BookID,
		TransactionID: input.TransactionID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    defaultString(input.OriginType, "browser_api"),
		Operation:     "transaction.delete_draft",
		ChangeReason:  "hard-deleted never-posted draft",
		OccurredAt:    s.now().UTC().Format(time.RFC3339),
	}); err != nil {
		return mapTransactionDBError(err)
	}

	return nil
}

func (s *TransactionService) ApproveTransaction(ctx context.Context, input ApproveTransactionInput) (Transaction, error) {
	if input.OwnerUserID <= 0 {
		return Transaction{}, ValidationError{Message: "owner user is required"}
	}
	if input.TransactionID <= 0 {
		return Transaction{}, ValidationError{Message: "transaction id is required"}
	}

	record, err := s.repository.ApproveTransaction(ctx, db.ApproveTransactionParams{
		BookID:        BookID,
		TransactionID: input.TransactionID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		Operation:     "transaction.approve",
		RecordedAt:    s.now().UTC().Format(time.RFC3339),
		ChangeReason:  input.ChangeReason,
	})
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}

	return s.enrichOne(ctx, toTransaction(record))
}
