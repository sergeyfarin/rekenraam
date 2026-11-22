package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchCatalogAuthenticationPaginationAndLocalizationKeys(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	unauth := httptest.NewRecorder()
	handler.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/api/v1/search/catalog", nil))
	require.Equal(t, http.StatusUnauthorized, unauth.Code)
	cookie, csrf, currencyID := setupAccountAPITest(t, handler)
	categories := mutateCategoriesSetup(t, handler, cookie, csrf, http.StatusCreated)
	account := createAccountForSession(t, handler, cookie, csrf, `{"name":"Café checking","account_class":"asset","account_kind":"checking","default_commodity_id":`+strconvFormatInt(currencyID)+`,"allows_postings":true}`)
	f := investmentAPITestFixture{sessionCookie: cookie, csrfToken: csrf}
	instrument := createInstrumentForSession(t, handler, f, "SEARCH")
	payee := createPayeeForSession(t, handler, cookie, csrf, `{"name":"Search Payee"}`)
	archived := createPayeeForSession(t, handler, cookie, csrf, `{"name":"Archived Search Payee"}`)
	mutatePayee(t, handler, cookie, csrf, http.MethodPost, "/api/v1/payees/"+strconvFormatInt(archived.ID)+"/archive", `{}`, http.StatusOK)
	seen := map[string]bool{}
	cursor := ""
	for {
		res := doInvestmentRequest(t, handler, cookie, "", http.MethodGet, "/api/v1/search/catalog?limit=7&cursor="+url.QueryEscape(cursor), nil, http.StatusOK)
		var result searchCatalogResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
		require.LessOrEqual(t, len(result.Items), 7)
		for _, item := range result.Items {
			key := item.Kind + ":" + strconvFormatInt(item.ID)
			require.False(t, seen[key], "duplicate cursor item %s", key)
			seen[key] = true
			if item.Kind == "category" {
				require.NotEmpty(t, item.BuiltinKey)
			}
			if item.Kind == "account" {
				assert.Equal(t, account.ID, item.ID)
			}
			if item.Kind == "payee" && item.ID == archived.ID {
				assert.Equal(t, "archived", item.Status)
			}
		}
		if result.NextCursor == "" {
			break
		}
		require.NotEqual(t, cursor, result.NextCursor)
		cursor = result.NextCursor
	}
	assert.Len(t, seen, len(categories.Categories)+4)
	assert.True(t, seen["instrument:"+strconvFormatInt(instrument.CommodityID)])
	assert.True(t, seen["payee:"+strconvFormatInt(payee.ID)])
	assert.True(t, seen["payee:"+strconvFormatInt(archived.ID)])
	doInvestmentRequest(t, handler, cookie, "", http.MethodGet, "/api/v1/search/catalog?cursor=payee:"+strconvFormatInt(payee.ID), nil, http.StatusOK)
	for _, query := range []string{"limit=0", "limit=501", "limit=x", "cursor=account:-1", "cursor=category:01", "cursor=oops:1", "cursor=instrument:1:2"} {
		doInvestmentRequest(t, handler, cookie, "", http.MethodGet, "/api/v1/search/catalog?"+query, nil, http.StatusBadRequest)
	}
}

func TestSearchCatalogEmptyArrayAndSystemExclusionAreReadOnly(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	empty := doInvestmentRequest(t, handler, cookie, "", http.MethodGet, "/api/v1/search/catalog", nil, http.StatusOK)
	assert.JSONEq(t, `{"items":[]}`, empty.Body.String())
	doInvestmentRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/setup/system-accounts", nil, http.StatusCreated)
	var before, after int
	require.NoError(t, database.QueryRow("SELECT count(*) FROM audit_events").Scan(&before))
	res := doInvestmentRequest(t, handler, cookie, "", http.MethodGet, "/api/v1/search/catalog", nil, http.StatusOK)
	var result searchCatalogResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
	require.Len(t, result.Items, 1) // Starter cash is visible; the eight system accounts are not.
	assert.Equal(t, "account", result.Items[0].Kind)
	require.NoError(t, database.QueryRow("SELECT count(*) FROM audit_events").Scan(&after))
	assert.Equal(t, before, after)
}
