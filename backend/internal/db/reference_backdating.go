package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ReferenceVersionBackdatedError refuses changes that cannot take effect in
// the requested historical window (#552). Refusals precede audit insertion.
type ReferenceVersionBackdatedError struct {
	LatestEffectiveFrom string
}

func (e ReferenceVersionBackdatedError) Error() string {
	return fmt.Sprintf("reference lifecycle or structural changes must take effect on or after %s", e.LatestEffectiveFrom)
}

// ErrReferenceVersionChanged refuses a write whose omitted fields were filled
// from a version that is no longer the one in effect on the requested date: a
// version was added between the service's read and the write transaction.
var ErrReferenceVersionChanged = errors.New("the record changed while this edit was prepared; reload it and try again")

// referenceVersionTable names a versioned reference table and its entity key.
// Only the fixed values below are used, so the names are safe to splice.
type referenceVersionTable struct {
	table, key string
}

var (
	accountVersionTable     = referenceVersionTable{table: "account_versions", key: "account_id"}
	payeeVersionTable       = referenceVersionTable{table: "payee_versions", key: "payee_id"}
	institutionVersionTable = referenceVersionTable{table: "institution_versions", key: "institution_id"}
)

func latestReferenceVersionDateTx(ctx context.Context, tx *sql.Tx, own referenceVersionTable, id int64) (string, error) {
	var latest sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT MAX(effective_from) FROM `+own.table+` WHERE `+own.key+` = ?`, id).Scan(&latest); err != nil {
		return "", fmt.Errorf("read latest %s date: %w", own.table, err)
	}
	return latest.String, nil
}

// referenceWindowDatesTx lists the dates on which a version of entityID dated
// effectiveFrom must satisfy a rule that depends on other entities (#552). The
// version governs from effectiveFrom until the entity's next later version, or
// open-ended (through today) when there is none. Other entities' as-of state
// changes only on the dates their versions take effect, so the version's own
// date plus each such date inside the window covers every day it governs.
// Checking only effectiveFrom, or only today, misses the rest of the window.
// Call it before the new version is inserted.
func referenceWindowDatesTx(ctx context.Context, tx *sql.Tx, own referenceVersionTable, entityID int64, effectiveFrom string, others ...referenceVersionTable) ([]string, error) {
	var end sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT MIN(effective_from) FROM `+own.table+` WHERE `+own.key+` = ? AND effective_from > ?`, entityID, effectiveFrom).Scan(&end); err != nil {
		return nil, fmt.Errorf("read %s window end: %w", own.table, err)
	}
	query := `SELECT ?`
	args := []any{effectiveFrom}
	for _, other := range others {
		query += ` UNION SELECT effective_from FROM ` + other.table + ` WHERE effective_from > ? AND (? IS NULL OR effective_from < ?)`
		args = append(args, effectiveFrom, end, end)
	}
	rows, err := tx.QueryContext(ctx, query+` ORDER BY 1`, args...)
	if err != nil {
		return nil, fmt.Errorf("read %s window dates: %w", own.table, err)
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, fmt.Errorf("scan %s window date: %w", own.table, err)
		}
		dates = append(dates, date)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s window dates: %w", own.table, err)
	}
	return dates, nil
}

// requireBaseVersionTx refuses the write when the version in effect on the
// requested date is not the one the caller filled omitted fields from. A zero
// base means the caller sent a complete snapshot and has nothing to go stale.
func requireBaseVersionTx(baseVersionID int64, asOfVersionID int64) error {
	if baseVersionID != 0 && baseVersionID != asOfVersionID {
		return ErrReferenceVersionChanged
	}
	return nil
}
