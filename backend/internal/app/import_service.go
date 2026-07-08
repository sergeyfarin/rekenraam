package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ImportService orchestrates the import pipeline:
// parse → normalize → dedupe → stage (preview) → commit
type ImportService struct {
	repository         *db.ImportRepository
	transactionService *TransactionService
	accountRepository  *db.AccountRepository
	registry           *adapterRegistry
	now                func() time.Time

	// Online-import dependencies (Slice 3). Both are nil-safe: callers that
	// only exercise the file-upload pipeline (e.g. unit tests) may omit them,
	// in which case StartOnlineImport/RefreshImportConnection/the fetch
	// worker simply report online import as unconfigured.
	connectionService *ImportConnectionService
	backgroundWork    *db.BackgroundWorkRepository
	httpClient        *http.Client
	trading212BaseURL string

	// investmentService routes resolved Trading 212 order-fill/dividend rows
	// to real Buy/Sell/Dividend calls at commit time (B-T212-INVST, Slice
	// 4b). Nil-safe: without it, these rows just commit as plain cash
	// transactions via the generic path, same as before Slice 4b.
	investmentService *InvestmentService
}

func NewImportService(
	repository *db.ImportRepository,
	transactionService *TransactionService,
	accountRepository *db.AccountRepository,
	connectionService *ImportConnectionService,
	backgroundWork *db.BackgroundWorkRepository,
	investmentService *InvestmentService,
) *ImportService {
	return &ImportService{
		repository:         repository,
		transactionService: transactionService,
		accountRepository:  accountRepository,
		registry:           newAdapterRegistry(&QIFAdapter{}, &Trading212Adapter{}),
		now:                time.Now,
		connectionService:  connectionService,
		backgroundWork:     backgroundWork,
		httpClient:         &http.Client{Timeout: 30 * time.Second},
		investmentService:  investmentService,
	}
}

// SetNowForTest overrides the clock used for batch/work timestamps and lease
// expiry, so tests can deterministically simulate lease expiry/restart.
func (s *ImportService) SetNowForTest(now func() time.Time) {
	s.now = now
}

// StartImport runs detect → parse → stage and returns the preview.
func (s *ImportService) StartImport(ctx context.Context, input StartImportInput) (StartImportResult, error) {
	if input.OwnerUserID <= 0 {
		return StartImportResult{}, ValidationError{Message: "owner user is required"}
	}

	adapter := s.registry.detect(input.Input)
	if adapter == nil {
		return StartImportResult{}, ValidationError{Message: "no adapter can handle this file format"}
	}

	var profile *ImportProfile
	// Future: load profile by input.ProfileID

	parseResult, err := adapter.Parse(ctx, input.Input, profile)
	if err != nil {
		return StartImportResult{}, fmt.Errorf("parse import: %w", err)
	}

	nowStr := s.now().UTC().Format(time.RFC3339)

	var profileID sql.NullInt64
	if input.ProfileID != nil {
		profileID = sql.NullInt64{Int64: *input.ProfileID, Valid: true}
	}

	metaJSON, _ := json.Marshal(parseResult.Meta)

	batch, err := s.repository.CreateImportBatch(ctx, db.CreateImportBatchParams{
		BookID:           BookID,
		SourceKind:       adapter.Kind(),
		ProfileID:        profileID,
		OriginalFilename: input.Input.Filename,
		SourceMetaJSON:   string(metaJSON),
		CreatedAt:        nowStr,
		ActorUserID:      input.OwnerUserID,
		AuthSessionID:    input.AuthSessionID,
		RequestID:        input.RequestID,
	})
	if err != nil {
		return StartImportResult{}, fmt.Errorf("create import batch: %w", err)
	}

	stagedRows, _, err := s.stageParseResult(ctx, batch.ID, parseResult)
	if err != nil {
		return StartImportResult{}, err
	}

	return StartImportResult{
		Batch:    toImportBatch(batch),
		Rows:     toImportStagedRows(stagedRows),
		Warnings: parseResult.Warnings,
		Meta:     parseResult.Meta,
	}, nil
}

// stageParseResult takes a SourceAdapter's ParseResult and runs the
// fingerprint-hash → dedupe → insert pipeline against an existing batch.
// Shared by StartImport (file path, always exactly one call per batch) and
// the Trading 212 fetch worker (Slice 3, online path — which, since adding
// pagination continuation, can call this more than once for the same batch
// as each chunk lands). The second return value is the row_index base this
// call assigned its rows at (== the count of rows already staged before this
// call), so a caller juggling multiple calls on one batch (continuation
// warnings) can translate a ParseResult-local RowIndex into a batch-global
// one.
func (s *ImportService) stageParseResult(ctx context.Context, batchID int64, parseResult ParseResult) ([]db.ImportStagedRowRecord, int, error) {
	existingRows, err := s.repository.ListAllImportStagedRows(ctx, batchID)
	if err != nil {
		return nil, 0, fmt.Errorf("list existing staged rows: %w", err)
	}
	baseIndex := len(existingRows)

	// seenInBatch starts seeded with fingerprints already staged from an
	// earlier call on this same batch. Without this, a continuation chunk
	// re-scanning the incremental cursor's overlap boundary (see the
	// Fetcher's same-timestamp handling) would insert that movement a
	// second time as a fresh "new" row instead of flagging it
	// "needs_attention" — this call has no other way to know about rows a
	// prior call already inserted.
	seenInBatch := make(map[string]bool, len(existingRows))
	for _, r := range existingRows {
		seenInBatch[r.DedupeFingerprint] = true
	}

	// Compute dedupe fingerprints and check against existing commit identities.
	for i := range parseResult.Rows {
		fp := hashFingerprint(parseResult.Rows[i].DedupeFingerprint)
		parseResult.Rows[i].DedupeFingerprint = fp
	}

	// Check each row against existing commit identities (ledger-committed
	// duplicates) — one DB lookup per unique fingerprint in this call.
	fpCommittedDupe := make(map[string]bool)
	for _, row := range parseResult.Rows {
		if _, checked := fpCommittedDupe[row.DedupeFingerprint]; checked {
			continue
		}
		_, found, err := s.repository.FindCommitIdentity(ctx, BookID, row.DedupeFingerprint)
		if err != nil {
			return nil, 0, fmt.Errorf("check commit identity: %w", err)
		}
		fpCommittedDupe[row.DedupeFingerprint] = found
	}

	// Insert staged rows.
	var dbRows []db.CreateImportStagedRowParams
	for i, row := range parseResult.Rows {
		rawJSON, _ := json.Marshal(row.Raw)
		normalizedJSON, _ := json.Marshal(map[string]any{
			"date":           row.Date,
			"amount":         row.Amount,
			"commodity_hint": row.CommodityHint,
			"payee_hint":     row.PayeeHint,
			"category_hint":  row.CategoryHint,
			"account_hint":   row.AccountHint,
			"transfer_hint":  row.TransferHint,
			"memo":           row.Memo,
			"external_ref":   row.ExternalRef,
			"splits":         row.Splits,
		})

		dedupeStatus := "new"
		if fpCommittedDupe[row.DedupeFingerprint] {
			// Exists in commit_identities → duplicate of a previously committed row.
			dedupeStatus = "duplicate"
		} else if seenInBatch[row.DedupeFingerprint] {
			// Duplicate within this batch (this call or an earlier one).
			dedupeStatus = "needs_attention"
		}
		seenInBatch[row.DedupeFingerprint] = true

		dbRows = append(dbRows, db.CreateImportStagedRowParams{
			BatchID:           batchID,
			BookID:            BookID,
			RowIndex:          baseIndex + i,
			DedupeFingerprint: row.DedupeFingerprint,
			RawJSON:           string(rawJSON),
			NormalizedJSON:    string(normalizedJSON),
			DedupeStatus:      dedupeStatus,
			ResolutionJSON:    "{}",
		})
	}

	if err := s.repository.InsertImportStagedRows(ctx, dbRows); err != nil {
		return nil, 0, fmt.Errorf("insert staged rows: %w", err)
	}

	stagedRows, err := s.repository.ListAllImportStagedRows(ctx, batchID)
	if err != nil {
		return nil, 0, fmt.Errorf("list staged rows: %w", err)
	}
	return stagedRows, baseIndex, nil
}

// GetImportBatch returns a batch with its staged rows (paginated).
func (s *ImportService) GetImportBatch(ctx context.Context, input GetImportBatchInput) (GetImportBatchResult, error) {
	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if err != nil {
		if errors.Is(err, db.ErrImportBatchNotFound) {
			return GetImportBatchResult{}, ErrImportBatchNotFound
		}
		return GetImportBatchResult{}, fmt.Errorf("get import batch: %w", err)
	}

	limit := input.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	var cursorRowIndex int
	var cursorID int64
	if input.Cursor != "" {
		_, _ = fmt.Sscanf(input.Cursor, "%d:%d", &cursorRowIndex, &cursorID)
	}

	stagedRows, err := s.repository.ListImportStagedRows(ctx, db.ListImportStagedRowsParams{
		BatchID:        input.BatchID,
		Limit:          limit + 1,
		CursorRowIndex: cursorRowIndex,
		CursorID:       cursorID,
	})
	if err != nil {
		return GetImportBatchResult{}, fmt.Errorf("list staged rows: %w", err)
	}

	nextCursor := ""
	if len(stagedRows) > limit {
		stagedRows = stagedRows[:limit]
		last := stagedRows[len(stagedRows)-1]
		nextCursor = fmt.Sprintf("%d:%d", last.RowIndex, last.ID)
	}

	return GetImportBatchResult{
		Batch:      toImportBatch(batch),
		Rows:       toImportStagedRows(stagedRows),
		NextCursor: nextCursor,
	}, nil
}

// PatchImportBatch applies row-level resolution updates (account mapping, exclude/include).
func (s *ImportService) PatchImportBatch(ctx context.Context, input PatchImportBatchInput) error {
	if input.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}

	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if err != nil {
		if errors.Is(err, db.ErrImportBatchNotFound) {
			return ErrImportBatchNotFound
		}
		return fmt.Errorf("get import batch for patch: %w", err)
	}
	if batch.Status != "previewing" {
		return ErrImportBatchNotOpen
	}

	for _, patch := range input.RowResolutions {
		if err := s.repository.UpdateImportStagedRowResolution(ctx, db.UpdateImportStagedRowResolutionParams{
			RowID:          patch.RowID,
			BatchID:        input.BatchID,
			DedupeStatus:   patch.DedupeStatus,
			ResolutionJSON: patch.ResolutionJSON,
		}); err != nil {
			return fmt.Errorf("update row %d resolution: %w", patch.RowID, err)
		}
	}

	return nil
}

// PreviewCommit performs a dry-run of commit to report reconciliation issues and counts.
func (s *ImportService) PreviewCommit(ctx context.Context, input PreviewCommitInput) (PreviewCommitResult, error) {
	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if err != nil {
		if errors.Is(err, db.ErrImportBatchNotFound) {
			return PreviewCommitResult{}, ErrImportBatchNotFound
		}
		return PreviewCommitResult{}, fmt.Errorf("get import batch for preview: %w", err)
	}
	if batch.Status != "previewing" {
		return PreviewCommitResult{}, ErrImportBatchNotOpen
	}

	rows, err := s.repository.ListAllImportStagedRows(ctx, input.BatchID)
	if err != nil {
		return PreviewCommitResult{}, fmt.Errorf("list rows for preview: %w", err)
	}

	var result PreviewCommitResult
	for _, row := range rows {
		if row.CommitStatus == "committed" || row.CommitStatus == "skipped" {
			continue
		}
		if row.DedupeStatus == "duplicate" || row.DedupeStatus == "excluded" {
			result.DuplicateCount++
			continue
		}

		resolution, err := parseResolutionJSON(row.ResolutionJSON)
		if err != nil || resolution.AccountID == 0 {
			// Not yet resolved — count as includable but cannot check reconciliation.
			result.IncludableCount++
			continue
		}

		spec, err := buildTransactionSpec(row, resolution)
		if err != nil {
			result.IncludableCount++
			continue
		}

		impact, err := s.transactionService.ReconciliationImpactForCreate(ctx, CreateReconciliationImpactInput{
			OwnerUserID: input.OwnerUserID,
			Spec:        spec,
		})
		if err != nil || len(impact.AffectedCheckpoints) == 0 {
			result.IncludableCount++
			continue
		}

		checkpointIDs := make([]int64, 0, len(impact.AffectedCheckpoints))
		for _, cp := range impact.AffectedCheckpoints {
			checkpointIDs = append(checkpointIDs, cp.CheckpointID)
		}
		result.ReconciliationIssues = append(result.ReconciliationIssues, ReconciliationIssuePreview{
			RowIndex:      row.RowIndex,
			CheckpointIDs: checkpointIDs,
		})
		result.IncludableCount++
	}

	return result, nil
}

// CommitImportBatch commits all pending staged rows via the transaction service.
// Each row is its own DB transaction (partial-commit semantics).
func (s *ImportService) CommitImportBatch(ctx context.Context, input CommitImportBatchInput) (CommitImportBatchResult, error) {
	if input.OwnerUserID <= 0 {
		return CommitImportBatchResult{}, ValidationError{Message: "owner user is required"}
	}

	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if err != nil {
		if errors.Is(err, db.ErrImportBatchNotFound) {
			return CommitImportBatchResult{}, ErrImportBatchNotFound
		}
		return CommitImportBatchResult{}, fmt.Errorf("get import batch for commit: %w", err)
	}
	if batch.Status != "previewing" && batch.Status != "partially_committed" {
		return CommitImportBatchResult{}, ErrImportBatchNotOpen
	}

	rows, err := s.repository.ListAllImportStagedRows(ctx, input.BatchID)
	if err != nil {
		return CommitImportBatchResult{}, fmt.Errorf("list rows for commit: %w", err)
	}
	sortRowsForInvestmentCommitOrder(rows)

	now := s.now().UTC()
	nowStr := now.Format(time.RFC3339)

	result := CommitImportBatchResult{BatchID: input.BatchID}
	result.TotalRows = len(rows)

	for _, row := range rows {
		if row.CommitStatus == "committed" || row.CommitStatus == "skipped" {
			// Already committed or skipped in a prior partial-commit run — count and continue.
			if row.CommitStatus == "committed" {
				result.CommittedCount++
			} else {
				result.SkippedCount++
			}
			continue
		}

		if row.DedupeStatus == "duplicate" || row.DedupeStatus == "excluded" {
			// Skip rows excluded by the user or already in the ledger.
			if err := s.repository.CommitImportStagedRow(ctx, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "skipped",
			}); err != nil {
				return result, fmt.Errorf("skip row %d: %w", row.ID, err)
			}
			result.SkippedCount++
			continue
		}

		// Check idempotency: if this fingerprint is already committed, skip.
		_, found, err := s.repository.FindCommitIdentity(ctx, BookID, row.DedupeFingerprint)
		if err != nil {
			return result, fmt.Errorf("check idempotency row %d: %w", row.ID, err)
		}
		if found {
			if err := s.repository.CommitImportStagedRow(ctx, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "skipped",
			}); err != nil {
				return result, fmt.Errorf("skip duplicate row %d: %w", row.ID, err)
			}
			result.SkippedCount++
			continue
		}

		// B-T212-INVST (Slice 4b): a Trading 212 order-fill/dividend row
		// tries the investment-posting path first — if it resolves (a cash
		// account is configured on the connection, the instrument matches
		// or can be created, and the trade actually posts), it's committed
		// as a real Buy/Sell/Dividend and this row is done. A row that
		// simply isn't an investment row, or hit an expected data gap (no
		// dividend default yet, insufficient lots), falls through unchanged
		// to the generic buildTransactionSpec path below — a strict superset
		// of pre-Slice-4b behavior. An unexpected error (missing system
		// account, invalid posting, a real ledger bug) fails just this row
		// instead of the whole batch, same as any other per-row commit
		// failure below.
		if handled, txnID, identityAccountID, err := s.commitTrading212InvestmentRow(ctx, row, batch, input, nowStr); err != nil {
			if err2 := s.repository.CommitImportStagedRow(ctx, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: err.Error(), Valid: true},
			}); err2 != nil {
				return result, fmt.Errorf("fail investment row %d after commit error: %w", row.ID, err2)
			}
			result.FailedCount++
			continue
		} else if handled {
			if err := s.recordCommitIdentityAndMarkRow(ctx, row.ID, row.DedupeFingerprint, batch.SourceKind, identityAccountID, txnID, nowStr); err != nil {
				return result, fmt.Errorf("record investment commit row %d: %w", row.ID, err)
			}
			result.CommittedCount++
			continue
		}

		resolution, err := parseResolutionJSON(row.ResolutionJSON)
		if err != nil || resolution.AccountID == 0 {
			if err := s.repository.CommitImportStagedRow(ctx, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: "missing account resolution", Valid: true},
			}); err != nil {
				return result, fmt.Errorf("fail unresolved row %d: %w", row.ID, err)
			}
			result.FailedCount++
			continue
		}

		spec, err := buildTransactionSpec(row, resolution)
		if err != nil {
			if err2 := s.repository.CommitImportStagedRow(ctx, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: err.Error(), Valid: true},
			}); err2 != nil {
				return result, fmt.Errorf("fail build spec row %d: %w", row.ID, err2)
			}
			result.FailedCount++
			continue
		}

		createParams, err := s.transactionService.prepareCreateTransactionForWrite(ctx, CreateTransactionInput{
			OwnerUserID:            input.OwnerUserID,
			AuthSessionID:          input.AuthSessionID,
			RequestID:              input.RequestID,
			OriginType:             "import",
			Operation:              "transaction.create",
			Spec:                   spec,
			ChangeReason:           "imported from " + batch.SourceKind,
			ReconciliationOverride: input.ReconciliationOverride,
		})
		if err != nil {
			if errors.Is(err, ErrReconciliationOverrideRequired) {
				// Row crosses a reconciled period and override not set → skip.
				if err2 := s.repository.CommitImportStagedRow(ctx, db.CommitImportStagedRowParams{
					RowID:        row.ID,
					CommitStatus: "skipped",
					CommitError:  sql.NullString{String: "reconciliation override required", Valid: true},
				}); err2 != nil {
					return result, fmt.Errorf("skip reconciliation row %d: %w", row.ID, err2)
				}
				result.SkippedCount++
				continue
			}

			errMsg := err.Error()
			if err2 := s.repository.CommitImportStagedRow(ctx, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: errMsg, Valid: true},
			}); err2 != nil {
				return result, fmt.Errorf("fail row %d after create error: %w", row.ID, err2)
			}
			result.FailedCount++
			continue
		}

		if _, err := s.repository.CommitImportedTransaction(ctx, db.CommitImportedTransactionParams{
			Identity: db.CreateImportCommitIdentityParams{
				BookID:            BookID,
				DedupeFingerprint: row.DedupeFingerprint,
				SourceKind:        batch.SourceKind,
				AccountID:         resolution.AccountID,
				CreatedAt:         nowStr,
			},
			Row: db.CommitImportStagedRowParams{
				RowID: row.ID,
			},
		}, func(tx *sql.Tx) (int64, error) {
			record, err := s.transactionService.createTransactionRecordInTx(ctx, tx, createParams)
			if err != nil {
				return 0, err
			}
			return record.ID, nil
		}); err != nil {
			return result, fmt.Errorf("record commit row %d: %w", row.ID, err)
		}
		result.CommittedCount++
	}

	// Determine final batch status.
	finalStatus := "committed"
	if result.FailedCount > 0 || result.SkippedCount > 0 {
		if result.CommittedCount == 0 {
			finalStatus = "failed"
		} else {
			finalStatus = "partially_committed"
		}
	}
	// If all were skipped (e.g. all duplicates), treat as committed.
	if result.CommittedCount == 0 && result.FailedCount == 0 {
		finalStatus = "committed"
	}

	detailJSON, _ := json.Marshal(map[string]any{
		"committed": result.CommittedCount,
		"skipped":   result.SkippedCount,
		"failed":    result.FailedCount,
	})

	eventKind := finalStatus
	if finalStatus == "committed" {
		eventKind = "committed"
	}

	if err := s.repository.UpdateImportBatchStatus(ctx, db.UpdateImportBatchStatusParams{
		BatchID:       input.BatchID,
		Status:        finalStatus,
		EventKind:     eventKind,
		DetailJSON:    string(detailJSON),
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OccurredAt:    nowStr,
	}); err != nil {
		return result, fmt.Errorf("update batch status: %w", err)
	}

	result.Status = finalStatus
	return result, nil
}

// recordCommitIdentityAndMarkRow records the commit identity + marks the
// staged row committed in a single DB transaction, so a crash between the
// two never leaves an orphan identity with no corresponding committed row
// (or vice versa) — a retry sees commit_status=committed and skips it.
//
// The generic import path uses CommitImportedTransaction instead so its ledger
// write joins this same transaction. The Trading 212 investment path still posts
// through investment-specific service methods first; T-26 tracks moving those
// ledger writes into the same transaction too.
func (s *ImportService) recordCommitIdentityAndMarkRow(ctx context.Context, rowID int64, dedupeFingerprint string, sourceKind string, accountID int64, transactionID int64, nowStr string) error {
	return s.repository.RecordCommitIdentityAndMarkRow(ctx, db.CommitImportedTransactionParams{
		Identity: db.CreateImportCommitIdentityParams{
			BookID:                 BookID,
			DedupeFingerprint:      dedupeFingerprint,
			CommittedTransactionID: transactionID,
			SourceKind:             sourceKind,
			AccountID:              accountID,
			CreatedAt:              nowStr,
		},
		Row: db.CommitImportStagedRowParams{
			RowID:                  rowID,
			CommitStatus:           "committed",
			CommittedTransactionID: sql.NullInt64{Int64: transactionID, Valid: true},
		},
	})
}

// DiscardImportBatch marks a previewing batch as discarded.
func (s *ImportService) DiscardImportBatch(ctx context.Context, input DiscardImportBatchInput) error {
	if input.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}

	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if err != nil {
		if errors.Is(err, db.ErrImportBatchNotFound) {
			return ErrImportBatchNotFound
		}
		return fmt.Errorf("get import batch for discard: %w", err)
	}
	if batch.Status != "previewing" {
		return ErrImportBatchNotOpen
	}

	now := s.now().UTC().Format(time.RFC3339)
	return s.repository.UpdateImportBatchStatus(ctx, db.UpdateImportBatchStatusParams{
		BatchID:       input.BatchID,
		Status:        "discarded",
		EventKind:     "discarded",
		DetailJSON:    "{}",
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OccurredAt:    now,
	})
}

// ListImportBatches returns paginated batch history.
func (s *ImportService) ListImportBatches(ctx context.Context, input ListImportBatchesInput) (ListImportBatchesResult, error) {
	limit := input.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var cursorCreatedAt string
	var cursorID int64
	if input.Cursor != "" {
		_, _ = fmt.Sscanf(input.Cursor, "%s:%d", &cursorCreatedAt, &cursorID)
	}

	batches, err := s.repository.ListImportBatches(ctx, db.ListImportBatchesParams{
		BookID:          BookID,
		Limit:           limit + 1,
		CursorCreatedAt: cursorCreatedAt,
		CursorID:        cursorID,
	})
	if err != nil {
		return ListImportBatchesResult{}, fmt.Errorf("list import batches: %w", err)
	}

	nextCursor := ""
	if len(batches) > limit {
		batches = batches[:limit]
		last := batches[len(batches)-1]
		nextCursor = fmt.Sprintf("%s:%d", last.CreatedAt, last.ID)
	}

	return ListImportBatchesResult{
		Batches:    toImportBatches(batches),
		NextCursor: nextCursor,
	}, nil
}

// --- Helpers ---

// sortRowsForInvestmentCommitOrder stable-sorts staged rows so Trading 212
// order-fill/dividend rows (see rawKeyKind) commit oldest-first by trade
// date, instead of the provider's newest-first paging order (row_index
// ASC). Without this, a full-history import can stage a recent SELL ahead
// of the older BUY that opened its lot, and commitTrading212InvestmentRow's
// Sell call fails "insufficient lots" purely from processing order — not a
// real data problem — and falls back to a plain cash transaction.
// Non-investment rows have no normalized date key here and sort by their
// empty key, which — being a stable sort — preserves their original
// row_index-relative order; only investment rows are reordered relative to
// each other and to non-investment rows.
func sortRowsForInvestmentCommitOrder(rows []db.ImportStagedRowRecord) {
	sort.SliceStable(rows, func(i, j int) bool {
		return investmentRowSortDate(rows[i]) < investmentRowSortDate(rows[j])
	})
}

// investmentRowSortDate returns the row's normalized trade date if it's a
// Trading 212 order-fill/dividend row, or "" otherwise (dates are
// YYYY-MM-DD, so "" sorts first and never collides with a real date).
func investmentRowSortDate(row db.ImportStagedRowRecord) string {
	var raw map[string]string
	if err := json.Unmarshal([]byte(row.RawJSON), &raw); err != nil {
		return ""
	}
	kind := raw[rawKeyKind]
	if kind != trading212RawKindOrderFill && kind != trading212RawKindDividend {
		return ""
	}
	var normalized struct {
		Date string `json:"date"`
	}
	if err := json.Unmarshal([]byte(row.NormalizedJSON), &normalized); err != nil {
		return ""
	}
	return normalized.Date
}

func hashFingerprint(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:16]) // 32-char hex, sufficient uniqueness
}

func parseResolutionJSON(resolutionJSON string) (ImportRowResolution, error) {
	if resolutionJSON == "" || resolutionJSON == "{}" {
		return ImportRowResolution{}, nil
	}
	var r ImportRowResolution
	if err := json.Unmarshal([]byte(resolutionJSON), &r); err != nil {
		return ImportRowResolution{}, fmt.Errorf("parse resolution json: %w", err)
	}
	return r, nil
}

// resolveOffsetAccount returns the account ID for the second posting leg.
// Transfer rows use TransferAccountID; ordinary rows use CategoryID.
func resolveOffsetAccount(resolution ImportRowResolution) (int64, error) {
	if resolution.TransferAccountID != nil {
		return *resolution.TransferAccountID, nil
	}
	if resolution.CategoryID != nil {
		return *resolution.CategoryID, nil
	}
	return 0, fmt.Errorf("missing category or transfer account resolution")
}

// buildTransactionSpec constructs a balanced TransactionInput from a staged row + resolution.
// For a simple bank account debit/credit, the two legs are:
//   - account posting: the signed amount
//   - category/offset posting: the negated amount (expense/income/transfer)
//
// When the row has QIF splits, each split becomes its own offset posting.
func buildTransactionSpec(row db.ImportStagedRowRecord, resolution ImportRowResolution) (TransactionInput, error) {
	// Parse normalized JSON for date, amount, splits, etc.
	var normalized struct {
		Date         string `json:"date"`
		Amount       string `json:"amount"`
		PayeeHint    string `json:"payee_hint"`
		Memo         string `json:"memo"`
		ExternalRef  string `json:"external_ref"`
		TransferHint string `json:"transfer_hint"`
		Splits       []struct {
			CategoryHint string `json:"category_hint"`
			Amount       string `json:"amount"`
			Memo         string `json:"memo"`
		} `json:"splits"`
	}
	if err := json.Unmarshal([]byte(row.NormalizedJSON), &normalized); err != nil {
		return TransactionInput{}, fmt.Errorf("parse normalized json: %w", err)
	}

	date, err := parseQIFDate(normalized.Date)
	if err != nil {
		return TransactionInput{}, fmt.Errorf("parse date %q: %w", normalized.Date, err)
	}

	coeff, scale, err := parseDecimalAmount(normalized.Amount)
	if err != nil {
		return TransactionInput{}, fmt.Errorf("parse amount %q: %w", normalized.Amount, err)
	}

	if resolution.CommodityID == 0 {
		return TransactionInput{}, fmt.Errorf("missing commodity resolution")
	}

	// Posting 1: the account (the raw amount, signed as-is from the file).
	posting1 := PostingInput{
		AccountID:     resolution.AccountID,
		QuantityValue: coeff,
		QuantityScale: scale,
		CommodityID:   resolution.CommodityID,
		Memo:          normalized.Memo,
	}

	var offsetPostings []PostingInput

	// Resolve the offset account: transfer takes priority over category.
	offsetAccountID, err := resolveOffsetAccount(resolution)
	if err != nil {
		return TransactionInput{}, err
	}

	if len(normalized.Splits) > 0 {
		// Splits: one offset posting per split line, each with its own amount.
		// All splits share the same resolved offset account (category or transfer).
		for _, split := range normalized.Splits {
			splitCoeff, splitScale, err := parseDecimalAmount(split.Amount)
			if err != nil {
				return TransactionInput{}, fmt.Errorf("parse split amount %q: %w", split.Amount, err)
			}
			negSplitCoeff := exact.Coefficient(new(big.Int).Neg(splitCoeff.BigInt()).String())
			offsetPostings = append(offsetPostings, PostingInput{
				AccountID:     offsetAccountID,
				QuantityValue: negSplitCoeff,
				QuantityScale: splitScale,
				CommodityID:   resolution.CommodityID,
				Memo:          split.Memo,
			})
		}
	} else {
		// No splits: single offset posting (category or transfer account), negated.
		negCoeff := exact.Coefficient(new(big.Int).Neg(coeff.BigInt()).String())
		offsetPostings = append(offsetPostings, PostingInput{
			AccountID:     offsetAccountID,
			QuantityValue: negCoeff,
			QuantityScale: scale,
			CommodityID:   resolution.CommodityID,
		})
	}

	postings := append([]PostingInput{posting1}, offsetPostings...)

	payeeName := resolution.PayeeName
	if payeeName == "" {
		payeeName = normalized.PayeeHint
	}

	spec := TransactionInput{
		Status:          "posted",
		TransactionKind: "ordinary",
		TransactionDate: date,
		PayeeID:         resolution.PayeeID,
		PayeeName:       payeeName,
		Description:     normalized.Memo,
		ExternalRefHint: normalized.ExternalRef,
		NeedsReview:     true,
		JournalEntries: []JournalEntryInput{
			{
				EntryDate: date,
				EntryKind: "ordinary",
				Postings:  postings,
			},
		},
	}

	return spec, nil
}

// parseQIFDate parses common QIF date formats into YYYY-MM-DD.
func parseQIFDate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("date is empty")
	}

	formats := []string{
		"01/02/06",   // MM/DD/YY (US)
		"01/02/2006", // MM/DD/YYYY (US)
		"02/01/06",   // DD/MM/YY (EU — detected by profile, fallback here)
		"02/01/2006", // DD/MM/YYYY (EU)
		"2006-01-02", // ISO (already normalized)
		"1/2/06",     // M/D/YY
		"1/2/2006",   // M/D/YYYY
		"2-Jan-06",   // D-Mon-YY (Quicken alternative)
		"2-Jan-2006", // D-Mon-YYYY
	}

	// Replace dash with slash to normalize separators.
	normalized := strings.ReplaceAll(raw, "-", "/")
	// But keep ISO if it was already ISO.
	if len(raw) == 10 && raw[4] == '-' && raw[7] == '-' {
		return raw, nil
	}

	for _, layout := range formats {
		layoutNorm := strings.ReplaceAll(layout, "-", "/")
		t, err := time.Parse(layoutNorm, normalized)
		if err == nil {
			return t.Format("2006-01-02"), nil
		}
	}

	// Try original formats (some use dash separators).
	for _, layout := range formats {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return t.Format("2006-01-02"), nil
		}
	}

	return "", fmt.Errorf("unrecognized date format")
}

// parseDecimalAmount parses a decimal string (e.g. "-123.45") into a Coefficient + scale.
// No floating-point is used; we parse digit strings directly.
func parseDecimalAmount(raw string) (exact.Coefficient, int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, fmt.Errorf("amount is empty")
	}

	negative := false
	if raw[0] == '-' {
		negative = true
		raw = raw[1:]
	} else if raw[0] == '+' {
		raw = raw[1:]
	}

	parts := strings.SplitN(raw, ".", 2)
	intPart := parts[0]
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}

	// Remove any remaining commas (thousands separators).
	intPart = strings.ReplaceAll(intPart, ",", "")
	fracPart = strings.ReplaceAll(fracPart, ",", "")

	scale := len(fracPart)
	digits := intPart + fracPart

	// Strip leading zeros to get canonical integer.
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		digits = "0"
	}
	if negative {
		digits = "-" + digits
	}

	coeff, err := exact.Parse(digits)
	if err != nil {
		return "", 0, fmt.Errorf("parse coefficient: %w", err)
	}
	return coeff, scale, nil
}

// --- Mapping helpers ---

func toImportBatch(rec db.ImportBatchRecord) ImportBatch {
	var profileID *int64
	if rec.ProfileID.Valid {
		profileID = &rec.ProfileID.Int64
	}
	var connectionID *int64
	if rec.ConnectionID.Valid {
		connectionID = &rec.ConnectionID.Int64
	}
	return ImportBatch{
		ID:               rec.ID,
		BookID:           rec.BookID,
		SourceKind:       rec.SourceKind,
		ProfileID:        profileID,
		ConnectionID:     connectionID,
		Status:           rec.Status,
		OriginalFilename: rec.OriginalFilename,
		SourceMetaJSON:   rec.SourceMetaJSON,
		CreatedAt:        rec.CreatedAt,
	}
}

func toImportBatches(records []db.ImportBatchRecord) []ImportBatch {
	out := make([]ImportBatch, len(records))
	for i, r := range records {
		out[i] = toImportBatch(r)
	}
	return out
}

func toImportStagedRow(rec db.ImportStagedRowRecord) ImportStagedRow {
	var txnID *int64
	if rec.CommittedTransactionID.Valid {
		txnID = &rec.CommittedTransactionID.Int64
	}
	commitError := ""
	if rec.CommitError.Valid {
		commitError = rec.CommitError.String
	}
	return ImportStagedRow{
		ID:                     rec.ID,
		BatchID:                rec.BatchID,
		BookID:                 rec.BookID,
		RowIndex:               rec.RowIndex,
		DedupeFingerprint:      rec.DedupeFingerprint,
		RawJSON:                rec.RawJSON,
		NormalizedJSON:         rec.NormalizedJSON,
		DedupeStatus:           rec.DedupeStatus,
		ResolutionJSON:         rec.ResolutionJSON,
		CommitStatus:           rec.CommitStatus,
		CommittedTransactionID: txnID,
		CommitError:            commitError,
	}
}

func toImportStagedRows(records []db.ImportStagedRowRecord) []ImportStagedRow {
	out := make([]ImportStagedRow, len(records))
	for i, r := range records {
		out[i] = toImportStagedRow(r)
	}
	return out
}
