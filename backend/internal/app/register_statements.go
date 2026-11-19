package app

import (
	"context"
	"fmt"

	"rekenraam/backend/internal/db"
)

// registerPage is one register page after postings and statement rows are
// merged into register order and cut to the page limit (#595).
type registerPage struct {
	Postings   []db.AccountRegisterEntryRecord
	Statements []registerPageStatement
	NextCursor string
}

type registerPageStatement struct {
	Record           db.RegisterStatementRecord
	BeforeEntryIndex int
}

// registerStatementIsNewer reports whether a statement row sorts above
// (is newer than) a posting in the register. The register key is
// (date, sequence, kind, id) descending with kind 1 for a statement and 0 for
// a posting, so a statement at its boundary (statement_date,
// statement_account_sequence) sits above every posting the boundary covers —
// the same rule the reconciliation guard uses — and below every later one.
func registerStatementIsNewer(statement db.RegisterStatementRecord, posting db.AccountRegisterEntryRecord) bool {
	if statement.StatementDate != posting.JournalEntry.EntryDate {
		return statement.StatementDate > posting.JournalEntry.EntryDate
	}
	return statement.StatementAccountSequence >= posting.Posting.AccountDaySequence
}

// mergeRegisterPage merges one limit-sized read of postings and one of
// statement rows, both in register order after the same cursor, and keeps the
// first limit rows. Each source holds at least every row of its kind that can
// fall in the kept range, so the cut is exact. The next cursor is the last kept
// row of either kind; it is empty when both sources ran short of the limit
// (nothing remains).
func mergeRegisterPage(postings []db.AccountRegisterEntryRecord, statements []db.RegisterStatementRecord, limit int) registerPage {
	page := registerPage{}
	postingIndex, statementIndex := 0, 0
	lastWasStatement := false
	for postingIndex+statementIndex < limit && (postingIndex < len(postings) || statementIndex < len(statements)) {
		takeStatement := statementIndex < len(statements) &&
			(postingIndex >= len(postings) || registerStatementIsNewer(statements[statementIndex], postings[postingIndex]))
		if takeStatement {
			page.Statements = append(page.Statements, registerPageStatement{
				Record:           statements[statementIndex],
				BeforeEntryIndex: postingIndex,
			})
			statementIndex++
		} else {
			page.Postings = append(page.Postings, postings[postingIndex])
			postingIndex++
		}
		lastWasStatement = takeStatement
	}
	if len(postings) < limit && len(statements) < limit && postingIndex == len(postings) && statementIndex == len(statements) {
		return page
	}
	if lastWasStatement {
		last := page.Statements[len(page.Statements)-1].Record
		page.NextCursor = db.EncodeRegisterStatementCursor(last.StatementDate, last.StatementAccountSequence, last.CheckpointID)
	} else if len(page.Postings) > 0 {
		last := page.Postings[len(page.Postings)-1]
		page.NextCursor = db.EncodeRegisterCursor(last.JournalEntry.EntryDate, last.Posting.AccountDaySequence, last.Posting.ID)
	}
	return page
}

// registerStatements resolves the page's statement rows for the response,
// with the commodity code and symbol each balance is shown in.
func (s *TransactionService) registerStatements(ctx context.Context, rows []registerPageStatement) ([]AccountRegisterStatement, error) {
	statements := make([]AccountRegisterStatement, 0, len(rows))
	if len(rows) == 0 {
		return statements, nil
	}
	commodityIDs := make([]int64, 0, len(rows))
	seen := map[int64]bool{}
	for _, row := range rows {
		if !seen[row.Record.CommodityID] {
			seen[row.Record.CommodityID] = true
			commodityIDs = append(commodityIDs, row.Record.CommodityID)
		}
	}
	commodities, err := s.commodityRepository.CommoditiesByIDs(ctx, BookID, commodityIDs)
	if err != nil {
		return nil, fmt.Errorf("register statements: commodities lookup: %w", err)
	}
	for _, row := range rows {
		record := row.Record
		commodity, ok := commodities[record.CommodityID]
		if !ok {
			return nil, fmt.Errorf("register statements: commodity %d not found", record.CommodityID)
		}
		statement := AccountRegisterStatement{
			CheckpointID:             record.CheckpointID,
			CommodityID:              record.CommodityID,
			CommodityCode:            commodity.Code,
			StatementDate:            record.StatementDate,
			StatementAccountSequence: record.StatementAccountSequence,
			StatementBalance:         reconciliationQuantity(record.CommodityID, record.StatementBalanceValue, record.StatementBalanceScale),
			BeforeEntryIndex:         row.BeforeEntryIndex,
		}
		if commodity.DisplaySymbol != "" {
			symbol := commodity.DisplaySymbol
			statement.CommoditySymbol = &symbol
		}
		statements = append(statements, statement)
	}
	return statements, nil
}
