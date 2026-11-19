package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/testdb"
	"rekenraam/backend/internal/web"
)

func fixedReferenceDateInstance(t *testing.T, instant string) testInstance {
	t.Helper()
	database, url := testdb.Open(t)
	instance := newTestInstanceOnDatabase(t, database, url, HandlerOptions{})
	now, err := time.Parse(time.RFC3339, instant)
	require.NoError(t, err)
	for _, service := range []interface{ SetNowForTest(func() time.Time) }{
		instance.services.Search, instance.services.Settings, instance.services.Account, instance.services.Institution,
		instance.services.Category, instance.services.Payee, instance.services.Currency,
		instance.services.Transaction, instance.services.Investment, instance.services.Pricing,
		instance.services.Export, instance.services.SelfCheck, instance.services.Forecast,
		instance.services.Budget, instance.services.Recurring,
	} {
		service.SetNowForTest(func() time.Time { return now })
	}
	return instance
}

func TestOwnerCurrentReferenceHTTPCreateListAndFutureExclusion(t *testing.T) {
	t.Parallel()
	// Both sides are relative to SQLite's real UTC day (testdb.RealUTCToday):
	// east, the owner's day is a week past it, so today's version is future to
	// an unpinned read; west, the owner's day is the one before it, so the
	// version appended for the owner's tomorrow is SQLite's today.
	today := testdb.RealUTCToday()
	for _, tc := range []struct{ name, instant, zone, today string }{
		{"east just after local midnight", testdb.UTCDayInstant(today, 7, "23:01:00"), "Europe/Amsterdam", testdb.UTCDay(today, 8)},
		{"west while UTC is tomorrow", testdb.UTCDayInstant(today, 0, "03:00:00"), "America/Los_Angeles", testdb.UTCDay(today, -1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			instance := fixedReferenceDateInstance(t, tc.instant)
			handler := instance.handler
			cookie, csrf, currencyID := setupAccountAPITest(t, handler)
			_, err := instance.services.Settings.SavePreferences(context.Background(), app.SaveUserPreferencesInput{UserID: 1, TimeZone: tc.zone})
			require.NoError(t, err)
			account := createAccountForSession(t, handler, cookie, csrf, `{"name":"Owner today HTTP","account_class":"asset","account_kind":"checking","default_commodity_id":`+strconvFormatInt(currencyID)+`}`)
			assert.Equal(t, tc.today, account.OpenedOn)
			assert.Equal(t, tc.today, account.EffectiveFrom)
			get := func() accountResponse {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+strconvFormatInt(account.ID), nil)
				req.AddCookie(cookie)
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				require.Equal(t, http.StatusOK, res.Code, res.Body.String())
				var body accountResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
				return body
			}
			assert.Equal(t, "Owner today HTTP", get().Name)
			listed := listAccountsForSession(t, handler, cookie, "?q=Owner%20today%20HTTP")
			require.Len(t, listed.Accounts, 1)
			assert.Equal(t, account.ID, listed.Accounts[0].ID)

			// Append a genuinely later version through HTTP, then return to the
			// original instant. West of UTC it is already the SQL UTC day, but
			// must remain excluded from the owner's current reads.
			now, err := time.Parse(time.RFC3339, tc.instant)
			require.NoError(t, err)
			instance.services.Account.SetNowForTest(func() time.Time { return now.Add(24 * time.Hour) })
			patchAccount(t, handler, cookie, csrf, account.ID, `{"name":"Future HTTP","account_class":"asset","account_kind":"checking","default_commodity_id":`+strconvFormatInt(currencyID)+`}`, http.StatusOK)
			instance.services.Account.SetNowForTest(func() time.Time { return now })
			assert.Equal(t, "Owner today HTTP", get().Name)
			listed = listAccountsForSession(t, handler, cookie, "?q=Future%20HTTP")
			assert.Empty(t, listed.Accounts)
			res := doInvestmentRequest(t, handler, cookie, "", http.MethodGet, "/api/v1/search/catalog", nil, http.StatusOK)
			var search searchCatalogResponse
			require.NoError(t, json.Unmarshal(res.Body.Bytes(), &search))
			require.Len(t, search.Items, 1)
			assert.Equal(t, "Owner today HTTP", search.Items[0].Name)
		})
	}
}

// TestOwnerReferenceBrowserServer is a test-only server for Playwright's fixed
// instant controls. No environment clock override or testing route is compiled
// into the application binary. Authentication retains its real expiry clock.
func TestOwnerReferenceBrowserServer(t *testing.T) {
	addressFile := os.Getenv("REKENRAAM_REFERENCE_BROWSER_ADDRESS_FILE")
	if addressFile == "" {
		t.Skip("only run by the fixed-instant browser fixture")
	}
	instance := fixedReferenceDateInstance(t, os.Getenv("REKENRAAM_REFERENCE_BROWSER_INSTANT"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(logger, web.Handler(), instance.services, HandlerOptions{})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	require.NoError(t, os.WriteFile(addressFile, []byte("http://"+listener.Addr().String()), 0600))
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	err = server.Serve(listener)
	if err != nil && !strings.Contains(err.Error(), "closed") {
		t.Fatal(err)
	}
}
