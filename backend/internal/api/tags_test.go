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

func TestListTagsRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestCreateTagPersistsAndAudits(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")

	tag := createTagForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Vacation Summer 2026",
		"kind":"project",
		"color":"#3B82F6",
		"icon":"sun",
		"metadata":{"scope":"trip"}
	}`)

	assert.Equal(t, "Vacation Summer 2026", tag.Name)
	assert.Equal(t, "project", tag.Kind)
	assert.Equal(t, "#3B82F6", tag.Color)
	assert.Equal(t, "sun", tag.Icon)
	assert.Equal(t, "active", tag.Status)
	assert.Equal(t, `{"scope":"trip"}`, string(tag.Metadata))

	read := readTagForSession(t, handler, sessionCookie, tag.ID, http.StatusOK)
	assert.Equal(t, tag.ID, read.ID)
	assert.Equal(t, "Vacation Summer 2026", read.Name)

	var (
		operation        string
		originType       string
		actorUserID      int64
		hasAuthSessionID int
		reason           string
	)
	err := database.QueryRowContext(context.Background(), `
		SELECT
			audit_events.operation,
			audit_events.origin_type,
			audit_events.actor_user_id,
			audit_events.auth_session_id IS NOT NULL,
			audit_events.reason
		FROM tags
		JOIN audit_events ON audit_events.id = tags.created_audit_event_id
		WHERE tags.id = ?
	`, tag.ID).Scan(&operation, &originType, &actorUserID, &hasAuthSessionID, &reason)
	require.NoError(t, err)
	assert.Equal(t, "tag.create", operation)
	assert.Equal(t, "browser_api", originType)
	assert.Equal(t, int64(1), actorUserID)
	assert.Equal(t, 1, hasAuthSessionID)
	assert.Equal(t, "created tag", reason)
}

func TestUpdateTagAndFilters(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	_ = createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"Needs receipt","kind":"flag"}`)
	tag := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"Summer trip","kind":"project"}`)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tags/"+strconvFormatInt(tag.ID), strings.NewReader(`{
		"name":"Vacation Summer 2026",
		"kind":"project",
		"color":"#10B981",
		"icon":"plane",
		"metadata":{"renamed":true}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body tagResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, tag.ID, body.ID)
	assert.Equal(t, "Vacation Summer 2026", body.Name)
	assert.Equal(t, "#10B981", body.Color)
	assert.Equal(t, "plane", body.Icon)
	assert.Equal(t, `{"renamed":true}`, string(body.Metadata))

	filtered := listTagsForSession(t, handler, sessionCookie, "?kind=project&q=Vacation")
	require.Len(t, filtered.Tags, 1)
	assert.Equal(t, tag.ID, filtered.Tags[0].ID)
}

func TestTagPatchKeepsOmittedFieldsAndClearsExplicitValues(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	tag := createTagForSession(t, handler, cookie, csrf, `{
		"name":"Vacation","kind":"project","color":"#3B82F6","icon":"sun",
		"metadata":{"reference":9007199254740993}
	}`)
	createTagForSession(t, handler, cookie, csrf, `{"name":"Vacation","kind":"flag"}`)

	patchTag(t, handler, cookie, csrf, tag.ID, `{"kind":"flag"}`, http.StatusConflict)
	patchTag(t, handler, cookie, csrf, tag.ID, `{"name":""}`, http.StatusBadRequest)

	changed := patchTag(t, handler, cookie, csrf, tag.ID, `{"color":"","icon":"","metadata":{}}`, http.StatusOK)
	assert.Equal(t, tag.Name, changed.Name)
	assert.Equal(t, tag.Kind, changed.Kind)
	assert.Empty(t, changed.Color)
	assert.Empty(t, changed.Icon)
	assert.JSONEq(t, `{}`, string(changed.Metadata))
}

func TestArchiveAndRestoreTag(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	tag := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"House renovation","kind":"project"}`)

	archived := mutateTag(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/tags/"+strconvFormatInt(tag.ID)+"/archive", http.StatusOK)
	assert.Equal(t, "archived", archived.Status)

	patchTag(t, handler, sessionCookie, csrfToken, tag.ID, `{
		"name":"Renamed while archived",
		"kind":"project"
	}`, http.StatusConflict)

	defaultList := listTagsForSession(t, handler, sessionCookie, "")
	assert.Empty(t, defaultList.Tags)

	archivedList := listTagsForSession(t, handler, sessionCookie, "?include_archived=true")
	require.Len(t, archivedList.Tags, 1)
	assert.Equal(t, "archived", archivedList.Tags[0].Status)

	restored := mutateTag(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/tags/"+strconvFormatInt(tag.ID)+"/restore", http.StatusOK)
	assert.Equal(t, "active", restored.Status)
}

func TestTagDuplicateAndValidationRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "missing name",
			body:       `{"kind":"project"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid kind",
			body:       `{"name":"Bad","kind":"budget"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid color",
			body:       `{"name":"Bad Color","color":"blue"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid icon",
			body:       `{"name":"Bad Icon","icon":"Plane"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "metadata array",
			body:       `{"name":"Bad Metadata","metadata":[1,2]}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newSetupTestHandler(t)
			sessionCookie, csrfToken := createOwnerSession(t, handler)
			createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")

			req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(csrfTokenHeader, csrfToken)
			setSameOrigin(req)
			req.AddCookie(sessionCookie)
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			require.Equal(t, test.wantStatus, res.Code)
		})
	}
}

func TestTagActiveNameKindMustBeUnique(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	tag := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"Vacation","kind":"project"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", strings.NewReader(`{"name":"vacation","kind":"project"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusConflict, res.Code)

	mutateTag(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/tags/"+strconvFormatInt(tag.ID)+"/archive", http.StatusOK)
	duplicate := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"vacation","kind":"project"}`)
	assert.NotEqual(t, tag.ID, duplicate.ID)

	mutateTag(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/tags/"+strconvFormatInt(tag.ID)+"/restore", http.StatusConflict)
}

func TestTagMutationsRequireAuthentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "create", method: http.MethodPost, path: "/api/v1/tags", body: `{"name":"No Auth"}`},
		{name: "update", method: http.MethodPatch, path: "/api/v1/tags/1", body: `{"name":"No Auth"}`},
		{name: "archive", method: http.MethodPost, path: "/api/v1/tags/1/archive"},
		{name: "restore", method: http.MethodPost, path: "/api/v1/tags/1/restore"},
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

func createTagForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string) tagResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var response tagResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))

	return response
}

func listTagsForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) tagsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response tagsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))

	return response
}

func readTagForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, tagID int64, wantStatus int) tagResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/"+strconvFormatInt(tagID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response tagResponse
	if wantStatus == http.StatusOK {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func patchTag(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, tagID int64, body string, wantStatus int) tagResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tags/"+strconvFormatInt(tagID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response tagResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func mutateTag(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, wantStatus int) tagResponse {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	if wantStatus < 200 || wantStatus >= 300 {
		return tagResponse{}
	}

	var response tagResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))

	return response
}

// TestTagListByIDResolvesAssignmentsPastTheCatalogLimit pins #657: a large
// archived catalog must neither hide active choices nor the archived tags a
// record already carries. Active choices come from the default list, and
// assigned tags are resolved by ID in one bounded read, whatever their status
// or position in the directory.
func TestTagListByIDResolvesAssignmentsPastTheCatalogLimit(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	active := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"Zz active","kind":"custom"}`)
	assigned := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"Zz assigned","kind":"custom"}`)
	mutateTag(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/tags/"+strconvFormatInt(assigned.ID)+"/archive", http.StatusOK)
	_, err := database.Exec(`
		WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 2001)
		INSERT INTO tags (book_id, name, kind, status, created_at, created_by_user_id, updated_at, updated_by_user_id)
		SELECT 1, printf('Archived %04d', i), 'project', 'archived', '2026-01-01T00:00:00Z', u.id, '2026-01-01T00:00:00Z', u.id
		FROM n, (SELECT id FROM users LIMIT 1) u
	`)
	require.NoError(t, err)

	activeOnly := listTagsForSession(t, handler, sessionCookie, "")
	require.Len(t, activeOnly.Tags, 1)
	assert.Equal(t, active.ID, activeOnly.Tags[0].ID)

	combined := listTagsForSession(t, handler, sessionCookie, "?include_archived=true")
	assert.NotContains(t, tagIDs(combined.Tags), assigned.ID, "the combined catalog is capped, which is why the editor must not rely on it")

	byID := listTagsForSession(t, handler, sessionCookie, "?include_archived=true&id="+strconvFormatInt(assigned.ID)+"&id="+strconvFormatInt(active.ID))
	assert.ElementsMatch(t, []int64{assigned.ID, active.ID}, tagIDs(byID.Tags))
	statuses := map[int64]string{}
	for _, tag := range byID.Tags {
		statuses[tag.ID] = tag.Status
	}
	assert.Equal(t, "archived", statuses[assigned.ID])

	withoutArchived := listTagsForSession(t, handler, sessionCookie, "?id="+strconvFormatInt(assigned.ID))
	assert.Empty(t, withoutArchived.Tags, "id narrows the list; it does not override the status filter")

	tooMany := strings.Repeat("&id=1", 201)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags?include_archived=true"+tooMany, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, http.StatusBadRequest, res.Code)
	for _, bad := range []string{"0", "x", "-3"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tags?id="+bad, nil)
		req.AddCookie(sessionCookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		assert.Equal(t, http.StatusBadRequest, res.Code, bad)
	}
}

func tagIDs(tags []tagResponse) []int64 {
	ids := make([]int64, len(tags))
	for i, tag := range tags {
		ids[i] = tag.ID
	}
	return ids
}
