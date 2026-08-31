package api

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Lifecycle tests use the real producer; the public creation route never gets
// an exemption from the draft origin guard for test fixtures.
func generateRecurringDraft(t *testing.T, handler http.Handler, database *sql.DB, cookie *http.Cookie, date string, accountID, categoryID, commodityID, amount int64) transactionResponse {
	t.Helper()
	service := app.NewRecurringService(db.NewRecurringRepository(database), app.NewTransactionService(
		db.NewTransactionRepository(database), db.NewPayeeRepository(database), db.NewAccountRepository(database), db.NewCommodityRepository(database)),
		app.NewSettingsService(db.NewSettingsRepository(database)))
	now, err := time.Parse(time.DateOnly, date)
	require.NoError(t, err)
	service.SetNowForTest(func() time.Time { return now.Add(12 * time.Hour) })
	ownerID, err := db.NewRecurringRepository(database).CurrentBookOwnerID(context.Background(), app.BookID)
	require.NoError(t, err)
	name, frequency, lead := "Lifecycle test", "daily", 0
	postings := []db.RecurringTemplatePostingSpec{
		{AccountID: accountID, CommodityID: commodityID, QuantityValue: exact.New(-amount), QuantityScale: 2},
		{AccountID: categoryID, CommodityID: commodityID, QuantityValue: exact.New(amount), QuantityScale: 2},
	}
	template, err := service.CreateTemplate(context.Background(), app.WriteRecurringTemplateInput{OwnerUserID: ownerID,
		Patch: app.RecurringTemplatePatch{Name: &name, Frequency: &frequency, StartsOn: &date, LeadDays: &lead, Postings: &postings}})
	require.NoError(t, err)
	result, err := service.GenerateDue(context.Background(), app.GenerateRecurringInput{OwnerUserID: ownerID, TemplateID: template.ID})
	require.NoError(t, err)
	require.Equal(t, 1, result.Generated)
	occurrences, err := db.NewRecurringRepository(database).ListRecurringOccurrences(context.Background(), app.BookID, template.ID, 10)
	require.NoError(t, err)
	require.Len(t, occurrences, 1)
	return readTransactionForSession(t, handler, cookie, occurrences[0].TransactionID.Int64, http.StatusOK)
}

func TestDiscardingGeneratedDraftPreservesOccurrenceAndCannotRegenerate(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	account := createLedgerAccount(t, handler, cookie, csrf, "Recurring Checking", "asset", "checking", commodityID, 2)
	category := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Recurring Expense","category_type":"expense"}`)
	draft := generateRecurringDraft(t, handler, database, cookie, "2026-06-07", account.ID, category.ID, commodityID, 10000)
	mutateTransactionNoBody(t, handler, cookie, csrf, http.MethodDelete, "/api/v1/transactions/"+strconvFormatInt(draft.ID), http.StatusNoContent)
	var templateID, auditID int64
	var status, reason string
	var transactionID sql.NullInt64
	require.NoError(t, database.QueryRow(`SELECT template_id, status, transaction_id, skip_reason, last_audit_event_id FROM recurring_occurrences`).Scan(&templateID, &status, &transactionID, &reason, &auditID))
	assert.Equal(t, "skipped", status)
	assert.False(t, transactionID.Valid)
	assert.NotEmpty(t, reason)
	var origin, operation, metadata string
	var actor int64
	require.NoError(t, database.QueryRow(`SELECT origin_type, operation, actor_user_id, metadata_json FROM audit_events WHERE id = ?`, auditID).Scan(&origin, &operation, &actor, &metadata))
	assert.Equal(t, "browser_api", origin)
	assert.Equal(t, "transaction.delete_draft", operation)
	assert.Positive(t, actor)
	assert.Contains(t, metadata, `"transaction_id":`+strconvFormatInt(draft.ID))
	// Force re-enumeration, as a schedule edit does: the tombstone, rather
	// than the watermark alone, must suppress regeneration.
	_, err := database.Exec(`UPDATE recurring_templates SET generate_from = '2026-06-07', revision = revision + 1 WHERE id = ?`, templateID)
	require.NoError(t, err)
	service := app.NewRecurringService(db.NewRecurringRepository(database), app.NewTransactionService(db.NewTransactionRepository(database), db.NewPayeeRepository(database), db.NewAccountRepository(database), db.NewCommodityRepository(database)), app.NewSettingsService(db.NewSettingsRepository(database)))
	service.SetNowForTest(func() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) })
	result, err := service.GenerateDue(context.Background(), app.GenerateRecurringInput{OwnerUserID: actor})
	require.NoError(t, err)
	assert.Zero(t, result.Generated)
	var count int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&count))
	assert.Zero(t, count)
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM recurring_occurrences`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestBrowserApiCannotCreateADraft(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	var before int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&before))
	res := recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions", `{"status":"draft"}`, http.StatusBadRequest)
	assert.Contains(t, res.Body.String(), "TRANSACTION_DRAFT_NOT_USER_CREATABLE")
	var count int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&count))
	assert.Zero(t, count)
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&count))
	assert.Equal(t, before, count)
	// Run-now cannot generate an unknown template.
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/recurring/templates/1/run-now", `{}`, http.StatusNotFound)
}

func TestGeneratedDraftDoesNotAffectReconciledBalance(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	account := createLedgerAccount(t, handler, cookie, csrf, "Protected Checking", "asset", "checking", commodityID, 2)
	category := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Protected Expense","category_type":"expense"}`)
	posted := createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-06-08",
		posting(account.ID, -10000, 2, commodityID), posting(category.ID, 10000, 2, commodityID)), http.StatusCreated)
	reconcilePostingForSession(t, handler, cookie, csrf, account.ID, commodityID, posted.JournalEntries[0].Postings[0], "2026-06-08")
	before := listReconciliationCheckpointsForSession(t, handler, cookie, account.ID, "")
	draft := generateRecurringDraft(t, handler, database, cookie, "2026-06-07", account.ID, category.ID, commodityID, 5000)
	assert.Empty(t, draft.InvalidatedCheckpointIDs)
	assert.Equal(t, before, listReconciliationCheckpointsForSession(t, handler, cookie, account.ID, ""))
	// Editing the draft before posting is equally outside the ledger.
	edited := mutateTransaction(t, handler, cookie, csrf, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(draft.ID), balancedBody("2026-06-07",
		posting(account.ID, -6000, 2, commodityID), posting(category.ID, 6000, 2, commodityID)), http.StatusOK)
	assert.Equal(t, "draft", edited.Status)
	assert.Equal(t, before, listReconciliationCheckpointsForSession(t, handler, cookie, account.ID, ""))
	visible := listTransactionsForSession(t, handler, cookie, "")
	require.Len(t, visible.Transactions, 1)
	assert.Equal(t, posted.ID, visible.Transactions[0].ID)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(draft.ID)+"/post", `{}`, http.StatusConflict)
	assert.Equal(t, before, listReconciliationCheckpointsForSession(t, handler, cookie, account.ID, ""))
	promoted := mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(draft.ID)+"/post", `{"reconciliation_override":true}`, http.StatusOK)
	assert.NotEmpty(t, promoted.InvalidatedCheckpointIDs)
}

func TestDiscardGeneratedDraftRollsBackTombstoneAndAuditOnDeleteFailure(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	account := createLedgerAccount(t, handler, cookie, csrf, "Rollback Checking", "asset", "checking", commodityID, 2)
	category := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Rollback Expense","category_type":"expense"}`)
	draft := generateRecurringDraft(t, handler, database, cookie, "2026-06-07", account.ID, category.ID, commodityID, 10000)
	_, err := database.Exec(`CREATE TRIGGER fail_draft_delete BEFORE DELETE ON transactions BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
	var before, after int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&before))
	mutateTransactionNoBody(t, handler, cookie, csrf, http.MethodDelete, "/api/v1/transactions/"+strconvFormatInt(draft.ID), http.StatusInternalServerError)
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&after))
	assert.Equal(t, before, after)
	var status, reason string
	var id int64
	require.NoError(t, database.QueryRow(`SELECT status, skip_reason, transaction_id FROM recurring_occurrences`).Scan(&status, &reason, &id))
	assert.Equal(t, "generated", status)
	assert.Empty(t, reason)
	assert.Equal(t, draft.ID, id)
	assert.Equal(t, draft, readTransactionForSession(t, handler, cookie, draft.ID, http.StatusOK))
	_, err = database.Exec(`DROP TRIGGER fail_draft_delete`)
	require.NoError(t, err)
	// Posting makes the transaction durable; even a later discard attempt
	// must leave both the occurrence and its posted journal untouched.
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(draft.ID)+"/post", `{}`, http.StatusOK)
	mutateTransactionNoBody(t, handler, cookie, csrf, http.MethodDelete, "/api/v1/transactions/"+strconvFormatInt(draft.ID), http.StatusConflict)
	require.NoError(t, database.QueryRow(`SELECT status, transaction_id FROM recurring_occurrences`).Scan(&status, &id))
	assert.Equal(t, "generated", status)
	assert.Equal(t, draft.ID, id)
}
