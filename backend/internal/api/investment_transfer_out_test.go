package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestExternalTransferOutAPIPreviewMatchesCommitAndLabelsBridge(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "IOUT")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 20000)
	buy.TransactionDate = "2026-01-01"
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", buy, http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&bought))
	require.NotNil(t, bought.LotID)
	path := "/api/v1/investments/transfers/external/out"
	request := externalTransferOutRequest{
		EffectiveOn: "2026-02-01", SourceAccountID: holding.ID, CommodityID: instrument.CommodityID,
		CostCommodityID: f.commodityID,
		LotAllocations:  []investmentLotAllocationRequest{{LotID: *bought.LotID, QuantityValue: exact.New(1)}},
		SourceEvidence:  json.RawMessage(`{"broker_reported_basis":"99.00"}`), Memo: "moved to broker B",
	}

	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/preview", request, http.StatusOK)
	var preview externalTransferOutPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	assert.Equal(t, "selected_lots", preview.Plan.BasisAllocation)
	assert.Zero(t, exact.ScaledIntFromInt64(int64(*preview.Plan.BasisValue), *preview.Plan.BasisScale).Cmp(
		exact.ScaledIntFromInt64(10000, 2)), "half the lot's 200.00, not the broker's figure")
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/reconciliation-impact", request, http.StatusOK)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, request, http.StatusForbidden)

	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	var posted externalTransferOutResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&posted))
	assert.Equal(t, preview.Plan.BasisValue, posted.Plan.BasisValue)
	require.Len(t, posted.Plan.Links, 1)
	assert.Equal(t, *bought.LotID, posted.Plan.Links[0].SourceLotID)
	var label string
	require.NoError(t, database.QueryRow(`SELECT link.role FROM investment_operation_journal_links link
		JOIN transaction_versions v ON v.id = link.transaction_version_id
		WHERE link.role <> 'primary' AND v.transaction_date = '2026-02-01'`).Scan(&label))
	assert.Equal(t, "transfer_bridge", label)
	listed := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/transactions", nil, http.StatusOK)
	assert.Contains(t, listed.Body.String(), `"system_label":"transfer_bridge"`)

	// A transfer dated behind the holding's latest depletion is admitted
	// through replay at its own slot (T-143).
	request.EffectiveOn = "2026-01-15"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
}
