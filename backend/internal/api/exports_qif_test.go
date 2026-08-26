package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
)

type qifDownload struct {
	contentType string
	filename    string
	body        []byte
}

func (d qifDownload) archive(t *testing.T) map[string][]byte {
	t.Helper()

	archive, err := zip.NewReader(bytes.NewReader(d.body), int64(len(d.body)))
	require.NoError(t, err)

	files := map[string][]byte{}
	for _, file := range archive.File {
		reader, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(reader)
		require.NoError(t, reader.Close())
		require.NoError(t, err)
		files[file.Name] = content
	}
	return files
}

// records splits a QIF file into its ^-terminated records.
func qifRecords(t *testing.T, content []byte) []map[string][]string {
	t.Helper()

	var records []map[string][]string
	current := map[string][]string{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if line == "^" {
			if len(current) > 0 {
				records = append(records, current)
				current = map[string][]string{}
			}
			continue
		}
		if strings.HasPrefix(line, "!") {
			current["!"] = append(current["!"], line)
			continue
		}
		code := line[:1]
		current[code] = append(current[code], line[1:])
	}
	return records
}

func downloadQIF(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, query string, wantStatus int) qifDownload {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/qif"+query, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	require.Equalf(t, wantStatus, res.Code, "response body: %s", res.Body.String())

	return qifDownload{
		contentType: res.Header().Get("Content-Type"),
		filename:    res.Header().Get("Content-Disposition"),
		body:        res.Body.Bytes(),
	}
}

// reimportQIF feeds an exported file back through the app's own QIF adapter,
// which is the only reader whose behaviour this repo can pin.
func reimportQIF(t *testing.T, content []byte, profileConfig string) app.ParseResult {
	t.Helper()

	adapter := &app.QIFAdapter{}
	var profile *app.ImportProfile
	if profileConfig != "" {
		profile = &app.ImportProfile{ConfigJSON: profileConfig}
	}
	result, err := adapter.Parse(context.Background(), app.RawInput{Filename: "export.qif", Bytes: content}, profile)
	require.NoError(t, err)
	return result
}

// The supported path: a file re-read under the layout it declares comes back
// unchanged. Everything else in this file is about the cases where that
// guarantee cannot be given.
func TestQIFRoundTripsUnderTheDeclaredLayout(t *testing.T) {
	t.Parallel()

	for _, layout := range []string{"mdy", "dmy"} {
		handler, _ := newSetupTestHandler(t)
		f := newBundleFixture(t, handler)

		createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
			posting(f.checking.ID, 200000, 2, f.usdID),
			posting(f.salary.ID, -200000, 2, f.usdID),
		), http.StatusCreated)
		// A day past the 12th, so the file is decisive and travels bare.
		createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-23",
			posting(f.checking.ID, -12345, 2, f.usdID),
			posting(f.groceries.ID, 12345, 2, f.usdID),
		), http.StatusCreated)

		download := downloadQIF(t, handler, f.sessionCookie,
			"?account_id="+strconvFormatInt(f.checking.ID)+"&qif_date_layout="+layout, http.StatusOK)
		require.Equal(t, "application/qif", download.contentType, layout)

		profile := `{"date_layout":"` + layout + `","decimal_separator":"."}`
		result := reimportQIF(t, download.body, profile)
		require.Len(t, result.Rows, 2, layout)

		byDate := map[string]string{}
		for _, row := range result.Rows {
			byDate[row.Date] = row.Amount
		}
		assert.Equal(t, "2000.00", byDate["2026-06-01"], "layout %s lost an amount", layout)
		assert.Equal(t, "-123.45", byDate["2026-07-23"], "layout %s lost an amount", layout)
	}
}

// Auto-detection works when the file contains a date that cannot be read the
// other way round — which is exactly what the bare-file rule guarantees.
func TestQIFAutoDetectionRoundTripsWithADecisiveDate(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-23",
		posting(f.checking.ID, -5000, 2, f.usdID),
		posting(f.groceries.ID, 5000, 2, f.usdID),
	), http.StatusCreated)

	for _, layout := range []string{"mdy", "dmy"} {
		download := downloadQIF(t, handler, f.sessionCookie,
			"?account_id="+strconvFormatInt(f.checking.ID)+"&qif_date_layout="+layout, http.StatusOK)

		// No profile at all: the adapter detects the layout from the file.
		result := reimportQIF(t, download.body, "")
		require.Len(t, result.Rows, 1, layout)
		assert.Equal(t, "2026-07-23", result.Rows[0].Date, "auto-detection misread a decisive %s file", layout)
		assert.Equal(t, "-50.00", result.Rows[0].Amount, layout)
	}
}

// A bare file has only its name to state the layout in, so it must.
func TestBareQIFFilenameStatesTheLayout(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-23",
		posting(f.checking.ID, -5000, 2, f.usdID),
		posting(f.groceries.ID, 5000, 2, f.usdID),
	), http.StatusCreated)

	for _, layout := range []string{"mdy", "dmy"} {
		download := downloadQIF(t, handler, f.sessionCookie,
			"?account_id="+strconvFormatInt(f.checking.ID)+"&qif_date_layout="+layout, http.StatusOK)
		assert.Equal(t, "application/qif", download.contentType)
		assert.Contains(t, download.filename, "-"+layout+"-", "the filename is the only place a bare file can say this")
		assert.Contains(t, download.filename, ".qif")
	}
}

// A file whose every date is ambiguous cannot say how it was written, so it is
// never handed over alone: it comes as an archive whose manifest and README
// both state the layout.
func TestAmbiguousOnlyQIFIsAlwaysArchived(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	// Every day is on or before the 12th: valid under both layouts.
	for _, date := range []string{"2026-06-01", "2026-07-05", "2026-08-12"} {
		createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody(date,
			posting(f.checking.ID, -1000, 2, f.usdID),
			posting(f.groceries.ID, 1000, 2, f.usdID),
		), http.StatusCreated)
	}

	download := downloadQIF(t, handler, f.sessionCookie,
		"?account_id="+strconvFormatInt(f.checking.ID)+"&qif_date_layout=dmy", http.StatusOK)
	require.Equal(t, "application/zip", download.contentType,
		"an undecidable file must not travel without the metadata that explains it")

	files := download.archive(t)
	require.Contains(t, files, "README.txt")
	require.Contains(t, files, "manifest.json")
	assert.Contains(t, string(files["README.txt"]), "DD/MM/YYYY")

	var manifest struct {
		DateLayout  string `json:"date_layout"`
		DecimalMark string `json:"decimal_mark"`
		Files       []struct {
			Name string `json:"name"`
			Rows int64  `json:"rows"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal(files["manifest.json"], &manifest))
	assert.Equal(t, "dmy", manifest.DateLayout)
	assert.Equal(t, ".", manifest.DecimalMark)
	require.Len(t, manifest.Files, 1)
	assert.Contains(t, manifest.Files[0].Name, "-dmy.qif", "the member filename states it too")

	// This file was rendered before the archive existed, to find out whether it
	// could travel bare. Its count has to survive that detour: a manifest that
	// says a non-empty file holds no records describes something that is not in
	// the archive.
	assert.Equal(t, int64(3), manifest.Files[0].Rows)
	assert.Len(t, qifRecords(t, files[manifest.Files[0].Name]), 3)
}

// Grouping is what makes a written amount ambiguous, so nothing is grouped.
func TestQIFExportNeverWritesGroupSeparators(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-23",
		posting(f.checking.ID, 123456789, 2, f.usdID),
		posting(f.salary.ID, -123456789, 2, f.usdID),
	), http.StatusCreated)

	download := downloadQIF(t, handler, f.sessionCookie,
		"?account_id="+strconvFormatInt(f.checking.ID), http.StatusOK)

	records := qifRecords(t, download.body)
	require.Len(t, records, 1)
	assert.Equal(t, []string{"1234567.89"}, records[0]["T"])
	assert.Equal(t, []string{"1234567.89"}, records[0]["U"], "Quicken reads U; the two must agree")
	assert.NotContains(t, string(download.body), "1,234,567", "a group separator invites a decimal-comma misread")
}

// The honest limit of the decimal story, pinned rather than claimed away.
//
// Rekenraam writes a point decimal mark. Read back under a comma-decimal
// profile, ordinary two-decimal money does not parse — "1234.56" would be a
// four-digit leading group, which is not valid grouping — so the row arrives
// carrying its raw text and a warning instead of a number that is wrong by
// 100x. Three-decimal money has no such luck: "1.234" *is* well-formed
// grouping under that profile and becomes 1234, silently. Rekenraam supports
// three-decimal currencies, so the corner is real, and the only mitigation is
// the out-of-band statement of the decimal mark that the archive, the filename,
// and the README all carry.
func TestCommaProfileFailsOnTwoDecimalMoneyAndIsUndetectableOnThreeDecimal(t *testing.T) {
	t.Parallel()

	commaProfile := `{"date_layout":"mdy","decimal_separator":","}`

	twoDecimals := reimportQIF(t, []byte("!Type:Bank\nD07/23/2026\nT1234.56\n^\n"), commaProfile)
	require.Len(t, twoDecimals.Rows, 1)
	assert.Equal(t, "1234.56", twoDecimals.Rows[0].Amount,
		"the raw text survives unconverted rather than becoming a number that is wrong by 100x")
	require.NotEmpty(t, twoDecimals.Warnings, "and the reader is told")
	assert.Contains(t, twoDecimals.Warnings[0].Message, "unrecognized amount")

	threeDecimals := reimportQIF(t, []byte("!Type:Bank\nD07/23/2026\nT1.234\n^\n"), commaProfile)
	require.Len(t, threeDecimals.Rows, 1)
	assert.Equal(t, "1234", threeDecimals.Rows[0].Amount,
		"three-decimal money read under a comma profile is valid grouping and cannot be detected")
	for _, warning := range threeDecimals.Warnings {
		assert.NotContains(t, warning.Message, "unrecognized amount",
			"nothing warns here, which is exactly what makes this corner dangerous")
	}
}

// A selection containing an account QIF cannot express is refused by name, not
// downloaded silently short.
func TestQIFExportRefusesMixedSelectionWithoutAcknowledgement(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	holding := createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "Brokerage", "asset", "brokerage", f.usdID, 2)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-23",
		posting(f.checking.ID, -5000, 2, f.usdID),
		posting(f.groceries.ID, 5000, 2, f.usdID),
	), http.StatusCreated)

	query := "?account_id=" + strconvFormatInt(f.checking.ID) + "&account_id=" + strconvFormatInt(holding.ID)
	refused := downloadQIF(t, handler, f.sessionCookie, query, http.StatusUnprocessableEntity)
	assert.Contains(t, string(refused.body), "QIF_ACCOUNT_UNSUPPORTED")
	assert.Contains(t, string(refused.body), "Brokerage", "the reader must be told which account is being dropped")
	assert.Contains(t, string(refused.body), "investment_account")
	assert.Empty(t, refused.filename, "a refusal must not look like a download")

	// Acknowledged, the same request downloads — as an archive, because the
	// omission has to travel with the file.
	accepted := downloadQIF(t, handler, f.sessionCookie, query+"&allow_partial=true", http.StatusOK)
	assert.Equal(t, "application/zip", accepted.contentType)

	files := accepted.archive(t)
	assert.Contains(t, string(files["README.txt"]), "ACCOUNTS NOT WRITTEN")
	assert.Contains(t, string(files["README.txt"]), "Brokerage")
}

// Partial approval leaving exactly one writable account still returns an
// archive: the omission metadata must travel with the file, not live in a
// dialog the reader already dismissed.
func TestSingleSupportedAccountAfterPartialApprovalStillReturnsAnArchive(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	holding := createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "Brokerage", "asset", "brokerage", f.usdID, 2)

	// A decisive date, so the only thing forcing an archive is the omission.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-23",
		posting(f.checking.ID, -5000, 2, f.usdID),
		posting(f.groceries.ID, 5000, 2, f.usdID),
	), http.StatusCreated)

	query := "?account_id=" + strconvFormatInt(f.checking.ID) +
		"&account_id=" + strconvFormatInt(holding.ID) + "&allow_partial=true"
	download := downloadQIF(t, handler, f.sessionCookie, query, http.StatusOK)

	assert.Equal(t, "application/zip", download.contentType)
	files := download.archive(t)

	var qifMembers []string
	for name := range files {
		if strings.HasSuffix(name, ".qif") {
			qifMembers = append(qifMembers, name)
		}
	}
	require.Len(t, qifMembers, 1, "one account was writable")

	var manifest struct {
		Included []struct {
			AccountPath string `json:"account_path"`
		} `json:"included_accounts"`
		Excluded []struct {
			AccountPath string `json:"account_path"`
			Reason      string `json:"reason"`
		} `json:"excluded_accounts"`
	}
	require.NoError(t, json.Unmarshal(files["manifest.json"], &manifest))
	require.Len(t, manifest.Excluded, 1)
	assert.Equal(t, "Brokerage", manifest.Excluded[0].AccountPath)
	assert.Equal(t, "investment_account", manifest.Excluded[0].Reason)
}

// Transfers and categories are different facts, and QIF has different syntax
// for each. Getting this wrong turns a move between accounts into spending.
func TestQIFWritesTransfersInBracketsAndCategoriesByPath(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-23",
		posting(f.checking.ID, -30000, 2, f.usdID),
		posting(f.savings.ID, 30000, 2, f.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-24",
		posting(f.checking.ID, -2500, 2, f.usdID),
		posting(f.groceries.ID, 2500, 2, f.usdID),
	), http.StatusCreated)

	download := downloadQIF(t, handler, f.sessionCookie,
		"?account_id="+strconvFormatInt(f.checking.ID), http.StatusOK)

	records := qifRecords(t, download.body)
	require.Len(t, records, 2)

	byDate := map[string][]string{}
	for _, record := range records {
		byDate[record["D"][0]] = record["L"]
	}
	assert.Equal(t, []string{"[Savings]"}, byDate["07/23/2026"], "a move between accounts is a transfer")
	assert.Equal(t, []string{"Groceries"}, byDate["07/24/2026"], "a category is written by its path")

	reimported := reimportQIF(t, download.body, `{"date_layout":"mdy","decimal_separator":"."}`)
	require.Len(t, reimported.Rows, 2)
}

// A split entry's amounts must sum to the record's own amount, or the file is
// arithmetically wrong wherever it is opened.
func TestQIFSplitAmountsSumToTheRecordAmount(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-23",
		posting(f.checking.ID, -12000, 2, f.usdID),
		posting(f.groceries.ID, 10000, 2, f.usdID),
		posting(f.salary.ID, 2000, 2, f.usdID),
	), http.StatusCreated)

	download := downloadQIF(t, handler, f.sessionCookie,
		"?account_id="+strconvFormatInt(f.checking.ID), http.StatusOK)

	records := qifRecords(t, download.body)
	require.Len(t, records, 1)
	assert.Equal(t, []string{"-120.00"}, records[0]["T"])
	assert.Equal(t, []string{"-100.00", "-20.00"}, records[0]["$"],
		"each split carries this account's side of that counterpart, so they sum to T")
	assert.Len(t, records[0]["S"], 2)
}

// An exchange has a side QIF cannot express. Saying so in the memo is honest;
// inventing a rate would not be.
func TestQIFStatesAForeignCounterpartInTheMemoRatherThanConvertingIt(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	eur := createCurrencyForSession(t, handler, f.sessionCookie, f.csrfToken, `{"code":"EUR","name":"Euro"}`)
	eurAccount := createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "Euro Account", "asset", "checking", eur.ID, 2)

	// An exchange balances per commodity through commodity_trading: the USD
	// legs balance each other and so do the EUR legs, which is why the entry is
	// legal and why QIF cannot state it in one currency.
	trading := systemAccountID(t, handler, f.sessionCookie, "commodity_trading")
	require.NotZero(t, trading)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, `{
		"transaction_date":"2026-07-23",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-07-23",
			"entry_kind":"exchange",
			"postings":[
				`+posting(f.checking.ID, -11000, 2, f.usdID)+`,
				`+posting(trading, 11000, 2, f.usdID)+`,
				`+posting(eurAccount.ID, 10000, 2, eur.ID)+`,
				`+posting(trading, -10000, 2, eur.ID)+`
			]
		}]
	}`, http.StatusCreated)

	download := downloadQIF(t, handler, f.sessionCookie,
		"?account_id="+strconvFormatInt(f.checking.ID), http.StatusOK)

	records := qifRecords(t, download.body)
	require.Len(t, records, 1)
	assert.Equal(t, []string{"-110.00"}, records[0]["T"], "this account's own side is exact")
	assert.Equal(t, []string{"[Euro Account]"}, records[0]["L"])
	require.NotEmpty(t, records[0]["M"])
	assert.Contains(t, records[0]["M"][0], "100.00 EUR", "the other side is stated, not converted")
	assert.Empty(t, records[0]["$"], "an unconvertible side must not become a split amount")
}

// Investment accounts are excluded, and a whole-book export says so by simply
// not writing them — while still writing everything it can.
func TestQIFWholeBookExportSkipsInvestmentAccountsWithoutRefusing(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "Brokerage", "asset", "brokerage", f.usdID, 2)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-23",
		posting(f.checking.ID, -5000, 2, f.usdID),
		posting(f.groceries.ID, 5000, 2, f.usdID),
	), http.StatusCreated)

	download := downloadQIF(t, handler, f.sessionCookie, "", http.StatusOK)
	assert.Equal(t, "application/zip", download.contentType, "more than one account means an archive")

	files := download.archive(t)
	for name := range files {
		assert.NotContains(t, name, "brokerage", "an investment account is not written")
	}
	assert.Contains(t, files, "checking-"+strconvFormatInt(f.checking.ID)+"-mdy.qif")
}

// The preview is where a screen learns what to confirm before it offers a
// download, so it must agree with what the download will do.
func TestExportPreviewReportsQIFSupportAndExclusions(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	holding := createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "Brokerage", "asset", "brokerage", f.usdID, 2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/preview?account_id="+
		strconvFormatInt(f.checking.ID)+"&account_id="+strconvFormatInt(holding.ID)+"&qif_date_layout=dmy", nil)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "response body: %s", res.Body.String())

	var preview exportPreviewResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &preview))

	assert.Equal(t, "dmy", preview.QIF.DateLayout)
	require.Len(t, preview.QIF.Supported, 1)
	assert.Equal(t, f.checking.ID, preview.QIF.Supported[0].AccountID)
	assert.Equal(t, "Bank", preview.QIF.Supported[0].QIFType)
	require.Len(t, preview.QIF.Unsupported, 1)
	assert.Equal(t, holding.ID, preview.QIF.Unsupported[0].AccountID)
	assert.Equal(t, "investment_account", preview.QIF.Unsupported[0].Reason)
}

func TestQIFExportRejectsAnUnknownDateLayout(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	download := downloadQIF(t, handler, f.sessionCookie, "?qif_date_layout=ymd", http.StatusBadRequest)
	assert.Contains(t, string(download.body), "qif_date_layout must be mdy or dmy")
}

// A posting is recorded at any scale its commodity permits, so one entry can
// hold a scale-0 leg beside a scale-2 one. Each split must render at the scale
// its own counterpart was recorded at.
//
// Borrowing the record's scale for the splits instead would look like a tidy
// simplification and would be wrong by a factor of a hundred per decimal place
// — the exact bug this project has shipped three times (T-36/T-45/T-47). The
// splits still have to sum to T, because the entry balances and the splits are
// its negation.
func TestQIFSplitsRenderAtEachCounterpartsOwnScale(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	// -120.00 = 100 (scale 0) + 20.00 (scale 2). All three are legal for USD
	// and the entry balances scale-aware.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-23",
		posting(f.checking.ID, -12000, 2, f.usdID),
		posting(f.groceries.ID, 100, 0, f.usdID),
		posting(f.salary.ID, 2000, 2, f.usdID),
	), http.StatusCreated)

	download := downloadQIF(t, handler, f.sessionCookie,
		"?account_id="+strconvFormatInt(f.checking.ID), http.StatusOK)

	records := qifRecords(t, download.body)
	require.Len(t, records, 1)
	assert.Equal(t, []string{"-120.00"}, records[0]["T"])
	assert.Equal(t, []string{"-100", "-20.00"}, records[0]["$"],
		"each split keeps the scale its counterpart was recorded at")

	// And the numbers still add up, whatever the text looks like.
	total := new(big.Rat)
	for _, split := range records[0]["$"] {
		value, ok := new(big.Rat).SetString(split)
		require.Truef(t, ok, "unparsable split %q", split)
		total.Add(total, value)
	}
	recordAmount, ok := new(big.Rat).SetString(records[0]["T"][0])
	require.True(t, ok)
	assert.Equal(t, 0, total.Cmp(recordAmount), "the splits must sum to the record amount exactly")
}
