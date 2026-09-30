package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceRevisionCheckPreservesDatabaseErrors(t *testing.T) {
	database := openTestDatabase(t)
	require.NoError(t, database.Close())
	err := NewImportRepository(database).CheckSourceRevision(context.Background(), CommitImportSourceRevisionParams{
		BookID: 1, IdentityID: 1, StagedRowID: 2, SourceOperationID: 1,
	})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrImportSourceRevisionConflict)
	require.Contains(t, err.Error(), "database is closed")
}
