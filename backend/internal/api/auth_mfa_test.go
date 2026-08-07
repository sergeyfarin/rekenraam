package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rekenraam/backend/internal/totp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The whole point of the second factor over HTTP: a correct password must not
// set a session cookie, and the protected API must stay closed until the code
// verifies.
func TestLoginWithMFA_HTTPJourney(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	secret := enrollMFAForSession(t, handler, sessionCookie, csrfToken)

	loginRes := postLogin(t, handler, `{"username":"owner","password":"test-password"}`)
	require.Equal(t, http.StatusOK, loginRes.Code)

	var body loginResponse
	require.NoError(t, json.NewDecoder(loginRes.Body).Decode(&body))
	assert.True(t, body.MFARequired)
	assert.Nil(t, body.User, "a half-authenticated attempt must not name the user it is for")

	cookies := loginRes.Result().Cookies()
	assert.Nil(t, findCookie(cookies, sessionCookieName), "no session cookie before the second factor")
	challengeCookie := findCookie(cookies, mfaChallengeCookieName)
	require.NotNil(t, challengeCookie, "the challenge must travel as its own cookie")
	assert.True(t, challengeCookie.HttpOnly)

	// The challenge alone must not authenticate anything.
	probe := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	probe.AddCookie(challengeCookie)
	probeRes := httptest.NewRecorder()
	handler.ServeHTTP(probeRes, probe)
	assert.Equal(t, http.StatusUnauthorized, probeRes.Code)

	code, err := totp.Code(secret, totp.Step(time.Now().UTC()))
	require.NoError(t, err)
	verifyRes := postLoginMFA(t, handler, challengeCookie, `{"code":"`+code+`"}`)
	require.Equal(t, http.StatusOK, verifyRes.Code)

	var verified loginResponse
	require.NoError(t, json.NewDecoder(verifyRes.Body).Decode(&verified))
	require.NotNil(t, verified.User)
	assert.Equal(t, "owner", verified.User.Username)

	verifiedCookies := verifyRes.Result().Cookies()
	newSession := findCookie(verifiedCookies, sessionCookieName)
	require.NotNil(t, newSession, "the session is issued only after the code verified")
	cleared := findCookie(verifiedCookies, mfaChallengeCookieName)
	require.NotNil(t, cleared)
	assert.Equal(t, -1, cleared.MaxAge, "the spent challenge cookie must be cleared")

	session := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	session.AddCookie(newSession)
	sessionRes := httptest.NewRecorder()
	handler.ServeHTTP(sessionRes, session)
	require.Equal(t, http.StatusOK, sessionRes.Code)
	var status authSessionResponse
	require.NoError(t, json.NewDecoder(sessionRes.Body).Decode(&status))
	assert.True(t, status.Authenticated)
}

func TestLoginMFARejectsAWrongCodeAndAMissingChallenge(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	enrollMFAForSession(t, handler, sessionCookie, csrfToken)

	loginRes := postLogin(t, handler, `{"username":"owner","password":"test-password"}`)
	require.Equal(t, http.StatusOK, loginRes.Code)
	challengeCookie := findCookie(loginRes.Result().Cookies(), mfaChallengeCookieName)
	require.NotNil(t, challengeCookie)

	wrong := postLoginMFA(t, handler, challengeCookie, `{"code":"000000"}`)
	require.Equal(t, http.StatusUnauthorized, wrong.Code)
	var body errorResponse
	require.NoError(t, json.NewDecoder(wrong.Body).Decode(&body))
	assert.Equal(t, "UNAUTHENTICATED", body.Error.Code)
	assert.Equal(t, "invalid authentication code", body.Error.Message)

	missing := postLoginMFA(t, handler, nil, `{"code":"000000"}`)
	assert.Equal(t, http.StatusUnauthorized, missing.Code)
}

func TestMFAStatusAndDisableOverHTTP(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)

	before := readMFAStatus(t, handler, sessionCookie)
	assert.Equal(t, "disabled", before.Status)
	assert.True(t, before.Configured)

	enrollMFAForSession(t, handler, sessionCookie, csrfToken)
	active := readMFAStatus(t, handler, sessionCookie)
	assert.Equal(t, "active", active.Status)
	assert.Equal(t, 10, active.RecoveryCodesRemaining)

	// The password is required again, even holding a valid session.
	rejected := postJSONWithSession(t, handler, sessionCookie, csrfToken,
		"/api/v1/auth/mfa/disable", `{"password":"not-the-password"}`)
	assert.Equal(t, http.StatusUnauthorized, rejected.Code)

	accepted := postJSONWithSession(t, handler, sessionCookie, csrfToken,
		"/api/v1/auth/mfa/disable", `{"password":"test-password"}`)
	require.Equal(t, http.StatusNoContent, accepted.Code)

	after := readMFAStatus(t, handler, sessionCookie)
	assert.Equal(t, "disabled", after.Status)

	// And a plain login works again, in one step.
	loginRes := postLogin(t, handler, `{"username":"owner","password":"test-password"}`)
	require.Equal(t, http.StatusOK, loginRes.Code)
	var body loginResponse
	require.NoError(t, json.NewDecoder(loginRes.Body).Decode(&body))
	assert.False(t, body.MFARequired)
	require.NotNil(t, body.User)
}

func TestMFAEndpointsRequireAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	createOwnerSession(t, handler)

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/auth/mfa"},
		{http.MethodPost, "/api/v1/auth/mfa/totp/enroll"},
		{http.MethodPost, "/api/v1/auth/mfa/totp/activate"},
		{http.MethodPost, "/api/v1/auth/mfa/disable"},
		{http.MethodPost, "/api/v1/auth/mfa/recovery-codes"},
	}
	for _, test := range tests {
		req := httptest.NewRequest(test.method, test.path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		setSameOrigin(req)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		assert.Equal(t, http.StatusUnauthorized, res.Code, "%s %s", test.method, test.path)
	}
}

// --- helpers ---

// enrollMFAForSession runs the real enroll + activate pair and returns the
// shared secret so a test can mint codes like an authenticator app.
func enrollMFAForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string) string {
	t.Helper()

	enrollRes := postJSONWithSession(t, handler, sessionCookie, csrfToken,
		"/api/v1/auth/mfa/totp/enroll", `{"password":"test-password"}`)
	require.Equal(t, http.StatusCreated, enrollRes.Code)
	var enrollment mfaEnrollResponse
	require.NoError(t, json.NewDecoder(enrollRes.Body).Decode(&enrollment))
	require.NotEmpty(t, enrollment.Secret)
	require.Contains(t, enrollment.OTPAuthURI, "otpauth://totp/")

	// Activate with the previous step's code so the current one is still
	// unspent — the replay guard would otherwise reject the login codes.
	code, err := totp.Code(enrollment.Secret, totp.Step(time.Now().UTC())-1)
	require.NoError(t, err)
	activateRes := postJSONWithSession(t, handler, sessionCookie, csrfToken,
		"/api/v1/auth/mfa/totp/activate", `{"code":"`+code+`"}`)
	require.Equal(t, http.StatusOK, activateRes.Code)
	var codes mfaRecoveryCodesResponse
	require.NoError(t, json.NewDecoder(activateRes.Body).Decode(&codes))
	require.Len(t, codes.RecoveryCodes, 10)

	return enrollment.Secret
}

func readMFAStatus(t *testing.T, handler http.Handler, sessionCookie *http.Cookie) mfaStatusResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/mfa", nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)

	var status mfaStatusResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&status))
	return status
}

func postJSONWithSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func postLogin(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func postLoginMFA(t *testing.T, handler http.Handler, challengeCookie *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/mfa", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	if challengeCookie != nil {
		req.AddCookie(challengeCookie)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}

	return nil
}
