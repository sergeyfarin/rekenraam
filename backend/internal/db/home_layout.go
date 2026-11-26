package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type HomeLayoutRecord struct {
	Revision int64
	Document string
}

func (r *TransactionRepository) ReadHomeLayout(ctx context.Context, bookID, ownerID int64) (HomeLayoutRecord, error) {
	var record HomeLayoutRecord
	err := r.database.QueryRowContext(ctx, `
		SELECT h.revision, h.document_json
		FROM books b JOIN home_layouts h ON h.book_id = b.id AND h.owner_user_id = b.owner_user_id
		WHERE b.id = ? AND b.owner_user_id = ?
	`, bookID, ownerID).Scan(&record.Revision, &record.Document)
	if errors.Is(err, sql.ErrNoRows) {
		return HomeLayoutRecord{}, nil
	}
	if err != nil {
		return HomeLayoutRecord{}, fmt.Errorf("read home layout: %w", err)
	}
	return record, nil
}

// SaveHomeLayout compares and replaces the whole document in one statement.
// Revision zero identifies the virtual default before a row has been written.
func (r *TransactionRepository) SaveHomeLayout(ctx context.Context, bookID, ownerID, expected int64, document string) (int64, error) {
	var revision int64
	err := r.database.QueryRowContext(ctx, `
		INSERT INTO home_layouts (book_id, owner_user_id, revision, document_json)
		SELECT id, owner_user_id, 1, ? FROM books
		WHERE id = ? AND owner_user_id = ? AND (
		  ? = 0 OR EXISTS (SELECT 1 FROM home_layouts WHERE book_id = books.id AND owner_user_id = books.owner_user_id)
		)
		ON CONFLICT (book_id, owner_user_id) DO UPDATE SET
		  revision = home_layouts.revision + 1, document_json = excluded.document_json
		WHERE home_layouts.revision = ?
		RETURNING revision
	`, document, bookID, ownerID, expected, expected).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("save home layout: %w", err)
	}
	return revision, nil
}
