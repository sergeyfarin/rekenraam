package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureAccountsInBookValidatesTheWholeSelection(t *testing.T) {
	t.Parallel()
	database, _, currencyID := migratedInvestmentTestDatabase(t)
	repository := NewTransactionRepository(database)
	first := createInvestmentTestAccount(t, database, currencyID)
	second := createInvestmentTestAccount(t, database, currencyID)

	require.NoError(t, repository.EnsureAccountsInBook(context.Background(), 1, nil))
	require.NoError(t, repository.EnsureAccountsInBook(context.Background(), 1, []int64{first, second}))
	require.NoError(t, repository.EnsureAccountsInBook(context.Background(), 1, []int64{first, first, second}))
	require.ErrorIs(t, repository.EnsureAccountsInBook(context.Background(), 1, []int64{first, second + 1000}), ErrNotFound)
}
