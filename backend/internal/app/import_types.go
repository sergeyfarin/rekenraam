package app

import "errors"

var (
	ErrImportBatchNotFound = errors.New("import batch not found")
	ErrImportBatchNotOpen  = errors.New("import batch is not in previewing state")
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
	ResolutionJSON         string
	CommitStatus           string
	CommittedTransactionID *int64
	CommitError            string
}

// ParseWarning is a non-fatal issue found during parsing.
type ParseWarning struct {
	RowIndex int
	Message  string
}

// SourceMeta carries hints extracted from the file about account/currency/date range.
type SourceMeta struct {
	AccountHints  []string
	CurrencyHints []string
	DateFrom      string
	DateTo        string
}

// StagedSplit is a sub-posting within a staged row.
type StagedSplit struct {
	CategoryHint string
	Amount       string
	Memo         string
}

// StagedRow is the canonical, format-independent row produced by a SourceAdapter.
type StagedRow struct {
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
}

// ParseResult is what a SourceAdapter returns.
type ParseResult struct {
	Rows     []StagedRow
	Warnings []ParseWarning
	Meta     SourceMeta
}

// RawInput carries the raw bytes (and optional metadata) for a source adapter.
type RawInput struct {
	Filename    string
	ContentType string
	Bytes       []byte
	Rows        [][]string
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
	Name        string
	AdapterKind string
	ConfigJSON  string
}

// ImportRowResolution carries the user-chosen account/category/payee for a staged row.
type ImportRowResolution struct {
	AccountID         int64  `json:"account_id,omitempty"`
	CommodityID       int64  `json:"commodity_id,omitempty"`
	PayeeID           *int64 `json:"payee_id,omitempty"`
	PayeeName         string `json:"payee_name,omitempty"`
	CategoryID        *int64 `json:"category_id,omitempty"`
	TransferAccountID *int64 `json:"transfer_account_id,omitempty"`
	Exclude           bool   `json:"exclude,omitempty"`
}

// --- Input types for service methods ---

type StartImportInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	Input         RawInput
	ProfileID     *int64
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
	Batch      ImportBatch
	Rows       []ImportStagedRow
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
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	BatchID                int64
	ReconciliationOverride bool
}

type CommitImportBatchResult struct {
	BatchID        int64
	Status         string
	TotalRows      int
	CommittedCount int
	SkippedCount   int
	FailedCount    int
}

type PreviewCommitInput struct {
	OwnerUserID int64
	BatchID     int64
}

type PreviewCommitResult struct {
	IncludableCount      int
	DuplicateCount       int
	ReconciliationIssues []ReconciliationIssuePreview
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
