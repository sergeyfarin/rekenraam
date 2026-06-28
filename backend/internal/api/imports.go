package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

// --- Response types ---

type importBatchResponse struct {
	ID               int64  `json:"id"`
	BookID           int64  `json:"book_id"`
	SourceKind       string `json:"source_kind"`
	ProfileID        *int64 `json:"profile_id,omitempty"`
	Status           string `json:"status"`
	OriginalFilename string `json:"original_filename"`
	SourceMetaJSON   string `json:"source_meta"`
	CreatedAt        string `json:"created_at"`
}

type importStagedRowResponse struct {
	ID                     int64  `json:"id"`
	BatchID                int64  `json:"batch_id"`
	RowIndex               int    `json:"row_index"`
	DedupeFingerprint      string `json:"dedupe_fingerprint"`
	NormalizedJSON         string `json:"normalized"`
	RawJSON                string `json:"raw"`
	DedupeStatus           string `json:"dedupe_status"`
	ResolutionJSON         string `json:"resolution"`
	CommitStatus           string `json:"commit_status"`
	CommittedTransactionID *int64 `json:"committed_transaction_id,omitempty"`
	CommitError            string `json:"commit_error,omitempty"`
}

type parseWarningResponse struct {
	RowIndex int    `json:"row_index"`
	Message  string `json:"message"`
}

type sourceMetaResponse struct {
	AccountHints  []string `json:"account_hints"`
	CurrencyHints []string `json:"currency_hints"`
	DateFrom      string   `json:"date_from,omitempty"`
	DateTo        string   `json:"date_to,omitempty"`
}

type startImportResponse struct {
	Batch    importBatchResponse       `json:"batch"`
	Rows     []importStagedRowResponse `json:"rows"`
	Warnings []parseWarningResponse    `json:"warnings"`
	Meta     sourceMetaResponse        `json:"meta"`
}

type getImportBatchResponse struct {
	Batch      importBatchResponse       `json:"batch"`
	Rows       []importStagedRowResponse `json:"rows"`
	NextCursor *string                   `json:"next_cursor"`
}

type listImportBatchesResponse struct {
	Batches    []importBatchResponse `json:"batches"`
	NextCursor *string               `json:"next_cursor"`
}

type rowResolutionPatchRequest struct {
	RowID          int64  `json:"row_id"`
	DedupeStatus   string `json:"dedupe_status,omitempty"`
	ResolutionJSON string `json:"resolution,omitempty"`
}

type patchImportBatchRequest struct {
	RowResolutions []rowResolutionPatchRequest `json:"row_resolutions"`
}

type commitImportBatchRequest struct {
	ReconciliationOverride bool `json:"reconciliation_override"`
}

type commitImportBatchResponse struct {
	BatchID        int64  `json:"batch_id"`
	Status         string `json:"status"`
	TotalRows      int    `json:"total_rows"`
	CommittedCount int    `json:"committed_count"`
	SkippedCount   int    `json:"skipped_count"`
	FailedCount    int    `json:"failed_count"`
}

type previewCommitResponse struct {
	IncludableCount      int                           `json:"includable_count"`
	DuplicateCount       int                           `json:"duplicate_count"`
	ReconciliationIssues []reconciliationIssueResponse `json:"reconciliation_issues"`
}

type reconciliationIssueResponse struct {
	RowIndex      int     `json:"row_index"`
	CheckpointIDs []int64 `json:"checkpoint_ids"`
}

// --- Handlers ---

func startImport(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		const maxUploadSize = 50 << 20 // 50 MB
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid multipart form")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "file field is required")
			return
		}
		defer file.Close()

		fileBytes, err := io.ReadAll(io.LimitReader(file, maxUploadSize))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "failed to read file")
			return
		}

		result, err := importService.StartImport(r.Context(), app.StartImportInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			Input: app.RawInput{
				Filename:    header.Filename,
				ContentType: header.Header.Get("Content-Type"),
				Bytes:       fileBytes,
			},
		})
		if err != nil {
			writeImportServiceError(w, r, logger, "start import", err)
			return
		}

		writeJSON(w, http.StatusCreated, toStartImportResponse(result))
	}))
}

func getImportBatch(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}

		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}

		result, err := importService.GetImportBatch(r.Context(), app.GetImportBatchInput{
			BatchID: batchID,
			Limit:   limit,
			Cursor:  r.URL.Query().Get("cursor"),
		})
		if err != nil {
			writeImportServiceError(w, r, logger, "get import batch", err)
			return
		}

		var nextCursor *string
		if result.NextCursor != "" {
			nextCursor = &result.NextCursor
		}

		writeJSON(w, http.StatusOK, getImportBatchResponse{
			Batch:      toImportBatchResponse(result.Batch),
			Rows:       toImportStagedRowResponses(result.Rows),
			NextCursor: nextCursor,
		})
	}
}

func patchImportBatch(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}

		var request patchImportBatchRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		patches := make([]app.RowResolutionPatch, 0, len(request.RowResolutions))
		for _, p := range request.RowResolutions {
			resJSON := p.ResolutionJSON
			if resJSON == "" {
				resJSON = "{}"
			}
			dedupeStatus := p.DedupeStatus
			if dedupeStatus == "" {
				dedupeStatus = "new"
			}
			patches = append(patches, app.RowResolutionPatch{
				RowID:          p.RowID,
				DedupeStatus:   dedupeStatus,
				ResolutionJSON: resJSON,
			})
		}

		if err := importService.PatchImportBatch(r.Context(), app.PatchImportBatchInput{
			OwnerUserID:    owner.ID,
			AuthSessionID:  authenticatedSessionID(r),
			RequestID:      RequestIDFromContext(r.Context()),
			BatchID:        batchID,
			RowResolutions: patches,
		}); err != nil {
			writeImportServiceError(w, r, logger, "patch import batch", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
}

func previewCommitImportBatch(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}

		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}

		result, err := importService.PreviewCommit(r.Context(), app.PreviewCommitInput{
			OwnerUserID: owner.ID,
			BatchID:     batchID,
		})
		if err != nil {
			writeImportServiceError(w, r, logger, "preview commit", err)
			return
		}

		issues := make([]reconciliationIssueResponse, 0, len(result.ReconciliationIssues))
		for _, issue := range result.ReconciliationIssues {
			issues = append(issues, reconciliationIssueResponse{
				RowIndex:      issue.RowIndex,
				CheckpointIDs: issue.CheckpointIDs,
			})
		}

		writeJSON(w, http.StatusOK, previewCommitResponse{
			IncludableCount:      result.IncludableCount,
			DuplicateCount:       result.DuplicateCount,
			ReconciliationIssues: issues,
		})
	}
}

func commitImportBatch(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}

		var request commitImportBatchRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		result, err := importService.CommitImportBatch(r.Context(), app.CommitImportBatchInput{
			OwnerUserID:            owner.ID,
			AuthSessionID:          authenticatedSessionID(r),
			RequestID:              RequestIDFromContext(r.Context()),
			BatchID:                batchID,
			ReconciliationOverride: request.ReconciliationOverride,
		})
		if err != nil {
			writeImportServiceError(w, r, logger, "commit import batch", err)
			return
		}

		writeJSON(w, http.StatusOK, commitImportBatchResponse{
			BatchID:        result.BatchID,
			Status:         result.Status,
			TotalRows:      result.TotalRows,
			CommittedCount: result.CommittedCount,
			SkippedCount:   result.SkippedCount,
			FailedCount:    result.FailedCount,
		})
	}))
}

func discardImportBatch(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}

		if err := importService.DiscardImportBatch(r.Context(), app.DiscardImportBatchInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			BatchID:       batchID,
		}); err != nil {
			writeImportServiceError(w, r, logger, "discard import batch", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
}

func listImportBatches(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}

		result, err := importService.ListImportBatches(r.Context(), app.ListImportBatchesInput{
			Limit:  limit,
			Cursor: r.URL.Query().Get("cursor"),
		})
		if err != nil {
			writeImportServiceError(w, r, logger, "list import batches", err)
			return
		}

		var nextCursor *string
		if result.NextCursor != "" {
			nextCursor = &result.NextCursor
		}

		batches := make([]importBatchResponse, 0, len(result.Batches))
		for _, b := range result.Batches {
			batches = append(batches, toImportBatchResponse(b))
		}

		writeJSON(w, http.StatusOK, listImportBatchesResponse{
			Batches:    batches,
			NextCursor: nextCursor,
		})
	}
}

// --- Error handler ---

func writeImportServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, op string, err error) {
	switch {
	case errors.Is(err, app.ErrImportBatchNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import batch not found")
	case errors.Is(err, app.ErrImportBatchNotOpen):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "import batch is not in an open state")
	default:
		var ve app.ValidationError
		if errors.As(err, &ve) {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", ve.Message)
			return
		}
		writeServiceInternalError(w, r, logger, op, err)
	}
}

// --- ID parsing ---

func readImportBatchID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("batch_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "batch_id is invalid")
		return 0, false
	}
	return id, true
}

// --- Mapping helpers ---

func toImportBatchResponse(b app.ImportBatch) importBatchResponse {
	meta := b.SourceMetaJSON
	if meta == "" {
		meta = "{}"
	}
	return importBatchResponse{
		ID:               b.ID,
		BookID:           b.BookID,
		SourceKind:       b.SourceKind,
		ProfileID:        b.ProfileID,
		Status:           b.Status,
		OriginalFilename: b.OriginalFilename,
		SourceMetaJSON:   meta,
		CreatedAt:        b.CreatedAt,
	}
}

func toImportStagedRowResponse(row app.ImportStagedRow) importStagedRowResponse {
	normalized := row.NormalizedJSON
	if normalized == "" {
		normalized = "{}"
	}
	raw := row.RawJSON
	if raw == "" {
		raw = "{}"
	}
	resolution := row.ResolutionJSON
	if resolution == "" {
		resolution = "{}"
	}
	return importStagedRowResponse{
		ID:                     row.ID,
		BatchID:                row.BatchID,
		RowIndex:               row.RowIndex,
		DedupeFingerprint:      row.DedupeFingerprint,
		NormalizedJSON:         normalized,
		RawJSON:                raw,
		DedupeStatus:           row.DedupeStatus,
		ResolutionJSON:         resolution,
		CommitStatus:           row.CommitStatus,
		CommittedTransactionID: row.CommittedTransactionID,
		CommitError:            row.CommitError,
	}
}

func toImportStagedRowResponses(rows []app.ImportStagedRow) []importStagedRowResponse {
	out := make([]importStagedRowResponse, len(rows))
	for i, r := range rows {
		out[i] = toImportStagedRowResponse(r)
	}
	return out
}

func toStartImportResponse(result app.StartImportResult) startImportResponse {
	warnings := make([]parseWarningResponse, 0, len(result.Warnings))
	for _, w := range result.Warnings {
		warnings = append(warnings, parseWarningResponse{
			RowIndex: w.RowIndex,
			Message:  w.Message,
		})
	}

	accountHints := result.Meta.AccountHints
	if accountHints == nil {
		accountHints = []string{}
	}
	currencyHints := result.Meta.CurrencyHints
	if currencyHints == nil {
		currencyHints = []string{}
	}

	return startImportResponse{
		Batch:    toImportBatchResponse(result.Batch),
		Rows:     toImportStagedRowResponses(result.Rows),
		Warnings: warnings,
		Meta: sourceMetaResponse{
			AccountHints:  accountHints,
			CurrencyHints: currencyHints,
			DateFrom:      result.Meta.DateFrom,
			DateTo:        result.Meta.DateTo,
		},
	}
}

// rawJSONString returns s or "{}" if s is empty or invalid JSON.
func rawJSONString(s string) string {
	if s == "" {
		return "{}"
	}
	if !json.Valid([]byte(s)) {
		return "{}"
	}
	return s
}
