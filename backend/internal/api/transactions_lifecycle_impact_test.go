package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLifecycleReconciliationImpactNamesEveryCheckpointTheWriteInvalidates is
// the #220 regression. A refused void or unvoid once previewed its impact
// through the edit endpoint with an empty body, which fails validation, so
// the owner could never see the impact or deliberately continue. The lifecycle
// preview names the complete invalidation set — later checkpoints included —
// writes nothing, and the override invalidates exactly that set.
func TestLifecycleReconciliationImpactNamesEveryCheckpointTheWriteInvalidates(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, cookie, csrf, "Lifecycle Impact Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Lifecycle Impact Expense","category_type":"expense"}`)
	spend := func(date string, cents int64) transactionResponse {
		return createTransactionForSession(t, handler, cookie, csrf, balancedBody(date,
			posting(checking.ID, -cents, 2, commodityID),
			posting(expense.ID, cents, 2, commodityID),
		), http.StatusCreated)
	}
	reconcile := func(tx transactionResponse, date string, balance string) int64 {
		changePostingReconciliationStatusForSession(t, handler, cookie, csrf, checking.ID, `{
			"posting_version_ids":[`+strconvFormatInt(tx.JournalEntries[0].Postings[0].ID)+`],"status":"cleared","cleared_on":"`+date+`"
		}`, http.StatusOK)
		started := startReconciliationForSession(t, handler, cookie, csrf, checking.ID, `{
			"commodity_id":`+strconvFormatInt(commodityID)+`,"source_kind":"statement","statement_date":"`+date+`",
			"statement_balance_value":"`+balance+`","statement_balance_scale":2
		}`, http.StatusCreated)
		finishReconciliationForSession(t, handler, cookie, csrf, started.ID, `{"change_reason":"matched statement"}`, http.StatusOK)
		checkpoints := listCheckpointsForSession(t, handler, cookie, checking.ID)
		for _, checkpoint := range checkpoints {
			if checkpoint.SessionID != nil && *checkpoint.SessionID == started.ID {
				return checkpoint.ID
			}
		}
		require.FailNow(t, "checkpoint not found for session")
		return 0
	}
	activeCheckpoints := func() []int64 {
		var ids []int64
		for _, checkpoint := range listCheckpointsForSession(t, handler, cookie, checking.ID) {
			if checkpoint.Status == "active" {
				ids = append(ids, checkpoint.ID)
			}
		}
		return ids
	}
	preview := func(action string, id int64) []int64 {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/"+strconvFormatInt(id)+"/"+action+"/reconciliation-impact", nil)
		req.AddCookie(cookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		require.Equalf(t, http.StatusOK, res.Code, "%s preview body: %s", action, res.Body.String())
		var impact reconciliationImpactResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&impact))
		ids := []int64{}
		for _, checkpoint := range impact.AffectedCheckpoints {
			if assert.NotNil(t, checkpoint.AccountName) {
				assert.Equal(t, "Lifecycle Impact Checking", *checkpoint.AccountName)
			}
			ids = append(ids, checkpoint.CheckpointID)
		}
		return ids
	}
	lifecycle := func(action string, id int64, override bool, want int) transactionResponse {
		return mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(id)+"/"+action,
			`{"change_reason":"statement fix","reconciliation_override":`+strconv.FormatBool(override)+`}`, want)
	}

	early := spend("2026-06-01", 1000)
	before := reconcile(early, "2026-06-01", "-1000")
	subject := spend("2026-06-07", 10000)
	covering := reconcile(subject, "2026-06-07", "-11000")
	late := spend("2026-06-20", 5000)
	later := reconcile(late, "2026-06-20", "-16000")

	// Void: refused, previewed with the later checkpoint, cancel writes nothing.
	lifecycle("void", subject.ID, false, http.StatusConflict)
	assert.ElementsMatch(t, []int64{covering, later}, preview("void", subject.ID))
	assert.ElementsMatch(t, []int64{before, covering, later}, activeCheckpoints(), "refusal and preview write nothing")
	assert.Equal(t, "posted", readTransactionForSession(t, handler, cookie, subject.ID, http.StatusOK).Status)

	voided := lifecycle("void", subject.ID, true, http.StatusOK)
	assert.Equal(t, "voided", voided.Status)
	assert.ElementsMatch(t, []int64{covering, later}, voided.InvalidatedCheckpointIDs)
	assert.Equal(t, []int64{before}, activeCheckpoints(), "only the previewed checkpoints are invalidated")
	assert.Empty(t, preview("void", subject.ID), "voiding again changes nothing")
	assert.Empty(t, preview("unvoid", subject.ID), "no active checkpoint covers the voided entry")

	// Unvoid after a new statement covers the date: same refusal and preview.
	// The late spend's checkpoint was invalidated, so it is re-verified on
	// this statement too (#240): -1000 opening, -5000 late, -2000 fresh.
	fresh := spend("2026-06-21", 2000)
	again := reconcile(fresh, "2026-06-21", "-8000")
	lifecycle("unvoid", subject.ID, false, http.StatusConflict)
	assert.Equal(t, []int64{again}, preview("unvoid", subject.ID))
	assert.ElementsMatch(t, []int64{before, again}, activeCheckpoints())
	assert.Equal(t, "voided", readTransactionForSession(t, handler, cookie, subject.ID, http.StatusOK).Status)

	unvoided := lifecycle("unvoid", subject.ID, true, http.StatusOK)
	assert.Equal(t, "posted", unvoided.Status)
	assert.Equal(t, []int64{again}, unvoided.InvalidatedCheckpointIDs)
	assert.Equal(t, []int64{before}, activeCheckpoints())

	// Soft delete and restore preview through the same guard.
	// Everything since the opening balance is re-verified: the unvoided
	// subject and the invalidated statement's postings, plus the new spend.
	again = reconcile(spend("2026-06-22", 3000), "2026-06-22", "-21000")
	assert.Equal(t, []int64{again}, preview("soft-delete", subject.ID))
	assert.Empty(t, preview("restore", subject.ID), "restoring a live transaction changes nothing")
}
