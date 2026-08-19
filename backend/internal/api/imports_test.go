package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Bootstrap ---

// bootstrapImportAPITest wires an owner/book/currency/account/category the
// same way the ledger tests do, so QIF rows can resolve to a real postable
// account and category.
func bootstrapImportAPITest(t *testing.T, handler http.Handler) (sessionCookie *http.Cookie, csrfToken string, commodityID int64, checking accountResponse, groceries categoryResponse) {
	t.Helper()
	sessionCookie, csrfToken, commodityID = setupAccountAPITest(t, handler)
	checking = createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	groceries = createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Groceries","category_type":"expense"}`)
	return sessionCookie, csrfToken, commodityID, checking, groceries
}

// --- Request helpers ---

func startQIFImportForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, filename string, qifContent string, wantStatus int) startImportResponse {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write([]byte(qifContent))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())

	var response startImportResponse
	if wantStatus == http.StatusCreated {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func startMultipartImportForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, includeFileField bool, filename string, contents []byte, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if includeFileField {
		part, err := writer.CreateFormFile("file", filename)
		require.NoError(t, err)
		_, err = part.Write(contents)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())
	return res
}

func getImportBatchForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, batchID int64, suffix string, wantStatus int) getImportBatchResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+strconv.FormatInt(batchID, 10)+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())

	var response getImportBatchResponse
	if wantStatus == http.StatusOK {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func patchImportBatchForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, batchID int64, body string, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/imports/"+strconv.FormatInt(batchID, 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())
	return res
}

func previewCommitImportBatchForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, batchID int64, wantStatus int) previewCommitResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+strconv.FormatInt(batchID, 10)+"/preview-commit", nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())

	var response previewCommitResponse
	if wantStatus == http.StatusOK {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func commitImportBatchForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, batchID int64, body string, wantStatus int) commitImportBatchResponse {
	t.Helper()

	if body == "" {
		body = "{}"
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+strconv.FormatInt(batchID, 10)+"/commit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())

	var response commitImportBatchResponse
	if wantStatus == http.StatusOK {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func discardImportBatchForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, batchID int64, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+strconv.FormatInt(batchID, 10)+"/discard", nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())
	return res
}

func listImportBatchesForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string, wantStatus int) listImportBatchesResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/imports"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())

	var response listImportBatchesResponse
	if wantStatus == http.StatusOK {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func qifRow(date, amount, payee string) string {
	return "!Type:Bank\nD" + date + "\nT" + amount + "\nP" + payee + "\n^\n"
}

// resolutionPatchBody builds a PATCH /imports/{id} request body resolving
// the given rows against one account/commodity/category. The resolution
// field is itself a JSON-encoded string (see rowResolutionPatchRequest), so
// this marshals in two passes rather than string-concatenating JSON into a
// JSON string, which is easy to get subtly wrong.
func resolutionPatchBody(t *testing.T, accountID, commodityID, categoryID int64, rowIDs ...int64) string {
	t.Helper()

	resolution := map[string]any{
		"account_id":   accountID,
		"commodity_id": commodityID,
		"category_id":  categoryID,
	}
	resolutionJSON, err := json.Marshal(resolution)
	require.NoError(t, err)

	patches := make([]rowResolutionPatchRequest, 0, len(rowIDs))
	for _, rowID := range rowIDs {
		patches = append(patches, rowResolutionPatchRequest{
			RowID:          rowID,
			ResolutionJSON: string(resolutionJSON),
		})
	}
	body, err := json.Marshal(patchImportBatchRequest{RowResolutions: patches})
	require.NoError(t, err)
	return string(body)
}

// --- Lifecycle ---

func TestImportQIFLifecycle_StartPatchPreviewCommitCreatesLedgerTransaction(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID, checking, groceries := bootstrapImportAPITest(t, handler)

	qif := qifRow("06/01/26", "-42.50", "Grocery Store") + qifRow("06/05/26", "1500.00", "Salary")
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qif, http.StatusCreated)

	assert.Equal(t, "previewing", started.Batch.Status)
	assert.Equal(t, "qif", started.Batch.SourceKind)
	require.Len(t, started.Rows, 2)
	assert.Equal(t, "new", started.Rows[0].DedupeStatus)
	assert.Equal(t, "pending", started.Rows[0].CommitStatus)

	patchBody := resolutionPatchBody(t, checking.ID, commodityID, groceries.ID, started.Rows[0].ID, started.Rows[1].ID)
	patchImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, patchBody, http.StatusNoContent)

	preview := previewCommitImportBatchForSession(t, handler, sessionCookie, started.Batch.ID, http.StatusOK)
	assert.Equal(t, 2, preview.IncludableCount)
	assert.Equal(t, 0, preview.DuplicateCount)
	assert.Empty(t, preview.ReconciliationIssues)

	commit := commitImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, "", http.StatusOK)
	assert.Equal(t, "committed", commit.Status)
	assert.Equal(t, 2, commit.TotalRows)
	assert.Equal(t, 2, commit.CommittedCount)
	assert.Equal(t, 0, commit.SkippedCount)
	assert.Equal(t, 0, commit.FailedCount)

	got := getImportBatchForSession(t, handler, sessionCookie, started.Batch.ID, "", http.StatusOK)
	assert.Equal(t, "committed", got.Batch.Status)
	require.Len(t, got.Rows, 2)
	for _, row := range got.Rows {
		assert.Equal(t, "committed", row.CommitStatus)
		require.NotNil(t, row.CommittedTransactionID)
	}

	list := listTransactionsForSession(t, handler, sessionCookie, "?q=Grocery")
	require.Len(t, list.Transactions, 1)
	assert.Equal(t, "posted", list.Transactions[0].Status)

	batches := listImportBatchesForSession(t, handler, sessionCookie, "", http.StatusOK)
	require.Len(t, batches.Batches, 1)
	assert.Equal(t, "committed", batches.Batches[0].Status)
}

// --- Malformed / missing input ---

func TestStartImport_UnrecognizedFormatRejectedNoBatchPersisted(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _, _, _ := bootstrapImportAPITest(t, handler)

	res := startMultipartImportForSession(t, handler, sessionCookie, csrfToken, true, "notes.txt", []byte("this is not a QIF file at all"), http.StatusBadRequest)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)

	batches := listImportBatchesForSession(t, handler, sessionCookie, "", http.StatusOK)
	assert.Empty(t, batches.Batches)
}

func TestStartImport_MissingFileFieldRejected(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _, _, _ := bootstrapImportAPITest(t, handler)

	startMultipartImportForSession(t, handler, sessionCookie, csrfToken, false, "", nil, http.StatusBadRequest)
}

// --- Not found ---

func TestImportBatchEndpoints_UnknownBatchReturnsNotFound(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _, _, _ := bootstrapImportAPITest(t, handler)
	const missingBatchID = 999999

	getImportBatchForSession(t, handler, sessionCookie, missingBatchID, "", http.StatusNotFound)
	previewCommitImportBatchForSession(t, handler, sessionCookie, missingBatchID, http.StatusNotFound)
	patchImportBatchForSession(t, handler, sessionCookie, csrfToken, missingBatchID, `{"row_resolutions":[]}`, http.StatusNotFound)
	commitImportBatchForSession(t, handler, sessionCookie, csrfToken, missingBatchID, "", http.StatusNotFound)
	discardImportBatchForSession(t, handler, sessionCookie, csrfToken, missingBatchID, http.StatusNotFound)
}

// --- Authentication / CSRF ---

func TestImportBatchEndpoints_RequireAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"list", http.MethodGet, "/api/v1/imports"},
		{"get", http.MethodGet, "/api/v1/imports/1"},
		{"patch", http.MethodPatch, "/api/v1/imports/1"},
		{"preview-commit", http.MethodPost, "/api/v1/imports/1/preview-commit"},
		{"commit", http.MethodPost, "/api/v1/imports/1/commit"},
		{"discard", http.MethodPost, "/api/v1/imports/1/discard"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusUnauthorized, res.Code, tc.name)
		})
	}
}

func TestImportBatchMutations_RequireCSRFToken(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID, checking, groceries := bootstrapImportAPITest(t, handler)
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qifRow("06/01/26", "-10.00", "Coffee"), http.StatusCreated)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"patch", http.MethodPatch, "/api/v1/imports/" + strconv.FormatInt(started.Batch.ID, 10), resolutionPatchBody(t, checking.ID, commodityID, groceries.ID, started.Rows[0].ID)},
		{"commit", http.MethodPost, "/api/v1/imports/" + strconv.FormatInt(started.Batch.ID, 10) + "/commit", "{}"},
		{"discard", http.MethodPost, "/api/v1/imports/" + strconv.FormatInt(started.Batch.ID, 10) + "/discard", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqBody *strings.Reader
			if tc.body != "" {
				reqBody = strings.NewReader(tc.body)
			} else {
				reqBody = strings.NewReader("")
			}
			req := httptest.NewRequest(tc.method, tc.path, reqBody)
			req.Header.Set("Content-Type", "application/json")
			setSameOrigin(req)
			req.AddCookie(sessionCookie)
			// Deliberately omit the CSRF token header.
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusForbidden, res.Code, tc.name)

			var body errorResponse
			require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
			assert.Equal(t, "CSRF_INVALID", body.Error.Code, tc.name)
		})
	}
}

func TestStartImport_RequiresCSRFToken(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, _, _, _, _ := bootstrapImportAPITest(t, handler)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "bank.qif")
	require.NoError(t, err)
	_, err = part.Write([]byte(qifRow("06/01/26", "-10.00", "Coffee")))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	assert.Equal(t, http.StatusForbidden, res.Code)
}

// --- State-transition guards ---

func TestCommitImportBatch_AlreadyCommittedReturnsConflict(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID, checking, groceries := bootstrapImportAPITest(t, handler)
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qifRow("06/01/26", "-10.00", "Coffee"), http.StatusCreated)
	patchBody := resolutionPatchBody(t, checking.ID, commodityID, groceries.ID, started.Rows[0].ID)
	patchImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, patchBody, http.StatusNoContent)
	commit := commitImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, "", http.StatusOK)
	require.Equal(t, "committed", commit.Status)

	res := &httptest.ResponseRecorder{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+strconv.FormatInt(started.Batch.ID, 10)+"/commit", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusConflict, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "CONFLICT", body.Error.Code)
}

func TestDiscardImportBatch_AfterCommitRejected(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID, checking, groceries := bootstrapImportAPITest(t, handler)
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qifRow("06/01/26", "-10.00", "Coffee"), http.StatusCreated)
	patchBody := resolutionPatchBody(t, checking.ID, commodityID, groceries.ID, started.Rows[0].ID)
	patchImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, patchBody, http.StatusNoContent)
	commitImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, "", http.StatusOK)

	discardImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, http.StatusConflict)
}

func TestDiscardImportBatch_MidReviewLeavesLedgerUntouchedAndBlocksFurtherMutation(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _, _, _ := bootstrapImportAPITest(t, handler)
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qifRow("06/01/26", "-10.00", "Coffee"), http.StatusCreated)

	discardImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, http.StatusNoContent)

	got := getImportBatchForSession(t, handler, sessionCookie, started.Batch.ID, "", http.StatusOK)
	assert.Equal(t, "discarded", got.Batch.Status)
	require.Len(t, got.Rows, 1)
	assert.Equal(t, "pending", got.Rows[0].CommitStatus)

	commitImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, "", http.StatusConflict)
	discardImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, http.StatusConflict)

	list := listTransactionsForSession(t, handler, sessionCookie, "?q=Coffee")
	assert.Empty(t, list.Transactions)
}

func TestPatchImportBatch_OnCommittedBatchRejected(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID, checking, groceries := bootstrapImportAPITest(t, handler)
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qifRow("06/01/26", "-10.00", "Coffee"), http.StatusCreated)
	patchBody := resolutionPatchBody(t, checking.ID, commodityID, groceries.ID, started.Rows[0].ID)
	patchImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, patchBody, http.StatusNoContent)
	commitImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, "", http.StatusOK)

	patchImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, patchBody, http.StatusConflict)
}

// --- Commit-time resolution failures ---

func TestCommitImportBatch_UnresolvedRowFailsWithMissingAccountResolution(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _, _, _ := bootstrapImportAPITest(t, handler)
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qifRow("06/01/26", "-10.00", "Coffee"), http.StatusCreated)

	commit := commitImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, "", http.StatusOK)
	assert.Equal(t, "failed", commit.Status)
	assert.Equal(t, 0, commit.CommittedCount)
	assert.Equal(t, 1, commit.FailedCount)

	got := getImportBatchForSession(t, handler, sessionCookie, started.Batch.ID, "", http.StatusOK)
	require.Len(t, got.Rows, 1)
	assert.Equal(t, "failed", got.Rows[0].CommitStatus)
	assert.Contains(t, got.Rows[0].CommitError, "missing account resolution")
}

func TestPatchImportBatch_InvalidResolutionJSONRejectedAsValidationError(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _, _, _ := bootstrapImportAPITest(t, handler)
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qifRow("06/01/26", "-10.00", "Coffee"), http.StatusCreated)

	patchBody := `{"row_resolutions":[{"row_id":` + strconv.FormatInt(started.Rows[0].ID, 10) + `,"resolution":"{not-valid-json"}]}`
	res := patchImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, patchBody, http.StatusBadRequest)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)

	// The rejected patch must not have partially written the row.
	got := getImportBatchForSession(t, handler, sessionCookie, started.Batch.ID, "", http.StatusOK)
	require.Len(t, got.Rows, 1)
	assert.Equal(t, "{}", got.Rows[0].ResolutionJSON)
}

func TestPatchImportBatch_InvalidDedupeStatusRejectedAsValidationError(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _, _, _ := bootstrapImportAPITest(t, handler)
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qifRow("06/01/26", "-10.00", "Coffee"), http.StatusCreated)

	patchBody := `{"row_resolutions":[{"row_id":` + strconv.FormatInt(started.Rows[0].ID, 10) + `,"dedupe_status":"bogus"}]}`
	res := patchImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, patchBody, http.StatusBadRequest)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
}

func TestCommitImportBatch_UnknownAccountResolutionFailsRow(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID, _, groceries := bootstrapImportAPITest(t, handler)
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qifRow("06/01/26", "-10.00", "Coffee"), http.StatusCreated)

	const unknownAccountID = 999999
	patchBody := resolutionPatchBody(t, unknownAccountID, commodityID, groceries.ID, started.Rows[0].ID)
	patchImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, patchBody, http.StatusNoContent)

	commit := commitImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, "", http.StatusOK)
	assert.Equal(t, "failed", commit.Status)
	assert.Equal(t, 1, commit.FailedCount)

	got := getImportBatchForSession(t, handler, sessionCookie, started.Batch.ID, "", http.StatusOK)
	require.Len(t, got.Rows, 1)
	assert.Equal(t, "failed", got.Rows[0].CommitStatus)
	assert.Contains(t, got.Rows[0].CommitError, "posting account is invalid")
}

// --- Dedupe across batches ---

func TestStartImport_ReimportingSameFileFlagsDuplicatesAndSkipsOnCommit(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID, checking, groceries := bootstrapImportAPITest(t, handler)
	qif := qifRow("06/01/26", "-10.00", "Coffee")

	first := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qif, http.StatusCreated)
	patchBody := resolutionPatchBody(t, checking.ID, commodityID, groceries.ID, first.Rows[0].ID)
	patchImportBatchForSession(t, handler, sessionCookie, csrfToken, first.Batch.ID, patchBody, http.StatusNoContent)
	firstCommit := commitImportBatchForSession(t, handler, sessionCookie, csrfToken, first.Batch.ID, "", http.StatusOK)
	require.Equal(t, 1, firstCommit.CommittedCount)

	second := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qif, http.StatusCreated)
	require.Len(t, second.Rows, 1)
	assert.Equal(t, "duplicate", second.Rows[0].DedupeStatus)

	secondCommit := commitImportBatchForSession(t, handler, sessionCookie, csrfToken, second.Batch.ID, "", http.StatusOK)
	assert.Equal(t, "committed", secondCommit.Status)
	assert.Equal(t, 0, secondCommit.CommittedCount)
	assert.Equal(t, 1, secondCommit.SkippedCount)

	list := listTransactionsForSession(t, handler, sessionCookie, "?q=Coffee")
	assert.Len(t, list.Transactions, 1, "duplicate import must not create a second ledger transaction")
}

// --- Pagination ---

func TestGetImportBatchRows_PaginatesWithCursor(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _, _, _ := bootstrapImportAPITest(t, handler)
	qif := qifRow("06/01/26", "-1.00", "Row One") + qifRow("06/02/26", "-2.00", "Row Two") + qifRow("06/03/26", "-3.00", "Row Three")
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qif, http.StatusCreated)

	page1 := getImportBatchForSession(t, handler, sessionCookie, started.Batch.ID, "?limit=2", http.StatusOK)
	require.Len(t, page1.Rows, 2)
	require.NotNil(t, page1.NextCursor)

	page2 := getImportBatchForSession(t, handler, sessionCookie, started.Batch.ID, "?limit=2&cursor="+*page1.NextCursor, http.StatusOK)
	require.Len(t, page2.Rows, 1)
	assert.Nil(t, page2.NextCursor)

	seen := map[int64]bool{}
	for _, row := range append(page1.Rows, page2.Rows...) {
		seen[row.ID] = true
	}
	assert.Len(t, seen, 3, "pagination must not repeat or drop rows")
}

// TestCommitImportBatch_LinksRowsToExistingPayeesByName drives a real commit
// rather than asserting on a builder's output: the import path shares
// cleanTransactionSpec, so the T-44 name match applies to it, and this proves
// it survives all the way through CommitImportBatch.
//
// A bulk import cannot ask the user to confirm each unknown name, so an
// unrecognized payee stays unlinked and is left for review — the report shows
// it as unattributed rather than the importer inventing records in bulk.
func TestCommitImportBatch_LinksRowsToExistingPayeesByName(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID, checking, groceries := bootstrapImportAPITest(t, handler)

	// Deliberately different capitalisation from the imported rows.
	groceryStore := createPayeeForSession(t, handler, sessionCookie, csrfToken, `{"name":"Grocery Store"}`)

	qif := qifRow("06/01/26", "-42.50", "GROCERY STORE") + qifRow("06/05/26", "1500.00", "Employer Nobody Recorded")
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif", qif, http.StatusCreated)
	require.Len(t, started.Rows, 2)

	patchBody := resolutionPatchBody(t, checking.ID, commodityID, groceries.ID, started.Rows[0].ID, started.Rows[1].ID)
	patchImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, patchBody, http.StatusNoContent)

	commit := commitImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID, "", http.StatusOK)
	require.Equal(t, 2, commit.CommittedCount)

	linked := listTransactionsForSession(t, handler, sessionCookie, "?payee_id="+strconvFormatInt(groceryStore.ID))
	require.Len(t, linked.Transactions, 1)
	require.NotNil(t, linked.Transactions[0].PayeeID)
	assert.Equal(t, groceryStore.ID, *linked.Transactions[0].PayeeID)
	assert.Equal(t, "Grocery Store", linked.Transactions[0].PayeeName)

	unknown := listTransactionsForSession(t, handler, sessionCookie, "?q=Employer")
	require.Len(t, unknown.Transactions, 1)
	assert.Nil(t, unknown.Transactions[0].PayeeID)

	// No payee record was invented for the unrecognized name.
	payees := listPayeesForSession(t, handler, sessionCookie, "")
	for _, payee := range payees.Payees {
		assert.NotEqual(t, "Employer Nobody Recorded", payee.Name)
	}
}
