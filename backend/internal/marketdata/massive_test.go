package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMassiveSplitContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer synthetic-key", r.Header.Get("Authorization"))
		assert.NotContains(t, r.URL.String(), "synthetic-key")
		switch r.URL.Path {
		case "/stocks/v1/splits":
			assert.Equal(t, "TEST", r.URL.Query().Get("ticker"))
			assert.Equal(t, "2026-05-01", r.URL.Query().Get("execution_date.gte"))
			assert.Equal(t, "2026-06-01", r.URL.Query().Get("execution_date.lte"))
			fmt.Fprint(w, `{"status":"OK","results":[
				{"id":"forward","ticker":"TEST","execution_date":"2026-05-02","adjustment_type":"forward_split","split_from":2.0,"split_to":3e0},
				{"id":"reverse","ticker":"TEST","execution_date":"2026-05-03","adjustment_type":"reverse_split","split_from":20.0,"split_to":1.0},
				{"id":"stock-div","ticker":"TEST","execution_date":"2026-05-04","adjustment_type":"stock_dividend","split_from":1,"split_to":1.08},
				{"id":"wrong","ticker":"TEST","execution_date":"2026-05-05","adjustment_type":"reverse_split","split_from":1,"split_to":2},
				{"id":"foreign","ticker":"TEST","execution_date":"2026-05-06","adjustment_type":"forward_split","split_from":1,"split_to":4}
			]}`)
		case "/v3/reference/tickers/TEST":
			figi := "test-figi"
			if r.URL.Query().Get("date") == "2026-05-06" {
				figi = "reused-ticker-figi"
			}
			fmt.Fprintf(w, `{"status":"OK","results":{"ticker":"TEST","primary_exchange":"XNAS","market":"stocks","locale":"us","share_class_figi":%q}}`, figi)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	provider := NewMassiveProvider(server.Client(), server.URL, "synthetic-key")
	rows, err := provider.FetchSplits(context.Background(), "TEST", "XNAS", "2026-05-01", "2026-06-01")
	require.NoError(t, err)
	require.Len(t, rows, 5)
	assert.Equal(t, int64(3), rows[0].Numerator)
	assert.Equal(t, int64(2), rows[0].Denominator)
	assert.Equal(t, "test-figi", rows[0].ShareFIGI)
	assert.Equal(t, int64(1), rows[1].Numerator)
	assert.Equal(t, int64(20), rows[1].Denominator)
	assert.Equal(t, "stock_dividend", rows[2].Kind)
	assert.Zero(t, rows[2].Numerator)
	assert.Zero(t, rows[3].Numerator)
	assert.Empty(t, rows[4].ShareFIGI)
}

func TestMassiveSplitErrorsNeverReturnSecretsOrFollowProviderURLs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"forbidden", "synthetic-key", 403},
		{"rate-limited", "synthetic-key", 429},
		{"redirect", "", 302},
		{"unexpected page", `{"status":"OK","next_url":"https://invalid.example/synthetic-key"}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/stocks/v1/splits", r.URL.Path)
				w.Header().Set("Location", "/unexpected")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(server.Close)
			_, err := NewMassiveProvider(server.Client(), server.URL, "synthetic-key").FetchSplits(ctx, "TEST", "XNAS", "2026-05-01", "2026-06-01")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "synthetic-key")
			if tc.status == 429 {
				var providerError *QuoteError
				require.ErrorAs(t, err, &providerError)
				assert.False(t, providerError.Terminal)
				assert.Equal(t, time.Minute, providerError.RetryAfter)
			}
		})
	}
	_, err := NewMassiveProvider(nil, "", "").FetchSplits(ctx, "TEST", "XNAS", "2026-05-01", "2026-06-01")
	assert.ErrorIs(t, err, ErrMissingSecret)
}
