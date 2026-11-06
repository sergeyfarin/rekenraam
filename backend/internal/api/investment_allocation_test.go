package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
)

func TestAllocationAPIReopensTargetsAndRefusesStaleAndUnauthorisedWrites(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "ALLOC")
	path := fmt.Sprintf("/api/v1/investments/allocation?from=2026-02-01&to=2026-02-01&reporting_currency_id=%d", f.commodityID)
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, path, nil, http.StatusOK)
	var initial app.AllocationRead
	require.NoError(t, json.NewDecoder(res.Body).Decode(&initial))
	require.Equal(t, app.AllocationPolicy, initial.Policy)
	require.Equal(t, int64(0), initial.Current.ID)
	require.NotNil(t, initial.Revisions)
	request := map[string]any{"expected_revision": 0, "account_ids": []int64{f.cashAccount.ID}, "targets": []db.AllocationTarget{{CommodityID: instrument.CommodityID, Units: 600000}, {Units: 400000}}}
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPut, "/api/v1/investments/allocation", request, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/investments/allocation", request, http.StatusOK)
	var revision db.AllocationRevision
	require.NoError(t, json.NewDecoder(res.Body).Decode(&revision))
	require.Positive(t, revision.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/investments/allocation", request, http.StatusConflict)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, path, nil, http.StatusOK)
	var read app.AllocationRead
	require.NoError(t, json.NewDecoder(res.Body).Decode(&read))
	require.Equal(t, revision, read.Current)
	require.Len(t, read.Revisions, 1)
	require.Equal(t, "2026-02-01", read.Points[0].Date)
	request["expected_revision"] = revision.ID
	request["targets"] = []db.AllocationTarget{{CommodityID: instrument.CommodityID, Units: 999999}}
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/investments/allocation", request, http.StatusBadRequest)
	for _, badPath := range []string{"/api/v1/investments/allocation?to=a", "/api/v1/investments/allocation?to=2026-02-30", "/api/v1/investments/allocation?reporting_currency_id=NaN", "/api/v1/investments/allocation?reporting_currency_id=-1"} {
		doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, badPath, nil, http.StatusBadRequest)
	}

	for _, malformed := range []any{
		map[string]any{"account_ids": []int64{}, "targets": []any{}},
		map[string]any{"expected_revision": revision.ID, "account_ids": []int64{}, "targets": []any{map[string]any{"units": 1000000}}},
		map[string]any{"expected_revision": revision.ID, "account_ids": []int64{}, "targets": []any{map[string]any{"commodity_id": 0}}},
	} {
		doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/investments/allocation", malformed, http.StatusBadRequest)
	}
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/investments/allocation", map[string]any{"expected_revision": revision.ID, "account_ids": []int64{999999}, "targets": []db.AllocationTarget{{Units: 1000000}}}, http.StatusBadRequest)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/investments/allocation", map[string]any{"expected_revision": revision.ID, "account_ids": revision.AccountIDs, "targets": []db.AllocationTarget{}}, http.StatusBadRequest)
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	// Exercise the actual export consumer, including its omission manifest.
	bundle := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/exports/bundle.zip", nil, http.StatusOK)
	archive, err := zip.NewReader(bytes.NewReader(bundle.Body.Bytes()), int64(bundle.Body.Len()))
	require.NoError(t, err)
	found := false
	for _, file := range archive.File {
		if file.Name == "manifest.json" {
			stream, err := file.Open()
			require.NoError(t, err)
			var manifest struct {
				Excluded []string `json:"excluded"`
			}
			require.NoError(t, json.NewDecoder(stream).Decode(&manifest))
			require.NoError(t, stream.Close())
			require.Contains(t, manifest.Excluded, "investment pool membership, allocation targets and their saved revision history")
			found = true
		}
	}
	require.True(t, found)

}
