package app

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
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

	// beforeImportRowCommitForTest creates a deterministic race window after
	// an idempotency read and before the row's write transaction. Production
	// callers leave it nil.
	beforeImportRowCommitForTest func()

	// beforeTrading212HoldingCreateForTest creates a deterministic race window
	// after a missing holding-map read and before account creation. Production
	// callers leave it nil.
	beforeTrading212HoldingCreateForTest func()

	// beforeImportCommitIdentityCheckForTest creates a deterministic stale-row
	// window after staged rows are listed and before their identity lookup.
	// Production callers leave it nil.
	beforeImportCommitIdentityCheckForTest func()
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

	// Profiles are not persisted yet (R5 mapping-profile work owns that); the
	// adapters already honor one when given it — a QIF profile's date_layout
	// and decimal_separator override the per-file detection.
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

	// Validate every patch before writing any of them: import_staged_rows
	// enforces dedupe_status and resolution_json shape with DB CHECK
	// constraints, and a violation from client input should surface as a
	// clean 400 VALIDATION_FAILED rather than a 500 from a raw SQL error
	// partway through the batch.
	for _, patch := range input.RowResolutions {
		if err := validateRowResolutionPatch(patch); err != nil {
			return err
		}
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

// validRowResolutionDedupeStatuses mirrors the import_staged_rows.dedupe_status
// CHECK constraint (migrations).
var validRowResolutionDedupeStatuses = map[string]bool{
	"new":             true,
	"duplicate":       true,
	"needs_attention": true,
	"excluded":        true,
}

func validateRowResolutionPatch(patch RowResolutionPatch) error {
	if patch.RowID <= 0 {
		return ValidationError{Message: "row_id is required"}
	}
	if patch.DedupeStatus != "" && !validRowResolutionDedupeStatuses[patch.DedupeStatus] {
		return ValidationError{Message: "dedupe_status is invalid"}
	}
	if patch.ResolutionJSON != "" && !json.Valid([]byte(patch.ResolutionJSON)) {
		return ValidationError{Message: "resolution is not valid JSON"}
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
			if err := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "skipped",
			}, nil); err != nil {
				return result, fmt.Errorf("skip row %d: %w", row.ID, err)
			}
			continue
		}

		// Check idempotency: if this fingerprint is already committed, skip.
		if s.beforeImportCommitIdentityCheckForTest != nil {
			s.beforeImportCommitIdentityCheckForTest()
		}
		identity, found, err := s.repository.FindCommitIdentity(ctx, BookID, row.DedupeFingerprint)
		if err != nil {
			return result, fmt.Errorf("check idempotency row %d: %w", row.ID, err)
		}
		if found {
			if err := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "skipped",
			}, &identity.CommittedTransactionID); err != nil {
				return result, fmt.Errorf("skip duplicate row %d: %w", row.ID, err)
			}
			continue
		}
		if s.beforeImportRowCommitForTest != nil {
			s.beforeImportRowCommitForTest()
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
		if handled, _, _, err := s.commitTrading212InvestmentRow(ctx, row, batch, input, nowStr); err != nil {
			if errors.Is(err, db.ErrCommitIdentityConflict) {
				if err := s.resolveConcurrentImportCommit(ctx, row); err != nil {
					return result, fmt.Errorf("resolve concurrent investment commit row %d: %w", row.ID, err)
				}
				result.CommittedCount++
				continue
			}
			if errors.Is(err, ErrReconciliationOverrideRequired) {
				if err2 := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
					RowID:        row.ID,
					CommitStatus: "skipped",
					CommitError:  sql.NullString{String: "reconciliation override required", Valid: true},
				}, nil); err2 != nil {
					return result, fmt.Errorf("skip investment reconciliation row %d: %w", row.ID, err2)
				}
				continue
			}
			if err2 := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: err.Error(), Valid: true},
			}, nil); err2 != nil {
				return result, fmt.Errorf("fail investment row %d after commit error: %w", row.ID, err2)
			}
			continue
		} else if handled {
			// The investment path recorded the import identity and marked this
			// staged row in the same transaction as its ledger/lot writes.
			// Keeping that work inside the investment transaction closes T-26:
			// a crash can no longer post an investment transaction without its
			// idempotency identity.
			result.CommittedCount++
			continue
		}

		resolution, err := parseResolutionJSON(row.ResolutionJSON)
		if err != nil || resolution.AccountID == 0 {
			if err := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: "missing account resolution", Valid: true},
			}, nil); err != nil {
				return result, fmt.Errorf("fail unresolved row %d: %w", row.ID, err)
			}
			continue
		}

		spec, err := buildTransactionSpec(row, resolution)
		if err != nil {
			if err2 := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: err.Error(), Valid: true},
			}, nil); err2 != nil {
				return result, fmt.Errorf("fail build spec row %d: %w", row.ID, err2)
			}
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
				if err2 := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
					RowID:        row.ID,
					CommitStatus: "skipped",
					CommitError:  sql.NullString{String: "reconciliation override required", Valid: true},
				}, nil); err2 != nil {
					return result, fmt.Errorf("skip reconciliation row %d: %w", row.ID, err2)
				}
				continue
			}

			errMsg := err.Error()
			if err2 := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: errMsg, Valid: true},
			}, nil); err2 != nil {
				return result, fmt.Errorf("fail row %d after create error: %w", row.ID, err2)
			}
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
			// A concurrent committer already won the race for this exact
			// fingerprint (e.g. two requests committing the same batch at
			// once): the loser's own transaction insert was rolled back
			// (CommitImportedTransaction's tx never committed), so there is
			// no partial write to clean up — just adopt the winner's
			// outcome, same as the Trading 212 investment path already does
			// via resolveConcurrentImportCommit.
			if errors.Is(err, db.ErrCommitIdentityConflict) {
				if err2 := s.resolveConcurrentImportCommit(ctx, row); err2 != nil {
					return result, fmt.Errorf("resolve concurrent commit row %d: %w", row.ID, err2)
				}
				result.CommittedCount++
				continue
			}
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

// recordImportStagedRowTerminal applies a failed or skipped terminal update.
// A caller may hold a stale pending snapshot while another caller atomically
// commits the transaction, identity, and row marker. In that case the guarded
// repository update reports ErrImportStagedRowAlreadyCommitted; reload and
// verify the winner before reporting the row as a successful commit.
func (s *ImportService) recordImportStagedRowTerminal(ctx context.Context, result *CommitImportBatchResult, params db.CommitImportStagedRowParams, expectedTransactionID *int64) error {
	err := s.repository.CommitImportStagedRow(ctx, params)
	if err == nil {
		switch params.CommitStatus {
		case "skipped":
			result.SkippedCount++
		case "failed":
			result.FailedCount++
		default:
			return fmt.Errorf("unsupported import staged-row terminal status %q", params.CommitStatus)
		}
		return nil
	}
	if !errors.Is(err, db.ErrImportStagedRowAlreadyCommitted) {
		return err
	}

	row, err := s.repository.ImportStagedRowByID(ctx, params.RowID)
	if err != nil {
		return fmt.Errorf("reload committed staged row: %w", err)
	}
	if row.CommitStatus != "committed" || !row.CommittedTransactionID.Valid {
		return fmt.Errorf("staged row %d reported committed transition conflict but is not committed", params.RowID)
	}
	if expectedTransactionID != nil && row.CommittedTransactionID.Int64 != *expectedTransactionID {
		return fmt.Errorf("staged row %d committed transaction does not match import identity", params.RowID)
	}
	result.CommittedCount++
	return nil
}

// resolveConcurrentImportCommit turns a duplicate transaction attempt into
// the successful outcome it represents. Commit identities and the winning
// row marker share one transaction, so once an identity conflict occurs its
// referenced transaction is durable and the row is either already committed
// or still pending for this conditional repair.
func (s *ImportService) resolveConcurrentImportCommit(ctx context.Context, row db.ImportStagedRowRecord) error {
	identity, found, err := s.repository.FindCommitIdentity(ctx, BookID, row.DedupeFingerprint)
	if err != nil {
		return fmt.Errorf("find existing commit identity: %w", err)
	}
	if !found {
		return errors.New("commit identity conflict without existing identity")
	}
	if _, err := s.repository.MarkImportStagedRowCommittedIfPending(ctx, row.ID, identity.CommittedTransactionID); err != nil {
		return fmt.Errorf("mark existing commit identity as committed: %w", err)
	}
	return nil
}

// recordCommitIdentityAndMarkRowInTx is the investment-import variant used by
// Buy/Sell/Dividend's post-write callback. The caller owns tx, which already
// contains the transaction and, for trades, lot/acquisition or disposal
// writes. Keeping the identity and staged-row marker in that tx makes the
// at-least-once import retry boundary genuinely idempotent.
func (s *ImportService) recordCommitIdentityAndMarkRowInTx(ctx context.Context, tx *sql.Tx, rowID int64, dedupeFingerprint string, sourceKind string, accountID int64, transactionID int64, nowStr string) error {
	if err := s.repository.CreateCommitIdentity(ctx, tx, db.CreateImportCommitIdentityParams{
		BookID:                 BookID,
		DedupeFingerprint:      dedupeFingerprint,
		CommittedTransactionID: transactionID,
		SourceKind:             sourceKind,
		AccountID:              accountID,
		CreatedAt:              nowStr,
	}); err != nil {
		return fmt.Errorf("create investment commit identity: %w", err)
	}
	if err := s.repository.CommitImportStagedRowInTx(ctx, tx, db.CommitImportStagedRowParams{
		RowID:                  rowID,
		CommitStatus:           "committed",
		CommittedTransactionID: sql.NullInt64{Int64: transactionID, Valid: true},
	}); err != nil {
		return fmt.Errorf("mark investment row committed: %w", err)
	}
	return nil
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
// order fills commit oldest-first by their full provider fill timestamp,
// instead of the provider's newest-first paging order (row_index ASC).
// Ordering by only the YYYY-MM-DD transaction date is insufficient: an
// intraday SELL can otherwise remain before the BUY that opened its lot,
// fail "insufficient lots", and fall back to a plain cash transaction.
// Non-investment rows have no normalized date key here and sort by their
// empty key, which — being a stable sort — preserves their original
// row_index-relative order; only investment rows are reordered relative to
// each other and to non-investment rows.
func sortRowsForInvestmentCommitOrder(rows []db.ImportStagedRowRecord) {
	// Decorate-sort-undecorate: investmentRowSortKey unmarshals JSON, so
	// computing it once per row rather than on every comparison keeps a large
	// brokerage import from re-parsing the same rows O(n log n) times. The key
	// travels with its row rather than sitting in a map, so rows that share an
	// ID (or carry none) still sort by their own key.
	type keyedRow struct {
		key string
		row db.ImportStagedRowRecord
	}
	decorated := make([]keyedRow, len(rows))
	for i, row := range rows {
		decorated[i] = keyedRow{key: investmentRowSortKey(row), row: row}
	}
	slices.SortStableFunc(decorated, func(a, b keyedRow) int {
		return cmp.Compare(a.key, b.key)
	})
	for i, d := range decorated {
		rows[i] = d.row
	}
}

// investmentRowSortKey returns a full order-fill timestamp where available,
// falling back to the normalized date for dividends and older staged data.
// Empty keys preserve original row order via the stable sort.
func investmentRowSortKey(row db.ImportStagedRowRecord) string {
	var raw map[string]string
	if err := json.Unmarshal([]byte(row.RawJSON), &raw); err != nil {
		return ""
	}
	kind := raw[rawKeyKind]
	if kind != trading212RawKindOrderFill && kind != trading212RawKindDividend {
		return ""
	}
	if kind == trading212RawKindOrderFill && raw[rawKeyFilledAt] != "" {
		return raw[rawKeyFilledAt]
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
//
// Staged rows produced by the QIF adapter already carry an ISO date — the
// adapter resolves the file's field order once, from the import profile or by
// detecting it across every row (see import_locale.go). This is the
// last-resort parse for rows staged before that, and for adapters that emit
// dates verbatim; with no file to look at it can only fall back to QIF's
// historic US default for genuinely ambiguous dates.
func parseQIFDate(raw string) (string, error) {
	return parseFlexibleDate(raw, dateOrderAuto)
}

// parseDecimalAmount parses a decimal string (e.g. "-123.45") into a Coefficient + scale.
// No floating-point is used; we parse digit strings directly. Decimal-comma
// input ("1.234,56") is normalized first — see canonicalDecimal for how the
// separator is decided.
func parseDecimalAmount(raw string) (exact.Coefficient, int, error) {
	canonical, err := canonicalDecimal(raw, 0)
	if err != nil {
		return "", 0, err
	}

	negative := false
	if canonical[0] == '-' {
		negative = true
		canonical = canonical[1:]
	}

	intPart, fracPart, _ := strings.Cut(canonical, ".")

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
