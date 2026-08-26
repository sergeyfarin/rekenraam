package api

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

type exportedBundle struct {
	files    map[string][]byte
	manifest struct {
		SchemaVersion              int    `json:"schema_version"`
		SelectionUnit              string `json:"selection_unit"`
		RecordPolicy               string `json:"record_policy"`
		IncludesSystemAccounts     bool   `json:"includes_system_accounts"`
		AllTransactionsComplete    bool   `json:"all_transactions_complete"`
		IncompleteTransactionCount int64  `json:"incomplete_transaction_count"`
		OutOfScopePostingsIncluded int64  `json:"out_of_scope_postings_included"`
		Query                      struct {
			From               string  `json:"from"`
			To                 string  `json:"to"`
			DateBasis          string  `json:"date_basis"`
			AccountIDs         []int64 `json:"account_ids"`
			ResolvedAccountIDs []int64 `json:"resolved_account_ids"`
			CommodityIDs       []int64 `json:"commodity_ids"`
		} `json:"query"`
		Excluded    []string `json:"excluded"`
		Attachments struct {
			Included  bool    `json:"included"`
			Directory *string `json:"directory"`
			Reason    string  `json:"reason"`
		} `json:"attachments"`
		Files []struct {
			Name   string `json:"name"`
			Rows   int64  `json:"rows"`
			Bytes  int64  `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
}

// table reads one CSV out of the archive as header + rows.
func (b exportedBundle) table(t *testing.T, name string) exportedLedger {
	t.Helper()

	raw, ok := b.files[name]
	require.Truef(t, ok, "%s is missing from the archive", name)
	require.True(t, bytes.HasPrefix(raw, []byte(utf8BOM)), "%s needs the byte order mark", name)

	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte(utf8BOM)))).ReadAll()
	require.NoError(t, err)
	require.NotEmptyf(t, records, "%s has no header", name)

	return exportedLedger{header: records[0], rows: records[1:]}
}

func downloadBundle(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, query string) exportedBundle {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/bundle.zip"+query, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	require.Equalf(t, http.StatusOK, res.Code, "response body: %s", res.Body.String())
	assert.Equal(t, "application/zip", res.Header().Get("Content-Type"))
	assert.Contains(t, res.Header().Get("Content-Disposition"), `attachment; filename="rekenraam-export-`)

	body := res.Body.Bytes()
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err, "a truncated archive must fail to open rather than look complete")

	bundle := exportedBundle{files: map[string][]byte{}}
	for _, file := range archive.File {
		reader, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(reader)
		require.NoError(t, reader.Close())
		require.NoError(t, err)
		bundle.files[file.Name] = content
	}

	manifest, ok := bundle.files["manifest.json"]
	require.True(t, ok, "the archive must describe itself")
	require.NoError(t, json.Unmarshal(manifest, &bundle.manifest))

	return bundle
}

func decimalColumn(t *testing.T, table exportedLedger, row []string, name string) *big.Rat {
	t.Helper()

	value, ok := new(big.Rat).SetString(table.column(row, name))
	require.Truef(t, ok, "%s = %q must parse exactly", name, table.column(row, name))
	return value
}

// trialBalanceFor returns the row for one account and commodity.
func trialBalanceFor(t *testing.T, table exportedLedger, accountID int64, commodityID int64) []string {
	t.Helper()

	for _, row := range table.rows {
		if table.column(row, "account_id") == strconvFormatInt(accountID) &&
			table.column(row, "commodity_id") == strconvFormatInt(commodityID) {
			return row
		}
	}
	t.Fatalf("no trial-balance row for account %d commodity %d", accountID, commodityID)
	return nil
}

// bundleFixture gives the trial balance something to be wrong about: two cash
// accounts, two categories, and — crucially — activity in the counterpart
// account that a filtered export must NOT claim to have accounted for.
type bundleFixture struct {
	exportFixture
	savings accountResponse
}

func newBundleFixture(t *testing.T, handler http.Handler) bundleFixture {
	t.Helper()

	base := newExportFixture(t, handler)
	savings := createLedgerAccount(t, handler, base.sessionCookie, base.csrfToken, "Savings", "asset", "savings", base.usdID, 2)

	return bundleFixture{exportFixture: base, savings: savings}
}

func TestExportBundleChecksumsVerify(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)

	bundle := downloadBundle(t, handler, f.sessionCookie, "")

	for _, name := range []string{
		"README.txt", "ledger.csv", "accounts.csv", "categories.csv", "payees.csv",
		"commodities.csv", "tags.csv", "lots.csv", "prices.csv", "trial-balance.csv", "manifest.json",
	} {
		assert.Containsf(t, bundle.files, name, "the archive must carry %s", name)
	}

	require.NotEmpty(t, bundle.manifest.Files)
	for _, file := range bundle.manifest.Files {
		content, ok := bundle.files[file.Name]
		require.Truef(t, ok, "manifest names %s, which the archive does not contain", file.Name)

		digest := sha256.Sum256(content)
		assert.Equalf(t, hex.EncodeToString(digest[:]), file.SHA256, "%s checksum does not match its content", file.Name)
		assert.Equalf(t, int64(len(content)), file.Bytes, "%s byte count is wrong", file.Name)
	}

	assert.Equal(t, 1, bundle.manifest.SchemaVersion)
	assert.Equal(t, "journal_entry", bundle.manifest.SelectionUnit)
	assert.True(t, bundle.manifest.IncludesSystemAccounts)
	assert.True(t, bundle.manifest.AllTransactionsComplete)
	assert.Zero(t, bundle.manifest.IncompleteTransactionCount)
	assert.NotEmpty(t, bundle.manifest.Excluded)
	assert.False(t, bundle.manifest.Attachments.Included)
	assert.Nil(t, bundle.manifest.Attachments.Directory)
	assert.Contains(t, string(bundle.files["README.txt"]), "sums to zero")
}

// Identity C: what the trial balance says this archive carries is what the
// archive actually carries.
func TestTrialBalanceInRangePlusOutOfRangeEqualsLedgerSum(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-04",
		posting(f.checking.ID, -12345, 2, f.usdID),
		posting(f.groceries.ID, 12345, 2, f.usdID),
	), http.StatusCreated)

	for _, query := range []string{"", "?from=2026-06-15&to=2026-12-31", "?account_id=" + strconvFormatInt(f.checking.ID)} {
		bundle := downloadBundle(t, handler, f.sessionCookie, query)
		ledger := bundle.table(t, "ledger.csv")
		trialBalance := bundle.table(t, "trial-balance.csv")

		ledgerSums := map[string]*big.Rat{}
		for _, row := range ledger.rows {
			key := ledger.column(row, "account_id") + "/" + ledger.column(row, "commodity_id")
			if ledgerSums[key] == nil {
				ledgerSums[key] = new(big.Rat)
			}
			ledgerSums[key].Add(ledgerSums[key], decimalColumn(t, ledger, row, "quantity"))
		}

		for _, row := range trialBalance.rows {
			key := trialBalance.column(row, "account_id") + "/" + trialBalance.column(row, "commodity_id")
			exported := new(big.Rat).Add(
				decimalColumn(t, trialBalance, row, "exported_in_range_movement"),
				decimalColumn(t, trialBalance, row, "exported_out_of_range_movement"),
			)
			want := ledgerSums[key]
			if want == nil {
				want = new(big.Rat)
			}
			assert.Equalf(t, want.RatString(), exported.RatString(),
				"query %q: trial balance and ledger.csv disagree for %s", query, key)
		}
	}
}

// Identities A and B, over the shape that broke the earlier design: an
// account-scoped export whose counterpart account has activity of its own that
// the export never sees.
func TestTrialBalanceIdentitiesHoldForAScopedExportWithCounterpartActivity(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	// Checking ↔ Salary: this is what the filter will ask for.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)
	// Savings ↔ Salary: unrelated activity in a counterpart account. The export
	// pulls Salary in as a counterpart but never sees this transaction.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-10",
		posting(f.savings.ID, 50000, 2, f.usdID),
		posting(f.salary.ID, -50000, 2, f.usdID),
	), http.StatusCreated)

	bundle := downloadBundle(t, handler, f.sessionCookie, "?account_id="+strconvFormatInt(f.checking.ID))
	trialBalance := bundle.table(t, "trial-balance.csv")

	for _, row := range trialBalance.rows {
		opening := decimalColumn(t, trialBalance, row, "opening_balance")
		exportedIn := decimalColumn(t, trialBalance, row, "exported_in_range_movement")
		excludedIn := decimalColumn(t, trialBalance, row, "excluded_in_range_movement")
		derived := decimalColumn(t, trialBalance, row, "derived_closing_balance")
		actual := decimalColumn(t, trialBalance, row, "actual_closing_balance")

		assert.Equal(t, new(big.Rat).Add(opening, exportedIn).RatString(), derived.RatString(),
			"identity A failed for account %s", trialBalance.column(row, "account_id"))
		assert.Equal(t, new(big.Rat).Add(derived, excludedIn).RatString(), actual.RatString(),
			"identity B failed for account %s", trialBalance.column(row, "account_id"))
	}

	// Salary is here only as a counterpart, and its derived figure is NOT its
	// closing balance — which is the whole reason the two columns are separate.
	salary := trialBalanceFor(t, trialBalance, f.salary.ID, f.usdID)
	assert.Equal(t, "false", trialBalance.column(salary, "in_scope"))
	assert.Equal(t, "-2000.00", trialBalance.column(salary, "derived_closing_balance"))
	assert.Equal(t, "-500.00", trialBalance.column(salary, "excluded_in_range_movement"))
	assert.Equal(t, "-2500.00", trialBalance.column(salary, "actual_closing_balance"))

	checking := trialBalanceFor(t, trialBalance, f.checking.ID, f.usdID)
	assert.Equal(t, "true", trialBalance.column(checking, "in_scope"))
	assert.Equal(t, bundle.manifest.OutOfScopePostingsIncluded, int64(1),
		"the salary counterpart is in the archive only because its entry was selected")
}

// The other shape that broke it: a transaction whose entries straddle the range
// boundary, exported under both date bases.
func TestStraddlingTransactionIsWholeUnderTransactionBasisAndPartialUnderEntryBasis(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	// One transaction, two entries, twelve days apart.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, `{
		"transaction_date":"2026-06-28",
		"journal_entries":[
			{"entry_date":"2026-06-28","postings":[
				`+posting(f.checking.ID, -30000, 2, f.usdID)+`,
				`+posting(f.savings.ID, 30000, 2, f.usdID)+`
			]},
			{"entry_date":"2026-07-10","postings":[
				`+posting(f.checking.ID, -10000, 2, f.usdID)+`,
				`+posting(f.groceries.ID, 10000, 2, f.usdID)+`
			]}
		]
	}`, http.StatusCreated)

	entryBasis := downloadBundle(t, handler, f.sessionCookie, "?from=2026-06-01&to=2026-06-30")
	entryLedger := entryBasis.table(t, "ledger.csv")
	require.Len(t, entryLedger.rows, 2, "entry basis takes only the in-range entry")
	for _, row := range entryLedger.rows {
		assert.Equal(t, "false", entryLedger.column(row, "transaction_complete"),
			"the export holds half of this transaction and must say so")
	}
	assert.False(t, entryBasis.manifest.AllTransactionsComplete)
	assert.Equal(t, int64(1), entryBasis.manifest.IncompleteTransactionCount)

	transactionBasis := downloadBundle(t, handler, f.sessionCookie, "?from=2026-06-01&to=2026-06-30&date_basis=transaction")
	transactionLedger := transactionBasis.table(t, "ledger.csv")
	require.Len(t, transactionLedger.rows, 4, "transaction basis takes every entry of the transaction")
	for _, row := range transactionLedger.rows {
		assert.Equal(t, "true", transactionLedger.column(row, "transaction_complete"))
	}
	assert.True(t, transactionBasis.manifest.AllTransactionsComplete)

	// The July entry is in the archive but outside the range, which is exactly
	// what the out-of-range column exists to report — and identity B must still
	// hold with it there.
	trialBalance := transactionBasis.table(t, "trial-balance.csv")
	groceries := trialBalanceFor(t, trialBalance, f.groceries.ID, f.usdID)
	assert.Equal(t, "100.00", trialBalance.column(groceries, "exported_out_of_range_movement"))
	// A figure that accumulated nothing takes the row's scale rather than
	// printing a bare "0" beside its neighbour's "0.00" — see
	// TestTrialBalanceZeroFiguresTakeTheRowScale for why that is safe.
	assert.Equal(t, "0.00", trialBalance.column(groceries, "exported_in_range_movement"))

	for _, row := range trialBalance.rows {
		derived := decimalColumn(t, trialBalance, row, "derived_closing_balance")
		excludedIn := decimalColumn(t, trialBalance, row, "excluded_in_range_movement")
		actual := decimalColumn(t, trialBalance, row, "actual_closing_balance")
		assert.Equal(t, new(big.Rat).Add(derived, excludedIn).RatString(), actual.RatString(),
			"identity B must survive a straddling transaction under transaction basis")
	}
}

// A filter widens to entry boundaries; it never cuts inside one, or the export
// stops balancing.
func TestFilteredExportKeepsWholeEntries(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)

	bundle := downloadBundle(t, handler, f.sessionCookie, "?account_id="+strconvFormatInt(f.checking.ID))
	ledger := bundle.table(t, "ledger.csv")
	require.Len(t, ledger.rows, 2, "the counterpart posting travels with its entry")

	sums := map[string]*big.Rat{}
	for _, row := range ledger.rows {
		key := ledger.column(row, "journal_entry_id") + "/" + ledger.column(row, "commodity")
		if sums[key] == nil {
			sums[key] = new(big.Rat)
		}
		sums[key].Add(sums[key], decimalColumn(t, ledger, row, "quantity"))
	}
	for key, sum := range sums {
		assert.Equalf(t, 0, sum.Sign(), "filtered export does not balance for entry %s", key)
	}
}

func TestBundleRejectsAnInvalidFilterBeforeStreaming(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	for query, wantMessage := range map[string]string{
		"?account_id=98765":              "account filter is invalid",
		"?commodity_id=98765":            "commodity filter is invalid",
		"?from=2026-13-01":               "from must use YYYY-MM-DD",
		"?from=2026-12-01&to=2026-01-01": "export range starts after it ends",
		"?date_basis=posting":            "date_basis must be entry or transaction",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/bundle.zip"+query, nil)
		req.AddCookie(f.sessionCookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)

		require.Equalf(t, http.StatusBadRequest, res.Code, "%s must be refused before a byte is written", query)
		assert.Contains(t, res.Body.String(), "VALIDATION_FAILED", query)
		assert.Contains(t, res.Body.String(), wantMessage, query)
		assert.Empty(t, res.Header().Get("Content-Disposition"), "a refused request must not look like a download")
	}
}

// An account filter means the subtree the user sees, the same way it does in a
// report — and the archive says what it resolved to.
func TestBundleExpandsAccountDescendantsWhenAsked(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	child := createAccountForSession(t, handler, f.sessionCookie, f.csrfToken, `{
		"name":"Checking Sub",
		"account_class":"asset",
		"account_kind":"checking",
		"default_commodity_id":`+strconvFormatInt(f.usdID)+`,
		"parent_account_id":`+strconvFormatInt(f.checking.ID)+`,
		"opened_on":"2026-01-01"
	}`)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-05",
		posting(child.ID, 25000, 2, f.usdID),
		posting(f.salary.ID, -25000, 2, f.usdID),
	), http.StatusCreated)

	without := downloadBundle(t, handler, f.sessionCookie, "?account_id="+strconvFormatInt(f.checking.ID))
	assert.Empty(t, without.table(t, "ledger.csv").rows, "the parent alone holds no postings")

	with := downloadBundle(t, handler, f.sessionCookie,
		"?account_id="+strconvFormatInt(f.checking.ID)+"&include_descendants=true")
	assert.Len(t, with.table(t, "ledger.csv").rows, 2, "the subtree brings the child's entry, counterpart included")
	assert.Contains(t, with.manifest.Query.ResolvedAccountIDs, child.ID,
		"the manifest must state what the filter resolved to, not only what was asked")
}

// accounts.csv is the complete set including categories; categories.csv is the
// income/expense subset restated. The overlap is deliberate and documented.
func TestBundleDimensionFilesCoverAccountsAndCategories(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)

	bundle := downloadBundle(t, handler, f.sessionCookie, "")
	accounts := bundle.table(t, "accounts.csv")
	categories := bundle.table(t, "categories.csv")

	accountIDs := map[string]bool{}
	for _, row := range accounts.rows {
		accountIDs[accounts.column(row, "account_id")] = true
	}
	assert.True(t, accountIDs[strconvFormatInt(f.checking.ID)])
	assert.True(t, accountIDs[strconvFormatInt(f.salary.ID)], "accounts.csv carries income accounts too")

	for _, row := range categories.rows {
		assert.Truef(t, accountIDs[categories.column(row, "account_id")],
			"every category must also appear in accounts.csv")
		assert.Containsf(t, []string{"income", "expense"}, categories.column(row, "category_type"),
			"categories.csv is the income/expense subset")
	}
	assert.Contains(t, string(bundle.files["README.txt"]), "Do not add the two together.")

	commodities := bundle.table(t, "commodities.csv")
	require.NotEmpty(t, commodities.rows)
	assert.Equal(t, []string{
		"commodity_id", "code", "kind", "name", "symbol", "standard_scale", "max_quantity_scale", "status",
	}, commodities.header)
}

// The four fields a beancount transform needs, present on every posting, plus
// the narration fallback rule stated once rather than inferred per row.
func TestBundleCarriesEveryFieldABeancountTransformNeeds(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)

	bundle := downloadBundle(t, handler, f.sessionCookie, "")
	ledger := bundle.table(t, "ledger.csv")
	require.NotEmpty(t, ledger.rows)

	for _, row := range ledger.rows {
		for _, required := range []string{"entry_date", "account_path", "quantity", "commodity"} {
			assert.NotEmptyf(t, ledger.column(row, required), "%s is required for every posting", required)
		}
		// Narration derives from description, falling back to payee, falling
		// back to empty — a documented rule, not an accident of which field
		// happened to be filled. Optional fields stay optional.
		narration := ledger.column(row, "description")
		if narration == "" {
			narration = ledger.column(row, "payee")
		}
		assert.NotNil(t, narration)
	}
}

// Scale policy, half one: a figure that accumulated nothing takes the row's
// deepest scale.
//
// Zero is the same number at every scale, so deepening it multiplies zero by a
// power of ten and cannot widen anything — which is exactly why this half is
// safe while restating *non-zero* figures at a common scale is not. The rule is
// the one cashflow's own view already applies to an absent measure.
func TestTrialBalanceZeroFiguresTakeTheRowScale(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)

	bundle := downloadBundle(t, handler, f.sessionCookie, "")
	trialBalance := bundle.table(t, "trial-balance.csv")
	checking := trialBalanceFor(t, trialBalance, f.checking.ID, f.usdID)

	// Unfiltered: nothing is excluded and nothing is out of range, so those two
	// columns accumulated nothing at all.
	assert.Equal(t, "2000.00", trialBalance.column(checking, "exported_in_range_movement"))
	assert.Equal(t, "0.00", trialBalance.column(checking, "excluded_in_range_movement"))
	assert.Equal(t, "0.00", trialBalance.column(checking, "exported_out_of_range_movement"))
	assert.Equal(t, "0.00", trialBalance.column(checking, "opening_balance"))
}

// Scale policy, half two: a figure that accumulated something keeps its own
// scale, so the precision of one figure never widens another.
func TestTrialBalanceKeepsEachFigureAtTheScaleItAccumulated(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	// Savings takes postings at two different scales; checking only at one.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.savings.ID, 100000, 2, f.usdID),
		posting(f.salary.ID, -100000, 2, f.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
		posting(f.checking.ID, 5, 0, f.usdID),
		posting(f.salary.ID, -5, 0, f.usdID),
	), http.StatusCreated)

	bundle := downloadBundle(t, handler, f.sessionCookie, "")
	trialBalance := bundle.table(t, "trial-balance.csv")

	checking := trialBalanceFor(t, trialBalance, f.checking.ID, f.usdID)
	assert.Equal(t, "5", trialBalance.column(checking, "exported_in_range_movement"),
		"a scale-0 account keeps scale 0: it is not deepened to match an unrelated account")

	savings := trialBalanceFor(t, trialBalance, f.savings.ID, f.usdID)
	assert.Equal(t, "1000.00", trialBalance.column(savings, "exported_in_range_movement"))
}

// The boundary case from the cashflow scale revert (04a1d354), aimed at the
// trial balance: a 38-digit posting and a 2.50 posting in the same account force
// their sum to 40 digits.
//
// Both postings are legal, and the sum is exact — it simply does not fit the
// 38-digit ceiling that governs *stored* coefficients. Rendering it through
// that gate used to fail the whole archive after the response had begun, so the
// caller received an HTTP 200 carrying zero bytes. An export exists to get data
// out; it does not refuse a figure it can write exactly.
func TestTrialBalanceRendersASumWiderThanAStoredCoefficient(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	wide := strings.Repeat("9", 38)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, `{
		"transaction_date":"2026-06-01",
		"journal_entries":[{"entry_date":"2026-06-01","postings":[
			{"account_id":`+strconvFormatInt(f.checking.ID)+`,"quantity_value":"`+wide+`","quantity_scale":0,"commodity_id":`+strconvFormatInt(f.usdID)+`},
			{"account_id":`+strconvFormatInt(f.salary.ID)+`,"quantity_value":"-`+wide+`","quantity_scale":0,"commodity_id":`+strconvFormatInt(f.usdID)+`}
		]}]
	}`, http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
		posting(f.checking.ID, 250, 2, f.usdID),
		posting(f.salary.ID, -250, 2, f.usdID),
	), http.StatusCreated)

	bundle := downloadBundle(t, handler, f.sessionCookie, "")
	trialBalance := bundle.table(t, "trial-balance.csv")
	checking := trialBalanceFor(t, trialBalance, f.checking.ID, f.usdID)

	// (10^38 - 1) + 2.50, stated as arithmetic rather than as a hand-copied
	// literal: the sum carries into a 39th digit, which is precisely the width
	// a stored coefficient may not have.
	wideValue, ok := new(big.Rat).SetString(wide)
	require.True(t, ok)
	want := new(big.Rat).Add(wideValue, big.NewRat(250, 100))

	for _, column := range []string{"actual_closing_balance", "derived_closing_balance"} {
		rendered := trialBalance.column(checking, column)
		assert.Equal(t, want.RatString(), decimalColumn(t, trialBalance, checking, column).RatString(),
			"%s must be the exact sum", column)
		assert.Greater(t, len(strings.TrimSuffix(strings.ReplaceAll(rendered, ".", ""), "")), 38,
			"%s is wider than a stored coefficient may be, which is the point of this case", column)
	}

	// And the identities still hold on a figure that wide.
	opening := decimalColumn(t, trialBalance, checking, "opening_balance")
	exportedIn := decimalColumn(t, trialBalance, checking, "exported_in_range_movement")
	derived := decimalColumn(t, trialBalance, checking, "derived_closing_balance")
	assert.Equal(t, new(big.Rat).Add(opening, exportedIn).RatString(), derived.RatString())
}

// --- T-67: the two money-bearing files that no fixture had ever populated ---
//
// lots.csv and prices.csv were written by every bundle and were always empty,
// so their columns, their ordering, and their scale handling had never been
// exercised. An export putting cost basis in the wrong column would have passed
// the whole suite.

func TestBundleLotsFileCarriesCostBasisAtItsOwnScale(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "VWRL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)

	bundle := downloadBundle(t, handler, f.sessionCookie, "")
	lots := bundle.table(t, "lots.csv")

	require.Equal(t, []string{
		"lot_id", "account_id", "account_path", "commodity_id", "opened_on", "status",
		"quantity", "remaining_quantity", "cost_basis", "remaining_cost_basis",
		"cost_commodity_id", "source_transaction_id",
	}, lots.header)
	require.Len(t, lots.rows, 1)

	row := lots.rows[0]
	assert.Equal(t, strconvFormatInt(holding.ID), lots.column(row, "account_id"))
	assert.Equal(t, "open", lots.column(row, "status"))
	assert.Equal(t, "10", lots.column(row, "quantity"))
	assert.Equal(t, "10", lots.column(row, "remaining_quantity"))
	// The buy cost 1000.00 in cash, recorded at the cash commodity's scale —
	// not the instrument's six-place quantity scale, which is the confusion
	// this file exists to make impossible.
	assert.Equal(t, "1000.00", lots.column(row, "cost_basis"))
	assert.Equal(t, "1000.00", lots.column(row, "remaining_cost_basis"))
	assert.NotEmpty(t, lots.column(row, "account_path"))
	assert.NotEmpty(t, lots.column(row, "source_transaction_id"),
		"a lot must point at the transaction that opened it")

	// A partial sale leaves the original quantity intact and reduces only what
	// remains — the distinction cost-basis reporting depends on.
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell",
		investmentTradeRequest{
			TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
			CashAccountID: f.cashAccount.ID, QuantityValue: exact.New(4), QuantityScale: 0,
			CashAmountValue: 60000, CashAmountScale: 2, CashCommodityID: f.commodityID,
		}, http.StatusCreated)

	afterSale := downloadBundle(t, handler, f.sessionCookie, "").table(t, "lots.csv")
	require.Len(t, afterSale.rows, 1)
	assert.Equal(t, "10", afterSale.column(afterSale.rows[0], "quantity"))
	assert.Equal(t, "6", afterSale.column(afterSale.rows[0], "remaining_quantity"))
	assert.Equal(t, "1000.00", afterSale.column(afterSale.rows[0], "cost_basis"))
	assert.Equal(t, "600.00", afterSale.column(afterSale.rows[0], "remaining_cost_basis"))
}

func TestBundlePricesFileCarriesObservationsAtTheirOwnScale(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "VWRL")
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices",
		map[string]any{
			"base_commodity_id":  instrument.CommodityID,
			"quote_commodity_id": f.commodityID,
			"valuation_date":     "2026-02-01",
			"price_value":        1234567,
			"price_scale":        4,
			"quote_type":         "manual",
			// is_manual is its own field rather than something inferred from
			// quote_type, so the export has to carry what was recorded, not
			// what the type implies.
			"is_manual": true,
		}, http.StatusCreated)

	bundle := downloadBundle(t, handler, f.sessionCookie, "")
	prices := bundle.table(t, "prices.csv")

	require.Equal(t, []string{
		"base_commodity_id", "quote_commodity_id", "valuation_date", "price",
		"base_quantity", "quote_type", "adjustment_basis", "is_manual", "is_derived", "source",
	}, prices.header)
	require.NotEmpty(t, prices.rows)

	var found bool
	for _, row := range prices.rows {
		if prices.column(row, "valuation_date") != "2026-02-01" {
			continue
		}
		found = true
		// Scale 4, written at scale 4: a price is not money at two decimals.
		assert.Equal(t, "123.4567", prices.column(row, "price"))
		assert.Equal(t, "1", prices.column(row, "base_quantity"))
		assert.Equal(t, "manual", prices.column(row, "quote_type"))
		assert.Equal(t, "true", prices.column(row, "is_manual"))
		assert.Equal(t, "false", prices.column(row, "is_derived"))
	}
	assert.True(t, found, "the recorded observation must be in the export")
}
