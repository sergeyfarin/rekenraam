package app

import (
	"errors"

	"rekenraam/backend/internal/db"
)

var (
	ErrImportBatchNotFound = errors.New("import batch not found")
	ErrImportBatchNotOpen  = errors.New("import batch is not open for review")
	// ErrImportStagedRowNotReviewable refuses a review patch on a row that
	// already committed or was skipped (#301).
	ErrImportStagedRowNotReviewable = errors.New("import row is already committed or skipped")
	ErrImportProfileNotFound        = errors.New("import profile not found")
	ErrImportRuleNotFound           = errors.New("import rule not found")
)

// ImportBatch is the service-layer representation of an import batch.
type ImportBatch struct {
	ID               int64
	BookID           int64
	SourceKind       string
	ProfileID        *int64
	ConnectionID     *int64
	Status           string
	OriginalFilename string
	SourceMetaJSON   string
	CreatedAt        string
}

// ImportStagedRow is the service-layer representation of a staged row.
type ImportStagedRow struct {
	ID                     int64
	BatchID                int64
	BookID                 int64
	RowIndex               int
	DedupeFingerprint      string
	RawJSON                string
	NormalizedJSON         string
	DedupeStatus           string
	SourceChanged          bool
	SourceBuyOperation     bool
	SourceSaleOperation    bool
	SourceTransactionID    *int64
	ResolutionJSON         string
	CommitStatus           string
	CommittedIdentityID    *int64
	CommittedTransactionID *int64
	CommitEffects          []ImportCommitEffect
	CommitError            string
}

type ImportCommitEffect struct {
	EffectSeq     int64
	OperationID   *int64
	TransactionID *int64
}

// ParseWarning is a non-fatal issue found during parsing. It is returned by
// POST /imports and kept in the batch's source_meta "warnings", so a reloaded
// review screen shows it too (#300). Code is a stable key the frontend
// translates (ParseWarningCode*); Value carries the code's one parameter, such
// as the rejected source text or the separator or date layout the file was
// read with. Message is the English fallback. RowIndex is -1 for a warning
// about the whole file.
type ParseWarning struct {
	RowIndex int    `json:"row_index"`
	Code     string `json:"code,omitempty"`
	Value    string `json:"value,omitempty"`
	Message  string `json:"message"`
}

// FileWarningRowIndex is a ParseWarning's RowIndex when it concerns the file,
// not one row.
const FileWarningRowIndex = -1

// Stable ParseWarning codes. New adapters reuse these or add their own; the
// frontend falls back to Message for a code it does not know.
const (
	ParseWarningCodeUnrecognizedDate        = "unrecognized_date"
	ParseWarningCodeUnrecognizedAmount      = "unrecognized_amount"
	ParseWarningCodeUnrecognizedSplitAmount = "unrecognized_split_amount"
	// Value is the separator the file was read with; empty when the file
	// could not settle it and each amount was read on its own.
	ParseWarningCodeDecimalSeparatorConflict = "decimal_separator_conflict"
	// Value is the date layout the file was read with (MDY, DMY, YMD).
	ParseWarningCodeDateLayoutConflict  = "date_layout_conflict"
	ParseWarningCodeDateLayoutAmbiguous = "date_layout_ambiguous"
)

// StagedParseError records one source value of a staged row that failed under
// the file's resolved convention (the declared profile value, or whole-file
// detection). It is stored in the row's normalized JSON as "parse_errors": the
// row needs attention, and commit refuses it rather than re-reading Value under
// another convention (#300).
type StagedParseError struct {
	// Field is "date", "amount" or "split_amount".
	Field string `json:"field"`
	Value string `json:"value"`
}

// SourceMeta carries hints and decoding provenance extracted from the source.
type SourceMeta struct {
	AccountHints       []string `json:"account_hints,omitempty"`
	CurrencyHints      []string `json:"currency_hints,omitempty"`
	DateFrom           string   `json:"date_from,omitempty"`
	DateTo             string   `json:"date_to,omitempty"`
	TextEncoding       string   `json:"text_encoding,omitempty"`
	EncodingSource     string   `json:"encoding_source,omitempty"`
	EncodingConfidence int      `json:"encoding_confidence,omitempty"`
	// Warnings is filled from ParseResult.Warnings when the batch is created,
	// so source_meta keeps them for GET /imports/{id}.
	Warnings []ParseWarning `json:"warnings,omitempty"`
}

// StagedSplit is a sub-posting within a staged row.
type StagedSplit struct {
	CategoryHint string
	Amount       string
	Memo         string
}

// StagedRow is the canonical, format-independent row produced by a SourceAdapter.
type StagedRow struct {
	Investment        *CSVInvestmentRow
	DedupeFingerprint string
	Date              string
	Amount            string
	CommodityHint     string
	PayeeHint         string
	CategoryHint      string
	AccountHint       string
	TransferHint      string
	Memo              string
	ExternalRef       string
	Splits            []StagedSplit
	Raw               map[string]string
	// ParseErrors lists the values that failed under the file's convention;
	// such a row keeps their source text and never commits.
	ParseErrors []StagedParseError
}

// ParseResult is what a SourceAdapter returns.
type ParseResult struct {
	Rows     []StagedRow
	Warnings []ParseWarning
	Meta     SourceMeta
}

// RawInput carries the raw bytes (and optional metadata) for a source adapter.
type RawInput struct {
	Filename     string
	ContentType  string
	Bytes        []byte
	Rows         [][]string
	TextEncoding string
}

// Confidence is how confident a SourceAdapter is that it can handle an input.
type Confidence int

const (
	ConfidenceNone   Confidence = 0
	ConfidenceLow    Confidence = 1
	ConfidenceMedium Confidence = 2
	ConfidenceHigh   Confidence = 3
)

// Profile is a saved per-provider adapter configuration.
type ImportProfile struct {
	ID          int64
	BookID      int64
	Name        string
	AdapterKind string
	ConfigJSON  string
	CreatedAt   string
	UpdatedAt   string
}

type CreateImportProfileInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	Name          string
	AdapterKind   string
	ConfigJSON    string
}

type UpdateImportProfileInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	ProfileID     int64
	Name          *string
	ConfigJSON    *string
}

type DeleteImportProfileInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	ProfileID     int64
}

type ImportRule struct {
	ID           int64
	BookID       int64
	Name         string
	Priority     int
	Enabled      bool
	MatchField   string
	ContainsText string
	CategoryID   *int64
	PayeeID      *int64
	TagIDs       []int64
	CreatedAt    string
	UpdatedAt    string
}

type CreateImportRuleInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	Name          string
	Priority      int
	Enabled       bool
	MatchField    string
	ContainsText  string
	CategoryID    *int64
	PayeeID       *int64
	TagIDs        []int64
}

type UpdateImportRuleInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	RuleID        int64
	Name          *string
	Priority      *int
	Enabled       *bool
	MatchField    *string
	ContainsText  *string
	CategoryID    *int64
	ClearCategory bool
	PayeeID       *int64
	ClearPayee    bool
	TagIDs        *[]int64
}

type DeleteImportRuleInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	RuleID        int64
}

// ImportRowResolution carries the user-chosen account/category/payee for a staged row.
type ImportRowResolution struct {
	InstrumentCommodityID int64   `json:"instrument_commodity_id,omitempty"`
	InvestmentKind        string  `json:"investment_kind,omitempty"`
	GrossAmount           *string `json:"gross_amount,omitempty"`
	WithholdingAmount     *string `json:"withholding_amount,omitempty"`
	WithholdingAccountID  *int64  `json:"withholding_account_id,omitempty"`
	AccountID             int64   `json:"account_id,omitempty"`
	CommodityID           int64   `json:"commodity_id,omitempty"`
	PayeeID               *int64  `json:"payee_id,omitempty"`
	PayeeName             string  `json:"payee_name,omitempty"`
	CategoryID            *int64  `json:"category_id,omitempty"`
	TransferAccountID     *int64  `json:"transfer_account_id,omitempty"`
	TagIDs                []int64 `json:"tag_ids,omitempty"`
	AppliedRuleID         *int64  `json:"applied_rule_id,omitempty"`
	AppliedRuleName       string  `json:"applied_rule_name,omitempty"`
	Exclude               bool    `json:"exclude,omitempty"`
	// Match is the owner's decision on a CSV or QIF row that matches a
	// posting already in the ledger (#485).
	Match *ImportMatchChoice `json:"match,omitempty"`
}

// --- Input types for service methods ---

type StartImportInput struct {
	Format           string
	HoldingAccountID int64
	CashAccountID    int64
	OwnerUserID      int64
	AuthSessionID    int64
	RequestID        string
	Input            RawInput
	ProfileID        *int64
}

type StartImportResult struct {
	Batch    ImportBatch
	Rows     []ImportStagedRow
	Warnings []ParseWarning
	Meta     SourceMeta
}

type GetImportBatchInput struct {
	BatchID int64
	Limit   int
	Cursor  string
}

type GetImportBatchResult struct {
	Batch ImportBatch
	Rows  []ImportStagedRow
	// Warnings are the parse warnings stored in the batch's source_meta.
	Warnings   []ParseWarning
	NextCursor string
}

type PatchImportBatchInput struct {
	OwnerUserID    int64
	AuthSessionID  int64
	RequestID      string
	BatchID        int64
	RowResolutions []RowResolutionPatch
}

type RowResolutionPatch struct {
	RowID          int64
	DedupeStatus   string
	ResolutionJSON string
}

type CommitImportBatchInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	BatchID       int64
	// ReconciliationOverride, when bound, carries the preview's
	// ReconciliationCheckpoints: each row commits only if the set it would
	// invalidate is contained in that reviewed set (#305).
	ReconciliationOverride CheckpointOverride
	// GainImpactAcknowledgements maps a staged row ID to the preview token for
	// the committed-disposal gain changes its imported acquisition makes (T-126).
	GainImpactAcknowledgements map[int64]string
}

type CommitImportBatchResult struct {
	DividendReviewRowIDs []int64
	BatchID              int64
	Status               string
	TotalRows            int
	CommittedCount       int
	SkippedCount         int
	FailedCount          int
	// GainReviewRowIDs are imported acquisitions whose gain changes were not
	// (or no longer) acknowledged. They stay pending, not skipped, so a fresh
	// preview and commit can post them.
	GainReviewRowIDs []int64
	// ReconciliationHoldRowIDs would cross a reconciliation checkpoint. They
	// stay pending with commit_error "reconciliation override required" until
	// a commit with ReconciliationOverride (#301).
	ReconciliationHoldRowIDs []int64
	// ReconciliationStaleRowIDs would invalidate a checkpoint outside the
	// reviewed set the override names (#305). Nothing was invalidated; they
	// stay pending with commit_error "reconciliation impact changed after
	// review" until another preview and commit.
	ReconciliationStaleRowIDs []int64
	// LinkedCount is the rows committed as links to existing postings: they
	// wrote no journal (#485). They are included in CommittedCount.
	LinkedCount int
	// MatchReviewRowIDs match postings already in the ledger with no
	// standing decision, or their linked posting changed since review. They
	// stay pending until the owner decides again (#485).
	MatchReviewRowIDs []int64
}

type PreviewCommitInput struct {
	OwnerUserID int64
	BatchID     int64
}

type PreviewCommitResult struct {
	IncludableCount      int
	DuplicateCount       int
	ReconciliationIssues []ReconciliationIssuePreview
	// ReconciliationCheckpoints is the union of the checkpoints the pending
	// rows would invalidate, one entry per checkpoint in ID order. A commit's
	// override names these IDs (#305).
	ReconciliationCheckpoints []db.CheckpointInvalidationRef
	// GainImpacts lists each pending imported acquisition whose rolled-back
	// writer replay changes committed disposal gains against the current
	// ledger. Rows commit chronologically, so an earlier row in the same run
	// can change a later row's set; that row is then held for another review.
	GainImpacts []RowGainImpactPreview
	// MatchReviews lists the pending CSV and QIF rows that match postings
	// already in the ledger, with the owner's decision so far (#485).
	MatchReviews []RowMatchReview
	// LinkCount is how many includable rows would commit as links.
	LinkCount int
}

type RowGainImpactPreview struct {
	RowID      int64
	RowIndex   int
	GainImpact db.InvestmentGainImpact
}

type ReconciliationIssuePreview struct {
	RowIndex      int
	CheckpointIDs []int64
}

type DiscardImportBatchInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	BatchID       int64
}

type ListImportBatchesInput struct {
	Limit  int
	Cursor string
}

type ListImportBatchesResult struct {
	Batches    []ImportBatch
	NextCursor string
}

// StartOnlineImportInput starts a new online (fetch-driven) import batch for
// a connection. Unlike StartImportInput, parsing does not happen inline —
// the durable fetch worker stages rows once the fetch completes.
type StartOnlineImportInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	ConnectionID  int64
}

// StartOnlineImportResult carries the newly created (still-fetching) batch.
// Rows are not included; the caller polls GetImportBatch until
// source_meta.fetch_status flips to "ready".
type StartOnlineImportResult struct {
	Batch ImportBatch
}

// RefreshImportConnectionInput requests an incremental fetch on the
// connection's saved cursor, opening a fresh batch for the new rows.
type RefreshImportConnectionInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	ConnectionID  int64
}
