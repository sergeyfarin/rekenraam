package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPayeeCreateUpdateArchiveRestoreAndRead(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	payee := createPayeeForSession(t, handler, sessionCookie, csrfToken, `{"name":"Albert Heijn"}`)
	assert.Equal(t, "active", payee.Status)
	assert.Equal(t, "Albert Heijn", payee.Name)

	read := readPayeeForSession(t, handler, sessionCookie, payee.ID, http.StatusOK)
	assert.Equal(t, payee.ID, read.ID)

	updated := mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/payees/"+strconvFormatInt(payee.ID), `{"name":"AH"}`, http.StatusOK)
	assert.Equal(t, "AH", updated.Name)

	archived := mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/payees/"+strconvFormatInt(payee.ID)+"/archive", `{"change_reason":"cleanup"}`, http.StatusOK)
	assert.Equal(t, "archived", archived.Status)

	list := listPayeesForSession(t, handler, sessionCookie, "")
	assert.Empty(t, list.Payees)

	restored := mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/payees/"+strconvFormatInt(payee.ID)+"/restore", `{}`, http.StatusOK)
	assert.Equal(t, "active", restored.Status)
}

func readPayeeForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, payeeID int64, wantStatus int) payeeResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payees/"+strconvFormatInt(payeeID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response payeeResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func listPayeesForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) payeesResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payees"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response payeesResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}
