package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionCreateListRegisterVoidAndPayeeSnapshot(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Groceries",
		"category_type":"expense"
	}`)
	payee := createPayeeForSession(t, handler, sessionCookie, csrfToken, `{"name":"Albert Heijn"}`)

	transaction := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"payee_id":`+strconvFormatInt(payee.ID)+`,
		"description":"Weekly groceries",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(groceries.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusCreated)

	assert.Equal(t, "posted", transaction.Status)
	assert.Equal(t, "Albert Heijn", transaction.PayeeName)
	require.Len(t, transaction.JournalEntries, 1)
	require.Len(t, transaction.JournalEntries[0].Postings, 2)
	assert.NotEmpty(t, transaction.JournalEntries[0].Postings[0].LineKey)

	list := listTransactionsForSession(t, handler, sessionCookie, "?q=Albert")
	require.Len(t, list.Transactions, 1)
	assert.Equal(t, transaction.ID, list.Transactions[0].ID)

	register := accountRegisterForSession(t, handler, sessionCookie, checking.ID, "?after_date=2026-06-01&before_date=2026-06-30")
	require.Len(t, register.Entries, 1)
	assert.Equal(t, transaction.ID, register.Entries[0].TransactionID)
	assert.Equal(t, checking.ID, register.Entries[0].Posting.AccountID)

	mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/payees/"+strconvFormatInt(payee.ID)+"/archive", `{"change_reason":"not used anymore"}`, http.StatusOK)
	read := readTransactionForSession(t, handler, sessionCookie, transaction.ID, http.StatusOK)
	assert.Equal(t, "Albert Heijn", read.PayeeName)

	voided := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(transaction.ID)+"/void", `{"change_reason":"duplicate import"}`, http.StatusOK)
	assert.Equal(t, "voided", voided.Status)
	assert.Equal(t, "duplicate import", voided.ChangeReason)

	postedList := listTransactionsForSession(t, handler, sessionCookie, "?status=posted")
	assert.Empty(t, postedList.Transactions)
}

func TestTransactionUnvoidSoftDeleteAndRestoreAreDistinct(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Lifecycle Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Lifecycle Expense","category_type":"expense"}`)
	transaction := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, commodityID),
		posting(expense.ID, 10000, 2, commodityID),
	), http.StatusCreated)

	voided := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(transaction.ID)+"/void", `{"change_reason":"investigating mismatch"}`, http.StatusOK)
	assert.Equal(t, "voided", voided.Status)
	require.Len(t, voided.JournalEntries, 1, "void keeps the posting snapshot for audit and unvoid")
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(transaction.ID), balancedBody("2026-06-07",
		posting(checking.ID, -20000, 2, commodityID),
		posting(expense.ID, 20000, 2, commodityID),
	), http.StatusConflict)

	unvoided := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(transaction.ID)+"/unvoid", `{"change_reason":"entry was valid"}`, http.StatusOK)
	assert.Equal(t, "posted", unvoided.Status)
	require.Len(t, unvoided.JournalEntries, 1)

	deleted := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(transaction.ID)+"/soft-delete", `{"change_reason":"entered by mistake"}`, http.StatusOK)
	assert.Equal(t, "posted", deleted.Status, "soft-delete is independent of lifecycle status")
	assert.NotEmpty(t, deleted.DeletedAt)
	assert.Empty(t, listTransactionsForSession(t, handler, sessionCookie, "").Transactions)
	assert.Empty(t, accountRegisterForSession(t, handler, sessionCookie, checking.ID, "").Entries)
	readTransactionForSession(t, handler, sessionCookie, transaction.ID, http.StatusNotFound)
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(transaction.ID), balancedBody("2026-06-07",
		posting(checking.ID, -20000, 2, commodityID),
		posting(expense.ID, 20000, 2, commodityID),
	), http.StatusNotFound)

	restored := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(transaction.ID)+"/restore", `{"change_reason":"restored from trash"}`, http.StatusOK)
	assert.Empty(t, restored.DeletedAt)
	assert.Equal(t, "posted", restored.Status)
	require.Len(t, listTransactionsForSession(t, handler, sessionCookie, "").Transactions, 1)
	var deletionEventCount int
	require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM transaction_deletion_events WHERE transaction_id = ?", transaction.ID).Scan(&deletionEventCount))
	assert.Equal(t, 2, deletionEventCount)
}

func TestTransactionValidationAndLifecycleGuards(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Other",
		"category_type":"expense"
	}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":9000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-100001,"quantity_scale":3,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":100001,"quantity_scale":3,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	tag := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"Vacation","kind":"project"}`)
	mutateTag(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/tags/"+strconvFormatInt(tag.ID)+"/archive", http.StatusOK)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"tag_ids":[`+strconvFormatInt(tag.ID)+`],
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusConflict)

	draft := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"status":"draft",
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusCreated)
	posted := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(draft.ID)+"/post", `{
		"change_reason":"completed draft"
	}`, http.StatusOK)
	assert.Equal(t, "posted", posted.Status)
	mutateTransactionNoBody(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/transactions/"+strconvFormatInt(posted.ID), http.StatusConflict)

	draft = createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"status":"draft",
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusCreated)
	mutateTransactionNoBody(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/transactions/"+strconvFormatInt(draft.ID), http.StatusNoContent)
	readTransactionForSession(t, handler, sessionCookie, draft.ID, http.StatusNotFound)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`,"reconciliation_status":"reconciled"},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`,"cleared_on":"2026-06-07"},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	base := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusCreated)
	reconciled := reconcilePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, commodityID, base.JournalEntries[0].Postings[0], "2026-06-07")

	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(reconciled.ID), `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+reconciled.JournalEntries[0].Postings[0].LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`,"reconciliation_status":"uncleared"},
				{"line_key":"`+reconciled.JournalEntries[0].Postings[1].LineKey+`","account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(reconciled.ID), `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+reconciled.JournalEntries[0].Postings[0].LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-11000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+reconciled.JournalEntries[0].Postings[1].LineKey+`","account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":11000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusConflict)

	assert.Equal(t, int64(3), countTransactionVersions(t, database, reconciled.ID))

	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(reconciled.ID)+"/correct", `{
		"transaction_date":"2026-06-08",
		"transaction_kind":"adjustment",
		"journal_entries":[{
			"entry_date":"2026-06-08",
			"entry_kind":"adjustment",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`,"reconciliation_status":"reconciled"},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	correction := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(reconciled.ID)+"/correct", `{
		"transaction_date":"2026-06-08",
		"transaction_kind":"adjustment",
		"journal_entries":[{
			"entry_date":"2026-06-08",
			"entry_kind":"adjustment",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusCreated)
	require.NotNil(t, correction.CorrectionOfTransactionID)
	assert.Equal(t, reconciled.ID, *correction.CorrectionOfTransactionID)
}

func TestTransactionTagReplacementSemantics(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Groceries","category_type":"expense"}`)
	otherExpense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Other Expense","category_type":"expense"}`)
	vacation := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"Vacation","kind":"project"}`)
	tax := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"Tax","kind":"flag"}`)
	reimbursable := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"Reimbursable","kind":"flag"}`)

	transaction := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"tag_ids":[`+strconvFormatInt(vacation.ID)+`,`+strconvFormatInt(tax.ID)+`],
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(groceries.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`,"tag_ids":[`+strconvFormatInt(reimbursable.ID)+`]}
			]
		}]
	}`, http.StatusCreated)
	assert.ElementsMatch(t, []int64{vacation.ID, tax.ID}, transaction.TagIDs)
	oldTaggedPosting := transaction.JournalEntries[0].Postings[1]
	assert.ElementsMatch(t, []int64{reimbursable.ID}, oldTaggedPosting.TagIDs)

	updated := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(transaction.ID), `{
		"transaction_date":"2026-06-07",
		"tag_ids":[`+strconvFormatInt(tax.ID)+`],
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+transaction.JournalEntries[0].Postings[0].LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(otherExpense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`,"tag_ids":[`+strconvFormatInt(vacation.ID)+`]}
			]
		}]
	}`, http.StatusOK)
	assert.ElementsMatch(t, []int64{tax.ID}, updated.TagIDs)
	require.Len(t, updated.JournalEntries, 1)
	require.Len(t, updated.JournalEntries[0].Postings, 2)
	assert.Empty(t, updated.JournalEntries[0].Postings[0].TagIDs)
	assert.ElementsMatch(t, []int64{vacation.ID}, updated.JournalEntries[0].Postings[1].TagIDs)
	assert.NotEqual(t, oldTaggedPosting.PostingLineID, updated.JournalEntries[0].Postings[1].PostingLineID)
	assert.Equal(t, 0, countPostingTags(t, database, oldTaggedPosting.PostingLineID))

	voided := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(transaction.ID)+"/void", `{"change_reason":"duplicate"}`, http.StatusOK)
	assert.Equal(t, "voided", voided.Status)
	assert.ElementsMatch(t, []int64{tax.ID}, voided.TagIDs)
	assert.Equal(t, 1, countPostingTags(t, database, updated.JournalEntries[0].Postings[1].PostingLineID))

	repeatedVoid := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(transaction.ID)+"/void", `{"change_reason":"duplicate"}`, http.StatusOK)
	assert.Equal(t, "voided", repeatedVoid.Status)
	assert.ElementsMatch(t, []int64{tax.ID}, repeatedVoid.TagIDs)
}

func TestReconciliationWorkflowAndCheckpointInvalidation(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Reconcile Checking", "asset", "checking", commodityID, 2)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Reconcile Salary","category_type":"income"}`)

	transaction := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"description":"Salary",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":100000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(salary.ID)+`,"quantity_value":-100000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusCreated)
	checkingPosting := transaction.JournalEntries[0].Postings[0]

	clearedTransactions := changePostingReconciliationStatusForSession(t, handler, sessionCookie, csrfToken, checking.ID, `{
		"posting_version_ids":[`+strconvFormatInt(checkingPosting.ID)+`],
		"status":"cleared",
		"cleared_on":"2026-06-07"
	}`, http.StatusOK)
	require.Len(t, clearedTransactions.Transactions, 1)
	cleared := clearedTransactions.Transactions[0]
	require.Len(t, cleared.JournalEntries, 1)
	clearedPosting := cleared.JournalEntries[0].Postings[0]
	assert.Equal(t, "cleared", clearedPosting.ReconciliationStatus)

	started := startReconciliationForSession(t, handler, sessionCookie, csrfToken, checking.ID, `{
		"commodity_id":`+strconvFormatInt(commodityID)+`,
		"source_kind":"statement",
		"statement_date":"2026-06-07",
		"statement_balance_value":100000,
		"statement_balance_scale":2
	}`, http.StatusCreated)
	assert.Equal(t, "open", started.Status)
	require.Len(t, started.Candidates, 1)
	assert.Equal(t, clearedPosting.ID, started.Candidates[0].PostingID)
	require.Len(t, started.SelectedPostings, 1)
	assert.Equal(t, clearedPosting.ID, started.SelectedPostings[0].PostingID)

	preview := previewReconciliationForSession(t, handler, sessionCookie, csrfToken, started.ID, `{
		"posting_version_ids":[`+strconvFormatInt(clearedPosting.ID)+`]
	}`, http.StatusOK)
	assert.Equal(t, "0", preview.Difference.QuantityValue.String())
	assert.Equal(t, "100000", preview.SelectedBalance.QuantityValue.String())

	finished := finishReconciliationForSession(t, handler, sessionCookie, csrfToken, started.ID, `{"change_reason":"matched statement"}`, http.StatusOK)
	assert.Equal(t, "finished", finished.Status)
	require.Len(t, finished.SelectedPostings, 1)
	assert.Equal(t, "reconciled", finished.SelectedPostings[0].ReconciliationStatus)

	checkpoints := listReconciliationCheckpointsForSession(t, handler, sessionCookie, checking.ID, "?commodity_id="+strconvFormatInt(commodityID))
	require.Len(t, checkpoints.Checkpoints, 1)
	assert.Equal(t, "active", checkpoints.Checkpoints[0].Status)

	reconciled := readTransactionForSession(t, handler, sessionCookie, transaction.ID, http.StatusOK)
	metadataEdited := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(transaction.ID), `{
		"transaction_date":"2026-06-07",
		"description":"Salary corrected memo only",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+reconciled.JournalEntries[0].Postings[0].LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":100000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+reconciled.JournalEntries[0].Postings[1].LineKey+`","account_id":`+strconvFormatInt(salary.ID)+`,"quantity_value":-100000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusOK)
	assert.Empty(t, metadataEdited.InvalidatedCheckpointIDs)
	assert.Equal(t, "reconciled", metadataEdited.JournalEntries[0].Postings[0].ReconciliationStatus)

	reconciled = metadataEdited
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(transaction.ID), `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+reconciled.JournalEntries[0].Postings[0].LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":99000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+reconciled.JournalEntries[0].Postings[1].LineKey+`","account_id":`+strconvFormatInt(salary.ID)+`,"quantity_value":-99000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusConflict)

	updated := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(transaction.ID), `{
		"transaction_date":"2026-06-07",
		"change_reason":"corrected reconciled salary",
		"reconciliation_override":true,
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+reconciled.JournalEntries[0].Postings[0].LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":99000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+reconciled.JournalEntries[0].Postings[1].LineKey+`","account_id":`+strconvFormatInt(salary.ID)+`,"quantity_value":-99000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusOK)
	assert.NotEmpty(t, updated.InvalidatedCheckpointIDs)

	checkpoints = listReconciliationCheckpointsForSession(t, handler, sessionCookie, checking.ID, "?commodity_id="+strconvFormatInt(commodityID))
	require.Len(t, checkpoints.Checkpoints, 1)
	assert.Equal(t, "invalidated", checkpoints.Checkpoints[0].Status)
}

func TestVoidReconciliationIdempotencyReturnsCompleteRecords(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Void Reconciliation Checking", "asset", "checking", commodityID, 2)
	income := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Void Reconciliation Income","category_type":"income"}`)

	transaction := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, 10000, 2, commodityID),
		posting(income.ID, -10000, 2, commodityID),
	), http.StatusCreated)
	checkingPosting := transaction.JournalEntries[0].Postings[0]
	clearedTransactions := changePostingReconciliationStatusForSession(t, handler, sessionCookie, csrfToken, checking.ID, `{
		"posting_version_ids":[`+strconvFormatInt(checkingPosting.ID)+`],
		"status":"cleared",
		"cleared_on":"2026-06-07"
	}`, http.StatusOK)
	clearedPosting := postingByLineKey(t, clearedTransactions.Transactions[0], checkingPosting.LineKey)

	session := startReconciliationForSession(t, handler, sessionCookie, csrfToken, checking.ID, `{
		"commodity_id":`+strconvFormatInt(commodityID)+`,
		"source_kind":"statement",
		"statement_date":"2026-06-07",
		"statement_balance_value":10000,
		"statement_balance_scale":2
	}`, http.StatusCreated)
	require.Len(t, session.Candidates, 1)
	require.Len(t, session.SelectedPostings, 1)
	assert.Equal(t, clearedPosting.ID, session.SelectedPostings[0].PostingID)

	voided := voidReconciliationForSession(t, handler, sessionCookie, csrfToken, session.ID, `{"change_reason":"entered in error"}`, http.StatusOK)
	assert.Equal(t, "voided", voided.Status)
	require.Len(t, voided.Candidates, 1)
	require.Len(t, voided.SelectedPostings, 1)
	assert.Equal(t, clearedPosting.ID, voided.SelectedPostings[0].PostingID)

	repeatedVoid := voidReconciliationForSession(t, handler, sessionCookie, csrfToken, session.ID, `{"change_reason":"entered in error"}`, http.StatusOK)
	assert.Equal(t, "voided", repeatedVoid.Status)
	require.Len(t, repeatedVoid.Candidates, 1)
	require.Len(t, repeatedVoid.SelectedPostings, 1)
	assert.Equal(t, clearedPosting.ID, repeatedVoid.SelectedPostings[0].PostingID)
}

func TestVoidReconciliationCheckpointIdempotencyReturnsRecord(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Void Checkpoint Checking", "asset", "checking", commodityID, 2)
	income := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Void Checkpoint Income","category_type":"income"}`)

	transaction := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, 10000, 2, commodityID),
		posting(income.ID, -10000, 2, commodityID),
	), http.StatusCreated)
	reconciled := reconcilePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, commodityID, transaction.JournalEntries[0].Postings[0], "2026-06-07")
	assert.Equal(t, "reconciled", reconciled.JournalEntries[0].Postings[0].ReconciliationStatus)

	checkpoints := listReconciliationCheckpointsForSession(t, handler, sessionCookie, checking.ID, "?commodity_id="+strconvFormatInt(commodityID))
	require.Len(t, checkpoints.Checkpoints, 1)
	checkpoint := checkpoints.Checkpoints[0]
	assert.Equal(t, "active", checkpoint.Status)

	voided := voidReconciliationCheckpointForSession(t, handler, sessionCookie, csrfToken, checkpoint.ID, `{"change_reason":"bad statement"}`, http.StatusOK)
	assert.Equal(t, checkpoint.ID, voided.ID)
	assert.Equal(t, "voided", voided.Status)
	assert.Equal(t, "bad statement", voided.VoidReason)

	repeatedVoid := voidReconciliationCheckpointForSession(t, handler, sessionCookie, csrfToken, checkpoint.ID, `{"change_reason":"bad statement"}`, http.StatusOK)
	assert.Equal(t, checkpoint.ID, repeatedVoid.ID)
	assert.Equal(t, "voided", repeatedVoid.Status)
	assert.Equal(t, "bad statement", repeatedVoid.VoidReason)
}

func TestPostingScopedReconciliationAllowsCategoryEditsAndSplits(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Category Edit Checking", "asset", "checking", commodityID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Category Edit Groceries","category_type":"expense"}`)
	dining := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Category Edit Dining","category_type":"expense"}`)
	household := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Category Edit Household","category_type":"expense"}`)

	transaction := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, commodityID),
		posting(groceries.ID, 10000, 2, commodityID),
	), http.StatusCreated)
	reconciled := reconcilePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, commodityID, transaction.JournalEntries[0].Postings[0], "2026-06-07")
	checkingPosting := reconciled.JournalEntries[0].Postings[0]
	categoryPosting := reconciled.JournalEntries[0].Postings[1]
	require.Equal(t, "reconciled", checkingPosting.ReconciliationStatus)

	categoryEdited := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(reconciled.ID), `{
		"transaction_date":"2026-06-07",
		"description":"category changed only",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+checkingPosting.LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+categoryPosting.LineKey+`","account_id":`+strconvFormatInt(dining.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusOK)
	assert.Empty(t, categoryEdited.InvalidatedCheckpointIDs)
	require.Equal(t, "reconciled", postingByLineKey(t, categoryEdited, checkingPosting.LineKey).ReconciliationStatus)

	categoryPosting = postingByLineKey(t, categoryEdited, categoryPosting.LineKey)
	split := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(categoryEdited.ID), `{
		"transaction_date":"2026-06-07",
		"description":"split category only",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+checkingPosting.LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+categoryPosting.LineKey+`","account_id":`+strconvFormatInt(dining.ID)+`,"quantity_value":6000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(household.ID)+`,"quantity_value":4000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusOK)
	assert.Empty(t, split.InvalidatedCheckpointIDs)
	assert.Equal(t, "reconciled", postingByLineKey(t, split, checkingPosting.LineKey).ReconciliationStatus)
	require.Len(t, split.JournalEntries[0].Postings, 3)

	checkpoints := listReconciliationCheckpointsForSession(t, handler, sessionCookie, checking.ID, "?commodity_id="+strconvFormatInt(commodityID))
	require.Len(t, checkpoints.Checkpoints, 1)
	assert.Equal(t, "active", checkpoints.Checkpoints[0].Status)
}

func TestPostingScopedReconciliationAllowsUnreconciledTransferLegEdit(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Transfer Lock Checking", "asset", "checking", commodityID, 2)
	savings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Transfer Lock Savings", "asset", "savings", commodityID, 2)
	otherSavings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Transfer Lock Other Savings", "asset", "savings", commodityID, 2)

	transaction := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"transfer_leg",
			"postings":[
				`+posting(checking.ID, -10000, 2, commodityID)+`,
				`+posting(savings.ID, 10000, 2, commodityID)+`
			]
		}]
	}`, http.StatusCreated)
	reconciled := reconcilePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, commodityID, transaction.JournalEntries[0].Postings[0], "2026-06-07")
	checkingPosting := reconciled.JournalEntries[0].Postings[0]
	savingsPosting := reconciled.JournalEntries[0].Postings[1]

	unreconciledLegEdited := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(reconciled.ID), `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"transfer_leg",
			"postings":[
				{"line_key":"`+checkingPosting.LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+savingsPosting.LineKey+`","account_id":`+strconvFormatInt(otherSavings.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusOK)
	assert.Empty(t, unreconciledLegEdited.InvalidatedCheckpointIDs)
	assert.Equal(t, "reconciled", postingByLineKey(t, unreconciledLegEdited, checkingPosting.LineKey).ReconciliationStatus)

	checkingPosting = postingByLineKey(t, unreconciledLegEdited, checkingPosting.LineKey)
	savingsPosting = postingByLineKey(t, unreconciledLegEdited, savingsPosting.LineKey)
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(unreconciledLegEdited.ID), `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"transfer_leg",
			"postings":[
				{"line_key":"`+checkingPosting.LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-9000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+savingsPosting.LineKey+`","account_id":`+strconvFormatInt(otherSavings.ID)+`,"quantity_value":9000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusConflict)
}

func TestVoidReconciledTransactionRequiresOverrideAndInvalidatesCheckpoint(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Void Reconciled Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Void Reconciled Expense","category_type":"expense"}`)

	transaction := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, commodityID),
		posting(expense.ID, 10000, 2, commodityID),
	), http.StatusCreated)
	reconciled := reconcilePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, commodityID, transaction.JournalEntries[0].Postings[0], "2026-06-07")

	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(reconciled.ID)+"/void", `{"change_reason":"duplicate statement item"}`, http.StatusConflict)

	voided := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(reconciled.ID)+"/void", `{
		"change_reason":"duplicate statement item",
		"reconciliation_override":true
	}`, http.StatusOK)
	assert.Equal(t, "voided", voided.Status)
	assert.NotEmpty(t, voided.InvalidatedCheckpointIDs)

	checkpoints := listReconciliationCheckpointsForSession(t, handler, sessionCookie, checking.ID, "?commodity_id="+strconvFormatInt(commodityID))
	require.Len(t, checkpoints.Checkpoints, 1)
	assert.Equal(t, "invalidated", checkpoints.Checkpoints[0].Status)
}

func TestUpdateTransactionCannotBypassBalanceValidationViaDraftStatus(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Balance Bypass Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Balance Bypass Expense","category_type":"expense"}`)

	posted := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, commodityID),
		posting(expense.ID, 10000, 2, commodityID),
	), http.StatusCreated)

	// The transaction is already posted, so the update path forces it back to
	// "posted" regardless of what the request body claims — a PATCH claiming
	// status:"draft" must not let balance validation be skipped for an
	// unbalanced posting set that will actually land as posted.
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(posted.ID), `{
		"status":"draft",
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+posted.JournalEntries[0].Postings[0].LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+posted.JournalEntries[0].Postings[1].LineKey+`","account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":9000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	// The rejected update must not have persisted a new version.
	assert.Equal(t, int64(1), countTransactionVersions(t, database, posted.ID))
	unchanged := readTransactionForSession(t, handler, sessionCookie, posted.ID, http.StatusOK)
	assert.Equal(t, "posted", unchanged.Status)
	assert.Equal(t, "-10000", unchanged.JournalEntries[0].Postings[0].QuantityValue.String())
}

func TestPostDraftTransactionIntoReconciledPeriodRequiresOverride(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Draft Promotion Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Draft Promotion Expense","category_type":"expense"}`)

	// Created first, so its checking posting claims the lowest
	// account_day_sequence for this account/date — before any posted
	// transaction exists to reconcile against.
	draft := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"status":"draft",
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-5000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":5000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusCreated)

	// Created and reconciled second: its checking posting takes the next
	// sequence position, and reconciling it sets the checkpoint's lock floor
	// to that (higher) sequence — which covers the draft's earlier position.
	posted := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, commodityID),
		posting(expense.ID, 10000, 2, commodityID),
	), http.StatusCreated)
	reconcilePostingForSession(t, handler, sessionCookie, csrfToken, checking.ID, commodityID, posted.JournalEntries[0].Postings[0], "2026-06-07")

	// Promoting the draft brings its already-fixed, earlier-positioned posting
	// into the ledger for the first time — inside the now-reconciled period —
	// even though promotion changes none of the posting's own fields (the
	// diff-based guard alone would see nothing to flag).
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(draft.ID)+"/post", `{
		"change_reason":"completed draft"
	}`, http.StatusConflict)

	promoted := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(draft.ID)+"/post", `{
		"change_reason":"completed draft",
		"reconciliation_override":true
	}`, http.StatusOK)
	assert.Equal(t, "posted", promoted.Status)
	assert.NotEmpty(t, promoted.InvalidatedCheckpointIDs)
}

func TestTransactionPostingAccountDateAndCommodityValidation(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	eur := createCurrencyForSession(t, handler, sessionCookie, csrfToken, `{"code":"EUR","name":"Euro"}`)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Validation Expense",
		"category_type":"expense"
	}`)
	laterAccount := createLedgerAccountOpenedOn(t, handler, sessionCookie, csrfToken, "Later Checking", "asset", "checking", usdID, 2, "2026-06-06")

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-05",
		"journal_entries":[{
			"entry_date":"2026-06-05",
			"postings":[
				{"account_id":`+strconvFormatInt(laterAccount.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	closedAccount := createLedgerAccountOpenedOn(t, handler, sessionCookie, csrfToken, "Closed Checking", "asset", "checking", usdID, 2, "2026-01-01")
	mutateAccountWithBody(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(closedAccount.ID)+"/close", `{
		"closed_on":"2026-06-06",
		"effective_from":"2026-06-06",
		"change_reason":"closed for test"
	}`, http.StatusOK)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(closedAccount.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	archivedAccount := createLedgerAccountOpenedOn(t, handler, sessionCookie, csrfToken, "Archived Checking", "asset", "checking", usdID, 2, "2026-01-01")
	mutateAccountWithBody(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(archivedAccount.ID)+"/close", `{
		"closed_on":"2026-06-06",
		"change_reason":"closed for test"
	}`, http.StatusOK)
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(archivedAccount.ID)+"/archive", http.StatusOK)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-06",
		"journal_entries":[{
			"entry_date":"2026-06-06",
			"postings":[
				{"account_id":`+strconvFormatInt(archivedAccount.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "USD Checking", "asset", "checking", usdID, 2)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(eur.ID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(eur.ID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(eur.ID)+`}
			]
		}]
	}`, http.StatusBadRequest)
}

func TestTransactionCommonLedgerScenarios(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	eur := createCurrencyForSession(t, handler, sessionCookie, csrfToken, `{"code":"EUR","name":"Euro"}`)
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)
	systemAccounts := listAccountsForSession(t, handler, sessionCookie, "?include_system=true")
	openingBalance := accountBySystemRole(t, systemAccounts.Accounts, "opening_balance")
	transferClearing := accountBySystemRole(t, systemAccounts.Accounts, "transfer_clearing")
	commodityTrading := accountBySystemRole(t, systemAccounts.Accounts, "commodity_trading")

	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", usdID, 2)
	savings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Savings", "asset", "savings", usdID, 2)
	card := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Credit Card", "liability", "credit_card", usdID, 2)
	eurChecking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "EUR Checking", "asset", "checking", eur.ID, 2)
	receivable := createLedgerAccountNoCommodity(t, handler, sessionCookie, csrfToken, "Receivable", "asset", "receivable")
	payable := createLedgerAccountNoCommodity(t, handler, sessionCookie, csrfToken, "Payable", "liability", "payable")
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Salary","category_type":"income"}`)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Scenario Expense","category_type":"expense"}`)

	salaryTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, 100000, 2, usdID),
		posting(salary.ID, -100000, 2, usdID),
	), http.StatusCreated)
	cardPurchaseTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(card.ID, -5000, 2, usdID),
		posting(expense.ID, 5000, 2, usdID),
	), http.StatusCreated)
	cardPaymentTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -5000, 2, usdID),
		posting(card.ID, 5000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, usdID),
		posting(savings.ID, 10000, 2, usdID),
	), http.StatusCreated)
	delayedTransferTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"transfer_leg",
			"postings":[
				`+posting(checking.ID, -10000, 2, usdID)+`,
				`+posting(transferClearing.ID, 10000, 2, usdID)+`
			]
		},{
			"entry_date":"2026-06-08",
			"entry_kind":"transfer_leg",
			"postings":[
				`+posting(transferClearing.ID, -10000, 2, usdID)+`,
				`+posting(savings.ID, 10000, 2, usdID)+`
			]
		}]
	}`, http.StatusCreated)
	fxTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"exchange",
			"postings":[
				`+posting(checking.ID, -108000, 2, usdID)+`,
				`+posting(commodityTrading.ID, 108000, 2, usdID)+`,
				`+posting(eurChecking.ID, 100000, 2, eur.ID)+`,
				`+posting(commodityTrading.ID, -100000, 2, eur.ID)+`
			]
		}]
	}`, http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -8000, 2, usdID),
		posting(receivable.ID, 8000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(expense.ID, 4000, 2, usdID),
		posting(payable.ID, -4000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(receivable.ID, 4000, 2, usdID),
		posting(expense.ID, -4000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"opening_balance",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"opening_balance",
			"postings":[
				`+posting(checking.ID, 50000, 2, usdID)+`,
				`+posting(openingBalance.ID, -50000, 2, usdID)+`
			]
		}]
	}`, http.StatusCreated)

	checkingRegister := accountRegisterForSession(t, handler, sessionCookie, checking.ID, "?after_date=2026-06-07&before_date=2026-06-07")
	assertRegisterPosting(t, checkingRegister, salaryTx.ID, checking.ID, usdID, 100000)
	assertRegisterPosting(t, checkingRegister, cardPaymentTx.ID, checking.ID, usdID, -5000)
	assertRegisterPosting(t, checkingRegister, fxTx.ID, checking.ID, usdID, -108000)

	cardRegister := accountRegisterForSession(t, handler, sessionCookie, card.ID, "?after_date=2026-06-07&before_date=2026-06-07")
	assertRegisterPosting(t, cardRegister, cardPurchaseTx.ID, card.ID, usdID, -5000)
	assertRegisterPosting(t, cardRegister, cardPaymentTx.ID, card.ID, usdID, 5000)

	eurRegister := accountRegisterForSession(t, handler, sessionCookie, eurChecking.ID, "?after_date=2026-06-07&before_date=2026-06-07")
	assertRegisterPosting(t, eurRegister, fxTx.ID, eurChecking.ID, eur.ID, 100000)

	tradingRegister := accountRegisterForSession(t, handler, sessionCookie, commodityTrading.ID, "?after_date=2026-06-07&before_date=2026-06-07")
	assertRegisterPosting(t, tradingRegister, fxTx.ID, commodityTrading.ID, usdID, 108000)
	assertRegisterPosting(t, tradingRegister, fxTx.ID, commodityTrading.ID, eur.ID, -100000)

	outgoingTransferRegister := accountRegisterForSession(t, handler, sessionCookie, checking.ID, "?after_date=2026-06-07&before_date=2026-06-07")
	assertRegisterPosting(t, outgoingTransferRegister, delayedTransferTx.ID, checking.ID, usdID, -10000)

	incomingTransferRegister := accountRegisterForSession(t, handler, sessionCookie, savings.ID, "?after_date=2026-06-08&before_date=2026-06-08")
	assertRegisterPosting(t, incomingTransferRegister, delayedTransferTx.ID, savings.ID, usdID, 10000)

	clearingRegister := accountRegisterForSession(t, handler, sessionCookie, transferClearing.ID, "?after_date=2026-06-07&before_date=2026-06-08")
	require.Len(t, clearingRegister.Entries, 2)
	assertRegisterPosting(t, clearingRegister, delayedTransferTx.ID, transferClearing.ID, usdID, -10000)
	assertRegisterPosting(t, clearingRegister, delayedTransferTx.ID, transferClearing.ID, usdID, 10000)
}

func createLedgerAccount(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, name string, accountClass string, accountKind string, commodityID int64, scaleOverride int) accountResponse {
	t.Helper()

	return createLedgerAccountOpenedOn(t, handler, sessionCookie, csrfToken, name, accountClass, accountKind, commodityID, scaleOverride, "2026-01-01")
}

func createLedgerAccountOpenedOn(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, name string, accountClass string, accountKind string, commodityID int64, scaleOverride int, openedOn string) accountResponse {
	t.Helper()

	return createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"`+name+`",
		"account_class":"`+accountClass+`",
		"account_kind":"`+accountKind+`",
		"default_commodity_id":`+strconvFormatInt(commodityID)+`,
		"quantity_scale_override":`+strconv.Itoa(scaleOverride)+`,
		"allows_postings":true,
		"opened_on":"`+openedOn+`",
		"effective_from":"`+openedOn+`"
	}`)
}

func createLedgerAccountNoCommodity(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, name string, accountClass string, accountKind string) accountResponse {
	t.Helper()

	return createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"`+name+`",
		"account_class":"`+accountClass+`",
		"account_kind":"`+accountKind+`",
		"allows_postings":true,
		"opened_on":"2026-01-01",
		"effective_from":"2026-01-01"
	}`)
}

func createCurrencyForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string) currencyResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/currencies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var response currencyResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func accountBySystemRole(t *testing.T, accounts []accountResponse, role string) accountResponse {
	t.Helper()

	for _, account := range accounts {
		if account.SystemRole == role {
			return account
		}
	}
	require.Failf(t, "system account not found", "role %s", role)
	return accountResponse{}
}

func posting(accountID int64, quantityValue int64, quantityScale int, commodityID int64) string {
	return `{"account_id":` + strconvFormatInt(accountID) + `,"quantity_value":` + strconv.FormatInt(quantityValue, 10) + `,"quantity_scale":` + strconv.Itoa(quantityScale) + `,"commodity_id":` + strconvFormatInt(commodityID) + `}`
}

func balancedBody(transactionDate string, postings ...string) string {
	return `{
		"transaction_date":"` + transactionDate + `",
		"journal_entries":[{
			"entry_date":"` + transactionDate + `",
			"postings":[` + strings.Join(postings, ",") + `]
		}]
	}`
}

func assertRegisterPosting(t *testing.T, register accountRegisterResponse, transactionID int64, accountID int64, commodityID int64, quantityValue int64) {
	t.Helper()

	seen := make([]string, 0)
	for _, entry := range register.Entries {
		seen = append(seen, strconvFormatInt(entry.TransactionID)+":"+entry.EntryDate+":"+strconvFormatInt(entry.Posting.AccountID)+":"+strconvFormatInt(entry.Posting.CommodityID)+":"+entry.Posting.QuantityValue.String())
		if entry.TransactionID != transactionID {
			continue
		}
		posting := entry.Posting
		if posting.AccountID == accountID && posting.CommodityID == commodityID && posting.QuantityValue.String() == strconvFormatInt(quantityValue) {
			return
		}
	}
	require.Failf(t, "register transaction not found", "transaction=%d account=%d commodity=%d quantity=%d seen=%s", transactionID, accountID, commodityID, quantityValue, strings.Join(seen, ","))
}

func countPostingTags(t *testing.T, database *sql.DB, postingLineID int64) int {
	t.Helper()

	var count int
	require.NoError(t, database.QueryRow(`
		SELECT COUNT(*)
		FROM posting_tags
		WHERE posting_line_id = ?
	`, postingLineID).Scan(&count))
	return count
}

func countTransactionVersions(t *testing.T, database *sql.DB, transactionID int64) int64 {
	t.Helper()

	var count int64
	require.NoError(t, database.QueryRow(`
		SELECT COUNT(*)
		FROM transaction_versions
		WHERE transaction_id = ?
	`, transactionID).Scan(&count))
	return count
}

func reconcilePostingForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, accountID int64, commodityID int64, posting postingResponse, statementDate string) transactionResponse {
	t.Helper()

	clearedTransactions := changePostingReconciliationStatusForSession(t, handler, sessionCookie, csrfToken, accountID, `{
		"posting_version_ids":[`+strconvFormatInt(posting.ID)+`],
		"status":"cleared",
		"cleared_on":"`+statementDate+`"
	}`, http.StatusOK)
	require.Len(t, clearedTransactions.Transactions, 1)
	cleared := clearedTransactions.Transactions[0]
	clearedPosting := postingByLineKey(t, cleared, posting.LineKey)
	require.Equal(t, "cleared", clearedPosting.ReconciliationStatus)

	started := startReconciliationForSession(t, handler, sessionCookie, csrfToken, accountID, `{
		"commodity_id":`+strconvFormatInt(commodityID)+`,
		"source_kind":"statement",
		"statement_date":"`+statementDate+`",
		"statement_balance_value":"`+clearedPosting.QuantityValue.String()+`",
		"statement_balance_scale":`+strconv.Itoa(clearedPosting.QuantityScale)+`
	}`, http.StatusCreated)

	finished := finishReconciliationForSession(t, handler, sessionCookie, csrfToken, started.ID, `{"change_reason":"matched statement"}`, http.StatusOK)
	require.Equal(t, "finished", finished.Status)

	return readTransactionForSession(t, handler, sessionCookie, cleared.ID, http.StatusOK)
}

func postingByLineKey(t *testing.T, transaction transactionResponse, lineKey string) postingResponse {
	t.Helper()

	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.LineKey == lineKey {
				return posting
			}
		}
	}
	require.Failf(t, "posting not found", "line_key=%s", lineKey)
	return postingResponse{}
}

func createPayeeForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string) payeeResponse {
	t.Helper()
	return mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/payees", body, http.StatusCreated)
}

func mutatePayee(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body string, wantStatus int) payeeResponse {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response payeeResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func createTransactionForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string, wantStatus int) transactionResponse {
	t.Helper()
	return mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions", body, wantStatus)
}

func mutateTransaction(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body string, wantStatus int) transactionResponse {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equalf(t, wantStatus, res.Code, "response body: %s", res.Body.String())

	var response transactionResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func mutateTransactionNoBody(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, wantStatus int) {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)
}

func readTransactionForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, transactionID int64, wantStatus int) transactionResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/"+strconvFormatInt(transactionID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response transactionResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func listTransactionsForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) transactionsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response transactionsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func accountRegisterForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, accountID int64, suffix string) accountRegisterResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+strconvFormatInt(accountID)+"/register"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response accountRegisterResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func changePostingReconciliationStatusForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, accountID int64, body string, wantStatus int) transactionsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(accountID)+"/postings/reconciliation-status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)
	var response transactionsResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func startReconciliationForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, accountID int64, body string, wantStatus int) reconciliationSessionResponse {
	t.Helper()
	return mutateReconciliationForSession(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(accountID)+"/reconciliations/start", body, wantStatus)
}

func previewReconciliationForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, reconciliationID int64, body string, wantStatus int) reconciliationSessionResponse {
	t.Helper()
	return mutateReconciliationForSession(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/reconciliations/"+strconvFormatInt(reconciliationID)+"/preview", body, wantStatus)
}

func finishReconciliationForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, reconciliationID int64, body string, wantStatus int) reconciliationSessionResponse {
	t.Helper()
	return mutateReconciliationForSession(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/reconciliations/"+strconvFormatInt(reconciliationID)+"/finish", body, wantStatus)
}

func voidReconciliationForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, reconciliationID int64, body string, wantStatus int) reconciliationSessionResponse {
	t.Helper()
	return mutateReconciliationForSession(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/reconciliations/"+strconvFormatInt(reconciliationID)+"/void", body, wantStatus)
}

func mutateReconciliationForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body string, wantStatus int) reconciliationSessionResponse {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)
	var response reconciliationSessionResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func listReconciliationCheckpointsForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, accountID int64, suffix string) reconciliationCheckpointsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+strconvFormatInt(accountID)+"/reconciliation-checkpoints"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var response reconciliationCheckpointsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func voidReconciliationCheckpointForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, checkpointID int64, body string, wantStatus int) reconciliationCheckpointResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/reconciliation-checkpoints/"+strconvFormatInt(checkpointID)+"/void", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)
	var response reconciliationCheckpointResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func TestCategoryIDFilter(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)

	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Groceries","category_type":"expense"}`)
	rent := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Rent","category_type":"expense"}`)

	// Transaction with groceries category
	groceriesTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-10",
		posting(checking.ID, -5000, 2, commodityID),
		posting(groceries.ID, 5000, 2, commodityID),
	), http.StatusCreated)

	// Transaction with rent category
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-10",
		posting(checking.ID, -120000, 2, commodityID),
		posting(rent.ID, 120000, 2, commodityID),
	), http.StatusCreated)

	// Filtering by groceries returns only the groceries transaction
	groceriesList := listTransactionsForSession(t, handler, sessionCookie, "?category_id="+strconvFormatInt(groceries.ID))
	require.Len(t, groceriesList.Transactions, 1)
	assert.Equal(t, groceriesTx.ID, groceriesList.Transactions[0].ID)

	// Filtering by rent returns only the rent transaction
	rentList := listTransactionsForSession(t, handler, sessionCookie, "?category_id="+strconvFormatInt(rent.ID))
	require.Len(t, rentList.Transactions, 1)
	assert.NotEqual(t, groceriesTx.ID, rentList.Transactions[0].ID)

	// category_id AND account_id compose with AND: both filters must match
	andList := listTransactionsForSession(t, handler, sessionCookie, "?category_id="+strconvFormatInt(groceries.ID)+"&account_id="+strconvFormatInt(checking.ID))
	require.Len(t, andList.Transactions, 1)
	assert.Equal(t, groceriesTx.ID, andList.Transactions[0].ID)

	// Passing an asset account as category_id returns VALIDATION_FAILED
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions?category_id="+strconvFormatInt(checking.ID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
	var errBody errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&errBody))
	assert.Equal(t, "VALIDATION_FAILED", errBody.Error.Code)

	// Passing a non-existent account as category_id returns VALIDATION_FAILED
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/transactions?category_id=999999", nil)
	req2.AddCookie(sessionCookie)
	res2 := httptest.NewRecorder()
	handler.ServeHTTP(res2, req2)
	require.Equal(t, http.StatusBadRequest, res2.Code)
	var errBody2 errorResponse
	require.NoError(t, json.NewDecoder(res2.Body).Decode(&errBody2))
	assert.Equal(t, "VALIDATION_FAILED", errBody2.Error.Code)
}

func TestListDeletedTransactionsEndpoint(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Groceries","category_type":"expense"}`)
	rent := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Rent","category_type":"expense"}`)

	// Create two transactions and soft-delete both.
	tx1 := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-10",
		posting(checking.ID, -5000, 2, commodityID),
		posting(groceries.ID, 5000, 2, commodityID),
	), http.StatusCreated)
	tx2 := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-12",
		posting(checking.ID, -120000, 2, commodityID),
		posting(rent.ID, 120000, 2, commodityID),
	), http.StatusCreated)

	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(tx1.ID)+"/soft-delete",
		`{"change_reason":"entered by mistake"}`, http.StatusOK)
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(tx2.ID)+"/soft-delete",
		`{"change_reason":"duplicate"}`, http.StatusOK)

	// Deleted list should return both transactions.
	deleted := listDeletedTransactionsForSession(t, handler, sessionCookie, "")
	require.Len(t, deleted.Transactions, 2)

	// Most-recently-deleted first (tx2 was deleted last).
	assert.Equal(t, tx2.ID, deleted.Transactions[0].ID)
	assert.Equal(t, tx1.ID, deleted.Transactions[1].ID)

	// Delete reason is captured.
	assert.Equal(t, "duplicate", deleted.Transactions[0].DeleteReason)
	assert.Equal(t, "entered by mistake", deleted.Transactions[1].DeleteReason)

	// Posted transactions are NOT restore-blocked unless there's a reconciliation checkpoint.
	assert.False(t, deleted.Transactions[0].RestoreBlockedByReconciliation)
	assert.False(t, deleted.Transactions[1].RestoreBlockedByReconciliation)

	// Enriched posting fields present.
	require.NotEmpty(t, deleted.Transactions[0].JournalEntries)
	firstPosting := deleted.Transactions[0].JournalEntries[0].Postings[0]
	assert.NotEmpty(t, firstPosting.AccountClass)
	assert.NotEmpty(t, firstPosting.CommodityCode)

	// Ordinary list returns zero (all deleted).
	live := listTransactionsForSession(t, handler, sessionCookie, "")
	assert.Empty(t, live.Transactions)

	// Restore one and confirm it disappears from deleted list.
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(tx1.ID)+"/restore",
		`{"change_reason":"restored"}`, http.StatusOK)

	deletedAfterRestore := listDeletedTransactionsForSession(t, handler, sessionCookie, "")
	require.Len(t, deletedAfterRestore.Transactions, 1)
	assert.Equal(t, tx2.ID, deletedAfterRestore.Transactions[0].ID)
}

func TestListDeletedTransactionsCursorPagination(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Expense","category_type":"expense"}`)

	// Create 3 transactions and soft-delete them all.
	ids := make([]int64, 3)
	for i := range ids {
		tx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-0"+strconv.Itoa(i+1),
			posting(checking.ID, -1000, 2, commodityID),
			posting(expense.ID, 1000, 2, commodityID),
		), http.StatusCreated)
		ids[i] = tx.ID
		mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost,
			"/api/v1/transactions/"+strconvFormatInt(tx.ID)+"/soft-delete",
			`{"change_reason":"cleanup"}`, http.StatusOK)
	}

	// Fetch first page of 2.
	page1 := listDeletedTransactionsForSession(t, handler, sessionCookie, "?limit=2")
	require.Len(t, page1.Transactions, 2)
	require.NotNil(t, page1.NextCursor, "first page should have a next cursor")

	// Fetch second page using cursor.
	page2 := listDeletedTransactionsForSession(t, handler, sessionCookie, "?limit=2&cursor="+*page1.NextCursor)
	require.Len(t, page2.Transactions, 1)
	assert.Nil(t, page2.NextCursor, "last page should have no next cursor")

	// All 3 IDs are present across both pages (no duplicates, no gaps).
	allIDs := make([]int64, 0, 3)
	for _, tx := range page1.Transactions {
		allIDs = append(allIDs, tx.ID)
	}
	for _, tx := range page2.Transactions {
		allIDs = append(allIDs, tx.ID)
	}
	require.Len(t, allIDs, 3)

	// No duplicates.
	seen := make(map[int64]bool)
	for _, id := range allIDs {
		assert.False(t, seen[id], "duplicate id %d across pages", id)
		seen[id] = true
	}
}

func TestListDeletedTransactionsVoidedRowNotRestoreBlocked(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Expense","category_type":"expense"}`)

	tx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-01",
		posting(checking.ID, -5000, 2, commodityID),
		posting(expense.ID, 5000, 2, commodityID),
	), http.StatusCreated)

	// Void then soft-delete.
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(tx.ID)+"/void",
		`{"change_reason":"test void"}`, http.StatusOK)
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(tx.ID)+"/soft-delete",
		`{"change_reason":"cleanup"}`, http.StatusOK)

	deleted := listDeletedTransactionsForSession(t, handler, sessionCookie, "")
	require.Len(t, deleted.Transactions, 1)
	assert.Equal(t, "voided", deleted.Transactions[0].Status)
	// Voided+soft-deleted rows are never restore-blocked.
	assert.False(t, deleted.Transactions[0].RestoreBlockedByReconciliation)
}

// listDeletedTransactionsForSession is a test helper that calls GET /api/v1/transactions/deleted.
func listDeletedTransactionsForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) deletedTransactionsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/deleted"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response deletedTransactionsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

// --- Posting date vs. account/commodity availability ---

// A user creates an account today and enters last month's transactions. The
// as-of version lookup finds no account version effective on that date, and
// used to report "posting account is invalid" — which reads as "your account
// is broken" rather than "your date is too early". That misdiagnosis is what
// made a wall-clock-dependent e2e failure expensive to trace.
func TestCreateTransaction_BeforeAccountOpenedReportsTheDateNotTheAccount(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccountOpenedOn(t, handler, sessionCookie, csrfToken, "Opened Later", "asset", "checking", commodityID, 2, "2026-06-01")
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Late Groceries","category_type":"expense"}`)

	res := createTransactionResponseForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-05-01",
		"description":"Before the account existed",
		"journal_entries":[{"entry_date":"2026-05-01","postings":[
			{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":"-1000","quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
			{"account_id":`+strconvFormatInt(groceries.ID)+`,"quantity_value":"1000","quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
		]}]
	}`, http.StatusBadRequest)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
	assert.Equal(t, "posting date is before account opened date", body.Error.Message,
		"the account exists; only the date is wrong, and the message must say which")
}

// The account genuinely does not exist — this must keep the original message,
// because there is no date the user could pick that would make it work.
func TestCreateTransaction_UnknownAccountStillReportsAnInvalidAccount(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Unknown Groceries","category_type":"expense"}`)

	res := createTransactionResponseForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-01",
		"description":"No such account",
		"journal_entries":[{"entry_date":"2026-06-01","postings":[
			{"account_id":999999,"quantity_value":"-1000","quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
			{"account_id":`+strconvFormatInt(groceries.ID)+`,"quantity_value":"1000","quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
		]}]
	}`, http.StatusBadRequest)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "posting account is invalid", body.Error.Message)
}

// A currency's enable date is app bookkeeping, not a financial fact, so it
// must not constrain the ledger: history predating setup has to post. This is
// the migration story — install today, import last year (T-42).
func TestCreateTransaction_HistoryPredatingCurrencySetupIsAccepted(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	// The fixture clock enables the currency on 2026-01-01; the account really
	// opened in 2025. Before T-42 the commodity as-of lookup rejected this
	// with "posting commodity is invalid".
	checking := createLedgerAccountOpenedOn(t, handler, sessionCookie, csrfToken, "Early Cash", "asset", "checking", commodityID, 2, "2025-01-01")
	// Categories default to opening today too, but unlike currencies they
	// accept an explicit date, so backdate this one to keep the test about
	// the commodity rule.
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Early Groceries",
		"category_type":"expense",
		"opened_on":"2025-01-01",
		"effective_from":"2025-01-01"
	}`)

	res := createTransactionResponseForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2025-06-01",
		"description":"History from before setup",
		"journal_entries":[{"entry_date":"2025-06-01","postings":[
			{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":"-1000","quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
			{"account_id":`+strconvFormatInt(groceries.ID)+`,"quantity_value":"1000","quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
		]}]
	}`, http.StatusCreated)

	var created transactionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	assert.Equal(t, "2025-06-01", created.TransactionDate)
	require.Len(t, created.JournalEntries, 1)
	assert.Len(t, created.JournalEntries[0].Postings, 2)
}

// createTransactionResponseForSession returns the raw recorder so a test can
// assert on the error envelope, not just the status.
func createTransactionResponseForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equalf(t, wantStatus, res.Code, "response body: %s", res.Body.String())
	return res
}
