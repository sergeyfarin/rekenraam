package app

import (
	"sort"
	"strconv"
	"strings"

	"rekenraam/backend/internal/exact"
)

// TransferShape is a stored transaction restated as the transfer it is
// (#481): what left the sending account and what reached the receiving one,
// each on its own posting day, at most one fee charged on top by one of
// them, and, for two currencies, which side converted. It is the one rule
// for which transactions Transfer entry edits; the editor never decides it.
//
// A transaction has a shape only when restating it as a transfer request and
// rebuilding it with the writer's own rules — the exchange legs, the fee and
// conversion days, the transfer_clearing bridge — reproduces the stored
// journal exactly: every entry date, account, commodity and amount. A
// journal anything else wrote (a residual left in clearing, a hand-built
// exchange, a fee on a third day) has none and stays in the split editor, so
// opening and saving it never reshapes it silently. Posting tags and posting
// metadata, which the transfer form does not carry, also leave it without a
// shape; memos and line keys are carried so an unchanged save keeps them.
// The fee must be a positive cost: a refund line is not a fee charged on
// top, and the form would turn it into one. The side that converted and the
// side that paid the fee are the recorded choices when the entries carry
// them, so they survive a day on which both sides post.
type TransferShape struct {
	Sending   TransferShapeSide
	Receiving TransferShapeSide
	Fee       *TransferShapeFee
	// ConversionSide is "sending" or "receiving" for an exchange, empty in
	// one currency.
	ConversionSide string
	EntryMemo      string
	// EntryKind is the stored kind of the entries, sent back so a restated
	// write does not change it.
	EntryKind string
}

// TransferShapeSide is one account's posting as its statement shows it.
type TransferShapeSide struct {
	AccountID     int64
	CommodityID   int64
	QuantityValue exact.Coefficient // signed, as stored
	QuantityScale int
	PostedOn      string
	LineKey       string
	Memo          string
}

// TransferShapeFee is the fee's expense posting and the side that paid it.
type TransferShapeFee struct {
	AccountID     int64
	CommodityID   int64
	QuantityValue exact.Coefficient
	QuantityScale int
	PaidBy        string // "sending" | "receiving"
	LineKey       string
	Memo          string
}

// TransferShapeOf returns the transaction's transfer shape, or nil.
func TransferShapeOf(transaction Transaction) *TransferShape {
	if transaction.TransactionKind == "investment" || len(transaction.JournalEntries) == 0 {
		return nil
	}
	first := transaction.JournalEntries[0]
	exchange := first.EntryKind == exchangeEntryKind
	var accounts []located
	var fee *located
	var trading, clearing int64
	tradingDays := map[string]bool{}
	for _, entry := range transaction.JournalEntries {
		if entry.EntryKind != first.EntryKind || entry.Memo != first.Memo || !emptyJSON(entry.MetadataJSON) ||
			entry.ConversionSide != first.ConversionSide || entry.FeePaidBy != first.FeePaidBy {
			return nil
		}
		for _, posting := range entry.Postings {
			if len(posting.TagIDs) > 0 || !emptyJSON(posting.MetadataJSON) {
				return nil
			}
			role := ""
			if posting.AccountSystemRole != nil {
				role = *posting.AccountSystemRole
			}
			switch {
			case role == "commodity_trading" && exchange:
				trading = posting.AccountID
				tradingDays[entry.EntryDate] = true
			case role == "transfer_clearing" && len(transaction.JournalEntries) > 1:
				clearing = posting.AccountID
			case role != "":
				return nil
			case posting.AccountClass == "asset" || posting.AccountClass == "liability":
				accounts = append(accounts, located{entry.EntryDate, posting})
			case posting.AccountClass == "expense" && fee == nil:
				fee = &located{entry.EntryDate, posting}
			default:
				return nil
			}
		}
	}
	if len(accounts) != 2 || accounts[0].posting.AccountID == accounts[1].posting.AccountID {
		return nil
	}
	sending, receiving := accounts[0], accounts[1]
	if sending.posting.QuantityValue.Sign() > 0 {
		sending, receiving = receiving, sending
	}
	if sending.posting.QuantityValue.Sign() >= 0 || receiving.posting.QuantityValue.Sign() <= 0 {
		return nil
	}
	twoCurrencies := sending.posting.CommodityID != receiving.posting.CommodityID
	if twoCurrencies != exchange {
		return nil
	}

	shape := &TransferShape{
		Sending:   sideOf(sending),
		Receiving: sideOf(receiving),
		EntryMemo: first.Memo,
		EntryKind: first.EntryKind,
	}
	if fee != nil {
		// The form's fee is a cost charged on top; a refund line is not one,
		// and turning it into its magnitude would change both balances.
		if fee.posting.QuantityValue.Sign() <= 0 {
			return nil
		}
		// The recorded payer, else the side the fee posts with. With both
		// sides on one day either fits; the rebuild below checks the choice.
		paidBy := first.FeePaidBy
		switch {
		case paidBy != "":
		case fee.posting.CommodityID == sending.posting.CommodityID && fee.date == sending.date:
			paidBy = conversionSending
		case fee.posting.CommodityID == receiving.posting.CommodityID && fee.date == receiving.date:
			paidBy = conversionReceiving
		default:
			return nil
		}
		shape.Fee = &TransferShapeFee{
			AccountID: fee.posting.AccountID, CommodityID: fee.posting.CommodityID,
			QuantityValue: fee.posting.QuantityValue, QuantityScale: fee.posting.QuantityScale,
			PaidBy: paidBy, LineKey: fee.posting.LineKey, Memo: fee.posting.Memo,
		}
	}
	if exchange {
		if len(tradingDays) != 1 {
			return nil
		}
		switch {
		case first.ConversionSide != "":
			shape.ConversionSide = first.ConversionSide
		case tradingDays[sending.date]:
			shape.ConversionSide = conversionSending
		case tradingDays[receiving.date]:
			shape.ConversionSide = conversionReceiving
		default:
			return nil
		}
	}

	rebuilt, err := shape.rebuild(transaction.TransactionDate, trading, clearing)
	if err != nil || !sameJournal(transaction.JournalEntries, rebuilt) {
		return nil
	}
	return shape
}

type located struct {
	date    string
	posting Posting
}

func sideOf(at located) TransferShapeSide {
	return TransferShapeSide{
		AccountID: at.posting.AccountID, CommodityID: at.posting.CommodityID,
		QuantityValue: at.posting.QuantityValue, QuantityScale: at.posting.QuantityScale,
		PostedOn: at.date, LineKey: at.posting.LineKey, Memo: at.posting.Memo,
	}
}

func emptyJSON(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "" || trimmed == "{}" || trimmed == "null"
}

// entry restates the shape as the transfer request that wrote it: each side
// on its day, the fee on its payer's day, the conversion side named.
func (shape *TransferShape) entry(transactionDate string) (JournalEntryInput, []postingDayRole, []exchangeLegKind) {
	entry := JournalEntryInput{EntryDate: transactionDate, EntryKind: shape.EntryKind, Memo: shape.EntryMemo}
	postings := []PostingInput{
		{LineKey: shape.Sending.LineKey, AccountID: shape.Sending.AccountID, CommodityID: shape.Sending.CommodityID,
			QuantityValue: shape.Sending.QuantityValue, QuantityScale: shape.Sending.QuantityScale, PostedOn: shape.Sending.PostedOn, Memo: shape.Sending.Memo},
		{LineKey: shape.Receiving.LineKey, AccountID: shape.Receiving.AccountID, CommodityID: shape.Receiving.CommodityID,
			QuantityValue: shape.Receiving.QuantityValue, QuantityScale: shape.Receiving.QuantityScale, PostedOn: shape.Receiving.PostedOn, Memo: shape.Receiving.Memo},
	}
	roles := []postingDayRole{{account: true}, {account: true}}
	kinds := []exchangeLegKind{exchangeAccountLeg, exchangeAccountLeg}
	if shape.Fee != nil {
		payerDay := shape.Sending.PostedOn
		if shape.Fee.PaidBy == conversionReceiving {
			payerDay = shape.Receiving.PostedOn
		}
		postings = append(postings, PostingInput{LineKey: shape.Fee.LineKey, AccountID: shape.Fee.AccountID, CommodityID: shape.Fee.CommodityID,
			QuantityValue: shape.Fee.QuantityValue, QuantityScale: shape.Fee.QuantityScale, PostedOn: payerDay, Memo: shape.Fee.Memo})
		roles = append(roles, postingDayRole{})
		kinds = append(kinds, exchangeFeeLeg)
		entry.FeePaidBy = shape.Fee.PaidBy
	}
	if shape.ConversionSide != "" {
		entry.EntryKind = exchangeEntryKind
		entry.ConversionSide = shape.ConversionSide
	}
	entry.Postings = postings
	return entry, roles, kinds
}

// input is the shape as a whole transaction request, for a write that
// restates it (move-day).
func (shape *TransferShape) input(base TransactionInput) TransactionInput {
	entry, _, _ := shape.entry(base.TransactionDate)
	base.JournalEntries = []JournalEntryInput{entry}
	return base
}

// rebuild applies the writer's rules to the restated request, with the
// stored system accounts, without touching the database.
func (shape *TransferShape) rebuild(transactionDate string, trading, clearing int64) ([]JournalEntryInput, error) {
	entry, roles, kinds := shape.entry(transactionDate)
	return rebuildRestatement(entry, transactionDate, roles, kinds, trading, clearing)
}

// sameJournal reports whether stored and rebuilt post exactly the same
// amounts to the same accounts on the same days, in the same entries.
func sameJournal(stored []JournalEntry, rebuilt []JournalEntryInput) bool {
	if len(stored) != len(rebuilt) {
		return false
	}
	storedDays := map[string][]string{}
	for _, entry := range stored {
		for _, posting := range entry.Postings {
			storedDays[entry.EntryDate] = append(storedDays[entry.EntryDate], postingKey(posting.AccountID, posting.CommodityID, posting.QuantityValue, posting.QuantityScale))
		}
	}
	rebuiltDays := map[string][]string{}
	for _, entry := range rebuilt {
		if entry.EntryKind != stored[0].EntryKind {
			return false
		}
		for _, posting := range entry.Postings {
			rebuiltDays[entry.EntryDate] = append(rebuiltDays[entry.EntryDate], postingKey(posting.AccountID, posting.CommodityID, posting.QuantityValue, posting.QuantityScale))
		}
	}
	if len(storedDays) != len(rebuiltDays) {
		return false
	}
	for day, keys := range storedDays {
		other := rebuiltDays[day]
		if len(keys) != len(other) {
			return false
		}
		sort.Strings(keys)
		sort.Strings(other)
		for index := range keys {
			if keys[index] != other[index] {
				return false
			}
		}
	}
	return true
}

func postingKey(accountID, commodityID int64, value exact.Coefficient, scale int) string {
	normalized := exact.ScaledIntFromCoefficient(value, scale).Normalized()
	return strings.Join([]string{strconv.FormatInt(accountID, 10), strconv.FormatInt(commodityID, 10), normalized.String()}, "|")
}
