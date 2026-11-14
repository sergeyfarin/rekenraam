package db

import (
	"context"
	"fmt"
	"time"
)

type referenceDateKey struct{}

// referenceDay is the reference date as parsed civil-date parts. Keeping the
// parts numeric means CurrentReferenceVersions renders its SQL literal from
// integers only: no caller-supplied text ever reaches the query string.
type referenceDay struct {
	raw              string
	year, month, day int
	valid            bool
}

// WithReferenceDate pins current reference-data selection to the application's
// owner-local financial day for one operation. Explicit historical predicates
// do not use this value. It is context-local, never connection or process state.
//
// The date must be YYYY-MM-DD. Callers validate request input before getting
// here; an invalid value is kept only so every query that would select
// reference versions with it fails instead of silently using a different day.
func WithReferenceDate(ctx context.Context, date string) context.Context {
	value := referenceDay{raw: date}
	if parsed, err := time.Parse(time.DateOnly, date); err == nil {
		value.year, value.month, value.day = parsed.Year(), int(parsed.Month()), parsed.Day()
		value.valid = true
	}
	return context.WithValue(ctx, referenceDateKey{}, value)
}

// ReferenceDate returns the pinned reference date, or "" when none is pinned.
func ReferenceDate(ctx context.Context) string {
	value, ok := ctx.Value(referenceDateKey{}).(referenceDay)
	if !ok {
		return ""
	}
	if !value.valid {
		return value.raw
	}
	return value.literal()
}

func (d referenceDay) literal() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.year, d.month, d.day)
}

// referenceDateSQL is the SQL expression for the reference date. Unpinned
// repository callers (and an empty pinned date) retain UTC selection. An
// invalid pinned date renders an unknown identifier so the statement fails to
// prepare ("no such column: invalid_reference_date") rather than comparing
// against a malformed value.
func referenceDateSQL(ctx context.Context) string {
	value, ok := ctx.Value(referenceDateKey{}).(referenceDay)
	switch {
	case !ok || value.raw == "":
		return "date('now')"
	case !value.valid:
		return "invalid_reference_date"
	default:
		return "'" + value.literal() + "'"
	}
}

// CurrentReferenceVersions supplies the same effective-date/version-sequence
// ordering as the former UTC views, with the application's date.
// Unwired repository callers retain UTC selection; application operations
// establish their reference date before opening a transaction or read snapshot.
func CurrentReferenceVersions(ctx context.Context, kind string) string {
	dateSQL := referenceDateSQL(ctx)
	table, key := kind+"_versions", kind+"_id"
	if kind == "investment_instrument" {
		key = "instrument_id"
	}
	return `(SELECT reference_version.* FROM ` + table + ` reference_version
		WHERE reference_version.id = (
			SELECT candidate.id FROM ` + table + ` candidate
			WHERE candidate.` + key + ` = reference_version.` + key + `
			  AND candidate.effective_from <= ` + dateSQL + `
			ORDER BY candidate.effective_from DESC, candidate.version_seq DESC LIMIT 1))`
}
