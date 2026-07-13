package app

import (
	"context"
	"time"

	"rekenraam/backend/internal/db"
)

func (s *TransactionService) moveOp(ctx context.Context, ownerUserID int64, fn func() (db.TransactionRecord, error)) (Transaction, error) {
	if ownerUserID <= 0 {
		return Transaction{}, ValidationError{Message: "owner user is required"}
	}
	record, err := fn()
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}
	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) MoveTransaction(ctx context.Context, input MoveTransactionInput) (Transaction, error) {
	if input.TransactionID <= 0 {
		return Transaction{}, ValidationError{Message: "transaction id is required"}
	}
	if input.Direction != "earlier" && input.Direction != "later" {
		return Transaction{}, ValidationError{Message: "direction must be 'earlier' or 'later'"}
	}
	return s.moveOp(ctx, input.OwnerUserID, func() (db.TransactionRecord, error) {
		return s.repository.MoveTransaction(ctx, db.MoveTransactionParams{
			BookID:        BookID,
			TransactionID: input.TransactionID,
			Direction:     input.Direction,
			ActorUserID:   input.OwnerUserID,
			AuthSessionID: input.AuthSessionID,
			RequestID:     input.RequestID,
			OriginType:    input.OriginType,
			RecordedAt:    s.now().UTC().Format(time.RFC3339),
		})
	})
}

func (s *TransactionService) MovePosting(ctx context.Context, input MovePostingInput) (Transaction, error) {
	if input.AccountID <= 0 {
		return Transaction{}, ValidationError{Message: "account id is required"}
	}
	if input.PostingLineID <= 0 {
		return Transaction{}, ValidationError{Message: "posting line id is required"}
	}
	if input.Direction != "earlier" && input.Direction != "later" {
		return Transaction{}, ValidationError{Message: "direction must be 'earlier' or 'later'"}
	}
	return s.moveOp(ctx, input.OwnerUserID, func() (db.TransactionRecord, error) {
		return s.repository.MovePosting(ctx, db.MovePostingParams{
			BookID:                 BookID,
			AccountID:              input.AccountID,
			PostingLineID:          input.PostingLineID,
			Direction:              input.Direction,
			ActorUserID:            input.OwnerUserID,
			AuthSessionID:          input.AuthSessionID,
			RequestID:              input.RequestID,
			OriginType:             input.OriginType,
			RecordedAt:             s.now().UTC().Format(time.RFC3339),
			ReconciliationOverride: input.ReconciliationOverride,
		})
	})
}
