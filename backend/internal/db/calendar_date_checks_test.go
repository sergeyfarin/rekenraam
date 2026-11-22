package db

import (
	"testing"

	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestPayeeVersionsRequireCalendarExactDates(t *testing.T) {
	t.Parallel()
	database := newRecurringTestDatabase(t)
	const insert = `INSERT INTO payee_versions (payee_id, version_seq, effective_from,
		recorded_at, changed_by_user_id, change_reason, status, name, normalized_name)
		VALUES (1, ?, ?, '2026-02-28T00:00:00Z', 1, 'test', 'active', 'Landlord', 'landlord')`

	for i, test := range []struct {
		name, date string
		valid      bool
	}{
		{"ordinary_date", "2026-02-28", true},
		{"leap_day", "2024-02-29", true},
		{"year_zero", "0000-01-01", true},
		{"non_leap_day", "2026-02-29", false},
		{"day_overflow", "2026-02-30", false},
		{"timestamp", "2026-02-03T00:00:00Z", false},
		{"unpadded_date", "2026-2-3", false},
		{"nonnumeric_date", "abcd-ef-gh", false},
		{"negative_year", "-0001-02-28", false},
		{"embedded_nul", "2026-02-28\x00tail", false},
		{"relative_date", "now", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := database.Exec(insert, i+2, test.date)
			if test.valid {
				require.NoError(t, err)
				return
			}
			var constraint sqlite3.Error
			require.ErrorAs(t, err, &constraint)
			require.Equal(t, sqlite3.ErrConstraintCheck, constraint.ExtendedCode)
			require.ErrorContains(t, err, "effective_from")
		})
	}
}

func TestRecurringTemplateEndDateIsOptionalAndOrdered(t *testing.T) {
	t.Parallel()
	database := newRecurringTestDatabase(t)
	const insert = `INSERT INTO recurring_templates (book_id, name, transaction_kind, frequency, starts_on,
		ends_on, generate_from, created_at, created_by_user_id, updated_at)
		VALUES (1, 'Rent', 'ordinary', 'daily', '2026-02-01', ?, '2026-02-01',
			'2026-02-01T00:00:00Z', 1, '2026-02-01T00:00:00Z')`

	for _, end := range []any{nil, "2026-02-01", "2026-02-28"} {
		_, err := database.Exec(insert, end)
		require.NoError(t, err, "end date %v", end)
	}
	for _, end := range []string{"2026-01-31", "2026-02-30"} {
		_, err := database.Exec(insert, end)
		var constraint sqlite3.Error
		require.ErrorAs(t, err, &constraint, "end date %s", end)
		require.Equal(t, sqlite3.ErrConstraintCheck, constraint.ExtendedCode)
		require.ErrorContains(t, err, "ends_on")
	}
}
