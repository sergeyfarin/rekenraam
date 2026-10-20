package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"rekenraam/backend/internal/exact"

	sqlite "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// assertMutationPostingsMatchRead checks the real wire representation against
// the canonical read, independently of the shared service/DTO converter. It
// leaves the recorder body intact for the journey's financial assertions.
func assertMutationPostingsMatchRead(t *testing.T, handler http.Handler, cookie *http.Cookie, response *httptest.ResponseRecorder) {
	t.Helper()
	var document any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
	var inspect func(any)
	inspect = func(value any) {
		switch value := value.(type) {
		case []any:
			for _, child := range value {
				inspect(child)
			}
		case map[string]any:
			if entries, ok := value["journal_entries"].([]any); ok && value["id"] != nil {
				id, ok := value["id"].(float64)
				if !ok || id == 0 {
					return
				} // journal-free result is not a posted transaction
				req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/"+strconv.FormatInt(int64(id), 10), nil)
				req.AddCookie(cookie)
				read := httptest.NewRecorder()
				handler.ServeHTTP(read, req)
				require.Equal(t, http.StatusOK, read.Code, read.Body.String())
				var canonical struct {
					Entries []struct {
						Postings []map[string]any `json:"postings"`
					} `json:"journal_entries"`
				}
				require.NoError(t, json.Unmarshal(read.Body.Bytes(), &canonical))
				expected := map[any]map[string]any{}
				for _, entry := range canonical.Entries {
					for _, posting := range entry.Postings {
						expected[posting["id"]] = posting
					}
				}
				for _, entry := range entries {
					for _, row := range entry.(map[string]any)["postings"].([]any) {
						posting := row.(map[string]any)
						wanted, ok := expected[posting["id"]]
						require.True(t, ok, "mutation posting must exist in canonical journal")
						require.Contains(t, []string{"asset", "liability", "equity", "income", "expense"}, posting["account_class"])
						require.NotEmpty(t, posting["commodity_code"])
						for _, field := range []string{"account_id", "commodity_id", "quantity_value", "quantity_scale", "tag_ids", "account_name", "account_code", "account_system_role", "account_builtin_key", "account_class", "commodity_code", "commodity_symbol"} {
							require.Contains(t, posting, field)
							require.Equal(t, wanted[field], posting[field], "transaction %d posting %v field %s", int64(id), posting["id"], field)
						}
						require.NotNil(t, posting["tag_ids"], "empty tags remain arrays")
					}
				}
				return
			}
			for _, child := range value {
				inspect(child)
			}
		}
	}
	inspect(document)
}

func TestMutationResponseEnrichmentBuyMatchesCanonicalRead(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "ENRICH")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	response := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 4000), http.StatusCreated)
	var body investmentTradeResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Transaction.JournalEntries, 1)
	require.Len(t, body.Transaction.JournalEntries[0].Postings, 4)
	for _, posting := range body.Transaction.JournalEntries[0].Postings {
		require.Contains(t, []string{"USD", "ENRICH"}, posting.CommodityCode)
	}
	assertMutationPostingsMatchRead(t, handler, f.sessionCookie, response)
}

func TestMutationResponseEnrichmentReconciliationBatchMatchesCanonicalReads(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, commodity := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, cookie, csrf, "Enriched checking", "asset", "checking", commodity, 2)
	expense := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Enriched expense","category_type":"expense"}`)
	ids := []int64{}
	for range 2 {
		transaction := createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-06-07", posting(checking.ID, -123, 2, commodity), posting(expense.ID, 123, 2, commodity)), http.StatusCreated)
		ids = append(ids, transaction.JournalEntries[0].Postings[0].ID)
	}
	response := doInvestmentRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(checking.ID)+"/postings/reconciliation-status", map[string]any{"posting_version_ids": ids, "status": "cleared", "cleared_on": "2026-06-07"}, http.StatusOK)
	var body transactionsResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Transactions, 2)
	assertMutationPostingsMatchRead(t, handler, cookie, response)
}

// denyEnrichmentAfterCommit denies only reads after the real writer reaches
// COMMIT. Preparation/financial writes retain production SQLite behavior.
func denyEnrichmentAfterCommit(t *testing.T, database *sql.DB, target, column string) func() {
	t.Helper()
	var committed atomic.Bool
	conn, err := database.Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, conn.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, table, field, source string) int {
			if operation == sqlite.SQLITE_TRANSACTION && table == "COMMIT" {
				committed.Store(true)
			}
			if committed.Load() && operation == sqlite.SQLITE_READ && table == target && field == column {
				return sqlite.SQLITE_DENY
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, conn.Close())
	reset := func() {
		conn, err := database.Conn(context.Background())
		require.NoError(t, err)
		require.NoError(t, conn.Raw(func(raw any) error { raw.(*sqlite.SQLiteConn).RegisterAuthorizer(nil); return nil }))
		require.NoError(t, conn.Close())
	}
	t.Cleanup(reset)
	return reset
}

func TestMutationResponseEnrichmentFailurePreservesCommittedBuyWithoutRetry(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, target, column string }{{"account", "account_kinds", "base_kind"}, {"commodity", "commodities", "code"}} {
		t.Run(test.name, func(t *testing.T) {
			handler, database := newSetupTestHandler(t)
			f := bootstrapInvestmentAPITest(t, handler)
			instrument := createInstrumentForSession(t, handler, f, "FAILENRICH")
			holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
			reset := denyEnrichmentAfterCommit(t, database, test.target, test.column)
			response := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 4000), http.StatusInternalServerError)
			require.JSONEq(t, `{"error":{"code":"INTERNAL_ERROR","message":"internal server error"}}`, response.Body.String())
			reset()
			var count int
			require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM transactions").Scan(&count))
			require.Equal(t, 1, count)
			require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM investment_lots").Scan(&count))
			require.Equal(t, 1, count)
			transaction := readTransactionForSession(t, handler, f.sessionCookie, 1, http.StatusOK)
			require.NotEmpty(t, transaction.JournalEntries[0].Postings[0].AccountClass)
			require.NotEmpty(t, transaction.JournalEntries[0].Postings[0].CommodityCode)
		})
	}
}

func TestMutationResponseEnrichmentFailurePreservesCompoundCorrection(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "PAIRFAIL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	response := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 4000), http.StatusCreated)
	var original investmentTradeResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &original))
	reset := denyEnrichmentAfterCommit(t, database, "commodities", "code")
	request := investmentBuyReplacementRequest{Reason: "correct acquisition", Replacement: tradeRequestBody(f, holding.ID, instrument.CommodityID, "3", 6000)}
	response = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/transactions/"+strconvFormatInt(original.Transaction.ID)+"/replace-buy", request, http.StatusInternalServerError)
	require.JSONEq(t, `{"error":{"code":"INTERNAL_ERROR","message":"internal server error"}}`, response.Body.String())
	reset()
	var count int
	require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM transactions").Scan(&count))
	require.Equal(t, 3, count, "source, inverse and replacement committed once")
	require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM audit_events WHERE operation = 'investment.buy.replace'").Scan(&count))
	require.Equal(t, 1, count)
	var remaining string
	require.NoError(t, database.QueryRow("SELECT remaining_quantity_value FROM investment_lot_state WHERE lot_id = 2").Scan(&remaining))
	require.Equal(t, "3", remaining)
	for _, id := range []int64{2, 3} {
		transaction := readTransactionForSession(t, handler, f.sessionCookie, id, http.StatusOK)
		require.NotEmpty(t, transaction.JournalEntries[0].Postings[0].AccountClass)
		require.NotEmpty(t, transaction.JournalEntries[0].Postings[0].CommodityCode)
	}
}

func TestMutationResponseEnrichmentExternalOutReplacementAndReversal(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "OUTENRICH")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	response := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "3", 6000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &bought))
	out := externalTransferOutRequest{EffectiveOn: "2026-03-01", SourceAccountID: holding.ID, CommodityID: instrument.CommodityID, CostCommodityID: f.commodityID, LotAllocations: []investmentLotAllocationRequest{{LotID: *bought.LotID, QuantityValue: exact.New(1)}}}
	response = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/transfers/external/out", out, http.StatusCreated)
	var original externalTransferOutResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &original))
	out.LotAllocations[0].QuantityValue = exact.New(2)
	response = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/transactions/"+strconvFormatInt(original.Transaction.ID)+"/replace-transfer-out", investmentTransferOutReplacementRequest{Reason: "correct quantity", Replacement: out}, http.StatusCreated)
	var corrected struct {
		Replacement externalTransferOutResponse `json:"replacement"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &corrected))
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/transactions/"+strconvFormatInt(corrected.Replacement.Transaction.ID)+"/reverse-transfer", investmentSaleReversalRequest{Reason: "transfer canceled"}, http.StatusCreated)
}
