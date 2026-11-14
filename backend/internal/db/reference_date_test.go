package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferenceDateRendersOnlyParsedCivilDates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	assert.Equal(t, "", ReferenceDate(ctx))
	assert.Equal(t, "date('now')", referenceDateSQL(ctx))

	pinned := WithReferenceDate(ctx, "2026-02-03")
	assert.Equal(t, "2026-02-03", ReferenceDate(pinned))
	assert.Equal(t, "'2026-02-03'", referenceDateSQL(pinned))

	// Empty keeps the unpinned UTC behaviour it always had.
	assert.Equal(t, "date('now')", referenceDateSQL(WithReferenceDate(pinned, "")))

	for _, invalid := range []string{"2026-2-3", "2026-02-30", "2026-02-03' OR '1'='1", "today"} {
		dated := WithReferenceDate(ctx, invalid)
		assert.Equal(t, invalid, ReferenceDate(dated), invalid)
		assert.Equal(t, "invalid_reference_date", referenceDateSQL(dated), invalid)
		assert.NotContains(t, CurrentReferenceVersions(dated, "account"), invalid+"'", invalid)
	}
}

// An invalid pinned date must fail the read, never fall back to another day
// or compare effective_from against malformed text.
func TestCurrentReferenceVersionsFailsClosedOnInvalidDate(t *testing.T) {
	t.Parallel()
	database, _, _ := migratedInvestmentTestDatabase(t)
	query := func(ctx context.Context) error {
		var count int
		return database.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts a JOIN `+
			CurrentReferenceVersions(ctx, "account")+` av ON av.account_id = a.id`).Scan(&count)
	}

	require.NoError(t, query(context.Background()))
	require.NoError(t, query(WithReferenceDate(context.Background(), "2026-02-03")))
	err := query(WithReferenceDate(context.Background(), "2026-02-03' OR '1'='1"))
	require.ErrorContains(t, err, "no such column: invalid_reference_date")
}
