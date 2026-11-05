package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ImportMatchWindowDays is how many days a statement row's date may lie
// before or after a posting's posted_on day and still match it (#485,
// owner decision 2026-11-01). The window is inclusive at both ends.
const ImportMatchWindowDays = 5

// Match decisions a row's resolution records. A row is linked only to the
// posting the owner chose; a row the owner keeps as new commits as today.
const (
	ImportMatchDecisionLink = "link"
	ImportMatchDecisionNew  = "new"
)

// Hold reasons for a CSV or QIF row whose match the owner must decide again.
const (
	importMatchReviewRequiredReason = "match review required"
	importMatchStaleReason          = "matched posting changed after review"
)

// ImportMatchChoice is the owner's decision on a row that matches postings
// already in the ledger. A link names the posting by its transaction and
// stable line key, plus the posted_on day the owner saw: the commit links
// only while that posting still has the reviewed facts.
type ImportMatchChoice struct {
	Decision      string `json:"decision"`
	TransactionID int64  `json:"transaction_id,omitempty"`
	LineKey       string `json:"line_key,omitempty"`
	PostedOn      string `json:"posted_on,omitempty"`
}

// ImportMatchCandidate is one existing posting a staged row matches, with
// what made it match: the same account and commodity, the exact amount, and
// the day offset of its posted_on day from the row's date.
type ImportMatchCandidate struct {
	TransactionID   int64
	LineKey         string
	TransactionDate string
	TransactionKind string
	PostedOn        string
	// DayOffset is posted_on minus the row's date, in days.
	DayOffset     int
	AccountID     int64
	CommodityID   int64
	QuantityValue exact.Coefficient
	QuantityScale int
	PayeeName     string
	Description   string
}

// RowMatchReview is a pending row with match candidates, or with a link
// choice that no longer stands, as preview reports it.
type RowMatchReview struct {
	RowID      int64
	RowIndex   int
	Candidates []ImportMatchCandidate
	Choice     *ImportMatchChoice
	// NeedsDecision is true while commit would hold the row: no decision
	// yet, or a link to a posting that is no longer a candidate.
	NeedsDecision bool
}

// importMatchRow is the part of a staged row the match rule reads.
type importMatchRow struct {
	date   string
	amount *exact.ScaledInt
}

// importMatchEligible reads a staged row the match rule applies to: a CSV or
// QIF bank row resolved to an account and commodity, with a canonical date
// and amount. Any other row never matches.
func importMatchEligible(batch db.ImportBatchRecord, row db.ImportStagedRowRecord, resolution ImportRowResolution) (importMatchRow, bool) {
	if !accountScopedImportIdentity(batch.SourceKind, isCSVInvestmentRow(batch, row)) || resolution.AccountID == 0 || resolution.CommodityID == 0 {
		return importMatchRow{}, false
	}
	if refuseStagedParseErrors(row.NormalizedJSON) != nil {
		return importMatchRow{}, false
	}
	var normalized struct {
		Date   string `json:"date"`
		Amount string `json:"amount"`
	}
	if err := json.Unmarshal([]byte(row.NormalizedJSON), &normalized); err != nil {
		return importMatchRow{}, false
	}
	if _, err := parseCanonicalDate(normalized.Date); err != nil {
		return importMatchRow{}, false
	}
	coefficient, scale, err := parseCanonicalAmount(normalized.Amount)
	if err != nil {
		return importMatchRow{}, false
	}
	return importMatchRow{date: normalized.Date, amount: exact.ScaledIntFromCoefficient(coefficient, scale)}, true
}

// importMatchDayOffset is posted_on minus the row date in days, and whether
// it lies within the window.
func importMatchDayOffset(rowDate, postedOn string) (int, bool) {
	row, err := time.Parse(time.DateOnly, rowDate)
	if err != nil {
		return 0, false
	}
	posted, err := time.Parse(time.DateOnly, postedOn)
	if err != nil {
		return 0, false
	}
	offset := int(posted.Sub(row).Hours() / 24)
	return offset, offset >= -ImportMatchWindowDays && offset <= ImportMatchWindowDays
}

// importPostingMatches is the one rule CSV and QIF share (#485): the same
// account and commodity, the exact amount at any scale, and a posting day
// within the window. Linked postings never reach it.
func importPostingMatches(row importMatchRow, resolution ImportRowResolution, posting db.ImportPostingCandidateRecord) (int, bool) {
	if posting.Linked || posting.AccountID != resolution.AccountID || posting.CommodityID != resolution.CommodityID {
		return 0, false
	}
	amount := exact.ScaledIntFromCoefficient(exact.Coefficient(posting.QuantityValue), posting.QuantityScale)
	if amount.Cmp(row.amount) != 0 {
		return 0, false
	}
	return importMatchDayOffset(row.date, posting.PostedOn)
}

// importMatchCandidates lists the postings a row matches now. ok is false
// for a row the rule does not apply to.
func (s *ImportService) importMatchCandidates(ctx context.Context, batch db.ImportBatchRecord, row db.ImportStagedRowRecord, resolution ImportRowResolution) ([]ImportMatchCandidate, bool, error) {
	matchRow, ok := importMatchEligible(batch, row, resolution)
	if !ok {
		return nil, false, nil
	}
	date, _ := time.Parse(time.DateOnly, matchRow.date)
	postings, err := s.repository.ListImportPostingCandidates(ctx, BookID, resolution.AccountID, resolution.CommodityID,
		date.AddDate(0, 0, -ImportMatchWindowDays).Format(time.DateOnly), date.AddDate(0, 0, ImportMatchWindowDays).Format(time.DateOnly))
	if err != nil {
		return nil, true, err
	}
	var candidates []ImportMatchCandidate
	for _, posting := range postings {
		offset, matches := importPostingMatches(matchRow, resolution, posting)
		if !matches {
			continue
		}
		candidates = append(candidates, ImportMatchCandidate{
			TransactionID: posting.TransactionID, LineKey: posting.LineKey,
			TransactionDate: posting.TransactionDate, TransactionKind: posting.TransactionKind,
			PostedOn: posting.PostedOn, DayOffset: offset,
			AccountID: posting.AccountID, CommodityID: posting.CommodityID,
			QuantityValue: exact.Coefficient(posting.QuantityValue), QuantityScale: posting.QuantityScale,
			PayeeName: posting.PayeeName, Description: posting.Description,
		})
	}
	return candidates, true, nil
}

// importMatchReview decides what preview shows for a pending row and what
// commit does with it: nil when the row matches nothing and has no link
// choice, so it commits as today.
func (s *ImportService) importMatchReview(ctx context.Context, batch db.ImportBatchRecord, row db.ImportStagedRowRecord, resolution ImportRowResolution) (*RowMatchReview, error) {
	candidates, eligible, err := s.importMatchCandidates(ctx, batch, row, resolution)
	if err != nil {
		return nil, fmt.Errorf("match row %d: %w", row.ID, err)
	}
	choice := resolution.Match
	if choice != nil && choice.Decision == ImportMatchDecisionNew {
		if len(candidates) == 0 {
			return nil, nil
		}
		return &RowMatchReview{RowID: row.ID, RowIndex: row.RowIndex, Candidates: candidates, Choice: choice}, nil
	}
	if !eligible || (len(candidates) == 0 && choice == nil) {
		if choice != nil && !eligible {
			// A link choice on a row the rule no longer applies to (its
			// account was changed, say) is decided again.
			return &RowMatchReview{RowID: row.ID, RowIndex: row.RowIndex, Choice: choice, NeedsDecision: true}, nil
		}
		return nil, nil
	}
	review := &RowMatchReview{RowID: row.ID, RowIndex: row.RowIndex, Candidates: candidates, Choice: choice, NeedsDecision: true}
	if choice != nil {
		for _, candidate := range candidates {
			if candidate.TransactionID == choice.TransactionID && candidate.LineKey == choice.LineKey && candidate.PostedOn == choice.PostedOn {
				review.NeedsDecision = false
			}
		}
	}
	return review, nil
}

// linkImportedRow commits a row as a link to the posting its choice names.
// The write transaction rereads the posting and links only while it is still
// posted, unlinked and has the reviewed facts; otherwise nothing is written
// and the caller holds the row (ErrImportPostingLinkStale).
func (s *ImportService) linkImportedRow(ctx context.Context, batch db.ImportBatchRecord, row db.ImportStagedRowRecord, resolution ImportRowResolution, commitKey string, input CommitImportBatchInput, nowStr string) error {
	choice := resolution.Match
	matchRow, ok := importMatchEligible(batch, row, resolution)
	if !ok || choice == nil || choice.Decision != ImportMatchDecisionLink {
		return db.ErrImportPostingLinkStale
	}
	return s.repository.LinkImportedPosting(ctx, db.LinkImportedPostingParams{
		Identity: db.CreateImportCommitIdentityParams{
			BookID:            BookID,
			DedupeFingerprint: commitKey,
			SourceFingerprint: row.DedupeFingerprint,
			SourceKind:        batch.SourceKind,
			AccountID:         resolution.AccountID,
			CreatedAt:         nowStr,
		},
		RowID:         row.ID,
		TransactionID: choice.TransactionID,
		LineKey:       choice.LineKey,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
	}, func(posting db.ImportPostingCandidateRecord, found bool) error {
		if !found || posting.PostedOn != choice.PostedOn {
			return db.ErrImportPostingLinkStale
		}
		if _, matches := importPostingMatches(matchRow, resolution, posting); !matches {
			return db.ErrImportPostingLinkStale
		}
		return nil
	})
}

// holdImportStagedRowForMatchReview keeps a row pending with the reason the
// owner must decide its match, so another preview shows the current
// candidates.
func (s *ImportService) holdImportStagedRowForMatchReview(ctx context.Context, result *CommitImportBatchResult, rowID int64, reason string) error {
	if err := s.repository.CommitImportStagedRow(ctx, db.CommitImportStagedRowParams{
		RowID:        rowID,
		CommitStatus: "pending",
		CommitError:  sql.NullString{String: reason, Valid: true},
	}); err != nil {
		if errors.Is(err, db.ErrImportStagedRowAlreadyCommitted) {
			return s.recordImportStagedRowTerminal(ctx, result, db.CommitImportStagedRowParams{RowID: rowID, CommitStatus: "pending"}, nil)
		}
		return err
	}
	result.MatchReviewRowIDs = append(result.MatchReviewRowIDs, rowID)
	return nil
}

// validateImportMatchChoice refuses a malformed match decision in a review
// patch; whether a link still stands is decided at preview and commit.
func validateImportMatchChoice(resolutionJSON string) error {
	resolution, err := parseResolutionJSON(resolutionJSON)
	if err != nil || resolution.Match == nil {
		return nil
	}
	choice := resolution.Match
	switch choice.Decision {
	case ImportMatchDecisionNew:
		return nil
	case ImportMatchDecisionLink:
		if choice.TransactionID <= 0 || choice.LineKey == "" {
			return ValidationError{Message: "a match link names a transaction and a line key"}
		}
		if _, err := parseCanonicalDate(choice.PostedOn); err != nil {
			return ValidationError{Message: "a match link names the posted_on day it was reviewed with"}
		}
		return nil
	default:
		return ValidationError{Message: "match decision is invalid"}
	}
}

// commitMatchedImportRow settles a CSV or QIF row against the match rule.
// handled is false when the row commits as a new transaction: it matches
// nothing, or the owner kept it as new.
func (s *ImportService) commitMatchedImportRow(ctx context.Context, result *CommitImportBatchResult, batch db.ImportBatchRecord, row db.ImportStagedRowRecord, resolution ImportRowResolution, commitKey string, input CommitImportBatchInput, nowStr string) (bool, error) {
	choice := resolution.Match
	if choice != nil && choice.Decision == ImportMatchDecisionNew {
		return false, nil
	}
	if choice == nil {
		candidates, _, err := s.importMatchCandidates(ctx, batch, row, resolution)
		if err != nil {
			return true, fmt.Errorf("match row %d: %w", row.ID, err)
		}
		if len(candidates) == 0 {
			return false, nil
		}
		// Never linked or posted beside a match the owner has not seen.
		if err := s.holdImportStagedRowForMatchReview(ctx, result, row.ID, importMatchReviewRequiredReason); err != nil {
			return true, fmt.Errorf("hold unmatched-review row %d: %w", row.ID, err)
		}
		return true, nil
	}
	err := s.linkImportedRow(ctx, batch, row, resolution, commitKey, input, nowStr)
	switch {
	case err == nil:
		result.CommittedCount++
		result.LinkedCount++
	case errors.Is(err, db.ErrImportPostingLinkStale):
		if err := s.holdImportStagedRowForMatchReview(ctx, result, row.ID, importMatchStaleReason); err != nil {
			return true, fmt.Errorf("hold stale match row %d: %w", row.ID, err)
		}
	case errors.Is(err, db.ErrCommitIdentityConflict):
		// A concurrent commit of this row won; adopt its outcome.
		if err := s.resolveConcurrentImportCommit(ctx, row, commitKey); err != nil {
			return true, fmt.Errorf("resolve concurrent link row %d: %w", row.ID, err)
		}
		result.CommittedCount++
	default:
		return true, fmt.Errorf("link row %d: %w", row.ID, err)
	}
	return true, nil
}

// --- Statement link corrections (#485 review) ---

var (
	// ErrImportStatementLinkNotFound means no statement row with that
	// identity was ever linked to a posting.
	ErrImportStatementLinkNotFound = errors.New("statement link not found")
	// ErrImportStatementLinkStale means the row's latest link is not the one
	// the owner reviewed, or the posting it would move to no longer matches.
	// Nothing was written.
	ErrImportStatementLinkStale = errors.New("statement link changed after review")
)

// ImportStatementLink is a committed statement row's latest link: where it
// points, whether it is still active, and the row it records.
type ImportStatementLink struct {
	IdentityID    int64
	LinkID        int64
	TransactionID int64
	LineKey       string
	AccountID     int64
	CommodityID   int64
	Active        bool
	LinkedAt      string
	RetiredAt     string
	RetireReason  string
	SourceKind    string
	BatchID       int64
	Filename      string
	RowDate       string
	RowAmount     exact.Coefficient
	RowScale      int
	RowPayee      string
}

func toImportStatementLink(record db.ImportStatementLinkRecord) ImportStatementLink {
	link := ImportStatementLink{
		IdentityID: record.IdentityID, LinkID: record.LinkID, TransactionID: record.TransactionID,
		LineKey: record.LineKey, AccountID: record.AccountID, Active: record.Active,
		LinkedAt: record.LinkedAt, RetiredAt: record.RetiredAt, RetireReason: record.RetireReason,
		SourceKind: record.SourceKind, BatchID: record.BatchID, Filename: record.Filename,
	}
	var normalized struct {
		Date      string `json:"date"`
		Amount    string `json:"amount"`
		PayeeHint string `json:"payee_hint"`
	}
	if json.Unmarshal([]byte(record.NormalizedJSON), &normalized) == nil {
		link.RowDate, link.RowPayee = normalized.Date, normalized.PayeeHint
		if coefficient, scale, err := parseCanonicalAmount(normalized.Amount); err == nil {
			link.RowAmount, link.RowScale = coefficient, scale
		}
	}
	if resolution, err := parseResolutionJSON(record.ResolutionJSON); err == nil {
		link.CommodityID = resolution.CommodityID
	}
	return link
}

// statementLinkMatchInput restates a linked row for the match rule: its
// staged facts, resolved to the identity's account.
func statementLinkMatchInput(record db.ImportStatementLinkRecord) (db.ImportBatchRecord, db.ImportStagedRowRecord, ImportRowResolution) {
	resolution, _ := parseResolutionJSON(record.ResolutionJSON)
	resolution.AccountID = record.AccountID
	return db.ImportBatchRecord{SourceKind: record.SourceKind}, db.ImportStagedRowRecord{NormalizedJSON: record.NormalizedJSON}, resolution
}

// StatementLinks lists the statement rows whose latest link names a line of
// the transaction, active or since unlinked.
func (s *ImportService) StatementLinks(ctx context.Context, transactionID int64) ([]ImportStatementLink, error) {
	records, err := s.repository.ImportStatementLinksForTransaction(ctx, BookID, transactionID)
	if err != nil {
		return nil, err
	}
	links := make([]ImportStatementLink, 0, len(records))
	for _, record := range records {
		links = append(links, toImportStatementLink(record))
	}
	return links, nil
}

func (s *ImportService) statementLinkRecord(ctx context.Context, identityID int64) (db.ImportStatementLinkRecord, error) {
	record, err := s.repository.ImportStatementLinkByIdentity(ctx, BookID, identityID)
	if errors.Is(err, db.ErrNotFound) {
		return db.ImportStatementLinkRecord{}, ErrImportStatementLinkNotFound
	}
	return record, err
}

// StatementLinkCandidates lists the postings a linked row could move to, by
// the same rule that matched it: its account and commodity, its exact
// amount, within the window, not linked to another row.
func (s *ImportService) StatementLinkCandidates(ctx context.Context, identityID int64) (ImportStatementLink, []ImportMatchCandidate, error) {
	record, err := s.statementLinkRecord(ctx, identityID)
	if err != nil {
		return ImportStatementLink{}, nil, err
	}
	batch, row, resolution := statementLinkMatchInput(record)
	candidates, _, err := s.importMatchCandidates(ctx, batch, row, resolution)
	if err != nil {
		return ImportStatementLink{}, nil, err
	}
	return toImportStatementLink(record), candidates, nil
}

// CorrectStatementLinkInput names a statement row, the link the owner
// reviewed and why it changes; a reassignment also names the posting it
// moves to and the posted_on day reviewed.
type CorrectStatementLinkInput struct {
	OwnerUserID    int64
	AuthSessionID  int64
	RequestID      string
	IdentityID     int64
	ExpectedLinkID int64
	// ExpectedLinkActive is whether the owner reviewed that link as active.
	// Unlinking retires a link without changing its ID, so the ID alone does
	// not bind what was reviewed.
	ExpectedLinkActive *bool
	Reason             string
	TransactionID      int64
	LineKey            string
	PostedOn           string
}

func (input CorrectStatementLinkInput) params(occurredAt string) db.ImportPostingLinkCorrectionParams {
	return db.ImportPostingLinkCorrectionParams{
		BookID: BookID, IdentityID: input.IdentityID, ExpectedLinkID: input.ExpectedLinkID,
		ExpectedLinkActive: input.ExpectedLinkActive != nil && *input.ExpectedLinkActive,
		Reason:             strings.TrimSpace(input.Reason), TransactionID: input.TransactionID, LineKey: input.LineKey,
		ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID, OccurredAt: occurredAt,
	}
}

func validateStatementLinkCorrection(input CorrectStatementLinkInput) error {
	if input.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}
	if input.ExpectedLinkID <= 0 || input.ExpectedLinkActive == nil {
		return ValidationError{Message: "expected_link_id and expected_link_active are required"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return ValidationError{Message: "a reason is required"}
	}
	return nil
}

func statementLinkCorrectionError(err error) error {
	switch {
	case errors.Is(err, db.ErrImportPostingLinkStale):
		return ErrImportStatementLinkStale
	case errors.Is(err, db.ErrNotFound):
		return ErrImportStatementLinkNotFound
	}
	return err
}

// UnlinkStatementRow retires a row's active link. Nothing in the ledger
// changes; the link and its retirement stay as evidence, and the row keeps
// its identity, so it can be linked again and re-importing it is still a
// duplicate.
func (s *ImportService) UnlinkStatementRow(ctx context.Context, input CorrectStatementLinkInput) error {
	if err := validateStatementLinkCorrection(input); err != nil {
		return err
	}
	if _, err := s.statementLinkRecord(ctx, input.IdentityID); err != nil {
		return err
	}
	return statementLinkCorrectionError(s.repository.UnlinkImportedPosting(ctx, input.params(s.now().UTC().Format(time.RFC3339))))
}

// ReassignStatementRow moves a row's link to another posting the match rule
// accepts for it. The write transaction rereads the target and refuses one
// that no longer matches the row or the reviewed posted_on day.
func (s *ImportService) ReassignStatementRow(ctx context.Context, input CorrectStatementLinkInput) error {
	if err := validateStatementLinkCorrection(input); err != nil {
		return err
	}
	if input.TransactionID <= 0 || input.LineKey == "" {
		return ValidationError{Message: "a reassignment names a transaction and a line key"}
	}
	if _, err := parseCanonicalDate(input.PostedOn); err != nil {
		return ValidationError{Message: "a reassignment names the posted_on day it was reviewed with"}
	}
	record, err := s.statementLinkRecord(ctx, input.IdentityID)
	if err != nil {
		return err
	}
	batch, row, resolution := statementLinkMatchInput(record)
	matchRow, ok := importMatchEligible(batch, row, resolution)
	if !ok {
		return ErrImportStatementLinkStale
	}
	err = s.repository.ReassignImportedPosting(ctx, input.params(s.now().UTC().Format(time.RFC3339)), func(posting db.ImportPostingCandidateRecord, found bool) error {
		if !found || posting.PostedOn != input.PostedOn {
			return db.ErrImportPostingLinkStale
		}
		if _, matches := importPostingMatches(matchRow, resolution, posting); !matches {
			return db.ErrImportPostingLinkStale
		}
		return nil
	})
	return statementLinkCorrectionError(err)
}
