package app

// JournalRestatementOf restates a stored transaction as the one journal entry
// the split editor shows and sends back (#481): every posting that is not a
// server-built system posting, each carrying its day as PostedOn where the
// posted-date rule would not put it there by itself, with the entry's kind,
// memo, metadata and transfer choices. It returns nil when rebuilding that
// entry with the writer's own rules — the exchange legs, the fee and
// conversion days, the transfer_clearing bridge — does not reproduce the
// stored journal exactly: a residual left in clearing, a system posting the
// writer would not build, entries that differ in more than their date. Such
// a journal is edited only in what it describes (UpdateTransactionInput's
// KeepJournal), so no edit normalizes it silently.
func JournalRestatementOf(transaction Transaction) *JournalEntryInput {
	if transaction.TransactionKind == "investment" || len(transaction.JournalEntries) == 0 {
		return nil
	}
	first := transaction.JournalEntries[0]
	exchange := first.EntryKind == exchangeEntryKind
	bridged := len(transaction.JournalEntries) > 1
	var kept []located
	var trading, clearing int64
	accountDates := map[string]bool{}
	for _, entry := range transaction.JournalEntries {
		if entry.EntryKind != first.EntryKind || entry.Memo != first.Memo || entry.MetadataJSON != first.MetadataJSON ||
			entry.ConversionSide != first.ConversionSide || entry.FeePaidBy != first.FeePaidBy {
			return nil
		}
		for _, posting := range entry.Postings {
			role := ""
			if posting.AccountSystemRole != nil {
				role = *posting.AccountSystemRole
			}
			switch {
			case role == "commodity_trading" && exchange:
				trading = posting.AccountID
			case role == "transfer_clearing" && bridged:
				clearing = posting.AccountID
			case role != "":
				return nil
			default:
				if isRestatedAccountPosting(posting) {
					accountDates[entry.EntryDate] = true
				}
				kept = append(kept, located{entry.EntryDate, posting})
			}
		}
	}

	// A posting other than an account's follows the account postings when
	// they share one day, else stays on the transaction date; only a day the
	// rule would not choose is named.
	followDate := transaction.TransactionDate
	if len(accountDates) == 1 {
		for date := range accountDates {
			followDate = date
		}
	}
	restated := JournalEntryInput{
		EntryDate: transaction.TransactionDate, EntryKind: first.EntryKind, Memo: first.Memo, MetadataJSON: first.MetadataJSON,
		ConversionSide: first.ConversionSide, FeePaidBy: first.FeePaidBy,
	}
	roles := make([]postingDayRole, 0, len(kept))
	kinds := make([]exchangeLegKind, 0, len(kept))
	for _, at := range kept {
		account := isRestatedAccountPosting(at.posting)
		postedOn := ""
		if (account && at.date != transaction.TransactionDate) || (!account && at.date != followDate) {
			postedOn = at.date
		}
		restated.Postings = append(restated.Postings, PostingInput{
			LineKey: at.posting.LineKey, AccountID: at.posting.AccountID, CommodityID: at.posting.CommodityID,
			QuantityValue: at.posting.QuantityValue, QuantityScale: at.posting.QuantityScale,
			PostedOn: postedOn, Memo: at.posting.Memo, MetadataJSON: at.posting.MetadataJSON, TagIDs: at.posting.TagIDs,
		})
		roles = append(roles, postingDayRole{account: account})
		switch {
		case account:
			kinds = append(kinds, exchangeAccountLeg)
		case at.posting.AccountClass == "expense":
			kinds = append(kinds, exchangeFeeLeg)
		case exchange:
			return nil
		default:
			kinds = append(kinds, 0)
		}
	}

	// A stored exchange without a recorded side (written before #481 kept
	// it) is checked under either.
	sides := []string{restated.ConversionSide}
	if exchange && restated.ConversionSide == "" {
		sides = []string{conversionSending, conversionReceiving}
	}
	for _, side := range sides {
		candidate := restated
		candidate.ConversionSide = side
		if rebuilt, err := rebuildRestatement(candidate, transaction.TransactionDate, roles, kinds, trading, clearing); err == nil && sameJournal(transaction.JournalEntries, rebuilt) {
			return &candidate
		}
	}
	return nil
}

func isRestatedAccountPosting(posting Posting) bool {
	return posting.AccountSystemRole == nil && (posting.AccountClass == "asset" || posting.AccountClass == "liability")
}

// rebuildRestatement applies the writer's rules to a restated entry with the
// stored system accounts, without touching the database.
func rebuildRestatement(entry JournalEntryInput, transactionDate string, roles []postingDayRole, kinds []exchangeLegKind, trading, clearing int64) ([]JournalEntryInput, error) {
	roles = append([]postingDayRole{}, roles...)
	if entry.EntryKind == exchangeEntryKind {
		if trading == 0 {
			return nil, ValidationError{Message: "the stored exchange has no trading legs to rebuild"}
		}
		built, err := exchangeLegs(entry, transactionDate, kinds, trading)
		if err != nil {
			return nil, err
		}
		for range built.Postings[len(roles):] {
			roles = append(roles, postingDayRole{system: true})
		}
		entry = built
	}
	return groupPostingsByDay(entry, transactionDate, roles, func() (int64, error) {
		if clearing == 0 {
			return 0, ValidationError{Message: "the stored journal has no bridge to rebuild"}
		}
		return clearing, nil
	})
}
