package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
)

func TestCreateBookRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/book", strings.NewReader(`{"code":"personal","name":"Personal"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "UNAUTHENTICATED", body.Error.Code)
}

func TestCreateBookCompletesBookStep(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/book", strings.NewReader(`{"code":"household","name":"Household"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var body createBookResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, int64(1), body.Book.ID)
	assert.Equal(t, int64(1), body.Book.OwnerUserID)
	assert.Equal(t, "household", body.Book.Code)
	assert.Equal(t, "Household", body.Book.Name)
	assert.Nil(t, body.Book.DefaultCurrencyCommodityID)
	assert.Equal(t, app.InstallStateConfigured, body.Setup.InstallState)
	assert.Equal(t, []string{"owner", "book", "currencies", "system_accounts", "categories"}, body.Setup.ImplementedSteps)
	assert.Equal(t, "currencies", body.Setup.CurrentStep)
	assert.Equal(t, setupStepResponse{Key: "book", Status: app.SetupStepStatusCompleted}, body.Setup.Steps[1])

	var completedAt sql.NullString
	var auditEventID sql.NullInt64
	var operation string
	err := database.QueryRowContext(context.Background(), `
		SELECT setup_steps.completed_at, setup_steps.completed_audit_event_id, audit_events.operation
		FROM setup_steps
		JOIN audit_events ON audit_events.id = setup_steps.completed_audit_event_id
		WHERE setup_steps.step_key = 'book'
	`).Scan(&completedAt, &auditEventID, &operation)
	require.NoError(t, err)
	assert.True(t, completedAt.Valid)
	assert.True(t, auditEventID.Valid)
	assert.Equal(t, "book.setup", operation)
}

func TestCreateBookDefaultsCode(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/book", strings.NewReader(`{"name":"Personal"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var body createBookResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "personal", body.Book.Code)
}

func TestCreateBookRejectsSecondBook(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/book", strings.NewReader(`{"code":"other","name":"Other"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusConflict, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "CONFLICT", body.Error.Code)
	assert.Equal(t, "book already exists", body.Error.Message)
}

func TestCreateBookValidatesRequest(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/book", strings.NewReader(`{"code":"Bad Code","name":" "}`))
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
}

func TestCurrentBookRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/books/current", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestCurrentBookReturnsNotFoundBeforeBookSetup(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, _ := createOwnerSession(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/books/current", nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusNotFound, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "NOT_FOUND", body.Error.Code)
}

func TestCurrentBookReturnsCreatedBook(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/books/current", nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body bookResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "personal", body.Code)
	assert.Equal(t, "Personal", body.Name)
}

func createOwnerSession(t *testing.T, handler http.Handler) (*http.Cookie, string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/owner", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)
	cookies := res.Result().Cookies()
	require.Len(t, cookies, 1)

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionReq.AddCookie(cookies[0])
	sessionRes := httptest.NewRecorder()
	handler.ServeHTTP(sessionRes, sessionReq)
	require.Equal(t, http.StatusOK, sessionRes.Code)

	var session authSessionResponse
	require.NoError(t, json.NewDecoder(sessionRes.Body).Decode(&session))
	require.NotEmpty(t, session.CSRFToken)

	return cookies[0], session.CSRFToken
}

func createBookForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, name string) {
	t.Helper()

	body, err := json.Marshal(createBookRequest{Name: name})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/book", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)
}
