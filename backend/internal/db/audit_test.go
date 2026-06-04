package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsertAuditEventValidatesOperationAndTimestamp(t *testing.T) {
	t.Parallel()

	database := openTestDatabase(t)
	require.NoError(t, Migrate(context.Background(), database))

	tx, err := database.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, tx.Rollback())
	}()

	_, err = insertAuditEvent(context.Background(), tx, AuditEventParams{
		OccurredAt: "2026-06-04T12:00:00Z",
		OriginType: "browser_api",
		Operation:  "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audit operation is invalid")

	_, err = insertAuditEvent(context.Background(), tx, AuditEventParams{
		OccurredAt: "2026-06-04 12:00:00",
		OriginType: "browser_api",
		Operation:  "account.create",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audit occurred_at")

	auditEventID, err := insertAuditEvent(context.Background(), tx, AuditEventParams{
		OccurredAt: "2026-06-04T12:00:00Z",
		OriginType: "browser_api",
		Operation:  "account.create",
	})
	require.NoError(t, err)
	assert.Positive(t, auditEventID)
}
