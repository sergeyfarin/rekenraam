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
// docs/plans/trading212-import-plan.md "Data model".
const trading212FetchWorkKind = "import.fetch.trading212"

// maxTrading212FetchAttempts bounds retries before a fetch is given up on as
// terminal. Like FX coverage (maxFXCoverageAttempts), an import fetch that
// keeps failing must eventually surface to the user as a failed batch rather
// than leave it "fetching" indefinitely.
const maxTrading212FetchAttempts = 8

// ErrImportFetchInProgress is returned when a connection already has an
// unfinished fetch (an existing previewing batch with fetch_status=fetching).
// The durable queue does not coalesce different connection/cursor payloads,
// so this guard lives at the service layer (see "Durable fetch worker" in
// the import plan).
var ErrImportFetchInProgress = errors.New("an import fetch is already in progress for this connection")

// trading212FetchStage identifies which of the three independently-paginated
// Trading 212 endpoints a work item processes. A whole logical fetch walks
// stages in this order (transactions -> orders -> dividends); each stage's
// own pagination/continuation is identical to Slice 3's original
// single-endpoint design (T-14) — B-T212-INVST only adds a stage transition
// once a stage's pagination naturally exhausts, rather than tripling the
// worker for three endpoints.
type trading212FetchStage string

const (
	trading212StageTransactions trading212FetchStage = "transactions"
	trading212StageOrders       trading212FetchStage = "orders"
	trading212StageDividends    trading212FetchStage = "dividends"
)

// trading212NextStage returns the stage after the given one, and whether the
// whole logical fetch is done (dividends was the last stage).
func trading212NextStage(stage trading212FetchStage) (next trading212FetchStage, done bool) {
	switch stage {
	case trading212StageTransactions:
		return trading212StageOrders, false
	case trading212StageOrders:
		return trading212StageDividends, false
	default:
		return "", true
	}
}

type trading212FetchWorkPayload struct {
	ConnectionID int64  `json:"connection_id"`
	BatchID      int64  `json:"batch_id"`
	Reason       string `json:"reason"` // "manual" | "continuation"

	// Stage is which endpoint this work item processes. Empty defaults to
	// trading212StageTransactions (the first stage of any fresh fetch).
	Stage trading212FetchStage `json:"stage,omitempty"`

	// TransactionsCursor/OrdersCursor/DividendsCursor: each endpoint's
	// incremental boundary for this whole logical fetch (fixed once that
	// stage starts — see Fetch's doc comment), seeded at fetch-start time
	// from the connection's saved cursors. Once a stage completes, its
	// field here is overwritten with the final value to persist, so by the
	// time the last stage (dividends) finishes, all three fields hold the
	// values to write back to the connection in one UpdateFetchCursor call.
	TransactionsCursor string `json:"transactions_cursor"`
	OrdersCursor       string `json:"orders_cursor"`
	DividendsCursor    string `json:"dividends_cursor"`

	// ResumePath/MaxCursorSoFar are scoped to the CURRENTLY ACTIVE stage
	// only — reset to zero value whenever Stage transitions to the next one.
	ResumePath     string `json:"resume_path,omitempty"`
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
		Incremental:   false,
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
		Incremental:   true,
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
	Connection ImportConnection
	// Incremental selects which cursors seed the fetch: false (StartOnlineImport)
	// starts all three endpoints from full history; true (RefreshImportConnection)
	// starts each from the connection's own saved per-endpoint cursor.
	Incremental   bool
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

	var transactionsCursor, ordersCursor, dividendsCursor string
	if params.Incremental {
		transactionsCursor = params.Connection.TransactionsCursor
		ordersCursor = params.Connection.OrdersCursor
		dividendsCursor = params.Connection.DividendsCursor
	}

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
			ConnectionID:       params.Connection.ID,
			BatchID:            batchID,
			Reason:             params.Reason,
			Stage:              trading212StageTransactions,
			TransactionsCursor: transactionsCursor,
			OrdersCursor:       ordersCursor,
			DividendsCursor:    dividendsCursor,
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

// runTrading212Fetch performs one fetch attempt for the payload's active
// stage: load connection + secret, call that stage's fetcher endpoint,
// parse via the registered Trading212Adapter (reusing the same
// RawInput.Bytes contract every adapter uses), run the investment
// resolution pass over any order-fill/dividend rows, stage rows through the
// shared stageParseResult pipeline, and persist progress. Returns nil on
// success (including the no-op case where the batch was closed by the user
// mid-fetch, the case where more pages remain in this stage and a
// continuation was enqueued, and the case where this stage finished and a
// work item for the next stage was enqueued instead of finishing the whole
// fetch).
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

	fetcher := trading212.NewFetcher(s.httpClient, s.trading212BaseURL)

	stage := payload.Stage
	if stage == "" {
		stage = trading212StageTransactions
	}

	var rawBytes []byte
	var hasMore bool
	var nextPageToken string
	var chunkCursor string // this HTTP call's own Fetch-family result.Cursor

	switch stage {
	case trading212StageTransactions:
		result, ferr := fetcher.Fetch(ctx, apiKey, payload.TransactionsCursor, payload.ResumePath)
		if ferr != nil {
			return ferr
		}
		hasMore, nextPageToken, chunkCursor = result.HasMore, result.NextPageToken, result.Cursor
		rawBytes, err = json.Marshal(trading212FetchPayload{ConnectionID: payload.ConnectionID, Movements: toAdapterMovements(result.Movements)})
	case trading212StageOrders:
		result, ferr := fetcher.FetchOrders(ctx, apiKey, payload.OrdersCursor, payload.ResumePath)
		if ferr != nil {
			return ferr
		}
		hasMore, nextPageToken, chunkCursor = result.HasMore, result.NextPageToken, result.Cursor
		rawBytes, err = json.Marshal(trading212FetchPayload{ConnectionID: payload.ConnectionID, OrderFills: toAdapterOrderFills(result.Fills)})
	case trading212StageDividends:
		result, ferr := fetcher.FetchDividends(ctx, apiKey, payload.DividendsCursor, payload.ResumePath)
		if ferr != nil {
			return ferr
		}
		hasMore, nextPageToken, chunkCursor = result.HasMore, result.NextPageToken, result.Cursor
		rawBytes, err = json.Marshal(trading212FetchPayload{ConnectionID: payload.ConnectionID, Dividends: toAdapterDividends(result.Dividends)})
	default:
		return fmt.Errorf("unknown trading212 fetch stage %q", stage)
	}
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

	// Instrument/holding-account resolution for order-fill/dividend rows
	// (B-T212-INVST) happens entirely at commit time (import_service.go
	// CommitImportBatch), not here — a fetch only stages rows into preview,
	// and resolution can create a new instrument/holding account (see the
	// discard-orphan concern in docs/plans/import-connection-accounts-plan.md);
	// only commit is allowed to create durable state. The preview UI shows
	// these rows with their raw ticker/side/quantity but not yet a
	// resolved/proposed instrument name — a documented, deferred UX
	// enhancement, not a correctness gap.

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

	// The running max across every chunk of this STAGE's logical fetch so
	// far, not just this chunk's chunkCursor — mirrors Fetch's own
	// cursor/MaxCursorSoFar split (see its doc comment).
	runningMax := payload.MaxCursorSoFar
	if chunkCursor > runningMax {
		runningMax = chunkCursor
	}

	if hasMore {
		// More pages remain in this stage than a single Fetch-family call
		// will follow. Persist progress and enqueue a continuation work item
		// for the SAME stage instead of marking the batch ready.
		merged.FetchStatus = "fetching"
		metaJSON, _ := json.Marshal(merged)
		if err := s.repository.UpdateImportBatchSourceMeta(ctx, payload.BatchID, string(metaJSON)); err != nil {
			return fmt.Errorf("update batch source meta: %w", err)
		}

		continuation := payload
		continuation.Reason = "continuation"
		continuation.ResumePath = nextPageToken
		continuation.MaxCursorSoFar = runningMax
		continuationJSON, err := json.Marshal(continuation)
		if err != nil {
			return fmt.Errorf("marshal continuation fetch payload: %w", err)
		}
		if _, err := s.backgroundWork.EnqueueBackgroundWork(ctx, BookID, trading212FetchWorkKind, string(continuationJSON), now); err != nil {
			return fmt.Errorf("enqueue continuation fetch: %w", err)
		}
		return nil
	}

	// This stage naturally exhausted. finalCursor is what to persist for
	// THIS stage — the running max seen, or (if nothing was seen this run)
	// the boundary it started with, so an empty run never regresses the
	// saved cursor.
	startingCursor := payload.TransactionsCursor
	if stage == trading212StageOrders {
		startingCursor = payload.OrdersCursor
	} else if stage == trading212StageDividends {
		startingCursor = payload.DividendsCursor
	}
	finalCursor := runningMax
	if finalCursor == "" {
		finalCursor = startingCursor
	}

	nextStage, allDone := trading212NextStage(stage)
	if !allDone {
		// Move to the next stage within the same logical fetch. The next
		// stage starts fresh (no resume path/running max of its own yet)
		// from whatever its own saved cursor already was on the payload.
		merged.FetchStatus = "fetching"
		metaJSON, _ := json.Marshal(merged)
		if err := s.repository.UpdateImportBatchSourceMeta(ctx, payload.BatchID, string(metaJSON)); err != nil {
			return fmt.Errorf("update batch source meta: %w", err)
		}

		next := payload
		next.Reason = "continuation"
		next.Stage = nextStage
		next.ResumePath = ""
		next.MaxCursorSoFar = ""
		switch stage {
		case trading212StageTransactions:
			next.TransactionsCursor = finalCursor
		case trading212StageOrders:
			next.OrdersCursor = finalCursor
		}
		nextJSON, err := json.Marshal(next)
		if err != nil {
			return fmt.Errorf("marshal next-stage fetch payload: %w", err)
		}
		if _, err := s.backgroundWork.EnqueueBackgroundWork(ctx, BookID, trading212FetchWorkKind, string(nextJSON), now); err != nil {
			return fmt.Errorf("enqueue next-stage fetch: %w", err)
		}
		return nil
	}

	// All three stages (transactions -> orders -> dividends) are done.
	merged.FetchStatus = "ready"
	metaJSON, _ := json.Marshal(merged)
	if err := s.repository.UpdateImportBatchSourceMeta(ctx, payload.BatchID, string(metaJSON)); err != nil {
		return fmt.Errorf("update batch source meta: %w", err)
	}

	// stage here is always trading212StageDividends when allDone is true
	// (see trading212NextStage), so finalCursor is the dividends cursor;
	// transactions/orders were already folded into payload during their own
	// stage transitions above.
	if err := s.connectionService.UpdateFetchCursor(ctx, payload.ConnectionID, payload.TransactionsCursor, payload.OrdersCursor, finalCursor, "ready", now); err != nil {
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

func toAdapterOrderFills(fills []trading212.OrderFill) []trading212OrderFill {
	out := make([]trading212OrderFill, len(fills))
	for i, f := range fills {
		out[i] = trading212OrderFill{
			FillID:           f.FillID,
			OrderID:          f.OrderID,
			Ticker:           f.Ticker,
			ISIN:             f.ISIN,
			Side:             f.Side,
			Quantity:         f.Quantity,
			Price:            f.Price,
			Currency:         f.Currency,
			FilledAt:         f.FilledAt,
			NetValue:         f.NetValue,
			NetValueCurrency: f.NetValueCurrency,
		}
	}
	return out
}

func toAdapterDividends(dividends []trading212.Dividend) []trading212Dividend {
	out := make([]trading212Dividend, len(dividends))
	for i, d := range dividends {
		out[i] = trading212Dividend{
			Reference: d.Reference,
			Ticker:    d.Ticker,
			ISIN:      d.ISIN,
			Quantity:  d.Quantity,
			Amount:    d.Amount,
			Currency:  d.Currency,
			PaidOn:    d.PaidOn,
			Type:      d.Type,
		}
	}
	return out
}
