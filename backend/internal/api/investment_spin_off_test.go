package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// #180: the spin-off endpoints, their preview, the reads and the named refusals.
func TestSpinOffAPIPreviewCommitReadsAndNamedRefusals(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	parent := createInstrumentForSession(t, handler, f, "PARENT")
	spun := createInstrumentForSession(t, handler, f, "SPINCO")
	holding := createHoldingAccountForSession(t, handler, f, parent.ID)
	spunHolding := createHoldingAccountForSession(t, handler, f, spun.ID)
	buy := tradeRequestBody(f, holding.ID, parent.CommodityID, "10", 10000)
	buy.TransactionDate = "2026-01-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", buy, http.StatusCreated)

	request := spinOffRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID,
		DestinationHoldingID: spunHolding.ID, CommodityID: parent.CommodityID, DestinationCommodityID: spun.CommodityID,
		RatioNumerator: 2, RatioDenominator: 4, BasisFractionValue: "250", BasisFractionScale: 3,
		SourceEvidence: json.RawMessage(`{"notice":"form 8937"}`)}
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/spin-offs/preview", request, http.StatusOK)
	var preview spinOffPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	assert.Equal(t, [2]int64{1, 2}, [2]int64{preview.Plan.RatioNumerator, preview.Plan.RatioDenominator}, "stored in lowest terms")
	assert.Equal(t, [2]any{"25", 2}, [2]any{preview.Plan.BasisFractionValue.String(), preview.Plan.BasisFractionScale})
	assert.Equal(t, "5", preview.Plan.DestinationQuantityValue.String())
	require.Len(t, preview.Plan.Links, 1)
	link := preview.Plan.Links[0]
	assert.Nil(t, link.DestinationLotID)
	assert.Equal(t, "10", link.SourceQuantityValue.String())
	require.NotNil(t, link.AllocatedBasisValue)
	require.NotNil(t, link.RemainingBasisValue)
	assert.Equal(t, "2026-01-01", *link.OriginalAcquiredOn)
	require.Len(t, preview.Plan.BasisTotals, 1)
	assert.NotNil(t, preview.Plan.BasisTotals[0].RemainingBasisValue)
	assert.Empty(t, preview.Impact.AffectedCheckpoints)

	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/spin-offs", request, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/spin-offs", request, http.StatusCreated)
	var created spinOffResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	require.Len(t, created.Transaction.JournalEntries, 1)
	assert.Len(t, created.Transaction.JournalEntries[0].Postings, 2)
	require.Len(t, created.Plan.Links, 1)
	require.NotNil(t, created.Plan.Links[0].DestinationLotID)
	assert.Equal(t, *link.AllocatedBasisValue, *created.Plan.Links[0].AllocatedBasisValue)

	// The new lot names the spin-off and keeps the parent's original date; the
	// chain explains it; the journal carries its title label.
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, fmt.Sprintf(
		"/api/v1/investments/lots?account_id=%d&commodity_id=%d", spunHolding.ID, spun.CommodityID), nil, http.StatusOK)
	var lots investmentLotsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&lots))
	require.Len(t, lots.Lots, 1)
	origin := lots.Lots[0].Origin
	require.NotNil(t, origin)
	assert.Equal(t, "spin_off", origin.TransferKind)
	assert.Equal(t, parent.CommodityID, origin.SourceCommodityID)
	assert.Equal(t, [2]int64{1, 2}, [2]int64{*origin.RatioNumerator, *origin.RatioDenominator})
	assert.Equal(t, "2026-01-01", *origin.OriginalAcquiredOn)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, fmt.Sprintf(
		"/api/v1/investments/transactions/%d/correction-chain", created.Transaction.ID), nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&chain))
	require.NotNil(t, chain.EffectiveSpinOff)
	assert.Equal(t, spunHolding.ID, chain.EffectiveSpinOff.DestinationHoldingAccountID)
	assert.JSONEq(t, `{"notice":"form 8937"}`, string(chain.EffectiveSpinOff.SourceEvidence))
	require.Len(t, chain.EffectiveSpinOff.Plan.Links, 1)
	assert.Equal(t, *created.Plan.Links[0].AllocatedBasisValue, *chain.EffectiveSpinOff.Plan.Links[0].AllocatedBasisValue)
	assert.Nil(t, chain.EffectiveSpinOff.Plan.Links[0].RemainingBasisValue, "the chain does not restate what stayed")
	assert.False(t, chain.CanReverseTransfer || chain.CanReplaceTransfer || chain.CanCorrectShareExchange,
		"a spin-off's correction is its own (#183)")
	assert.True(t, chain.CanCorrectSpinOff)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, fmt.Sprintf(
		"/api/v1/transactions/%d", created.Transaction.ID), nil, http.StatusOK)
	assert.Contains(t, res.Body.String(), `"system_label":"spin_off"`)

	// Named refusals.
	empty := createInstrumentForSession(t, handler, f, "EMPTY")
	emptyHolding := createHoldingAccountForSession(t, handler, f, empty.ID)
	none := request
	none.EffectiveOn, none.HoldingAccountID, none.CommodityID = "2026-03-01", emptyHolding.ID, empty.CommodityID
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/spin-offs", none, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_SPIN_OFF_NO_HOLDINGS")
	request.EffectiveOn, request.RatioNumerator, request.RatioDenominator = "2026-03-01", 1, 7
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/spin-offs", request, http.StatusUnprocessableEntity)
	assert.Contains(t, res.Body.String(), "INVESTMENT_SPIN_OFF_FRACTION_UNREPRESENTABLE")
	request.RatioNumerator, request.BasisFractionValue, request.BasisFractionScale = 1, "1", 0
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/spin-offs", request, http.StatusBadRequest)
	assert.Contains(t, res.Body.String(), "VALIDATION_FAILED")
	sale := tradeRequestBody(f, holding.ID, parent.CommodityID, "2", 3000)
	sale.TransactionDate = "2026-04-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/sell", sale, http.StatusCreated)
	// A spin-off dated behind the April sale is admitted by replay (#183)
	// once the restated gain is acknowledged.
	request.BasisFractionValue, request.BasisFractionScale, request.RatioDenominator = "1", 1, 1
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/spin-offs", request, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED")
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/spin-offs/preview", request, http.StatusOK)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	require.NotNil(t, preview.Impact.GainImpact)
	assert.Equal(t, "10", preview.Plan.Links[0].SourceQuantityValue.String(), "entitled with the units held in March")
	request.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/spin-offs", request, http.StatusCreated)
}

// #183: reversal and replacement of a spin-off, the dependency refusal and the
// correction fences.
func TestSpinOffCorrectionAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	parent := createInstrumentForSession(t, handler, f, "PARENT")
	spun := createInstrumentForSession(t, handler, f, "SPINCO")
	holding := createHoldingAccountForSession(t, handler, f, parent.ID)
	spunHolding := createHoldingAccountForSession(t, handler, f, spun.ID)
	buy := tradeRequestBody(f, holding.ID, parent.CommodityID, "10", 10000)
	buy.TransactionDate = "2026-01-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", buy, http.StatusCreated)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/spin-offs", spinOffRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID,
			DestinationHoldingID: spunHolding.ID, CommodityID: parent.CommodityID, DestinationCommodityID: spun.CommodityID,
			RatioNumerator: 1, RatioDenominator: 1, BasisFractionValue: "1", BasisFractionScale: 1}, http.StatusCreated)
	var first spinOffResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&first))
	path := func(id int64, action string) string {
		return fmt.Sprintf("/api/v1/investments/transactions/%d/%s", id, action)
	}
	// The exchange commands do not reach a spin-off.
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(first.Transaction.ID, "reverse-share-exchange"), investmentSaleReversalRequest{Reason: "x"}, http.StatusNotFound)

	// Replace 10 % with 25 % and 1:1 with 2:1: the preview and the commit agree.
	replacement := spinOffReplacementRequest{Reason: "issuer restated the allocation", EffectiveOn: "2026-02-01",
		DestinationHoldingID: spunHolding.ID, DestinationCommodityID: spun.CommodityID,
		RatioNumerator: 2, RatioDenominator: 1, BasisFractionValue: "25", BasisFractionScale: 2}
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path(first.Transaction.ID, "replace-spin-off/preview"), replacement, http.StatusOK)
	var preview spinOffPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	assert.Equal(t, "20", preview.Plan.DestinationQuantityValue.String())
	total := preview.Plan.BasisTotals[0]
	require.NotNil(t, total.AllocatedBasisValue)
	assert.Zero(t, exact.ScaledIntFromCoefficient(exact.Coefficient(*total.AllocatedBasisValue), *total.AllocatedBasisScale).Cmp(
		exact.ScaledIntFromInt64(2500, 2)), "a quarter of 100.00")
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path(first.Transaction.ID, "replace-spin-off"), replacement, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(first.Transaction.ID, "replace-spin-off"), replacement, http.StatusCreated)
	var replaced spinOffReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&replaced))
	assert.Equal(t, first.Transaction.ID, replaced.CorrectedTransactionID)
	assert.Equal(t, "20", replaced.Plan.DestinationQuantityValue.String())
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(first.Transaction.ID, "replace-spin-off"), replacement, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_SPIN_OFF_ALREADY_CORRECTED")
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(replaced.Replacement.ID, "replace-spin-off"), replacement, http.StatusBadRequest)
	assert.Contains(t, res.Body.String(), "VALIDATION_FAILED", "unchanged terms")
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		path(replaced.Replacement.ID, "correction-chain"), nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&chain))
	assert.True(t, chain.CanCorrectSpinOff)

	// A sale of the new units blocks reversing the replacement, by name.
	sale := tradeRequestBody(f, spunHolding.ID, spun.CommodityID, "5", 10000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/sell", sale, http.StatusCreated)
	reversal := investmentSaleReversalRequest{Reason: "not a spin-off"}
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path(replaced.Replacement.ID, "reverse-spin-off/reconciliation-impact"), reversal, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_TRANSFER_DEPENDENCY")
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(replaced.Replacement.ID, "reverse-spin-off"), reversal, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_TRANSFER_DEPENDENCY")
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(999999, "reverse-spin-off"), reversal, http.StatusNotFound)
}
