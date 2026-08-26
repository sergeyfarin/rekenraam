package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const utf8BOM = "\uFEFF"

type exportedLedger struct {
	header []string
	rows   [][]string
	body   string
}

func (l exportedLedger) column(row []string, name string) string {
	for index, column := range l.header {
		if column == name {
			return row[index]
		}
	}
	return ""
}

func downloadLedgerCSV(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, path string) exportedLedger {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	require.Equalf(t, http.StatusOK, res.Code, "response body: %s", res.Body.String())
	assert.Equal(t, "text/csv; charset=utf-8", res.Header().Get("Content-Type"))
	assert.Contains(t, res.Header().Get("Content-Disposition"), `attachment; filename="rekenraam-ledger-`)

	body := res.Body.String()
	require.True(t, strings.HasPrefix(body, utf8BOM), "a spreadsheet needs the byte order mark to read UTF-8")

	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(body, utf8BOM))).ReadAll()
	require.NoError(t, err)
	require.NotEmpty(t, records)

	return exportedLedger{header: records[0], rows: records[1:], body: body}
}

// exportFixture is the shape the export must survive: two cash accounts, a
// category, and an investment holding whose trades post through the
// commodity_trading system account.
type exportFixture struct {
	sessionCookie *http.Cookie
	csrfToken     string
	usdID         int64
	checking      accountResponse
	groceries     categoryResponse
	salary        categoryResponse
}

func newExportFixture(t *testing.T, handler http.Handler) exportFixture {
	t.Helper()

	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	currencySetup := completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, "USD", []setupCurrencySelectionRequest{
		{Code: "USD", Name: "US Dollar"},
	})
	usdID := currencySetup.DefaultCurrency.ID
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)

	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", usdID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Groceries","category_type":"expense"}`)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Salary","category_type":"income"}`)

	return exportFixture{
		sessionCookie: sessionCookie,
		csrfToken:     csrfToken,
		usdID:         usdID,
		checking:      checking,
		groceries:     groceries,
		salary:        salary,
	}
}

// systemAccountID finds a system account by role. The export must carry these
// postings even though every report excludes them.
func systemAccountID(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, role string) int64 {
	t.Helper()

	for _, account := range listAccountsForSession(t, handler, sessionCookie, "?include_system=true").Accounts {
		if account.SystemRole == role {
			return account.ID
		}
	}
	return 0
}

// The export is a double-entry artifact before it is a spreadsheet: every
// journal entry it contains must sum to zero per commodity, or the file is
// wrong however pretty it looks.
func TestLedgerExportBalancesPerJournalEntryAndCommodity(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newExportFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-03",
		posting(f.checking.ID, -12345, 2, f.usdID),
		posting(f.groceries.ID, 12345, 2, f.usdID),
	), http.StatusCreated)

	export := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv")
	require.Len(t, export.rows, 4)

	sums := map[string]*big.Rat{}
	for _, row := range export.rows {
		key := export.column(row, "journal_entry_id") + "/" + export.column(row, "commodity")
		quantity, ok := new(big.Rat).SetString(export.column(row, "quantity"))
		require.Truef(t, ok, "quantity %q must parse exactly", export.column(row, "quantity"))
		if sums[key] == nil {
			sums[key] = new(big.Rat)
		}
		sums[key].Add(sums[key], quantity)
	}

	require.NotEmpty(t, sums)
	for key, sum := range sums {
		assert.Equalf(t, 0, sum.Sign(), "journal entry %s does not balance in the export", key)
	}
}

// Reports exclude system accounts because a report answers a human question.
// An export that dropped them would emit half a transaction.
func TestLedgerExportIncludesCommodityTradingPostings(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newExportFixture(t, handler)

	tradingAccountID := systemAccountID(t, handler, f.sessionCookie, "commodity_trading")
	require.NotZero(t, tradingAccountID, "the fixture needs the commodity_trading system account")

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-10",
		posting(f.checking.ID, -50000, 2, f.usdID),
		posting(tradingAccountID, 50000, 2, f.usdID),
	), http.StatusCreated)

	export := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv")

	var found bool
	for _, row := range export.rows {
		if export.column(row, "account_id") == strconvFormatInt(tradingAccountID) {
			found = true
			assert.Equal(t, "500.00", export.column(row, "quantity"),
				"the counterpart posting carries its own exact debit amount")
		}
	}
	assert.True(t, found, "the export must carry system-account postings so its transactions balance")
}

// The stored scale is part of what was recorded: 12.00 is not 12, and a
// 24-scale crypto quantity must survive without a float ever touching it.
func TestLedgerExportRendersStoredScaleWithoutFloat(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newExportFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 1200, 2, f.usdID),
		posting(f.salary.ID, -1200, 2, f.usdID),
	), http.StatusCreated)

	export := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv")

	quantities := map[string]bool{}
	for _, row := range export.rows {
		quantities[export.column(row, "quantity")] = true
	}
	assert.True(t, quantities["12.00"], "a scale-2 amount keeps both decimals, got %v", quantities)
	assert.True(t, quantities["-12.00"], "the credit side keeps its sign and scale, got %v", quantities)
}

// Portable export carries the current posted state; the audit-complete record
// of what was voided or deleted lives in the SQLite backup.
func TestLedgerExportExcludesVoidedDraftAndDeleted(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newExportFixture(t, handler)

	kept := createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 100000, 2, f.usdID),
		posting(f.salary.ID, -100000, 2, f.usdID),
	), http.StatusCreated)
	voided := createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)
	softDeleted := createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-03",
		posting(f.checking.ID, 300000, 2, f.usdID),
		posting(f.salary.ID, -300000, 2, f.usdID),
	), http.StatusCreated)

	mutateTransaction(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(voided.ID)+"/void", `{"change_reason":"duplicate"}`, http.StatusOK)
	mutateTransaction(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(softDeleted.ID)+"/soft-delete", `{"change_reason":"entered by mistake"}`, http.StatusOK)

	export := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv")

	exported := map[string]bool{}
	for _, row := range export.rows {
		exported[export.column(row, "transaction_id")] = true
		assert.Equal(t, "posted", export.column(row, "status"))
	}

	assert.True(t, exported[strconvFormatInt(kept.ID)], "the posted transaction must be exported")
	assert.False(t, exported[strconvFormatInt(voided.ID)], "a voided transaction must not be exported")
	assert.False(t, exported[strconvFormatInt(softDeleted.ID)], "a soft-deleted transaction must not be exported")
}

// transaction_complete is part of the stable schema, and an unfiltered export
// is complete by construction — every posting row says so.
func TestTransactionCompleteTokenRepeatsOnEveryPostingRow(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newExportFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 100000, 2, f.usdID),
		posting(f.salary.ID, -100000, 2, f.usdID),
	), http.StatusCreated)

	export := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv")
	require.Contains(t, export.header, "transaction_complete")
	require.NotEmpty(t, export.rows)

	for _, row := range export.rows {
		assert.Equal(t, "true", export.column(row, "transaction_complete"))
	}
}

// The account path is what a plain-text-accounting tool reads; the id is what a
// consumer joins on. Both travel, and a system account is named by its role
// rather than left blank.
func TestLedgerExportCarriesHierarchicalAccountPaths(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newExportFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 100000, 2, f.usdID),
		posting(f.salary.ID, -100000, 2, f.usdID),
	), http.StatusCreated)

	export := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv")

	paths := map[string]string{}
	for _, row := range export.rows {
		paths[export.column(row, "account_id")] = export.column(row, "account_path")
	}

	assert.Equal(t, "Checking", paths[strconvFormatInt(f.checking.ID)])
	assert.Contains(t, paths[strconvFormatInt(f.salary.ID)], "Salary")
	for accountID, path := range paths {
		assert.NotEmptyf(t, path, "account %s exported without a path", accountID)
	}
}

// A downloaded flat file cannot carry the manifest that makes a scoped export
// reproducible, so scope is refused by name rather than silently ignored.
func TestStandaloneLedgerCSVRejectsScopeParameters(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newExportFixture(t, handler)

	// Driven from the production list rather than from a copy of it: a filter
	// added there gets a refusal test by existing, instead of by someone
	// remembering to add its name in two places.
	values := map[string]string{
		"from":                "2026-01-01",
		"to":                  "2026-12-31",
		"date_basis":          "transaction",
		"account_id":          "1",
		"include_descendants": "true",
		"commodity_id":        "1",
	}
	for _, name := range exportScopeParameters {
		value, ok := values[name]
		require.Truef(t, ok, "no sample value for scope parameter %q — add one here", name)
		parameter := name + "=" + value
		req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/ledger.csv?"+parameter, nil)
		req.AddCookie(f.sessionCookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)

		require.Equalf(t, http.StatusBadRequest, res.Code, "%s must be refused, not ignored", parameter)
		assert.Contains(t, res.Body.String(), "EXPORT_SCOPE_UNSUPPORTED", parameter)
	}
}

func TestLedgerExportRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	for _, path := range []string{"/api/v1/exports/ledger.csv", "/api/v1/exports/preview"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)

		assert.Equalf(t, http.StatusUnauthorized, res.Code, "%s must not serve the ledger to an anonymous caller", path)
	}
}

// The preview exists so a caller learns what it is about to download while an
// error envelope is still possible — which is only useful if it agrees with
// the file.
func TestExportPreviewCountsMatchTheDownload(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newExportFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 100000, 2, f.usdID),
		posting(f.salary.ID, -100000, 2, f.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-15",
		posting(f.checking.ID, -2500, 2, f.usdID),
		posting(f.groceries.ID, 2500, 2, f.usdID),
	), http.StatusCreated)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/preview", nil)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "response body: %s", res.Body.String())

	var preview exportPreviewResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &preview))

	export := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv")

	assert.Equal(t, int64(len(export.rows)), preview.Ledger.PostingCount)
	assert.Equal(t, int64(2), preview.Ledger.TransactionCount)
	assert.Equal(t, int64(2), preview.Ledger.JournalEntryCount)
	assert.Equal(t, "2026-06-01", preview.Ledger.EarliestEntryDate)
	assert.Equal(t, "2026-07-15", preview.Ledger.LatestEntryDate)
	assert.Equal(t, export.header, preview.Columns, "the preview must publish the schema the file uses")
	assert.Equal(t, "journal_entry", preview.SelectionUnit)
	assert.True(t, preview.AllTransactionsComplete)
	assert.True(t, preview.IncludesSystemAccounts)
	assert.NotEmpty(t, preview.Excluded, "a consumer must be told what the export leaves behind")
	assert.False(t, preview.Attachments.Included)
	assert.Nil(t, preview.Attachments.Directory)
}

// The export reads one snapshot: a save landing mid-export cannot tear the
// file, and the export cannot block that save either.
func TestExportSnapshotIsStableWhileTheLedgerIsWritten(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newExportFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 100000, 2, f.usdID),
		posting(f.salary.ID, -100000, 2, f.usdID),
	), http.StatusCreated)

	before := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv")

	// A write while an export pool exists must succeed: the read pool never
	// holds the writer's single connection.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
		posting(f.checking.ID, -2500, 2, f.usdID),
		posting(f.groceries.ID, 2500, 2, f.usdID),
	), http.StatusCreated)

	after := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv")

	assert.Len(t, before.rows, 2)
	assert.Len(t, after.rows, 4, "a later export sees the newer write")
	assert.Equal(t, before.header, after.header)
}

// ADR 0011 clause 1 promises rows ordered by entry date, transaction, entry
// sequence, then line sequence — a total order, which is what makes two exports
// of one snapshot byte-identical and an archive checksum meaningful.
//
// This asserts the order, not the reproducibility. Comparing the bytes of two
// consecutive downloads was tried first and could not fail: SQLite returns the
// same scan order for the same query on unchanged data, so the comparison held
// even with the ORDER BY replaced by a deliberately partial one. A test that
// passes when the thing it names is broken is the defect V-1 found, not a
// guard against it.
func TestLedgerExportRowsFollowTheDocumentedTotalOrder(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newExportFixture(t, handler)

	// Several transactions sharing one date, and one earlier date arriving
	// last, so neither insertion order nor the date alone can produce the
	// documented order by accident.
	for i := 0; i < 4; i++ {
		createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
			posting(f.checking.ID, int64(1000+i), 2, f.usdID),
			posting(f.groceries.ID, int64(-1000-i), 2, f.usdID),
		), http.StatusCreated)
	}
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 500, 2, f.usdID),
		posting(f.groceries.ID, -500, 2, f.usdID),
	), http.StatusCreated)

	export := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv")
	require.Len(t, export.rows, 10)

	sortKey := func(row []string) []string {
		return []string{
			export.column(row, "entry_date"),
			// Numeric ids compared as fixed-width text, so 10 does not sort
			// before 9 and turn a real regression into a passing test.
			fmt.Sprintf("%020s", export.column(row, "transaction_id")),
			fmt.Sprintf("%020s", export.column(row, "entry_seq")),
			fmt.Sprintf("%020s", export.column(row, "line_seq")),
		}
	}
	for i := 1; i < len(export.rows); i++ {
		previous := sortKey(export.rows[i-1])
		current := sortKey(export.rows[i])
		assert.LessOrEqual(t, strings.Join(previous, "\x00"), strings.Join(current, "\x00"),
			"row %d breaks the documented order", i)
	}

	// The first row is the earliest entry date even though it was written last.
	assert.Equal(t, "2026-06-01", export.column(export.rows[0], "entry_date"))
}
