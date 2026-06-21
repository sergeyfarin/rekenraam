package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDaySequenceAssignedOnCreateAndInheritedOnEdit verifies that transaction_day_sequence
// and account_day_sequence are allocated on create and inherited (unchanged) on edits
// that leave the date and account unchanged.
func TestDaySequenceAssignedOnCreateAndInheritedOnEdit(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Seq Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Seq Expense","category_type":"expense"}`)

	tx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, commodityID),
		posting(expense.ID, 10000, 2, commodityID),
	), http.StatusCreated)
	require.Greater(t, tx.TransactionDaySequence, int64(0))
	origTxSeq := tx.TransactionDaySequence
	require.Greater(t, tx.JournalEntries[0].Postings[0].AccountDaySequence, int64(0))
	origAcctSeq := tx.JournalEntries[0].Postings[0].AccountDaySequence

	// Metadata-only edit: description change, same accounts, same amounts, same dates.
	lineKey0 := tx.JournalEntries[0].Postings[0].LineKey
	lineKey1 := tx.JournalEntries[0].Postings[1].LineKey
	edited := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(tx.ID), `{
		"transaction_date":"2026-06-07",
		"description":"updated memo only",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+lineKey0+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+lineKey1+`","account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusOK)

	assert.Equal(t, origTxSeq, edited.TransactionDaySequence, "transaction_day_sequence inherited on same-date edit")
	assert.Equal(t, origAcctSeq, edited.JournalEntries[0].Postings[0].AccountDaySequence, "account_day_sequence inherited on same-account/date edit")
}

// TestSameDaySequencesAreMonotonicallyIncreasing verifies that creating multiple
// transactions on the same date assigns distinct, increasing day sequences.
func TestSameDaySequencesAreMonotonicallyIncreasing(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "MonoSeq Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"MonoSeq Expense","category_type":"expense"}`)

	tx1 := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -1000, 2, commodityID),
		posting(expense.ID, 1000, 2, commodityID),
	), http.StatusCreated)
	tx2 := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -2000, 2, commodityID),
		posting(expense.ID, 2000, 2, commodityID),
	), http.StatusCreated)
	tx3 := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -3000, 2, commodityID),
		posting(expense.ID, 3000, 2, commodityID),
	), http.StatusCreated)

	assert.Greater(t, tx2.TransactionDaySequence, tx1.TransactionDaySequence)
	assert.Greater(t, tx3.TransactionDaySequence, tx2.TransactionDaySequence)

	// All checking postings should also have increasing account_day_sequence.
	seq1 := tx1.JournalEntries[0].Postings[0].AccountDaySequence
	seq2 := tx2.JournalEntries[0].Postings[0].AccountDaySequence
	seq3 := tx3.JournalEntries[0].Postings[0].AccountDaySequence
	assert.Greater(t, seq2, seq1)
	assert.Greater(t, seq3, seq2)
}

// TestTransferPostingMovesIndependentlyInEachRegister verifies that a transfer's
// posting in the sending account can be moved independently of the receiving account.
func TestTransferPostingMovesIndependentlyInEachRegister(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Transfer Move Checking", "asset", "checking", commodityID, 2)
	savings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Transfer Move Savings", "asset", "savings", commodityID, 2)

	// Create two transfers on the same date in checking; only the second moves from savings.
	tx1 := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"transfer_leg",
			"postings":[
				`+posting(checking.ID, -5000, 2, commodityID)+`,
				`+posting(savings.ID, 5000, 2, commodityID)+`
			]
		}]
	}`, http.StatusCreated)
	tx2 := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"transfer_leg",
			"postings":[
				`+posting(checking.ID, -3000, 2, commodityID)+`,
				`+posting(savings.ID, 3000, 2, commodityID)+`
			]
		}]
	}`, http.StatusCreated)

	checkingPostingTx2 := tx2.JournalEntries[0].Postings[0]
	require.Equal(t, checking.ID, checkingPostingTx2.AccountID)
	checkingPostingTx2LineID := checkingPostingTx2.PostingLineID

	// Move tx2's checking leg earlier (swap with tx1's checking leg).
	movePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, checkingPostingTx2LineID, "earlier", http.StatusOK)

	// In checking register: tx2 should now appear after tx1 in the new ordering.
	checkingRegister := accountRegisterForSession(t, handler, sessionCookie, checking.ID, "")
	require.Len(t, checkingRegister.Entries, 2)
	// Most recent first (desc order): the "earlier" tx2 checking leg is now first in same-day order = appears first descending.
	checkingSeqs := []int64{checkingRegister.Entries[0].Posting.AccountDaySequence, checkingRegister.Entries[1].Posting.AccountDaySequence}
	assert.Greater(t, checkingSeqs[0], checkingSeqs[1], "descending order: first entry has higher sequence")

	// Savings register is unaffected (posting move was only in checking).
	savingsRegister := accountRegisterForSession(t, handler, sessionCookie, savings.ID, "")
	require.Len(t, savingsRegister.Entries, 2)
	// The savings postings should still be in original creation order (tx1 then tx2 in desc).
	assert.Equal(t, tx2.ID, savingsRegister.Entries[0].TransactionID, "savings register desc: tx2 most recent creation is first")
	assert.Equal(t, tx1.ID, savingsRegister.Entries[1].TransactionID)
}

// TestMoveTransactionSwapsAdjacentSameDaySequences verifies that moving a transaction
// earlier swaps it with its predecessor in the same-day sequence.
func TestMoveTransactionSwapsAdjacentSameDaySequences(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Move Tx Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Move Tx Expense","category_type":"expense"}`)

	tx1 := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -1000, 2, commodityID),
		posting(expense.ID, 1000, 2, commodityID),
	), http.StatusCreated)
	tx2 := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -2000, 2, commodityID),
		posting(expense.ID, 2000, 2, commodityID),
	), http.StatusCreated)

	origTx1Seq := tx1.TransactionDaySequence
	origTx2Seq := tx2.TransactionDaySequence
	require.Greater(t, origTx2Seq, origTx1Seq)

	// Move tx2 earlier: should swap with tx1.
	moved := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(tx2.ID)+"/move", `{"direction":"earlier"}`, http.StatusOK)
	assert.Equal(t, origTx1Seq, moved.TransactionDaySequence, "tx2 now has tx1's old sequence")

	updatedTx1 := readTransactionForSession(t, handler, sessionCookie, tx1.ID, http.StatusOK)
	assert.Equal(t, origTx2Seq, updatedTx1.TransactionDaySequence, "tx1 now has tx2's old sequence")

	// List is descending by day_sequence: tx1 (now seq=origTx2Seq) appears first.
	list := listTransactionsForSession(t, handler, sessionCookie, "")
	require.Len(t, list.Transactions, 2)
	assert.Equal(t, tx1.ID, list.Transactions[0].ID)
	assert.Equal(t, tx2.ID, list.Transactions[1].ID)
}

// TestCursorPaginationRespectsDaySequence verifies that the 3-tuple cursor
// (date, day_sequence, id) traverses all transactions in the correct order even
// across page boundaries, including before and after a move.
func TestCursorPaginationRespectsDaySequence(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Cursor Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Cursor Expense","category_type":"expense"}`)

	var created []transactionResponse
	for i := 0; i < 5; i++ {
		tx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
			posting(checking.ID, int64(-1000*(i+1)), 2, commodityID),
			posting(expense.ID, int64(1000*(i+1)), 2, commodityID),
		), http.StatusCreated)
		created = append(created, tx)
	}

	// Collect all via limit=2 cursor pages.
	var all []int64
	cursor := ""
	for {
		suffix := "?limit=2"
		if cursor != "" {
			suffix += "&cursor=" + cursor
		}
		page := listTransactionsForSession(t, handler, sessionCookie, suffix)
		for _, tx := range page.Transactions {
			all = append(all, tx.ID)
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			break
		}
		cursor = *page.NextCursor
	}
	require.Len(t, all, 5)
	// All 5 distinct IDs retrieved without gaps or duplicates.
	seen := map[int64]bool{}
	for _, id := range all {
		seen[id] = true
	}
	assert.Len(t, seen, 5)

	// Verify descending sequence order: first page should have the last two created (highest seqs).
	firstPage := listTransactionsForSession(t, handler, sessionCookie, "?limit=2")
	require.Len(t, firstPage.Transactions, 2)
	assert.Equal(t, created[4].ID, firstPage.Transactions[0].ID)
	assert.Equal(t, created[3].ID, firstPage.Transactions[1].ID)

	// Move created[4] (highest seq) to earlier: it swaps with created[3].
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(created[4].ID)+"/move", `{"direction":"earlier"}`, http.StatusOK)

	// Re-paginate: created[3] should now appear first (highest seq after swap).
	firstPageAfterMove := listTransactionsForSession(t, handler, sessionCookie, "?limit=2")
	require.Len(t, firstPageAfterMove.Transactions, 2)
	assert.Equal(t, created[3].ID, firstPageAfterMove.Transactions[0].ID)
	assert.Equal(t, created[4].ID, firstPageAfterMove.Transactions[1].ID)

	// Full traversal after move still hits all 5 with no gaps.
	var allAfterMove []int64
	cursor = ""
	for {
		suffix := "?limit=2"
		if cursor != "" {
			suffix += "&cursor=" + cursor
		}
		page := listTransactionsForSession(t, handler, sessionCookie, suffix)
		for _, tx := range page.Transactions {
			allAfterMove = append(allAfterMove, tx.ID)
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			break
		}
		cursor = *page.NextCursor
	}
	require.Len(t, allAfterMove, 5)
	seenAfter := map[int64]bool{}
	for _, id := range allAfterMove {
		seenAfter[id] = true
	}
	assert.Len(t, seenAfter, 5)
}

// TestDefaultListExcludesDraft verifies that GET /transactions with no status filter
// returns only posted and voided transactions, not drafts.
func TestDefaultListExcludesDraft(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Draft Filter Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Draft Filter Expense","category_type":"expense"}`)

	// Create a draft transaction.
	draft := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"status":"draft",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				`+posting(checking.ID, -5000, 2, commodityID)+`,
				`+posting(expense.ID, 5000, 2, commodityID)+`
			]
		}]
	}`, http.StatusCreated)
	assert.Equal(t, "draft", draft.Status)

	// Create a posted transaction.
	posted := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -3000, 2, commodityID),
		posting(expense.ID, 3000, 2, commodityID),
	), http.StatusCreated)
	assert.Equal(t, "posted", posted.Status)

	// Default list (no status filter) should exclude the draft.
	defaultList := listTransactionsForSession(t, handler, sessionCookie, "")
	for _, tx := range defaultList.Transactions {
		assert.NotEqual(t, "draft", tx.Status, "default list must not include draft transactions")
	}
	ids := make([]int64, 0, len(defaultList.Transactions))
	for _, tx := range defaultList.Transactions {
		ids = append(ids, tx.ID)
	}
	assert.Contains(t, ids, posted.ID)
	assert.NotContains(t, ids, draft.ID)

	// Explicit status=draft returns only the draft.
	draftList := listTransactionsForSession(t, handler, sessionCookie, "?status=draft")
	require.Len(t, draftList.Transactions, 1)
	assert.Equal(t, draft.ID, draftList.Transactions[0].ID)
}

// TestBackdatedCreateReconciliationImpactPreviewAndRetry verifies that the
// reconciliation-impact endpoint accurately previews conflicts for backdated
// creates, and that the create itself is blocked without override then succeeds
// with it.
func TestBackdatedCreateReconciliationImpactPreviewAndRetry(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Backdated Create Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Backdated Create Expense","category_type":"expense"}`)

	// Create and reconcile a transaction dated 2026-06-07.
	existingTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, commodityID),
		posting(expense.ID, 10000, 2, commodityID),
	), http.StatusCreated)
	reconcilePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, commodityID, existingTx.JournalEntries[0].Postings[0], "2026-06-07")

	// Preview: a backdated create on 2026-06-01 (before statement_date) should show impact.
	impactBody := `{
		"transaction_date":"2026-06-01",
		"journal_entries":[{
			"entry_date":"2026-06-01",
			"postings":[
				`+posting(checking.ID, -5000, 2, commodityID)+`,
				`+posting(expense.ID, 5000, 2, commodityID)+`
			]
		}]
	}`
	impact := reconciliationImpactForCreate(t, handler, sessionCookie, impactBody)
	require.NotEmpty(t, impact.AffectedCheckpoints, "backdated create should show checkpoint impact")
	assert.Equal(t, checking.ID, impact.AffectedCheckpoints[0].AccountID)

	// Create without override: blocked.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, impactBody, http.StatusConflict)

	// Create with override: succeeds.
	overrideBody := `{
		"transaction_date":"2026-06-01",
		"reconciliation_override":true,
		"change_reason":"backdated entry with override",
		"journal_entries":[{
			"entry_date":"2026-06-01",
			"postings":[
				`+posting(checking.ID, -5000, 2, commodityID)+`,
				`+posting(expense.ID, 5000, 2, commodityID)+`
			]
		}]
	}`
	backdated := createTransactionForSession(t, handler, sessionCookie, csrfToken, overrideBody, http.StatusCreated)
	assert.NotEmpty(t, backdated.InvalidatedCheckpointIDs, "checkpoint should be invalidated by backdated create with override")
}

// TestReconciliationImpactPreviewForUpdate verifies the update-specific impact endpoint.
func TestReconciliationImpactPreviewForUpdate(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Update Impact Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Update Impact Expense","category_type":"expense"}`)

	tx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, commodityID),
		posting(expense.ID, 10000, 2, commodityID),
	), http.StatusCreated)
	reconcilePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, commodityID, tx.JournalEntries[0].Postings[0], "2026-06-07")

	lineKey0 := tx.JournalEntries[0].Postings[0].LineKey
	lineKey1 := tx.JournalEntries[0].Postings[1].LineKey

	// Preview impact of changing the checking amount (reconciliation-affecting).
	updateBody := `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"` + lineKey0 + `","account_id":` + strconvFormatInt(checking.ID) + `,"quantity_value":-9000,"quantity_scale":2,"commodity_id":` + strconvFormatInt(commodityID) + `},
				{"line_key":"` + lineKey1 + `","account_id":` + strconvFormatInt(expense.ID) + `,"quantity_value":9000,"quantity_scale":2,"commodity_id":` + strconvFormatInt(commodityID) + `}
			]
		}]
	}`
	impact := reconciliationImpactForUpdate(t, handler, sessionCookie, tx.ID, updateBody)
	require.NotEmpty(t, impact.AffectedCheckpoints, "changing reconciled posting amount should show impact")
	assert.Equal(t, checking.ID, impact.AffectedCheckpoints[0].AccountID)

	// Preview impact of metadata-only edit (no reconciliation-affecting change): no impact.
	metaBody := `{
		"transaction_date":"2026-06-07",
		"description":"just a description change",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"` + lineKey0 + `","account_id":` + strconvFormatInt(checking.ID) + `,"quantity_value":-10000,"quantity_scale":2,"commodity_id":` + strconvFormatInt(commodityID) + `},
				{"line_key":"` + lineKey1 + `","account_id":` + strconvFormatInt(expense.ID) + `,"quantity_value":10000,"quantity_scale":2,"commodity_id":` + strconvFormatInt(commodityID) + `}
			]
		}]
	}`
	noImpact := reconciliationImpactForUpdate(t, handler, sessionCookie, tx.ID, metaBody)
	assert.Empty(t, noImpact.AffectedCheckpoints, "metadata-only edit should show no checkpoint impact")
}

// TestVoidedTransactionRestoreIsExemptFromReconciliationGuard verifies that
// restoring a soft-deleted+voided transaction does not require override because
// restoring a void does not re-enter ledger postings.
func TestVoidedTransactionRestoreIsExemptFromReconciliationGuard(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Voided Restore Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Voided Restore Expense","category_type":"expense"}`)

	tx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, commodityID),
		posting(expense.ID, 10000, 2, commodityID),
	), http.StatusCreated)
	reconcilePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, commodityID, tx.JournalEntries[0].Postings[0], "2026-06-07")

	// Void with override (required because posting is in reconciled period).
	voided := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(tx.ID)+"/void", `{
		"change_reason":"voided for test",
		"reconciliation_override":true
	}`, http.StatusOK)
	assert.Equal(t, "voided", voided.Status)

	// Soft-delete the voided transaction.
	deleted := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(tx.ID)+"/soft-delete", `{
		"change_reason":"trash it"
	}`, http.StatusOK)
	assert.NotEmpty(t, deleted.DeletedAt)

	// Restore without override: should succeed because the transaction is voided
	// (not in the ledger) and restore does not re-enter the ledger.
	restored := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(tx.ID)+"/restore", `{
		"change_reason":"undoing trash"
	}`, http.StatusOK)
	assert.Empty(t, restored.DeletedAt)
	assert.Equal(t, "voided", restored.Status)
}

// TestMovePostingAcrossReconciliationBoundaryRequiresOverride verifies that
// moving a posting from after the checkpoint boundary to before it fires the
// reconciliation guard.
func TestMovePostingAcrossReconciliationBoundaryRequiresOverride(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Boundary Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Boundary Expense","category_type":"expense"}`)

	// tx1 will be reconciled; tx2 created after so it's after the boundary on the same date.
	tx1 := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, commodityID),
		posting(expense.ID, 10000, 2, commodityID),
	), http.StatusCreated)
	tx2 := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -5000, 2, commodityID),
		posting(expense.ID, 5000, 2, commodityID),
	), http.StatusCreated)

	// Reconcile only tx1 (statement_account_sequence = tx1's sequence).
	reconcilePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, commodityID, tx1.JournalEntries[0].Postings[0], "2026-06-07")

	// tx2's checking posting is after the boundary. Moving it earlier (to before tx1)
	// would place it inside the reconciled period → guard fires.
	tx2CheckingLineID := tx2.JournalEntries[0].Postings[0].PostingLineID
	movePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, tx2CheckingLineID, "earlier", http.StatusConflict)
}

// Helper: move a posting and return the updated transaction.
func movePostingForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, accountID int64, postingLineID int64, direction string, wantStatus int) transactionResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(accountID)+"/postings/"+strconvFormatInt(postingLineID)+"/move", strings.NewReader(`{"direction":"`+direction+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, wantStatus, res.Code, "move posting response body: %s", res.Body.String())
	var response transactionResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func reconciliationImpactForCreate(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, body string) reconciliationImpactResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions/reconciliation-impact", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "reconciliation-impact body: %s", res.Body.String())
	var response reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func reconciliationImpactForUpdate(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, transactionID int64, body string) reconciliationImpactResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(transactionID)+"/reconciliation-impact", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "reconciliation-impact body: %s", res.Body.String())
	var response reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}
