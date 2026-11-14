package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

// A nil figure means "keep the stored value"; a pointer to zero means "this
// run produced zero". A zero-value sentinel would silently turn the second
// case into the first, the partial-update bug the article's GORM example
// describes.
func TestUpdateBackupRunKeepsOmittedFiguresAndPersistsExplicitZero(t *testing.T) {
	t.Parallel()
	harness := newBackupHarness(t)
	ctx := context.Background()
	run, err := harness.service.RequestBackup(ctx, 1)
	require.NoError(t, err)

	byteSize, pageCount := int64(4096), int64(4)
	require.NoError(t, harness.repository.UpdateBackupRun(ctx, db.UpdateBackupRunParams{
		ID: run.ID, Status: "running", ByteSize: &byteSize, PageCount: &pageCount,
		Now: harness.now.Format(time.RFC3339),
	}))

	require.NoError(t, harness.repository.UpdateBackupRun(ctx, db.UpdateBackupRunParams{
		ID: run.ID, Status: "running", Now: harness.now.Format(time.RFC3339),
	}))
	kept, err := harness.repository.BackupRunByID(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, byteSize, kept.ByteSize.Int64)
	assert.Equal(t, pageCount, kept.PageCount.Int64)

	zero := int64(0)
	require.NoError(t, harness.repository.UpdateBackupRun(ctx, db.UpdateBackupRunParams{
		ID: run.ID, Status: "running", ByteSize: &zero, PageCount: &zero,
		Now: harness.now.Format(time.RFC3339),
	}))
	zeroed, err := harness.repository.BackupRunByID(ctx, run.ID)
	require.NoError(t, err)
	assert.True(t, zeroed.ByteSize.Valid)
	assert.Zero(t, zeroed.ByteSize.Int64)
	assert.True(t, zeroed.PageCount.Valid)
	assert.Zero(t, zeroed.PageCount.Int64)
}
