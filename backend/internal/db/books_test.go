package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Setup creates book 1, returns the row it stored (updated_by included),
// CurrentBook reads that same row, and a second setup is refused by the
// application guard before the schema sees it.
func TestBookSetupCreatesTheSingleBookOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database := openTestDatabase(t)
	require.NoError(t, Migrate(ctx, database))
	books := NewBookRepository(database)

	_, err := books.CurrentBook(ctx)
	require.ErrorIs(t, err, ErrNotFound)

	owner, err := NewSetupRepository(database).CompleteOwnerSetup(ctx, CompleteOwnerSetupParams{
		Username:         "owner",
		PasswordHash:     "password-hash",
		SessionTokenHash: "session-token-hash",
		CreatedAt:        "2026-06-12T08:00:00Z",
		SessionExpiresAt: "2026-06-13T08:00:00Z",
	})
	require.NoError(t, err)
	setup := CompleteBookSetupParams{
		OwnerUserID: owner.ID,
		RequestID:   "request-book",
		OriginType:  "browser_api",
		Operation:   "setup.book.complete",
		Code:        "main",
		Name:        "Main",
		CreatedAt:   "2026-06-12T08:01:00Z",
	}
	created, err := books.CompleteBookSetup(ctx, setup)
	require.NoError(t, err)
	assert.Equal(t, int64(1), created.ID)

	current, err := books.CurrentBook(ctx)
	require.NoError(t, err)
	assert.Equal(t, created, current)

	setup.Code, setup.RequestID = "second", "request-second-book"
	_, err = books.CompleteBookSetup(ctx, setup)
	require.ErrorIs(t, err, ErrBookExists)
	var count int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM books`).Scan(&count))
	assert.Equal(t, 1, count)
}
