package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// The second #481 review: a journal the transfer form or the split editor
// admits must survive the editor's read → edit → save unchanged, and a
// journal neither can restate must not be normalized by a save.

func TestTransferShapeDeclinesARefundLine(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)

	// 100.00 out, 110.00 in, a 10.00 refund credited to the expense: the
	// form's fee is a cost charged on top, so this is not a transfer.
	refund, err := f.create(t, TransactionInput{TransactionDate: "2026-06-01", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
		{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10000), QuantityScale: 2},
		{AccountID: f.savingsAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(11000), QuantityScale: 2},
		{AccountID: f.feesAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-1000), QuantityScale: 2},
	}}}})
	require.NoError(t, err)
	assert.Nil(t, TransferShapeOf(refund), "a refund line stays a split")
	require.NotNil(t, JournalRestatementOf(refund), "the split editor restates it as it is")

	// Naming a payer for it is refused rather than read as a fee.
	_, err = f.create(t, TransactionInput{TransactionDate: "2026-06-01", JournalEntries: []JournalEntryInput{{FeePaidBy: "sending", Postings: []PostingInput{
		{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10000), QuantityScale: 2},
		{AccountID: f.savingsAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(11000), QuantityScale: 2},
		{AccountID: f.feesAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-1000), QuantityScale: 2},
	}}}})
	requireValidationContains(t, err, "must be positive")
}

func TestExchangeKeepsTheReceivingConversionSideOnOneDay(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)

	// Both sides post on one day, so the days cannot tell who converted;
	// the stored choice does.
	created, err := f.exchangeEntry(t, "2026-06-01", "receiving", f.eur(-10000, ""), f.usd(10800, ""))
	require.NoError(t, err)
	shape := TransferShapeOf(created)
	require.NotNil(t, shape)
	assert.Equal(t, "receiving", shape.ConversionSide)
	restated := JournalRestatementOf(created)
	require.NotNil(t, restated)
	assert.Equal(t, "receiving", restated.ConversionSide)

	// When the arrival moves, the conversion moves with it.
	moved, err := f.moveDay(t, created, f.usdAccountID, "later", false)
	require.NoError(t, err)
	assert.Contains(t, journalShape(moved), datedPosting{"2026-06-02", f.tradingAccountID, "-10800"})
	assert.Contains(t, journalShape(moved), datedPosting{"2026-06-02", f.tradingAccountID, "10000"})
	assert.Equal(t, "receiving", TransferShapeOf(moved).ConversionSide)
}

func TestTransferFeePayerSurvivesOneDayAndFollowsItsSide(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)

	// 110.00 transferred, the receiving bank keeps 10.00: the journal is the
	// same as 100.00 transferred with a 10.00 fee paid by the sender.
	created, err := f.create(t, TransactionInput{TransactionDate: "2026-06-01", JournalEntries: []JournalEntryInput{{FeePaidBy: "receiving", Postings: []PostingInput{
		{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-11000), QuantityScale: 2},
		{AccountID: f.savingsAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(10000), QuantityScale: 2},
		{AccountID: f.feesAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(1000), QuantityScale: 2},
	}}}})
	require.NoError(t, err)
	shape := TransferShapeOf(created)
	require.NotNil(t, shape)
	require.NotNil(t, shape.Fee)
	assert.Equal(t, "receiving", shape.Fee.PaidBy)

	moved, err := f.moveDay(t, created, f.savingsAccountID, "later", false)
	require.NoError(t, err)
	assert.Contains(t, journalShape(moved), datedPosting{"2026-06-02", f.feesAccountID, "1000"}, "the fee moves with the side that paid it")
	assert.Equal(t, "receiving", TransferShapeOf(moved).Fee.PaidBy)
}

func TestJournalRestatementRebuildsDelayedFeesMemosAndTheConversionDay(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)
	ctx := context.Background()

	delayed, err := f.create(t, TransactionInput{TransactionDate: "2026-06-01", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
		{AccountID: f.cashAccountID, PostedOn: "2026-06-02", CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10100), QuantityScale: 2, Memo: "sender reference"},
		{AccountID: f.savingsAccountID, PostedOn: "2026-06-03", CommodityID: f.eurCommodityID, QuantityValue: exact.New(10000), QuantityScale: 2, Memo: "receiver reference"},
		{AccountID: f.feesAccountID, PostedOn: "2026-06-02", CommodityID: f.eurCommodityID, QuantityValue: exact.New(100), QuantityScale: 2, Memo: "fee reference"},
	}}}})
	require.NoError(t, err)
	restated := JournalRestatementOf(delayed)
	require.NotNil(t, restated)
	days := map[int64]string{}
	for _, posting := range restated.Postings {
		days[posting.AccountID] = posting.PostedOn
	}
	assert.Equal(t, map[int64]string{f.cashAccountID: "2026-06-02", f.savingsAccountID: "2026-06-03", f.feesAccountID: "2026-06-02"}, days,
		"the fee keeps its day though the editor shows no day for it")

	// Saving the restatement unchanged writes the same journal and memos.
	spec := transactionInputFromTransaction(delayed)
	spec.JournalEntries = []JournalEntryInput{*restated}
	updated, err := f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		ExpectedVersionID: delayed.VersionID, OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		TransactionID: delayed.ID, Spec: spec, ChangeReason: "unchanged",
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, journalShape(delayed), journalShape(updated))
	assert.ElementsMatch(t, postingMemos(delayed), postingMemos(updated))

	// A receiving-bank exchange restates with its side, and saving it keeps
	// the conversion on the arrival day.
	exchange, err := f.exchangeEntry(t, "2026-06-01", "receiving", f.eur(-10000, ""), f.usd(10800, "2026-06-03"))
	require.NoError(t, err)
	restatedExchange := JournalRestatementOf(exchange)
	require.NotNil(t, restatedExchange)
	assert.Equal(t, "receiving", restatedExchange.ConversionSide)
	assert.Len(t, restatedExchange.Postings, 2, "the trading legs are the writer's")
	spec = transactionInputFromTransaction(exchange)
	spec.JournalEntries = []JournalEntryInput{*restatedExchange}
	updatedExchange, err := f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		ExpectedVersionID: exchange.VersionID, OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		TransactionID: exchange.ID, Spec: spec, ChangeReason: "unchanged",
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, journalShape(exchange), journalShape(updatedExchange))
	assert.Contains(t, journalShape(updatedExchange), datedPosting{"2026-06-03", f.tradingAccountID, "-10800"})
}

func TestJournalRestatementDeclinesAResidualAndKeepJournalPreservesIt(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)
	ctx := context.Background()
	leg := func(accountID, commodityID, value int64) PostingInput {
		return PostingInput{AccountID: accountID, CommodityID: commodityID, QuantityValue: exact.New(value), QuantityScale: 2}
	}

	// The reviewer's journal: 99.00 EUR converted, 1.00 EUR left in clearing.
	created, err := f.create(t, TransactionInput{TransactionDate: "2026-06-01", Description: "Clearing preservation", JournalEntries: []JournalEntryInput{
		{EntryDate: "2026-06-01", Postings: []PostingInput{
			leg(f.cashAccountID, f.eurCommodityID, -10000), leg(f.tradingAccountID, f.eurCommodityID, 9900), leg(f.clearingAccountID, f.eurCommodityID, 100),
			leg(f.tradingAccountID, f.usdCommodityID, -10800), leg(f.clearingAccountID, f.usdCommodityID, 10800),
		}},
		{EntryDate: "2026-06-03", Postings: []PostingInput{leg(f.usdAccountID, f.usdCommodityID, 10800), leg(f.clearingAccountID, f.usdCommodityID, -10800)}},
	}})
	require.NoError(t, err)
	assert.Nil(t, TransferShapeOf(created))
	assert.Nil(t, JournalRestatementOf(created), "no editor restates a residual in clearing")

	// A description edit keeps the journal exactly.
	spec := transactionInputFromTransaction(created)
	spec.Description = "Clearing preservation edited"
	spec.JournalEntries = nil
	updated, err := f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		ExpectedVersionID: created.VersionID, OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		TransactionID: created.ID, Spec: spec, ChangeReason: "description only", KeepJournal: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "Clearing preservation edited", updated.Description)
	assert.ElementsMatch(t, journalShape(created), journalShape(updated), "the 1.00 EUR stays in clearing")
	assert.Len(t, updated.JournalEntries, 2)

	// Keeping the journal and sending one is a contradiction.
	spec = transactionInputFromTransaction(updated)
	_, err = f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		ExpectedVersionID: updated.VersionID, OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		TransactionID: updated.ID, Spec: spec, ChangeReason: "both", KeepJournal: true,
	})
	requireValidationContains(t, err, "names no journal entries")
}

func postingMemos(transaction Transaction) []string {
	var memos []string
	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.Memo != "" {
				memos = append(memos, posting.Memo)
			}
		}
	}
	return memos
}

func requireValidationContains(t *testing.T, err error, message string) {
	t.Helper()
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Contains(t, validation.Message, message)
}
