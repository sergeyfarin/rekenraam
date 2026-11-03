package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #481: a currency exchange through the HTTP contract — the request names
// two account postings and a fee, the server adds the trading legs, the
// single-transaction read explains the deal, and exports keep it whole.
func TestExchangeAPICreatesReadsAndExports(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newReportingFixture(t, handler)
	euro := createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "Euro account", "asset", "checking", f.eurID, 2)
	fees := createCategoryForSession(t, handler, f.sessionCookie, f.csrfToken, `{"name":"Bank fees","category_type":"expense"}`)
	recordRate(t, handler, f, "2026-06-01", 920000, 6) // 1 USD = 0.92 EUR

	created := createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, `{
		"transaction_kind":"transfer","transaction_date":"2026-06-01","description":"To dollars",
		"journal_entries":[{"entry_kind":"exchange","postings":[
			{"account_id":`+strconvFormatInt(euro.ID)+`,"commodity_id":`+strconvFormatInt(f.eurID)+`,"quantity_value":"-10250","quantity_scale":2},
			{"account_id":`+strconvFormatInt(f.checking.ID)+`,"commodity_id":`+strconvFormatInt(f.usdID)+`,"quantity_value":"10800","quantity_scale":2,"posted_on":"2026-06-03"},
			{"account_id":`+strconvFormatInt(fees.ID)+`,"commodity_id":`+strconvFormatInt(f.eurID)+`,"quantity_value":"250","quantity_scale":2}]}]}`, http.StatusCreated)
	require.Len(t, created.JournalEntries, 2, "the arrival two days later is its own dated entry")
	trading := systemAccountID(t, handler, f.sessionCookie, "commodity_trading")
	tradingLegs := 0
	for _, entry := range created.JournalEntries {
		assert.Equal(t, "exchange", entry.EntryKind)
		for _, posting := range entry.Postings {
			if posting.AccountID == trading {
				tradingLegs++
			}
		}
	}
	assert.Equal(t, 2, tradingLegs, "one trading leg per currency")
	assert.Nil(t, created.ExchangeConversion, "only the single-transaction read explains the deal")

	read := readTransactionForSession(t, handler, f.sessionCookie, created.ID, http.StatusOK)
	require.NotNil(t, read.ExchangeConversion)
	conversion := read.ExchangeConversion
	assert.Equal(t, "10000", conversion.Sent.QuantityValue.String(), "the fee on top is outside the conversion")
	assert.Equal(t, "1080000", conversion.ImpliedRate.QuantityValue.String())
	require.NotNil(t, conversion.Market)
	assert.Equal(t, "64", conversion.Market.Cost.QuantityValue.String(), "0.64 EUR below the market rate")
	assert.Equal(t, "2026-06-01", conversion.Market.ObservationDate)

	// The transfer shape carries the statement amounts, days and fee payer,
	// on the read and in lists.
	require.NotNil(t, read.Transfer)
	assert.Equal(t, euro.ID, read.Transfer.Sending.AccountID)
	assert.Equal(t, "-10250", read.Transfer.Sending.QuantityValue.String())
	assert.Equal(t, "2026-06-03", read.Transfer.Receiving.PostedOn)
	require.NotNil(t, read.Transfer.Fee)
	assert.Equal(t, "sending", read.Transfer.Fee.PaidBy)
	assert.Equal(t, "sending", read.Transfer.ConversionSide)
	require.NotNil(t, created.Transfer)

	// A request naming a system account is refused before any write.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, `{
		"transaction_date":"2026-06-01",
		"journal_entries":[{"entry_kind":"exchange","postings":[
			{"account_id":`+strconvFormatInt(euro.ID)+`,"commodity_id":`+strconvFormatInt(f.eurID)+`,"quantity_value":"-100","quantity_scale":2},
			{"account_id":`+strconvFormatInt(f.checking.ID)+`,"commodity_id":`+strconvFormatInt(f.usdID)+`,"quantity_value":"108","quantity_scale":2},
			{"account_id":`+strconvFormatInt(trading)+`,"commodity_id":`+strconvFormatInt(f.eurID)+`,"quantity_value":"100","quantity_scale":2},
			{"account_id":`+strconvFormatInt(trading)+`,"commodity_id":`+strconvFormatInt(f.usdID)+`,"quantity_value":"-108","quantity_scale":2}]}]}`, http.StatusBadRequest)

	// QIF: the euro record is a transfer of the converted 100.00 to the money
	// in transit, plus the fee — never the whole 102.50 as Bank fees.
	var euroQIF []byte
	for name, content := range downloadQIF(t, handler, f.sessionCookie, "?account_id="+strconvFormatInt(euro.ID)+"&qif_date_layout=dmy", http.StatusOK).archive(t) {
		if strings.HasSuffix(name, ".qif") {
			euroQIF = content
		}
	}
	records := qifRecords(t, euroQIF)
	require.Len(t, records, 1)
	record := records[0]
	assert.Equal(t, []string{"-102.50"}, record["T"])
	require.Len(t, record["L"], 1)
	assert.True(t, strings.HasPrefix(record["L"][0], "["), "the record is a transfer, not spending: %v", record["L"])
	assert.Equal(t, []string{"-100.00", "-2.50"}, record["$"], "the converted amount and the fee, summing to T")
	assert.Equal(t, record["L"][0], record["S"][0])
	assert.Equal(t, "Bank fees", record["S"][1])
	assert.Equal(t, []string{"To dollars (other side: 108.00 USD)"}, record["M"])
}
