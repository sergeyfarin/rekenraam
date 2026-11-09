package db

import (
	"context"
	"database/sql"
	"fmt"
)

// nextVersionSeqTx allocates the next append-only version number for one
// reference-data entity. The next number follows the newest row, never the
// version in effect on the owner's reference date: a backdated version can be
// appended without making the next update collide on (entity_id, version_seq)
// (#549).
func nextVersionSeqTx(ctx context.Context, tx *sql.Tx, table, entityColumn string, entityID int64) (int64, error) {
	switch table + "." + entityColumn {
	case "account_versions.account_id",
		"institution_versions.institution_id",
		"payee_versions.payee_id",
		"investment_instrument_versions.instrument_id":
	default:
		return 0, fmt.Errorf("next version sequence: unsupported version table %s.%s", table, entityColumn)
	}

	var next int64
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT COALESCE(MAX(version_seq), 0) + 1 FROM %s WHERE %s = ?`,
		table, entityColumn,
	), entityID).Scan(&next); err != nil {
		return 0, fmt.Errorf("read next %s version sequence: %w", table, err)
	}
	return next, nil
}
