package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type transactionResponse struct {
	ID                        int64                  `json:"id"`
	BookID                    int64                  `json:"book_id"`
	CorrectionOfTransactionID *int64                 `json:"correction_of_transaction_id,omitempty"`
	Status                    string                 `json:"status"`
	TransactionKind           string                 `json:"transaction_kind"`
	TransactionDate           string                 `json:"transaction_date"`
	PayeeID                   *int64                 `json:"payee_id,omitempty"`
	PayeeName                 string                 `json:"payee_name,omitempty"`
	Description               string                 `json:"description"`
	ExternalRefHint           string                 `json:"external_ref_hint,omitempty"`
	NoteMarkdown              string                 `json:"note_markdown"`
	Metadata                  json.RawMessage        `json:"metadata"`
	VersionID                 int64                  `json:"version_id"`
	VersionSeq                int64                  `json:"version_seq"`
	SupersedesVersionID       *int64                 `json:"supersedes_version_id,omitempty"`
	TagIDs                    []int64                `json:"tag_ids"`
	JournalEntries            []journalEntryResponse `json:"journal_entries"`
	CreatedAt                 string                 `json:"created_at"`
	UpdatedAt                 string                 `json:"updated_at"`
	ChangeReason              string                 `json:"change_reason"`
	InvalidatedCheckpointIDs  []int64                `json:"invalidated_checkpoint_ids,omitempty"`
}

type journalEntryResponse struct {
	ID        int64             `json:"id"`
	EntrySeq  int64             `json:"entry_seq"`
	EntryDate string            `json:"entry_date"`
	EntryKind string            `json:"entry_kind"`
	Memo      string            `json:"memo"`
	Metadata  json.RawMessage   `json:"metadata"`
	Postings  []postingResponse `json:"postings"`
}

type postingResponse struct {
	ID                   int64           `json:"id"`
	PostingLineID        int64           `json:"posting_line_id"`
	LineKey              string          `json:"line_key"`
	LineSeq              int64           `json:"line_seq"`
	AccountID            int64           `json:"account_id"`
	QuantityValue        int64           `json:"quantity_value"`
	QuantityScale        int             `json:"quantity_scale"`
	CommodityID          int64           `json:"commodity_id"`
	Memo                 string          `json:"memo"`
	ReconciliationStatus string          `json:"reconciliation_status"`
	ClearedOn            string          `json:"cleared_on,omitempty"`
	Metadata             json.RawMessage `json:"metadata"`
	TagIDs               []int64         `json:"tag_ids"`
}

type transactionsResponse struct {
	Transactions []transactionResponse `json:"transactions"`
	NextCursor   string                `json:"next_cursor,omitempty"`
}

type accountRegisterResponse struct {
	Entries    []accountRegisterEntryResponse `json:"entries"`
	NextCursor string                         `json:"next_cursor,omitempty"`
}

type accountRegisterEntryResponse struct {
	TransactionID             int64                   `json:"transaction_id"`
	BookID                    int64                   `json:"book_id"`
	CorrectionOfTransactionID *int64                  `json:"correction_of_transaction_id,omitempty"`
	Status                    string                  `json:"status"`
	TransactionKind           string                  `json:"transaction_kind"`
	TransactionDate           string                  `json:"transaction_date"`
	PayeeID                   *int64                  `json:"payee_id,omitempty"`
	PayeeName                 string                  `json:"payee_name,omitempty"`
	Description               string                  `json:"description"`
	ExternalRefHint           string                  `json:"external_ref_hint,omitempty"`
	VersionID                 int64                   `json:"version_id"`
	VersionSeq                int64                   `json:"version_seq"`
	SupersedesVersionID       *int64                  `json:"supersedes_version_id,omitempty"`
	TransactionTagIDs         []int64                 `json:"transaction_tag_ids"`
	JournalEntryID            int64                   `json:"journal_entry_id"`
	EntrySeq                  int64                   `json:"entry_seq"`
	EntryDate                 string                  `json:"entry_date"`
	EntryKind                 string                  `json:"entry_kind"`
	EntryMemo                 string                  `json:"entry_memo"`
	Posting                   postingResponse         `json:"posting"`
	RunningBalance            balanceQuantityResponse `json:"running_balance"`
	CreatedAt                 string                  `json:"created_at"`
	UpdatedAt                 string                  `json:"updated_at"`
	ChangeReason              string                  `json:"change_reason"`
}

type transactionRequest struct {
	Status                 string                `json:"status"`
	TransactionKind        string                `json:"transaction_kind"`
	TransactionDate        string                `json:"transaction_date"`
	PayeeID                *int64                `json:"payee_id"`
	PayeeName              string                `json:"payee_name"`
	Description            string                `json:"description"`
	ExternalRefHint        string                `json:"external_ref_hint"`
	NoteMarkdown           string                `json:"note_markdown"`
	Metadata               json.RawMessage       `json:"metadata"`
	TagIDs                 []int64               `json:"tag_ids"`
	JournalEntries         []journalEntryRequest `json:"journal_entries"`
	ChangeReason           string                `json:"change_reason"`
	ReconciliationOverride bool                  `json:"reconciliation_override"`
}

type journalEntryRequest struct {
	EntryDate string           `json:"entry_date"`
	EntryKind string           `json:"entry_kind"`
	Memo      string           `json:"memo"`
	Metadata  json.RawMessage  `json:"metadata"`
	Postings  []postingRequest `json:"postings"`
}

type postingRequest struct {
	LineKey              string          `json:"line_key"`
	AccountID            int64           `json:"account_id"`
	QuantityValue        int64           `json:"quantity_value"`
	QuantityScale        int             `json:"quantity_scale"`
	CommodityID          int64           `json:"commodity_id"`
	Memo                 string          `json:"memo"`
	ReconciliationStatus string          `json:"reconciliation_status"`
	ClearedOn            string          `json:"cleared_on"`
	Metadata             json.RawMessage `json:"metadata"`
	TagIDs               []int64         `json:"tag_ids"`
}

type voidTransactionRequest struct {
	ChangeReason           string `json:"change_reason"`
	ReconciliationOverride bool   `json:"reconciliation_override"`
}

func listTransactions(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		input, ok := readTransactionListInput(w, r)
		if !ok {
			return
		}

		result, err := transactionService.ListTransactions(r.Context(), input)
		if err != nil {
			writeTransactionServiceError(w, r, logger, "list transactions", err)
			return
		}

		writeJSON(w, http.StatusOK, transactionsResponse{
			Transactions: toTransactionResponses(result.Transactions),
			NextCursor:   result.NextCursor,
		})
	}
}

func readTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		transaction, err := transactionService.Transaction(r.Context(), transactionID)
		if err != nil {
			writeTransactionServiceError(w, r, logger, "read transaction", err)
			return
		}

		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}
}

func createTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request transactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.CreateTransaction(r.Context(), app.CreateTransactionInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			Operation:     "transaction.create",
			Spec:          toTransactionInput(request),
			ChangeReason:  request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "create transaction", err)
			return
		}

		writeJSON(w, http.StatusCreated, toTransactionResponse(transaction))
	}))
}

func updateTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		var request transactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.UpdateTransaction(r.Context(), app.UpdateTransactionInput{
			OwnerUserID:            owner.ID,
			AuthSessionID:          authenticatedSessionID(r),
			RequestID:              RequestIDFromContext(r.Context()),
			OriginType:             "browser_api",
			Operation:              "transaction.update",
			TransactionID:          transactionID,
			Spec:                   toTransactionInput(request),
			ChangeReason:           request.ChangeReason,
			ReconciliationOverride: request.ReconciliationOverride,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "update transaction", err)
			return
		}

		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}))
}

func postTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		var request voidTransactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.PostTransaction(r.Context(), app.PostTransactionInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			TransactionID: transactionID,
			ChangeReason:  request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "post transaction", err)
			return
		}

		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}))
}

func voidTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		var request voidTransactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.VoidTransaction(r.Context(), app.VoidTransactionInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			TransactionID: transactionID,
			ChangeReason:  request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "void transaction", err)
			return
		}

		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}))
}

func correctTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		correctedTransactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		var request transactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.CreateTransaction(r.Context(), app.CreateTransactionInput{
			OwnerUserID:               owner.ID,
			AuthSessionID:             authenticatedSessionID(r),
			RequestID:                 RequestIDFromContext(r.Context()),
			OriginType:                "browser_api",
			Operation:                 "transaction.correct",
			CorrectionOfTransactionID: &correctedTransactionID,
			Spec:                      toTransactionInput(request),
			ChangeReason:              request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "correct transaction", err)
			return
		}

		writeJSON(w, http.StatusCreated, toTransactionResponse(transaction))
	}))
}

func deleteDraftTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedMutationOwner(w, r); !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		if err := transactionService.DeleteDraftTransaction(r.Context(), app.DeleteDraftTransactionInput{TransactionID: transactionID}); err != nil {
			writeTransactionServiceError(w, r, logger, "delete draft transaction", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
}

func accountRegister(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}
		input, ok := readTransactionListInput(w, r)
		if !ok {
			return
		}

		result, err := transactionService.Register(r.Context(), accountID, input)
		if err != nil {
			writeTransactionServiceError(w, r, logger, "read account register", err)
			return
		}

		writeJSON(w, http.StatusOK, accountRegisterResponse{
			Entries:    toAccountRegisterEntryResponses(result.Entries),
			NextCursor: result.NextCursor,
		})
	}
}

func readTransactionListInput(w http.ResponseWriter, r *http.Request) (app.ListTransactionsInput, bool) {
	query := r.URL.Query()
	accountID, ok := parseOptionalPositiveInt64(w, query.Get("account_id"), "account id")
	if !ok {
		return app.ListTransactionsInput{}, false
	}
	payeeID, ok := parseOptionalPositiveInt64(w, query.Get("payee_id"), "payee id")
	if !ok {
		return app.ListTransactionsInput{}, false
	}
	limit := 0
	if query.Get("limit") != "" {
		parsed, err := strconv.Atoi(query.Get("limit"))
		if err != nil || parsed <= 0 {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "limit is invalid")
			return app.ListTransactionsInput{}, false
		}
		limit = parsed
	}

	return app.ListTransactionsInput{
		AccountID:  accountID,
		PayeeID:    payeeID,
		Status:     query.Get("status"),
		Kind:       query.Get("kind"),
		Query:      query.Get("q"),
		AfterDate:  query.Get("after_date"),
		BeforeDate: query.Get("before_date"),
		Limit:      limit,
		Cursor:     query.Get("cursor"),
	}, true
}

func parseOptionalPositiveInt64(w http.ResponseWriter, value string, field string) (int64, bool) {
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", field+" is invalid")
		return 0, false
	}
	return parsed, true
}

func readTransactionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	transactionID, err := strconv.ParseInt(r.PathValue("transaction_id"), 10, 64)
	if err != nil || transactionID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "transaction id is invalid")
		return 0, false
	}
	return transactionID, true
}

func writeTransactionServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.Is(err, app.ErrTransactionNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "transaction not found")
	case errors.Is(err, app.ErrTransactionProtected):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "transaction requires a corrective workflow")
	case errors.Is(err, app.ErrTransactionPosted):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "posted or voided transaction cannot be deleted")
	case errors.Is(err, app.ErrTransactionVoided):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "voided transaction cannot be edited")
	case errors.Is(err, app.ErrTransactionTag):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "transaction tag is invalid")
	case errors.Is(err, app.ErrReconciliationOverrideRequired):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "reconciliation override is required")
	case errors.Is(err, app.ErrReconciliationNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "reconciliation not found")
	case errors.Is(err, app.ErrReconciliationClosed):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "reconciliation is not open")
	case errors.Is(err, app.ErrReconciliationNotBalanced):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "reconciliation difference must be zero")
	case errors.Is(err, app.ErrReconciliationPosting):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "reconciliation posting is invalid")
	default:
		logger.ErrorContext(r.Context(), action, slog.Any("err", err))
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func toTransactionInput(request transactionRequest) app.TransactionInput {
	entries := make([]app.JournalEntryInput, 0, len(request.JournalEntries))
	for _, entry := range request.JournalEntries {
		postings := make([]app.PostingInput, 0, len(entry.Postings))
		for _, posting := range entry.Postings {
			postings = append(postings, app.PostingInput{
				LineKey:              posting.LineKey,
				AccountID:            posting.AccountID,
				QuantityValue:        posting.QuantityValue,
				QuantityScale:        posting.QuantityScale,
				CommodityID:          posting.CommodityID,
				Memo:                 posting.Memo,
				ReconciliationStatus: posting.ReconciliationStatus,
				ClearedOn:            posting.ClearedOn,
				MetadataJSON:         rawJSONText(posting.Metadata),
				TagIDs:               posting.TagIDs,
			})
		}
		entries = append(entries, app.JournalEntryInput{
			EntryDate:    entry.EntryDate,
			EntryKind:    entry.EntryKind,
			Memo:         entry.Memo,
			MetadataJSON: rawJSONText(entry.Metadata),
			Postings:     postings,
		})
	}

	return app.TransactionInput{
		Status:          request.Status,
		TransactionKind: request.TransactionKind,
		TransactionDate: request.TransactionDate,
		PayeeID:         request.PayeeID,
		PayeeName:       request.PayeeName,
		Description:     request.Description,
		ExternalRefHint: request.ExternalRefHint,
		NoteMarkdown:    request.NoteMarkdown,
		MetadataJSON:    rawJSONText(request.Metadata),
		TagIDs:          request.TagIDs,
		JournalEntries:  entries,
	}
}

func toTransactionResponse(transaction app.Transaction) transactionResponse {
	entries := make([]journalEntryResponse, 0, len(transaction.JournalEntries))
	for _, entry := range transaction.JournalEntries {
		postings := make([]postingResponse, 0, len(entry.Postings))
		for _, posting := range entry.Postings {
			postings = append(postings, postingResponse{
				ID:                   posting.ID,
				PostingLineID:        posting.PostingLineID,
				LineKey:              posting.LineKey,
				LineSeq:              posting.LineSeq,
				AccountID:            posting.AccountID,
				QuantityValue:        posting.QuantityValue,
				QuantityScale:        posting.QuantityScale,
				CommodityID:          posting.CommodityID,
				Memo:                 posting.Memo,
				ReconciliationStatus: posting.ReconciliationStatus,
				ClearedOn:            posting.ClearedOn,
				Metadata:             json.RawMessage(posting.MetadataJSON),
				TagIDs:               posting.TagIDs,
			})
		}
		entries = append(entries, journalEntryResponse{
			ID:        entry.ID,
			EntrySeq:  entry.EntrySeq,
			EntryDate: entry.EntryDate,
			EntryKind: entry.EntryKind,
			Memo:      entry.Memo,
			Metadata:  json.RawMessage(entry.MetadataJSON),
			Postings:  postings,
		})
	}

	return transactionResponse{
		ID:                        transaction.ID,
		BookID:                    transaction.BookID,
		CorrectionOfTransactionID: transaction.CorrectionOfTransactionID,
		Status:                    transaction.Status,
		TransactionKind:           transaction.TransactionKind,
		TransactionDate:           transaction.TransactionDate,
		PayeeID:                   transaction.PayeeID,
		PayeeName:                 transaction.PayeeName,
		Description:               transaction.Description,
		ExternalRefHint:           transaction.ExternalRefHint,
		NoteMarkdown:              transaction.NoteMarkdown,
		Metadata:                  json.RawMessage(transaction.MetadataJSON),
		VersionID:                 transaction.VersionID,
		VersionSeq:                transaction.VersionSeq,
		SupersedesVersionID:       transaction.SupersedesVersionID,
		TagIDs:                    transaction.TagIDs,
		JournalEntries:            entries,
		CreatedAt:                 transaction.CreatedAt,
		UpdatedAt:                 transaction.UpdatedAt,
		ChangeReason:              transaction.ChangeReason,
		InvalidatedCheckpointIDs:  transaction.InvalidatedCheckpointIDs,
	}
}

func toTransactionResponses(transactions []app.Transaction) []transactionResponse {
	responses := make([]transactionResponse, 0, len(transactions))
	for _, transaction := range transactions {
		responses = append(responses, toTransactionResponse(transaction))
	}
	return responses
}

func toAccountRegisterEntryResponses(entries []app.AccountRegisterEntry) []accountRegisterEntryResponse {
	responses := make([]accountRegisterEntryResponse, 0, len(entries))
	for _, entry := range entries {
		responses = append(responses, accountRegisterEntryResponse{
			TransactionID:             entry.TransactionID,
			BookID:                    entry.BookID,
			CorrectionOfTransactionID: entry.CorrectionOfTransactionID,
			Status:                    entry.Status,
			TransactionKind:           entry.TransactionKind,
			TransactionDate:           entry.TransactionDate,
			PayeeID:                   entry.PayeeID,
			PayeeName:                 entry.PayeeName,
			Description:               entry.Description,
			ExternalRefHint:           entry.ExternalRefHint,
			VersionID:                 entry.VersionID,
			VersionSeq:                entry.VersionSeq,
			SupersedesVersionID:       entry.SupersedesVersionID,
			TransactionTagIDs:         entry.TransactionTagIDs,
			JournalEntryID:            entry.JournalEntryID,
			EntrySeq:                  entry.EntrySeq,
			EntryDate:                 entry.EntryDate,
			EntryKind:                 entry.EntryKind,
			EntryMemo:                 entry.EntryMemo,
			Posting: postingResponse{
				ID:                   entry.Posting.ID,
				PostingLineID:        entry.Posting.PostingLineID,
				LineKey:              entry.Posting.LineKey,
				LineSeq:              entry.Posting.LineSeq,
				AccountID:            entry.Posting.AccountID,
				QuantityValue:        entry.Posting.QuantityValue,
				QuantityScale:        entry.Posting.QuantityScale,
				CommodityID:          entry.Posting.CommodityID,
				Memo:                 entry.Posting.Memo,
				ReconciliationStatus: entry.Posting.ReconciliationStatus,
				ClearedOn:            entry.Posting.ClearedOn,
				Metadata:             json.RawMessage(entry.Posting.MetadataJSON),
				TagIDs:               entry.Posting.TagIDs,
			},
			RunningBalance: toBalanceQuantityResponse(entry.RunningBalance),
			CreatedAt:      entry.CreatedAt,
			UpdatedAt:      entry.UpdatedAt,
			ChangeReason:   entry.ChangeReason,
		})
	}
	return responses
}
