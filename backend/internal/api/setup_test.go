package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
)

func TestSetupStatusReturnsInitialSteps(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body setupStatusResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.False(t, body.Completed)
	assert.Equal(t, "owner", body.CurrentStep)
	require.Len(t, body.Steps, 5)
	assert.Equal(t, setupStepResponse{Key: "owner", Status: app.SetupStepStatusPending}, body.Steps[0])
	assert.Equal(t, setupStepResponse{Key: "book", Status: app.SetupStepStatusPending}, body.Steps[1])
	assert.Equal(t, setupStepResponse{Key: "currencies", Status: app.SetupStepStatusPending}, body.Steps[2])
	assert.Equal(t, setupStepResponse{Key: "system_accounts", Status: app.SetupStepStatusPending}, body.Steps[3])
	assert.Equal(t, setupStepResponse{Key: "categories", Status: app.SetupStepStatusPending}, body.Steps[4])
}

func TestCreateOwnerCompletesOwnerStepAndSetsSessionCookie(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/owner", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var body createOwnerResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, int64(1), body.Owner.ID)
	assert.Equal(t, "owner", body.Owner.Username)
	assert.False(t, body.Setup.Completed)
	assert.Equal(t, "book", body.Setup.CurrentStep)
	assert.Equal(t, setupStepResponse{Key: "owner", Status: app.SetupStepStatusCompleted}, body.Setup.Steps[0])

	response := res.Result()
	cookies := response.Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, sessionCookieName, cookies[0].Name)
	assert.NotEmpty(t, cookies[0].Value)
	assert.Equal(t, "/", cookies[0].Path)
	assert.True(t, cookies[0].HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
	assert.False(t, cookies[0].Secure)

	var passwordHash string
	err := database.QueryRowContext(context.Background(), `SELECT password_hash FROM users WHERE username = ?`, "owner").Scan(&passwordHash)
	require.NoError(t, err)
	assert.NotEqual(t, "test-password", passwordHash)
	assert.True(t, strings.HasPrefix(passwordHash, "$argon2id$"))

	var sessionCount int
	err = database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions`).Scan(&sessionCount)
	require.NoError(t, err)
	assert.Equal(t, 1, sessionCount)
}

func TestCreateOwnerRejectsSecondOwner(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	firstReq := httptest.NewRequest(http.MethodPost, "/api/v1/setup/owner", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRes := httptest.NewRecorder()
	handler.ServeHTTP(firstRes, firstReq)
	require.Equal(t, http.StatusCreated, firstRes.Code)

	secondReq := httptest.NewRequest(http.MethodPost, "/api/v1/setup/owner", strings.NewReader(`{"username":"other","password":"test-password"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRes := httptest.NewRecorder()

	handler.ServeHTTP(secondRes, secondReq)

	require.Equal(t, http.StatusConflict, secondRes.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(secondRes.Body).Decode(&body))
	assert.Equal(t, "SETUP_ALREADY_COMPLETE", body.Error.Code)
	assert.Equal(t, "setup owner is already complete", body.Error.Message)
}

func TestCreateOwnerValidatesRequest(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/owner", strings.NewReader(`{"username":" ","password":""}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
	assert.Equal(t, "username is required", body.Error.Message)
}

func newSetupTestHandler(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()

	database, err := db.Open(context.Background(), "file:"+filepath.Join(t.TempDir(), "rekenraam.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, database.Close())
	})

	require.NoError(t, db.Migrate(context.Background(), database))

	setupRepository := db.NewSetupRepository(database)
	setupService := app.NewSetupService(setupRepository)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return NewHandler(logger, http.NotFoundHandler(), setupService), database
}
