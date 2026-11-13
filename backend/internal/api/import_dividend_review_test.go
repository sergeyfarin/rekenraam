package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
)

func seedAPIDividendRow(t *testing.T, database *sql.DB, source string) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	repository := db.NewImportRepository(database)
	batch, err := repository.CreateImportBatch(ctx, db.CreateImportBatchParams{BookID: app.BookID, SourceKind: "trading212", SourceMetaJSON: `{"fetch_status":"ready"}`, CreatedAt: "2026-03-02T00:00:00Z", ActorUserID: 1})
	require.NoError(t, err)
	require.NoError(t, repository.InsertImportStagedRows(ctx, []db.CreateImportStagedRowParams{{BatchID: batch.ID, BookID: app.BookID, RowIndex: 0, DedupeFingerprint: fmt.Sprintf("dividend-api-%d", batch.ID), RawJSON: `{"kind":"trading212_dividend","ticker":"APIPAYER","isin":""}`, NormalizedJSON: source, DedupeStatus: "new", ResolutionJSON: "{}"}}))
	rows, err := repository.ListAllImportStagedRows(ctx, batch.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	return batch.ID, rows[0].ID
}

func TestImportDividendReviewAPIAccountsEconomicsAuthAndExport(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "APIPAYER")
	batch, rowID := seedAPIDividendRow(t, database, `{"date":"2026-03-01","amount":"85.00","commodity_hint":"USD","memo":"API dividend","external_ref":"api-div"}`)
	base := fmt.Sprintf("/api/v1/imports/%d/rows/%d/review-dividend", batch, rowID)
	attentionResult := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/attention", nil, http.StatusOK)
	var attention attentionResponse
	require.NoError(t, json.NewDecoder(attentionResult.Body).Decode(&attention))
	for _, source := range attention.Sources {
		if source.ID == "imports" {
			require.NotNil(t, source.DividendReviewCount)
			require.Equal(t, 1, *source.DividendReviewCount)
			require.Equal(t, 1, *source.Count)
		}
	}

	doInvestmentRequest(t, handler, nil, "", http.MethodGet, "/api/v1/import-dividends", nil, http.StatusUnauthorized)
	read := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/import-dividends?limit=1", nil, http.StatusOK)
	var reviews app.ImportDividendReviews
	require.NoError(t, json.NewDecoder(read.Body).Decode(&reviews))
	require.Len(t, reviews.Rows, 1)
	require.Nil(t, reviews.Rows[0].Resolution.GrossAmount)
	require.Equal(t, "USD", reviews.Rows[0].SourceCurrency)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base, map[string]any{"resolution": map[string]any{}}, http.StatusForbidden)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, map[string]any{"resolution": map[string]any{}}, http.StatusBadRequest)
	tax := createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "Tax", "expense", "expense", f.commodityID, 2)
	gross, taxAmount := "100.00", "15.00"
	resolution := app.ImportRowResolution{AccountID: f.cashAccount.ID, CommodityID: f.commodityID, InstrumentCommodityID: instrument.CommodityID, CategoryID: &f.incomeAccount.ID, GrossAmount: &gross, WithholdingAmount: &taxAmount, WithholdingAccountID: &tax.ID}
	request := reviewImportDividendRequest{Resolution: resolution}
	bad := request
	bad.ExpectedTransactionVersionID = 999999
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/reconciliation-impact", bad, http.StatusConflict)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, bad, http.StatusConflict)
	bad = request
	wrongGross := "99.00"
	bad.Resolution.GrossAmount = &wrongGross
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, bad, http.StatusBadRequest)
	bad = request
	bad.Resolution.InstrumentCommodityID = 999999
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, bad, http.StatusBadRequest)
	bad = request
	hugeGross, hugeTax := "9223372036854775808", "9223372036854775723"
	bad.Resolution.GrossAmount, bad.Resolution.WithholdingAmount = &hugeGross, &hugeTax
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, bad, http.StatusUnprocessableEntity)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/reconciliation-impact", request, http.StatusOK)
	recorded := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, request, http.StatusOK)
	var transaction transactionResponse
	require.NoError(t, json.NewDecoder(recorded.Body).Decode(&transaction))
	require.Equal(t, "investment", transaction.TransactionKind)
	attentionResult = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/attention", nil, http.StatusOK)
	require.NoError(t, json.NewDecoder(attentionResult.Body).Decode(&attention))
	for _, source := range attention.Sources {
		if source.ID == "imports" {
			require.NotNil(t, source.DividendReviewCount)
			require.Zero(t, *source.DividendReviewCount)
			require.Zero(t, *source.Count)
		}
	}

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, request, http.StatusConflict)
	activity := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/transactions?commodity_id="+strconv.FormatInt(instrument.CommodityID, 10), nil, http.StatusOK)
	var listed struct {
		Transactions []transactionResponse `json:"transactions"`
	}
	require.NoError(t, json.NewDecoder(activity.Body).Decode(&listed))
	require.Len(t, listed.Transactions, 1)
	bundle := downloadBundle(t, handler, f.sessionCookie, "")
	require.Len(t, bundle.table(t, "investment-dividend-facts.csv").rows, 1)
}

func TestImportDividendRepairAPIKeepsSamePaymentAndRejectsStaleVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "APIPAYER")
	batch, rowID := seedAPIDividendRow(t, database, `{"date":"2026-03-01","amount":"85.00","commodity_hint":"USD","memo":"Legacy API dividend","external_ref":"legacy"}`)
	legacy := createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, fmt.Sprintf(`{"transaction_date":"2026-03-01","description":"Legacy API dividend","note_markdown":"Owner note","journal_entries":[{"entry_date":"2026-03-01","entry_kind":"ordinary","postings":[{"account_id":%d,"commodity_id":%d,"quantity_value":"8500","quantity_scale":2},{"account_id":%d,"commodity_id":%d,"quantity_value":"-8500","quantity_scale":2}]}]}`, f.cashAccount.ID, f.commodityID, f.incomeAccount.ID, f.commodityID), http.StatusCreated)
	repository := db.NewImportRepository(database)
	row, err := repository.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)
	_, err = repository.CommitImportedTransaction(ctx, db.CommitImportedTransactionParams{Identity: db.CreateImportCommitIdentityParams{BookID: app.BookID, DedupeFingerprint: row.DedupeFingerprint, SourceKind: "trading212", AccountID: f.cashAccount.ID, CreatedAt: "2026-03-02T00:00:00Z"}, Row: db.CommitImportStagedRowParams{RowID: rowID, CommitStatus: "committed"}}, func(*sql.Tx) (int64, error) { return legacy.ID, nil })
	require.NoError(t, err)
	base := fmt.Sprintf("/api/v1/imports/%d/rows/%d/review-dividend", batch, rowID)
	gross, tax := "85.00", "0"
	request := reviewImportDividendRequest{Resolution: app.ImportRowResolution{AccountID: f.cashAccount.ID, CommodityID: f.commodityID, InstrumentCommodityID: instrument.CommodityID, CategoryID: &f.incomeAccount.ID, GrossAmount: &gross, WithholdingAmount: &tax}, Reason: "Statement payer declaration", ExpectedTransactionVersionID: legacy.VersionID - 1}
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, request, http.StatusConflict)
	request.ExpectedTransactionVersionID = legacy.VersionID
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/reconciliation-impact", request, http.StatusOK)
	repaired := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, request, http.StatusOK)
	var transaction transactionResponse
	require.NoError(t, json.NewDecoder(repaired.Body).Decode(&transaction))
	require.Equal(t, legacy.ID, transaction.ID)
	require.Equal(t, legacy.VersionSeq+1, transaction.VersionSeq)
	require.Equal(t, "Owner note", transaction.NoteMarkdown)
	var count int
	require.NoError(t, database.QueryRow("SELECT count(*) FROM transactions").Scan(&count))
	require.Equal(t, 1, count)
	bundle := downloadBundle(t, handler, f.sessionCookie, "")
	require.Len(t, bundle.table(t, "investment-dividend-facts.csv").rows, 1)
}
