package app

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/testdb"
)

// cloneReferenceVersion appends fixture history without changing any source row.
// A future version followed by an owner-today version also checks that effective
// date takes precedence over version_seq, with version_seq breaking same-day ties.
func cloneReferenceVersion(t *testing.T, database *sql.DB, table, key string, id int64, date, labelColumn, label string) int64 {
	t.Helper()
	rows, err := database.Query("PRAGMA table_info(" + table + ")")
	require.NoError(t, err)
	var columns []string
	for rows.Next() {
		var cid, notNull, primary int
		var name, dataType string
		var defaultValue sql.NullString
		require.NoError(t, rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primary))
		if name != "id" {
			columns = append(columns, name)
		}
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	values := append([]string(nil), columns...)
	for i, column := range columns {
		switch column {
		case "version_seq":
			values[i] = "version_seq+1"
		case "effective_from":
			values[i] = "'" + date + "'"
		case labelColumn:
			values[i] = "'" + label + "'"
		}
	}
	result, err := database.Exec(fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s WHERE %s=? ORDER BY version_seq DESC LIMIT 1", table, strings.Join(columns, ","), strings.Join(values, ","), table, key), id)
	require.NoError(t, err)
	versionID, err := result.LastInsertId()
	require.NoError(t, err)
	return versionID
}

func TestCurrentReferenceSiblingsAndComposedReadsUseOwnerDay(t *testing.T) {
	t.Parallel()
	// Relative to SQLite's real UTC day (testdb.RealUTCToday): east, the owner's
	// day is a week past it, so today's version is future to an unpinned read;
	// west, the owner's day is the one before it, so the owner's tomorrow is
	// SQLite's today.
	today := testdb.RealUTCToday()
	for _, tc := range []struct{ name, instant, zone, today string }{
		{"east local midnight", testdb.UTCDayInstant(today, 7, "23:01:00"), "Europe/Amsterdam", testdb.UTCDay(today, 8)},
		{"west UTC tomorrow", testdb.UTCDayInstant(today, 0, "03:00:00"), "America/Los_Angeles", testdb.UTCDay(today, -1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			now, err := time.Parse(time.RFC3339, tc.instant)
			require.NoError(t, err)
			clock := func() time.Time { return now }
			settings := NewSettingsService(db.NewSettingsRepository(f.database))
			settings.SetNowForTest(clock)
			_, err = settings.SavePreferences(ctx, SaveUserPreferencesInput{UserID: f.ownerUserID, TimeZone: tc.zone})
			require.NoError(t, err)
			institutions := NewInstitutionService(db.NewInstitutionRepository(f.database))
			categories := NewCategoryService(db.NewCategoryRepository(f.database), nil)
			payees := NewPayeeService(db.NewPayeeRepository(f.database), db.NewAccountRepository(f.database))
			currencies := NewCurrencyService(db.NewCommodityRepository(f.database), nil)
			for _, service := range []interface {
				SetSettings(*SettingsService)
				SetNowForTest(func() time.Time)
			}{f.accountService, institutions, categories, payees, currencies, f.investmentService} {
				service.SetSettings(settings)
				service.SetNowForTest(clock)
			}
			institution, err := institutions.CreateInstitution(ctx, CreateInstitutionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "institution.create", Name: "Original institution", Kind: "bank"})
			require.NoError(t, err)
			account, err := f.accountService.CreateAccount(ctx, CreateAccountInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create", Name: "Original account", AccountClass: "asset", AccountKind: "checking", DefaultCommodityID: &f.eurCommodityID, InstitutionID: &institution.ID})
			require.NoError(t, err)
			category, err := categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Original category", CategoryType: "expense"})
			require.NoError(t, err)
			payee, err := payees.CreatePayee(ctx, CreatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.create", Name: "Original payee", DefaultAccountID: &account.ID, DefaultCategoryAccountID: &category.ID})
			require.NoError(t, err)
			instrument, err := f.investmentService.CreateInstrument(ctx, InvestmentInstrumentInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.create", CommodityCode: "LOCAL", DisplayName: "Original instrument", InstrumentType: "stock", QuantityScale: 6, PriceScale: 4})
			require.NoError(t, err)
			date, err := time.Parse(time.DateOnly, tc.today)
			require.NoError(t, err)
			tomorrow := date.AddDate(0, 0, 1).Format(time.DateOnly)
			for _, item := range []struct {
				table, key, label string
				id                int64
			}{
				{"account_versions", "account_id", "name", account.ID},
				{"account_versions", "account_id", "name", category.ID},
				{"institution_versions", "institution_id", "name", institution.ID},
				{"payee_versions", "payee_id", "name", payee.ID},
				{"commodity_versions", "commodity_id", "name", f.eurCommodityID},
				{"investment_instrument_versions", "instrument_id", "display_name", instrument.ID},
			} {
				cloneReferenceVersion(t, f.database, item.table, item.key, item.id, tomorrow, item.label, "Future")
				cloneReferenceVersion(t, f.database, item.table, item.key, item.id, tc.today, item.label, "Owner current")
			}
			readAccount, err := f.accountService.Account(ctx, account.ID)
			require.NoError(t, err)
			assert.Equal(t, "Owner current", readAccount.Name)
			readInstitution, err := institutions.Institution(ctx, institution.ID)
			require.NoError(t, err)
			assert.Equal(t, "Owner current", readInstitution.Name)
			readCategory, err := categories.Category(ctx, category.ID)
			require.NoError(t, err)
			assert.Equal(t, "Owner current", readCategory.Name)
			readPayee, err := payees.Payee(ctx, payee.ID)
			require.NoError(t, err)
			assert.Equal(t, "Owner current", readPayee.Name)
			readInstrument, err := f.investmentService.Instrument(ctx, instrument.ID)
			require.NoError(t, err)
			assert.Equal(t, "Owner current", readInstrument.DisplayName)
			readCurrencies, err := currencies.ListCurrencies(ctx)
			require.NoError(t, err)
			require.Len(t, readCurrencies, 1)
			assert.Equal(t, "Owner current", readCurrencies[0].Name)

			// Explicit dates still resolve source history, even with a current
			// reference date on the same operation context.
			repository := db.NewTransactionRepository(f.database)
			rule, err := repository.PostingAccountRule(db.WithReferenceDate(ctx, tc.today), BookID, account.ID, tomorrow)
			require.NoError(t, err)
			assert.Equal(t, tomorrow, rule.EffectiveFrom)
			history, err := f.accountService.AccountVersions(ctx, account.ID)
			require.NoError(t, err)
			require.Len(t, history, 3)
			assert.Equal(t, "Original account", history[2].Name)

			// Page composition and export use the same day. They must not
			// silently drop a newly created account, nor choose its UTC-future
			// label. Currency/account amounts remain exact and untouched.
			budget := NewBudgetService(db.NewBudgetRepository(f.database), settings)
			budget.SetNowForTest(clock)
			month, err := budget.Month(ctx, tc.today[:7]+"-01")
			require.NoError(t, err)
			assert.Contains(t, month.Accounts, BudgetAccount{ID: account.ID, Name: "Owner current", AccountClass: "asset", AccountKind: "checking", Treatment: "on_budget"})
			referenceCtx, err := settings.referenceContext(ctx, now)
			require.NoError(t, err)
			reportNames, err := db.NewPayeeRepository(f.database).ReportPayeeNames(referenceCtx, BookID, []int64{payee.ID})
			require.NoError(t, err)
			assert.Equal(t, "Owner current", reportNames[payee.ID])
			workspace, err := db.NewPricingRepository(f.database).PriceManagementSnapshot(referenceCtx, BookID, tc.today, f.eurCommodityID, nil)
			require.NoError(t, err)
			assert.Contains(t, workspace.Commodities, db.PricingCommodityRecord{ID: f.eurCommodityID, Code: "EUR", Name: "Owner current", Kind: "currency", Status: "active"})
			exports := NewExportService(db.NewExportRepository(f.database))
			exports.SetSettings(settings)
			exports.SetNowForTest(clock)
			var bundle bytes.Buffer
			require.NoError(t, exports.WriteBundle(ctx, &bundle, ExportFilter{}))
			archive, err := zip.NewReader(bytes.NewReader(bundle.Bytes()), int64(bundle.Len()))
			require.NoError(t, err)
			checked := 0
			for _, file := range archive.File {
				if file.Name != "accounts.csv" && file.Name != "payees.csv" && file.Name != "commodities.csv" {
					continue
				}
				reader, err := file.Open()
				require.NoError(t, err)
				content, err := io.ReadAll(reader)
				require.NoError(t, reader.Close())
				require.NoError(t, err)
				assert.Contains(t, string(content), "Owner current", file.Name)
				assert.NotContains(t, string(content), "Future", file.Name)
				checked++
			}
			assert.Equal(t, 3, checked)
		})
	}
}

func TestReferenceDateRefreshesPreferencesWithoutLeakingAcrossOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	now := time.Date(2027, 1, 1, 23, 1, 0, 0, time.UTC)
	settings := NewSettingsService(db.NewSettingsRepository(f.database))
	f.accountService.SetSettings(settings)
	f.accountService.SetNowForTest(func() time.Time { return now })
	_, err := settings.SavePreferences(ctx, SaveUserPreferencesInput{UserID: f.ownerUserID, TimeZone: "Europe/Amsterdam"})
	require.NoError(t, err)
	account, err := f.accountService.CreateAccount(ctx, CreateAccountInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create", Name: "January first", AccountClass: "asset", AccountKind: "checking", DefaultCommodityID: &f.eurCommodityID, OpenedOn: "2027-01-01", EffectiveFrom: "2027-01-01"})
	require.NoError(t, err)
	cloneReferenceVersion(t, f.database, "account_versions", "account_id", account.ID, "2027-01-02", "name", "January second")
	eastContext, err := settings.referenceContext(ctx, now)
	require.NoError(t, err)
	read, err := f.accountService.Account(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, "January second", read.Name)
	_, err = settings.SavePreferences(ctx, SaveUserPreferencesInput{UserID: f.ownerUserID, TimeZone: "America/Los_Angeles"})
	require.NoError(t, err)
	read, err = f.accountService.Account(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, "January first", read.Name)
	pinned, err := f.accountService.repository.CurrentAccountByID(eastContext, BookID, account.ID)
	require.NoError(t, err)
	assert.Equal(t, "January second", pinned.Name.String, "an in-flight context keeps its own date")
	var before, after int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&before))
	_, err = f.accountService.ListAccounts(ctx, ListAccountsInput{})
	require.NoError(t, err)
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&after))
	assert.Equal(t, before, after, "current reads never update stored dates or history")
	_, err = f.database.Exec(`UPDATE user_preferences SET time_zone='Not/AZone' WHERE user_id=?`, f.ownerUserID)
	require.NoError(t, err)
	_, err = f.accountService.Account(ctx, account.ID)
	require.ErrorContains(t, err, "load owner time zone")
	_, err = f.database.Exec(`DELETE FROM user_preferences WHERE user_id=?`, f.ownerUserID)
	require.NoError(t, err)
	read, err = f.accountService.Account(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, "January first", read.Name, "absent preferences use UTC")
	now = now.Add(2 * time.Hour)
	read, err = f.accountService.Account(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, "January second", read.Name, "a new operation refreshes the clock")
}
