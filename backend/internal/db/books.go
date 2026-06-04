package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrBookExists = errors.New("book already exists")

type BookRepository struct {
	database *sql.DB
}

type BookRecord struct {
	ID                         int64
	OwnerUserID                int64
	Code                       string
	Name                       string
	DefaultCurrencyCommodityID sql.NullInt64
	UpdatedByUserID            sql.NullInt64
	CreatedAt                  string
	UpdatedAt                  string
}

type CompleteBookSetupParams struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Code          string
	Name          string
	CreatedAt     string
}

func NewBookRepository(database *sql.DB) *BookRepository {
	return &BookRepository{database: database}
}

func (r *BookRepository) CurrentBook(ctx context.Context) (BookRecord, error) {
	var record BookRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT id, owner_user_id, code, name, default_currency_commodity_id, updated_by_user_id, created_at, updated_at
		FROM books
		WHERE id = 1
	`).Scan(
		&record.ID,
		&record.OwnerUserID,
		&record.Code,
		&record.Name,
		&record.DefaultCurrencyCommodityID,
		&record.UpdatedByUserID,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BookRecord{}, ErrNotFound
		}
		return BookRecord{}, fmt.Errorf("read current book: %w", err)
	}

	return record, nil
}

func (r *BookRepository) CompleteBookSetup(ctx context.Context, params CompleteBookSetupParams) (BookRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return BookRecord{}, fmt.Errorf("begin book setup transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var bookCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM books`).Scan(&bookCount); err != nil {
		return BookRecord{}, fmt.Errorf("count books: %w", err)
	}
	if bookCount > 0 {
		return BookRecord{}, ErrBookExists
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO books (id, owner_user_id, code, name, default_currency_commodity_id, updated_by_user_id, created_at, updated_at)
		VALUES (1, ?, ?, ?, NULL, ?, ?, ?)
	`, params.OwnerUserID, params.Code, params.Name, params.OwnerUserID, params.CreatedAt, params.CreatedAt)
	if err != nil {
		return BookRecord{}, fmt.Errorf("insert book: %w", err)
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        1,
		ActorUserID:   params.OwnerUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        "created book during setup",
	})
	if err != nil {
		return BookRecord{}, err
	}

	// The first book must exist before its audit event can safely reference
	// book_id = 1, so book setup attaches the creation event after insert.
	if _, err := tx.ExecContext(ctx, `
		UPDATE books
		SET created_audit_event_id = ?, updated_audit_event_id = ?
		WHERE id = 1
	`, auditEventID, auditEventID); err != nil {
		return BookRecord{}, fmt.Errorf("attach book audit event: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE setup_steps
		SET completed_at = ?, completed_audit_event_id = ?
		WHERE step_key = 'book' AND completed_at IS NULL
	`, params.CreatedAt, auditEventID)
	if err != nil {
		return BookRecord{}, fmt.Errorf("mark book setup step complete: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return BookRecord{}, fmt.Errorf("read book setup step rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return BookRecord{}, fmt.Errorf("mark book setup step complete: updated %d rows", rowsAffected)
	}

	if err := tx.Commit(); err != nil {
		return BookRecord{}, fmt.Errorf("commit book setup transaction: %w", err)
	}
	committed = true

	return BookRecord{
		ID:          1,
		OwnerUserID: params.OwnerUserID,
		Code:        params.Code,
		Name:        params.Name,
		CreatedAt:   params.CreatedAt,
		UpdatedAt:   params.CreatedAt,
	}, nil
}
