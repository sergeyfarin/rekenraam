package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"rekenraam/backend/internal/db"
)

// Trading 212 split review (T-122). The provider's Fill schema names a
// STOCK_SPLIT type but supplies no split ratio or entitlement, so the fill
// type alone cannot justify posting a split; no representative payload has
// been verified. Such rows stay in review and never post. After recording the
// split manually, the user links the row to it, which admits the row's dedupe
// identity against the existing split without writing a second journal.

const trading212FillTypeStockSplit = "STOCK_SPLIT"

// ErrImportSplitLinkUnavailable means the row is not a pending provider split
// in this batch, or the chosen split is not an effective, unlinked split of
// the row's security.
var ErrImportSplitLinkUnavailable = errors.New("import row cannot be linked to this split")

type Trading212SplitRowInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	BatchID       int64
	RowID         int64
}

type LinkTrading212SplitInput struct {
	Trading212SplitRowInput
	OperationID int64
}

// Trading212SplitCandidate is a recorded split of the row's security.
type Trading212SplitCandidate struct {
	OperationID      int64
	TransactionID    int64
	HoldingAccountID int64
	EffectiveOn      string
	RatioNumerator   int64
	RatioDenominator int64
	Linked           bool
}

type trading212SplitRow struct {
	row         db.ImportStagedRowRecord
	sourceKind  string
	commodityID int64
	found       bool
}

func (s *ImportService) trading212SplitRow(ctx context.Context, input Trading212SplitRowInput) (trading212SplitRow, error) {
	if input.OwnerUserID <= 0 || input.BatchID <= 0 || input.RowID <= 0 {
		return trading212SplitRow{}, ValidationError{Message: "owner, batch and row ids are required"}
	}
	if s.investmentService == nil {
		return trading212SplitRow{}, ErrImportSplitLinkUnavailable
	}
	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if errors.Is(err, db.ErrImportBatchNotFound) {
		return trading212SplitRow{}, ErrImportBatchNotFound
	}
	if err != nil {
		return trading212SplitRow{}, fmt.Errorf("read split row batch: %w", err)
	}
	if batch.SourceKind != "trading212" || batch.Status == "discarded" || batch.Status == "rolled_back" {
		return trading212SplitRow{}, ErrImportSplitLinkUnavailable
	}
	row, err := s.repository.ImportStagedRowByID(ctx, input.RowID)
	if errors.Is(err, db.ErrNotFound) {
		return trading212SplitRow{}, ErrImportSplitLinkUnavailable
	}
	if err != nil {
		return trading212SplitRow{}, fmt.Errorf("read staged split row: %w", err)
	}
	var raw map[string]string
	if row.BatchID != input.BatchID || row.CommitStatus == "committed" || row.DedupeStatus == "duplicate" ||
		json.Unmarshal([]byte(row.RawJSON), &raw) != nil || raw[rawKeyKind] != trading212RawKindOrderFill ||
		raw["fill_type"] != trading212FillTypeStockSplit {
		return trading212SplitRow{}, ErrImportSplitLinkUnavailable
	}
	// Find, never create: linking must not leave an instrument behind.
	_, commodityID, found, err := s.investmentService.FindInstrumentForImport(ctx, raw[rawKeyISIN], raw[rawKeyTicker])
	if err != nil {
		return trading212SplitRow{}, fmt.Errorf("resolve split row instrument: %w", err)
	}
	return trading212SplitRow{row: row, sourceKind: batch.SourceKind, commodityID: commodityID, found: found}, nil
}

// Trading212SplitCandidates lists recorded splits of the staged split row's
// security, newest first. An unknown security has no candidates.
func (s *ImportService) Trading212SplitCandidates(ctx context.Context, input Trading212SplitRowInput) ([]Trading212SplitCandidate, error) {
	split, err := s.trading212SplitRow(ctx, input)
	if err != nil || !split.found {
		return []Trading212SplitCandidate{}, err
	}
	records, err := s.repository.ListSplitLinkCandidates(ctx, BookID, split.commodityID)
	if err != nil {
		return nil, err
	}
	candidates := make([]Trading212SplitCandidate, 0, len(records))
	for _, record := range records {
		candidates = append(candidates, Trading212SplitCandidate(record))
	}
	return candidates, nil
}

// LinkTrading212Split marks a staged provider split row as evidence of an
// existing recorded split. Its dedupe identity names that split, so a retry
// or re-fetch of the same fill is a duplicate and nothing posts twice.
func (s *ImportService) LinkTrading212Split(ctx context.Context, input LinkTrading212SplitInput) error {
	if input.OperationID <= 0 {
		return ValidationError{Message: "split operation is required"}
	}
	split, err := s.trading212SplitRow(ctx, input.Trading212SplitRowInput)
	if err != nil {
		return err
	}
	if !split.found {
		return ErrImportSplitLinkUnavailable
	}
	_, err = s.repository.LinkStagedRowToSplit(ctx, db.LinkStagedRowToSplitParams{
		BookID: BookID, BatchID: input.BatchID, RowID: input.RowID,
		DedupeFingerprint: split.row.DedupeFingerprint, SourceKind: split.sourceKind,
		OperationID: input.OperationID, CommodityID: split.commodityID,
		ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		Now: s.now().UTC().Format(time.RFC3339),
	})
	if errors.Is(err, db.ErrSplitLinkUnavailable) {
		return ErrImportSplitLinkUnavailable
	}
	return err
}
