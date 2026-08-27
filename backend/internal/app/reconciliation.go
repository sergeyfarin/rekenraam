package app

import (
	"context"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

var reconciliationSourceKinds = map[string]bool{
	"statement":         true,
	"online_balance":    true,
	"manual_cash_count": true,
	"asset_valuation":   true,
	"other":             true,
}

type ReconciliationSession struct {
	ID                   int64
	BookID               int64
	AccountID            int64
	CommodityID          int64
	Status               string
	SourceKind           string
	StatementDate        string
	StatementBalance     BalanceQuantity
	StartingCheckpointID *int64
	StartingBalance      BalanceQuantity
	SelectedBalance      BalanceQuantity
	Difference           BalanceQuantity
	MetadataJSON         string
	CreatedAt            string
	UpdatedAt            string
	FinishedAt           string
	VoidedAt             string
	ChangeReason         string
	Candidates           []ReconciliationPosting
	SelectedPostings     []ReconciliationPosting
}

type ReconciliationCheckpoint struct {
	ID                   int64
	BookID               int64
	AccountID            int64
	CommodityID          int64
	SessionID            *int64
	PreviousCheckpointID *int64
	Status               string
	StatementDate        string
	StatementBalance     BalanceQuantity
	CreatedAt            string
	InvalidatedAt        string
	InvalidationReason   string
	VoidedAt             string
	VoidReason           string
}

type ReconciliationPosting struct {
	PostingID            int64
	TransactionID        int64
	TransactionVersionID int64
	PostingLineID        int64
	AccountID            int64
	CommodityID          int64
	EntryDate            string
	QuantityValue        exact.Coefficient
	QuantityScale        int
	ReconciliationStatus string
	PayeeName            string
	Description          string
	Selected             bool
}

type StartReconciliationInput struct {
	OwnerUserID           int64
	AuthSessionID         int64
	RequestID             string
	OriginType            string
	AccountID             int64
	CommodityID           int64
	SourceKind            string
	StatementDate         string
	StatementBalanceValue exact.Coefficient
	StatementBalanceScale int
	MetadataJSON          string
	ChangeReason          string
}

type ReconciliationSelectionInput struct {
	OwnerUserID       int64
	AuthSessionID     int64
	RequestID         string
	OriginType        string
	SessionID         int64
	PostingVersionIDs []int64
	ChangeReason      string
}

type FinishReconciliationInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	SessionID     int64
	ChangeReason  string
}

type VoidReconciliationInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	ID            int64
	ChangeReason  string
}

type ListReconciliationCheckpointsInput struct {
	AccountID   int64
	CommodityID int64
}

type ChangePostingReconciliationStatusInput struct {
	OwnerUserID       int64
	AuthSessionID     int64
	RequestID         string
	OriginType        string
	AccountID         int64
	PostingVersionIDs []int64
	Status            string
	ClearedOn         string
	ChangeReason      string
}

func (s *TransactionService) StartReconciliation(ctx context.Context, input StartReconciliationInput) (ReconciliationSession, error) {
	if input.OwnerUserID <= 0 {
		return ReconciliationSession{}, ValidationError{Message: "owner user is required"}
	}
	if input.AccountID <= 0 {
		return ReconciliationSession{}, ValidationError{Message: "account id is required"}
	}
	if input.CommodityID <= 0 {
		return ReconciliationSession{}, ValidationError{Message: "commodity id is required"}
	}
	sourceKind := strings.TrimSpace(input.SourceKind)
	if sourceKind == "" {
		sourceKind = "statement"
	}
	if !reconciliationSourceKinds[sourceKind] {
		return ReconciliationSession{}, ValidationError{Message: "reconciliation source kind is invalid"}
	}
	statementDate, err := cleanRequiredDate(input.StatementDate, "statement date")
	if err != nil {
		return ReconciliationSession{}, err
	}
	if input.StatementBalanceScale < 0 || input.StatementBalanceScale > exact.MaxCryptoScale {
		return ReconciliationSession{}, ValidationError{Message: "statement balance scale is invalid"}
	}
	commodityRule, err := s.repository.PostingCommodityRule(ctx, BookID, input.CommodityID, statementDate)
	if err != nil || commodityRule.Status != "active" {
		return ReconciliationSession{}, ValidationError{Message: "reconciliation commodity is invalid"}
	}
	if input.StatementBalanceScale > commodityRule.MaxQuantityScale || input.StatementBalanceScale > exact.MaxScaleForCommodityKind(commodityRule.CommodityKind) {
		return ReconciliationSession{}, ValidationError{Message: "statement balance scale exceeds commodity precision"}
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "metadata", ledgerJSONMaxBytes)
	if err != nil {
		return ReconciliationSession{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "started reconciliation")
	if err != nil {
		return ReconciliationSession{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.CreateReconciliationSession(ctx, db.CreateReconciliationSessionParams{
		BookID:                BookID,
		AccountID:             input.AccountID,
		CommodityID:           input.CommodityID,
		SourceKind:            sourceKind,
		StatementDate:         statementDate,
		StatementBalanceValue: input.StatementBalanceValue,
		StatementBalanceScale: input.StatementBalanceScale,
		MetadataJSON:          metadataJSON,
		ActorUserID:           input.OwnerUserID,
		AuthSessionID:         input.AuthSessionID,
		RequestID:             input.RequestID,
		OriginType:            input.OriginType,
		OccurredAt:            now,
		ChangeReason:          changeReason,
	})
	if err != nil {
		return ReconciliationSession{}, mapTransactionDBError(err)
	}
	clearedPostingIDs := make([]int64, 0)
	for _, candidate := range record.Candidates {
		if candidate.ReconciliationStatus == "cleared" {
			clearedPostingIDs = append(clearedPostingIDs, candidate.PostingID)
		}
	}
	if len(clearedPostingIDs) > 0 {
		return s.replaceReconciliationSelection(ctx, ReconciliationSelectionInput{
			OwnerUserID:       input.OwnerUserID,
			AuthSessionID:     input.AuthSessionID,
			RequestID:         input.RequestID,
			OriginType:        input.OriginType,
			SessionID:         record.ID,
			PostingVersionIDs: clearedPostingIDs,
			ChangeReason:      "selected cleared postings",
		}, "reconciliation.start_selection", "selected cleared postings")
	}
	return toReconciliationSession(record), nil
}

func (s *TransactionService) ReconciliationSession(ctx context.Context, sessionID int64) (ReconciliationSession, error) {
	if sessionID <= 0 {
		return ReconciliationSession{}, ValidationError{Message: "reconciliation id is required"}
	}
	record, err := s.repository.ReconciliationSessionByID(ctx, BookID, sessionID)
	if err != nil {
		return ReconciliationSession{}, mapTransactionDBError(err)
	}
	return toReconciliationSession(record), nil
}

func (s *TransactionService) PreviewReconciliation(ctx context.Context, input ReconciliationSelectionInput) (ReconciliationSession, error) {
	return s.replaceReconciliationSelection(ctx, input, "reconciliation.preview", "previewed reconciliation")
}

func (s *TransactionService) UpdateReconciliationSelection(ctx context.Context, input ReconciliationSelectionInput) (ReconciliationSession, error) {
	return s.replaceReconciliationSelection(ctx, input, "reconciliation.selection", "updated reconciliation selection")
}

func (s *TransactionService) replaceReconciliationSelection(ctx context.Context, input ReconciliationSelectionInput, operation string, fallbackReason string) (ReconciliationSession, error) {
	if input.OwnerUserID <= 0 {
		return ReconciliationSession{}, ValidationError{Message: "owner user is required"}
	}
	if input.SessionID <= 0 {
		return ReconciliationSession{}, ValidationError{Message: "reconciliation id is required"}
	}
	postingIDs, err := cleanPositiveInt64IDs(input.PostingVersionIDs, "posting id")
	if err != nil {
		return ReconciliationSession{}, err
	}
	session, err := s.repository.ReconciliationSessionByID(ctx, BookID, input.SessionID)
	if err != nil {
		return ReconciliationSession{}, mapTransactionDBError(err)
	}
	selected := make([]db.ReconciliationPostingRecord, 0)
	selectedByID := map[int64]bool{}
	for _, id := range postingIDs {
		selectedByID[id] = true
	}
	for _, candidate := range session.Candidates {
		if selectedByID[candidate.PostingID] {
			selected = append(selected, candidate)
		}
	}
	if len(selected) != len(postingIDs) {
		return ReconciliationSession{}, ErrReconciliationPosting
	}
	selectedBalance, difference, err := reconciliationTotals(session, selected)
	if err != nil {
		return ReconciliationSession{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, fallbackReason)
	if err != nil {
		return ReconciliationSession{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.ReplaceReconciliationSessionSelection(ctx, db.ReconciliationSessionSelectionParams{
		BookID:               BookID,
		SessionID:            input.SessionID,
		PostingVersionIDs:    postingIDs,
		SelectedBalanceValue: selectedBalance.QuantityValue,
		SelectedBalanceScale: selectedBalance.QuantityScale,
		DifferenceValue:      difference.QuantityValue,
		DifferenceScale:      difference.QuantityScale,
		ActorUserID:          input.OwnerUserID,
		AuthSessionID:        input.AuthSessionID,
		RequestID:            input.RequestID,
		OriginType:           input.OriginType,
		OccurredAt:           now,
		Operation:            operation,
		ChangeReason:         changeReason,
	})
	if err != nil {
		return ReconciliationSession{}, mapTransactionDBError(err)
	}
	return toReconciliationSession(record), nil
}

func (s *TransactionService) FinishReconciliation(ctx context.Context, input FinishReconciliationInput) (ReconciliationSession, error) {
	if input.OwnerUserID <= 0 {
		return ReconciliationSession{}, ValidationError{Message: "owner user is required"}
	}
	if input.SessionID <= 0 {
		return ReconciliationSession{}, ValidationError{Message: "reconciliation id is required"}
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "")
	if err != nil {
		return ReconciliationSession{}, err
	}
	if changeReason == "" {
		return ReconciliationSession{}, ValidationError{Message: "change reason is required"}
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.FinishReconciliationSession(ctx, db.FinishReconciliationSessionParams{
		BookID:        BookID,
		SessionID:     input.SessionID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		OccurredAt:    now,
		ChangeReason:  changeReason,
	})
	if err != nil {
		return ReconciliationSession{}, mapTransactionDBError(err)
	}
	return toReconciliationSession(record), nil
}

func (s *TransactionService) VoidReconciliationSession(ctx context.Context, input VoidReconciliationInput) (ReconciliationSession, error) {
	if input.OwnerUserID <= 0 {
		return ReconciliationSession{}, ValidationError{Message: "owner user is required"}
	}
	if input.ID <= 0 {
		return ReconciliationSession{}, ValidationError{Message: "reconciliation id is required"}
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "")
	if err != nil {
		return ReconciliationSession{}, err
	}
	if changeReason == "" {
		return ReconciliationSession{}, ValidationError{Message: "change reason is required"}
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.VoidReconciliationSession(ctx, db.VoidReconciliationParams{
		BookID:        BookID,
		ID:            input.ID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		OccurredAt:    now,
		Operation:     "reconciliation.void",
		ChangeReason:  changeReason,
	})
	if err != nil {
		return ReconciliationSession{}, mapTransactionDBError(err)
	}
	return toReconciliationSession(record), nil
}

func (s *TransactionService) ReconciliationCheckpoints(ctx context.Context, input ListReconciliationCheckpointsInput) ([]ReconciliationCheckpoint, error) {
	if input.AccountID <= 0 {
		return nil, ValidationError{Message: "account id is required"}
	}
	records, err := s.repository.ListReconciliationCheckpoints(ctx, BookID, input.AccountID, input.CommodityID)
	if err != nil {
		return nil, mapTransactionDBError(err)
	}
	checkpoints := make([]ReconciliationCheckpoint, 0, len(records))
	for _, record := range records {
		checkpoints = append(checkpoints, toReconciliationCheckpoint(record))
	}
	return checkpoints, nil
}

func (s *TransactionService) VoidReconciliationCheckpoint(ctx context.Context, input VoidReconciliationInput) (ReconciliationCheckpoint, error) {
	if input.OwnerUserID <= 0 {
		return ReconciliationCheckpoint{}, ValidationError{Message: "owner user is required"}
	}
	if input.ID <= 0 {
		return ReconciliationCheckpoint{}, ValidationError{Message: "checkpoint id is required"}
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "")
	if err != nil {
		return ReconciliationCheckpoint{}, err
	}
	if changeReason == "" {
		return ReconciliationCheckpoint{}, ValidationError{Message: "change reason is required"}
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.VoidReconciliationCheckpoint(ctx, db.VoidReconciliationParams{
		BookID:        BookID,
		ID:            input.ID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		OccurredAt:    now,
		Operation:     "reconciliation_checkpoint.void",
		ChangeReason:  changeReason,
	})
	if err != nil {
		return ReconciliationCheckpoint{}, mapTransactionDBError(err)
	}
	return toReconciliationCheckpoint(record), nil
}

func (s *TransactionService) ChangePostingReconciliationStatus(ctx context.Context, input ChangePostingReconciliationStatusInput) ([]Transaction, error) {
	if input.OwnerUserID <= 0 {
		return nil, ValidationError{Message: "owner user is required"}
	}
	if input.AccountID <= 0 {
		return nil, ValidationError{Message: "account id is required"}
	}
	postingIDs, err := cleanPositiveInt64IDs(input.PostingVersionIDs, "posting id")
	if err != nil {
		return nil, err
	}
	status := strings.TrimSpace(input.Status)
	if status != "cleared" && status != "uncleared" {
		return nil, ValidationError{Message: "posting reconciliation status is invalid"}
	}
	clearedOn, err := cleanOptionalDate(input.ClearedOn, "cleared date")
	if err != nil {
		return nil, err
	}
	if status == "cleared" && clearedOn == "" {
		clearedOn = s.now().UTC().Format(time.DateOnly)
	}
	if status == "uncleared" {
		clearedOn = ""
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "updated reconciliation status")
	if err != nil {
		return nil, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	records, err := s.repository.ChangePostingReconciliationStatus(ctx, db.ChangePostingReconciliationStatusParams{
		BookID:            BookID,
		AccountID:         input.AccountID,
		PostingVersionIDs: postingIDs,
		Status:            status,
		ClearedOn:         nullableSQLString(clearedOn),
		ActorUserID:       input.OwnerUserID,
		AuthSessionID:     input.AuthSessionID,
		RequestID:         input.RequestID,
		OriginType:        input.OriginType,
		OccurredAt:        now,
		ChangeReason:      changeReason,
	})
	if err != nil {
		return nil, mapTransactionDBError(err)
	}
	return toTransactions(records), nil
}

func reconciliationTotals(session db.ReconciliationSessionRecord, selected []db.ReconciliationPostingRecord) (BalanceQuantity, BalanceQuantity, error) {
	selectedAmount := exact.NewScaledInt()
	for _, posting := range selected {
		selectedAmount.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
	}
	selectedValue, err := selectedAmount.Coefficient()
	if err != nil {
		return BalanceQuantity{}, BalanceQuantity{}, err
	}

	expected := exact.NewScaledInt()
	expected.AddCoefficient(session.StartingBalanceValue, session.StartingBalanceScale)
	expected.AddScaled(selectedAmount)

	difference := exact.NewScaledInt()
	difference.AddCoefficient(session.StatementBalanceValue, session.StatementBalanceScale)
	difference.SubScaled(expected)
	differenceValue, err := difference.Coefficient()
	if err != nil {
		return BalanceQuantity{}, BalanceQuantity{}, err
	}
	return BalanceQuantity{
		CommodityID:         session.CommodityID,
		QuantityValue:       selectedValue,
		QuantityScale:       selectedAmount.Scale(),
		NormalQuantityValue: selectedValue,
	}, BalanceQuantity{
		CommodityID:         session.CommodityID,
		QuantityValue:       differenceValue,
		QuantityScale:       difference.Scale(),
		NormalQuantityValue: differenceValue,
	}, nil
}

func cleanPositiveInt64IDs(values []int64, field string) ([]int64, error) {
	if len(values) == 0 {
		return nil, nil
	}
	seen := map[int64]bool{}
	cleaned := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			return nil, ValidationError{Message: field + " is invalid"}
		}
		if !seen[value] {
			seen[value] = true
			cleaned = append(cleaned, value)
		}
	}
	return cleaned, nil
}

func toReconciliationSession(record db.ReconciliationSessionRecord) ReconciliationSession {
	return ReconciliationSession{
		ID:                   record.ID,
		BookID:               record.BookID,
		AccountID:            record.AccountID,
		CommodityID:          record.CommodityID,
		Status:               record.Status,
		SourceKind:           record.SourceKind,
		StatementDate:        record.StatementDate,
		StatementBalance:     reconciliationQuantity(record.CommodityID, record.StatementBalanceValue, record.StatementBalanceScale),
		StartingCheckpointID: nullableInt64FromSQL(record.StartingCheckpointID),
		StartingBalance:      reconciliationQuantity(record.CommodityID, record.StartingBalanceValue, record.StartingBalanceScale),
		SelectedBalance:      reconciliationQuantity(record.CommodityID, record.SelectedBalanceValue, record.SelectedBalanceScale),
		Difference:           reconciliationQuantity(record.CommodityID, record.DifferenceValue, record.DifferenceScale),
		MetadataJSON:         record.MetadataJSON,
		CreatedAt:            record.CreatedAt,
		UpdatedAt:            record.UpdatedAt,
		FinishedAt:           nullableString(record.FinishedAt),
		VoidedAt:             nullableString(record.VoidedAt),
		ChangeReason:         record.ChangeReason,
		Candidates:           toReconciliationPostings(record.Candidates),
		SelectedPostings:     toReconciliationPostings(record.SelectedPostings),
	}
}

func toReconciliationCheckpoint(record db.ReconciliationCheckpointRecord) ReconciliationCheckpoint {
	return ReconciliationCheckpoint{
		ID:                   record.ID,
		BookID:               record.BookID,
		AccountID:            record.AccountID,
		CommodityID:          record.CommodityID,
		SessionID:            nullableInt64FromSQL(record.SessionID),
		PreviousCheckpointID: nullableInt64FromSQL(record.PreviousCheckpointID),
		Status:               record.Status,
		StatementDate:        record.StatementDate,
		StatementBalance:     reconciliationQuantity(record.CommodityID, record.StatementBalanceValue, record.StatementBalanceScale),
		CreatedAt:            record.CreatedAt,
		InvalidatedAt:        nullableString(record.InvalidatedAt),
		InvalidationReason:   record.InvalidationReason,
		VoidedAt:             nullableString(record.VoidedAt),
		VoidReason:           record.VoidReason,
	}
}

func toReconciliationPostings(records []db.ReconciliationPostingRecord) []ReconciliationPosting {
	postings := make([]ReconciliationPosting, 0, len(records))
	for _, record := range records {
		postings = append(postings, ReconciliationPosting{
			PostingID:            record.PostingID,
			TransactionID:        record.TransactionID,
			TransactionVersionID: record.TransactionVersionID,
			PostingLineID:        record.PostingLineID,
			AccountID:            record.AccountID,
			CommodityID:          record.CommodityID,
			EntryDate:            record.EntryDate,
			QuantityValue:        record.QuantityValue,
			QuantityScale:        record.QuantityScale,
			ReconciliationStatus: record.ReconciliationStatus,
			PayeeName:            nullableString(record.PayeeName),
			Description:          record.Description,
			Selected:             record.Selected,
		})
	}
	return postings
}

func reconciliationQuantity(commodityID int64, value exact.Coefficient, scale int) BalanceQuantity {
	return BalanceQuantity{
		CommodityID:         commodityID,
		QuantityValue:       value,
		QuantityScale:       scale,
		NormalQuantityValue: value,
	}
}
