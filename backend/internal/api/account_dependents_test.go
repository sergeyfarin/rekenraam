package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #294. "Active accounts cannot be parented under archived accounts"
// (accounts-system-design.md). Every account write enforces that, so archiving
// the parent — or the institution an account uses — while the dependent is
// still in use left an account that could not even be renamed.

func TestArchivingAParentOrInstitutionWithDependentsIsRefused(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	usdText := strconvFormatInt(usd)
	salary := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Salary","category_type":"income"}`)
	institution := createInstitutionForSession(t, handler, cookie, csrf, `{"name":"Bank","kind":"bank","country_code":"NL"}`)
	institutionText := strconvFormatInt(institution.ID)

	parent := createAccountForSession(t, handler, cookie, csrf, `{"name":"Parent","account_class":"asset","account_kind":"other_asset","allows_postings":false,"opened_on":"2026-01-01"}`)
	parentPath := "/api/v1/accounts/" + strconvFormatInt(parent.ID)
	child := createAccountForSession(t, handler, cookie, csrf, `{
		"name":"Child","account_class":"asset","account_kind":"checking","default_commodity_id":`+usdText+`,
		"quantity_scale_override":2,"parent_account_id":`+strconvFormatInt(parent.ID)+`,"opened_on":"2026-01-01"
	}`)
	childPath := "/api/v1/accounts/" + strconvFormatInt(child.ID)
	banked := createAccountForSession(t, handler, cookie, csrf, `{
		"name":"Banked","account_class":"asset","account_kind":"checking","default_commodity_id":`+usdText+`,
		"quantity_scale_override":2,"institution_id":`+institutionText+`,"opened_on":"2026-01-01"
	}`)
	bankedPath := "/api/v1/accounts/" + strconvFormatInt(banked.ID)
	for _, accountID := range []int64{child.ID, banked.ID} {
		createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-03-15",
			posting(accountID, 10000, 2, usd), posting(salary.ID, -10000, 2, usd)), http.StatusCreated)
	}

	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, parentPath+"/close", `{}`, http.StatusOK)
	for _, refusal := range []struct{ name, path string }{
		{"parent with an active child", parentPath + "/archive"},
		{"institution with an active account", "/api/v1/institutions/" + institutionText + "/archive"},
	} {
		res := referenceDataRequest(handler, cookie, csrf, http.MethodPost, refusal.path, `{}`)
		require.Equalf(t, http.StatusConflict, res.Code, "%s: %s", refusal.name, res.Body.String())
		want := "CONFLICT"
		if strings.HasPrefix(refusal.path, "/api/v1/institutions/") {
			// Institution archive names the account and date (#552).
			want = "REFERENCE_DATED_CONFLICT"
		}
		assert.Equal(t, want, apiErrorCode(t, res), refusal.name)
	}

	// A closed dependent still counts: its next rename would be refused too.
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, childPath+"/close", `{}`, http.StatusOK)
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, bankedPath+"/close", `{}`, http.StatusOK)
	require.Equal(t, http.StatusConflict, referenceDataRequest(handler, cookie, csrf, http.MethodPost, parentPath+"/archive", `{}`).Code)
	require.Equal(t, http.StatusConflict, referenceDataRequest(handler, cookie, csrf, http.MethodPost, "/api/v1/institutions/"+institutionText+"/archive", `{}`).Code)

	// Once the dependents are archived, the parent and institution can follow.
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, childPath+"/archive", `{}`, http.StatusOK)
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, bankedPath+"/archive", `{}`, http.StatusOK)
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, parentPath+"/archive", `{}`, http.StatusOK)
	mutateInstitution(t, handler, cookie, csrf, http.MethodPost, "/api/v1/institutions/"+institutionText+"/archive", http.StatusOK)

	// Restoring a dependent while its parent or institution stays archived
	// would recreate the stranded state.
	for _, path := range []string{childPath, bankedPath} {
		res := referenceDataRequest(handler, cookie, csrf, http.MethodPost, path+"/restore", `{}`)
		require.Equalf(t, http.StatusBadRequest, res.Code, "%s: %s", path, res.Body.String())
		assert.Equal(t, "VALIDATION_FAILED", apiErrorCode(t, res))
	}
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, parentPath+"/restore", `{}`, http.StatusOK)
	mutateInstitution(t, handler, cookie, csrf, http.MethodPost, "/api/v1/institutions/"+institutionText+"/restore", http.StatusOK)
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, childPath+"/restore", `{}`, http.StatusOK)
	restored := mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, bankedPath+"/restore", `{}`, http.StatusOK)
	assert.Equal(t, "closed", restored.Status)
}

// #295. A system account is never a user account's parent: the child would
// sit in a hidden subtree, and a report expanding that system account's
// descendants would include user money.
func TestSystemAccountsAreNotAcceptedAsParents(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	mutateAccount(t, handler, cookie, csrf, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)
	system := listAccountsForSession(t, handler, cookie, "?include_system=true")
	usdText := strconvFormatInt(usd)
	savings := createAccountForSession(t, handler, cookie, csrf, `{"name":"Savings","account_class":"asset","account_kind":"savings","default_commodity_id":`+usdText+`}`)

	for _, account := range system.Accounts {
		if !account.IsSystem || account.AccountClass == "equity" {
			continue
		}
		class, kind := account.AccountClass, "savings"
		if class != "asset" {
			kind = class
		}
		parentText := strconvFormatInt(account.ID)
		create := referenceDataRequest(handler, cookie, csrf, http.MethodPost, "/api/v1/accounts", `{
			"name":"Under `+account.SystemRole+`","account_class":"`+class+`","account_kind":"`+kind+`",
			"default_commodity_id":`+usdText+`,"parent_account_id":`+parentText+`
		}`)
		require.Equalf(t, http.StatusBadRequest, create.Code, "%s: %s", account.SystemRole, create.Body.String())
		assert.Equal(t, "VALIDATION_FAILED", apiErrorCode(t, create))
		if class == "asset" {
			move := referenceDataRequest(handler, cookie, csrf, http.MethodPatch, "/api/v1/accounts/"+strconvFormatInt(savings.ID), `{
				"name":"Savings","account_class":"asset","account_kind":"savings",
				"default_commodity_id":`+usdText+`,"parent_account_id":`+parentText+`
			}`)
			require.Equalf(t, http.StatusBadRequest, move.Code, "%s: %s", account.SystemRole, move.Body.String())
			assert.Equal(t, "VALIDATION_FAILED", apiErrorCode(t, move))
		}
	}
}
