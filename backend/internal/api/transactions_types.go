package api

import (
	"encoding/json"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type transactionResponse struct {
	ID                        int64                  `json:"id"`
	BookID                    int64                  `json:"book_id"`
	CorrectionOfTransactionID *int64                 `json:"correction_of_transaction_id,omitempty"`
	Status                    string                 `json:"status"`
	TransactionKind           string                 `json:"transaction_kind"`
	TransactionDate           string                 `json:"transaction_date"`
	TransactionDaySequence    int64                  `json:"transaction_day_sequence"`
	PayeeID                   *int64                 `json:"payee_id,omitempty"`
	PayeeName                 string                 `json:"payee_name,omitempty"`
	Description               string                 `json:"description"`
	ExternalRefHint           string                 `json:"external_ref_hint,omitempty"`
	NoteMarkdown              string                 `json:"note_markdown"`
	Metadata                  json.RawMessage        `json:"metadata"`
	NeedsReview               bool                   `json:"needs_review"`
	VersionID                 int64                  `json:"version_id"`
	VersionSeq                int64                  `json:"version_seq"`
	SupersedesVersionID       *int64                 `json:"supersedes_version_id,omitempty"`
	TagIDs                    []int64                `json:"tag_ids"`
	JournalEntries            []journalEntryResponse `json:"journal_entries"`
	CreatedAt                 string                 `json:"created_at"`
	UpdatedAt                 string                 `json:"updated_at"`
	DeletedAt                 string                 `json:"deleted_at,omitempty"`
	ChangeReason              string                 `json:"change_reason"`
	SystemLabel               string                 `json:"system_label,omitempty"`
	InvalidatedCheckpointIDs  []int64                `json:"invalidated_checkpoint_ids"`
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
	ID                   int64             `json:"id"`
	PostingLineID        int64             `json:"posting_line_id"`
	LineKey              string            `json:"line_key"`
	LineSeq              int64             `json:"line_seq"`
	AccountID            int64             `json:"account_id"`
	AccountDaySequence   int64             `json:"account_day_sequence"`
	QuantityValue        exact.Coefficient `json:"quantity_value"`
	QuantityScale        int               `json:"quantity_scale"`
	CommodityID          int64             `json:"commodity_id"`
	Memo                 string            `json:"memo"`
	ReconciliationStatus string            `json:"reconciliation_status"`
	ClearedOn            string            `json:"cleared_on,omitempty"`
	Metadata             json.RawMessage   `json:"metadata"`
	TagIDs               []int64           `json:"tag_ids"`

	// Enriched display metadata — populated on every response path.
	AccountName       *string `json:"account_name"`
	AccountCode       *string `json:"account_code"`
	AccountSystemRole *string `json:"account_system_role"`
	AccountBuiltinKey *string `json:"account_builtin_key"`
	AccountClass      string  `json:"account_class"`
	CommodityCode     string  `json:"commodity_code"`
	CommoditySymbol   *string `json:"commodity_symbol"`
}

type transactionsResponse struct {
	Transactions []transactionResponse `json:"transactions"`
	NextCursor   *string               `json:"next_cursor"`
}

type deletedTransactionResponse struct {
	transactionResponse
	DeleteReason                   string `json:"delete_reason"`
	RestoreBlockedByReconciliation bool   `json:"restore_blocked_by_reconciliation"`
}

type deletedTransactionsResponse struct {
	Transactions []deletedTransactionResponse `json:"transactions"`
	NextCursor   *string                      `json:"next_cursor"`
}

type accountRegisterResponse struct {
	Entries    []accountRegisterEntryResponse `json:"entries"`
	NextCursor *string                        `json:"next_cursor"`
}

type accountRegisterEntryResponse struct {
	TransactionID             int64                            `json:"transaction_id"`
	BookID                    int64                            `json:"book_id"`
	CorrectionOfTransactionID *int64                           `json:"correction_of_transaction_id,omitempty"`
	Status                    string                           `json:"status"`
	TransactionKind           string                           `json:"transaction_kind"`
	TransactionDate           string                           `json:"transaction_date"`
	PayeeID                   *int64                           `json:"payee_id,omitempty"`
	PayeeName                 string                           `json:"payee_name,omitempty"`
	Description               string                           `json:"description"`
	ExternalRefHint           string                           `json:"external_ref_hint,omitempty"`
	NeedsReview               bool                             `json:"needs_review"`
	VersionID                 int64                            `json:"version_id"`
	VersionSeq                int64                            `json:"version_seq"`
	SupersedesVersionID       *int64                           `json:"supersedes_version_id,omitempty"`
	TransactionTagIDs         []int64                          `json:"transaction_tag_ids"`
	JournalEntryID            int64                            `json:"journal_entry_id"`
	EntrySeq                  int64                            `json:"entry_seq"`
	EntryDate                 string                           `json:"entry_date"`
	EntryKind                 string                           `json:"entry_kind"`
	EntryMemo                 string                           `json:"entry_memo"`
	Posting                   postingResponse                  `json:"posting"`
	RunningBalance            balanceQuantityResponse          `json:"running_balance"`
	CreatedAt                 string                           `json:"created_at"`
	UpdatedAt                 string                           `json:"updated_at"`
	ChangeReason              string                           `json:"change_reason"`
	SystemLabel               string                           `json:"system_label,omitempty"`
	CorrectionChain           *registerCorrectionChainResponse `json:"correction_chain,omitempty"`
}

type registerCorrectionChainResponse struct {
	RootTransactionID      int64                              `json:"root_transaction_id"`
	Role                   string                             `json:"role"`
	EffectiveTransactionID *int64                             `json:"effective_transaction_id"`
	NetEffect              registerCorrectionAmountResponse   `json:"net_effect"`
	Members                []registerCorrectionMemberResponse `json:"members"`
}

type registerCorrectionMemberResponse struct {
	TransactionID             int64                             `json:"transaction_id"`
	CorrectionOfTransactionID *int64                            `json:"correction_of_transaction_id"`
	Role                      string                            `json:"role"`
	TransactionDate           string                            `json:"transaction_date"`
	Status                    string                            `json:"status"`
	Deleted                   bool                              `json:"deleted"`
	Reason                    string                            `json:"reason"`
	Amount                    *registerCorrectionAmountResponse `json:"amount"`
}

type registerCorrectionAmountResponse struct {
	QuantityValue exact.Coefficient `json:"quantity_value"`
	QuantityScale int               `json:"quantity_scale"`
}

func toRegisterCorrectionChainResponse(chain *app.RegisterCorrectionChain) *registerCorrectionChainResponse {
	if chain == nil {
		return nil
	}
	members := make([]registerCorrectionMemberResponse, 0, len(chain.Members))
	for _, member := range chain.Members {
		var amount *registerCorrectionAmountResponse
		if member.Amount != nil {
			amount = &registerCorrectionAmountResponse{QuantityValue: member.Amount.QuantityValue, QuantityScale: member.Amount.QuantityScale}
		}
		members = append(members, registerCorrectionMemberResponse{
			TransactionID: member.TransactionID, CorrectionOfTransactionID: member.CorrectionOfTransactionID,
			Role: member.Role, TransactionDate: member.TransactionDate, Status: member.Status,
			Deleted: member.Deleted, Reason: member.Reason, Amount: amount,
		})
	}
	return &registerCorrectionChainResponse{
		RootTransactionID: chain.RootTransactionID, Role: chain.Role,
		EffectiveTransactionID: chain.EffectiveTransactionID,
		NetEffect:              registerCorrectionAmountResponse{QuantityValue: chain.NetEffect.QuantityValue, QuantityScale: chain.NetEffect.QuantityScale},
		Members:                members,
	}
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
	NeedsReview            bool                  `json:"needs_review"`
	TagIDs                 []int64               `json:"tag_ids"`
	JournalEntries         []journalEntryRequest `json:"journal_entries"`
	ChangeReason           string                `json:"change_reason"`
	ReconciliationOverride bool                  `json:"reconciliation_override"`
	// ReconciliationOverrideCheckpointIDs is the set the owner reviewed for
	// the override; it is required with the override (#296).
	ReconciliationOverrideCheckpointIDs []int64 `json:"reconciliation_override_checkpoint_ids"`
	// ExpectedVersionID is the version an edit was composed against; PATCH
	// requires it (#296). Create and correct ignore it.
	ExpectedVersionID int64 `json:"expected_version_id"`
}

type journalEntryRequest struct {
	EntryDate string           `json:"entry_date"`
	EntryKind string           `json:"entry_kind"`
	Memo      string           `json:"memo"`
	Metadata  json.RawMessage  `json:"metadata"`
	Postings  []postingRequest `json:"postings"`
}

type postingRequest struct {
	LineKey       string            `json:"line_key"`
	PostedOn      string            `json:"posted_on"`
	AccountID     int64             `json:"account_id"`
	QuantityValue exact.Coefficient `json:"quantity_value"`
	QuantityScale int               `json:"quantity_scale"`
	CommodityID   int64             `json:"commodity_id"`
	Memo          string            `json:"memo"`
	Metadata      json.RawMessage   `json:"metadata"`
	TagIDs        []int64           `json:"tag_ids"`
}

type voidTransactionRequest struct {
	ChangeReason                        string  `json:"change_reason"`
	ReconciliationOverride              bool    `json:"reconciliation_override"`
	ReconciliationOverrideCheckpointIDs []int64 `json:"reconciliation_override_checkpoint_ids"`
	// ExpectedVersionID, when positive, is the version the owner decided on.
	ExpectedVersionID int64 `json:"expected_version_id"`
}

type approveTransactionRequest struct {
	ChangeReason string `json:"change_reason"`
}

// reviewedOverride is the checkpoint set a browser override acknowledges. An
// override must name the set its preview (or a reorder's refusal) returned,
// so the write can refuse a set the owner never saw (#296); without an
// override the set is ignored.
func reviewedOverride(override bool, checkpointIDs []int64) ([]int64, error) {
	if !override {
		return nil, nil
	}
	if checkpointIDs == nil {
		return nil, app.ValidationError{Message: "reconciliation_override_checkpoint_ids must name the checkpoints the override was reviewed against"}
	}
	return checkpointIDs, nil
}

func toTransactionInput(request transactionRequest) app.TransactionInput {
	entries := make([]app.JournalEntryInput, 0, len(request.JournalEntries))
	for _, entry := range request.JournalEntries {
		postings := make([]app.PostingInput, 0, len(entry.Postings))
		for _, posting := range entry.Postings {
			postings = append(postings, app.PostingInput{
				LineKey:       posting.LineKey,
				PostedOn:      posting.PostedOn,
				AccountID:     posting.AccountID,
				QuantityValue: posting.QuantityValue,
				QuantityScale: posting.QuantityScale,
				CommodityID:   posting.CommodityID,
				Memo:          posting.Memo,
				MetadataJSON:  rawJSONText(posting.Metadata),
				TagIDs:        posting.TagIDs,
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
		NeedsReview:     request.NeedsReview,
		TagIDs:          request.TagIDs,
		JournalEntries:  entries,
	}
}

func toTransactionResponse(transaction app.Transaction) transactionResponse {
	entries := make([]journalEntryResponse, 0, len(transaction.JournalEntries))
	for _, entry := range transaction.JournalEntries {
		postings := make([]postingResponse, 0, len(entry.Postings))
		for _, posting := range entry.Postings {
			tagIDs := posting.TagIDs
			if tagIDs == nil {
				tagIDs = []int64{}
			}
			postings = append(postings, postingResponse{
				ID:                   posting.ID,
				PostingLineID:        posting.PostingLineID,
				LineKey:              posting.LineKey,
				LineSeq:              posting.LineSeq,
				AccountID:            posting.AccountID,
				AccountDaySequence:   posting.AccountDaySequence,
				QuantityValue:        posting.QuantityValue,
				QuantityScale:        posting.QuantityScale,
				CommodityID:          posting.CommodityID,
				Memo:                 posting.Memo,
				ReconciliationStatus: posting.ReconciliationStatus,
				ClearedOn:            posting.ClearedOn,
				Metadata:             json.RawMessage(posting.MetadataJSON),
				TagIDs:               tagIDs,
				AccountName:          posting.AccountName,
				AccountCode:          posting.AccountCode,
				AccountSystemRole:    posting.AccountSystemRole,
				AccountBuiltinKey:    posting.AccountBuiltinKey,
				AccountClass:         posting.AccountClass,
				CommodityCode:        posting.CommodityCode,
				CommoditySymbol:      posting.CommoditySymbol,
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

	tagIDs := transaction.TagIDs
	if tagIDs == nil {
		tagIDs = []int64{}
	}
	invalidatedCheckpointIDs := transaction.InvalidatedCheckpointIDs
	if invalidatedCheckpointIDs == nil {
		invalidatedCheckpointIDs = []int64{}
	}

	return transactionResponse{
		ID:                        transaction.ID,
		BookID:                    transaction.BookID,
		CorrectionOfTransactionID: transaction.CorrectionOfTransactionID,
		Status:                    transaction.Status,
		TransactionKind:           transaction.TransactionKind,
		TransactionDate:           transaction.TransactionDate,
		TransactionDaySequence:    transaction.TransactionDaySequence,
		PayeeID:                   transaction.PayeeID,
		PayeeName:                 transaction.PayeeName,
		Description:               transaction.Description,
		ExternalRefHint:           transaction.ExternalRefHint,
		NoteMarkdown:              transaction.NoteMarkdown,
		Metadata:                  json.RawMessage(transaction.MetadataJSON),
		NeedsReview:               transaction.NeedsReview,
		VersionID:                 transaction.VersionID,
		VersionSeq:                transaction.VersionSeq,
		SupersedesVersionID:       transaction.SupersedesVersionID,
		TagIDs:                    tagIDs,
		JournalEntries:            entries,
		CreatedAt:                 transaction.CreatedAt,
		UpdatedAt:                 transaction.UpdatedAt,
		DeletedAt:                 transaction.DeletedAt,
		ChangeReason:              transaction.ChangeReason,
		SystemLabel:               transaction.SystemLabel,
		InvalidatedCheckpointIDs:  invalidatedCheckpointIDs,
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
		tagIDs := entry.Posting.TagIDs
		if tagIDs == nil {
			tagIDs = []int64{}
		}
		transactionTagIDs := entry.TransactionTagIDs
		if transactionTagIDs == nil {
			transactionTagIDs = []int64{}
		}
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
			NeedsReview:               entry.NeedsReview,
			VersionID:                 entry.VersionID,
			VersionSeq:                entry.VersionSeq,
			SupersedesVersionID:       entry.SupersedesVersionID,
			TransactionTagIDs:         transactionTagIDs,
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
				AccountDaySequence:   entry.Posting.AccountDaySequence,
				QuantityValue:        entry.Posting.QuantityValue,
				QuantityScale:        entry.Posting.QuantityScale,
				CommodityID:          entry.Posting.CommodityID,
				Memo:                 entry.Posting.Memo,
				ReconciliationStatus: entry.Posting.ReconciliationStatus,
				ClearedOn:            entry.Posting.ClearedOn,
				Metadata:             json.RawMessage(entry.Posting.MetadataJSON),
				TagIDs:               tagIDs,
				AccountName:          entry.Posting.AccountName,
				AccountCode:          entry.Posting.AccountCode,
				AccountSystemRole:    entry.Posting.AccountSystemRole,
				AccountBuiltinKey:    entry.Posting.AccountBuiltinKey,
				AccountClass:         entry.Posting.AccountClass,
				CommodityCode:        entry.Posting.CommodityCode,
				CommoditySymbol:      entry.Posting.CommoditySymbol,
			},
			RunningBalance:  toBalanceQuantityResponse(entry.RunningBalance),
			CreatedAt:       entry.CreatedAt,
			UpdatedAt:       entry.UpdatedAt,
			ChangeReason:    entry.ChangeReason,
			SystemLabel:     entry.SystemLabel,
			CorrectionChain: toRegisterCorrectionChainResponse(entry.CorrectionChain),
		})
	}
	return responses
}

// reconciliationOverrideFields is the override an investment or import write
// request carries (#304). An override must name the checkpoint set the owner
// reviewed (the action's preview), checked once at decode; the write refuses
// a different set with 409 RECONCILIATION_IMPACT_STALE.
type reconciliationOverrideFields struct {
	ReconciliationOverride              bool    `json:"reconciliation_override"`
	ReconciliationOverrideCheckpointIDs []int64 `json:"reconciliation_override_checkpoint_ids"`
}

// validateDecoded runs after decodeJSONBody fills a request that embeds these
// fields, so no handler can accept an unbound browser override.
func (f *reconciliationOverrideFields) validateDecoded() error {
	_, err := reviewedOverride(f.ReconciliationOverride, f.ReconciliationOverrideCheckpointIDs)
	return err
}

func (f reconciliationOverrideFields) checkpointOverride() app.CheckpointOverride {
	return app.CheckpointOverride{Allowed: f.ReconciliationOverride, Reviewed: f.ReconciliationOverrideCheckpointIDs}
}
