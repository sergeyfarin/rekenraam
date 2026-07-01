package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/onlinesource/trading212"
)

// trading212FetchWorkKind is the durable work queue kind processed by
// ImportService.StartBackgroundWorker. Payload shape per
// docs/trading212-import-plan.md "Data model".
const trading212FetchWorkKind = "import.fetch.trading212"

// maxTrading212FetchAttempts bounds retries before a fetch is given up on as
// terminal. Unlike FX coverage (which retries forever), an import fetch that
// keeps failing must eventually surface to the user as a failed batch rather
// than leave it "fetching" indefinitely.
const maxTrading212FetchAttempts = 8

// ErrImportFetchInProgress is returned when a connection already has an
// unfinished fetch (an existing previewing batch with fetch_status=fetching).
// The durable queue does not coalesce different connection/cursor payloads,
// so this guard lives at the service layer (see "Durable fetch worker" in
// the import plan).
var ErrImportFetchInProgress = errors.New("an import fetch is already in progress for this connection")

type trading212FetchWorkPayload struct {
	ConnectionID int64  `json:"connection_id"`
	BatchID      int64  `json:"batch_id"`
	Reason       string `json:"reason"` // "manual" | "continuation"
	// Cursor is the incremental boundary passed to trading212.Fetcher.Fetch.
	// It is fixed for a whole logical fetch (empty = full history) — a
	// continuation payload carries the exact same value as the fetch it
	// continues, never an updated one. See Fetch's doc comment for why: the
	// boundary and the pagination resume point are different things.
	Cursor string `json:"cursor"`
	// ResumePath is the page to resume HTTP pagination from (Fetcher's
	// NextPageToken from the previous chunk). Empty means start at page 1 —
	// true for the first chunk of any fetch, manual or continuation-seeded.
	ResumePath string `json:"resume_path,omitempty"`
	// MaxCursorSoFar tracks the running max movement timestamp seen across
	// every chunk of this logical fetch so far. Only the final chunk (no
	// more continuation) persists this to the connection's saved cursor.
	MaxCursorSoFar string `json:"max_cursor_so_far,omitempty"`
}

// trading212BatchMeta is the JSON written to import_batches.source_meta_json
// for online batches. It deliberately carries connection_display_name (a
// snapshot, not a live join) so a batch's provenance stays readable even
// after the connection that produced it is deleted (closes T-12).
type trading212BatchMeta struct {
	FetchStatus           string         `json:"fetch_status"` // "fetching" | "ready" | "failed"
	ConnectionDisplayName string         `json:"connection_display_name,omitempty"`
	AccountHints          []string       `json:"account_hints,omitempty"`
	CurrencyHints         []string       `json:"currency_hints,omitempty"`
	DateFrom              string         `json:"date_from,omitempty"`
	DateTo                string         `json:"date_to,omitempty"`
	Warnings              []ParseWarning `json:"warnings,omitempty"`
	Error                 string         `json:"error,omitempty"`
}

// --- Service entry points (HTTP-facing) ---

// StartOnlineImport enqueues a full Trading 212 fetch (empty cursor) and
// creates the previewing batch it will stage rows into. Mirrors StartImport
// but the fetch runs on the durable queue instead of inline.
func (s *ImportService) StartOnlineImport(ctx context.Context, input StartOnlineImportInput) (StartOnlineImportResult, error) {
	if input.OwnerUserID <= 0 {
		return StartOnlineImportResult{}, ValidationError{Message: "owner user is required"}
	}
	if s.connectionService == nil || s.backgroundWork == nil {
		return StartOnlineImportResult{}, fmt.Errorf("online import is not configured")
	}

	conn, err := s.connectionService.GetImportConnection(ctx, input.ConnectionID)
	if err != nil {
		return StartOnlineImportResult{}, err
	}

	batch, err := s.startTrading212Fetch(ctx, trading212FetchStartParams{
		Connection:    conn,
		Cursor:        "",
		Reason:        "manual",
		OwnerUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
	})
	if err != nil {
		return StartOnlineImportResult{}, err
	}
	return StartOnlineImportResult{Batch: batch}, nil
}

// RefreshImportConnection enqueues an incremental fetch using the
// connection's saved cursor, opening a fresh batch for the new rows.
func (s *ImportService) RefreshImportConnection(ctx context.Context, input RefreshImportConnectionInput) (StartOnlineImportResult, error) {
	if input.OwnerUserID <= 0 {
		return StartOnlineImportResult{}, ValidationError{Message: "owner user is required"}
	}
	if s.connectionService == nil || s.backgroundWork == nil {
		return StartOnlineImportResult{}, fmt.Errorf("online import is not configured")
	}

	conn, err := s.connectionService.GetImportConnection(ctx, input.ConnectionID)
	if err != nil {
		return StartOnlineImportResult{}, err
	}

	batch, err := s.startTrading212Fetch(ctx, trading212FetchStartParams{
		Connection:    conn,
		Cursor:        conn.FetchCursor,
		Reason:        "manual",
		OwnerUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
	})
	if err != nil {
		return StartOnlineImportResult{}, err
	}
	return StartOnlineImportResult{Batch: batch}, nil
}

type trading212FetchStartParams struct {
	Connection    ImportConnection
	Cursor        string
	Reason        string
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
}

// startTrading212Fetch creates the previewing batch and enqueues the durable
// fetch work item, atomically (see ImportRepository.StartOnlineImportBatch).
// Shared by StartOnlineImport (full fetch) and RefreshImportConnection
// (incremental fetch).
func (s *ImportService) startTrading212Fetch(ctx context.Context, params trading212FetchStartParams) (ImportBatch, error) {
	if params.Connection.SourceKind != "trading212" {
		return ImportBatch{}, ValidationError{Message: "unsupported online source"}
	}

	nowStr := s.now().UTC().Format(time.RFC3339)
	metaJSON, _ := json.Marshal(trading212BatchMeta{
		FetchStatus:           "fetching",
		ConnectionDisplayName: params.Connection.DisplayName,
	})

	batch, err := s.repository.StartOnlineImportBatch(ctx, db.StartOnlineImportBatchParams{
		BookID:         BookID,
		ConnectionID:   params.Connection.ID,
		SourceKind:     params.Connection.SourceKind,
		SourceMetaJSON: string(metaJSON),
		CreatedAt:      nowStr,
		ActorUserID:    params.OwnerUserID,
		AuthSessionID:  params.AuthSessionID,
		RequestID:      params.RequestID,
		WorkKind:       trading212FetchWorkKind,
	}, func(batchID int64) (string, error) {
		payloadJSON, err := json.Marshal(trading212FetchWorkPayload{
			ConnectionID: params.Connection.ID,
			BatchID:      batchID,
			Reason:       params.Reason,
			Cursor:       params.Cursor,
		})
		return string(payloadJSON), err
	})
	if err != nil {
		if errors.Is(err, db.ErrImportFetchAlreadyInProgress) {
			return ImportBatch{}, ErrImportFetchInProgress
		}
		return ImportBatch{}, fmt.Errorf("start online import batch: %w", err)
	}

	return toImportBatch(batch), nil
}

// --- Durable worker ---

// StartBackgroundWorker starts the Trading 212 fetch worker loop, mirroring
// PricingService.StartBackgroundWorker (pricing_worker.go). No-ops if the
// service was constructed without online-import dependencies.
func (s *ImportService) StartBackgroundWorker(ctx context.Context, logger *slog.Logger) {
	if s.backgroundWork == nil || s.connectionService == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	workerID := uuid.NewString()
	go func() {
		s.runDueTrading212Fetches(ctx, logger, workerID)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runDueTrading212Fetches(ctx, logger, workerID)
			}
		}
	}()
}

func (s *ImportService) runDueTrading212Fetches(ctx context.Context, logger *slog.Logger, workerID string) {
	for processed := 0; processed < 4; processed++ {
		now := s.now().UTC()
		item, err := s.backgroundWork.ClaimBackgroundWork(ctx, trading212FetchWorkKind, workerID, now.Format(time.RFC3339), now.Add(15*time.Minute).Format(time.RFC3339))
		if errors.Is(err, db.ErrNotFound) || ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.WarnContext(ctx, "claim import fetch work", slog.Any("err", err))
			return
		}
		s.processTrading212FetchWork(ctx, logger, workerID, item)
	}
}

func (s *ImportService) processTrading212FetchWork(ctx context.Context, logger *slog.Logger, workerID string, item db.BackgroundWorkItemRecord) {
	var payload trading212FetchWorkPayload
	if item.PayloadVersion != 1 || json.Unmarshal([]byte(item.PayloadJSON), &payload) != nil || payload.ConnectionID <= 0 || payload.BatchID <= 0 {
		if err := s.backgroundWork.FailBackgroundWork(ctx, item.ID, workerID, s.now().UTC().Format(time.RFC3339), "invalid import fetch work payload"); err != nil {
			logger.WarnContext(ctx, "fail invalid import fetch work", slog.Int64("work_id", item.ID), slog.Any("err", err))
		}
		return
	}

	err := s.runTrading212Fetch(ctx, payload)
	if err == nil {
		now := s.now().UTC().Format(time.RFC3339)
		if completeErr := s.backgroundWork.CompleteBackgroundWork(ctx, item.ID, workerID, now); completeErr != nil {
			logger.WarnContext(ctx, "complete import fetch work", slog.Int64("work_id", item.ID), slog.Any("err", completeErr))
		}
		return
	}
	if ctx.Err() != nil {
		return
	}

	terminal := errors.Is(err, trading212.ErrUnauthorized) || item.Attempts >= maxTrading212FetchAttempts
	if !terminal {
		now := s.now().UTC()
		delay := retryDelay(item.Attempts, item.ID)
		if retryErr := s.backgroundWork.RetryBackgroundWork(ctx, item.ID, workerID, now.Add(delay).Format(time.RFC3339), now.Format(time.RFC3339), err.Error()); retryErr != nil {
			logger.WarnContext(ctx, "schedule import fetch retry", slog.Int64("work_id", item.ID), slog.Any("err", retryErr))
			return
		}
		logger.WarnContext(ctx, "import fetch will retry", slog.Int64("work_id", item.ID), slog.Duration("retry_in", delay), slog.Any("err", err))
		return
	}

	now := s.now().UTC().Format(time.RFC3339)
	if failErr := s.backgroundWork.FailBackgroundWork(ctx, item.ID, workerID, now, err.Error()); failErr != nil {
		logger.WarnContext(ctx, "fail import fetch work", slog.Int64("work_id", item.ID), slog.Any("err", failErr))
		return
	}
	s.markTrading212FetchTerminalFailure(ctx, logger, payload, err, now)
}

// runTrading212Fetch performs one fetch attempt: load connection + secret,
// call the fetcher, parse via the registered Trading212Adapter (reusing the
// same RawInput.Bytes contract every adapter uses), stage rows through the
// shared stageParseResult pipeline, and persist progress. Returns nil on
// success (including the no-op case where the batch was closed by the user
// mid-fetch, and the case where more pages remain and a continuation was
// enqueued instead of finishing).
func (s *ImportService) runTrading212Fetch(ctx context.Context, payload trading212FetchWorkPayload) error {
	batch, err := s.repository.ImportBatchByID(ctx, BookID, payload.BatchID)
	if err != nil {
		return fmt.Errorf("load import batch: %w", err)
	}
	if batch.Status != "previewing" {
		return nil // discarded or committed already; nothing to do.
	}

	conn, err := s.connectionService.GetImportConnection(ctx, payload.ConnectionID)
	if err != nil {
		return fmt.Errorf("load import connection: %w", err)
	}
	apiKey, err := s.connectionService.OpenSecret(ctx, payload.ConnectionID)
	if err != nil {
		return fmt.Errorf("open import connection secret: %w", err)
	}

	baseURL := trading212BaseURLFromConfig(conn.ConfigJSON)
	fetcher := trading212.NewFetcher(s.httpClient, baseURL)
	result, err := fetcher.Fetch(ctx, apiKey, payload.Cursor, payload.ResumePath)
	if err != nil {
		return err
	}

	rawBytes, err := json.Marshal(trading212FetchPayload{
		ConnectionID: payload.ConnectionID,
		Movements:    toAdapterMovements(result.Movements),
	})
	if err != nil {
		return fmt.Errorf("marshal trading212 fetch payload: %w", err)
	}

	adapter, ok := s.registry.byKindName("trading212")
	if !ok {
		return fmt.Errorf("trading212 adapter not registered")
	}
	parseResult, err := adapter.Parse(ctx, RawInput{Bytes: rawBytes}, nil)
	if err != nil {
		return fmt.Errorf("parse trading212 fetch: %w", err)
	}

	// Re-check: fetching can take a while; the user may have discarded or
	// committed the batch while it was in flight.
	batch, err = s.repository.ImportBatchByID(ctx, BookID, payload.BatchID)
	if err != nil {
		return fmt.Errorf("reload import batch: %w", err)
	}
	if batch.Status != "previewing" {
		return nil
	}

	var existingMeta trading212BatchMeta
	_ = json.Unmarshal([]byte(batch.SourceMetaJSON), &existingMeta)

	baseIndex := 0
	if len(parseResult.Rows) > 0 {
		_, baseIndex, err = s.stageParseResult(ctx, payload.BatchID, parseResult)
		if err != nil {
			return fmt.Errorf("stage trading212 rows: %w", err)
		}
	}

	merged := mergeTrading212BatchMeta(existingMeta, parseResult, baseIndex)
	merged.ConnectionDisplayName = conn.DisplayName
	now := s.now().UTC().Format(time.RFC3339)

	// The running max across every chunk of this logical fetch so far, not
	// just this chunk's result.Cursor — a later (older-page) chunk's max can
	// never exceed an earlier chunk's if the provider is newest-first as
	// documented, but tracking the true running max doesn't depend on that
	// ordering assumption holding exactly.
	runningMax := payload.MaxCursorSoFar
	if result.Cursor > runningMax {
		runningMax = result.Cursor
	}

	if result.HasMore {
		// More pages remain than a single Fetch call will follow (see
		// trading212.maxPages). Persist progress and enqueue a continuation
		// work item instead of marking the batch ready — this closes the
		// gap where a deep-history account was silently truncated at the
		// page cap with no signal and no way to recover the missing tail.
		// ResumePath (not Cursor) carries where to continue pagination from:
		// reusing Cursor here would immediately re-trigger the incremental
		// stop on the very next page, since every Fetch call restarts
		// pagination at page 1 (see Fetch's doc comment).
		merged.FetchStatus = "fetching"
		metaJSON, _ := json.Marshal(merged)
		if err := s.repository.UpdateImportBatchSourceMeta(ctx, payload.BatchID, string(metaJSON)); err != nil {
			return fmt.Errorf("update batch source meta: %w", err)
		}

		continuationJSON, err := json.Marshal(trading212FetchWorkPayload{
			ConnectionID:   payload.ConnectionID,
			BatchID:        payload.BatchID,
			Reason:         "continuation",
			Cursor:         payload.Cursor, // unchanged incremental boundary for the whole logical fetch
			ResumePath:     result.NextPageToken,
			MaxCursorSoFar: runningMax,
		})
		if err != nil {
			return fmt.Errorf("marshal continuation fetch payload: %w", err)
		}
		if _, err := s.backgroundWork.EnqueueBackgroundWork(ctx, BookID, trading212FetchWorkKind, string(continuationJSON), now); err != nil {
			return fmt.Errorf("enqueue continuation fetch: %w", err)
		}
		return nil
	}

	merged.FetchStatus = "ready"
	metaJSON, _ := json.Marshal(merged)
	if err := s.repository.UpdateImportBatchSourceMeta(ctx, payload.BatchID, string(metaJSON)); err != nil {
		return fmt.Errorf("update batch source meta: %w", err)
	}

	// The connection's saved cursor only advances once the fetch (across all
	// its continuation chunks) has actually caught up to "now" — persisting
	// a partial cursor mid-backfill and then failing terminally on a later
	// chunk would make a future incremental refresh skip the un-fetched gap.
	cursor := runningMax
	if cursor == "" {
		cursor = payload.Cursor // no movements seen this run; keep the existing cursor.
	}
	if err := s.connectionService.UpdateFetchCursor(ctx, payload.ConnectionID, cursor, "ready", now); err != nil {
		return fmt.Errorf("update connection cursor: %w", err)
	}

	return nil
}

// mergeTrading212BatchMeta folds a fetch chunk's ParseResult into the batch's
// existing meta so that, across pagination continuation chunks, hints and
// warnings accumulate rather than each chunk overwriting the last.
// baseIndex is the row_index this chunk's rows were staged at (see
// stageParseResult), used to translate each ParseWarning's chunk-local
// RowIndex into a batch-global one before appending.
func mergeTrading212BatchMeta(existing trading212BatchMeta, parseResult ParseResult, baseIndex int) trading212BatchMeta {
	merged := existing
	for _, h := range parseResult.Meta.AccountHints {
		if !containsString(merged.AccountHints, h) {
			merged.AccountHints = append(merged.AccountHints, h)
		}
	}
	for _, h := range parseResult.Meta.CurrencyHints {
		if !containsString(merged.CurrencyHints, h) {
			merged.CurrencyHints = append(merged.CurrencyHints, h)
		}
	}
	if parseResult.Meta.DateFrom != "" && (merged.DateFrom == "" || parseResult.Meta.DateFrom < merged.DateFrom) {
		merged.DateFrom = parseResult.Meta.DateFrom
	}
	if parseResult.Meta.DateTo != "" && (merged.DateTo == "" || parseResult.Meta.DateTo > merged.DateTo) {
		merged.DateTo = parseResult.Meta.DateTo
	}
	for _, w := range parseResult.Warnings {
		merged.Warnings = append(merged.Warnings, ParseWarning{RowIndex: baseIndex + w.RowIndex, Message: w.Message})
	}
	return merged
}

// markTrading212FetchTerminalFailure records a terminal fetch failure two
// places the caller cares about: import_batches.status (the source of truth
// used by commit/discard/preview's "is this batch open" checks) AND
// source_meta_json's fetch_status (the field the frontend actually polls —
// see +page.svelte's pollFetchStatus, which never looks at batch.status).
// Writing only the former left the UI polling forever on a batch that had
// already failed on the backend.
func (s *ImportService) markTrading212FetchTerminalFailure(ctx context.Context, logger *slog.Logger, payload trading212FetchWorkPayload, fetchErr error, now string) {
	metaJSON, _ := json.Marshal(trading212BatchMeta{FetchStatus: "failed", Error: fetchErr.Error()})
	batch, err := s.repository.ImportBatchByID(ctx, BookID, payload.BatchID)
	if err == nil && batch.Status == "previewing" {
		if err := s.repository.UpdateImportBatchStatus(ctx, db.UpdateImportBatchStatusParams{
			BatchID:    payload.BatchID,
			Status:     "failed",
			EventKind:  "failed",
			DetailJSON: string(metaJSON),
			OccurredAt: now,
		}); err != nil {
			logger.WarnContext(ctx, "mark import batch failed", slog.Int64("batch_id", payload.BatchID), slog.Any("err", err))
		}
		if err := s.repository.UpdateImportBatchSourceMeta(ctx, payload.BatchID, string(metaJSON)); err != nil {
			logger.WarnContext(ctx, "mark import batch source meta failed", slog.Int64("batch_id", payload.BatchID), slog.Any("err", err))
		}
	} else if err != nil {
		logger.WarnContext(ctx, "load import batch for terminal failure", slog.Int64("batch_id", payload.BatchID), slog.Any("err", err))
	}

	if err := s.connectionService.MarkFetchFailed(ctx, payload.ConnectionID, now); err != nil {
		logger.WarnContext(ctx, "mark connection fetch failed", slog.Int64("connection_id", payload.ConnectionID), slog.Any("err", err))
	}
}

func toAdapterMovements(movements []trading212.Movement) []trading212Movement {
	out := make([]trading212Movement, len(movements))
	for i, m := range movements {
		out[i] = trading212Movement{
			ID:        m.ID,
			Type:      m.Type,
			Timestamp: m.Timestamp,
			Amount:    m.Amount,
			Currency:  m.Currency,
			Notes:     m.Notes,
		}
	}
	return out
}
