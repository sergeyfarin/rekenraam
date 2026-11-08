package api

import (
	"encoding/json"
	"maps"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// reportScenarioFile is shared with e2e/playwright/reports.spec.ts (#110):
// both layers seed the same ledger and must report the same exact figures.
const reportScenarioFile = "../../../fixtures/report-scenario.json"

type reportScenario struct {
	Scale       int      `json:"scale"`
	Commodities []string `json:"commodities"`
	Accounts    []struct {
		Key       string `json:"key"`
		Name      string `json:"name"`
		Class     string `json:"class"`
		Kind      string `json:"kind"`
		Commodity string `json:"commodity"`
	} `json:"accounts"`
	Categories []struct {
		Key  string `json:"key"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"categories"`
	Transactions []struct {
		Why       string      `json:"why"`
		Day       int         `json:"day"`
		Commodity string      `json:"commodity"`
		Void      bool        `json:"void"`
		Postings  [][2]string `json:"postings"`
	} `json:"transactions"`
	Reports struct {
		Spending struct {
			Accounts []string                    `json:"accounts"`
			Expense  []reportScenarioSpendingRow `json:"expense"`
			Income   []reportScenarioSpendingRow `json:"income"`
		} `json:"spending"`
		Cashflow struct {
			Accounts []string                    `json:"accounts"`
			Rows     []reportScenarioCashflowRow `json:"rows"`
		} `json:"cashflow"`
	} `json:"reports"`
}

type reportScenarioSpendingRow struct {
	Category  string `json:"category"`
	Commodity string `json:"commodity"`
	Amount    string `json:"amount"`
}

type reportScenarioCashflowRow struct {
	Commodity    string `json:"commodity"`
	Inflow       string `json:"inflow"`
	Outflow      string `json:"outflow"`
	OperatingNet string `json:"operating_net"`
	TransferIn   string `json:"transfer_in"`
	TransferOut  string `json:"transfer_out"`
	TransferNet  string `json:"transfer_net"`
	NetMovement  string `json:"net_movement"`
}

// seededReportScenario maps the scenario's keys to the ids one run produced.
type seededReportScenario struct {
	scenario      reportScenario
	sessionCookie *http.Cookie
	anchor        string
	commodityIDs  map[string]int64
	accountIDs    map[string]int64 // accounts and categories alike
}

func loadReportScenario(t *testing.T) reportScenario {
	t.Helper()

	raw, err := os.ReadFile(reportScenarioFile)
	require.NoError(t, err)
	var scenario reportScenario
	require.NoError(t, json.Unmarshal(raw, &scenario))
	require.NotEmpty(t, scenario.Transactions)
	return scenario
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

// seedReportScenario writes the shared scenario through the public API, the
// same way the browser suite does, with every report anchored on anchor.
func seedReportScenario(t *testing.T, handler http.Handler, anchor string) seededReportScenario {
	t.Helper()

	scenario := loadReportScenario(t)
	anchorDate, err := time.Parse(time.DateOnly, anchor)
	require.NoError(t, err)
	openedOn := anchorDate.AddDate(0, 0, -30).Format(time.DateOnly)

	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	currencySetup := completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, scenario.Commodities[0], []setupCurrencySelectionRequest{
		{Code: scenario.Commodities[0], Name: scenario.Commodities[0]},
	})
	commodityIDs := map[string]int64{scenario.Commodities[0]: currencySetup.DefaultCurrency.ID}
	for _, code := range scenario.Commodities[1:] {
		commodityIDs[code] = createCurrencyForSession(t, handler, sessionCookie, csrfToken, mustJSON(t, map[string]string{"code": code, "name": code})).ID
	}
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)

	accountIDs := map[string]int64{}
	for _, account := range scenario.Accounts {
		commodityID, ok := commodityIDs[account.Commodity]
		require.True(t, ok, "account %s names unknown commodity %s", account.Key, account.Commodity)
		accountIDs[account.Key] = createAccountForSession(t, handler, sessionCookie, csrfToken, mustJSON(t, map[string]any{
			"name":                    account.Name,
			"account_class":           account.Class,
			"account_kind":            account.Kind,
			"default_commodity_id":    commodityID,
			"quantity_scale_override": scenario.Scale,
			"allows_postings":         true,
			"opened_on":               openedOn,
			"effective_from":          openedOn,
		})).ID
	}
	for _, category := range scenario.Categories {
		accountIDs[category.Key] = createCategoryForSession(t, handler, sessionCookie, csrfToken, mustJSON(t, map[string]string{
			"name":          category.Name,
			"category_type": category.Type,
		})).ID
	}

	for _, transaction := range scenario.Transactions {
		date := anchorDate.AddDate(0, 0, transaction.Day).Format(time.DateOnly)
		postings := make([]map[string]any, 0, len(transaction.Postings))
		for _, line := range transaction.Postings {
			accountID, ok := accountIDs[line[0]]
			require.True(t, ok, "%q posts to unknown key %s", transaction.Why, line[0])
			postings = append(postings, map[string]any{
				"account_id":     accountID,
				"commodity_id":   commodityIDs[transaction.Commodity],
				"quantity_value": line[1],
				"quantity_scale": scenario.Scale,
			})
		}
		created := createTransactionForSession(t, handler, sessionCookie, csrfToken, mustJSON(t, map[string]any{
			"transaction_date": date,
			"description":      transaction.Why,
			"journal_entries":  []map[string]any{{"entry_date": date, "postings": postings}},
		}), http.StatusCreated)
		if transaction.Void {
			mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost,
				"/api/v1/transactions/"+strconvFormatInt(created.ID)+"/void",
				`{"change_reason":"report scenario"}`, http.StatusOK)
		}
	}

	return seededReportScenario{
		scenario:      scenario,
		sessionCookie: sessionCookie,
		anchor:        anchor,
		commodityIDs:  commodityIDs,
		accountIDs:    accountIDs,
	}
}

func (s seededReportScenario) query(accounts []string, extra url.Values) string {
	values := url.Values{
		"start_date":          {s.anchor},
		"end_date":            {s.anchor},
		"bucket":              {"day"},
		"include_descendants": {"false"},
	}
	for _, key := range accounts {
		values.Add("account_id", strconvFormatInt(s.accountIDs[key]))
	}
	maps.Copy(values, extra)
	return "?" + values.Encode()
}

func (s seededReportScenario) commodityCode(t *testing.T, id int64) string {
	t.Helper()
	for code, commodityID := range s.commodityIDs {
		if commodityID == id {
			return code
		}
	}
	t.Fatalf("report names commodity %d, which the scenario never created", id)
	return ""
}

func (s seededReportScenario) categoryKey(t *testing.T, id int64) string {
	t.Helper()
	for key, accountID := range s.accountIDs {
		if accountID == id {
			return key
		}
	}
	t.Fatalf("report names category %d, which the scenario never created", id)
	return ""
}

// spendingRows flattens a spending response into the scenario's row order:
// groups in rank order, each group's commodities in response order — the same
// order the browser's table and CSV use.
func (s seededReportScenario) spendingRows(t *testing.T, report spendingResponse) []reportScenarioSpendingRow {
	t.Helper()
	rows := []reportScenarioSpendingRow{}
	for _, group := range report.Groups {
		require.NotNil(t, group.CategoryID, "scenario reports group by category")
		for _, total := range group.Totals {
			rows = append(rows, reportScenarioSpendingRow{
				Category:  s.categoryKey(t, *group.CategoryID),
				Commodity: s.commodityCode(t, total.CommodityID),
				Amount:    exact.Decimal(total.QuantityValue, total.QuantityScale),
			})
		}
	}
	return rows
}

// cashflowMeasure renders one commodity's figure for one measure. An absent
// entry is zero, as the browser renders it; a present one must render exactly.
func cashflowMeasure(quantities []balanceQuantityResponse, commodityID int64, scale int) string {
	for _, quantity := range quantities {
		if quantity.CommodityID == commodityID {
			return exact.Decimal(quantity.QuantityValue, quantity.QuantityScale)
		}
	}
	return exact.DecimalFromBig(big.NewInt(0), scale)
}

// The shared scenario produces its declared figures in every report. The same
// file drives the browser suite, so a disagreement here and there is a
// disagreement about one ledger, not about two fixtures.
func TestReportScenarioProducesItsDeclaredFigures(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	seeded := seedReportScenario(t, handler, "2026-06-15")
	expected := seeded.scenario.Reports

	expense := readSpendingForSession(t, handler, seeded.sessionCookie,
		seeded.query(expected.Spending.Accounts, url.Values{"group_by": {"category"}}))
	assert.Equal(t, expected.Spending.Expense, seeded.spendingRows(t, expense), "expense spending")

	income := readSpendingForSession(t, handler, seeded.sessionCookie,
		seeded.query(expected.Spending.Accounts, url.Values{"group_by": {"category"}, "mode": {"income"}}))
	assert.Equal(t, expected.Spending.Income, seeded.spendingRows(t, income), "income")

	cashflow := readCashflowForSession(t, handler, seeded.sessionCookie, seeded.query(expected.Cashflow.Accounts, nil))
	require.Len(t, cashflow.Buckets, 1, "the scenario reports one day")
	bucket := cashflow.Buckets[0]
	got := []reportScenarioCashflowRow{}
	for _, code := range seeded.scenario.Commodities {
		id := seeded.commodityIDs[code]
		row := reportScenarioCashflowRow{
			Commodity:    code,
			Inflow:       cashflowMeasure(bucket.Inflow, id, seeded.scenario.Scale),
			Outflow:      cashflowMeasure(bucket.Outflow, id, seeded.scenario.Scale),
			OperatingNet: cashflowMeasure(bucket.OperatingNet, id, seeded.scenario.Scale),
			TransferIn:   cashflowMeasure(bucket.TransferIn, id, seeded.scenario.Scale),
			TransferOut:  cashflowMeasure(bucket.TransferOut, id, seeded.scenario.Scale),
			TransferNet:  cashflowMeasure(bucket.TransferNet, id, seeded.scenario.Scale),
			NetMovement:  cashflowMeasure(bucket.NetMovement, id, seeded.scenario.Scale),
		}
		got = append(got, row)
	}
	assert.Equal(t, expected.Cashflow.Rows, got, "cashflow")

	// Every commodity the response carries is one the scenario expects a row
	// for: a stray commodity would be a row the browser shows and this test
	// would otherwise never look at.
	for _, measure := range [][]balanceQuantityResponse{bucket.Inflow, bucket.Outflow, bucket.OperatingNet, bucket.TransferIn, bucket.TransferOut, bucket.TransferNet, bucket.NetMovement} {
		for _, quantity := range measure {
			seeded.commodityCode(t, quantity.CommodityID)
		}
	}
}
