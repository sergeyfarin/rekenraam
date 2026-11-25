package api

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #292. `closed_on` is "the first date after which ordinary new postings
// should not target this account" (accounts-system-design.md): the closing day
// itself is still open, the day after is not, and an account may not be
// closed — or otherwise versioned — so that a posting it already has falls
// outside those dates.

func TestAccountCloseRefusesADateBeforeExistingPostings(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	checking := createAccountForSession(t, handler, cookie, csrf, `{
		"name":"Checking","account_class":"asset","account_kind":"checking",
		"default_commodity_id":`+strconvFormatInt(usd)+`,"quantity_scale_override":2,"opened_on":"2026-01-01"
	}`)
	salary := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Salary","category_type":"income"}`)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-03-15",
		posting(checking.ID, 10000, 2, usd), posting(salary.ID, -10000, 2, usd)), http.StatusCreated)
	path := "/api/v1/accounts/" + strconvFormatInt(checking.ID)

	res := referenceDataRequest(handler, cookie, csrf, http.MethodPost, path+"/close", `{"closed_on":"2026-02-01"}`)
	require.Equalf(t, http.StatusConflict, res.Code, res.Body.String())
	assert.Contains(t, res.Body.String(), "2026-03-15")
	assert.Equal(t, "CONFLICT", apiErrorCode(t, res))
	assert.Equal(t, "active", readAccountForSession(t, handler, cookie, checking.ID).Status)
	versions := listAccountsForSession(t, handler, cookie, "/"+strconvFormatInt(checking.ID)+"/versions")
	assert.Len(t, versions.Accounts, 1, "a refused close leaves no version behind")

	// Closing on the day of the last posting is allowed.
	closed := mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, path+"/close", `{"closed_on":"2026-03-15"}`, http.StatusOK)
	assert.Equal(t, "closed", closed.Status)

	// An archive effective on the closing day would govern that posting
	// (same date, later version): the same stranding by another route.
	res = referenceDataRequest(handler, cookie, csrf, http.MethodPost, path+"/archive", `{"effective_from":"2026-03-15"}`)
	require.Equalf(t, http.StatusConflict, res.Code, res.Body.String())
	assert.Equal(t, "CONFLICT", apiErrorCode(t, res))
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, path+"/archive", `{}`, http.StatusOK)
}

func TestAccountClosingDayIsStillPostable(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	checking := createAccountForSession(t, handler, cookie, csrf, `{
		"name":"Checking","account_class":"asset","account_kind":"checking",
		"default_commodity_id":`+strconvFormatInt(usd)+`,"quantity_scale_override":2,"opened_on":"2026-01-01"
	}`)
	salary := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Salary","category_type":"income"}`)
	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(checking.ID)+"/close", `{"closed_on":"2026-02-01"}`, http.StatusOK)

	final := createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-02-01",
		posting(checking.ID, -500, 2, usd), posting(salary.ID, 500, 2, usd)), http.StatusCreated)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-02-02",
		posting(checking.ID, -500, 2, usd), posting(salary.ID, 500, 2, usd)), http.StatusBadRequest)

	// The closing-day transaction stays correctable.
	mutateTransaction(t, handler, cookie, csrf, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(final.ID), balancedBody("2026-02-01",
		posting(checking.ID, -600, 2, usd), posting(salary.ID, 600, 2, usd)), http.StatusOK)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(final.ID)+"/void", `{"change_reason":"wrong amount"}`, http.StatusOK)
}

// A book written before the close refusal existed can already hold a posting
// after its account's close. Correcting that posting must answer with a
// validation result naming the cause, never an internal error, and reopening
// from the closing date repairs it.
func TestIneligibleAccountDateOnCorrectionIsAValidationError(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	checking := createAccountForSession(t, handler, cookie, csrf, `{
		"name":"Checking","account_class":"asset","account_kind":"checking",
		"default_commodity_id":`+strconvFormatInt(usd)+`,"quantity_scale_override":2,"opened_on":"2026-01-01"
	}`)
	salary := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Salary","category_type":"income"}`)
	tx := createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-03-15",
		posting(checking.ID, 10000, 2, usd), posting(salary.ID, -10000, 2, usd)), http.StatusCreated)
	insertStrandingCloseVersion(t, database, checking.ID, "2026-02-01")

	txPath := "/api/v1/transactions/" + strconvFormatInt(tx.ID)
	res := referenceDataRequest(handler, cookie, csrf, http.MethodPost, txPath+"/void", `{"change_reason":"probe"}`)
	require.Equalf(t, http.StatusBadRequest, res.Code, res.Body.String())
	assert.Equal(t, "VALIDATION_FAILED", apiErrorCode(t, res))

	mutateAccountWithBody(t, handler, cookie, csrf, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(checking.ID)+"/reopen", `{"effective_from":"2026-02-01"}`, http.StatusOK)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, txPath+"/void", `{"change_reason":"repaired"}`, http.StatusOK)
}

// insertStrandingCloseVersion writes the state the API now refuses: a closed
// version effective before a posting the account already has.
func insertStrandingCloseVersion(t *testing.T, database *sql.DB, accountID int64, closedOn string) {
	t.Helper()
	insertAccountVersionCopy(t, database, accountID, "closed", closedOn)
}

// insertAccountVersionCopy appends a copy of the account's latest version with
// status, effective from and closed on date, bypassing every service check.
func insertAccountVersionCopy(t *testing.T, database *sql.DB, accountID int64, status string, date string) {
	t.Helper()
	ctx := context.Background()
	rows, err := database.QueryContext(ctx, `SELECT name FROM pragma_table_info('account_versions') WHERE name <> 'id'`)
	require.NoError(t, err)
	var columns, values []string
	for rows.Next() {
		var column string
		require.NoError(t, rows.Scan(&column))
		columns = append(columns, column)
		switch column {
		case "version_seq":
			values = append(values, "version_seq + 1")
		case "effective_from", "closed_on":
			values = append(values, "'"+date+"'")
		case "status":
			values = append(values, "'"+status+"'")
		default:
			values = append(values, column)
		}
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	_, err = database.ExecContext(ctx, `
		INSERT INTO account_versions (`+strings.Join(columns, ", ")+`)
		SELECT `+strings.Join(values, ", ")+`
		FROM account_versions
		WHERE account_id = ?
		ORDER BY version_seq DESC
		LIMIT 1
	`, accountID)
	require.NoError(t, err)
}
