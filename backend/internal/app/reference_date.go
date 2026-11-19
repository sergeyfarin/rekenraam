package app

import (
	"context"
	"fmt"
	"time"

	"rekenraam/backend/internal/db"
)

// referenceContext resolves the owner day before repository transactions. Nested
// reads share that day without another preference query, including read-only
// snapshots. A scheduler must establish a new context for each operation.
func (s *SettingsService) referenceContext(ctx context.Context, now time.Time) (context.Context, error) {
	if db.ReferenceDate(ctx) != "" {
		return ctx, nil
	}
	zone := "UTC"
	if s != nil {
		var err error
		zone, err = s.repository.OwnerTimeZone(ctx)
		if err != nil {
			return nil, err
		}
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("load owner time zone: %w", err)
	}
	return db.WithReferenceDate(ctx, now.In(location).Format(time.DateOnly)), nil
}

// pinnedOwnerDay returns the owner day referenceContext pinned on ctx. A write
// validates and defaults its dates on this day rather than reading the clock
// again: a second reading can fall after midnight, and the write would then
// accept or default to a date its own reads and response treat as the future
// (#566 review).
func pinnedOwnerDay(ctx context.Context) (string, error) {
	day := db.ReferenceDate(ctx)
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		return "", fmt.Errorf("owner day is not pinned (%q): %w", day, err)
	}
	return day, nil
}
