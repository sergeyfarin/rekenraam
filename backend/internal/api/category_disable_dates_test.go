package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/migrations"
)

// #664. A category disabled effective on D writes an archived version with
// closed_on = D. closed_on is the last day the category is open, as it is for
// a closed account (#292): postings on D stay valid and correctable, a fresh
// posting on D or earlier is still accepted by the ledger, and a disable that
// would strand a posting after D is refused with that posting's date.

const (
	categoryDisableDay       = "2025-12-15"
	categoryDisableDayBefore = "2025-12-14"
	categoryDisableDayAfter  = "2025-12-16"
	// The category clock in the test handler is 2026-01-01, so this posting
	// is dated after the owner's today.
	categoryDisableFutureDay = "2026-02-01"
)

type categoryDisableFixture struct {
	handler  http.Handler
	database *sql.DB
	cookie   *http.Cookie
	csrf     string
	usd      int64
	checking accountResponse
	category categoryResponse
}

func newCategoryDisableFixture(t *testing.T) categoryDisableFixture {
	t.Helper()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	return categoryDisableFixture{
		handler:  handler,
		database: database,
		cookie:   cookie,
		csrf:     csrf,
		usd:      usd,
		checking: createLedgerAccountOpenedOn(t, handler, cookie, csrf, "Disable checking", "asset", "checking", usd, 2, "2025-01-01"),
		category: createCategoryForSession(t, handler, cookie, csrf, `{"name":"Disable groceries","category_type":"expense"}`),
	}
}

func (f categoryDisableFixture) spend(t *testing.T, date string, wantStatus int) transactionResponse {
	t.Helper()
	return createTransactionForSession(t, f.handler, f.cookie, f.csrf, balancedBody(date,
		posting(f.checking.ID, -1000, 2, f.usd), posting(f.category.ID, 1000, 2, f.usd)), wantStatus)
}

func (f categoryDisableFixture) spendOn(t *testing.T, categoryID int64, date string) {
	t.Helper()
	createTransactionForSession(t, f.handler, f.cookie, f.csrf, balancedBody(date,
		posting(f.checking.ID, -1000, 2, f.usd), posting(categoryID, 1000, 2, f.usd)), http.StatusCreated)
}

func (f categoryDisableFixture) disablePath() string {
	return "/api/v1/categories/" + strconvFormatInt(f.category.ID) + "/disable"
}

func (f categoryDisableFixture) versionCount(t *testing.T) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(1) FROM account_versions WHERE account_id = ?`, f.category.ID).Scan(&count))
	return count
}

func (f categoryDisableFixture) disableAuditCount(t *testing.T) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(1) FROM audit_events WHERE operation = 'category.disable'`).Scan(&count))
	return count
}

func TestCategoryDisableKeepsItsLastPostingDay(t *testing.T) {
	t.Parallel()
	f := newCategoryDisableFixture(t)
	f.spend(t, categoryDisableDayBefore, http.StatusCreated)
	onDay := f.spend(t, categoryDisableDay, http.StatusCreated)

	disabled := mutateCategory(t, f.handler, f.cookie, f.csrf, http.MethodPost, f.disablePath(),
		`{"effective_from":"`+categoryDisableDay+`"}`, http.StatusOK)
	assert.Equal(t, "archived", disabled.Status)
	assert.Equal(t, categoryDisableDay, disabled.ClosedOn)

	// Everyday pickers read active categories; the disabled one leaves them at once.
	for _, active := range listCategoriesForSession(t, f.handler, f.cookie, "?status=active").Categories {
		assert.NotEqual(t, f.category.ID, active.ID, "a disabled category leaves the active list immediately")
	}

	path := "/api/v1/transactions/" + strconvFormatInt(onDay.ID)
	line := func(index int) postingResponse { return onDay.JournalEntries[0].Postings[index] }

	t.Run("a keep-journal edit of a posting on the disable day succeeds", func(t *testing.T) {
		updated := mutateTransaction(t, f.handler, f.cookie, f.csrf, http.MethodPatch, path,
			`{"transaction_date":"`+categoryDisableDay+`","description":"Kept journal","keep_journal":true}`, http.StatusOK)
		assert.Equal(t, "Kept journal", updated.Description)
		assert.Equal(t, f.category.ID, updated.JournalEntries[0].Postings[1].AccountID)
	})

	t.Run("a full-spec edit naming the same lines succeeds", func(t *testing.T) {
		updated := mutateTransaction(t, f.handler, f.cookie, f.csrf, http.MethodPatch, path, `{
			"transaction_date":"`+categoryDisableDay+`","description":"Full spec",
			"journal_entries":[{"entry_date":"`+categoryDisableDay+`","postings":[
				{"line_key":"`+line(0).LineKey+`","account_id":`+strconvFormatInt(f.checking.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(f.usd)+`},
				{"line_key":"`+line(1).LineKey+`","account_id":`+strconvFormatInt(f.category.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(f.usd)+`}
			]}]
		}`, http.StatusOK)
		assert.Equal(t, "Full spec", updated.Description)
		assert.Equal(t, f.category.ID, updated.JournalEntries[0].Postings[1].AccountID)
		assert.Equal(t, "1000", updated.JournalEntries[0].Postings[1].QuantityValue.String())
	})

	t.Run("void and unvoid succeed", func(t *testing.T) {
		mutateTransaction(t, f.handler, f.cookie, f.csrf, http.MethodPost, path+"/void", `{"change_reason":"probe"}`, http.StatusOK)
		unvoided := mutateTransaction(t, f.handler, f.cookie, f.csrf, http.MethodPost, path+"/unvoid", `{"change_reason":"probe"}`, http.StatusOK)
		assert.Equal(t, "posted", unvoided.Status)
	})

	t.Run("a fresh posting on or before the disable day is accepted, after it is refused", func(t *testing.T) {
		f.spend(t, categoryDisableDay, http.StatusCreated)
		f.spend(t, categoryDisableDayBefore, http.StatusCreated)
		res := referenceDataRequest(f.handler, f.cookie, f.csrf, http.MethodPost, "/api/v1/transactions", balancedBody(categoryDisableDayAfter,
			posting(f.checking.ID, -1000, 2, f.usd), posting(f.category.ID, 1000, 2, f.usd)))
		require.Equalf(t, http.StatusBadRequest, res.Code, res.Body.String())
		assert.Equal(t, "VALIDATION_FAILED", apiErrorCode(t, res))
	})

	t.Run("the SQLite trigger agrees at the boundary", func(t *testing.T) {
		assert.NoError(t, copyPostingToDate(f.database, line(1).PostingLineID, categoryDisableDay))
		assert.ErrorContains(t, copyPostingToDate(f.database, line(1).PostingLineID, categoryDisableDayAfter),
			"posting account is not eligible on entry date")
	})
}

func TestCategoryDisableRefusesALaterPosting(t *testing.T) {
	t.Parallel()
	f := newCategoryDisableFixture(t)
	f.spend(t, categoryDisableDay, http.StatusCreated)
	later := f.spend(t, categoryDisableDayAfter, http.StatusCreated)
	future := f.spend(t, categoryDisableFutureDay, http.StatusCreated)
	versions, audits := f.versionCount(t), f.disableAuditCount(t)

	refuse := func(wantDate string) {
		t.Helper()
		res := referenceDataRequest(f.handler, f.cookie, f.csrf, http.MethodPost, f.disablePath(), `{"effective_from":"`+categoryDisableDay+`"}`)
		require.Equalf(t, http.StatusConflict, res.Code, res.Body.String())
		var body struct {
			Error struct {
				Code         string `json:"code"`
				ConflictRule string `json:"conflict_rule"`
				ConflictDate string `json:"conflict_date"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
		assert.Equal(t, "REFERENCE_DATED_CONFLICT", body.Error.Code)
		assert.Equal(t, "category_posting_after_disable", body.Error.ConflictRule)
		assert.Equal(t, wantDate, body.Error.ConflictDate, "the earliest blocked posting")
		assert.Equal(t, versions, f.versionCount(t), "a refused disable leaves no version")
		assert.Equal(t, audits, f.disableAuditCount(t), "a refused disable leaves no audit event")
		assert.Equal(t, "active", readCategoryForSession(t, f.handler, f.cookie, f.category.ID, http.StatusOK).Status)
	}

	refuse(categoryDisableDayAfter)

	// A voided transaction is not examined; the future-dated one still blocks.
	mutateTransaction(t, f.handler, f.cookie, f.csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(later.ID)+"/void", `{"change_reason":"wrong day"}`, http.StatusOK)
	refuse(categoryDisableFutureDay)

	// A soft-deleted transaction is not examined either.
	mutateTransaction(t, f.handler, f.cookie, f.csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(future.ID)+"/soft-delete", `{"change_reason":"not mine"}`, http.StatusOK)
	disabled := mutateCategory(t, f.handler, f.cookie, f.csrf, http.MethodPost, f.disablePath(), `{"effective_from":"`+categoryDisableDay+`"}`, http.StatusOK)
	assert.Equal(t, "archived", disabled.Status)
}

// Controls: the closing-day allowance is for categories only. An archived
// ordinary account, including a plain expense account without category
// metadata, still accepts nothing, and protected categories stay protected.
func TestCategoryDisableDayAllowanceLeavesOtherContracts(t *testing.T) {
	t.Parallel()
	f := newCategoryDisableFixture(t)

	t.Run("an archived ordinary account refuses its closing day", func(t *testing.T) {
		plain := createLedgerAccountNoCommodityOpenedOn(t, f, "Plain fees", "expense", "expense")
		accountPath := "/api/v1/accounts/" + strconvFormatInt(plain.ID)
		mutateAccountWithBody(t, f.handler, f.cookie, f.csrf, http.MethodPost, accountPath+"/close", `{"closed_on":"`+categoryDisableDay+`"}`, http.StatusOK)
		createTransactionForSession(t, f.handler, f.cookie, f.csrf, balancedBody(categoryDisableDay,
			posting(f.checking.ID, -1000, 2, f.usd), posting(plain.ID, 1000, 2, f.usd)), http.StatusCreated)
		// Archiving on the closing day would govern that posting and refuse it.
		res := referenceDataRequest(f.handler, f.cookie, f.csrf, http.MethodPost, accountPath+"/archive", `{"effective_from":"`+categoryDisableDay+`"}`)
		require.Equalf(t, http.StatusConflict, res.Code, res.Body.String())
		assert.Equal(t, "CONFLICT", apiErrorCode(t, res))
		mutateAccountWithBody(t, f.handler, f.cookie, f.csrf, http.MethodPost, accountPath+"/archive", `{"effective_from":"`+categoryDisableDayAfter+`"}`, http.StatusOK)
		createTransactionForSession(t, f.handler, f.cookie, f.csrf, balancedBody(categoryDisableDayAfter,
			posting(f.checking.ID, -1000, 2, f.usd), posting(plain.ID, 1000, 2, f.usd)), http.StatusBadRequest)
	})

	t.Run("Go validation and the SQLite trigger refuse an archived ordinary account on its closed_on", func(t *testing.T) {
		plain := createLedgerAccountNoCommodityOpenedOn(t, f, "Plain bank fees", "expense", "expense")
		onDay := createTransactionForSession(t, f.handler, f.cookie, f.csrf, balancedBody(categoryDisableDay,
			posting(f.checking.ID, -1000, 2, f.usd), posting(plain.ID, 1000, 2, f.usd)), http.StatusCreated)
		// The state a category disable writes, on an ordinary account: the API
		// refuses it, so it is written directly.
		insertAccountVersionCopy(t, f.database, plain.ID, "archived", categoryDisableDay)
		createTransactionForSession(t, f.handler, f.cookie, f.csrf, balancedBody(categoryDisableDay,
			posting(f.checking.ID, -1000, 2, f.usd), posting(plain.ID, 1000, 2, f.usd)), http.StatusBadRequest)
		assert.ErrorContains(t, copyPostingToDate(f.database, onDay.JournalEntries[0].Postings[1].PostingLineID, categoryDisableDay),
			"posting account is not eligible on entry date")
	})

	t.Run("a protected category still cannot be disabled", func(t *testing.T) {
		setup := mutateCategoriesSetup(t, f.handler, f.cookie, f.csrf, http.StatusCreated)
		other := categoryByCode(t, setup.Categories, "expense_other")
		require.True(t, other.IsBuiltin)
		f.spendOn(t, other.ID, categoryDisableDay)
		mutateCategory(t, f.handler, f.cookie, f.csrf, http.MethodPost, "/api/v1/categories/"+strconvFormatInt(other.ID)+"/disable",
			`{"effective_from":"`+categoryDisableDay+`"}`, http.StatusConflict)
		assert.Equal(t, "active", readCategoryForSession(t, f.handler, f.cookie, other.ID, http.StatusOK).Status)
	})
}

// The upgrade migration replaces the trigger on a database that already holds
// a category disabled on the day of its last posting.
func TestCategoryDisableDayMigrationUpgradesAnExistingDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newCategoryDisableFixture(t)
	onDay := f.spend(t, categoryDisableDay, http.StatusCreated)
	mutateCategory(t, f.handler, f.cookie, f.csrf, http.MethodPost, f.disablePath(), `{"effective_from":"`+categoryDisableDay+`"}`, http.StatusOK)
	lineID := onDay.JournalEntries[0].Postings[1].PostingLineID

	provider, err := goose.NewProvider(goose.DialectSQLite3, f.database, migrations.FS)
	require.NoError(t, err)
	_, err = provider.DownTo(ctx, 15)
	require.NoError(t, err)
	require.ErrorContains(t, copyPostingToDate(f.database, lineID, categoryDisableDay),
		"posting account is not eligible on entry date", "the baseline trigger refused the disable day")

	require.NoError(t, db.Migrate(ctx, f.database))
	assert.NoError(t, copyPostingToDate(f.database, lineID, categoryDisableDay))
	assert.ErrorContains(t, copyPostingToDate(f.database, lineID, categoryDisableDayAfter), "posting account is not eligible on entry date")
	mutateTransaction(t, f.handler, f.cookie, f.csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(onDay.ID)+"/void", `{"change_reason":"after upgrade"}`, http.StatusOK)
}

func createLedgerAccountNoCommodityOpenedOn(t *testing.T, f categoryDisableFixture, name string, accountClass string, accountKind string) accountResponse {
	t.Helper()
	return createAccountForSession(t, f.handler, f.cookie, f.csrf, `{
		"name":"`+name+`","account_class":"`+accountClass+`","account_kind":"`+accountKind+`","opened_on":"2025-01-01"
	}`)
}

// copyPostingToDate copies a posting line's latest version into a new journal
// entry dated date, straight through SQLite and rolled back, so the
// posting_versions_account_version_valid trigger decides alone.
func copyPostingToDate(database *sql.DB, postingLineID int64, date string) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var journalEntryID int64
	if err := tx.QueryRow(`SELECT journal_entry_id FROM posting_versions WHERE posting_line_id = ? ORDER BY id DESC LIMIT 1`, postingLineID).Scan(&journalEntryID); err != nil {
		return err
	}
	inserted, err := tx.Exec(`
		INSERT INTO journal_entries (book_id, transaction_version_id, entry_seq, entry_date, entry_kind, memo, metadata_json)
		SELECT book_id, transaction_version_id,
			(SELECT MAX(entry_seq) + 1 FROM journal_entries other WHERE other.transaction_version_id = je.transaction_version_id),
			?, entry_kind, memo, metadata_json
		FROM journal_entries je WHERE id = ?
	`, date, journalEntryID)
	if err != nil {
		return err
	}
	newEntryID, err := inserted.LastInsertId()
	if err != nil {
		return err
	}
	line, err := tx.Exec(`
		INSERT INTO posting_lines (book_id, transaction_id, line_key, created_at, created_by_user_id)
		SELECT book_id, transaction_id, line_key || '-probe', created_at, created_by_user_id
		FROM posting_lines WHERE id = ?
	`, postingLineID)
	if err != nil {
		return err
	}
	newLineID, err := line.LastInsertId()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO posting_versions (book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq,
			account_id, quantity_value, quantity_scale, commodity_id, memo, reconciliation_status, cleared_on, metadata_json)
		SELECT book_id, transaction_version_id, ?, ?, line_seq,
			account_id, quantity_value, quantity_scale, commodity_id, memo, reconciliation_status, cleared_on, metadata_json
		FROM posting_versions WHERE posting_line_id = ? ORDER BY id DESC LIMIT 1
	`, newEntryID, newLineID, postingLineID)
	return err
}

// A posting the category already refused before the disable is not stranded
// by it. Turning off postings effective D leaves a posting on D ineligible
// (#680); a disable on D must not report it as a posting after its date, and
// must not be refused for it (PR #681 review).
func TestCategoryDisableIgnoresPostingsAlreadyRefused(t *testing.T) {
	t.Parallel()
	f := newCategoryDisableFixture(t)
	f.spend(t, categoryDisableDay, http.StatusCreated)
	f.spend(t, categoryDisableDayAfter, http.StatusCreated)
	mutateCategory(t, f.handler, f.cookie, f.csrf, http.MethodPatch, "/api/v1/categories/"+strconvFormatInt(f.category.ID),
		`{"allows_postings":false,"effective_from":"`+categoryDisableDay+`"}`, http.StatusOK)

	disabled := mutateCategory(t, f.handler, f.cookie, f.csrf, http.MethodPost, f.disablePath(), `{"effective_from":"`+categoryDisableDay+`"}`, http.StatusOK)
	assert.Equal(t, "archived", disabled.Status)
}
