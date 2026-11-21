package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The #271 import audit (area 4 of #260) keeps its evidence here:
// docs/reviews/import-audit-2026-10-17.md. Every amount and date below is
// derived by hand from the file bytes and the documented parsing rules
// (import_locale.go's contract, the import plan), never read back from the app.

// uploadImportForAudit posts one file with optional form fields and returns
// the staged preview.
func uploadImportForAudit(t *testing.T, c ledgerAuditClient, filename string, contents []byte, fields map[string]string) startImportResponse {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(contents)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfTokenHeader, c.csrf)
	setSameOrigin(req)
	req.AddCookie(c.cookie)
	res := httptest.NewRecorder()
	c.handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusCreated, res.Code, res.Body.String())
	var started startImportResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&started))
	return started
}

// allImportRowsForAudit follows next_cursor, as the review screen does.
func allImportRowsForAudit(t *testing.T, c ledgerAuditClient, batchID int64) []importStagedRowResponse {
	t.Helper()
	var rows []importStagedRowResponse
	suffix := "?limit=100"
	for {
		page := getImportBatchForSession(t, c.handler, c.cookie, batchID, suffix, http.StatusOK)
		rows = append(rows, page.Rows...)
		if page.NextCursor == nil {
			return rows
		}
		suffix = "?limit=100&cursor=" + *page.NextCursor
	}
}

func rowIDsForAudit(rows []importStagedRowResponse) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func dedupeCountsForAudit(rows []importStagedRowResponse) map[string]int {
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.DedupeStatus]++
	}
	return counts
}

// TestImportAuditEncodingAndDecimalMatrixCommitsHandDerivedAmounts drives each
// encoding × decimal convention × format through upload, review and the real
// ledger commit, then reads the account balance. One account per file keeps
// each expected balance a sum of that file's rows.
func TestImportAuditEncodingAndDecimalMatrixCommitsHandDerivedAmounts(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, eur, _, groceries := bootstrapImportAPITest(t, handler)
	c := ledgerAuditClient{t: t, handler: handler, cookie: cookie, csrf: csrf}

	// An API profile with neither date_layout nor decimal_separator, so the
	// file itself settles both (the UI always writes explicit values).
	var detecting importProfileResponse
	c.json(http.MethodPost, "/api/v1/import-profiles", `{"name":"Detecting","adapter_kind":"csv","config":"{\"delimiter\":\"semicolon\",\"date_column\":\"Datum\",\"payee_column\":\"Naam\",\"amount_column\":\"Bedrag\"}"}`, http.StatusCreated, &detecting)
	var declared importProfileResponse
	c.json(http.MethodPost, "/api/v1/import-profiles", `{"name":"Declared","adapter_kind":"csv","config":"{\"delimiter\":\"semicolon\",\"date_column\":\"Datum\",\"payee_column\":\"Naam\",\"amount_column\":\"Bedrag\",\"date_layout\":\"DMY\",\"decimal_separator\":\",\"}"}`, http.StatusCreated, &declared)
	detectingID := strconv.FormatInt(detecting.ID, 10)
	declaredID := strconv.FormatInt(declared.ID, 10)

	cases := []struct {
		name, file string
		body       []byte
		fields     map[string]string
		encoding   string // reported text encoding / source
		dates      []string
		payees     []string
		balance    string // hand sum of the rows
	}{
		{
			name: "CSV UTF-8 with BOM, decimal comma detected", file: "bom.csv",
			body:   append([]byte("\xef\xbb\xbf"), "Datum;Naam;Bedrag\n01/02/2026;Café;-1.234,56\n13/02/2026;Bäcker;-2,05\n"...),
			fields: map[string]string{"profile_id": detectingID}, encoding: "utf-8/bom",
			// 13/02 proves DMY for the file; "1.234,56" and "2,05" prove the comma.
			dates: []string{"2026-02-01", "2026-02-13"}, payees: []string{"Café", "Bäcker"},
			balance: "-1236.61", // -1234.56 - 2.05
		},
		{
			name: "CSV Windows-1252 detected, round thousands settled by the file", file: "cp1252.csv",
			body:   []byte("Datum;Naam;Bedrag\n01/03/2026;Caf\xe9 \x80;-10,10\n14/03/2026;B\xe4cker;-3.000\n15/03/2026;M\xfcller;-0,95\n"),
			fields: map[string]string{"profile_id": detectingID}, encoding: "windows-1252/detected",
			// "-3.000" alone is undecidable; the file's "10,10"/"0,95" make it 3000 (T-36 class).
			dates: []string{"2026-03-01", "2026-03-14", "2026-03-15"}, payees: []string{"Café €", "Bäcker", "Müller"},
			balance: "-3011.05", // -10.10 - 3000 - 0.95
		},
		{
			name: "CSV Windows-1252 selected, declared DMY and comma", file: "declared.csv",
			body:   []byte("Datum;Naam;Bedrag\n01/04/2026;Caf\xe9 \x80;-10,10\n02/04/2026;B\xe4cker;1.000\n"),
			fields: map[string]string{"profile_id": declaredID, "text_encoding": "windows-1252"}, encoding: "windows-1252/selected",
			// Every date is ambiguous; the declared DMY decides. "1.000" under a declared comma is 1000.
			dates: []string{"2026-04-01", "2026-04-02"}, payees: []string{"Café €", "Bäcker"},
			balance: "989.9", // -10.10 + 1000
		},
		{
			name: "CSV Latin-1 selected, declared DMY and comma", file: "latin1.csv",
			body:   []byte("Datum;Naam;Bedrag\n05/01/2026;Gr\xfcn;-7,5\n"),
			fields: map[string]string{"profile_id": declaredID, "text_encoding": "iso-8859-1"}, encoding: "iso-8859-1/selected",
			dates: []string{"2026-01-05"}, payees: []string{"Grün"},
			balance: "-7.5",
		},
		{
			name: "QIF Latin-1 selected, dotted dates and decimal comma detected", file: "latin1.qif",
			body:   []byte("!Type:Bank\nD15.06.2026\nT-1.234,56\nPM\xfcller\n^\nD01.06.2026\nT-1.000\nPGr\xfcn\n^\n"),
			fields: map[string]string{"text_encoding": "iso-8859-1"}, encoding: "iso-8859-1/selected",
			// 15.06 proves DMY; "1.234,56" proves the comma, so "-1.000" is -1000.
			dates: []string{"2026-06-15", "2026-06-01"}, payees: []string{"Müller", "Grün"},
			balance: "-2234.56", // -1234.56 - 1000
		},
		{
			name: "QIF UTF-8 with BOM, US dates and decimal point detected", file: "bom.qif",
			body:     append([]byte("\xef\xbb\xbf"), "!Type:Bank\nD07/15/2026\nT1,234.56\nPZoë\n^\nD07/01/2026\nT-1,000\nPØre\n^\n"...),
			encoding: "utf-8/bom",
			// 07/15 proves MDY; "1,234.56" proves the point, so "-1,000" is -1000.
			dates: []string{"2026-07-15", "2026-07-01"}, payees: []string{"Zoë", "Øre"},
			balance: "234.56", // 1234.56 - 1000
		},
	}
	for i, tc := range cases {
		account := createLedgerAccount(t, handler, cookie, csrf, fmt.Sprintf("Audit %d", i), "asset", "checking", eur, 2)
		fields := tc.fields
		if fields == nil {
			fields = map[string]string{}
		}
		started := uploadImportForAudit(t, c, tc.file, tc.body, fields)
		assert.Equal(t, tc.encoding, started.Meta.TextEncoding+"/"+started.Meta.EncodingSource, tc.name)
		require.Len(t, started.Rows, len(tc.dates), tc.name)
		for j, row := range started.Rows {
			var normalized struct {
				Date      string `json:"date"`
				PayeeHint string `json:"payee_hint"`
			}
			require.NoError(t, json.Unmarshal([]byte(row.NormalizedJSON), &normalized))
			assert.Equal(t, tc.dates[j], normalized.Date, tc.name)
			assert.Equal(t, tc.payees[j], normalized.PayeeHint, tc.name)
		}
		assert.Empty(t, started.Warnings, tc.name)
		patchImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, resolutionPatchBody(t, account.ID, eur, groceries.ID, rowIDsForAudit(started.Rows)...), http.StatusNoContent)
		commit := commitImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, "", http.StatusOK)
		require.Equal(t, len(tc.dates), commit.CommittedCount, tc.name)
		assert.Equal(t, auditRat(t, tc.balance), c.balance(account.ID, eur, "2026-12-31"), tc.name)
	}
}

// TestImportAuditLargeBatchPartialCommitAndRepeatImportAreIdempotent stages
// 250 rows (past the 200-row page and the old silent clamp), commits all but
// one through the real ledger, then shows that a repeat import of the same
// file, after owner corrections to committed rows, only offers the missing row.
func TestImportAuditLargeBatchPartialCommitAndRepeatImportAreIdempotent(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, eur, checking, groceries := bootstrapImportAPITest(t, handler)
	c := ledgerAuditClient{t: t, handler: handler, cookie: cookie, csrf: csrf}

	// Row i spends i + 0.01: the total of 250 rows is 31375 + 2.50 = 31377.50.
	var file strings.Builder
	for i := 1; i <= 250; i++ {
		file.WriteString(qifRow(fmt.Sprintf("2026-03-%02d", (i%28)+1), fmt.Sprintf("-%d.01", i), fmt.Sprintf("Payee %d", i)))
	}
	statement := []byte(file.String())

	started := uploadImportForAudit(t, c, "large.qif", statement, nil)
	rows := allImportRowsForAudit(t, c, started.Batch.ID)
	require.Len(t, rows, 250, "paging returns every staged row once")
	assert.Equal(t, map[string]int{"new": 250}, dedupeCountsForAudit(rows))

	// Leave row 8 (8.01) unresolved: it fails, the other 249 commit.
	resolved := append(append([]int64{}, rowIDsForAudit(rows[:7])...), rowIDsForAudit(rows[8:])...)
	patchImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, resolutionPatchBody(t, checking.ID, eur, groceries.ID, resolved...), http.StatusNoContent)
	partial := commitImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, "", http.StatusOK)
	assert.Equal(t, "partially_committed", partial.Status)
	assert.Equal(t, 249, partial.CommittedCount)
	assert.Equal(t, 1, partial.FailedCount)
	assert.Equal(t, auditRat(t, "-31369.49"), c.balance(checking.ID, eur, "2026-12-31"), "31377.50 - 8.01")

	committed := allImportRowsForAudit(t, c, started.Batch.ID)
	require.Equal(t, "failed", committed[7].CommitStatus)
	assert.Equal(t, "missing account resolution", committed[7].CommitError)
	for _, row := range committed {
		var raw map[string]string
		require.NoError(t, json.Unmarshal([]byte(row.RawJSON), &raw), "raw source evidence is retained")
		assert.Equal(t, "large.qif", raw["_source_file"])
	}

	// Owner corrections to committed rows: row 1 (-1.01) becomes -1.11, row 2
	// (-2.01) is voided. A repeat import must not undo or duplicate either.
	first := readTransactionForSession(t, handler, cookie, *committed[0].CommittedTransactionID, http.StatusOK)
	var categoryLine, accountLine string
	for _, posting := range first.JournalEntries[0].Postings {
		if posting.AccountID == checking.ID {
			accountLine = posting.LineKey
		} else {
			categoryLine = posting.LineKey
		}
	}
	mutateTransaction(t, handler, cookie, csrf, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(first.ID), `{
		"expected_version_id":`+strconvFormatInt(first.VersionID)+`,"transaction_date":"2026-03-02","description":"",
		"journal_entries":[{"entry_date":"2026-03-02","postings":[
			{"line_key":"`+accountLine+`","account_id":`+strconvFormatInt(checking.ID)+`,"commodity_id":`+strconvFormatInt(eur)+`,"quantity_value":"-111","quantity_scale":2},
			{"line_key":"`+categoryLine+`","account_id":`+strconvFormatInt(groceries.ID)+`,"commodity_id":`+strconvFormatInt(eur)+`,"quantity_value":"111","quantity_scale":2}]}]}`, http.StatusOK)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(*committed[1].CommittedTransactionID)+"/void", `{"change_reason":"duplicate charge"}`, http.StatusOK)
	corrected := auditRat(t, "-31367.58") // -31369.49 - 0.10 + 2.01
	assert.Equal(t, corrected, c.balance(checking.ID, eur, "2026-12-31"))

	// The repeat import offers only the row that never committed.
	again := uploadImportForAudit(t, c, "large.qif", statement, nil)
	againRows := allImportRowsForAudit(t, c, again.Batch.ID)
	assert.Equal(t, map[string]int{"duplicate": 249, "new": 1}, dedupeCountsForAudit(againRows))
	require.Equal(t, "new", againRows[7].DedupeStatus)
	patchImportBatchForSession(t, handler, cookie, csrf, again.Batch.ID, resolutionPatchBody(t, checking.ID, eur, groceries.ID, againRows[7].ID), http.StatusNoContent)
	retry := commitImportBatchForSession(t, handler, cookie, csrf, again.Batch.ID, "", http.StatusOK)
	assert.Equal(t, 1, retry.CommittedCount)
	assert.Equal(t, 249, retry.SkippedCount)
	assert.Equal(t, auditRat(t, "-31375.59"), c.balance(checking.ID, eur, "2026-12-31"), "-31367.58 - 8.01; corrections kept")

	// A third import of the same file commits nothing.
	third := uploadImportForAudit(t, c, "large.qif", statement, nil)
	assert.Equal(t, map[string]int{"duplicate": 250}, dedupeCountsForAudit(allImportRowsForAudit(t, c, third.Batch.ID)))
	none := commitImportBatchForSession(t, handler, cookie, csrf, third.Batch.ID, "", http.StatusOK)
	assert.Equal(t, 0, none.CommittedCount)
	assert.Equal(t, auditRat(t, "-31375.59"), c.balance(checking.ID, eur, "2026-12-31"))
}

// TestImportAuditQIFReexportIdentityIgnoresFileNameAndPosition is the #299
// regression. A QIF row's identity is its content plus its occurrence among
// identical rows in the file, so an overlapping re-export that starts one row
// earlier, or the same rows under another file name, flags the already
// committed rows as duplicates instead of posting them twice.
func TestImportAuditQIFReexportIdentityIgnoresFileNameAndPosition(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, eur, checking, groceries := bootstrapImportAPITest(t, handler)
	c := ledgerAuditClient{t: t, handler: handler, cookie: cookie, csrf: csrf}

	commitAll := func(started startImportResponse) commitImportBatchResponse {
		t.Helper()
		rows := allImportRowsForAudit(t, c, started.Batch.ID)
		patchImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, resolutionPatchBody(t, checking.ID, eur, groceries.ID, rowIDsForAudit(rows)...), http.StatusNoContent)
		return commitImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, "", http.StatusOK)
	}

	coffeeAndLunch := qifRow("06/01/26", "-10.00", "Coffee") + qifRow("06/02/26", "-20.00", "Lunch")
	first := uploadImportForAudit(t, c, "bank.qif", []byte(coffeeAndLunch), nil)
	require.Equal(t, map[string]int{"new": 2}, dedupeCountsForAudit(allImportRowsForAudit(t, c, first.Batch.ID)))
	committed := commitAll(first)
	require.Equal(t, 2, committed.CommittedCount)
	require.Equal(t, "-30", c.balance(checking.ID, eur, "2026-12-31")) // -10.00 - 20.00

	// The same file name, one extra earlier row: Bus is new, Coffee and Lunch
	// moved down one position but are the same transactions.
	overlapping := uploadImportForAudit(t, c, "bank.qif", []byte(qifRow("05/31/26", "-5.00", "Bus")+coffeeAndLunch), nil)
	overlapRows := allImportRowsForAudit(t, c, overlapping.Batch.ID)
	assert.Equal(t, map[string]int{"new": 1, "duplicate": 2}, dedupeCountsForAudit(overlapRows))
	for _, row := range overlapRows {
		assert.Equalf(t, row.RowIndex == 0, row.DedupeStatus == "new", "only Bus (row 0) is new, row %d is %s", row.RowIndex, row.DedupeStatus)
	}

	// The original rows under another file name.
	renamed := uploadImportForAudit(t, c, "bank-2026-06.qif", []byte(coffeeAndLunch), nil)
	assert.Equal(t, map[string]int{"duplicate": 2}, dedupeCountsForAudit(allImportRowsForAudit(t, c, renamed.Batch.ID)))

	overlapCommit := commitAll(overlapping)
	assert.Equal(t, 1, overlapCommit.CommittedCount)
	assert.Equal(t, 2, overlapCommit.SkippedCount)
	assert.Equal(t, "-35", c.balance(checking.ID, eur, "2026-12-31"), "only Bus may post: -30 - 5.00")

	renamedCommit := commitAll(renamed)
	assert.Equal(t, 0, renamedCommit.CommittedCount)
	assert.Equal(t, 2, renamedCommit.SkippedCount)
	assert.Equal(t, "-35", c.balance(checking.ID, eur, "2026-12-31"), "the renamed copy must post nothing")
}

// TestImportAuditRejectedConventionRowsAreFlaggedAndNeverCommitted is the
// #300 regression. A row whose amount or date fails under the file's resolved
// convention (a declared profile value, or whole-file detection) stays flagged
// after a reload, keeps its batch warnings, and is refused at commit instead
// of being re-read under another convention. Including the row in review
// (dedupe_status "new") does not clear the flag.
func TestImportAuditRejectedConventionRowsAreFlaggedAndNeverCommitted(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, eur, _, groceries := bootstrapImportAPITest(t, handler)
	c := ledgerAuditClient{t: t, handler: handler, cookie: cookie, csrf: csrf}

	var declared importProfileResponse
	c.json(http.MethodPost, "/api/v1/import-profiles", `{"name":"Declared","adapter_kind":"csv","config":"{\"delimiter\":\"semicolon\",\"date_column\":\"Datum\",\"payee_column\":\"Naam\",\"amount_column\":\"Bedrag\",\"date_layout\":\"DMY\",\"decimal_separator\":\",\"}"}`, http.StatusCreated, &declared)

	cases := []struct {
		name, file string
		body       []byte
		fields     map[string]string
		rejected   map[int]string // row_index → field whose source text was rejected
		balance    string         // hand sum of the accepted rows only
	}{
		{
			name: "CSV declared DMY and decimal comma", file: "declared.csv",
			body:   []byte("Datum;Naam;Bedrag\n01/02/2026;A;-12,50\n02/02/2026;B;-1.234\n03/02/2026;C;-12.50\n30/02/2026;D;-1,00\n"),
			fields: map[string]string{"profile_id": strconv.FormatInt(declared.ID, 10)},
			// "-12.50" is not a decimal-comma number (per-value detection would
			// read -12.50); "30/02/2026" is no date at all.
			rejected: map[int]string{2: "amount", 3: "date"},
			balance:  "-1246.5", // -12.50 - 1234
		},
		{
			name: "QIF with the comma and DMY settled by the file", file: "detected.qif",
			body: []byte(qifRow("15.06.2026", "-1.234,56", "A") + qifRow("16.06.2026", "-2,05", "B") +
				qifRow("31.02.2026", "-3,00", "C") + qifRow("01.06.2026", "-12.50", "D") +
				"!Type:Bank\nD02.06.2026\nT-10,00\nPE\nS\n$-4.00\nS\n$-6,00\n^\n"),
			// 15.06 and 16.06 make the file DMY; three comma amounts make it a
			// decimal-comma file. Row E's split "-4.00" fails like row D's amount.
			rejected: map[int]string{2: "date", 3: "amount", 4: "split_amount"},
			balance:  "-1236.61", // -1234.56 - 2.05
		},
	}
	for i, tc := range cases {
		account := createLedgerAccount(t, handler, cookie, csrf, fmt.Sprintf("Rejected %d", i), "asset", "checking", eur, 2)
		fields := tc.fields
		if fields == nil {
			fields = map[string]string{}
		}
		started := uploadImportForAudit(t, c, tc.file, tc.body, fields)
		require.NotEmpty(t, started.Warnings, tc.name)

		// A reload sees the same flags and warnings as the upload response.
		reloaded := getImportBatchForSession(t, handler, cookie, started.Batch.ID, "?limit=100", http.StatusOK)
		assert.Equal(t, started.Warnings, reloaded.Warnings, tc.name)
		for _, w := range tc.rejected {
			found := false
			for _, warning := range reloaded.Warnings {
				found = found || strings.Contains(warning.Code, w)
			}
			assert.Truef(t, found, "%s: a stored %s warning", tc.name, w)
		}
		for _, row := range reloaded.Rows {
			var normalized struct {
				ParseErrors []struct {
					Field string `json:"field"`
					Value string `json:"value"`
				} `json:"parse_errors"`
			}
			require.NoError(t, json.Unmarshal([]byte(row.NormalizedJSON), &normalized))
			field, rejected := tc.rejected[row.RowIndex]
			if !rejected {
				assert.Equal(t, "new", row.DedupeStatus, "%s row %d", tc.name, row.RowIndex)
				assert.Empty(t, normalized.ParseErrors, "%s row %d", tc.name, row.RowIndex)
				continue
			}
			assert.Equal(t, "needs_attention", row.DedupeStatus, "%s row %d", tc.name, row.RowIndex)
			require.Len(t, normalized.ParseErrors, 1, "%s row %d", tc.name, row.RowIndex)
			assert.Equal(t, field, normalized.ParseErrors[0].Field, "%s row %d", tc.name, row.RowIndex)
		}

		// The owner includes every row; the rejected ones still never post.
		patchImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, resolutionPatchBody(t, account.ID, eur, groceries.ID, rowIDsForAudit(started.Rows)...), http.StatusNoContent)
		commit := commitImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, "", http.StatusOK)
		assert.Equal(t, len(started.Rows)-len(tc.rejected), commit.CommittedCount, tc.name)
		assert.Equal(t, len(tc.rejected), commit.FailedCount, tc.name)
		assert.Equal(t, auditRat(t, tc.balance), c.balance(account.ID, eur, "2026-12-31"), tc.name)
		for _, row := range allImportRowsForAudit(t, c, started.Batch.ID) {
			if _, rejected := tc.rejected[row.RowIndex]; rejected {
				assert.Equal(t, "failed", row.CommitStatus, "%s row %d", tc.name, row.RowIndex)
				assert.Contains(t, row.CommitError, "convention", "%s row %d", tc.name, row.RowIndex)
			}
		}

		// The batch stays open for its failed rows (#301), but a recommit,
		// even after the owner re-includes them, refuses them again.
		patchImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, resolutionPatchBody(t, account.ID, eur, groceries.ID, rejectedRowIDs(started.Rows, tc.rejected)...), http.StatusNoContent)
		again := commitImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, reviewedImportOverrideBody(t, handler, cookie, started.Batch.ID), http.StatusOK)
		assert.Equal(t, "partially_committed", again.Status, tc.name)
		assert.Equal(t, len(tc.rejected), again.FailedCount, tc.name)
		assert.Equal(t, auditRat(t, tc.balance), c.balance(account.ID, eur, "2026-12-31"), tc.name)
	}
}

func rejectedRowIDs(rows []importStagedRowResponse, rejected map[int]string) []int64 {
	var ids []int64
	for _, row := range rows {
		if _, ok := rejected[row.RowIndex]; ok {
			ids = append(ids, row.ID)
		}
	}
	return ids
}

// TestImportAuditHeldAndFailedRowsResumeInTheirBatch is the #301 regression.
// A row held because it would cross a reconciliation checkpoint stays pending
// and a retry with the override commits it exactly once; a batch that still
// holds or failed rows is never reported committed; and a failed row is
// repaired by PATCH and a recommit of the same batch, in a partially
// committed batch as well as a failed one. Committed rows stay immutable.
func TestImportAuditHeldAndFailedRowsResumeInTheirBatch(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, eur, checking, groceries := bootstrapImportAPITest(t, handler)
	c := ledgerAuditClient{t: t, handler: handler, cookie: cookie, csrf: csrf}

	// A 4.50 spend reconciled on 2026-06-07 closes the period before it.
	reconciled := createTransactionForSession(t, handler, cookie, csrf,
		balancedBody("2026-06-07", posting(checking.ID, -450, 2, eur), posting(groceries.ID, 450, 2, eur)), http.StatusCreated)
	reconcilePostingForSession(t, handler, cookie, csrf, checking.ID, eur, reconciled.JournalEntries[0].Postings[0], "2026-06-07")
	require.Equal(t, auditRat(t, "-4.50"), c.balance(checking.ID, eur, "2026-12-31"))

	upload := func(name, file string) ([]importStagedRowResponse, int64) {
		t.Helper()
		started := uploadImportForAudit(t, c, name, []byte(file), nil)
		return allImportRowsForAudit(t, c, started.Batch.ID), started.Batch.ID
	}
	resolve := func(batchID int64, want int, rowIDs ...int64) {
		t.Helper()
		patchImportBatchForSession(t, handler, cookie, csrf, batchID, resolutionPatchBody(t, checking.ID, eur, groceries.ID, rowIDs...), want)
	}

	// 1. Row 0 (06-01, -10.00) crosses the checkpoint; row 1 (06-20, -20.00)
	// does not. Without the override row 0 is held, not skipped.
	rows, batchID := upload("held.qif", qifRow("2026-06-01", "-10.00", "Held")+qifRow("2026-06-20", "-20.00", "Later"))
	resolve(batchID, http.StatusNoContent, rowIDsForAudit(rows)...)
	first := commitImportBatchForSession(t, handler, cookie, csrf, batchID, "", http.StatusOK)
	assert.Equal(t, "partially_committed", first.Status)
	assert.Equal(t, 1, first.CommittedCount)
	assert.Equal(t, 0, first.SkippedCount)
	assert.Equal(t, []int64{rows[0].ID}, first.ReconciliationHoldRowIDs)
	held := allImportRowsForAudit(t, c, batchID)
	assert.Equal(t, "pending", held[0].CommitStatus)
	assert.Equal(t, "reconciliation override required", held[0].CommitError)
	assert.Equal(t, auditRat(t, "-24.50"), c.balance(checking.ID, eur, "2026-12-31"), "-4.50 - 20.00")

	// A committed row cannot be re-resolved.
	resolve(batchID, http.StatusConflict, rows[1].ID)

	// 2. A batch whose only row is held committed nothing and is not committed.
	// It is staged before the override in step 1 invalidates the checkpoint.
	onlyRows, onlyBatchID := upload("only-held.qif", qifRow("2026-06-02", "-7.00", "Only"))
	resolve(onlyBatchID, http.StatusNoContent, onlyRows[0].ID)
	none := commitImportBatchForSession(t, handler, cookie, csrf, onlyBatchID, "", http.StatusOK)
	assert.NotEqual(t, "committed", none.Status)
	assert.Equal(t, 0, none.CommittedCount)
	assert.Equal(t, []int64{onlyRows[0].ID}, none.ReconciliationHoldRowIDs)
	assert.Equal(t, auditRat(t, "-24.50"), c.balance(checking.ID, eur, "2026-12-31"))

	retry := commitImportBatchForSession(t, handler, cookie, csrf, batchID, reviewedImportOverrideBody(t, handler, cookie, batchID), http.StatusOK)
	assert.Equal(t, "committed", retry.Status)
	assert.Equal(t, 2, retry.CommittedCount)
	assert.Empty(t, retry.ReconciliationHoldRowIDs)
	assert.Equal(t, auditRat(t, "-34.50"), c.balance(checking.ID, eur, "2026-12-31"), "-24.50 - 10.00, once")
	commitImportBatchForSession(t, handler, cookie, csrf, batchID, `{"reconciliation_override":true,"reconciliation_override_checkpoint_ids":[]}`, http.StatusConflict)
	assert.Equal(t, auditRat(t, "-34.50"), c.balance(checking.ID, eur, "2026-12-31"))

	resumed := commitImportBatchForSession(t, handler, cookie, csrf, onlyBatchID, reviewedImportOverrideBody(t, handler, cookie, onlyBatchID), http.StatusOK)
	assert.Equal(t, "committed", resumed.Status)
	assert.Equal(t, 1, resumed.CommittedCount)
	assert.Equal(t, auditRat(t, "-41.50"), c.balance(checking.ID, eur, "2026-12-31"), "-34.50 - 7.00")

	// 3. Row 1 is left unresolved and fails; PATCH and a recommit repair it.
	rows, batchID = upload("failed.qif", qifRow("2026-07-01", "-1.00", "Resolved")+qifRow("2026-07-02", "-2.00", "Unresolved"))
	resolve(batchID, http.StatusNoContent, rows[0].ID)
	partial := commitImportBatchForSession(t, handler, cookie, csrf, batchID, "", http.StatusOK)
	assert.Equal(t, "partially_committed", partial.Status)
	assert.Equal(t, 1, partial.FailedCount)
	resolve(batchID, http.StatusNoContent, rows[1].ID)
	repaired := commitImportBatchForSession(t, handler, cookie, csrf, batchID, "", http.StatusOK)
	assert.Equal(t, "committed", repaired.Status)
	assert.Equal(t, 2, repaired.CommittedCount)
	assert.Equal(t, 0, repaired.FailedCount)
	assert.Equal(t, auditRat(t, "-44.50"), c.balance(checking.ID, eur, "2026-12-31"), "-41.50 - 1.00 - 2.00")

	// 4. Every row failed: the batch is failed, and still repairable.
	rows, batchID = upload("all-failed.qif", qifRow("2026-07-03", "-3.00", "Nothing resolved"))
	failed := commitImportBatchForSession(t, handler, cookie, csrf, batchID, "", http.StatusOK)
	assert.Equal(t, "failed", failed.Status)
	resolve(batchID, http.StatusNoContent, rows[0].ID)
	assert.Equal(t, "pending", allImportRowsForAudit(t, c, batchID)[0].CommitStatus, "a re-resolved row is pending again")
	fixed := commitImportBatchForSession(t, handler, cookie, csrf, batchID, "", http.StatusOK)
	assert.Equal(t, "committed", fixed.Status)
	assert.Equal(t, 1, fixed.CommittedCount)
	assert.Equal(t, auditRat(t, "-47.50"), c.balance(checking.ID, eur, "2026-12-31"), "-44.50 - 3.00")
}

// TestImportAuditIdenticalRowsInTwoAccountsStatementsCommitToEach is the #302
// regression. One CSV profile (and QIF) feeds two accounts' statements that
// share an identical row. A committed row's identity is scoped to the account
// it posted to, so the savings copy commits once the owner picks savings, both
// when the review keeps the staged "duplicate" flag (as the UI does) and when
// it includes the row; a repeat import of either statement commits nothing.
func TestImportAuditIdenticalRowsInTwoAccountsStatementsCommitToEach(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, eur, checking, groceries := bootstrapImportAPITest(t, handler)
	savings := createLedgerAccount(t, handler, cookie, csrf, "Savings", "asset", "savings", eur, 2)
	c := ledgerAuditClient{t: t, handler: handler, cookie: cookie, csrf: csrf}

	var profile importProfileResponse
	c.json(http.MethodPost, "/api/v1/import-profiles", `{"name":"Bank","adapter_kind":"csv","config":"{\"delimiter\":\"semicolon\",\"date_column\":\"Datum\",\"payee_column\":\"Naam\",\"amount_column\":\"Bedrag\",\"external_ref_column\":\"Ref\",\"date_layout\":\"DMY\",\"decimal_separator\":\",\"}"}`, http.StatusCreated, &profile)
	fields := map[string]string{"profile_id": strconv.FormatInt(profile.ID, 10)}

	// keepStatus resolves rows to one account and sends back each row's staged
	// dedupe_status, as the review screen does.
	keepStatus := func(batchID, accountID int64, rows []importStagedRowResponse) {
		t.Helper()
		resolution, err := json.Marshal(map[string]any{"account_id": accountID, "commodity_id": eur, "category_id": groceries.ID})
		require.NoError(t, err)
		patches := make([]rowResolutionPatchRequest, 0, len(rows))
		for _, row := range rows {
			patches = append(patches, rowResolutionPatchRequest{RowID: row.ID, DedupeStatus: row.DedupeStatus, ResolutionJSON: string(resolution)})
		}
		body, err := json.Marshal(patchImportBatchRequest{RowResolutions: patches})
		require.NoError(t, err)
		patchImportBatchForSession(t, handler, cookie, csrf, batchID, string(body), http.StatusNoContent)
	}
	include := func(batchID, accountID int64, rows []importStagedRowResponse) {
		t.Helper()
		patchImportBatchForSession(t, handler, cookie, csrf, batchID, resolutionPatchBody(t, accountID, eur, groceries.ID, rowIDsForAudit(rows)...), http.StatusNoContent)
	}
	commit := func(batchID int64, committed, skipped int) {
		t.Helper()
		result := commitImportBatchForSession(t, handler, cookie, csrf, batchID, "", http.StatusOK)
		assert.Equal(t, committed, result.CommittedCount, "committed")
		assert.Equal(t, skipped, result.SkippedCount, "skipped")
		assert.Equal(t, 0, result.FailedCount, "failed")
	}
	balances := func(wantChecking, wantSavings, msg string) {
		t.Helper()
		assert.Equal(t, auditRat(t, wantChecking), c.balance(checking.ID, eur, "2026-12-31"), "checking: "+msg)
		assert.Equal(t, auditRat(t, wantSavings), c.balance(savings.ID, eur, "2026-12-31"), "savings: "+msg)
	}

	// The fee is content-keyed; "B-7" is a bank reference both statements carry.
	checkingCSV := []byte("Datum;Naam;Bedrag;Ref\n01/09/2026;Bank fee;-2,50;\n02/09/2026;Coffee;-3,00;\n03/09/2026;Interest;0,40;B-7\n")
	savingsCSV := []byte("Datum;Naam;Bedrag;Ref\n01/09/2026;Bank fee;-2,50;\n03/09/2026;Interest;0,40;B-7\n")

	first := uploadImportForAudit(t, c, "checking.csv", checkingCSV, fields)
	firstRows := allImportRowsForAudit(t, c, first.Batch.ID)
	require.Equal(t, map[string]int{"new": 3}, dedupeCountsForAudit(firstRows))
	keepStatus(first.Batch.ID, checking.ID, firstRows)
	commit(first.Batch.ID, 3, 0)
	balances("-5.1", "0", "-2.50 - 3.00 + 0.40")

	// Before an account is picked the savings rows read as already imported
	// (to checking); picking savings clears the flag, and both commit.
	second := uploadImportForAudit(t, c, "savings.csv", savingsCSV, fields)
	secondRows := allImportRowsForAudit(t, c, second.Batch.ID)
	require.Equal(t, map[string]int{"duplicate": 2}, dedupeCountsForAudit(secondRows))
	keepStatus(second.Batch.ID, savings.ID, secondRows)
	assert.Equal(t, map[string]int{"new": 2}, dedupeCountsForAudit(allImportRowsForAudit(t, c, second.Batch.ID)), "not a duplicate in savings")
	commit(second.Batch.ID, 2, 0)
	balances("-5.1", "-2.1", "savings: -2.50 + 0.40")

	// Repeats of either statement commit nothing, whether the review keeps
	// the flag or includes the rows.
	again := uploadImportForAudit(t, c, "checking.csv", checkingCSV, fields)
	againRows := allImportRowsForAudit(t, c, again.Batch.ID)
	require.Equal(t, map[string]int{"duplicate": 3}, dedupeCountsForAudit(againRows))
	keepStatus(again.Batch.ID, checking.ID, againRows)
	assert.Equal(t, map[string]int{"duplicate": 3}, dedupeCountsForAudit(allImportRowsForAudit(t, c, again.Batch.ID)), "still duplicates in checking")
	commit(again.Batch.ID, 0, 3)
	againSavings := uploadImportForAudit(t, c, "savings.csv", savingsCSV, fields)
	againSavingsRows := allImportRowsForAudit(t, c, againSavings.Batch.ID)
	require.Equal(t, map[string]int{"duplicate": 2}, dedupeCountsForAudit(againSavingsRows))
	include(againSavings.Batch.ID, savings.ID, againSavingsRows)
	commit(againSavings.Batch.ID, 0, 2)
	balances("-5.1", "-2.1", "repeats post nothing")

	// QIF has the same account scope.
	fee := []byte(qifRow("09/05/26", "-1.25", "Card fee"))
	for _, account := range []accountResponse{checking, savings} {
		started := uploadImportForAudit(t, c, "fee.qif", fee, nil)
		include(started.Batch.ID, account.ID, allImportRowsForAudit(t, c, started.Batch.ID))
		commit(started.Batch.ID, 1, 0)
	}
	balances("-6.35", "-3.35", "-1.25 in each")
	for _, account := range []accountResponse{checking, savings} {
		repeat := uploadImportForAudit(t, c, "fee.qif", fee, nil)
		repeatRows := allImportRowsForAudit(t, c, repeat.Batch.ID)
		require.Equal(t, map[string]int{"duplicate": 1}, dedupeCountsForAudit(repeatRows))
		include(repeat.Batch.ID, account.ID, repeatRows)
		commit(repeat.Batch.ID, 0, 1)
	}
	balances("-6.35", "-3.35", "QIF repeats post nothing")
}

// TestImportAuditCSVWithoutDateLayoutStoresTheDateLayoutWarning is the #303
// API check. A profile created through the API without date_layout falls back
// to whole-file detection; an all-ambiguous file's warning must be stored
// with the batch so a reload still tells the owner which layout was chosen.
func TestImportAuditCSVWithoutDateLayoutStoresTheDateLayoutWarning(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, _, _, _ := bootstrapImportAPITest(t, handler)
	c := ledgerAuditClient{t: t, handler: handler, cookie: cookie, csrf: csrf}

	var profile importProfileResponse
	c.json(http.MethodPost, "/api/v1/import-profiles", `{"name":"No layout","adapter_kind":"csv","config":"{\"delimiter\":\"comma\",\"date_column\":\"Date\",\"payee_column\":\"Description\",\"amount_column\":\"Amount\",\"decimal_separator\":\".\"}"}`, http.StatusCreated, &profile)

	started := uploadImportForAudit(t, c, "ambiguous.csv",
		[]byte("Date,Description,Amount\n05/01/2026,Rent,-10.00\n06/02/2026,Coffee,-2.00\n"),
		map[string]string{"profile_id": strconv.FormatInt(profile.ID, 10)})
	require.Len(t, started.Warnings, 1)
	assert.Equal(t, "date_layout_ambiguous", started.Warnings[0].Code)
	assert.Equal(t, "MDY", started.Warnings[0].Value)

	reloaded := getImportBatchForSession(t, handler, cookie, started.Batch.ID, "?limit=100", http.StatusOK)
	assert.Equal(t, started.Warnings, reloaded.Warnings)
}

// TestImportBatchOverrideIsBoundToTheReviewedCheckpointUnion is the #305
// regression. The owner reviews the batch preview's union of checkpoints and
// commits with an override naming it. A statement finished after the review
// grows one row's set from [A] to [A, B]: that row is held with nothing
// invalidated and B stays active, while a row whose set ([C]) is a proper
// subset of the reviewed union, and a row reaching no checkpoint, commit.
func TestImportBatchOverrideIsBoundToTheReviewedCheckpointUnion(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, eur, checking, groceries := bootstrapImportAPITest(t, handler)
	c := ledgerAuditClient{t: t, handler: handler, cookie: cookie, csrf: csrf}
	savings := createLedgerAccount(t, handler, cookie, csrf, "Savings", "asset", "savings", eur, 2)
	wallet := createLedgerAccount(t, handler, cookie, csrf, "Wallet", "asset", "cash", eur, 2)

	reconcile := func(accountID int64, date string) int64 {
		t.Helper()
		seed := createTransactionForSession(t, handler, cookie, csrf,
			balancedBody(date, posting(accountID, -100, 2, eur), posting(groceries.ID, 100, 2, eur)), http.StatusCreated)
		reconcilePostingForSession(t, handler, cookie, csrf, accountID, eur, seed.JournalEntries[0].Postings[0], date)
		require.Len(t, c.checkpoints(accountID), 1)
		for id := range c.checkpoints(accountID) {
			return id
		}
		return 0
	}
	checkpointA := reconcile(checking.ID, "2026-06-07")
	checkpointC := reconcile(wallet.ID, "2026-06-07")

	started := uploadImportForAudit(t, c, "bound.qif", []byte(
		qifRow("2026-06-01", "-10.00", "Transfer")+ // checking → savings: [A], then [A, B]
			qifRow("2026-06-02", "-20.00", "Wallet")+ // wallet: [C]
			qifRow("2026-07-20", "-30.00", "Later")), nil) // checking, after A: none
	rows := allImportRowsForAudit(t, c, started.Batch.ID)
	require.Len(t, rows, 3)
	patchImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, resolutionPatchBody(t, checking.ID, eur, savings.ID, rows[0].ID), http.StatusNoContent)
	patchImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, resolutionPatchBody(t, wallet.ID, eur, groceries.ID, rows[1].ID), http.StatusNoContent)
	patchImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, resolutionPatchBody(t, checking.ID, eur, groceries.ID, rows[2].ID), http.StatusNoContent)

	// The preview lists the union, labelled for the owner.
	preview := previewCommitImportBatchForSession(t, handler, cookie, started.Batch.ID, http.StatusOK)
	var union []int64
	for _, checkpoint := range preview.ReconciliationCheckpoints {
		union = append(union, checkpoint.CheckpointID)
		assert.Equal(t, "2026-06-07", checkpoint.StatementDate)
		assert.NotNil(t, checkpoint.AccountName, "the checkpoint carries the account's label parts")
	}
	assert.ElementsMatch(t, []int64{checkpointA, checkpointC}, union)
	reviewed := reviewedImportOverrideBody(t, handler, cookie, started.Batch.ID)

	// An unbound override is refused before any row is touched.
	commitImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, `{"reconciliation_override":true}`, http.StatusBadRequest)

	// Savings finishes a statement after the review: row 0 would now also
	// invalidate B.
	checkpointB := reconcile(savings.ID, "2026-06-07")

	committed := commitImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, reviewed, http.StatusOK)
	assert.Equal(t, "partially_committed", committed.Status)
	assert.Equal(t, 2, committed.CommittedCount)
	assert.Equal(t, []int64{rows[0].ID}, committed.ReconciliationStaleRowIDs)
	assert.Empty(t, committed.ReconciliationHoldRowIDs)

	after := allImportRowsForAudit(t, c, started.Batch.ID)
	assert.Equal(t, "pending", after[0].CommitStatus)
	assert.Equal(t, "reconciliation impact changed after review", after[0].CommitError)
	assert.Equal(t, "active", c.checkpoints(savings.ID)[checkpointB].Status, "B was never reviewed")
	assert.Equal(t, "active", c.checkpoints(checking.ID)[checkpointA].Status, "the held row invalidated nothing")
	assert.Equal(t, "invalidated", c.checkpoints(wallet.ID)[checkpointC].Status, "a subset of the union commits")
	// Gross: each account moved by exactly the rows that committed.
	assert.Equal(t, auditRat(t, "-31.00"), c.balance(checking.ID, eur, "2026-12-31"), "-1.00 - 30.00; row 0 not posted")
	assert.Equal(t, auditRat(t, "-1.00"), c.balance(savings.ID, eur, "2026-12-31"))
	assert.Equal(t, auditRat(t, "-21.00"), c.balance(wallet.ID, eur, "2026-12-31"), "-1.00 - 20.00")

	// A fresh preview shows the current set; reviewing it commits the row once.
	again := previewCommitImportBatchForSession(t, handler, cookie, started.Batch.ID, http.StatusOK)
	var current []int64
	for _, checkpoint := range again.ReconciliationCheckpoints {
		current = append(current, checkpoint.CheckpointID)
	}
	assert.ElementsMatch(t, []int64{checkpointA, checkpointB}, current)
	resumed := commitImportBatchForSession(t, handler, cookie, csrf, started.Batch.ID, reviewedImportOverrideBody(t, handler, cookie, started.Batch.ID), http.StatusOK)
	assert.Equal(t, "committed", resumed.Status)
	assert.Empty(t, resumed.ReconciliationStaleRowIDs)
	assert.Equal(t, auditRat(t, "-41.00"), c.balance(checking.ID, eur, "2026-12-31"))
	assert.Equal(t, auditRat(t, "9.00"), c.balance(savings.ID, eur, "2026-12-31"))
}
