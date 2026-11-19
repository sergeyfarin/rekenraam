package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
)

// #595. The register always carries a statements array — empty, never null —
// so a client can place statement rows without a nil check.
func TestAccountRegisterAlwaysReturnsStatementsArray(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)

	for _, suffix := range []string{"", "?status=voided"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+strconvFormatInt(checking.ID)+"/register"+suffix, nil)
		req.AddCookie(sessionCookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Code)

		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		assert.JSONEq(t, `[]`, string(body["statements"]), suffix)
		assert.JSONEq(t, `[]`, string(body["entries"]), suffix)
	}
}

// #598 review. The register reads its account's class to sign its balances,
// and its statement rows' commodities (#595). Those reads, like every
// reference read in the request, use the owner's local day: just after
// midnight in Amsterdam a card and a currency added today are invisible on
// SQLite's UTC day, and an unpinned lookup turned the register into a 500.
//
// SQLite's day is the real one, so the frozen instant sits a week after it:
// the card and currency stay future-dated to an unpinned read whatever the
// date the test runs on, even if the run crosses UTC midnight.
func TestAccountRegisterReadsTheCardCreatedJustAfterOwnerMidnight(t *testing.T) {
	t.Parallel()
	instance := fixedReferenceDateInstance(t, realUTCDayInstant(7, "23:01:00"))
	ownerDay := realUTCDay(8)
	handler := instance.handler
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	_, err := instance.services.Settings.SavePreferences(context.Background(), app.SaveUserPreferencesInput{UserID: 1, TimeZone: "Europe/Amsterdam"})
	require.NoError(t, err)
	currencyID := createCurrencyForSession(t, handler, cookie, csrf, `{"code":"EUR","name":"Euro"}`).ID
	card := createAccountForSession(t, handler, cookie, csrf, `{"name":"Midnight card","account_class":"liability","account_kind":"credit_card","default_commodity_id":`+strconvFormatInt(currencyID)+`}`)
	checking := createAccountForSession(t, handler, cookie, csrf, `{"name":"Midnight checking","account_class":"asset","account_kind":"checking","default_commodity_id":`+strconvFormatInt(currencyID)+`}`)
	require.Equal(t, ownerDay, card.EffectiveFrom, "the card exists from the owner's day, a day after UTC's")
	require.Greater(t, card.EffectiveFrom, realUTCDay(1), "the card is future-dated to SQLite's real UTC day")
	createTransactionForSession(t, handler, cookie, csrf, balancedBody(ownerDay,
		posting(card.ID, -50000, 2, currencyID),
		posting(checking.ID, 50000, 2, currencyID),
	), http.StatusCreated)

	var started reconciliationSessionResponse
	res := doInvestmentRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(card.ID)+"/reconciliations/start", map[string]any{
		"commodity_id": currencyID, "source_kind": "statement", "statement_date": ownerDay,
		"statement_balance_value": "-50000", "statement_balance_scale": 2,
	}, http.StatusCreated)
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &started))
	require.Len(t, started.Candidates, 1)
	doInvestmentRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/reconciliations/"+strconvFormatInt(started.ID)+"/preview",
		map[string]any{"posting_version_ids": []int64{started.Candidates[0].PostingID}}, http.StatusOK)
	finishReconciliationForSession(t, handler, cookie, csrf, started.ID, `{"change_reason":"matched statement"}`, http.StatusOK)

	res = doInvestmentRequest(t, handler, cookie, "", http.MethodGet, "/api/v1/accounts/"+strconvFormatInt(card.ID)+"/register", nil, http.StatusOK)
	var register accountRegisterResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &register))
	require.Len(t, register.Entries, 1)
	require.Len(t, register.Statements, 1)
	assert.Equal(t, "Midnight card", *register.Entries[0].Posting.AccountName)
	assert.Equal(t, "-50000", register.Entries[0].RunningBalance.QuantityValue.String())
	assert.Equal(t, "50000", register.Entries[0].RunningBalance.NormalQuantityValue.String(), "a card owing 500 reads 500")
	assert.Equal(t, "50000", register.Statements[0].StatementBalance.NormalQuantityValue.String())
}
