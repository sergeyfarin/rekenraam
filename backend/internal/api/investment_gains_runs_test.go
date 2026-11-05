package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #234's named acceptance: a valuation at D remains reproducible after a
// later reversal of a sale dated before D, while both live projections restate.
func TestInvestmentGainsRunReproducesBeforeSaleReversal(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "FROZEN")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 4000), http.StatusCreated)
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 5000)
	sale.TransactionDate = "2026-03-01"
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)
	var sold investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&sold))
	saved := []db.GainsRunRecord{}
	for _, request := range []app.GainsRunRequest{
		{ReportType: "gains", From: "2026-03-01", To: "2026-03-31", ValuationAsOf: "2026-03-31"},
		{ReportType: "statement", From: "2026-03-01", To: "2026-03-31"},
	} {
		res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/gains/runs", request, http.StatusCreated)
		var run db.GainsRunRecord
		require.NoError(t, json.NewDecoder(res.Body).Decode(&run))
		require.True(t, db.ValidGainsRunDocument(run), string(run.InputsJSON))
		assert.Equal(t, "/api/v1/investments/gains/runs/"+strconv.FormatInt(run.ID, 10), res.Header().Get("Location"))
		saved = append(saved, run)
	}
	var initial investmentGainsResponse
	require.NoError(t, json.Unmarshal(saved[0].OutputJSON, &initial))
	require.Len(t, initial.Realized, 1)
	assertMoneyValue(t, 3000, 2, *initial.Realized[0].RealizedGainValue, *initial.Realized[0].RealizedGainScale)
	var statement investmentGainsStatementResponse
	require.NoError(t, json.Unmarshal(saved[1].OutputJSON, &statement))
	require.Len(t, statement.Rows, 1)
	assertMoneyValue(t, 3000, 2, *statement.Rows[0].RealizedGainValue, *statement.Rows[0].RealizedGainScale)

	path := "/api/v1/investments/transactions/" + strconv.FormatInt(sold.Transaction.ID, 10) + "/reverse-sale"
	request := investmentSaleReversalRequest{Reason: "broker canceled fill after report"}
	preview := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/reconciliation-impact", request, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(preview.Body).Decode(&impact))
	request.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)

	for _, run := range saved {
		res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains/runs/"+strconv.FormatInt(run.ID, 10), nil, http.StatusOK)
		var retrieved db.GainsRunRecord
		require.NoError(t, json.NewDecoder(res.Body).Decode(&retrieved))
		assert.Equal(t, run, retrieved, "saved document and provenance never change")
	}
	live := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains?from=2026-03-01&to=2026-03-31&valuation_as_of=2026-03-31", nil, http.StatusOK)
	var liveGains investmentGainsResponse
	require.NoError(t, json.NewDecoder(live.Body).Decode(&liveGains))
	assert.Empty(t, liveGains.Realized)
	require.Len(t, liveGains.Unrealized, 1)
	assert.Equal(t, "2", liveGains.Unrealized[0].QuantityValue.String())
	live = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains/statement?from=2026-03-01&to=2026-03-31", nil, http.StatusOK)
	var liveStatement investmentGainsStatementResponse
	require.NoError(t, json.NewDecoder(live.Body).Decode(&liveStatement))
	require.Len(t, liveStatement.Rows, 1)
	assert.Zero(t, liveStatement.Rows[0].DisposalCount)
	assertMoneyValue(t, 0, 0, *liveStatement.Rows[0].RealizedGainValue, *liveStatement.Rows[0].RealizedGainScale)

	// The portable archive includes complete documents even with a ledger scope
	// outside their period. Backup/restore also preserves their exact bytes.
	bundle := downloadBundle(t, handler, f.sessionCookie, "?from=2026-04-01&to=2026-04-30")
	assert.Equal(t, 19, bundle.manifest.SchemaVersion)
	table := bundle.table(t, "investment-gains-runs.csv")
	require.Len(t, table.rows, 2)
	for i, row := range table.rows {
		assert.Equal(t, string(saved[i].InputsJSON), table.column(row, "inputs_json"))
		assert.Equal(t, string(saved[i].OutputJSON), table.column(row, "output_json"))
		assert.Equal(t, saved[i].OutputSHA256, table.column(row, "output_sha256"))
	}
	ctx := context.Background()
	backup := filepath.Join(t.TempDir(), "restored.sqlite")
	require.NoError(t, db.BackupSQLiteDatabase(ctx, database, backup))
	restored, err := db.Open(ctx, db.SQLiteFileURL(backup))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, restored.Close()) })
	reader := app.NewInvestmentService(db.NewInvestmentRepository(restored), nil, nil, nil)
	for _, run := range saved {
		record, err := reader.GainsRun(ctx, run.ID)
		require.NoError(t, err)
		// The wire omits BookID; restore reads the original single-book identity.
		record.BookID = 0
		assert.Equal(t, run, record)
	}
	readOnly, err := db.OpenReadOnly(ctx, db.SQLiteFileURL(backup))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })
	self := app.NewSelfCheckService(db.NewSelfCheckRepository(restored, readOnly))
	check, err := self.RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	assert.Equal(t, app.SelfCheckPassed, check.Status)

	// SQLite refuses ordinary mutation. Simulated out-of-band corruption is
	// diagnosed without recalculating the saved figures against corrected history.
	_, err = restored.ExecContext(ctx, `UPDATE investment_gains_runs SET output_json = '{}' WHERE id = ?`, saved[0].ID)
	require.ErrorContains(t, err, "immutable")
	_, err = restored.ExecContext(ctx, `DELETE FROM investment_gains_runs WHERE id = ?`, saved[0].ID)
	require.ErrorContains(t, err, "immutable")
	_, err = restored.ExecContext(ctx, `DROP TRIGGER investment_gains_runs_no_update`)
	require.NoError(t, err)
	_, err = restored.ExecContext(ctx, `UPDATE investment_gains_runs SET output_json = '{}' WHERE id = ?`, saved[0].ID)
	require.NoError(t, err)
	_, err = reader.GainsRun(ctx, saved[0].ID)
	require.ErrorIs(t, err, app.ErrGainsRunIntegrity)
	check, err = self.RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	found := false
	for _, result := range check.Results {
		if result.CheckID == app.CheckInvestmentFoundation {
			found = true
			assert.Equal(t, app.SelfCheckFailed, result.Status)
			assert.Contains(t, result.Summary, "saved gains runs failed document integrity")
			assert.Contains(t, result.Sample, saved[0].ID)
		}
	}
	require.True(t, found)
}

func TestInvestmentGainsRunValidationAndReadOnlyRetrieval(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	base := "/api/v1/investments/gains/runs"
	doInvestmentRequest(t, handler, nil, "", http.MethodGet, base, nil, http.StatusUnauthorized)
	doInvestmentRequest(t, handler, nil, "", http.MethodPost, base, app.GainsRunRequest{ReportType: "gains"}, http.StatusUnauthorized)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base, app.GainsRunRequest{ReportType: "gains"}, http.StatusForbidden)
	for _, input := range []any{
		app.GainsRunRequest{}, app.GainsRunRequest{ReportType: "tax"},
		app.GainsRunRequest{ReportType: "gains", From: "2026-02-30"},
		app.GainsRunRequest{ReportType: "gains", From: "2026-04-01", To: "2026-03-01"},
		app.GainsRunRequest{ReportType: "gains", ValuationAsOf: "2099-01-01"},
		app.GainsRunRequest{ReportType: "statement"},
		app.GainsRunRequest{ReportType: "statement", From: "2026-03-01", To: "2026-03-31", ValuationAsOf: "2026-03-31"},
		map[string]any{"report_type": "gains", "output": map[string]any{}},
		map[string]any{"report_type": "gains", "knowledge_cutoff": "2026-01-01T00:00:00Z"},
	} {
		doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, input, http.StatusBadRequest)
	}
	var count int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM investment_gains_runs`).Scan(&count))
	assert.Zero(t, count)
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'investment.gains_run.create'`).Scan(&count))
	assert.Zero(t, count)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/99999", nil, http.StatusNotFound)
	for _, suffix := range []string{"/0", "/bad", "?cursor=0", "?cursor=", "?cursor=bogus", "?cursor=1&cursor=2"} {
		doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+suffix, nil, http.StatusBadRequest)
	}
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, app.GainsRunRequest{ReportType: "gains"}, http.StatusCreated)
	var saved db.GainsRunRecord
	require.NoError(t, json.NewDecoder(res.Body).Decode(&saved))
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&count))
	before := count
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base, nil, http.StatusOK)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/"+strconv.FormatInt(saved.ID, 10), nil, http.StatusOK)
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&count))
	assert.Equal(t, before, count, "GETs never create report runs or audits")
}

func TestInvestmentGainsRunPreservesWithdrawnPrice(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "RUNPRICE")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	// A trade supplies an approximate fallback; the saved run retains the manual
	// price even after withdrawal, rather than silently selecting that fallback.
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 4000), http.StatusCreated)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{
		BaseCommodityID: instrument.CommodityID, QuoteCommodityID: f.commodityID, QuoteType: "manual", AdjustmentBasis: "raw",
		PriceValue: 9000, PriceScale: 2, ValuationDate: "2026-03-31", IsManual: true,
		Derivation: json.RawMessage(`{}`), SeriesMetadata: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`)}, http.StatusCreated)
	var price priceObservationResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&price))
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/gains/runs", app.GainsRunRequest{ReportType: "gains", ValuationAsOf: "2026-03-31"}, http.StatusCreated)
	var saved db.GainsRunRecord
	require.NoError(t, json.NewDecoder(res.Body).Decode(&saved))
	var frozen investmentGainsResponse
	require.NoError(t, json.Unmarshal(saved.OutputJSON, &frozen))
	require.Len(t, frozen.Unrealized, 1)
	assert.Equal(t, moneyCoefficient(9000), *frozen.Unrealized[0].LatestPriceValue)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices/"+strconv.FormatInt(price.ID, 10)+"/void", priceObservationVoidRequest{VoidReason: "withdrawn after report"}, http.StatusOK)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains/runs/"+strconv.FormatInt(saved.ID, 10), nil, http.StatusOK)
	var restored db.GainsRunRecord
	require.NoError(t, json.NewDecoder(res.Body).Decode(&restored))
	assert.Equal(t, saved, restored)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains?valuation_as_of=2026-03-31", nil, http.StatusOK)
	var live investmentGainsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&live))
	require.Len(t, live.Unrealized, 1)
	assert.NotEqual(t, *frozen.Unrealized[0].LatestPriceValue, *live.Unrealized[0].LatestPriceValue)

	// A boundary before any acquisition remains a known zero.
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/gains/runs", app.GainsRunRequest{ReportType: "statement", From: "2026-01-01", To: "2026-03-31"}, http.StatusCreated)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&restored))
	var statement investmentGainsStatementResponse
	require.NoError(t, json.Unmarshal(restored.OutputJSON, &statement))
	require.Len(t, statement.Rows, 1)
	assert.NotNil(t, statement.Rows[0].Closing.UnrealizedGainValue)
	assert.Zero(t, statement.Rows[0].Opening.PositionCount)
	assertMoneyValue(t, 0, 0, *statement.Rows[0].Opening.UnrealizedGainValue, *statement.Rows[0].Opening.UnrealizedGainScale)
}

func TestInvestmentGainsRunCursorPagination(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	base := "/api/v1/investments/gains/runs"
	ids := []int64{}
	for range 21 {
		res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base, app.GainsRunRequest{ReportType: "gains"}, http.StatusCreated)
		var run db.GainsRunRecord
		require.NoError(t, json.NewDecoder(res.Body).Decode(&run))
		ids = append(ids, run.ID)
	}
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base, nil, http.StatusOK)
	var page struct {
		Runs       []db.GainsRunRecord `json:"runs"`
		NextCursor *string             `json:"next_cursor"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&page))
	require.Len(t, page.Runs, 20)
	require.NotNil(t, page.NextCursor)
	for i, run := range page.Runs {
		assert.Equal(t, ids[20-i], run.ID)
		assert.Empty(t, run.OutputJSON, "large output is retrieved only on selection")
	}
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"?cursor="+*page.NextCursor, nil, http.StatusOK)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&page))
	require.Len(t, page.Runs, 1)
	assert.Equal(t, ids[0], page.Runs[0].ID)
	assert.Nil(t, page.NextCursor)
}

func TestInvestmentGainsRunRetainsUnavailableFiguresAndCapturedCurrencyMetadata(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "RUNUNKNOWN")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/transfers/external/in",
		externalTransferInRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID, BasisKnowledge: "unknown",
			CommodityID: instrument.CommodityID, QuantityValue: exact.New(2), CostCommodityID: f.commodityID}, http.StatusCreated)
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 6000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/gains/runs",
		app.GainsRunRequest{ReportType: "statement", From: "2026-03-01", To: "2026-03-31"}, http.StatusCreated)
	var run db.GainsRunRecord
	require.NoError(t, json.NewDecoder(res.Body).Decode(&run))
	var statement investmentGainsStatementResponse
	require.NoError(t, json.Unmarshal(run.OutputJSON, &statement))
	require.Len(t, statement.Rows, 1)
	row := statement.Rows[0]
	assert.Nil(t, row.Opening.UnrealizedGainValue)
	require.NotNil(t, row.Opening.UnavailableReason)
	assert.Equal(t, "unpriced_and_unresolved", *row.Opening.UnavailableReason)
	assert.Equal(t, 1, row.Opening.UnpricedCount)
	assert.Equal(t, 1, row.Opening.UnresolvedCount)
	assert.Nil(t, row.Closing.UnrealizedGainValue)
	assert.Nil(t, row.RealizedGainValue)
	assert.Nil(t, row.UnrealizedChangeValue)
	assert.Nil(t, row.TotalValue)
	// Even later display-metadata changes cannot alter saved currency precision.
	// Appending a version models the independent reference-data family;
	// the run reads no current reference tables during retrieval.
	_, err := database.Exec(`INSERT INTO commodity_versions
 (commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason, status,
 symbol, display_symbol, name, standard_scale, max_quantity_scale, metadata_json, change_audit_event_id)
 SELECT commodity_id, version_seq + 1, effective_from, recorded_at, changed_by_user_id, 'later metadata fixture', status,
 symbol, display_symbol, 'Later name', 3, max_quantity_scale, metadata_json, change_audit_event_id
 FROM `+db.CurrentReferenceVersions(context.Background(), "commodity")+` WHERE commodity_id = ?`, f.commodityID)
	require.NoError(t, err)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains/runs/"+strconv.FormatInt(run.ID, 10)+"?download=true", nil, http.StatusOK)
	assert.Contains(t, res.Header().Get("Content-Disposition"), "rekenraam-gains-run-")
	var reopened db.GainsRunRecord
	require.NoError(t, json.NewDecoder(res.Body).Decode(&reopened))
	assert.Equal(t, run, reopened)
}
