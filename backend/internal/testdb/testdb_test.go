package testdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenReturnsIndependentMigratedCopies(t *testing.T) {
	first, _ := Open(t)
	second, _ := Open(t)

	var migrationCount int
	require.NoError(t, first.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM goose_db_version WHERE is_applied = 1
	`).Scan(&migrationCount))
	assert.Positive(t, migrationCount)

	_, err := first.ExecContext(context.Background(), `CREATE TABLE copy_isolation_marker (id INTEGER PRIMARY KEY)`)
	require.NoError(t, err)

	var markerCount int
	err = second.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'copy_isolation_marker'
	`).Scan(&markerCount)
	require.NoError(t, err)
	assert.Zero(t, markerCount)
}
