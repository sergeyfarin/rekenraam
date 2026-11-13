package db

import (
	"context"
	"database/sql"
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

func latestReferenceVersionDateTx(ctx context.Context, tx *sql.Tx, kind string, id int64) (string, error) {
	switch kind {
	case "payee", "institution":
	default:
		return "", fmt.Errorf("unsupported reference version kind %q", kind)
	}
	var latest string
	if err := tx.QueryRowContext(ctx, `SELECT MAX(effective_from) FROM `+kind+`_versions WHERE `+kind+`_id = ?`, id).Scan(&latest); err != nil {
		return "", fmt.Errorf("read latest %s version date: %w", kind, err)
	}
	return latest, nil
}
