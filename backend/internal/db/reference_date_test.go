package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var malformedReferenceDates = []string{
	"2026-2-3",
	"2026-02-30",
	"2026-02-03T00:00:00Z", // RFC3339: the likeliest caller regression
	"2026-02-03 ",
	"2026-02-03\x00",
	"2026-02-03\n",
	"2026-02-03' OR '1'='1",
	"today",
}

func TestReferenceDateRendersOnlyParsedCivilDates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	assert.Equal(t, "", ReferenceDate(ctx))
	assert.Equal(t, "date('now')", referenceDateSQL(ctx))

	pinned := WithReferenceDate(ctx, "2026-02-03")
	assert.Equal(t, "2026-02-03", ReferenceDate(pinned))
	assert.Equal(t, "'2026-02-03'", referenceDateSQL(pinned))

	// Empty keeps the unpinned UTC behaviour it always had.
	unpinned := WithReferenceDate(pinned, "")
	assert.Equal(t, "", ReferenceDate(unpinned))
	assert.Equal(t, "date('now')", referenceDateSQL(unpinned))

	for _, malformed := range malformedReferenceDates {
		dated := WithReferenceDate(ctx, malformed)
		// Still a pin (so nothing re-pins to today), but never the raw text.
		assert.Equal(t, invalidReferenceDate, ReferenceDate(dated), "%q", malformed)
		assert.Equal(t, invalidReferenceDateSQL, referenceDateSQL(dated), "%q", malformed)
		assert.NotContains(t, CurrentReferenceVersions(dated, "account"), malformed, "%q", malformed)
	}
}

func TestCurrentReferenceVersionsRejectsUnknownKind(t *testing.T) {
	t.Parallel()
	assert.PanicsWithValue(t, `db: unknown reference version kind "account_versions; --"`, func() {
		CurrentReferenceVersions(context.Background(), "account_versions; --")
	})
}

// An invalid pinned date must fail the read, never fall back to another day
// or compare effective_from against malformed text.
func TestCurrentReferenceVersionsFailsClosedOnInvalidDate(t *testing.T) {
	t.Parallel()
	database, _, _ := migratedInvestmentTestDatabase(t)
	query := func(ctx context.Context, kind string) error {
		var count int
		return database.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+CurrentReferenceVersions(ctx, kind)+` v`).Scan(&count)
	}

	for kind := range referenceVersionSources {
		require.NoError(t, query(context.Background(), kind), kind)
		require.NoError(t, query(WithReferenceDate(context.Background(), "2026-02-03"), kind), kind)
		for _, malformed := range malformedReferenceDates {
			err := query(WithReferenceDate(context.Background(), malformed), kind)
			require.ErrorContains(t, err, "no such function: invalid_reference_date", "%s %q", kind, malformed)
		}
	}
}

// The sentinel must not depend on query scope. SQLite resolves an unknown
// column outward through enclosing scopes, even when qualified, so a column
// sentinel would be satisfied by a caller table aliased "candidate" with an
// invalid_reference_date column once the helper sits in a correlated subquery,
// turning the fail-closed read into a successful one.
func TestCurrentReferenceVersionsSentinelIgnoresCallerColumns(t *testing.T) {
	t.Parallel()
	database, _, _ := migratedInvestmentTestDatabase(t)
	dated := WithReferenceDate(context.Background(), "2026-02-30")
	var count int
	err := database.QueryRowContext(dated, `SELECT (SELECT COUNT(*) FROM `+CurrentReferenceVersions(dated, "account")+` v)
		FROM (SELECT '2026-02-03' AS invalid_reference_date) candidate`).Scan(&count)
	require.ErrorContains(t, err, "no such function: invalid_reference_date")
}
