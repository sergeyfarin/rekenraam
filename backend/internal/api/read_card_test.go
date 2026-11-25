package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlite "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/app"
)

func TestReadCardLogsWrappedFailureCause(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
	err := fmt.Errorf("home budgets: %w", fmt.Errorf("read budget month: %w", sqlite.ErrBusy))

	card := mapReadCard(request, logger, "home: budgets", app.ReadCard[app.HomeBudgetSummary]{Err: err}, toHomeBudgetSummary)

	var entry map[string]any
	require.NoError(t, json.NewDecoder(&logs).Decode(&entry))
	assert.Equal(t, "ERROR", entry["level"])
	assert.Equal(t, "home: budgets", entry["source"])
	assert.Equal(t, "home budgets: read budget month: database is locked", entry["err"])
	body, marshalErr := json.Marshal(card)
	require.NoError(t, marshalErr)
	assert.JSONEq(t, `{"status":"failed","summary":null}`, string(body))
}
