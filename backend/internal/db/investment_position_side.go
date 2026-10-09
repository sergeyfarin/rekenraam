package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Position sides (ADR 0013). A short lot holds borrowed units at a positive
// quantity and records its exact opening proceeds in the basis columns; a
// cover consumes it. Every writer that predates named shorts (#173) is a long
// writer, so an empty side means long.
const (
	PositionSideLong  = "long"
	PositionSideShort = "short"
)

// ErrPositionSideConflict refuses a lot opening while the holding is, or later
// becomes, a position on the opposite side. One holding account carries one
// side of an instrument at a time; crossing zero is two explicit operations.
var ErrPositionSideConflict = errors.New("investment position side conflict")

func positionSideOrLong(side string) string {
	if side == "" {
		return PositionSideLong
	}
	return side
}

func validPositionSide(side string) bool {
	return side == PositionSideLong || side == PositionSideShort
}

// requirePositionSideAvailableTx refuses a lot opening on side when the same
// account and instrument has an open lot on the other side, or an effective
// lot event on the other side dated after openedOn. Either would make the two
// sides overlap in time, which this book does not model (#173). Same-day
// activity is allowed, so a holding can close one side and open the other on
// one date, in the order the two operations are entered.
func requirePositionSideAvailableTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID int64, side, openedOn string) error {
	var conflictDate sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT MAX(conflict_date) FROM (
			SELECT '9999-12-31' AS conflict_date FROM current_investment_lots
			WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side <> ? AND status = 'open'
			UNION ALL
			SELECT e.event_date FROM effective_investment_lot_events e
			JOIN investment_lots l ON l.id = e.lot_id
			WHERE l.book_id = ? AND l.account_id = ? AND l.commodity_id = ? AND l.position_side <> ?
				AND e.event_date > ?
		)`, bookID, accountID, commodityID, side, bookID, accountID, commodityID, side, openedOn).Scan(&conflictDate); err != nil {
		return fmt.Errorf("read opposite position side: %w", err)
	}
	if !conflictDate.Valid {
		return nil
	}
	other := PositionSideShort
	if side == PositionSideShort {
		other = PositionSideLong
	}
	if conflictDate.String == "9999-12-31" {
		return fmt.Errorf("%w: this holding has an open %s position; close it before opening a %s lot", ErrPositionSideConflict, other, side)
	}
	return fmt.Errorf("%w: this holding has %s activity on %s, after this %s opening on %s", ErrPositionSideConflict, other, conflictDate.String, side, openedOn)
}
