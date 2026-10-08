package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerdb "rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// --- Bootstrap ---

type investmentAPITestFixture struct {
	sessionCookie *http.Cookie
	csrfToken     string
	commodityID   int64
	cashAccount   accountResponse
	incomeAccount accountResponse
}

func bootstrapInvestmentAPITest(t *testing.T, handler http.Handler) investmentAPITestFixture {
	t.Helper()
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)

	// Buy/Sell require the commodity_trading system account, created by the
	// setup wizard's system-accounts step (see TestSystemAccountSetupCreatesRolesAndProtectsThem).
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/system-accounts", nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusCreated, res.Code, res.Body.String())

	cashAccount := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	incomeAccount := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Dividend Income", "income", "income", commodityID, 2)
	return investmentAPITestFixture{
		sessionCookie: sessionCookie, csrfToken: csrfToken, commodityID: commodityID,
		cashAccount: cashAccount, incomeAccount: incomeAccount,
	}
}

// --- Request helpers ---

func doInvestmentRequest(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body any, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = strings.NewReader(string(encoded))
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrfToken != "" {
		req.Header.Set(csrfTokenHeader, csrfToken)
		setSameOrigin(req)
	}
	if sessionCookie != nil {
		req.AddCookie(sessionCookie)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())
	return res
}

func createInstrumentForSession(t *testing.T, handler http.Handler, f investmentAPITestFixture, symbol string) investmentInstrumentResponse {
	t.Helper()
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/instruments", investmentInstrumentRequest{
		CommodityCode: symbol, InstrumentType: "etf", DisplayName: "Test Instrument " + symbol, Symbol: symbol,
		QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-01-01",
		Identifiers: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var response investmentInstrumentResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func createHoldingAccountForSession(t *testing.T, handler http.Handler, f investmentAPITestFixture, instrumentID int64) accountResponse {
	t.Helper()
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/holding-accounts", holdingAccountRequest{
		InstrumentID: instrumentID, Name: "Holding", OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01",
	}, http.StatusCreated)
	var response accountResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func TestExternalTransferInAPIRequiresKnownBasisAndPostsOneLot(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "XFER")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	path := "/api/v1/investments/transfers/external/in"
	request := externalTransferInRequest{
		EffectiveOn: "2026-05-01", HoldingAccountID: holding.ID, CommodityID: instrument.CommodityID,
		QuantityValue: exact.New(2), CarriedBasisScale: 2, CostCommodityID: f.commodityID,
		OriginalAcquiredOn: "2020-01-01", SourceEvidence: json.RawMessage(`{"statement":"42"}`),
	}
	missing := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusBadRequest)
	assert.Contains(t, missing.Body.String(), "known carried basis is required")
	zero := moneyCoefficient(0)
	request.CarriedBasisValue = &zero
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/reconciliation-impact", request, http.StatusOK)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, request, http.StatusForbidden)
	result := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	var response externalTransferInResponse
	require.NoError(t, json.NewDecoder(result.Body).Decode(&response))
	assert.Positive(t, response.LotID)
	assert.Equal(t, "posted", response.Transaction.Status)
	var count int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_lot_links WHERE destination_lot_id = ? AND carried_basis_value = '0'`, response.LotID).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestInternalTransferAPIRequiresCSRFAndPreviewsBothAccounts(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "IXFER")
	source := createHoldingAccountForSession(t, handler, f, instrument.ID)
	destination := createHoldingAccountForSession(t, handler, f, instrument.ID)
	path := "/api/v1/investments/transfers/internal"
	buySource := tradeRequestBody(f, source.ID, instrument.CommodityID, "2", 20000)
	buySource.TransactionDate = "2026-01-01"
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", buySource, http.StatusCreated)
	var boughtSource investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&boughtSource))
	require.NotNil(t, boughtSource.LotID)
	buyDest := tradeRequestBody(f, destination.ID, instrument.CommodityID, "1", 10000)
	buyDest.TransactionDate = "2026-01-01"
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", buyDest, http.StatusCreated)
	var boughtDest investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&boughtDest))
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, source.ID,
		instrument.CommodityID, postingByAccount(t, boughtSource.Transaction, source.ID), "2026-03-01")
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, destination.ID,
		instrument.CommodityID, postingByAccount(t, boughtDest.Transaction, destination.ID), "2026-03-01")
	request := internalTransferRequest{
		EffectiveOn: "2026-02-01", SourceAccountID: source.ID,
		DestinationAccountID: destination.ID, CommodityID: instrument.CommodityID,
		CostCommodityID: f.commodityID,
		LotAllocations: []investmentLotAllocationRequest{{LotID: *boughtSource.LotID,
			QuantityValue: exact.New(1), QuantityScale: 0}},
		SourceEvidence: json.RawMessage(`{"statement":"42"}`),
	}
	preview := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path+"/reconciliation-impact", request, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(preview.Body).Decode(&impact))
	require.Len(t, impact.AffectedCheckpoints, 2)
	refused := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path, request, http.StatusConflict)
	assert.Contains(t, refused.Body.String(), "reconciliation override")
	request.ReconciliationOverride = true
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, request, http.StatusForbidden)
	posted := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path, request, http.StatusCreated)
	var transfer internalTransferResponse
	require.NoError(t, json.NewDecoder(posted.Body).Decode(&transfer))
	require.Len(t, transfer.DestinationLotIDs, 1)
	require.Len(t, transfer.Transaction.JournalEntries, 1)
	require.Len(t, transfer.Transaction.JournalEntries[0].Postings, 2)
	var linkCount int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_lot_links
		WHERE source_lot_id = ? AND destination_lot_id = ?`, *boughtSource.LotID,
		transfer.DestinationLotIDs[0]).Scan(&linkCount))
	assert.Equal(t, 1, linkCount)
}

func TestInternalTransferAllocationConflictsHaveSpecificCodes(t *testing.T) {
	for err, code := range map[error]string{
		ledgerdb.ErrAverageCostTransferRequiresPoolAllocation: "INVESTMENT_TRANSFER_POOL_REQUIRED",
		ledgerdb.ErrPooledTransferRequiresAverageCost:         "INVESTMENT_TRANSFER_POOL_UNAVAILABLE",
	} {
		recorder := httptest.NewRecorder()
		writeInvestmentServiceError(recorder, nil, nil, "internal transfer", err)
		require.Equal(t, http.StatusConflict, recorder.Code)
		var response errorResponse
		require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
		assert.Equal(t, code, response.Error.Code)
	}
}

func TestPooledInternalTransferAPIPreviewMatchesCommit(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "IPOOL")
	source := createHoldingAccountForSession(t, handler, f, instrument.ID)
	destination := createHoldingAccountForSession(t, handler, f, instrument.ID)
	for _, buy := range []struct {
		date, quantity string
		cash           int64
	}{{"2026-01-01", "1", 10000}, {"2026-01-02", "1", 20000}} {
		request := tradeRequestBody(f, source.ID, instrument.CommodityID, buy.quantity, buy.cash)
		request.TransactionDate = buy.date
		doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
			"/api/v1/investments/buy", request, http.StatusCreated)
	}
	sale := tradeRequestBody(f, source.ID, instrument.CommodityID, "1", 30000)
	sale.TransactionDate = "2026-01-03"
	sale.CostBasisMethod = "average_cost"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/sell", sale, http.StatusCreated)
	request := internalTransferRequest{
		EffectiveOn: "2026-02-01", SourceAccountID: source.ID,
		DestinationAccountID: destination.ID, CommodityID: instrument.CommodityID,
		CostCommodityID: f.commodityID, QuantityValue: exact.New(1),
	}
	path := "/api/v1/investments/transfers/internal"
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/preview", request, http.StatusOK)
	var preview internalTransferPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	require.Equal(t, "average_cost_pool", preview.Plan.BasisAllocation)
	require.Equal(t, "position_lock", preview.Plan.ResolutionTier)
	require.Equal(t, "pooled_lot", preview.Plan.DestinationLineage, "the default for a pooled quantity (T-135)")
	require.Len(t, preview.Plan.Links, 1)
	assert.Nil(t, preview.Plan.Links[0].DestinationLotID)
	assert.Nil(t, preview.Plan.Links[0].SourceLotID, "a pooled lot is carried from the pool")
	require.NotNil(t, preview.Plan.Links[0].OriginalAcquiredOn)
	assert.Equal(t, "2026-01-02", *preview.Plan.Links[0].OriginalAcquiredOn, "the latest unit moved")
	assert.Empty(t, preview.Impact.AffectedCheckpoints)
	// The opt-in lineage is echoed with the source lot it carries.
	sourceLots := request
	sourceLots.DestinationLineage = "source_lots"
	var sourceLotsPreview internalTransferPreviewResponse
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/preview", sourceLots, http.StatusOK)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&sourceLotsPreview))
	assert.Equal(t, "source_lots", sourceLotsPreview.Plan.DestinationLineage)
	require.NotNil(t, sourceLotsPreview.Plan.Links[0].SourceLotID)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	var committed internalTransferResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&committed))
	require.Len(t, committed.Plan.Links, 1)
	require.NotNil(t, committed.Plan.Links[0].DestinationLotID)
	assert.Equal(t, committed.DestinationLotIDs[0], *committed.Plan.Links[0].DestinationLotID)
	// The pool after an average-cost sale is 150.00 for one share.
	assert.Equal(t, preview.Plan.Links[0].CarriedBasisValue, committed.Plan.Links[0].CarriedBasisValue)
	assert.Equal(t, preview.Plan.Links[0].CarriedBasisScale, committed.Plan.Links[0].CarriedBasisScale)
	assert.Zero(t, exact.ScaledIntFromInt64(int64(*committed.Plan.Links[0].CarriedBasisValue),
		*committed.Plan.Links[0].CarriedBasisScale).Cmp(exact.ScaledIntFromInt64(15000, 2)))
	// A pooled lot needs a pooled quantity.
	request.DestinationLineage = "pooled_lot"
	// The destination has no average-cost lock or default, so its moved lot
	// goes back as a selected lot.
	request.QuantityValue = ""
	request.LotAllocations = []investmentLotAllocationRequest{{LotID: committed.DestinationLotIDs[0], QuantityValue: exact.New(1)}}
	request.SourceAccountID, request.DestinationAccountID = destination.ID, source.ID
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/preview", request, http.StatusBadRequest)
	request.DestinationLineage = ""
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/preview", request, http.StatusOK)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	assert.Equal(t, "selected_lots", preview.Plan.BasisAllocation)
	assert.Equal(t, "source_lots", preview.Plan.DestinationLineage)
}

func TestExternalTransferInPreviewNamesCheckpointAndWriteRequiresOverride(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "XFR2")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 10000)
	buy.TransactionDate = "2026-01-01"
	response := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", buy, http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&bought))
	posting := postingByAccount(t, bought.Transaction, holding.ID)
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken,
		holding.ID, instrument.CommodityID, posting, "2026-02-01")
	checkpointID := activeCheckpointID(t, handler, f.sessionCookie, holding.ID)

	basis := moneyCoefficient(2500)
	request := externalTransferInRequest{
		EffectiveOn: "2026-01-15", HoldingAccountID: holding.ID, CommodityID: instrument.CommodityID,
		QuantityValue: exact.New(1), CarriedBasisValue: &basis, CarriedBasisScale: 2,
		CostCommodityID: f.commodityID,
	}
	path := "/api/v1/investments/transfers/external/in"
	preview := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path+"/reconciliation-impact", request, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(preview.Body).Decode(&impact))
	require.Len(t, impact.AffectedCheckpoints, 1)
	assert.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	refused := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path, request, http.StatusConflict)
	assert.Contains(t, refused.Body.String(), "reconciliation override")
	request.ReconciliationOverride = true
	created := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path, request, http.StatusCreated)
	var transfer externalTransferInResponse
	require.NoError(t, json.NewDecoder(created.Body).Decode(&transfer))
	assert.Contains(t, transfer.Transaction.InvalidatedCheckpointIDs, checkpointID)
}

func tradeRequestBody(f investmentAPITestFixture, holdingAccountID, commodityID int64, quantity string, cashValue int64) investmentTradeRequest {
	qty, err := exact.Parse(quantity)
	if err != nil {
		panic(err)
	}
	return investmentTradeRequest{
		TransactionDate: "2026-02-01", CommodityID: commodityID, HoldingAccountID: holdingAccountID,
		CashAccountID: f.cashAccount.ID, QuantityValue: qty, QuantityScale: 0,
		CashAmountValue: moneyCoefficient(cashValue), CashAmountScale: 2, CashCommodityID: f.commodityID,
	}
}

func TestExactTradeRequestAndPreviewUseSignedGrossChargesAndNet(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "EXACT")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buyGross, buyNet := moneyCoefficient(-10000), moneyCoefficient(-10200)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 0)
	buy.GrossAmountValue, buy.GrossAmountScale = &buyGross, 2
	buy.NetSettlementValue, buy.NetSettlementScale = &buyNet, 2
	buy.Charges = []investmentTradeChargeRequest{{Kind: "commission", AmountValue: -200, AmountScale: 2, CommodityID: f.commodityID}}
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusCreated)
	sellGross, sellNet := moneyCoefficient(12000), moneyCoefficient(11800)
	sell := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 0)
	sell.TransactionDate = "2026-03-01"
	sell.GrossAmountValue, sell.GrossAmountScale = &sellGross, 2
	sell.NetSettlementValue, sell.NetSettlementScale = &sellNet, 2
	sell.Charges = buy.Charges
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/sell/preview", sell, http.StatusOK)
	var preview sellPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	require.Equal(t, moneyCoefficient(12000), *preview.GrossAmountValue)
	require.Equal(t, moneyCoefficient(11800), preview.NetSettlementValue)
	require.Equal(t, moneyCoefficient(11800), preview.DisposalDecision.ProceedsValue)
	assertMoneyValue(t, 1600, 2, *preview.RealizedGain, *preview.RealizedGainScale)
	require.Len(t, preview.Charges, 1)
}

func TestReverseManualSaleAPI(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "REV")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000), http.StatusCreated)
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 24000)
	sale.TransactionDate = "2026-03-01"
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/sell", sale, http.StatusCreated)
	var sold investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&sold))
	path := "/api/v1/investments/transactions/" + strconv.FormatInt(sold.Transaction.ID, 10) + "/reverse-sale"
	chainPath := "/api/v1/investments/transactions/" + strconv.FormatInt(sold.Transaction.ID, 10) + "/correction-chain"
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		"/api/v1/investments/transactions/999999/correction-chain", nil, http.StatusNotFound)
	chainRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, chainPath, nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(chainRes.Body).Decode(&chain))
	require.Len(t, chain.Operations, 1)
	require.True(t, chain.CanReverseManualSale)
	require.True(t, chain.CanReverseSale)
	request := investmentSaleReversalRequest{Reason: "broker canceled fill"}
	previewRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/reconciliation-impact", request, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(previewRes.Body).Decode(&impact))
	require.NotNil(t, impact.GainImpact)
	require.Len(t, impact.GainImpact.Changes, 1)
	require.Equal(t, "removed", impact.GainImpact.Changes[0].ChangeKind)
	require.Nil(t, impact.GainImpact.Changes[0].After)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, request, http.StatusForbidden)
	required := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusConflict)
	require.Contains(t, required.Body.String(), "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED")
	request.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	var reversed investmentSaleReversalResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&reversed))
	require.Equal(t, sold.Transaction.ID, reversed.CorrectedTransactionID)
	require.Equal(t, "posted", reversed.Transaction.Status)
	chainRes = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, chainPath, nil, http.StatusOK)
	require.NoError(t, json.NewDecoder(chainRes.Body).Decode(&chain))
	require.Len(t, chain.Operations, 2)
	require.Nil(t, chain.EffectiveTransactionID)
	require.False(t, chain.CanReverseManualSale)
	var remaining int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM current_investment_lots WHERE book_id = 1 AND remaining_quantity_value = '5'`).Scan(&remaining))
	require.Equal(t, 1, remaining)
	conflict := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusConflict)
	require.Contains(t, conflict.Body.String(), "INVESTMENT_SALE_ALREADY_CORRECTED")
}

func TestReverseManualBuyAPI(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "REVB")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&bought))
	path := "/api/v1/investments/transactions/" + strconv.FormatInt(bought.Transaction.ID, 10) + "/reverse-buy"
	chainPath := "/api/v1/investments/transactions/" + strconv.FormatInt(bought.Transaction.ID, 10) + "/correction-chain"
	chainRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, chainPath, nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(chainRes.Body).Decode(&chain))
	require.True(t, chain.CanReverseManualBuy)
	require.True(t, chain.CanReverseBuy)
	request := investmentSaleReversalRequest{Reason: "duplicate acquisition"}
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/reconciliation-impact", request, http.StatusOK)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, request, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	var reversed investmentSaleReversalResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&reversed))
	require.Equal(t, bought.Transaction.ID, reversed.CorrectedTransactionID)
	require.Equal(t, "posted", reversed.Transaction.Status)
	chainRes = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, chainPath, nil, http.StatusOK)
	require.NoError(t, json.NewDecoder(chainRes.Body).Decode(&chain))
	require.Nil(t, chain.EffectiveTransactionID)
	require.False(t, chain.CanReverseManualBuy)
	conflict := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusConflict)
	require.Contains(t, conflict.Body.String(), "INVESTMENT_BUY_ALREADY_CORRECTED")
}

func TestReverseManualBuyAPIDependentSaleConflict(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "REVD")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&bought))
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 12000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/sell", sale, http.StatusCreated)
	path := "/api/v1/investments/transactions/" + strconv.FormatInt(bought.Transaction.ID, 10) + "/reverse-buy"
	request := investmentSaleReversalRequest{Reason: "duplicate acquisition"}
	for _, endpoint := range []string{path + "/reconciliation-impact", path} {
		t.Run(endpoint, func(t *testing.T) {
			csrf := f.csrfToken
			if endpoint != path {
				csrf = ""
			}
			conflict := doInvestmentRequest(t, handler, f.sessionCookie, csrf, http.MethodPost,
				endpoint, request, http.StatusConflict)
			require.Contains(t, conflict.Body.String(), "INVESTMENT_BUY_DEPENDENCY")
		})
	}
}

func TestReplaceLatestManualSaleAPI(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "REPL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000), http.StatusCreated)
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 24000)
	sale.TransactionDate = "2026-03-01"
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/sell", sale, http.StatusCreated)
	var sold investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&sold))
	laterSale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 13000)
	laterSale.TransactionDate = "2026-04-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/sell", laterSale, http.StatusCreated)
	contextResult := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		"/api/v1/investments/transactions/"+strconv.FormatInt(sold.Transaction.ID, 10)+"/trade-correction-context",
		nil, http.StatusOK)
	var source investmentTradeCorrectionContextResponse
	require.NoError(t, json.NewDecoder(contextResult.Body).Decode(&source))
	require.True(t, source.CanReplaceSale)
	require.Zero(t, source.SourceIdentityID)
	require.Empty(t, source.SourceKind)
	require.Len(t, source.AvailableLots, 1)
	require.Equal(t, "5", source.AvailableLots[0].QuantityValue)
	path := "/api/v1/investments/transactions/" + strconv.FormatInt(sold.Transaction.ID, 10) + "/replace-sale"
	replacement := tradeRequestBody(f, holding.ID, instrument.CommodityID, "3", 39000)
	replacement.TransactionDate = sale.TransactionDate
	replacement.CostBasisMethod = "fifo"
	request := investmentSaleReplacementRequest{Reason: "corrected fill", Replacement: replacement}
	impossible := request
	impossible.Replacement.QuantityValue = "5"
	conflictingPreview := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path+"/reconciliation-impact", impossible, http.StatusConflict)
	require.Contains(t, conflictingPreview.Body.String(), "INVESTMENT_SALE_DEPENDENCY")
	preview := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path+"/reconciliation-impact", request, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(preview.Body).Decode(&impact))
	require.NotNil(t, impact.GainImpact)
	kinds := map[string]int{}
	for _, change := range impact.GainImpact.Changes {
		kinds[change.ChangeKind]++
	}
	require.Equal(t, map[string]int{"replaced": 1}, kinds, "a single lot keeps the later sale's per-share basis")
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, request, http.StatusForbidden)
	stale := request
	stale.GainImpactAcknowledgement = "stale"
	staleRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, stale, http.StatusConflict)
	require.Contains(t, staleRes.Body.String(), "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_STALE")
	request.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	var corrected investmentSaleReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&corrected))
	require.Equal(t, sold.Transaction.ID, corrected.CorrectedTransactionID)
	require.Equal(t, "posted", corrected.InverseTransaction.Status)
	require.Equal(t, "posted", corrected.Replacement.Transaction.Status)
	var remaining int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM current_investment_lots
		WHERE book_id = 1 AND remaining_quantity_value = '1'`).Scan(&remaining))
	require.Equal(t, 1, remaining)
	conflict := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusConflict)
	require.Contains(t, conflict.Body.String(), "INVESTMENT_SALE_ALREADY_CORRECTED")
}

func TestReplaceOldManualBuyAPI(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "BUYREPL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", buy, http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&bought))
	contextPath := "/api/v1/investments/transactions/" + strconv.FormatInt(bought.Transaction.ID, 10) + "/trade-correction-context"
	doInvestmentRequest(t, handler, nil, "", http.MethodGet, contextPath, nil, http.StatusUnauthorized)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		"/api/v1/investments/transactions/999999/trade-correction-context", nil, http.StatusNotFound)
	contextResult := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		contextPath, nil, http.StatusOK)
	var source investmentTradeCorrectionContextResponse
	require.NoError(t, json.NewDecoder(contextResult.Body).Decode(&source))
	require.Equal(t, "buy", source.OperationKind)
	require.Equal(t, "-50000", source.NetValue)
	require.Equal(t, "5", source.QuantityValue)
	require.Empty(t, source.Charges)
	require.False(t, source.AlreadyCorrected)
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 24000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/sell", sale, http.StatusCreated)
	path := "/api/v1/investments/transactions/" + strconv.FormatInt(bought.Transaction.ID, 10) + "/replace-buy"
	replacement := tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 100000)
	request := investmentBuyReplacementRequest{Reason: "corrected purchase cost", Replacement: replacement}
	preview := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path+"/reconciliation-impact", request, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(preview.Body).Decode(&impact))
	require.NotNil(t, impact.GainImpact)
	require.Len(t, impact.GainImpact.Changes, 1)
	assertGainCoefficient(t, 20000, 2, impact.GainImpact.Changes[0].Before.DisposedBasisValue, impact.GainImpact.Changes[0].Before.DisposedBasisScale)
	assertGainCoefficient(t, 40000, 2, impact.GainImpact.Changes[0].After.DisposedBasisValue, impact.GainImpact.Changes[0].After.DisposedBasisScale)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, request, http.StatusForbidden)
	required := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusConflict)
	require.Contains(t, required.Body.String(), "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED")
	request.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path, request, http.StatusCreated)
	var corrected investmentBuyReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&corrected))
	require.Equal(t, bought.Transaction.ID, corrected.CorrectedTransactionID)
	require.Equal(t, "posted", corrected.InverseTransaction.Status)
	require.Equal(t, "posted", corrected.Replacement.Transaction.Status)
	var activeLotCount int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM current_investment_lots
		WHERE book_id = 1 AND account_id = ? AND remaining_quantity_value = '3'`, holding.ID).Scan(&activeLotCount))
	require.Equal(t, 1, activeLotCount)
	contextResult = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		contextPath, nil, http.StatusOK)
	require.NoError(t, json.NewDecoder(contextResult.Body).Decode(&source))
	require.True(t, source.AlreadyCorrected)
	conflict := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path, request, http.StatusConflict)
	require.Contains(t, conflict.Body.String(), "INVESTMENT_BUY_ALREADY_CORRECTED")
}

// T-116: the replacement may move the buy's date and holding account. A move
// past the dependent sale names it; a move to another account replays both.
func TestReplaceBuyDateAndHoldingAccountAPI(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "MOVE")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	other := createHoldingAccountForSession(t, handler, f, instrument.ID)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&bought))
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 24000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/sell", sale, http.StatusCreated)
	path := "/api/v1/investments/transactions/" + strconv.FormatInt(bought.Transaction.ID, 10) + "/replace-buy"

	late := tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000)
	late.TransactionDate = "2026-04-01"
	dependency := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path,
		investmentBuyReplacementRequest{Reason: "settled in April", Replacement: late}, http.StatusConflict)
	require.Contains(t, dependency.Body.String(), "INVESTMENT_BUY_DEPENDENCY")

	// The sale needs two shares the moved buy no longer provides, so first
	// give the source account a second acquisition.
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 30000), http.StatusCreated)
	moved := tradeRequestBody(f, other.ID, instrument.CommodityID, "5", 50000)
	moved.TransactionDate = "2026-01-15"
	request := investmentBuyReplacementRequest{Reason: "bought at the other broker", Replacement: moved}
	preview := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path+"/reconciliation-impact", request, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(preview.Body).Decode(&impact))
	require.NotNil(t, impact.GainImpact)
	request.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	var corrected investmentBuyReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&corrected))
	require.Equal(t, "2026-01-15", corrected.Replacement.Transaction.TransactionDate)
	require.Equal(t, "2026-02-01", corrected.InverseTransaction.TransactionDate)
	var movedLots, sourceOpen int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM current_investment_lots
		WHERE account_id = ? AND remaining_quantity_value = '5'`, other.ID).Scan(&movedLots))
	require.Equal(t, 1, movedLots)
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM current_investment_lots
		WHERE account_id = ? AND status = 'open'`, holding.ID).Scan(&sourceOpen))
	require.Zero(t, sourceOpen)
}

// --- Lifecycle ---

// assertMoneyValue compares a coefficient/scale pair against an expected
// amount stated at whatever scale reads most naturally. A gain carries the
// scale its own subtraction needed (T-101) and a disposed basis the scale the
// position's allocation policy fixes (T-103), so neither coefficient means
// anything read on its own.
func assertMoneyValue(t *testing.T, expectedValue int64, expectedScale int, gotValue moneyCoefficient, gotScale int, msgAndArgs ...any) {
	t.Helper()
	expected := exact.ScaledIntFromInt64(expectedValue, expectedScale)
	got := exact.ScaledIntFromInt64(int64(gotValue), gotScale)
	assert.Zerof(t, got.Cmp(expected), "expected %s, got %s %v", expected.String(), got.String(), msgAndArgs)
}

func TestInvestmentLifecycle_InstrumentBuyPreviewSellDividendReflectEverywhere(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "VWRL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	buyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(buyRes.Body).Decode(&bought))
	require.NotNil(t, bought.LotID)

	previewBody := investmentTradeRequest{
		TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		CashAccountID: f.cashAccount.ID, QuantityValue: exact.New(10), QuantityScale: 0,
		CashAmountValue: 120000, CashAmountScale: 2, CashCommodityID: f.commodityID,
	}
	previewRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/sell/preview", previewBody, http.StatusOK)
	var preview sellPreviewResponse
	require.NoError(t, json.NewDecoder(previewRes.Body).Decode(&preview))
	assertMoneyValue(t, 20000, 2, *preview.RealizedGain, *preview.RealizedGainScale)
	assert.Equal(t, "fallback", preview.DisposalDecision.ResolutionTier)
	assert.Equal(t, "fifo", preview.DisposalDecision.CostBasisMethod)
	assert.Nil(t, preview.DisposalDecision.ID)

	sellRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", previewBody, http.StatusCreated)
	var sold investmentTradeResponse
	require.NoError(t, json.NewDecoder(sellRes.Body).Decode(&sold))
	require.Len(t, sold.Allocations, 1)
	require.NotNil(t, sold.DisposalDecision)
	require.NotNil(t, sold.DisposalDecision.ID)
	require.NotNil(t, sold.DisposalDecision.TransactionVersionID)
	require.NotNil(t, sold.DisposalDecision.AuditEventID)
	assert.Equal(t, preview.DisposalDecision.CostBasisMethod, sold.DisposalDecision.CostBasisMethod)
	assert.Equal(t, preview.DisposalDecision.ResolutionTier, sold.DisposalDecision.ResolutionTier)
	assert.Equal(t, preview.DisposalDecision.Allocations, sold.DisposalDecision.Allocations)
	assert.Equal(t, preview.Allocations[0].CostBasisValue, sold.Allocations[0].CostBasisValue, "preview and commit must agree")

	dividendRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/dividend", dividendRequest{
		TransactionDate: "2026-04-01", CashAccountID: f.cashAccount.ID, CashCommodityID: f.commodityID,
		IncomeAccountID: &f.incomeAccount.ID, AmountValue: 5000, AmountScale: 2,
	}, http.StatusCreated)
	_ = dividendRes

	lotsRes := httptest.NewRequest(http.MethodGet, "/api/v1/investments/lots?account_id="+strconv.FormatInt(holding.ID, 10)+"&commodity_id="+strconv.FormatInt(instrument.CommodityID, 10), nil)
	lotsRes.AddCookie(f.sessionCookie)
	lotsRec := httptest.NewRecorder()
	handler.ServeHTTP(lotsRec, lotsRes)
	require.Equal(t, http.StatusOK, lotsRec.Code, lotsRec.Body.String())
	var lots investmentLotsResponse
	require.NoError(t, json.NewDecoder(lotsRec.Body).Decode(&lots))
	require.Len(t, lots.Lots, 1)
	assert.Equal(t, "closed", lots.Lots[0].Status)

	positionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/investments/positions", nil)
	positionsReq.AddCookie(f.sessionCookie)
	positionsRec := httptest.NewRecorder()
	handler.ServeHTTP(positionsRec, positionsReq)
	require.Equal(t, http.StatusOK, positionsRec.Code)

	gainsReq := httptest.NewRequest(http.MethodGet, "/api/v1/investments/gains", nil)
	gainsReq.AddCookie(f.sessionCookie)
	gainsRec := httptest.NewRecorder()
	handler.ServeHTTP(gainsRec, gainsReq)
	require.Equal(t, http.StatusOK, gainsRec.Code)
	var gains investmentGainsResponse
	require.NoError(t, json.NewDecoder(gainsRec.Body).Decode(&gains))
	require.Len(t, gains.Realized, 1)
	assertMoneyValue(t, 20000, 2, *gains.Realized[0].RealizedGainValue, *gains.Realized[0].RealizedGainScale)
}

func TestInvestmentLifecycle_GenericDeleteReturnsInvestmentWorkflowRequired(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "FENCE")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	buyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(buyRes.Body).Decode(&bought))

	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconv.FormatInt(bought.Transaction.ID, 10)+"/soft-delete",
		map[string]any{"change_reason": "must be refused"}, http.StatusConflict)
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&envelope))
	assert.Equal(t, "INVESTMENT_WORKFLOW_REQUIRED", envelope.Error.Code)
}

// --- Write-off (T-38) ---

func TestWriteOffInvestment_PreviewAndCommitCloseThePositionAtALoss(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "DEAD")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)

	body := investmentWriteOffRequest{
		TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		QuantityValue: exact.New(10), QuantityScale: 0, Reason: "fund closed; units cancelled at zero",
	}

	previewRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/write-off/preview", body, http.StatusOK)
	var preview sellPreviewResponse
	require.NoError(t, json.NewDecoder(previewRes.Body).Decode(&preview))
	assertMoneyValue(t, -100000, 2, *preview.RealizedGain, *preview.RealizedGainScale)
	assert.Equal(t, moneyCoefficient(0), preview.CashAmountValue)

	writeOffRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off", body, http.StatusCreated)
	var written investmentTradeResponse
	require.NoError(t, json.NewDecoder(writeOffRes.Body).Decode(&written))
	require.Len(t, written.Allocations, 1)
	assert.Equal(t, preview.Allocations[0].CostBasisValue, written.Allocations[0].CostBasisValue, "preview and commit must agree")

	gainsReq := httptest.NewRequest(http.MethodGet, "/api/v1/investments/gains", nil)
	gainsReq.AddCookie(f.sessionCookie)
	gainsRec := httptest.NewRecorder()
	handler.ServeHTTP(gainsRec, gainsReq)
	require.Equal(t, http.StatusOK, gainsRec.Code)
	var gains investmentGainsResponse
	require.NoError(t, json.NewDecoder(gainsRec.Body).Decode(&gains))
	require.Len(t, gains.Realized, 1)
	assertMoneyValue(t, -100000, 2, *gains.Realized[0].RealizedGainValue, *gains.Realized[0].RealizedGainScale)
	assert.Empty(t, gains.Unrealized, "a written-off position must not linger as an open holding")
}

func TestWriteOffInvestment_MissingReasonReturnsBadRequest(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "DEAD")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)

	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off", investmentWriteOffRequest{
		TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		QuantityValue: exact.New(10), QuantityScale: 0,
	}, http.StatusBadRequest)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
}

// --- Error mapping ---

func TestSellInvestment_InsufficientLotsReturnsConflict(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "VWRL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000), http.StatusCreated)

	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusConflict)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "CONFLICT", body.Error.Code)
}

func TestBuyInvestment_ValidationFailureReturnsBadRequest(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "VWRL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	body := tradeRequestBody(f, holding.ID, instrument.CommodityID, "0", 100000) // zero quantity
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", body, http.StatusBadRequest)
	var errBody errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&errBody))
	assert.Equal(t, "VALIDATION_FAILED", errBody.Error.Code)
}

func TestReadInvestmentInstrument_UnknownIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/investments/instruments/999999", nil)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestAcceptInvestmentEventSuggestion_AlreadyAcceptedReturnsConflict(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	suggestionID := seedSuggestionForAPITest(t, database, f)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/event-suggestions/"+strconv.FormatInt(suggestionID, 10)+"/accept", nil, http.StatusOK)

	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/event-suggestions/"+strconv.FormatInt(suggestionID, 10)+"/accept", nil, http.StatusConflict)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "CONFLICT", body.Error.Code)
}

func seedSuggestionForAPITest(t *testing.T, database *sql.DB, f investmentAPITestFixture) int64 {
	t.Helper()
	ctx := context.Background()
	var manualSourceID int64
	require.NoError(t, database.QueryRowContext(ctx, `SELECT id FROM market_data_sources WHERE code = 'manual'`).Scan(&manualSourceID))
	res, err := database.ExecContext(ctx, `
		INSERT INTO investment_provider_events (book_id, source_id, event_family, event_date, status, created_at)
		VALUES (1, ?, 'dividend', '2026-06-01', 'new', '2026-06-01T00:00:00Z')
	`, manualSourceID)
	require.NoError(t, err)
	providerEventID, err := res.LastInsertId()
	require.NoError(t, err)

	proposal := `{"kind":"dividend_income","transaction_date":"2026-06-01","cash_account_id":` +
		strconv.FormatInt(f.cashAccount.ID, 10) + `,"cash_commodity_id":` + strconv.FormatInt(f.commodityID, 10) +
		`,"amount_value":1250,"amount_scale":2,"income_account_id":` + strconv.FormatInt(f.incomeAccount.ID, 10) + `}`

	res, err = database.ExecContext(ctx, `
		INSERT INTO investment_event_suggestions (
			book_id, provider_event_id, confidence_bps, status, proposed_transaction_json, created_at, updated_at
		) VALUES (1, ?, 9000, 'suggested', ?, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')
	`, providerEventID, proposal)
	require.NoError(t, err)
	suggestionID, err := res.LastInsertId()
	require.NoError(t, err)
	return suggestionID
}

// --- Authentication / CSRF ---

func TestInvestmentMutations_RequireCSRFToken(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "VWRL")

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"create instrument", http.MethodPost, "/api/v1/investments/instruments", investmentInstrumentRequest{CommodityCode: "AAPL", InstrumentType: "stock", DisplayName: "Apple", Symbol: "AAPL", QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-01-01"}},
		{"update instrument", http.MethodPatch, "/api/v1/investments/instruments/" + strconv.FormatInt(instrument.ID, 10), investmentInstrumentRequest{CommodityCode: "VWRL", InstrumentType: "etf", DisplayName: "Renamed", Symbol: "VWRL", QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-01-01"}},
		{"create holding account", http.MethodPost, "/api/v1/investments/holding-accounts", holdingAccountRequest{InstrumentID: instrument.ID, Name: "Holding", OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"}},
		{"buy", http.MethodPost, "/api/v1/investments/buy", tradeRequestBody(f, 1, instrument.CommodityID, "1", 1000)},
		{"write-off", http.MethodPost, "/api/v1/investments/write-off", investmentWriteOffRequest{TransactionDate: "2026-02-01", CommodityID: instrument.CommodityID, HoldingAccountID: 1, QuantityValue: exact.New(1), Reason: "delisted"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.body)
			require.NoError(t, err)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(string(encoded)))
			req.Header.Set("Content-Type", "application/json")
			setSameOrigin(req)
			req.AddCookie(f.sessionCookie)
			// Deliberately omit the CSRF token header.
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusForbidden, res.Code, tc.name)
		})
	}
}

func TestInvestmentEndpoints_RequireAuthentication(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"search", http.MethodGet, "/api/v1/investments/search"},
		{"list instruments", http.MethodGet, "/api/v1/investments/instruments"},
		{"create instrument", http.MethodPost, "/api/v1/investments/instruments"},
		{"positions", http.MethodGet, "/api/v1/investments/positions"},
		{"lots", http.MethodGet, "/api/v1/investments/lots"},
		{"gains", http.MethodGet, "/api/v1/investments/gains"},
		{"buy", http.MethodPost, "/api/v1/investments/buy"},
		{"write-off", http.MethodPost, "/api/v1/investments/write-off"},
		{"write-off preview", http.MethodPost, "/api/v1/investments/write-off/preview"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusUnauthorized, res.Code, tc.name)
		})
	}
}

// --- Handler-level parsing edges ---

func TestReadInvestmentInstrument_NonIntegerPathIDRejected(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/investments/instruments/not-a-number", nil)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
}

func TestCreateInvestmentInstrument_MalformedJSONBodyRejected(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/investments/instruments", strings.NewReader(`{not-valid-json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, f.csrfToken)
	setSameOrigin(req)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
}

// --- Remaining config/list handlers ---

func TestUpdateInvestmentInstrument_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "VWRL")

	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPatch, "/api/v1/investments/instruments/"+strconv.FormatInt(instrument.ID, 10), investmentInstrumentRequest{
		CommodityCode: "VWRL", InstrumentType: "etf", DisplayName: "Vanguard FTSE All-World UCITS ETF", Symbol: "VWRL",
		QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-06-01",
		Identifiers: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
	}, http.StatusOK)
	var updated investmentInstrumentResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&updated))
	assert.Equal(t, "Vanguard FTSE All-World UCITS ETF", updated.DisplayName)
}

func TestCostBasisProfiles_ListAndSave_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	listRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/cost-basis-profiles", nil, http.StatusOK)
	var listed costBasisProfilesResponse
	require.NoError(t, json.NewDecoder(listRes.Body).Decode(&listed))
	require.Len(t, listed.Profiles, 1, "the default FIFO profile is seeded on first list")

	createRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/cost-basis-profiles", costBasisProfileRequest{
		Name: "ISA Average Cost", Method: "average_cost", Status: "active",
		Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var created costBasisProfileResponse
	require.NoError(t, json.NewDecoder(createRes.Body).Decode(&created))

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPatch, "/api/v1/investments/cost-basis-profiles/"+strconv.FormatInt(created.ID, 10), costBasisProfileRequest{
		Name: "ISA Average Cost v2", Method: "average_cost", Status: "active",
		Metadata: json.RawMessage(`{}`),
	}, http.StatusOK)
}

func TestDividendDefaults_ListAndSave_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	createRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/dividend-defaults", dividendDefaultRequest{
		IncomeAccountID: f.incomeAccount.ID, Status: "active", EffectiveFrom: "2026-01-01",
		Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var created dividendDefaultResponse
	require.NoError(t, json.NewDecoder(createRes.Body).Decode(&created))
	assert.Equal(t, f.incomeAccount.ID, created.IncomeAccountID)

	listRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/dividend-defaults", nil, http.StatusOK)
	var listed dividendDefaultsResponse
	require.NoError(t, json.NewDecoder(listRes.Body).Decode(&listed))
	require.Len(t, listed.Defaults, 1)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPatch, "/api/v1/investments/dividend-defaults/"+strconv.FormatInt(created.ID, 10), dividendDefaultRequest{
		IncomeAccountID: f.incomeAccount.ID, Status: "archived", EffectiveFrom: "2026-01-01",
		Metadata: json.RawMessage(`{}`),
	}, http.StatusOK)
}

func TestAutomationRules_ListAndReplace_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	saveRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/investments/automation-rules", investmentAutomationRulesRequest{
		Rules: []investmentAutomationRuleRequest{
			{EventFamily: "dividend", Mode: "suggest", ConfidenceThresholdBPS: 8000, EffectiveFrom: "2026-01-01", RequiredAccounts: json.RawMessage(`{}`)},
		},
	}, http.StatusOK)
	var saved investmentAutomationRulesResponse
	require.NoError(t, json.NewDecoder(saveRes.Body).Decode(&saved))
	require.Len(t, saved.Rules, 1)

	listRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/automation-rules", nil, http.StatusOK)
	var listed investmentAutomationRulesResponse
	require.NoError(t, json.NewDecoder(listRes.Body).Decode(&listed))
	require.Len(t, listed.Rules, 1)

	// An empty-list PUT archives the rule — true replace semantics (Part 1).
	emptyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/investments/automation-rules", investmentAutomationRulesRequest{}, http.StatusOK)
	var emptied investmentAutomationRulesResponse
	require.NoError(t, json.NewDecoder(emptyRes.Body).Decode(&emptied))
	assert.Empty(t, emptied.Rules)
}

func TestCreateReinvestedDividend_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "VWRL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/reinvested-dividend", reinvestedDividendRequest{
		TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		IncomeAccountID: &f.incomeAccount.ID, QuantityValue: exact.New(1), QuantityScale: 0,
		AmountValue: 2500, AmountScale: 2, CashCommodityID: f.commodityID,
	}, http.StatusCreated)
	var trade investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&trade))
	require.NotNil(t, trade.LotID)
}

func TestListInvestmentEventsAndSuggestions_HTTP(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	seedSuggestionForAPITest(t, database, f)

	eventsRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/events", nil, http.StatusOK)
	var events investmentProviderEventsResponse
	require.NoError(t, json.NewDecoder(eventsRes.Body).Decode(&events))
	require.Len(t, events.Events, 1)

	suggestionsRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/event-suggestions", nil, http.StatusOK)
	var suggestions investmentEventSuggestionsResponse
	require.NoError(t, json.NewDecoder(suggestionsRes.Body).Decode(&suggestions))
	require.Len(t, suggestions.Suggestions, 1)
	assert.Equal(t, "suggested", suggestions.Suggestions[0].Status)
}

// --- Write-off (T-38) ---

// TestWriteOffInvestment_RealizesTheWholeBasisAsALoss covers the case that was
// impossible before: a fund closure or worthless delisting left shares in open
// lots forever and the loss never reached realized gains.
func TestWriteOffInvestment_RealizesTheWholeBasisAsALoss(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "DEADCO")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	// 10 shares for 1000.00.
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)

	writeOffRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off",
		investmentWriteOffRequest{
			TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
			QuantityValue: exact.New(10), QuantityScale: 0,
			Reason: "fund liquidated with no distribution",
		}, http.StatusCreated)
	var wroteOff investmentTradeResponse
	require.NoError(t, json.NewDecoder(writeOffRes.Body).Decode(&wroteOff))
	require.Len(t, wroteOff.Allocations, 1)

	// The lot is closed, not left open with phantom shares.
	lotsReq := httptest.NewRequest(http.MethodGet, "/api/v1/investments/lots?account_id="+strconv.FormatInt(holding.ID, 10)+"&commodity_id="+strconv.FormatInt(instrument.CommodityID, 10), nil)
	lotsReq.AddCookie(f.sessionCookie)
	lotsRec := httptest.NewRecorder()
	handler.ServeHTTP(lotsRec, lotsReq)
	require.Equal(t, http.StatusOK, lotsRec.Code)
	var lots investmentLotsResponse
	require.NoError(t, json.NewDecoder(lotsRec.Body).Decode(&lots))
	require.Len(t, lots.Lots, 1)
	assert.Equal(t, "closed", lots.Lots[0].Status)

	// The whole cost basis lands as a realized loss: proceeds of zero against a
	// 1000.00 basis is -1000.00.
	gainsReq := httptest.NewRequest(http.MethodGet, "/api/v1/investments/gains", nil)
	gainsReq.AddCookie(f.sessionCookie)
	gainsRec := httptest.NewRecorder()
	handler.ServeHTTP(gainsRec, gainsReq)
	require.Equal(t, http.StatusOK, gainsRec.Code)
	var gains investmentGainsResponse
	require.NoError(t, json.NewDecoder(gainsRec.Body).Decode(&gains))
	require.Len(t, gains.Realized, 1)
	assertMoneyValue(t, -100000, 2, *gains.Realized[0].RealizedGainValue, *gains.Realized[0].RealizedGainScale)
	assert.Equal(t, moneyCoefficient(0), gains.Realized[0].ProceedsValue)

	// The transaction carries only the commodity legs — a zero-valued cash
	// posting would be noise a reconciler has to look at and dismiss.
	require.Len(t, wroteOff.Transaction.JournalEntries, 1)
	assert.Len(t, wroteOff.Transaction.JournalEntries[0].Postings, 2)
	for _, posting := range wroteOff.Transaction.JournalEntries[0].Postings {
		assert.Equal(t, instrument.CommodityID, posting.CommodityID, "a write-off has no cash side")
	}
}

func TestWriteOffInvestment_RequiresStatedIntent(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "DEADCO2")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)

	base := func() investmentTradeRequest {
		return investmentTradeRequest{
			TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
			QuantityValue: exact.New(1), QuantityScale: 0, ChangeReason: "worthless",
		}
	}

	// A reason is required: "why is this worth nothing" is the whole record.
	noReason := base()
	noReason.ChangeReason = "  "
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off", noReason, http.StatusBadRequest)

	// A cash amount is refused rather than ignored, so the route can never
	// become a second, unlabelled way to sell.
	withCash := base()
	withCash.CashAmountValue = 5000
	withCash.CashAmountScale = 2
	withCash.CashAccountID = f.cashAccount.ID
	withCash.CashCommodityID = f.commodityID
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off", withCash, http.StatusBadRequest)

	// Selling still demands a cash amount — the write-off route did not loosen
	// the ordinary path.
	sellNoCash := base()
	sellNoCash.CashAccountID = f.cashAccount.ID
	sellNoCash.CashCommodityID = f.commodityID
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sellNoCash, http.StatusBadRequest)

	// Still a mutation: CSRF applies.
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/write-off", base(), http.StatusForbidden)
}

// TestWriteOffInvestment_BackdatedIntoAReconciledPeriodIsGuarded proves the new
// mutation path goes through the reconciliation guard. Forgetting the guard on
// a new path over postings is a severity-1 bug in this repo, and "it reuses the
// guarded prepare function" is an assumption worth a test rather than a comment.
func TestWriteOffInvestment_BackdatedIntoAReconciledPeriodIsGuarded(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "DEADCO3")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	buyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(buyRes.Body).Decode(&bought))

	// Reconcile the holding account's commodity leg up to 2026-02-01.
	holdingPosting := postingByAccount(t, bought.Transaction, holding.ID)
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, holding.ID, instrument.CommodityID, holdingPosting, "2026-02-01")

	// A write-off dated inside the reconciled period would move a reconciled
	// balance, so it must be refused rather than silently accepted.
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off",
		investmentWriteOffRequest{
			TransactionDate: "2026-01-15", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
			QuantityValue: exact.New(10), QuantityScale: 0, Reason: "worthless",
		}, http.StatusConflict)

	// After the checkpoint it is allowed.
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off",
		investmentWriteOffRequest{
			TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
			QuantityValue: exact.New(10), QuantityScale: 0, Reason: "worthless",
		}, http.StatusCreated)
}

// TestInvestmentTrade_ReconciliationOverrideProceedsAndInvalidates is T-53: the
// guard above is correct, but until now the investments API had no way to say
// "yes, I mean it" the way the transaction editor does, so a backdated trade
// was not merely guarded — it was unreachable. Being refused with no way
// forward is a different bug from being refused.
func TestInvestmentTrade_ReconciliationOverrideProceedsAndInvalidates(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "DEADCO4")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	// The acquisition has to precede the backdated disposal below. What this
	// test is about is the reconciliation override, and it reached that guard
	// through a write-off dated before the shared fixture's own buy date —
	// which T-95 now rejects on its own terms, before reconciliation is ever
	// consulted. Buying in January keeps the disposal inside the reconciled
	// window without also making it a sale of shares not yet held.
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000)
	buy.TransactionDate = "2026-01-01"
	buyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		buy, http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(buyRes.Body).Decode(&bought))

	holdingPosting := postingByAccount(t, bought.Transaction, holding.ID)
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, holding.ID, instrument.CommodityID, holdingPosting, "2026-02-01")

	checkpointID := activeCheckpointID(t, handler, f.sessionCookie, holding.ID)

	backdated := func() investmentWriteOffRequest {
		return investmentWriteOffRequest{
			TransactionDate: "2026-01-15", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
			QuantityValue: exact.New(10), QuantityScale: 0, Reason: "delisted, written off", ChangeReason: "delisted, written off",
		}
	}

	// Without the flag the refusal stands — the override must be deliberate.
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off",
		backdated(), http.StatusConflict)

	withOverride := backdated()
	withOverride.ReconciliationOverride = true
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off",
		withOverride, http.StatusCreated)

	// Proceeding is only safe if the checkpoint it invalidates actually stops
	// claiming the account is reconciled. A 201 alone would leave a checkpoint
	// asserting a balance that the new posting has just changed.
	checkpoints := listCheckpointsForSession(t, handler, f.sessionCookie, holding.ID)
	var invalidated bool
	for _, checkpoint := range checkpoints {
		if checkpoint.ID != checkpointID {
			continue
		}
		invalidated = checkpoint.Status == "invalidated"
		require.NotEmpty(t, checkpoint.InvalidatedAt, "invalidated checkpoint records when")
		require.Equal(t, "delisted, written off", checkpoint.InvalidationReason, "the change reason carries into the invalidation")
	}
	require.True(t, invalidated, "the checkpoint the trade backdated past is invalidated")
}

// TestInvestmentReconciliationImpact_NamesTheCheckpointsTheWriteInvalidates is
// the other half of T-53. An override the user cannot see the consequences of
// is barely better than no override, so the UI names the checkpoints first —
// and a preview is only worth having if it plans the same postings the write
// does. This pins preview and write to the same answer rather than trusting
// that they share a builder.
func TestInvestmentReconciliationImpact_NamesTheCheckpointsTheWriteInvalidates(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "DEADCO5")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	// Dated before the backdated write-off below, for the same reason as
	// TestInvestmentTrade_ReconciliationOverrideProceedsAndInvalidates: the
	// subject here is which checkpoints the preview names, not whether a
	// disposal may precede its acquisition (T-95).
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000)
	buy.TransactionDate = "2026-01-01"
	buyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		buy, http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(buyRes.Body).Decode(&bought))

	holdingPosting := postingByAccount(t, bought.Transaction, holding.ID)
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, holding.ID, instrument.CommodityID, holdingPosting, "2026-02-01")
	checkpointID := activeCheckpointID(t, handler, f.sessionCookie, holding.ID)

	backdated := investmentWriteOffRequest{
		TransactionDate: "2026-01-15", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		QuantityValue: exact.New(10), QuantityScale: 0, Reason: "delisted, written off",
	}

	// The preview persists nothing, so it needs no CSRF token — same as
	// sell/preview.
	previewRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/write-off/reconciliation-impact", backdated, http.StatusOK)
	var preview reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(previewRes.Body).Decode(&preview))

	require.Len(t, preview.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, preview.AffectedCheckpoints[0].CheckpointID)
	require.Equal(t, holding.ID, preview.AffectedCheckpoints[0].AccountID)
	require.Equal(t, "2026-02-01", preview.AffectedCheckpoints[0].StatementDate)
	require.NotEmpty(t, preview.AffectedCheckpoints[0].AccountLabel, "the warning names the account, not just its id")

	// Nothing was written: the trade is still refused without the override.
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off",
		backdated, http.StatusConflict)

	// And the write invalidates exactly what the preview named.
	withOverride := backdated
	withOverride.ReconciliationOverride = true
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/write-off",
		withOverride, http.StatusCreated)

	invalidated := make([]int64, 0, 1)
	for _, checkpoint := range listCheckpointsForSession(t, handler, f.sessionCookie, holding.ID) {
		if checkpoint.Status == "invalidated" {
			invalidated = append(invalidated, checkpoint.ID)
		}
	}
	require.Equal(t, []int64{checkpointID}, invalidated, "preview and write agree on which checkpoints are affected")
}

// A trade that lands clear of every checkpoint must preview as empty, so the UI
// submits straight through instead of asking the user to confirm nothing.
func TestInvestmentReconciliationImpact_EmptyWhenNoCheckpointIsCrossed(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "DEADCO6")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	buyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(buyRes.Body).Decode(&bought))

	holdingPosting := postingByAccount(t, bought.Transaction, holding.ID)
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, holding.ID, instrument.CommodityID, holdingPosting, "2026-02-01")

	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/write-off/reconciliation-impact", investmentWriteOffRequest{
			TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
			QuantityValue: exact.New(10), QuantityScale: 0, Reason: "worthless",
		}, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&impact))
	require.Empty(t, impact.AffectedCheckpoints)
}

// The dividend and reinvested-dividend routes gained reconciliation_override
// and their own impact previews at the same time as buy/sell/write-off, but
// they take their own request types rather than the shared trade type, so the
// field had to be added to each one separately. These two tests exist because
// that is exactly the shape of change that gets wired for three of five routes
// and quietly missed on the other two: a dividend reaches the cash account and
// a reinvested dividend reaches the holding account, so each crosses a
// different checkpoint and neither is covered by the trade tests above.
func TestDividend_ReconciliationOverrideProceedsAndInvalidates(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "DIVCO1")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	// A dividend credits the cash account, so it is the cash account's
	// checkpoint that a backdated dividend would invalidate.
	buyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(buyRes.Body).Decode(&bought))

	cashPosting := postingByAccount(t, bought.Transaction, f.cashAccount.ID)
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, f.cashAccount.ID, f.commodityID, cashPosting, "2026-02-01")
	checkpointID := activeCheckpointID(t, handler, f.sessionCookie, f.cashAccount.ID)

	backdated := dividendRequest{
		TransactionDate: "2026-01-15", CommodityID: &instrument.CommodityID,
		CashAccountID: f.cashAccount.ID, CashCommodityID: f.commodityID,
		IncomeAccountID: &f.incomeAccount.ID, AmountValue: 5000, AmountScale: 2,
	}

	// The preview names the checkpoint and persists nothing, so it needs no
	// CSRF token.
	previewRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/dividend/reconciliation-impact", backdated, http.StatusOK)
	var preview reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(previewRes.Body).Decode(&preview))
	require.Len(t, preview.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, preview.AffectedCheckpoints[0].CheckpointID)
	require.Equal(t, f.cashAccount.ID, preview.AffectedCheckpoints[0].AccountID)
	require.NotEmpty(t, preview.AffectedCheckpoints[0].AccountLabel, "the warning names the account, not just its id")

	// Without the override the dividend is refused, and refused as a conflict
	// rather than surfacing as an internal error.
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/dividend",
		backdated, http.StatusConflict)

	withOverride := backdated
	withOverride.ReconciliationOverride = true
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/dividend",
		withOverride, http.StatusCreated)

	invalidated := make([]int64, 0, 1)
	for _, checkpoint := range listCheckpointsForSession(t, handler, f.sessionCookie, f.cashAccount.ID) {
		if checkpoint.Status == "invalidated" {
			invalidated = append(invalidated, checkpoint.ID)
		}
	}
	require.Equal(t, []int64{checkpointID}, invalidated, "preview and write agree on which checkpoints are affected")
}

func TestReinvestedDividend_ReconciliationOverrideProceedsAndInvalidates(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "DIVCO2")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	// A reinvested dividend buys more of the instrument rather than paying
	// cash, so the leg that lands in a reconciled period is the holding one.
	buyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(buyRes.Body).Decode(&bought))

	holdingPosting := postingByAccount(t, bought.Transaction, holding.ID)
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, holding.ID, instrument.CommodityID, holdingPosting, "2026-02-01")
	checkpointID := activeCheckpointID(t, handler, f.sessionCookie, holding.ID)

	backdated := reinvestedDividendRequest{
		TransactionDate: "2026-01-15", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		IncomeAccountID: &f.incomeAccount.ID, QuantityValue: exact.New(1), QuantityScale: 0,
		AmountValue: 2500, AmountScale: 2, CashCommodityID: f.commodityID,
	}

	previewRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/reinvested-dividend/reconciliation-impact", backdated, http.StatusOK)
	var preview reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(previewRes.Body).Decode(&preview))
	require.Len(t, preview.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, preview.AffectedCheckpoints[0].CheckpointID)
	require.Equal(t, holding.ID, preview.AffectedCheckpoints[0].AccountID)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/reinvested-dividend",
		backdated, http.StatusConflict)

	withOverride := backdated
	withOverride.ReconciliationOverride = true
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/reinvested-dividend",
		withOverride, http.StatusCreated)

	invalidated := make([]int64, 0, 1)
	for _, checkpoint := range listCheckpointsForSession(t, handler, f.sessionCookie, holding.ID) {
		if checkpoint.Status == "invalidated" {
			invalidated = append(invalidated, checkpoint.ID)
		}
	}
	require.Equal(t, []int64{checkpointID}, invalidated, "preview and write agree on which checkpoints are affected")
}

func listCheckpointsForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, accountID int64) []reconciliationCheckpointResponse {
	t.Helper()

	res := doInvestmentRequest(t, handler, sessionCookie, "", http.MethodGet,
		"/api/v1/accounts/"+strconv.FormatInt(accountID, 10)+"/reconciliation-checkpoints", nil, http.StatusOK)
	var body struct {
		Checkpoints []reconciliationCheckpointResponse `json:"checkpoints"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	return body.Checkpoints
}

func activeCheckpointID(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, accountID int64) int64 {
	t.Helper()

	for _, checkpoint := range listCheckpointsForSession(t, handler, sessionCookie, accountID) {
		if checkpoint.Status == "active" {
			return checkpoint.ID
		}
	}
	require.Failf(t, "no active checkpoint", "account_id=%d", accountID)
	return 0
}

func postingByAccount(t *testing.T, transaction transactionResponse, accountID int64) postingResponse {
	t.Helper()

	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.AccountID == accountID {
				return posting
			}
		}
	}
	require.Failf(t, "posting not found", "account_id=%d", accountID)
	return postingResponse{}
}

// T-98. The write boundary is the API, not the picker: the buy form never
// offers a holding account as the cash leg, and that is exactly why the
// endpoint has to refuse it on its own.
func TestBuyInvestment_RejectsHoldingAccountRolesThroughHTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "ROLE")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	otherHolding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	cases := []struct {
		name string
		body investmentTradeRequest
	}{
		{
			name: "the holding account settles its own purchase",
			body: investmentTradeRequest{
				TransactionDate: "2026-02-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
				CashAccountID: holding.ID, QuantityValue: exact.New(10), QuantityScale: 0,
				CashAmountValue: 10, CashAmountScale: 0, CashCommodityID: instrument.CommodityID,
			},
		},
		{
			name: "another holding account settles the purchase",
			body: investmentTradeRequest{
				TransactionDate: "2026-02-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
				CashAccountID: otherHolding.ID, QuantityValue: exact.New(10), QuantityScale: 0,
				CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: f.commodityID,
			},
		},
		{
			name: "the security is its own settlement commodity",
			body: investmentTradeRequest{
				TransactionDate: "2026-02-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
				CashAccountID: f.cashAccount.ID, QuantityValue: exact.New(10), QuantityScale: 0,
				CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: instrument.CommodityID,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", tc.body, http.StatusBadRequest)
			var body errorResponse
			require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
			assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
		})
	}

	// Nothing was created by any of them.
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/positions", nil, http.StatusOK)
	var positions investmentPositionsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&positions))
	assert.Empty(t, positions.Positions)
}

// T-95. A backdated trade is refused with its own code, not a bare validation
// failure: the rule is not visible in the form, so the client has something
// specific to explain.
func TestSellInvestment_BackdatedBehindASaleReplaysOrNamesDependency(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "ORDR")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)

	sell := investmentTradeRequest{
		TransactionDate: "2026-07-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		CashAccountID: f.cashAccount.ID, QuantityValue: exact.New(5), QuantityScale: 0,
		CashAmountValue: 60000, CashAmountScale: 2, CashCommodityID: f.commodityID,
	}
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sell, http.StatusCreated)

	// Selling 6 in March leaves only 4 for the recorded July sale of 5: the
	// preview and the commit name that dependency (T-117).
	impossible := sell
	impossible.TransactionDate = "2026-03-01"
	impossible.QuantityValue = exact.New(6)
	for _, path := range []string{"/api/v1/investments/sell", "/api/v1/investments/sell/preview",
		"/api/v1/investments/sell/reconciliation-impact"} {
		res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, impossible, http.StatusConflict)
		var body errorResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		assert.Equal(t, "INVESTMENT_SALE_DEPENDENCY", body.Error.Code, path)
	}

	// Selling 5 in March replays July onto the remaining shares at the same
	// per-share basis, so no committed gain changes and no acknowledgement is needed.
	backdated := sell
	backdated.TransactionDate = "2026-03-01"
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/sell/reconciliation-impact", backdated, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&impact))
	require.NotNil(t, impact.GainImpact)
	assert.Empty(t, impact.GainImpact.Changes)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", backdated, http.StatusCreated)
}

func TestBuyInvestment_BackdatedBehindSaleReplaysPosition(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "BUYBACK")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 60000)
	sale.TransactionDate = "2026-07-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)
	backdated := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 300000)
	backdated.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", backdated, http.StatusCreated)
}

func TestBuyReplacementPreviewAPIRejectsDependentDisposal(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "BUYPREVIEW")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&bought))
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 24000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)
	var before int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM transactions`).Scan(&before))
	request := investmentBuyReplacementRequest{Reason: "corrected quantity", Replacement: tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 10000)}
	path := "/api/v1/investments/transactions/" + strconv.FormatInt(bought.Transaction.ID, 10) + "/replace-buy/reconciliation-impact"
	conflict := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, request, http.StatusConflict)
	require.Contains(t, conflict.Body.String(), "INVESTMENT_BUY_DEPENDENCY")
	var after int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM transactions`).Scan(&after))
	require.Equal(t, before, after)
}
