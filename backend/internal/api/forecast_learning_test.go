package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestForecastLearningAPIRejectsOrphanAndInvalidOptions proves the opt-in
// parameters only mean something together, and that a malformed one is a
// validation error rather than a silently ignored setting.
func TestForecastLearningAPIRejectsOrphanAndInvalidOptions(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	cases := []struct {
		name  string
		query string
	}{
		{"history without the model", "?history_complete_from=2024-01-01"},
		{"category without the model", "?expense_category_id=1"},
		{"pattern without the model", "?expense_pattern=1:weekly"},
		{"unknown model", "?spending_model=neural_v9"},
		{"model without confirmed history", "?spending_model=adaptive_v1"},
		{"malformed history date", "?spending_model=adaptive_v1&history_complete_from=01-01-2024"},
		{"pattern without a colon", "?spending_model=adaptive_v1&history_complete_from=2024-01-01&expense_pattern=weekly"},
		{"pattern without a pattern", "?spending_model=adaptive_v1&history_complete_from=2024-01-01&expense_pattern=1:"},
		{"pattern without a category", "?spending_model=adaptive_v1&history_complete_from=2024-01-01&expense_pattern=:weekly"},
		{"unsupported pattern", "?spending_model=adaptive_v1&history_complete_from=2024-01-01&expense_pattern=1:quarterly"},
		{"negative category", "?spending_model=adaptive_v1&history_complete_from=2024-01-01&expense_category_id=-2"},
		{"still unknown parameter", "?spending_model=off&learning_debug=true"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balances"+testCase.query, http.StatusBadRequest)
		})
	}
}

// TestForecastLearningAPIOffMatchesPreModelResponse proves that turning the
// option off returns exactly the core forecast, byte for byte.
func TestForecastLearningAPIOffMatchesPreModelResponse(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	fixture.createFuturePosting(t, "2500")

	implicit := forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balances?horizon_days=30", http.StatusOK)
	explicit := forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balances?horizon_days=30&spending_model=off", http.StatusOK)

	var implicitBody, explicitBody map[string]any
	require.NoError(t, json.Unmarshal(implicit.Body.Bytes(), &implicitBody))
	require.NoError(t, json.Unmarshal(explicit.Body.Bytes(), &explicitBody))
	// computed_at moves between the two calls; nothing else may.
	delete(implicitBody, "computed_at")
	delete(explicitBody, "computed_at")
	assert.Equal(t, implicitBody, explicitBody)
	assert.Nil(t, implicitBody["learned_spending"], "an off request carries no learned overlay")
}

// TestForecastLearningAPIReturnsUnavailableOverlayWithoutHistory proves a fresh
// book gets an explicit, honest overlay rather than a zero-spending forecast.
func TestForecastLearningAPIReturnsUnavailableOverlayWithoutHistory(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	body := forecastBalancesFor(t, fixture, "?horizon_days=30&spending_model=adaptive_v1&history_complete_from=2024-01-01")

	require.NotNil(t, body.LearnedSpending)
	overlay := body.LearnedSpending
	assert.Equal(t, "unavailable", overlay.Status)
	assert.Equal(t, "adaptive_spending_v1", overlay.PolicyVersion)
	assert.Equal(t, "2024-01-01", overlay.HistoryCompleteFrom)
	assert.Equal(t, 0, overlay.EligibleGroupCount)
	assert.Empty(t, overlay.Series, "no learned curve is returned when nothing is estimable")
	assert.Empty(t, overlay.Groups)
	// The owner can still see which categories they may pick.
	require.NotEmpty(t, overlay.CategoryOptions)
	found := false
	for _, option := range overlay.CategoryOptions {
		found = found || option.AccountID == fixture.expense.ID
	}
	assert.True(t, found, "the book's expense account is offered as a category")
}

// TestForecastLearningAPIAcceptsCategoryAndPatternSelection proves the recipe
// parameters round-trip and that identities are validated from the snapshot.
func TestForecastLearningAPIAcceptsCategoryAndPatternSelection(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	expense := strconvFormatInt(fixture.expense.ID)
	checking := strconvFormatInt(fixture.checking.ID)
	base := "/api/v1/forecasts/balances?horizon_days=30&spending_model=adaptive_v1&history_complete_from=2024-01-01"

	forecastRequest(t, fixture.handler, fixture.session, base+"&expense_category_id="+expense+"&expense_pattern="+expense+":annual_seasonal", http.StatusOK)
	forecastRequest(t, fixture.handler, fixture.session, base+"&expense_category_id="+expense+"&expense_pattern="+expense+":monthly", http.StatusOK)

	// A funding account is not an expense category.
	forecastRequest(t, fixture.handler, fixture.session, base+"&expense_category_id="+checking, http.StatusBadRequest)
	// An override for a category outside the explicit selection would never
	// be applied, so it is rejected rather than ignored.
	forecastRequest(t, fixture.handler, fixture.session, base+"&expense_category_id="+expense+"&expense_pattern="+checking+":weekly", http.StatusBadRequest)
}

// TestForecastLearningAPIKeepsCoreSeriesIdentical proves the overlay is purely
// additive: enabling it never edits a core point.
func TestForecastLearningAPIKeepsCoreSeriesIdentical(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	fixture.createFuturePosting(t, "2500")

	off := forecastBalancesFor(t, fixture, "?horizon_days=30")
	on := forecastBalancesFor(t, fixture, "?horizon_days=30&spending_model=adaptive_v1&history_complete_from=2024-01-01")

	require.NotNil(t, on.LearnedSpending)
	assert.Equal(t, off.Series, on.Series)
	assert.Equal(t, off.Totals, on.Totals)
	assert.Equal(t, off.Assumptions, on.Assumptions)
	assert.Equal(t, off.Scope, on.Scope)
}
