package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"rekenraam/backend/internal/app"
)

// --- Response types ---

type importBatchResponse struct {
	ID               int64  `json:"id"`
	BookID           int64  `json:"book_id"`
	SourceKind       string `json:"source_kind"`
	ProfileID        *int64 `json:"profile_id,omitempty"`
	ConnectionID     *int64 `json:"connection_id,omitempty"`
	Status           string `json:"status"`
	OriginalFilename string `json:"original_filename"`
	SourceMetaJSON   string `json:"source_meta"`
	CreatedAt        string `json:"created_at"`
}

type importStagedRowResponse struct {
	ID                     int64                        `json:"id"`
	BatchID                int64                        `json:"batch_id"`
	RowIndex               int                          `json:"row_index"`
	DedupeFingerprint      string                       `json:"dedupe_fingerprint"`
	NormalizedJSON         string                       `json:"normalized"`
	RawJSON                string                       `json:"raw"`
	DedupeStatus           string                       `json:"dedupe_status"`
	SourceChanged          bool                         `json:"source_changed"`
	SourceBuyOperation     bool                         `json:"source_buy_operation"`
	SourceSaleOperation    bool                         `json:"source_sale_operation"`
	SourceTransactionID    *int64                       `json:"source_transaction_id,omitempty"`
	ResolutionJSON         string                       `json:"resolution"`
	CommitStatus           string                       `json:"commit_status"`
	CommittedIdentityID    *int64                       `json:"committed_identity_id,omitempty"`
	CommittedTransactionID *int64                       `json:"committed_transaction_id,omitempty"`
	CommitEffects          []importCommitEffectResponse `json:"commit_effects"`
	CommitError            string                       `json:"commit_error,omitempty"`
}

type importCommitEffectResponse struct {
	EffectSeq     int64  `json:"effect_seq"`
	OperationID   *int64 `json:"operation_id,omitempty"`
	TransactionID *int64 `json:"transaction_id,omitempty"`
}

type parseWarningResponse struct {
	RowIndex int    `json:"row_index"`
	Code     string `json:"code,omitempty"`
	Value    string `json:"value,omitempty"`
	Message  string `json:"message"`
}

type sourceMetaResponse struct {
	AccountHints       []string `json:"account_hints"`
	CurrencyHints      []string `json:"currency_hints"`
	DateFrom           string   `json:"date_from,omitempty"`
	DateTo             string   `json:"date_to,omitempty"`
	TextEncoding       string   `json:"text_encoding,omitempty"`
	EncodingSource     string   `json:"encoding_source,omitempty"`
	EncodingConfidence int      `json:"encoding_confidence,omitempty"`
}

type startImportResponse struct {
	Batch    importBatchResponse       `json:"batch"`
	Rows     []importStagedRowResponse `json:"rows"`
	Warnings []parseWarningResponse    `json:"warnings"`
	Meta     sourceMetaResponse        `json:"meta"`
}

type analyzeCSVImportResponse struct {
	Headers            []string `json:"headers"`
	Delimiter          string   `json:"delimiter"`
	TextEncoding       string   `json:"text_encoding"`
	EncodingSource     string   `json:"encoding_source"`
	EncodingConfidence int      `json:"encoding_confidence"`
}

type startOnlineImportRequest struct {
	Source       string `json:"source"`
	ConnectionID int64  `json:"connection_id"`
}

// startOnlineImportResponse is returned for both the JSON POST /imports
// branch and POST /import-connections/{id}/refresh: the batch has been
// created but the durable worker hasn't fetched/staged rows yet, so there is
// nothing else to return. The caller polls GET /imports/{id} until
// batch.source_meta's fetch_status flips to "ready".
type startOnlineImportResponse struct {
	Batch importBatchResponse `json:"batch"`
}

type getImportBatchResponse struct {
	Batch      importBatchResponse       `json:"batch"`
	Rows       []importStagedRowResponse `json:"rows"`
	Warnings   []parseWarningResponse    `json:"warnings"`
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
	// The override names the preview's reconciliation_checkpoints; each row
	// commits only if its own set is contained in them (#305).
	reconciliationOverrideFields
	// GainImpactAcknowledgements echo preview-commit gain tokens per row (T-126).
	GainImpactAcknowledgements []rowGainImpactAcknowledgementRequest `json:"gain_impact_acknowledgements,omitempty"`
}

type rowGainImpactAcknowledgementRequest struct {
	RowID           int64  `json:"row_id"`
	Acknowledgement string `json:"acknowledgement"`
}

type correctTrading212SourceRequest struct {
	Reason string `json:"reason"`
	reconciliationOverrideFields
	GainImpactAcknowledgement string `json:"gain_impact_acknowledgement,omitempty"`
}

type commitImportBatchResponse struct {
	DividendReviewRowIDs []int64 `json:"dividend_review_row_ids"`
	BatchID              int64   `json:"batch_id"`
	Status               string  `json:"status"`
	TotalRows            int     `json:"total_rows"`
	CommittedCount       int     `json:"committed_count"`
	SkippedCount         int     `json:"skipped_count"`
	FailedCount          int     `json:"failed_count"`
	// GainReviewRowIDs stay pending until their current gain changes are
	// acknowledged in another preview and commit.
	GainReviewRowIDs []int64 `json:"gain_review_row_ids"`
	// ReconciliationHoldRowIDs would cross a reconciliation checkpoint and
	// stay pending until a commit with reconciliation_override (#301).
	ReconciliationHoldRowIDs []int64 `json:"reconciliation_hold_row_ids"`
	// ReconciliationStaleRowIDs would invalidate a checkpoint the override
	// did not name; they stay pending, with nothing invalidated, until
	// another preview and commit (#305).
	ReconciliationStaleRowIDs []int64 `json:"reconciliation_stale_row_ids"`
	// LinkedCount rows were committed as links to existing postings (#485).
	LinkedCount int `json:"linked_count"`
	// MatchReviewRowIDs match existing postings with no standing decision
	// and stay pending until the owner decides (#485).
	MatchReviewRowIDs []int64 `json:"match_review_row_ids"`
}

type previewCommitResponse struct {
	IncludableCount      int                           `json:"includable_count"`
	DuplicateCount       int                           `json:"duplicate_count"`
	ReconciliationIssues []reconciliationIssueResponse `json:"reconciliation_issues"`
	// ReconciliationCheckpoints is the union a commit's override names (#305).
	ReconciliationCheckpoints []checkpointImpactResponse `json:"reconciliation_checkpoints"`
	GainImpacts               []rowGainImpactResponse    `json:"gain_impacts"`
	MatchReviews              []rowMatchReviewResponse   `json:"match_reviews"`
	LinkCount                 int                        `json:"link_count"`
}

type importMatchChoiceResponse struct {
	Decision      string `json:"decision"`
	TransactionID int64  `json:"transaction_id,omitempty"`
	LineKey       string `json:"line_key,omitempty"`
	PostedOn      string `json:"posted_on,omitempty"`
}

type importMatchCandidateResponse struct {
	TransactionID   int64  `json:"transaction_id"`
	LineKey         string `json:"line_key"`
	TransactionDate string `json:"transaction_date"`
	TransactionKind string `json:"transaction_kind"`
	PostedOn        string `json:"posted_on"`
	DayOffset       int    `json:"day_offset"`
	AccountID       int64  `json:"account_id"`
	CommodityID     int64  `json:"commodity_id"`
	QuantityValue   string `json:"quantity_value"`
	QuantityScale   int    `json:"quantity_scale"`
	PayeeName       string `json:"payee_name"`
	Description     string `json:"description"`
}

type rowMatchReviewResponse struct {
	RowID         int64                          `json:"row_id"`
	RowIndex      int                            `json:"row_index"`
	Candidates    []importMatchCandidateResponse `json:"candidates"`
	Choice        *importMatchChoiceResponse     `json:"choice"`
	NeedsDecision bool                           `json:"needs_decision"`
	WindowDays    int                            `json:"window_days"`
}

func toRowMatchReviewResponses(reviews []app.RowMatchReview) []rowMatchReviewResponse {
	responses := make([]rowMatchReviewResponse, 0, len(reviews))
	for _, review := range reviews {
		candidates := make([]importMatchCandidateResponse, 0, len(review.Candidates))
		for _, candidate := range review.Candidates {
			candidates = append(candidates, importMatchCandidateResponse{
				TransactionID: candidate.TransactionID, LineKey: candidate.LineKey,
				TransactionDate: candidate.TransactionDate, TransactionKind: candidate.TransactionKind,
				PostedOn: candidate.PostedOn, DayOffset: candidate.DayOffset,
				AccountID: candidate.AccountID, CommodityID: candidate.CommodityID,
				QuantityValue: string(candidate.QuantityValue), QuantityScale: candidate.QuantityScale,
				PayeeName: candidate.PayeeName, Description: candidate.Description,
			})
		}
		response := rowMatchReviewResponse{RowID: review.RowID, RowIndex: review.RowIndex, Candidates: candidates,
			NeedsDecision: review.NeedsDecision, WindowDays: app.ImportMatchWindowDays}
		if review.Choice != nil {
			response.Choice = &importMatchChoiceResponse{Decision: review.Choice.Decision, TransactionID: review.Choice.TransactionID,
				LineKey: review.Choice.LineKey, PostedOn: review.Choice.PostedOn}
		}
		responses = append(responses, response)
	}
	return responses
}

type rowGainImpactResponse struct {
	RowID      int64              `json:"row_id"`
	RowIndex   int                `json:"row_index"`
	GainImpact gainImpactResponse `json:"gain_impact"`
}

type reconciliationIssueResponse struct {
	RowIndex      int     `json:"row_index"`
	CheckpointIDs []int64 `json:"checkpoint_ids"`
}

type importProfileResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	AdapterKind string `json:"adapter_kind"`
	ConfigJSON  string `json:"config"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type listImportProfilesResponse struct {
	Profiles []importProfileResponse `json:"profiles"`
}
type createImportProfileRequest struct {
	Name        string `json:"name"`
	AdapterKind string `json:"adapter_kind"`
	ConfigJSON  string `json:"config"`
}
type updateImportProfileRequest struct {
	Name       *string `json:"name"`
	ConfigJSON *string `json:"config"`
}

type importRuleResponse struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Priority     int     `json:"priority"`
	Enabled      bool    `json:"enabled"`
	MatchField   string  `json:"match_field"`
	ContainsText string  `json:"contains_text"`
	CategoryID   *int64  `json:"category_id,omitempty"`
	PayeeID      *int64  `json:"payee_id,omitempty"`
	TagIDs       []int64 `json:"tag_ids"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type listImportRulesResponse struct {
	Rules []importRuleResponse `json:"rules"`
}

type createImportRuleRequest struct {
	Name         string  `json:"name"`
	Priority     *int    `json:"priority"`
	Enabled      *bool   `json:"enabled"`
	MatchField   string  `json:"match_field"`
	ContainsText string  `json:"contains_text"`
	CategoryID   *int64  `json:"category_id"`
	PayeeID      *int64  `json:"payee_id"`
	TagIDs       []int64 `json:"tag_ids"`
}

type updateImportRuleRequest struct {
	Name          *string  `json:"name"`
	Priority      *int     `json:"priority"`
	Enabled       *bool    `json:"enabled"`
	MatchField    *string  `json:"match_field"`
	ContainsText  *string  `json:"contains_text"`
	CategoryID    *int64   `json:"category_id"`
	ClearCategory bool     `json:"clear_category"`
	PayeeID       *int64   `json:"payee_id"`
	ClearPayee    bool     `json:"clear_payee"`
	TagIDs        *[]int64 `json:"tag_ids"`
}

// --- Handlers ---

func startImport(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		// Content-negotiated: application/json starts an online (fetch-driven)
		// import from a connection; multipart/form-data is the existing
		// file-upload path.
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			var req startOnlineImportRequest
			if err := decodeJSONBody(r, &req); err != nil {
				writeDecodeError(w, err)
				return
			}
			if req.Source != "trading212" {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "source must be \"trading212\"")
				return
			}
			result, err := importService.StartOnlineImport(r.Context(), app.StartOnlineImportInput{
				OwnerUserID:   owner.ID,
				AuthSessionID: authenticatedSessionID(r),
				RequestID:     RequestIDFromContext(r.Context()),
				ConnectionID:  req.ConnectionID,
			})
			if err != nil {
				writeImportServiceError(w, r, logger, "start online import", err)
				return
			}
			writeJSON(w, http.StatusAccepted, startOnlineImportResponse{Batch: toImportBatchResponse(result.Batch)})
			return
		}

		rawInput, err := readImportUpload(w, r)
		if err != nil {
			writeImportUploadError(w, err)
			return
		}

		var profileID *int64
		if raw := strings.TrimSpace(r.FormValue("profile_id")); raw != "" {
			parsed, parseErr := strconv.ParseInt(raw, 10, 64)
			if parseErr != nil || parsed <= 0 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "profile_id is invalid")
				return
			}
			profileID = &parsed
		}

		format := strings.TrimSpace(r.FormValue("format"))
		var holdingID, cashID int64
		if format == "crypto_csv" {
			var parseErr error
			holdingID, parseErr = strconv.ParseInt(r.FormValue("holding_account_id"), 10, 64)
			if parseErr == nil {
				cashID, parseErr = strconv.ParseInt(r.FormValue("cash_account_id"), 10, 64)
			}
			if parseErr != nil || holdingID <= 0 || cashID <= 0 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "crypto holding and settlement accounts are required")
				return
			}
		}

		result, err := importService.StartImport(r.Context(), app.StartImportInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			Input:         rawInput,
			Format:        format, HoldingAccountID: holdingID, CashAccountID: cashID,
			ProfileID: profileID,
		})
		if err != nil {
			writeImportServiceError(w, r, logger, "start import", err)
			return
		}

		writeJSON(w, http.StatusCreated, toStartImportResponse(result))
	}))
}

func analyzeCSVImport(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedMutationOwner(w, r); !ok {
			return
		}
		rawInput, err := readImportUpload(w, r)
		if err != nil {
			writeImportUploadError(w, err)
			return
		}
		result, err := importService.AnalyzeCSV(r.Context(), rawInput, r.FormValue("delimiter"))
		if err != nil {
			writeImportServiceError(w, r, logger, "analyze csv import", err)
			return
		}
		writeJSON(w, http.StatusOK, analyzeCSVImportResponse{
			Headers:            result.Headers,
			Delimiter:          result.Delimiter,
			TextEncoding:       result.TextEncoding,
			EncodingSource:     result.EncodingSource,
			EncodingConfidence: result.EncodingConfidence,
		})
	}))
}

const (
	maxImportUploadSize      = 50 << 20                        // 50 MiB per file, inclusive.
	maxImportRequestSize     = maxImportUploadSize + (1 << 20) // Multipart overhead allowance.
	maxImportMultipartMemory = 1 << 20                         // Larger files spill to temporary storage.
)

var errImportUploadTooLarge = errors.New("import upload exceeds resource limits (50 MiB file, 51 MiB request)")

func writeImportUploadError(w http.ResponseWriter, err error) {
	if errors.Is(err, errImportUploadTooLarge) {
		writeAPIError(w, http.StatusRequestEntityTooLarge, "IMPORT_UPLOAD_TOO_LARGE", errImportUploadTooLarge.Error())
		return
	}
	writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
}

func importMultipartError(err error) error {
	var sizeErr *http.MaxBytesError
	if errors.As(err, &sizeErr) || errors.Is(err, multipart.ErrMessageTooLarge) {
		return errImportUploadTooLarge
	}
	return errors.New("invalid multipart form")
}

func readImportUpload(w http.ResponseWriter, r *http.Request) (app.RawInput, error) {
	// Content-Length is only an early rejection. The reader also bounds bodies
	// with unknown or inaccurate lengths, including all fields and extra parts.
	r.Body = http.MaxBytesReader(w, r.Body, maxImportRequestSize)
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	if r.ContentLength > maxImportRequestSize {
		return app.RawInput{}, errImportUploadTooLarge
	}
	if err := r.ParseMultipartForm(maxImportMultipartMemory); err != nil {
		return app.RawInput{}, importMultipartError(err)
	}
	// Multipart parsing stops at its final boundary. Consume any epilogue through
	// the same bounded reader before allowing adapter parsing or batch creation.
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		return app.RawInput{}, importMultipartError(err)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return app.RawInput{}, errors.New("file field is required")
	}
	defer file.Close()
	if header.Size > maxImportUploadSize {
		return app.RawInput{}, errImportUploadTooLarge
	}
	fileBytes, err := io.ReadAll(io.LimitReader(file, maxImportUploadSize+1))
	if err != nil {
		return app.RawInput{}, errors.New("failed to read file")
	}
	if len(fileBytes) > maxImportUploadSize {
		return app.RawInput{}, errImportUploadTooLarge
	}
	return app.RawInput{
		Filename:     header.Filename,
		ContentType:  header.Header.Get("Content-Type"),
		Bytes:        fileBytes,
		TextEncoding: strings.TrimSpace(r.FormValue("text_encoding")),
	}, nil
}

func listImportProfiles(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		profiles, err := importService.ListImportProfiles(r.Context())
		if err != nil {
			writeImportServiceError(w, r, logger, "list import profiles", err)
			return
		}
		response := make([]importProfileResponse, 0, len(profiles))
		for _, profile := range profiles {
			response = append(response, toImportProfileResponse(profile))
		}
		writeJSON(w, http.StatusOK, listImportProfilesResponse{Profiles: response})
	}
}

func createImportProfile(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request createImportProfileRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		profile, err := importService.CreateImportProfile(r.Context(), app.CreateImportProfileInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), Name: request.Name, AdapterKind: request.AdapterKind, ConfigJSON: request.ConfigJSON})
		if err != nil {
			writeImportServiceError(w, r, logger, "create import profile", err)
			return
		}
		writeJSON(w, http.StatusCreated, toImportProfileResponse(profile))
	}))
}

func updateImportProfile(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		profileID, ok := readImportProfileID(w, r)
		if !ok {
			return
		}
		var request updateImportProfileRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		profile, err := importService.UpdateImportProfile(r.Context(), app.UpdateImportProfileInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), ProfileID: profileID, Name: request.Name, ConfigJSON: request.ConfigJSON})
		if err != nil {
			writeImportServiceError(w, r, logger, "update import profile", err)
			return
		}
		writeJSON(w, http.StatusOK, toImportProfileResponse(profile))
	}))
}

func deleteImportProfile(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		profileID, ok := readImportProfileID(w, r)
		if !ok {
			return
		}
		if err := importService.DeleteImportProfile(r.Context(), app.DeleteImportProfileInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), ProfileID: profileID}); err != nil {
			writeImportServiceError(w, r, logger, "delete import profile", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func listImportRules(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		rules, err := importService.ListImportRules(r.Context())
		if err != nil {
			writeImportServiceError(w, r, logger, "list import rules", err)
			return
		}
		response := make([]importRuleResponse, 0, len(rules))
		for _, rule := range rules {
			response = append(response, toImportRuleResponse(rule))
		}
		writeJSON(w, http.StatusOK, listImportRulesResponse{Rules: response})
	}
}

func createImportRule(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request createImportRuleRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		priority := 100
		if request.Priority != nil {
			priority = *request.Priority
		}
		enabled := true
		if request.Enabled != nil {
			enabled = *request.Enabled
		}
		rule, err := importService.CreateImportRule(r.Context(), app.CreateImportRuleInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			Name: request.Name, Priority: priority, Enabled: enabled, MatchField: request.MatchField,
			ContainsText: request.ContainsText, CategoryID: request.CategoryID, PayeeID: request.PayeeID, TagIDs: request.TagIDs,
		})
		if err != nil {
			writeImportServiceError(w, r, logger, "create import rule", err)
			return
		}
		writeJSON(w, http.StatusCreated, toImportRuleResponse(rule))
	}))
}

func updateImportRule(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		ruleID, ok := readImportRuleID(w, r)
		if !ok {
			return
		}
		var request updateImportRuleRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		rule, err := importService.UpdateImportRule(r.Context(), app.UpdateImportRuleInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), RuleID: ruleID,
			Name: request.Name, Priority: request.Priority, Enabled: request.Enabled, MatchField: request.MatchField,
			ContainsText: request.ContainsText, CategoryID: request.CategoryID, ClearCategory: request.ClearCategory,
			PayeeID: request.PayeeID, ClearPayee: request.ClearPayee, TagIDs: request.TagIDs,
		})
		if err != nil {
			writeImportServiceError(w, r, logger, "update import rule", err)
			return
		}
		writeJSON(w, http.StatusOK, toImportRuleResponse(rule))
	}))
}

func deleteImportRule(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		ruleID, ok := readImportRuleID(w, r)
		if !ok {
			return
		}
		if err := importService.DeleteImportRule(r.Context(), app.DeleteImportRuleInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), RuleID: ruleID}); err != nil {
			writeImportServiceError(w, r, logger, "delete import rule", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
			Warnings:   toParseWarningResponses(result.Warnings),
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
		// Omitted fields are defaulted by the service (dedupe_status "new",
		// resolution "{}").
		for _, p := range request.RowResolutions {
			patches = append(patches, app.RowResolutionPatch{
				RowID:          p.RowID,
				DedupeStatus:   p.DedupeStatus,
				ResolutionJSON: p.ResolutionJSON,
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

		gainImpacts := make([]rowGainImpactResponse, 0, len(result.GainImpacts))
		for _, row := range result.GainImpacts {
			impact, err := toGainImpactResponse(&row.GainImpact)
			if err != nil {
				writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
				return
			}
			gainImpacts = append(gainImpacts, rowGainImpactResponse{RowID: row.RowID, RowIndex: row.RowIndex, GainImpact: *impact})
		}

		writeJSON(w, http.StatusOK, previewCommitResponse{
			IncludableCount:      result.IncludableCount,
			DuplicateCount:       result.DuplicateCount,
			ReconciliationIssues: issues,
			ReconciliationCheckpoints: toReconciliationImpactResponse(app.ReconciliationImpact{
				AffectedCheckpoints: result.ReconciliationCheckpoints,
			}).AffectedCheckpoints,
			GainImpacts:  gainImpacts,
			MatchReviews: toRowMatchReviewResponses(result.MatchReviews),
			LinkCount:    result.LinkCount,
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

		gainAcknowledgements := make(map[int64]string, len(request.GainImpactAcknowledgements))
		for _, acknowledgement := range request.GainImpactAcknowledgements {
			gainAcknowledgements[acknowledgement.RowID] = acknowledgement.Acknowledgement
		}
		result, err := importService.CommitImportBatch(r.Context(), app.CommitImportBatchInput{
			OwnerUserID:                owner.ID,
			AuthSessionID:              authenticatedSessionID(r),
			RequestID:                  RequestIDFromContext(r.Context()),
			BatchID:                    batchID,
			ReconciliationOverride:     request.checkpointOverride(),
			GainImpactAcknowledgements: gainAcknowledgements,
		})
		if err != nil {
			writeImportServiceError(w, r, logger, "commit import batch", err)
			return
		}

		gainReviewRowIDs := result.GainReviewRowIDs
		if gainReviewRowIDs == nil {
			gainReviewRowIDs = []int64{}
		}
		dividendReviewRowIDs := result.DividendReviewRowIDs
		if dividendReviewRowIDs == nil {
			dividendReviewRowIDs = []int64{}
		}
		reconciliationHoldRowIDs := result.ReconciliationHoldRowIDs
		if reconciliationHoldRowIDs == nil {
			reconciliationHoldRowIDs = []int64{}
		}
		reconciliationStaleRowIDs := result.ReconciliationStaleRowIDs
		if reconciliationStaleRowIDs == nil {
			reconciliationStaleRowIDs = []int64{}
		}
		matchReviewRowIDs := result.MatchReviewRowIDs
		if matchReviewRowIDs == nil {
			matchReviewRowIDs = []int64{}
		}
		writeJSON(w, http.StatusOK, commitImportBatchResponse{
			BatchID:              result.BatchID,
			Status:               result.Status,
			TotalRows:            result.TotalRows,
			CommittedCount:       result.CommittedCount,
			SkippedCount:         result.SkippedCount,
			FailedCount:          result.FailedCount,
			GainReviewRowIDs:     gainReviewRowIDs,
			DividendReviewRowIDs: dividendReviewRowIDs,

			ReconciliationHoldRowIDs:  reconciliationHoldRowIDs,
			ReconciliationStaleRowIDs: reconciliationStaleRowIDs,
			LinkedCount:               result.LinkedCount,
			MatchReviewRowIDs:         matchReviewRowIDs,
		})
	}))
}

func trading212BuyCorrectionReconciliationImpact(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}
		rowID, ok := readPathInt64(w, r, "row_id", "row id")
		if !ok {
			return
		}
		var request correctTrading212SourceRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := importService.Trading212BuyCorrectionReconciliationImpact(r.Context(), app.CorrectTrading212BuyInput{
			OwnerUserID: owner.ID, BatchID: batchID, RowID: rowID, Reason: request.Reason,
		})
		if err != nil {
			switch {
			case errors.Is(err, app.ErrImportBatchNotFound):
				writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import batch not found")
			case errors.Is(err, app.ErrImportSourceCorrectionConflict):
				writeAPIError(w, http.StatusConflict, "CONFLICT", "source buy revision is no longer eligible")
			default:
				writeInvestmentServiceError(w, r, logger, "preview Trading 212 buy source correction", err)
			}
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func correctTrading212Buy(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}
		rowID, ok := readPathInt64(w, r, "row_id", "row id")
		if !ok {
			return
		}
		var request correctTrading212SourceRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := importService.CorrectTrading212Buy(r.Context(), app.CorrectTrading212BuyInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
			RequestID: RequestIDFromContext(r.Context()), BatchID: batchID, RowID: rowID,
			Reason: request.Reason, ReconciliationOverride: request.checkpointOverride(),
			GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			switch {
			case errors.Is(err, app.ErrImportBatchNotFound):
				writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import batch not found")
			case errors.Is(err, app.ErrImportSourceCorrectionConflict):
				writeAPIError(w, http.StatusConflict, "CONFLICT", "source buy revision is no longer eligible")
			default:
				writeInvestmentServiceError(w, r, logger, "correct Trading 212 buy source", err)
			}
			return
		}
		writeJSON(w, http.StatusCreated, investmentBuyReplacementResponse{
			InverseTransaction:     toTransactionResponse(result.Inverse),
			Replacement:            toInvestmentTradeResponse(result.Replacement),
			CorrectedTransactionID: result.CorrectedTransactionID,
		})
	}))
}

func trading212SaleCorrectionReconciliationImpact(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}
		rowID, ok := readPathInt64(w, r, "row_id", "row id")
		if !ok {
			return
		}
		var request correctTrading212SourceRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := importService.Trading212SaleCorrectionReconciliationImpact(r.Context(), app.CorrectTrading212SaleInput{
			OwnerUserID: owner.ID, BatchID: batchID, RowID: rowID, Reason: request.Reason,
		})
		if err != nil {
			switch {
			case errors.Is(err, app.ErrImportBatchNotFound):
				writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import batch not found")
			case errors.Is(err, app.ErrImportSourceCorrectionConflict):
				writeAPIError(w, http.StatusConflict, "CONFLICT", "source sale revision is no longer eligible")
			default:
				writeInvestmentServiceError(w, r, logger, "preview Trading 212 sale source correction", err)
			}
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func correctTrading212Sale(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}
		rowID, ok := readPathInt64(w, r, "row_id", "row id")
		if !ok {
			return
		}
		var request correctTrading212SourceRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := importService.CorrectTrading212Sale(r.Context(), app.CorrectTrading212SaleInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
			RequestID: RequestIDFromContext(r.Context()), BatchID: batchID, RowID: rowID,
			Reason: request.Reason, ReconciliationOverride: request.checkpointOverride(),
			GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			switch {
			case errors.Is(err, app.ErrImportBatchNotFound):
				writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import batch not found")
			case errors.Is(err, app.ErrImportSourceCorrectionConflict):
				writeAPIError(w, http.StatusConflict, "CONFLICT", "source sale revision is no longer eligible")
			default:
				writeInvestmentServiceError(w, r, logger, "correct Trading 212 sale source", err)
			}
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReplacementResponse{
			CorrectedTransactionID: result.CorrectedTransactionID,
			InverseTransaction:     toTransactionResponse(result.Inverse),
			Replacement:            toInvestmentTradeResponse(result.Replacement),
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
	if writeCheckpointImpactRefusal(w, err) {
		return
	}
	switch {
	case errors.Is(err, app.ErrReconciliationOverrideRequired):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "reconciliation override required")
	case errors.Is(err, app.ErrReconciliationImpactStale):
		writeAPIError(w, http.StatusConflict, "RECONCILIATION_IMPACT_STALE", "the checkpoints this write would invalidate changed after they were reviewed; review them again")
	case errors.Is(err, app.ErrImportBatchNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import batch not found")
	case errors.Is(err, app.ErrImportBatchNotOpen):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "import batch is not in an open state")
	case errors.Is(err, app.ErrImportStagedRowNotReviewable):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "import row is already committed or skipped")
	case errors.Is(err, app.ErrImportProfileNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import profile not found")
	case errors.Is(err, app.ErrImportRuleNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import rule not found")
	case errors.Is(err, app.ErrImportConnectionNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import connection not found")
	case errors.Is(err, app.ErrImportFetchInProgress):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "an import fetch is already in progress for this connection")
	case errors.Is(err, app.ErrSecretKeyMissing):
		writeAPIError(w, http.StatusServiceUnavailable, "CONFIG_REQUIRED", "REKENRAAM_SECRET_KEY is not configured on the server")
	default:
		if ve, ok := errors.AsType[app.ValidationError](err); ok {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", ve.Message)
			return
		}
		writeServiceInternalError(w, r, logger, op, err)
	}
}

func toImportProfileResponse(profile app.ImportProfile) importProfileResponse {
	return importProfileResponse{ID: profile.ID, Name: profile.Name, AdapterKind: profile.AdapterKind, ConfigJSON: profile.ConfigJSON, CreatedAt: profile.CreatedAt, UpdatedAt: profile.UpdatedAt}
}

func toImportRuleResponse(rule app.ImportRule) importRuleResponse {
	tagIDs := rule.TagIDs
	if tagIDs == nil {
		tagIDs = []int64{}
	}
	return importRuleResponse{
		ID: rule.ID, Name: rule.Name, Priority: rule.Priority, Enabled: rule.Enabled,
		MatchField: rule.MatchField, ContainsText: rule.ContainsText,
		CategoryID: rule.CategoryID, PayeeID: rule.PayeeID, TagIDs: tagIDs,
		CreatedAt: rule.CreatedAt, UpdatedAt: rule.UpdatedAt,
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

func readImportProfileID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("profile_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "profile_id is invalid")
		return 0, false
	}
	return id, true
}

func readImportRuleID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("rule_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "rule_id is invalid")
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
		ConnectionID:     b.ConnectionID,
		Status:           b.Status,
		OriginalFilename: b.OriginalFilename,
		SourceMetaJSON:   meta,
		CreatedAt:        b.CreatedAt,
	}
}

func toImportStagedRowResponse(row app.ImportStagedRow) importStagedRowResponse {
	effects := make([]importCommitEffectResponse, 0, len(row.CommitEffects))
	for _, effect := range row.CommitEffects {
		effects = append(effects, importCommitEffectResponse{
			EffectSeq: effect.EffectSeq, OperationID: effect.OperationID, TransactionID: effect.TransactionID,
		})
	}
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
		SourceChanged:          row.SourceChanged,
		SourceBuyOperation:     row.SourceBuyOperation,
		SourceSaleOperation:    row.SourceSaleOperation,
		SourceTransactionID:    row.SourceTransactionID,
		ResolutionJSON:         resolution,
		CommitStatus:           row.CommitStatus,
		CommittedIdentityID:    row.CommittedIdentityID,
		CommittedTransactionID: row.CommittedTransactionID,
		CommitEffects:          effects,
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

func toParseWarningResponses(in []app.ParseWarning) []parseWarningResponse {
	warnings := make([]parseWarningResponse, 0, len(in))
	for _, w := range in {
		warnings = append(warnings, parseWarningResponse{RowIndex: w.RowIndex, Code: w.Code, Value: w.Value, Message: w.Message})
	}
	return warnings
}

func toStartImportResponse(result app.StartImportResult) startImportResponse {
	warnings := toParseWarningResponses(result.Warnings)

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
			AccountHints:       accountHints,
			CurrencyHints:      currencyHints,
			DateFrom:           result.Meta.DateFrom,
			DateTo:             result.Meta.DateTo,
			TextEncoding:       result.Meta.TextEncoding,
			EncodingSource:     result.Meta.EncodingSource,
			EncodingConfidence: result.Meta.EncodingConfidence,
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

type importLinkSplitRequest struct {
	OperationID int64 `json:"operation_id"`
}

type importSplitCandidateResponse struct {
	OperationID      int64  `json:"operation_id"`
	TransactionID    *int64 `json:"transaction_id"`
	HoldingAccountID int64  `json:"holding_account_id"`
	EffectiveOn      string `json:"effective_on"`
	RatioNumerator   int64  `json:"ratio_numerator"`
	RatioDenominator int64  `json:"ratio_denominator"`
	Linked           bool   `json:"linked"`
}

type importSplitCandidatesResponse struct {
	Candidates []importSplitCandidateResponse `json:"candidates"`
}

func writeImportSplitLinkError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	switch {
	case errors.Is(err, app.ErrImportBatchNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import batch not found")
	case errors.Is(err, app.ErrImportSplitLinkUnavailable):
		writeAPIError(w, http.StatusConflict, "IMPORT_SPLIT_LINK_UNAVAILABLE", err.Error())
	default:
		writeInvestmentServiceError(w, r, logger, action, err)
	}
}

func importSplitCandidates(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}
		rowID, ok := readPathInt64(w, r, "row_id", "row id")
		if !ok {
			return
		}
		candidates, err := importService.Trading212SplitCandidates(r.Context(), app.Trading212SplitRowInput{
			OwnerUserID: owner.ID, BatchID: batchID, RowID: rowID,
		})
		if err != nil {
			writeImportSplitLinkError(w, r, logger, "list import split candidates", err)
			return
		}
		response := importSplitCandidatesResponse{Candidates: make([]importSplitCandidateResponse, 0, len(candidates))}
		for _, candidate := range candidates {
			response.Candidates = append(response.Candidates, importSplitCandidateResponse{
				OperationID: candidate.OperationID, TransactionID: splitTransactionRef(candidate.TransactionID), HoldingAccountID: candidate.HoldingAccountID,
				EffectiveOn: candidate.EffectiveOn, RatioNumerator: candidate.RatioNumerator, RatioDenominator: candidate.RatioDenominator, Linked: candidate.Linked})
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func linkImportSplit(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}
		rowID, ok := readPathInt64(w, r, "row_id", "row id")
		if !ok {
			return
		}
		var request importLinkSplitRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		if err := importService.LinkTrading212Split(r.Context(), app.LinkTrading212SplitInput{
			Trading212SplitRowInput: app.Trading212SplitRowInput{
				OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
				RequestID: RequestIDFromContext(r.Context()), BatchID: batchID, RowID: rowID,
			},
			OperationID: request.OperationID,
		}); err != nil {
			writeImportSplitLinkError(w, r, logger, "link import split", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

type importLinkShortRequest struct {
	OperationID int64 `json:"operation_id"`
}

type importShortSourceResponse struct {
	OperationKind string `json:"operation_kind"`
	FillType      string `json:"fill_type"`
	Side          string `json:"side"`
	Ticker        string `json:"ticker"`
	ISIN          string `json:"isin"`
	Date          string `json:"date"`
	Quantity      string `json:"quantity"`
	NetValue      string `json:"net_value"`
	NetCurrency   string `json:"net_currency"`
}

type importShortCandidateResponse struct {
	OperationID      int64    `json:"operation_id"`
	OperationKind    string   `json:"operation_kind"`
	TransactionID    int64    `json:"transaction_id"`
	HoldingAccountID int64    `json:"holding_account_id"`
	CashAccountID    int64    `json:"cash_account_id"`
	EventDate        string   `json:"event_date"`
	Quantity         string   `json:"quantity"`
	NetAmount        string   `json:"net_amount"`
	CashCurrency     string   `json:"cash_currency"`
	Linked           bool     `json:"linked"`
	Mismatches       []string `json:"mismatches"`
}

type importShortCandidatesResponse struct {
	Source     importShortSourceResponse      `json:"source"`
	Candidates []importShortCandidateResponse `json:"candidates"`
}

func writeImportShortLinkError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	switch {
	case errors.Is(err, app.ErrImportBatchNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import batch not found")
	case errors.Is(err, app.ErrImportShortLinkUnavailable):
		writeAPIError(w, http.StatusConflict, "IMPORT_SHORT_LINK_UNAVAILABLE", err.Error())
	case errors.Is(err, app.ErrImportShortLinkChanged):
		writeAPIError(w, http.StatusConflict, "IMPORT_SHORT_LINK_CHANGED", err.Error())
	case errors.Is(err, app.ErrImportShortLinkMismatch):
		writeAPIError(w, http.StatusConflict, "IMPORT_SHORT_LINK_MISMATCH", err.Error())
	default:
		writeInvestmentServiceError(w, r, logger, action, err)
	}
}

func importShortCandidates(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}
		rowID, ok := readPathInt64(w, r, "row_id", "row id")
		if !ok {
			return
		}
		review, err := importService.Trading212ShortCandidates(r.Context(), app.Trading212ShortRowInput{
			OwnerUserID: owner.ID, BatchID: batchID, RowID: rowID,
		})
		if err != nil {
			writeImportShortLinkError(w, r, logger, "list import short candidates", err)
			return
		}
		source := review.Source
		response := importShortCandidatesResponse{
			Source: importShortSourceResponse{OperationKind: source.OperationKind, FillType: source.FillType, Side: source.Side,
				Ticker: source.Ticker, ISIN: source.ISIN, Date: source.Date, Quantity: source.Quantity,
				NetValue: source.NetValue, NetCurrency: source.NetCurrency},
			Candidates: make([]importShortCandidateResponse, 0, len(review.Candidates)),
		}
		for _, candidate := range review.Candidates {
			response.Candidates = append(response.Candidates, importShortCandidateResponse{
				OperationID: candidate.OperationID, OperationKind: candidate.OperationKind, TransactionID: candidate.TransactionID,
				HoldingAccountID: candidate.HoldingAccountID, CashAccountID: candidate.CashAccountID, EventDate: candidate.EventDate,
				Quantity: candidate.Quantity, NetAmount: candidate.NetAmount, CashCurrency: candidate.CashCurrency,
				Linked: candidate.Linked, Mismatches: candidate.Mismatches,
			})
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func linkImportShort(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}
		rowID, ok := readPathInt64(w, r, "row_id", "row id")
		if !ok {
			return
		}
		var request importLinkShortRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		if err := importService.LinkTrading212Short(r.Context(), app.LinkTrading212ShortInput{
			Trading212ShortRowInput: app.Trading212ShortRowInput{
				OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
				RequestID: RequestIDFromContext(r.Context()), BatchID: batchID, RowID: rowID,
			},
			OperationID: request.OperationID,
		}); err != nil {
			writeImportShortLinkError(w, r, logger, "link import short", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
