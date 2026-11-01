package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
