package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #293. An account version dated before the account's latest version governs
// only the days up to that later version, so it cannot change what the account
// is today. The contract: lifecycle and structural changes take effect on or
// after the latest version; an earlier-dated version is a historical
// correction of descriptive fields, keeping the structure and lifecycle the
// account had on that date. Parent rules hold on every date a new version
// governs, not only today.

func TestBackdatedLifecycleAndStructureVersionsAreRefused(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	usdText := strconvFormatInt(usd)
	savings := createAccountForSession(t, handler, cookie, csrf, `{
		"name":"Savings","account_class":"asset","account_kind":"savings",
		"default_commodity_id":`+usdText+`,"opened_on":"2026-01-01"
	}`)
	path := "/api/v1/accounts/" + strconvFormatInt(savings.ID)
	patchAccount(t, handler, cookie, csrf, savings.ID, `{
		"name":"Reserve","account_class":"asset","account_kind":"savings",
		"default_commodity_id":`+usdText+`,"effective_from":"2026-06-01"
	}`, http.StatusOK)

	for _, refusal := range []struct{ name, method, suffix, body string }{
		{"close before the latest version", http.MethodPost, "/close", `{"closed_on":"2026-03-01"}`},
		{"kind change before the latest version", http.MethodPatch, "", `{
			"name":"Reserve","account_class":"asset","account_kind":"checking",
			"default_commodity_id":` + usdText + `,"effective_from":"2026-03-01"
		}`},
		{"posting behavior change before the latest version", http.MethodPatch, "", `{
			"name":"Reserve","account_class":"asset","account_kind":"savings","allows_postings":false,
			"default_commodity_id":` + usdText + `,"effective_from":"2026-03-01"
		}`},
	} {
		res := referenceDataRequest(handler, cookie, csrf, refusal.method, path+refusal.suffix, refusal.body)
		require.Equalf(t, http.StatusConflict, res.Code, "%s: %s", refusal.name, res.Body.String())
		// The shared reference-data refusal (#552, #566), so the client
		// names the earliest allowed date in every locale.
		assert.JSONEq(t, `{"error":{"code":"REFERENCE_VERSION_BACKDATED","message":"reference version is backdated: lifecycle or structural changes must take effect on or after 2026-06-01","conflict_date":"2026-06-01"}}`, res.Body.String(), refusal.name)
	}
	current := readAccountForSession(t, handler, cookie, savings.ID)
	assert.Equal(t, "active", current.Status)
	assert.Equal(t, "savings", current.AccountKind)

	// On or after the latest version, the same changes are ordinary.
	closed := mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, path+"/close", `{"closed_on":"2026-06-01"}`, http.StatusOK)
	assert.Equal(t, "closed", closed.Status)
	assert.Equal(t, "closed", readAccountForSession(t, handler, cookie, savings.ID).Status)

	// The audit's sequence: a reopen today, then a close dated earlier, used
	// to answer "closed" and leave the account active.
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, path+"/reopen", `{}`, http.StatusOK)
	res := referenceDataRequest(handler, cookie, csrf, http.MethodPost, path+"/close", `{"closed_on":"2026-07-01"}`)
	require.Equalf(t, http.StatusConflict, res.Code, res.Body.String())
	assert.Equal(t, "active", readAccountForSession(t, handler, cookie, savings.ID).Status)
}

func TestBackdatedRenameKeepsTheStructureAndLifecycleOfItsDate(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	usdText := strconvFormatInt(usd)
	savings := createAccountForSession(t, handler, cookie, csrf, `{
		"name":"A","account_class":"asset","account_kind":"savings",
		"default_commodity_id":`+usdText+`,"opened_on":"2026-01-01"
	}`)
	patchAccount(t, handler, cookie, csrf, savings.ID, `{
		"name":"B","account_class":"asset","account_kind":"savings",
		"default_commodity_id":`+usdText+`,"effective_from":"2026-06-01"
	}`, http.StatusOK)
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(savings.ID)+"/close", `{"closed_on":"2026-08-01"}`, http.StatusOK)

	// The editor sends today's full account, closed. The correction takes the
	// name only; on 2026-03-01 the account was open, and it stays open there.
	// The response is the account as it stands today, closed and named B,
	// not the March version just written (#566).
	corrected := patchAccount(t, handler, cookie, csrf, savings.ID, `{
		"name":"C","account_class":"asset","account_kind":"savings",
		"default_commodity_id":`+usdText+`,"effective_from":"2026-03-01"
	}`, http.StatusOK)
	assert.Equal(t, readAccountForSession(t, handler, cookie, savings.ID), corrected)
	assert.Equal(t, "B", corrected.Name)
	assert.Equal(t, "2026-08-01", corrected.EffectiveFrom)
	assert.Equal(t, "closed", corrected.Status)
	assert.Equal(t, "2026-08-01", corrected.ClosedOn)

	for _, read := range []struct{ asOf, name, status string }{
		{"2026-02-01", "A", "active"},
		{"2026-04-01", "C", "active"},
		{"2026-07-01", "B", "active"},
		{"2026-09-01", "B", "closed"},
	} {
		balances := readAccountBalancesForSession(t, handler, cookie, "?as_of="+read.asOf)
		account := balanceAccountByID(t, balances.Accounts, savings.ID)
		assert.Equal(t, read.name, account.Name, read.asOf)
		assert.Equal(t, read.status, account.Status, read.asOf)
	}
	current := readAccountForSession(t, handler, cookie, savings.ID)
	assert.Equal(t, "B", current.Name)
	assert.Equal(t, "closed", current.Status)

	// An ordinary edit, dated on the latest version, answers with its own
	// values: the version written is today's state.
	renamed := patchAccount(t, handler, cookie, csrf, savings.ID, `{
		"name":"D","account_class":"asset","account_kind":"savings",
		"default_commodity_id":`+usdText+`,"effective_from":"2026-08-01"
	}`, http.StatusOK)
	assert.Equal(t, "D", renamed.Name)
	assert.Equal(t, "closed", renamed.Status)
	assert.Equal(t, readAccountForSession(t, handler, cookie, savings.ID), renamed)
}

// An account opened before its first version has no version to correct on
// the days in between: the shared before-first refusal names the first
// version's date (#566).
func TestAccountCorrectionBeforeFirstVersionReturnsCodedConflict(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	usdText := strconvFormatInt(usd)
	savings := createAccountForSession(t, handler, cookie, csrf, `{
		"name":"Savings","account_class":"asset","account_kind":"savings",
		"default_commodity_id":`+usdText+`,"opened_on":"2026-01-01","effective_from":"2026-02-01"
	}`)
	var before, after int
	require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM audit_events").Scan(&before))
	res := referenceDataRequest(handler, cookie, csrf, http.MethodPatch, "/api/v1/accounts/"+strconvFormatInt(savings.ID), `{
		"name":"January name","account_class":"asset","account_kind":"savings",
		"default_commodity_id":`+usdText+`,"effective_from":"2026-01-15"
	}`)
	require.Equal(t, http.StatusConflict, res.Code, res.Body.String())
	assert.JSONEq(t, `{"error":{"code":"REFERENCE_VERSION_BEFORE_FIRST","message":"reference version is dated before the record's first version: the record starts on 2026-02-01","conflict_date":"2026-02-01"}}`, res.Body.String())
	require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM audit_events").Scan(&after))
	assert.Equal(t, before, after)
	assert.Equal(t, "Savings", readAccountForSession(t, handler, cookie, savings.ID).Name)
}

func TestParentCycleIsRefusedOnEveryDateTheVersionGoverns(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	container := func(name string) accountResponse {
		return createAccountForSession(t, handler, cookie, csrf, `{"name":"`+name+`","account_class":"asset","account_kind":"other_asset","allows_postings":false,"opened_on":"2026-01-01"}`)
	}
	x, y := container("X"), container("Y")
	xText, yText := strconvFormatInt(x.ID), strconvFormatInt(y.ID)
	reparent := func(id int64, name, parent, effectiveFrom string) *httpResult {
		parentField := ""
		if parent != "" {
			parentField = `,"parent_account_id":` + parent
		}
		res := referenceDataRequest(handler, cookie, csrf, http.MethodPatch, "/api/v1/accounts/"+strconvFormatInt(id), `{
			"name":"`+name+`","account_class":"asset","account_kind":"other_asset","allows_postings":false`+parentField+`,
			"effective_from":"`+effectiveFrom+`"
		}`)
		return &httpResult{code: res.Code, body: res.Body.String(), errorCode: func() string { return apiErrorCode(t, res) }}
	}

	require.Equal(t, http.StatusOK, reparent(y.ID, "Y", xText, "2026-02-01").code)
	require.Equal(t, http.StatusOK, reparent(y.ID, "Y", "", "2026-06-01").code)

	// Today Y is a root, but from 2026-02-01 to 2026-05-31 Y sits under X:
	// X under Y from 2026-01-01 would make X its own ancestor in that window.
	cycle := reparent(x.ID, "X", yText, "2026-01-01")
	require.Equalf(t, http.StatusBadRequest, cycle.code, cycle.body)
	assert.Equal(t, "VALIDATION_FAILED", cycle.errorCode())
	balances := readAccountBalancesForSession(t, handler, cookie, "?as_of=2026-04-01")
	assert.Nil(t, balanceAccountByID(t, balances.Accounts, x.ID).ParentAccountID)

	// From 2026-06-01 on, Y is a root again, so the same move is fine.
	require.Equal(t, http.StatusOK, reparent(x.ID, "X", yText, "2026-07-01").code)
}

type httpResult struct {
	code      int
	body      string
	errorCode func() string
}
