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

// invalidReferenceDate is what ReferenceDate reports for a malformed pin. It is
// non-empty so callers such as app.referenceContext still see a pin and do not
// silently re-pin to today, and it keeps the rejected text out of error
// messages built from ReferenceDate.
const invalidReferenceDate = "invalid-date"

// ReferenceDate returns the pinned reference date, "" when none is pinned, or
// invalidReferenceDate when the pinned value was not a YYYY-MM-DD date.
func ReferenceDate(ctx context.Context) string {
	value, ok := ctx.Value(referenceDateKey{}).(referenceDay)
	switch {
	case !ok || value.raw == "":
		return ""
	case !value.valid:
		return invalidReferenceDate
	default:
		return value.literal()
	}
}

func (d referenceDay) literal() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.year, d.month, d.day)
}

// invalidReferenceDateSQL fails the statement at prepare time ("no such
// column: candidate.invalid_reference_date"). It is qualified by the helper's
// own candidate alias, so it resolves only against the version tables in
// referenceVersionSources, never against a caller's tables;
// TestCurrentReferenceVersionsFailsClosedOnInvalidDate proves none of them has
// that column.
const invalidReferenceDateSQL = "candidate.invalid_reference_date"

// referenceDateSQL is the SQL expression for the reference date. Unpinned
// repository callers (and an empty pinned date) retain UTC selection. An
// invalid pinned date fails the statement rather than comparing against a
// malformed value or falling back to another day.
func referenceDateSQL(ctx context.Context) string {
	value, ok := ctx.Value(referenceDateKey{}).(referenceDay)
	switch {
	case !ok || value.raw == "":
		return "date('now')"
	case !value.valid:
		return invalidReferenceDateSQL
	default:
		return "'" + value.literal() + "'"
	}
}

// referenceVersionSources is the closed set of versioned reference kinds, so
// CurrentReferenceVersions never builds a table or column name from its input.
var referenceVersionSources = map[string]struct{ table, key string }{
	"account":               {"account_versions", "account_id"},
	"commodity":             {"commodity_versions", "commodity_id"},
	"institution":           {"institution_versions", "institution_id"},
	"investment_instrument": {"investment_instrument_versions", "instrument_id"},
	"payee":                 {"payee_versions", "payee_id"},
}

// CurrentReferenceVersions supplies the same effective-date/version-sequence
// ordering as the former UTC views, with the application's date.
// Unwired repository callers retain UTC selection; application operations
// establish their reference date before opening a transaction or read snapshot.
func CurrentReferenceVersions(ctx context.Context, kind string) string {
	source, ok := referenceVersionSources[kind]
	if !ok {
		// Every caller passes a constant kind; an unknown one is a programming
		// error that any test reaching the call site surfaces.
		panic(fmt.Sprintf("db: unknown reference version kind %q", kind))
	}
	dateSQL := referenceDateSQL(ctx)
	table, key := source.table, source.key
	return `(SELECT reference_version.* FROM ` + table + ` reference_version
		WHERE reference_version.id = (
			SELECT candidate.id FROM ` + table + ` candidate
			WHERE candidate.` + key + ` = reference_version.` + key + `
			  AND candidate.effective_from <= ` + dateSQL + `
			ORDER BY candidate.effective_from DESC, candidate.version_seq DESC LIMIT 1))`
}
