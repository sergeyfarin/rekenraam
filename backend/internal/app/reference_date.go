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
	ctx, _, err := s.referenceContextInZone(ctx, now)
	return ctx, err
}

// referenceContextInZone is referenceContext for a caller that also schedules
// by the owner's clock. It reads the owner's zone once and returns it with the
// pinned context, so a zone change mid-run cannot pair the pinned day with
// another zone's midnight (#605 review). A day the caller already pinned is
// kept.
func (s *SettingsService) referenceContextInZone(ctx context.Context, now time.Time) (context.Context, *time.Location, error) {
	zone := "UTC"
	if s != nil {
		var err error
		zone, err = s.repository.OwnerTimeZone(ctx)
		if err != nil {
			return nil, nil, err
		}
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, nil, fmt.Errorf("load owner time zone: %w", err)
	}
	if db.ReferenceDate(ctx) != "" {
		return ctx, location, nil
	}
	day := now.In(location).Format(time.DateOnly)
	if s == nil {
		// Without settings, reference reads default to UTC. Live valuation
		// can still resolve the actual owner zone from its repository.
		return db.WithReferenceDate(ctx, day), location, nil
	}
	return db.WithOwnerToday(ctx, day), location, nil
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
