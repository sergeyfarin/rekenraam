package db

import (
	"context"
	"database/sql"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// ShortLotQuantityEvent is one dated change to the borrowed units a holding
// owes: a short opening adds quantity, a cover removes it (#173). Folding
// them by date gives the negative balance a named short explains.
type ShortLotQuantityEvent struct {
	EventDate     string
	AccountID     int64
	CommodityID   int64
	QuantityValue exact.Coefficient
	QuantityScale int
}

// ShortLotQuantityEventsThrough reads every effective short-lot event dated
// on or before through (all of them when through is empty), in date order.
func (r *TransactionRepository) ShortLotQuantityEventsThrough(ctx context.Context, bookID int64, through string) ([]ShortLotQuantityEvent, error) {
	return shortLotQuantityEvents(ctx, r.database, bookID, through)
}

// SelfCheckShortLotQuantityEvents reads the same events inside the self-check
// snapshot.
func (r *SelfCheckRepository) SelfCheckShortLotQuantityEvents(ctx context.Context, transaction *sql.Tx, bookID int64) ([]ShortLotQuantityEvent, error) {
	return shortLotQuantityEvents(ctx, transaction, bookID, "")
}

func shortLotQuantityEvents(ctx context.Context, source queryer, bookID int64, through string) ([]ShortLotQuantityEvent, error) {
	rows, err := source.QueryContext(ctx, `
		SELECT e.event_date, l.account_id, l.commodity_id, e.quantity_value, e.quantity_scale
		FROM effective_investment_lot_events e
		JOIN investment_lots l ON l.id = e.lot_id
		WHERE l.book_id = ? AND l.position_side = 'short' AND (? = '' OR e.event_date <= ?)
		ORDER BY e.event_date, e.id`, bookID, through, through)
	if err != nil {
		return nil, fmt.Errorf("read short lot quantity events: %w", err)
	}
	defer rows.Close()
	var events []ShortLotQuantityEvent
	for rows.Next() {
		var event ShortLotQuantityEvent
		if err := rows.Scan(&event.EventDate, &event.AccountID, &event.CommodityID, &event.QuantityValue, &event.QuantityScale); err != nil {
			return nil, fmt.Errorf("scan short lot quantity event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate short lot quantity events: %w", err)
	}
	return events, nil
}
