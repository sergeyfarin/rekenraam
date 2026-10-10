package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// A sale over unknown basis reports NULL basis and gain with explicit
// knowledge, and its currency's realized total is NULL rather than the sum of
// the known entries (T-145).
func TestUnresolvedSaleReportsNullBasisGainAndTotal(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "UNK")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000), http.StatusCreated)
	// The projection alone is enough for the sale writer to see unknown basis.
	_, err := database.Exec(`UPDATE investment_lot_state SET basis_knowledge = 'unknown',
		remaining_cost_basis_value = NULL, remaining_cost_basis_scale = NULL`)
	require.NoError(t, err)

	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 24000)
	sale.TransactionDate = "2026-03-01"
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/sell/preview", sale, http.StatusOK)
	var preview map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &preview))
	require.Equal(t, "unknown", preview["basis_knowledge"])
	require.Contains(t, preview, "realized_gain")
	require.Nil(t, preview["realized_gain"])
	require.Nil(t, preview["realized_gain_scale"])
	decision := preview["disposal_decision"].(map[string]any)
	require.Equal(t, "unknown", decision["basis_knowledge"])
	require.Nil(t, decision["disposed_basis_value"])
	allocation := decision["allocations"].([]any)[0].(map[string]any)
	require.Equal(t, "unknown", allocation["basis_knowledge"])
	require.Nil(t, allocation["cost_basis_value"])
	require.Equal(t, "2", allocation["quantity_value"], "quantity is exact whatever the basis")

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains", nil, http.StatusOK)
	var gains map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &gains))
	realized := gains["realized"].([]any)
	require.Len(t, realized, 1)
	entry := realized[0].(map[string]any)
	require.Equal(t, "unknown", entry["basis_knowledge"])
	require.Nil(t, entry["realized_gain_value"])
	require.Nil(t, entry["disposed_basis_value"])
	require.Equal(t, "24000", entry["proceeds_value"])
	total := gains["realized_totals"].([]any)[0].(map[string]any)
	require.Equal(t, "unknown", total["basis_knowledge"])
	require.Nil(t, total["total_gain_value"])
	require.Equal(t, float64(1), total["unresolved_count"])
}

// Unknown inbound basis must be stated: basis_knowledge unknown with no
// amount. An omitted amount is still refused, and unknown with an amount too.
func TestExternalTransferInAPIRequiresExplicitUnknownBasis(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "UNKIN")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	request := externalTransferInRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID,
		CommodityID: instrument.CommodityID, QuantityValue: exact.New(2), CostCommodityID: f.commodityID}
	path := "/api/v1/investments/transfers/external/in"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusBadRequest)
	amount := moneyCoefficient(100)
	withAmount := request
	withAmount.BasisKnowledge, withAmount.CarriedBasisValue, withAmount.CarriedBasisScale = "unknown", &amount, 2
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, withAmount, http.StatusBadRequest)

	request.BasisKnowledge = "unknown"
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	var in externalTransferInResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &in))
	require.Len(t, in.Transaction.JournalEntries[0].Postings, 2, "security legs only")
	chain := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		"/api/v1/investments/transactions/"+strconv.FormatInt(in.Transaction.ID, 10)+"/correction-chain", nil, http.StatusOK)
	var body map[string]any
	require.NoError(t, json.Unmarshal(chain.Body.Bytes(), &body))
	terms := body["effective_transfer"].(map[string]any)
	require.Equal(t, "unknown", terms["basis_knowledge"])
	require.Nil(t, terms["carried_basis_value"], "never prefilled as zero")
}

// T-145 boundary 4 over HTTP: an unknown inbound offers resolution, the
// preview discloses the unresolved-to-known gain change, and the commit binds
// its acknowledgement. Afterwards the transfer can no longer be corrected.
func TestResolveTransferBasisAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "RESOLVE")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	in := externalTransferInRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID, BasisKnowledge: "unknown",
		CommodityID: instrument.CommodityID, QuantityValue: exact.New(2), CostCommodityID: f.commodityID}
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/transfers/external/in", in, http.StatusCreated)
	var transfer externalTransferInResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &transfer))
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 6000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)

	base := "/api/v1/investments/transactions/" + strconv.FormatInt(transfer.Transaction.ID, 10)
	chain := func() map[string]any {
		res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/correction-chain", nil, http.StatusOK)
		var body map[string]any
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
		return body
	}
	require.Equal(t, true, chain()["can_resolve_basis"])

	basis := moneyCoefficient(8000)
	request := investmentBasisResolutionRequest{BasisValue: &basis, BasisScale: 2, Reason: "statement found"}
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/resolve-basis/reconciliation-impact", request, http.StatusOK)
	var impact map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &impact))
	gainImpact := impact["gain_impact"].(map[string]any)
	change := gainImpact["changes"].([]any)[0].(map[string]any)
	require.Equal(t, "unknown", change["before"].(map[string]any)["basis_knowledge"])
	require.Equal(t, "known", change["after"].(map[string]any)["basis_knowledge"])
	request.GainImpactAcknowledgement = gainImpact["acknowledgement"].(string)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/resolve-basis", request, http.StatusCreated)

	after := chain()
	require.Equal(t, false, after["can_resolve_basis"])
	require.Equal(t, false, after["can_reverse_transfer"])
	request.GainImpactAcknowledgement = ""
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/resolve-basis", request, http.StatusConflict)
	require.Contains(t, res.Body.String(), "INVESTMENT_TRANSFER_BASIS_NOT_UNKNOWN")
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/reverse-transfer",
		map[string]any{"reason": "wrong"}, http.StatusConflict)
	require.Contains(t, res.Body.String(), "INVESTMENT_TRANSFER_BASIS_RESOLVED")
}

// A resolution is corrected through its transfer: the chain offers it with
// the effective terms, a replacement needs the gain acknowledgement, and a
// reversal makes the transfer resolvable again (#168).
func TestCorrectTransferBasisResolutionAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "RECORRECT")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	in := externalTransferInRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID, BasisKnowledge: "unknown",
		CommodityID: instrument.CommodityID, QuantityValue: exact.New(2), CostCommodityID: f.commodityID}
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/transfers/external/in", in, http.StatusCreated)
	var transfer externalTransferInResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &transfer))
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 6000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)

	base := "/api/v1/investments/transactions/" + strconv.FormatInt(transfer.Transaction.ID, 10)
	chain := func() map[string]any {
		res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/correction-chain", nil, http.StatusOK)
		var body map[string]any
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
		return body
	}
	acknowledgement := func(path string, request any) string {
		res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/reconciliation-impact", request, http.StatusOK)
		var impact map[string]any
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &impact))
		return impact["gain_impact"].(map[string]any)["acknowledgement"].(string)
	}
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/reverse-basis-resolution",
		map[string]any{"reason": "nothing yet"}, http.StatusConflict)
	require.Contains(t, res.Body.String(), "INVESTMENT_TRANSFER_BASIS_NOT_RESOLVED")

	basis := moneyCoefficient(8000)
	resolve := investmentBasisResolutionRequest{BasisValue: &basis, BasisScale: 2, Reason: "statement found"}
	resolve.GainImpactAcknowledgement = acknowledgement(base+"/resolve-basis", resolve)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/resolve-basis", resolve, http.StatusCreated)
	var resolved investmentBasisResolutionResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &resolved))
	terms := chain()
	require.Equal(t, true, terms["can_correct_basis_resolution"])
	effective := terms["effective_basis_resolution"].(map[string]any)
	require.Equal(t, "8000", effective["basis_value"])
	require.Equal(t, float64(resolved.Transaction.ID), effective["transaction_id"])

	corrected := moneyCoefficient(6000)
	replace := investmentBasisResolutionRequest{BasisValue: &corrected, BasisScale: 2, Reason: "statement mistyped"}
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/replace-basis-resolution", replace, http.StatusConflict)
	require.Contains(t, res.Body.String(), "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED")
	replace.GainImpactAcknowledgement = acknowledgement(base+"/replace-basis-resolution", replace)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/replace-basis-resolution", replace, http.StatusCreated)
	var replaced investmentBasisResolutionReplacementResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &replaced))
	require.Equal(t, transfer.Transaction.ID, replaced.TransferTransactionID)
	require.NotNil(t, replaced.Inverse)
	require.NotNil(t, replaced.Replacement)
	require.Equal(t, "6000", chain()["effective_basis_resolution"].(map[string]any)["basis_value"])

	reversal := map[string]any{"reason": "statement belonged to another account"}
	reversal["gain_impact_acknowledgement"] = acknowledgement(base+"/reverse-basis-resolution", reversal)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/reverse-basis-resolution", reversal, http.StatusCreated)
	after := chain()
	require.Equal(t, true, after["can_resolve_basis"])
	require.Equal(t, true, after["can_reverse_transfer"])
	require.Equal(t, false, after["can_correct_basis_resolution"])
	require.Nil(t, after["effective_basis_resolution"])
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/transactions/999999/reverse-basis-resolution/reconciliation-impact",
		map[string]any{"reason": "missing"}, http.StatusNotFound)
}

// A sourced known zero resolves through the same endpoint without a journal:
// the response has no transaction, and it is reversed the same way (#168).
func TestKnownZeroBasisResolutionAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "ZERO")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	in := externalTransferInRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID, BasisKnowledge: "unknown",
		CommodityID: instrument.CommodityID, QuantityValue: exact.New(2), CostCommodityID: f.commodityID}
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/transfers/external/in", in, http.StatusCreated)
	var transfer externalTransferInResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &transfer))
	base := "/api/v1/investments/transactions/" + strconv.FormatInt(transfer.Transaction.ID, 10)

	zero := moneyCoefficient(0)
	resolve := investmentBasisResolutionRequest{BasisValue: &zero, BasisScale: 2, Reason: "gifted shares, statement shows nil cost"}
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/resolve-basis/reconciliation-impact", resolve, http.StatusOK)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/resolve-basis", resolve, http.StatusCreated)
	var body map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Contains(t, body, "transaction")
	require.Nil(t, body["transaction"], "a known zero posts no journal")

	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/correction-chain", nil, http.StatusOK)
	var chain map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &chain))
	effective := chain["effective_basis_resolution"].(map[string]any)
	require.Equal(t, "0", effective["basis_value"])
	require.Nil(t, effective["transaction_id"], "a journal-free resolution has no transaction")

	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/reverse-basis-resolution",
		map[string]any{"reason": "the statement was for another lot"}, http.StatusCreated)
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Nil(t, body["transaction"])
	require.Equal(t, float64(transfer.Transaction.ID), body["transfer_transaction_id"])
}
