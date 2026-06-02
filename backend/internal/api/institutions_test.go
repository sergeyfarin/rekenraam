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

func TestCreateInstitutionPersistsLogoFields(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")

	institution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{
		"name": "Example Bank",
		"kind": "bank",
		"country_code": "nl",
		"website": "https://example.test",
		"logo_url": "https://example.test/logo.svg",
		"logo_small_url": "/institution-assets/example-small.svg",
		"backdrop_url": "https://example.test/backdrop.jpg",
		"comment_markdown": "Primary bank"
	}`)

	assert.Equal(t, "Example Bank", institution.Name)
	assert.Equal(t, "bank", institution.Kind)
	assert.Equal(t, "NL", institution.CountryCode)
	assert.Equal(t, "https://example.test/logo.svg", institution.LogoURL)
	assert.Equal(t, "/institution-assets/example-small.svg", institution.LogoSmallURL)
	assert.Equal(t, "https://example.test/backdrop.jpg", institution.BackdropURL)
	assert.Equal(t, "{}", institution.AddressJSON)
	assert.Equal(t, "{}", institution.MetadataJSON)
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

func TestUpdateInstitutionCreatesAppendOnlyVersion(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	institution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{"name":"Old Bank","kind":"bank"}`)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/institutions/"+strconvFormatInt(institution.ID), strings.NewReader(`{
		"name": "Renamed Bank",
		"kind": "credit_union",
		"country_code": "US"
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
	assert.Equal(t, "Old Bank", versions.Institutions[1].Name)
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
			name: "invalid logo url",
			body: `{"name":"Bad Logo","kind":"bank","logo_url":"ftp://example.test/logo.svg"}`,
		},
		{
			name: "invalid metadata json",
			body: `{"name":"Bad Metadata","kind":"bank","metadata_json":"not json"}`,
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
