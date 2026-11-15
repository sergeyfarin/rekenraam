package db

import (
	"context"
	"fmt"
	"time"
)

type referenceDateKey struct{}

// referenceDay is the reference date as parsed civil-date parts. It holds no
// caller text at all: CurrentReferenceVersions renders its SQL literal from the
// integers only, and ReferenceDate reports a fixed value for a malformed pin.
type referenceDay struct {
	empty            bool
	valid            bool
	year, month, day int
}

// WithReferenceDate pins current reference-data selection to the application's
// owner-local financial day for one operation. Explicit historical predicates
// do not use this value. It is context-local, never connection or process state.
//
// The date must be YYYY-MM-DD. Callers validate request input before getting
// here; an invalid pin is recorded (as a flag, without its text) only so every
// query that would select reference versions with it fails instead of silently
// using a different day.
func WithReferenceDate(ctx context.Context, date string) context.Context {
	value := referenceDay{empty: date == ""}
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
	case !ok || value.empty:
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
// function: invalid_reference_date"). It is a function call, not a column:
// SQLite resolves an unknown column — qualified or not — outward through every
// enclosing scope, so a caller table or alias could satisfy a column sentinel
// when the helper sits in a correlated subquery. Function names are not
// scope-resolved, and this package registers no SQL functions (sqlite.go).
// The check runs at prepare time, so a run-time short-circuit (a bound false
// before AND) cannot skip it; only a literal "0 AND …" that the parser folds
// away avoids it, and that removes the whole reference read with it.
const invalidReferenceDateSQL = "invalid_reference_date()"

// referenceDateSQL is the SQL expression for the reference date. Unpinned
// repository callers (and an empty pinned date) retain UTC selection. An
// invalid pinned date fails the statement rather than comparing against a
// malformed value or falling back to another day.
func referenceDateSQL(ctx context.Context) string {
	value, ok := ctx.Value(referenceDateKey{}).(referenceDay)
	switch {
	case !ok || value.empty:
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
//
// kind must be one of the keys of referenceVersionSources ("account",
// "commodity", "institution", "investment_instrument", "payee"); any other
// value panics, since every caller passes a constant. A malformed pinned
// reference date makes the returned subquery fail to prepare.
func CurrentReferenceVersions(ctx context.Context, kind string) string {
	source, ok := referenceVersionSources[kind]
	if !ok {
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
