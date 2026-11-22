package db

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

var (
	sqliteCheckStart = regexp.MustCompile(`(?i)\bCHECK\s*\(`)
	sqliteDateCall   = regexp.MustCompile(`(?i)\bdate\s*\(`)
	legacyDateGlob   = regexp.MustCompile(`(?i)\bGLOB\s*'(\?{4}-\?{2}-[^']*)'`)
)

func TestInstalledCalendarDateChecks(t *testing.T) {
	t.Parallel()
	for _, encoding := range []string{"UTF-8", "UTF-16le", "UTF-16be"} {
		t.Run(encoding, func(t *testing.T) {
			database := openTestDatabase(t)
			_, err := database.Exec(`PRAGMA encoding = '` + encoding + `'`)
			require.NoError(t, err)
			require.NoError(t, Migrate(context.Background(), database))
			var installedEncoding string
			require.NoError(t, database.QueryRow(`PRAGMA encoding`).Scan(&installedEncoding))
			require.Equal(t, encoding, installedEncoding)
			requireCalendarDateCheckInventory(t, database)

			for _, column := range calendarDateColumns {
				t.Run(column.table+"."+column.name, func(t *testing.T) {
					check := columnCheckExpression(t, database, column.table, column.name)
					require.Regexp(t, sqliteDateCall, check)
					requireCalendarDateCheckValues(t, database, column.name, check)
				})
			}
		})
	}
}

func requireCalendarDateCheckInventory(t *testing.T, database *sql.DB) {
	t.Helper()
	expected := make(map[string]int)
	listed := make([]string, 0, len(calendarDateColumns))
	for _, column := range calendarDateColumns {
		listed = append(listed, column.table+"."+column.name)
		expected[column.table]++
	}
	require.ElementsMatch(t, listed, installedCalendarDateColumnNames(t, database),
		"named calendar-date columns in the installed schema")

	// Every inventoried column is also tested below with its own installed
	// inline CHECK. Matching the count per table detects any additional CHECK,
	// including one added by a forward migration or declared at table level.
	rows, err := database.Query(`SELECT type, name, sql FROM sqlite_schema WHERE sql IS NOT NULL`)
	require.NoError(t, err)
	defer rows.Close()
	installed := make(map[string]int)
	for rows.Next() {
		var kind, name, definition string
		require.NoError(t, rows.Scan(&kind, &name, &definition))
		for _, match := range legacyDateGlob.FindAllStringSubmatch(definition, -1) {
			// The audit timestamp has its own RFC 3339 shape constraint.
			require.True(t, kind == "table" && name == "audit_events" && match[1] == "????-??-??T??:??:??Z",
				"%s contains a shape-only date check: %s", name, match[0])
		}
		if kind == "table" {
			for _, start := range sqliteCheckStart.FindAllStringIndex(definition, -1) {
				if sqliteDateCall.MatchString(sqliteCheckBody(t, definition, start[1]-1)) {
					installed[name]++
				}
			}
		}
	}
	require.NoError(t, rows.Err())
	require.Equal(t, expected, installed, "inventory of installed date CHECKs by table")
}

func installedCalendarDateColumnNames(t *testing.T, database *sql.DB) []string {
	t.Helper()
	// Find date columns even when their CHECK is missing or only checks shape.
	rows, err := database.Query(`SELECT tbl.name || '.' || col.name
		FROM sqlite_schema AS tbl CROSS JOIN pragma_table_info(tbl.name) AS col
		WHERE tbl.type = 'table'
			AND (lower(col.name) GLOB '*_date' OR lower(col.name) GLOB '*_on'
				OR lower(col.name) GLOB '*_from' OR lower(col.name) GLOB '*_to'
				OR lower(col.name) GLOB '*period_start*' OR lower(col.name) GLOB '*_day')`)
	require.NoError(t, err)
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	return names
}

func requireCalendarDateCheckValues(t *testing.T, database *sql.DB, column, check string) {
	t.Helper()
	supportingColumns := ""
	if column == "ends_on" {
		supportingColumns = ", starts_on TEXT NOT NULL DEFAULT '0000-01-01'"
	}
	// Keep the real TEXT affinity and isolate this CHECK from foreign keys,
	// triggers, and other columns' constraints.
	_, err := database.Exec(`CREATE TEMP TABLE date_probe (` + column + ` TEXT CHECK (` + check + `)` + supportingColumns + `)`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := database.Exec(`DROP TABLE date_probe`)
		require.NoError(t, err)
	})

	insert := `INSERT INTO date_probe (` + column + `) VALUES (?)`
	for _, value := range []any{nil, "0000-01-01", "2024-02-01", "2026-02-01"} {
		_, err := database.Exec(insert, value)
		require.NoError(t, err, "value %v", value)
	}
	for _, value := range []string{"2026-02-28", "2024-02-29", "0000-02-29", "2000-02-29", "9999-12-31"} {
		_, err := database.Exec(insert, value)
		if column == "period_start" {
			requireDateCheckFailure(t, err, value)
		} else {
			require.NoError(t, err, "value %q", value)
		}
	}
	for _, value := range []any{
		"2026-02-29", "2026-02-30", "2026-04-31", "1900-02-29",
		"2026-00-01", "2026-13-01", "2026-02-00", "2026-02-32", "abcd-ef-gh", "abcd-ef-01",
		"2026-02-03T00:00:00Z", "2026-2-3", " 2026-02-01", "2026-02-01 ", "-0001-02-01",
		"", "now", "NOW", "2461074.50", "2026-02-01\x00tail", []byte("2026-02-01"), 20260201,
	} {
		_, err := database.Exec(insert, value)
		requireDateCheckFailure(t, err, value)
	}
}

func requireDateCheckFailure(t *testing.T, err error, value any) {
	t.Helper()
	var constraint sqlite3.Error
	require.ErrorAs(t, err, &constraint, "value %v", value)
	require.Equal(t, sqlite3.ErrConstraintCheck, constraint.ExtendedCode, "value %v", value)
}

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
			requireDateCheckFailure(t, err, test.date)
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
		requireDateCheckFailure(t, err, end)
		require.ErrorContains(t, err, "ends_on")
	}
}

func TestBudgetTargetsRequireRealMonthStarts(t *testing.T) {
	t.Parallel()
	database := seedReplayTestBook(t)
	var category int64
	require.NoError(t, database.QueryRow(`SELECT account_id FROM account_versions
		WHERE account_class = 'expense' ORDER BY id LIMIT 1`).Scan(&category))
	const insert = `INSERT INTO budget_targets (book_id, category_account_id, commodity_id, period_start,
		quantity_value, quantity_scale, created_at, updated_at, updated_by_user_id, updated_audit_event_id)
		VALUES (1, ?, 1, ?, '100', 2, '2026-02-01T00:00:00Z', '2026-02-01T00:00:00Z', 1, 1)`
	for _, value := range []string{"2026-02-01", "2024-02-01"} {
		_, err := database.Exec(insert, category, value)
		require.NoError(t, err, "month start %q", value)
	}
	for _, value := range []string{"2026-02-02", "2026-13-01", "abcd-ef-01"} {
		t.Run(value, func(t *testing.T) {
			_, err := database.Exec(insert, category, value)
			requireDateCheckFailure(t, err, value)
			require.ErrorContains(t, err, "period_start")
		})
	}
}

// Each column exercises its own CHECK from the fully migrated SQLite schema.
var calendarDateColumns = []struct{ table, name string }{
	{"account_budget_treatment_versions", "effective_from"},
	{"account_versions", "closed_on"},
	{"account_versions", "effective_from"},
	{"account_versions", "opened_on"},
	{"backup_runs", "scheduled_for_local_date"},
	{"budget_targets", "period_start"},
	{"commodity_versions", "effective_from"},
	{"dividend_defaults", "effective_from"},
	{"dividend_defaults", "effective_to"},
	{"institution_versions", "effective_from"},
	{"investment_automation_rules", "effective_from"},
	{"investment_automation_rules", "effective_to"},
	{"investment_capital_return_facts", "effective_on"},
	{"investment_capital_return_facts", "payment_on"},
	{"investment_cash_in_lieu_facts", "payment_on"},
	{"investment_disposal_decisions", "event_date"},
	{"investment_disposal_decisions", "source_effective_from"},
	{"investment_fee_policy_versions", "effective_from"},
	{"investment_instrument_versions", "effective_from"},
	{"investment_lot_events", "event_date"},
	{"investment_lots", "opened_on"},
	{"investment_operation_components", "amount_date"},
	{"investment_operation_dates", "event_date"},
	{"investment_operations", "event_date"},
	{"investment_payment_in_lieu_facts", "ex_date"},
	{"investment_provider_events", "event_date"},
	{"investment_split_facts", "effective_on"},
	{"investment_transfer_facts", "effective_on"},
	{"investment_transfer_link_revisions", "original_acquired_on"},
	{"investment_transfer_lot_links", "original_acquired_on"},
	{"journal_entries", "entry_date"},
	{"payee_versions", "effective_from"},
	{"posting_versions", "cleared_on"},
	{"price_observations", "valuation_date"},
	{"pricing_refresh_state", "last_success_date"},
	{"pricing_source_assignments", "effective_from"},
	{"pricing_source_assignments", "effective_to"},
	{"reconciliation_checkpoint_postings", "entry_date"},
	{"reconciliation_checkpoints", "statement_date"},
	{"reconciliation_session_postings", "entry_date"},
	{"reconciliation_sessions", "statement_date"},
	{"recurring_occurrences", "occurrence_date"},
	{"recurring_templates", "ends_on"},
	{"recurring_templates", "generate_from"},
	{"recurring_templates", "starts_on"},
	{"transaction_versions", "transaction_date"},
}
