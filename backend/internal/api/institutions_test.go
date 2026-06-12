package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListInstitutionsRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/institutions", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestCreateInstitutionPersistsMetadata(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")

	institution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{
		"name": "Example Bank",
		"kind": "bank",
		"country_code": "nl",
		"website": "https://example.test",
		"metadata": {
			"images": {
				"logo_url": "https://example.test/logo.svg",
				"logo_small_url": "/institution-assets/example-small.svg",
				"backdrop_url": "https://example.test/backdrop.jpg"
			}
		},
		"comment_markdown": "Primary bank"
	}`)

	assert.Equal(t, "Example Bank", institution.Name)
	assert.Equal(t, "bank", institution.Kind)
	assert.Equal(t, "NL", institution.CountryCode)
	assert.Equal(t, "{}", string(institution.Address))
	assert.Equal(t, `{"images":{"logo_url":"https://example.test/logo.svg","logo_small_url":"/institution-assets/example-small.svg","backdrop_url":"https://example.test/backdrop.jpg"}}`, string(institution.Metadata))
	assert.Equal(t, "Primary bank", institution.CommentMarkdown)

	var versionCount int
	err := database.QueryRowContext(context.Background(), `
		SELECT COUNT(1)
		FROM institution_versions
		WHERE institution_id = ?
	`, institution.ID).Scan(&versionCount)
	require.NoError(t, err)
	assert.Equal(t, 1, versionCount)
}

func TestReadInstitutionReturnsSingleInstitution(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	institution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{
		"name": "Read Me Bank",
		"kind": "bank",
		"address": {"street": "123 Main"},
		"metadata": {"source": "manual"}
	}`)

	read := readInstitutionForSession(t, handler, sessionCookie, institution.ID)

	assert.Equal(t, institution.ID, read.ID)
	assert.Equal(t, "Read Me Bank", read.Name)
	assert.Equal(t, `{"street":"123 Main"}`, string(read.Address))
	assert.Equal(t, `{"source":"manual"}`, string(read.Metadata))
}

func TestUpdateInstitutionCreatesAppendOnlyVersion(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	institution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{"name":"Old Bank","kind":"bank"}`)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/institutions/"+strconvFormatInt(institution.ID), strings.NewReader(`{
		"name": "Renamed Bank",
		"kind": "credit_union",
		"country_code": "US",
		"effective_from": "2021-04-05",
		"change_reason": "Imported corrected institution details"
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body institutionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, institution.ID, body.ID)
	assert.Equal(t, "Renamed Bank", body.Name)
	assert.Equal(t, "credit_union", body.Kind)
	assert.Equal(t, "US", body.CountryCode)
	assert.Equal(t, "2021-04-05", body.EffectiveFrom)
	assert.Equal(t, "Imported corrected institution details", body.ChangeReason)

	var versionCount int
	err := database.QueryRowContext(context.Background(), `
		SELECT COUNT(1)
		FROM institution_versions
		WHERE institution_id = ?
	`, institution.ID).Scan(&versionCount)
	require.NoError(t, err)
	assert.Equal(t, 2, versionCount)

	versions := listInstitutionsForSession(t, handler, sessionCookie, "/"+strconvFormatInt(institution.ID)+"/versions")
	require.Len(t, versions.Institutions, 2)
	assert.Equal(t, "Renamed Bank", versions.Institutions[0].Name)
	assert.Equal(t, "Imported corrected institution details", versions.Institutions[0].ChangeReason)
	assert.Equal(t, "Old Bank", versions.Institutions[1].Name)
}

func TestUpdateInstitutionCanClearOptionalFields(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	institution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Clear Fields Bank",
		"kind":"bank",
		"country_code":"US",
		"website":"https://clear.example",
		"address":{"city":"Portland"},
		"comment_markdown":"Clear me",
		"metadata":{"tag":"old"}
	}`)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/institutions/"+strconvFormatInt(institution.ID), strings.NewReader(`{
		"name": "Clear Fields Bank",
		"kind": "bank"
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body institutionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "", body.CountryCode)
	assert.Equal(t, "", body.Website)
	assert.Equal(t, "{}", string(body.Address))
	assert.Equal(t, "", body.CommentMarkdown)
	assert.Equal(t, "{}", string(body.Metadata))
}

func TestArchiveInstitutionHidesFromDefaultListAndRestoreShowsIt(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	institution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{"name":"Archive Me","kind":"other"}`)

	mutateInstitution(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/institutions/"+strconvFormatInt(institution.ID)+"/archive", http.StatusOK)

	defaultList := listInstitutionsForSession(t, handler, sessionCookie, "")
	assert.Empty(t, defaultList.Institutions)

	archivedList := listInstitutionsForSession(t, handler, sessionCookie, "?include_archived=true")
	require.Len(t, archivedList.Institutions, 1)
	assert.Equal(t, "archived", archivedList.Institutions[0].Status)

	mutateInstitution(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/institutions/"+strconvFormatInt(institution.ID)+"/restore", http.StatusOK)

	restoredList := listInstitutionsForSession(t, handler, sessionCookie, "")
	require.Len(t, restoredList.Institutions, 1)
	assert.Equal(t, "active", restoredList.Institutions[0].Status)
}

func TestListInstitutionsFiltersByStatus(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	active := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{"name":"Active Bank","kind":"bank"}`)
	archived := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{"name":"Archived Bank","kind":"bank"}`)
	mutateInstitution(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/institutions/"+strconvFormatInt(archived.ID)+"/archive", http.StatusOK)

	activeList := listInstitutionsForSession(t, handler, sessionCookie, "?status=active")
	require.Len(t, activeList.Institutions, 1)
	assert.Equal(t, active.ID, activeList.Institutions[0].ID)
	assert.Equal(t, "active", activeList.Institutions[0].Status)

	archivedList := listInstitutionsForSession(t, handler, sessionCookie, "?status=archived")
	require.Len(t, archivedList.Institutions, 1)
	assert.Equal(t, archived.ID, archivedList.Institutions[0].ID)
	assert.Equal(t, "archived", archivedList.Institutions[0].Status)
}

func TestDeleteUnusedInstitutionRemovesErroneousInstitution(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	institution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{"name":"Typo Bank","kind":"bank"}`)

	mutateInstitutionNoResponse(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/institutions/"+strconvFormatInt(institution.ID), http.StatusNoContent)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/institutions/"+strconvFormatInt(institution.ID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestDeleteInstitutionRejectsAccountReferences(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	institution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{"name":"Referenced Bank","kind":"bank"}`)
	createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Checking",
		"account_class":"asset",
		"account_kind":"checking",
		"institution_id":`+strconvFormatInt(institution.ID)+`,
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`)

	mutateInstitutionNoResponse(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/institutions/"+strconvFormatInt(institution.ID), http.StatusConflict)
}

func TestInstitutionMutationsRequireAuthentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "create",
			method: http.MethodPost,
			path:   "/api/v1/institutions",
			body:   `{"name":"No Auth","kind":"bank"}`,
		},
		{
			name:   "update",
			method: http.MethodPatch,
			path:   "/api/v1/institutions/1",
			body:   `{"name":"No Auth","kind":"bank"}`,
		},
		{
			name:   "archive",
			method: http.MethodPost,
			path:   "/api/v1/institutions/1/archive",
		},
		{
			name:   "restore",
			method: http.MethodPost,
			path:   "/api/v1/institutions/1/restore",
		},
		{
			name:   "delete",
			method: http.MethodDelete,
			path:   "/api/v1/institutions/1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newSetupTestHandler(t)
			req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			require.Equal(t, http.StatusUnauthorized, res.Code)
		})
	}
}

func TestCreateInstitutionValidatesInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing name",
			body: `{"kind":"bank"}`,
		},
		{
			name: "invalid kind",
			body: `{"name":"Bad Kind","kind":"wallet"}`,
		},
		{
			name: "invalid metadata object",
			body: `{"name":"Bad Metadata","kind":"bank","metadata":"not object"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newSetupTestHandler(t)
			sessionCookie, csrfToken := createOwnerSession(t, handler)
			createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")

			req := httptest.NewRequest(http.MethodPost, "/api/v1/institutions", strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(csrfTokenHeader, csrfToken)
			setSameOrigin(req)
			req.AddCookie(sessionCookie)
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			require.Equal(t, http.StatusBadRequest, res.Code)

			var body errorResponse
			require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
			assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
		})
	}
}

func createInstitutionForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string) institutionResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/institutions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var response institutionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))

	return response
}

func readInstitutionForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, institutionID int64) institutionResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/institutions/"+strconvFormatInt(institutionID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response institutionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))

	return response
}

func listInstitutionsForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) institutionsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/institutions"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response institutionsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))

	return response
}

func mutateInstitution(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, wantStatus int) institutionResponse {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response institutionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))

	return response
}

func mutateInstitutionNoResponse(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, wantStatus int) {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equalf(t, wantStatus, res.Code, "response body: %s", res.Body.String())
}
