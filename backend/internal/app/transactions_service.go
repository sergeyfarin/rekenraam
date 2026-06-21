package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"rekenraam/backend/internal/db"
)

type TransactionService struct {
	repository          *db.TransactionRepository
	payeeRepository     *db.PayeeRepository
	accountRepository   *db.AccountRepository
	commodityRepository *db.CommodityRepository
	now                 func() time.Time
	newLineKey          func() string
}

func NewTransactionService(
	repository *db.TransactionRepository,
	payeeRepository *db.PayeeRepository,
	accountRepository *db.AccountRepository,
	commodityRepository *db.CommodityRepository,
) *TransactionService {
	return &TransactionService{
		repository:          repository,
		payeeRepository:     payeeRepository,
		accountRepository:   accountRepository,
		commodityRepository: commodityRepository,
		now:                 time.Now,
		newLineKey:          uuid.NewString,
	}
}

func (s *TransactionService) ListTransactions(ctx context.Context, input ListTransactionsInput) (ListTransactionsResult, error) {
	params, err := s.listParams(ctx, input, false)
	if err != nil {
		return ListTransactionsResult{}, err
	}

	records, err := s.repository.ListTransactions(ctx, params)
	if err != nil {
		return ListTransactionsResult{}, fmt.Errorf("list transactions: %w", err)
	}

	nextCursor := ""
	if len(records) == params.Limit {
		last := records[len(records)-1]
		nextCursor = db.EncodeTransactionCursor(last.TransactionDate, last.TransactionDaySequence, last.ID)
	}

	txns := toTransactions(records)
	if err := s.enrichPostings(ctx, txns); err != nil {
		return ListTransactionsResult{}, err
	}

	return ListTransactionsResult{
		Transactions: txns,
		NextCursor:   nextCursor,
	}, nil
}

func (s *TransactionService) Register(ctx context.Context, accountID int64, input ListTransactionsInput) (AccountRegisterResult, error) {
	if accountID <= 0 {
		return AccountRegisterResult{}, ValidationError{Message: "account id is required"}
	}
	input.AccountID = accountID
	if strings.TrimSpace(input.Status) == "" {
		input.Status = "posted"
	}

	params, err := s.listParams(ctx, input, true)
	if err != nil {
		return AccountRegisterResult{}, err
	}

	records, err := s.repository.AccountRegister(ctx, params)
	if err != nil {
		return AccountRegisterResult{}, fmt.Errorf("list register: %w", err)
	}
	runningBalances, err := s.accountRegisterRunningBalances(ctx, params, records)
	if err != nil {
		return AccountRegisterResult{}, err
	}

	nextCursor := ""
	if len(records) == params.Limit {
		last := records[len(records)-1]
		nextCursor = db.EncodeRegisterCursor(last.JournalEntry.EntryDate, last.Posting.AccountDaySequence, last.Posting.ID)
	}

	entries := toAccountRegisterEntries(records, runningBalances)
	if err := s.enrichRegisterPostings(ctx, entries); err != nil {
		return AccountRegisterResult{}, err
	}

	return AccountRegisterResult{
		Entries:    entries,
		NextCursor: nextCursor,
	}, nil
}

func (s *TransactionService) Transaction(ctx context.Context, transactionID int64) (Transaction, error) {
	if transactionID <= 0 {
		return Transaction{}, ValidationError{Message: "transaction id is required"}
	}

	record, err := s.repository.TransactionByID(ctx, BookID, transactionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Transaction{}, ErrTransactionNotFound
		}
		return Transaction{}, fmt.Errorf("read transaction: %w", err)
	}

	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) enrichOne(ctx context.Context, txn Transaction) (Transaction, error) {
	slice := []Transaction{txn}
	if err := s.enrichPostings(ctx, slice); err != nil {
		return Transaction{}, err
	}
	return slice[0], nil
}

// enrichPostings performs one bulk account lookup and one bulk commodity lookup
// across all postings in the given transactions, then fills the enriched fields
// on each Posting in memory. It must be called before any response path returns
// transaction data to callers. A missing account or commodity is an internal error.
func (s *TransactionService) enrichPostings(ctx context.Context, transactions []Transaction) error {
	accountIDs := make(map[int64]struct{})
	commodityIDs := make(map[int64]struct{})
	for i := range transactions {
		for j := range transactions[i].JournalEntries {
			for k := range transactions[i].JournalEntries[j].Postings {
				accountIDs[transactions[i].JournalEntries[j].Postings[k].AccountID] = struct{}{}
				commodityIDs[transactions[i].JournalEntries[j].Postings[k].CommodityID] = struct{}{}
			}
		}
	}

	flatAccountIDs := make([]int64, 0, len(accountIDs))
	for id := range accountIDs {
		flatAccountIDs = append(flatAccountIDs, id)
	}
	flatCommodityIDs := make([]int64, 0, len(commodityIDs))
	for id := range commodityIDs {
		flatCommodityIDs = append(flatCommodityIDs, id)
	}

	accountMap, err := s.accountRepository.AccountsByIDs(ctx, BookID, flatAccountIDs)
	if err != nil {
		return fmt.Errorf("enrich postings: accounts lookup: %w", err)
	}
	commodityMap, err := s.commodityRepository.CommoditiesByIDs(ctx, BookID, flatCommodityIDs)
	if err != nil {
		return fmt.Errorf("enrich postings: commodities lookup: %w", err)
	}

	for i := range transactions {
		for j := range transactions[i].JournalEntries {
			for k := range transactions[i].JournalEntries[j].Postings {
				p := &transactions[i].JournalEntries[j].Postings[k]

				acct, ok := accountMap[p.AccountID]
				if !ok {
					return fmt.Errorf("enrich postings: account %d not found", p.AccountID)
				}
				if acct.Name.Valid {
					name := acct.Name.String
					p.AccountName = &name
				}
				if acct.Code.Valid {
					code := acct.Code.String
					p.AccountCode = &code
				}
				if acct.SystemRole.Valid {
					role := acct.SystemRole.String
					p.AccountSystemRole = &role
				}
				if acct.BuiltinKey.Valid {
					key := acct.BuiltinKey.String
					p.AccountBuiltinKey = &key
				}
				p.AccountClass = acct.AccountClass

				comm, ok := commodityMap[p.CommodityID]
				if !ok {
					return fmt.Errorf("enrich postings: commodity %d not found", p.CommodityID)
				}
				p.CommodityCode = comm.Code
				if comm.DisplaySymbol != "" {
					sym := comm.DisplaySymbol
					p.CommoditySymbol = &sym
				}
			}
		}
	}

	return nil
}

// enrichRegisterPostings is the AccountRegisterEntry equivalent of enrichPostings.
func (s *TransactionService) enrichRegisterPostings(ctx context.Context, entries []AccountRegisterEntry) error {
	accountIDs := make(map[int64]struct{})
	commodityIDs := make(map[int64]struct{})
	for i := range entries {
		accountIDs[entries[i].Posting.AccountID] = struct{}{}
		commodityIDs[entries[i].Posting.CommodityID] = struct{}{}
	}

	flatAccountIDs := make([]int64, 0, len(accountIDs))
	for id := range accountIDs {
		flatAccountIDs = append(flatAccountIDs, id)
	}
	flatCommodityIDs := make([]int64, 0, len(commodityIDs))
	for id := range commodityIDs {
		flatCommodityIDs = append(flatCommodityIDs, id)
	}

	accountMap, err := s.accountRepository.AccountsByIDs(ctx, BookID, flatAccountIDs)
	if err != nil {
		return fmt.Errorf("enrich register postings: accounts lookup: %w", err)
	}
	commodityMap, err := s.commodityRepository.CommoditiesByIDs(ctx, BookID, flatCommodityIDs)
	if err != nil {
		return fmt.Errorf("enrich register postings: commodities lookup: %w", err)
	}

	for i := range entries {
		p := &entries[i].Posting

		acct, ok := accountMap[p.AccountID]
		if !ok {
			return fmt.Errorf("enrich register postings: account %d not found", p.AccountID)
		}
		if acct.Name.Valid {
			name := acct.Name.String
			p.AccountName = &name
		}
		if acct.Code.Valid {
			code := acct.Code.String
			p.AccountCode = &code
		}
		if acct.SystemRole.Valid {
			role := acct.SystemRole.String
			p.AccountSystemRole = &role
		}
		if acct.BuiltinKey.Valid {
			key := acct.BuiltinKey.String
			p.AccountBuiltinKey = &key
		}
		p.AccountClass = acct.AccountClass

		comm, ok := commodityMap[p.CommodityID]
		if !ok {
			return fmt.Errorf("enrich register postings: commodity %d not found", p.CommodityID)
		}
		p.CommodityCode = comm.Code
		if comm.DisplaySymbol != "" {
			sym := comm.DisplaySymbol
			p.CommoditySymbol = &sym
		}
	}

	return nil
}

func toTransactions(records []db.TransactionRecord) []Transaction {
	transactions := make([]Transaction, 0, len(records))
	for _, record := range records {
		transactions = append(transactions, toTransaction(record))
	}
	return transactions
}

func toAccountRegisterEntries(records []db.AccountRegisterEntryRecord, runningBalances map[int64]BalanceQuantity) []AccountRegisterEntry {
	entries := make([]AccountRegisterEntry, 0, len(records))
	for _, record := range records {
		transaction := toTransaction(record.Transaction)
		posting := Posting{
			ID:                   record.Posting.ID,
			PostingLineID:        record.Posting.PostingLineID,
			LineKey:              record.Posting.LineKey,
			LineSeq:              record.Posting.LineSeq,
			AccountID:            record.Posting.AccountID,
			AccountDaySequence:   record.Posting.AccountDaySequence,
			QuantityValue:        record.Posting.QuantityValue,
			QuantityScale:        record.Posting.QuantityScale,
			CommodityID:          record.Posting.CommodityID,
			Memo:                 record.Posting.Memo,
			ReconciliationStatus: record.Posting.ReconciliationStatus,
			ClearedOn:            nullableString(record.Posting.ClearedOn),
			MetadataJSON:         record.Posting.MetadataJSON,
			TagIDs:               record.Posting.TagIDs,
		}
		entries = append(entries, AccountRegisterEntry{
			TransactionID:             transaction.ID,
			BookID:                    transaction.BookID,
			CorrectionOfTransactionID: transaction.CorrectionOfTransactionID,
			Status:                    transaction.Status,
			TransactionKind:           transaction.TransactionKind,
			TransactionDate:           transaction.TransactionDate,
			PayeeID:                   transaction.PayeeID,
			PayeeName:                 transaction.PayeeName,
			Description:               transaction.Description,
			ExternalRefHint:           transaction.ExternalRefHint,
			NeedsReview:               transaction.NeedsReview,
			VersionID:                 transaction.VersionID,
			VersionSeq:                transaction.VersionSeq,
			SupersedesVersionID:       transaction.SupersedesVersionID,
			TransactionTagIDs:         transaction.TagIDs,
			JournalEntryID:            record.JournalEntry.ID,
			EntrySeq:                  record.JournalEntry.EntrySeq,
			EntryDate:                 record.JournalEntry.EntryDate,
			EntryKind:                 record.JournalEntry.EntryKind,
			EntryMemo:                 record.JournalEntry.Memo,
			Posting:                   posting,
			RunningBalance:            runningBalances[record.Posting.ID],
			CreatedAt:                 transaction.CreatedAt,
			UpdatedAt:                 transaction.UpdatedAt,
			ChangeReason:              transaction.ChangeReason,
		})
	}
	return entries
}

func toTransaction(record db.TransactionRecord) Transaction {
	entries := make([]JournalEntry, 0, len(record.JournalEntries))
	for _, entry := range record.JournalEntries {
		postings := make([]Posting, 0, len(entry.Postings))
		for _, posting := range entry.Postings {
			postings = append(postings, Posting{
				ID:                   posting.ID,
				PostingLineID:        posting.PostingLineID,
				LineKey:              posting.LineKey,
				LineSeq:              posting.LineSeq,
				AccountID:            posting.AccountID,
				AccountDaySequence:   posting.AccountDaySequence,
				QuantityValue:        posting.QuantityValue,
				QuantityScale:        posting.QuantityScale,
				CommodityID:          posting.CommodityID,
				Memo:                 posting.Memo,
				ReconciliationStatus: posting.ReconciliationStatus,
				ClearedOn:            nullableString(posting.ClearedOn),
				MetadataJSON:         posting.MetadataJSON,
				TagIDs:               posting.TagIDs,
			})
		}
		entries = append(entries, JournalEntry{
			ID:           entry.ID,
			EntrySeq:     entry.EntrySeq,
			EntryDate:    entry.EntryDate,
			EntryKind:    entry.EntryKind,
			Memo:         entry.Memo,
			MetadataJSON: entry.MetadataJSON,
			Postings:     postings,
		})
	}

	return Transaction{
		ID:                        record.ID,
		BookID:                    record.BookID,
		CorrectionOfTransactionID: nullableInt64FromSQL(record.CorrectionOfTransactionID),
		Status:                    record.Status,
		TransactionKind:           record.TransactionKind,
		TransactionDate:           record.TransactionDate,
		TransactionDaySequence:    record.TransactionDaySequence,
		PayeeID:                   nullableInt64FromSQL(record.PayeeID),
		PayeeName:                 nullableString(record.PayeeName),
		Description:               record.Description,
		ExternalRefHint:           nullableString(record.ExternalRefHint),
		NoteMarkdown:              record.NoteMarkdown,
		MetadataJSON:              record.MetadataJSON,
		NeedsReview:               record.NeedsReview,
		VersionID:                 record.VersionID,
		VersionSeq:                record.VersionSeq,
		SupersedesVersionID:       nullableInt64FromSQL(record.SupersedesVersionID),
		TagIDs:                    record.TagIDs,
		JournalEntries:            entries,
		CreatedAt:                 record.CreatedAt,
		UpdatedAt:                 record.RecordedAt,
		DeletedAt:                 nullableString(record.DeletedAt),
		ChangeReason:              record.ChangeReason,
		InvalidatedCheckpointIDs:  record.InvalidatedCheckpointIDs,
	}
}
