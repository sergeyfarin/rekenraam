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

var ErrTransactionDraftNotUserCreatable = errors.New("draft transactions can only be created by a system producer")

func (s *TransactionService) CreateTransaction(ctx context.Context, input CreateTransactionInput) (Transaction, error) {
	if input.CorrectionOfTransactionID != nil {
		if err := s.rejectInvestmentLinkedMutation(ctx, *input.CorrectionOfTransactionID); err != nil {
			return Transaction{}, err
		}
	}
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

// rejectIfCheckpointAlreadyInTheWay fails a write before it starts when a
// checkpoint is visibly in its way and no override was granted. It is not the
// guard — enforceCheckpointBoundaryTx is, inside the write's own transaction
// (T-94) — but refusing here keeps the common case cheap and gives the API the
// same error without a rollback.
func (s *TransactionService) rejectIfCheckpointAlreadyInTheWay(ctx context.Context, candidates []db.PeriodScopedCheckpointRef, override bool) error {
	if override || len(candidates) == 0 {
		return nil
	}
	refs, err := s.resolveCheckpointRefs(ctx, candidates)
	if err != nil {
		return err
	}
	if len(refs) > 0 {
		return ErrReconciliationOverrideRequired
	}
	return nil
}

func (s *TransactionService) prepareCreateTransactionForWrite(ctx context.Context, input CreateTransactionInput) (db.CreateTransactionParams, error) {
	return s.prepareCreateTransactionForWriteWithOptions(ctx, input, cleanTransactionOptions{})
}

// prepareInvestmentTransactionForWrite is the investment subledger's entry into
// the shared transaction write path, and the only way a posting to a
// subledger-managed holding account gets past cleanPosting's T-96 guard. It is
// unexported and takes no caller-supplied flag, so the exemption cannot be
// requested from the API layer or set by populating a request field — a caller
// either is the investment service or it is not. Every use of it must write the
// matching lot facts in the same database transaction as the journal.
func (s *TransactionService) prepareInvestmentTransactionForWrite(ctx context.Context, input CreateTransactionInput) (db.CreateTransactionParams, error) {
	return s.prepareCreateTransactionForWriteWithOptions(ctx, input, cleanTransactionOptions{AllowSubledgerManagedPostings: true})
}

func (s *TransactionService) prepareCreateTransactionForWriteWithOptions(ctx context.Context, input CreateTransactionInput, options cleanTransactionOptions) (db.CreateTransactionParams, error) {
	params, err := s.prepareCreateTransaction(ctx, input, options)
	if err != nil {
		return db.CreateTransactionParams{}, err
	}

	// The write transaction re-resolves these candidates and applies the guard
	// itself (T-94), so this early check is a courtesy, not the boundary: it
	// fails the request before any work is done when a checkpoint is already in
	// the way, and it is what the import loop reads to mark a row skipped. A
	// checkpoint that appears after this point is caught at commit instead.
	candidates := reconciliationCandidatesFromSpec(params.Spec)
	if !input.ReconciliationOverride {
		refs, err := s.resolveCheckpointRefs(ctx, candidates)
		if err != nil {
			return db.CreateTransactionParams{}, err
		}
		if len(refs) > 0 {
			return db.CreateTransactionParams{}, ErrReconciliationOverrideRequired
		}
	}
	params.CheckpointCandidates = candidates
	params.ReconciliationOverride = input.ReconciliationOverride
	params.InvalidateCheckpointReason = params.ChangeReason

	return params, nil
}

func (s *TransactionService) createTransactionRecordInTx(ctx context.Context, tx *sql.Tx, params db.CreateTransactionParams) (db.TransactionRecord, error) {
	record, err := s.repository.CreateTransactionInTx(ctx, tx, params)
	if err != nil {
		return db.TransactionRecord{}, mapTransactionDBError(err)
	}
	return record, nil
}

func (s *TransactionService) prepareCreateTransaction(ctx context.Context, input CreateTransactionInput, options cleanTransactionOptions) (db.CreateTransactionParams, error) {
	if input.OwnerUserID <= 0 {
		return db.CreateTransactionParams{}, ValidationError{Message: "owner user is required"}
	}
	if strings.TrimSpace(input.Spec.Status) == "draft" && strings.TrimSpace(input.OriginType) == "browser_api" {
		return db.CreateTransactionParams{}, ErrTransactionDraftNotUserCreatable
	}

	now := s.now().UTC()
	options.DefaultStatus = "posted"
	spec, err := s.cleanTransactionSpec(ctx, input.Spec, options)
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
	if err := s.rejectInvestmentLinkedMutation(ctx, input.TransactionID); err != nil {
		return Transaction{}, err
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

	var candidates []db.PeriodScopedCheckpointRef
	if promotingDraft {
		// Promotion changes no posting's own fields, so the diff-based guard
		// below would find nothing to flag even though every posting is
		// entering the ledger for the first time. Guard on the postings'
		// already-assigned real positions instead — the same period-scoped
		// check used for unvoid/restore, since promotion is structurally the
		// same "posting enters a reconciled period" case.
		candidates = periodScopedCandidatesFromRecord(current)
	} else {
		candidates = reconciliationCandidates(current, spec)
	}
	if err := s.rejectIfCheckpointAlreadyInTheWay(ctx, candidates, input.ReconciliationOverride); err != nil {
		return Transaction{}, err
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
		CheckpointCandidates:       candidates,
		ReconciliationOverride:     input.ReconciliationOverride,
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
	if err := s.rejectInvestmentLinkedMutation(ctx, input.TransactionID); err != nil {
		return Transaction{}, err
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
	if err := s.rejectInvestmentLinkedMutation(ctx, input.TransactionID); err != nil {
		return Transaction{}, err
	}
	candidates := periodScopedCandidatesFromRecord(current)
	if err := s.rejectIfCheckpointAlreadyInTheWay(ctx, candidates, input.ReconciliationOverride); err != nil {
		return Transaction{}, err
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
		CheckpointCandidates:       candidates,
		ReconciliationOverride:     input.ReconciliationOverride,
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
	candidates := periodScopedCandidatesFromTransaction(current)
	if err := s.rejectIfCheckpointAlreadyInTheWay(ctx, candidates, input.ReconciliationOverride); err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.UnvoidTransaction(ctx, db.TransactionLifecycleParams{
		BookID: BookID, TransactionID: input.TransactionID, ActorUserID: input.OwnerUserID,
		AuthSessionID: input.AuthSessionID, RequestID: input.RequestID, OriginType: input.OriginType,
		Operation: "transaction.unvoid", RecordedAt: now, ChangeReason: changeReason,
		CheckpointCandidates: candidates, ReconciliationOverride: input.ReconciliationOverride,
		InvalidateCheckpointReason: changeReason,
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
	// Both directions of this flag are guarded: soft-deleting removes postings
	// from a reconciled period, and restoring puts them back — either changes
	// what a completed reconciliation reflects, symmetric with unvoid's guard.
	candidates := periodScopedCandidatesFromTransaction(current)
	if err := s.rejectIfCheckpointAlreadyInTheWay(ctx, candidates, input.ReconciliationOverride); err != nil {
		return Transaction{}, err
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
			CheckpointCandidates: candidates, ReconciliationOverride: input.ReconciliationOverride,
			InvalidateCheckpointReason: changeReason,
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
	if err := s.rejectInvestmentLinkedMutation(ctx, input.TransactionID); err != nil {
		return Transaction{}, "", "", err
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
	if err := s.rejectInvestmentLinkedMutation(ctx, input.TransactionID); err != nil {
		return err
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

func (s *TransactionService) rejectInvestmentLinkedMutation(ctx context.Context, transactionID int64) error {
	linked, err := s.repository.TransactionHasInvestmentLinks(ctx, BookID, transactionID)
	if err != nil {
		return err
	}
	if linked {
		return ErrInvestmentWorkflowRequired
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
