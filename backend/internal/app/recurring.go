package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/recur"
)

var (
	ErrRecurringTemplateUnbalanced = errors.New("recurring template is not balanced by commodity")
	ErrRecurringScheduleInvalid    = errors.New("recurring schedule is invalid")
	ErrRecurringTemplateNotFound   = errors.New("recurring template not found")
	ErrRecurringTemplateArchived   = errors.New("recurring template is archived")
	ErrRecurringTemplateConflict   = errors.New("recurring template changed concurrently")
)

// NullablePatch distinguishes omission (preserve) from explicit null (clear).
// JSON decoding belongs to the API; the service receives the same distinction.
type NullablePatch[T any] struct {
	Set   bool
	Value *T
}

type RecurringTemplatePatch struct {
	Name            *string
	Enabled         *bool
	TransactionKind *string
	PayeeID         NullablePatch[int64]
	PayeeName       *string
	Description     *string
	NoteMarkdown    *string
	Frequency       *string
	IntervalCount   *int
	ByWeekday       NullablePatch[int]
	DayOfMonth      NullablePatch[int]
	LastDayOfMonth  *bool
	MonthOfYear     NullablePatch[int]
	StartsOn        *string
	EndsOn          NullablePatch[string]
	MaxOccurrences  NullablePatch[int]
	LeadDays        *int
	Postings        *[]db.RecurringTemplatePostingSpec
	TagIDs          *[]int64
}

type WriteRecurringTemplateInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	TemplateID    int64
	Patch         RecurringTemplatePatch
}

type RecurringTemplate struct {
	ID         int64
	Revision   int64
	Spec       db.RecurringTemplateSpec
	ArchivedAt string
	CreatedAt  string
	UpdatedAt  string
	NextDueOn  string
}

type RecurringService struct {
	repository   *db.RecurringRepository
	transactions *TransactionService
	settings     *SettingsService
	now          func() time.Time
}

func NewRecurringService(repository *db.RecurringRepository, transactions *TransactionService, settings *SettingsService) *RecurringService {
	return &RecurringService{repository: repository, transactions: transactions, settings: settings, now: time.Now}
}

func (s *RecurringService) SetNowForTest(now func() time.Time) { s.now = now }

func (s *RecurringService) localToday(ctx context.Context, ownerID int64) (string, error) {
	if ownerID <= 0 {
		return "", ValidationError{Message: "owner user is required"}
	}
	preferences, err := s.settings.Preferences(ctx, ownerID)
	if err != nil {
		return "", err
	}
	location, err := time.LoadLocation(preferences.TimeZone)
	if err != nil {
		return "", fmt.Errorf("load recurring time zone: %w", err)
	}
	return s.now().In(location).Format(time.DateOnly), nil
}

func (s *RecurringService) ListTemplates(ctx context.Context, ownerID int64, includeArchived bool) ([]RecurringTemplate, error) {
	if ownerID <= 0 {
		return nil, ValidationError{Message: "owner user is required"}
	}
	records, err := s.repository.ListRecurringTemplates(ctx, db.ListRecurringTemplatesParams{BookID: BookID, IncludeArchived: includeArchived})
	if err != nil {
		return nil, mapRecurringError(err)
	}
	result := make([]RecurringTemplate, 0, len(records))
	for _, record := range records {
		template, err := recurringTemplateFromRecord(record)
		if err != nil {
			return nil, err
		}
		result = append(result, template)
	}
	return result, nil
}

func (s *RecurringService) Template(ctx context.Context, ownerID, templateID int64) (RecurringTemplate, error) {
	if ownerID <= 0 {
		return RecurringTemplate{}, ValidationError{Message: "owner user is required"}
	}
	if templateID <= 0 {
		return RecurringTemplate{}, ValidationError{Message: "template id is required"}
	}
	record, err := s.repository.RecurringTemplateByID(ctx, BookID, templateID)
	if err != nil {
		return RecurringTemplate{}, mapRecurringError(err)
	}
	return recurringTemplateFromRecord(record)
}

func (s *RecurringService) CreateTemplate(ctx context.Context, input WriteRecurringTemplateInput) (RecurringTemplate, error) {
	today, err := s.localToday(ctx, input.OwnerUserID)
	if err != nil {
		return RecurringTemplate{}, err
	}
	spec := db.RecurringTemplateSpec{Enabled: true, TransactionKind: "ordinary", IntervalCount: 1, LeadDays: 5}
	mergeRecurringPatch(&spec, input.Patch)
	spec.GenerateFrom = max(today, strings.TrimSpace(spec.StartsOn))
	spec, err = s.cleanTemplate(ctx, spec, today)
	if err != nil {
		return RecurringTemplate{}, err
	}
	record, err := s.repository.CreateRecurringTemplate(ctx, db.CreateRecurringTemplateParams{
		BookID: BookID, ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		CreatedAt: s.now().UTC().Format(time.RFC3339), Spec: spec,
	})
	if err != nil {
		return RecurringTemplate{}, mapRecurringError(err)
	}
	return recurringTemplateFromRecord(record)
}

func (s *RecurringService) UpdateTemplate(ctx context.Context, input WriteRecurringTemplateInput) (RecurringTemplate, error) {
	current, err := s.Template(ctx, input.OwnerUserID, input.TemplateID)
	if err != nil {
		return RecurringTemplate{}, err
	}
	if current.ArchivedAt != "" {
		return RecurringTemplate{}, ErrRecurringTemplateArchived
	}
	today, err := s.localToday(ctx, input.OwnerUserID)
	if err != nil {
		return RecurringTemplate{}, err
	}
	spec := current.Spec
	mergeRecurringPatch(&spec, input.Patch)
	if recurringSchedulePatched(input.Patch) {
		// Re-enumerate only the unmaterialized future. Existing occurrence rows
		// retain their identities and the generator will exclude them (slice 3).
		spec.GenerateFrom = max(today, strings.TrimSpace(spec.StartsOn))
	}
	spec, err = s.cleanTemplate(ctx, spec, today)
	if err != nil {
		return RecurringTemplate{}, err
	}
	record, err := s.repository.UpdateRecurringTemplate(ctx, db.UpdateRecurringTemplateParams{
		BookID: BookID, TemplateID: input.TemplateID, ActorUserID: input.OwnerUserID,
		AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		UpdatedAt: s.now().UTC().Format(time.RFC3339), Spec: spec, ExpectedRevision: &current.Revision,
	})
	if err != nil {
		return RecurringTemplate{}, mapRecurringError(err)
	}
	return recurringTemplateFromRecord(record)
}

func (s *RecurringService) ArchiveTemplate(ctx context.Context, input WriteRecurringTemplateInput) (RecurringTemplate, error) {
	current, err := s.Template(ctx, input.OwnerUserID, input.TemplateID)
	if err != nil {
		return RecurringTemplate{}, err
	}
	if current.ArchivedAt != "" {
		return current, nil
	}
	record, err := s.repository.ArchiveRecurringTemplate(ctx, db.ArchiveRecurringTemplateParams{
		BookID: BookID, TemplateID: input.TemplateID, ActorUserID: input.OwnerUserID,
		AuthSessionID: input.AuthSessionID, RequestID: input.RequestID, ArchivedAt: s.now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return RecurringTemplate{}, mapRecurringError(err)
	}
	return recurringTemplateFromRecord(record)
}

func mergeRecurringPatch(spec *db.RecurringTemplateSpec, patch RecurringTemplatePatch) {
	applyRecurringField(&spec.Name, patch.Name)
	applyRecurringField(&spec.Enabled, patch.Enabled)
	applyRecurringField(&spec.TransactionKind, patch.TransactionKind)
	applyRecurringField(&spec.PayeeName, patch.PayeeName)
	if patch.PayeeID.Set {
		spec.PayeeID = patch.PayeeID.Value
		if patch.PayeeID.Value == nil && patch.PayeeName == nil {
			spec.PayeeName = ""
		}
	}
	// Choosing a new typed name intentionally unlinks the old payee unless
	// this PATCH also explicitly chooses an ID. Validation resolves known names.
	if patch.PayeeName != nil && !patch.PayeeID.Set {
		spec.PayeeID = nil
	}
	applyRecurringField(&spec.Description, patch.Description)
	applyRecurringField(&spec.NoteMarkdown, patch.NoteMarkdown)
	applyRecurringField(&spec.Frequency, patch.Frequency)
	applyRecurringField(&spec.IntervalCount, patch.IntervalCount)
	if patch.ByWeekday.Set {
		spec.ByWeekday = patch.ByWeekday.Value
	}
	if patch.DayOfMonth.Set {
		spec.DayOfMonth = patch.DayOfMonth.Value
	}
	applyRecurringField(&spec.LastDayOfMonth, patch.LastDayOfMonth)
	if patch.MonthOfYear.Set {
		spec.MonthOfYear = patch.MonthOfYear.Value
	}
	applyRecurringField(&spec.StartsOn, patch.StartsOn)
	if patch.EndsOn.Set {
		spec.EndsOn = ""
		applyRecurringField(&spec.EndsOn, patch.EndsOn.Value)
	}
	if patch.MaxOccurrences.Set {
		spec.MaxOccurrences = patch.MaxOccurrences.Value
	}
	applyRecurringField(&spec.LeadDays, patch.LeadDays)
	applyRecurringField(&spec.Postings, patch.Postings)
	applyRecurringField(&spec.TagIDs, patch.TagIDs)
}

func applyRecurringField[T any](target *T, value *T) {
	if value != nil {
		*target = *value
	}
}

func recurringSchedulePatched(p RecurringTemplatePatch) bool {
	return p.Frequency != nil || p.IntervalCount != nil || p.ByWeekday.Set || p.DayOfMonth.Set ||
		p.LastDayOfMonth != nil || p.MonthOfYear.Set || p.StartsOn != nil || p.EndsOn.Set || p.MaxOccurrences.Set
}

func recurringSchedule(spec db.RecurringTemplateSpec) recur.ScheduleSpec {
	result := recur.ScheduleSpec{Frequency: recur.Frequency(spec.Frequency), IntervalCount: spec.IntervalCount,
		LastDayOfMonth: spec.LastDayOfMonth, StartsOn: spec.StartsOn, EndsOn: spec.EndsOn}
	if spec.ByWeekday != nil {
		weekday := time.Weekday(*spec.ByWeekday)
		result.ByWeekday = &weekday
	}
	applyRecurringField(&result.DayOfMonth, spec.DayOfMonth)
	applyRecurringField(&result.MonthOfYear, spec.MonthOfYear)
	applyRecurringField(&result.MaxOccurrences, spec.MaxOccurrences)
	return result
}

func (s *RecurringService) cleanTemplate(ctx context.Context, spec db.RecurringTemplateSpec, today string) (db.RecurringTemplateSpec, error) {
	var err error
	spec.Name, err = cleanOptionalText(spec.Name, "template name", transactionTextMaxBytes)
	if err != nil {
		return spec, err
	}
	if spec.Name == "" {
		return spec, ValidationError{Message: "template name is required"}
	}
	spec.Frequency = strings.TrimSpace(spec.Frequency)
	spec.StartsOn = strings.TrimSpace(spec.StartsOn)
	spec.EndsOn = strings.TrimSpace(spec.EndsOn)
	if err := recurringSchedule(spec).Validate(); err != nil {
		return spec, ErrRecurringScheduleInvalid
	}
	// The enumerator ignores irrelevant fields; the persisted template must
	// still mean exactly one thing (and satisfy the schema's stronger checks).
	if (spec.Frequency != "weekly" && spec.ByWeekday != nil) ||
		(spec.Frequency != "yearly" && spec.MonthOfYear != nil) ||
		((spec.Frequency == "daily" || spec.Frequency == "weekly") && (spec.DayOfMonth != nil || spec.LastDayOfMonth)) ||
		(spec.LastDayOfMonth && spec.DayOfMonth != nil) ||
		(spec.MaxOccurrences != nil && *spec.MaxOccurrences < 1) || spec.IntervalCount > 10000 {
		return spec, ErrRecurringScheduleInvalid
	}
	if spec.LeadDays < 0 || spec.LeadDays > 90 {
		return spec, ErrRecurringScheduleInvalid
	}
	spec.TransactionKind = strings.TrimSpace(spec.TransactionKind)
	if spec.TransactionKind != "ordinary" && spec.TransactionKind != "transfer" {
		return spec, ValidationError{Message: "recurring templates support ordinary and transfer transactions only"}
	}
	if len(spec.Postings) < 2 || len(spec.Postings) > 500 {
		return spec, ValidationError{Message: "template requires 2 to 500 postings"}
	}
	accountIDs, commodityIDs := []int64{}, []int64{}
	seenAccounts, seenCommodities := map[int64]bool{}, map[int64]bool{}
	for _, posting := range spec.Postings {
		if !seenAccounts[posting.AccountID] {
			accountIDs = append(accountIDs, posting.AccountID)
			seenAccounts[posting.AccountID] = true
		}
		if !seenCommodities[posting.CommodityID] {
			commodityIDs = append(commodityIDs, posting.CommodityID)
			seenCommodities[posting.CommodityID] = true
		}
	}
	accounts, err := s.transactions.accountRepository.AccountsByIDs(ctx, BookID, accountIDs)
	if err != nil {
		return spec, fmt.Errorf("read recurring posting accounts: %w", err)
	}
	commodities, err := s.transactions.commodityRepository.CommoditiesByIDs(ctx, BookID, commodityIDs)
	if err != nil {
		return spec, fmt.Errorf("read recurring posting commodities: %w", err)
	}
	for _, account := range accounts {
		if account.AccountKind == "security_holding" {
			return spec, ValidationError{Message: "recurring investment postings are not supported"}
		}
	}
	for _, commodity := range commodities {
		if commodity.Kind == "security" {
			return spec, ValidationError{Message: "recurring investment postings are not supported"}
		}
	}
	postings := make([]PostingInput, 0, len(spec.Postings))
	keys := map[string]bool{}
	totals := map[int64]*exact.ScaledInt{}
	for _, posting := range spec.Postings {
		key := strings.TrimSpace(posting.LineKey)
		if key != "" && keys[key] {
			return spec, ValidationError{Message: "template posting line keys must be unique"}
		}
		keys[key] = true
		if posting.QuantityScale < 0 || posting.QuantityScale > exact.MaxCryptoScale {
			return spec, ValidationError{Message: "posting quantity scale is invalid"}
		}
		if totals[posting.CommodityID] == nil {
			totals[posting.CommodityID] = exact.NewScaledInt()
		}
		totals[posting.CommodityID].AddCoefficient(posting.QuantityValue, posting.QuantityScale)
		postings = append(postings, PostingInput{LineKey: key, AccountID: posting.AccountID, CommodityID: posting.CommodityID,
			QuantityValue: posting.QuantityValue, QuantityScale: posting.QuantityScale, Memo: posting.Memo})
	}
	for _, total := range totals {
		if total.Sign() != 0 {
			return spec, ErrRecurringTemplateUnbalanced
		}
	}
	// This is validation only: no journal, audit, FX work or reconciliation
	// invalidation is produced by saving a template. Generation validates again
	// for each actual occurrence date through the same transaction consumer.
	cleaned, err := s.transactions.cleanTransactionSpec(ctx, TransactionInput{
		Status: "posted", TransactionKind: spec.TransactionKind, TransactionDate: max(today, spec.StartsOn),
		PayeeID: spec.PayeeID, PayeeName: spec.PayeeName, Description: spec.Description,
		NoteMarkdown: spec.NoteMarkdown, TagIDs: spec.TagIDs,
		JournalEntries: []JournalEntryInput{{EntryKind: "ordinary", Postings: postings}},
	}, cleanTransactionOptions{DefaultStatus: "posted"})
	if err != nil {
		return spec, err
	}
	spec.PayeeID = nil
	spec.PayeeName = cleaned.PayeeName.String
	if cleaned.PayeeID.Valid {
		id := cleaned.PayeeID.Int64
		spec.PayeeID = &id
		spec.PayeeName = ""
	}
	spec.Description, spec.NoteMarkdown, spec.TagIDs = cleaned.Description, cleaned.NoteMarkdown, cleaned.TagIDs
	spec.Postings = make([]db.RecurringTemplatePostingSpec, 0, len(postings))
	for _, posting := range cleaned.JournalEntries[0].Postings {
		spec.Postings = append(spec.Postings, db.RecurringTemplatePostingSpec{LineKey: posting.LineKey, AccountID: posting.AccountID,
			QuantityValue: posting.QuantityValue, QuantityScale: posting.QuantityScale, CommodityID: posting.CommodityID, Memo: posting.Memo})
	}
	return spec, nil
}

func recurringTemplateFromRecord(record db.RecurringTemplateRecord) (RecurringTemplate, error) {
	spec := db.RecurringTemplateSpec{Name: record.Name, Enabled: record.Enabled, TransactionKind: record.TransactionKind,
		PayeeName: record.PayeeName.String, Description: record.Description, NoteMarkdown: record.NoteMarkdown,
		Frequency: record.Frequency, IntervalCount: record.IntervalCount, LastDayOfMonth: record.LastDayOfMonth,
		StartsOn: record.StartsOn, EndsOn: record.EndsOn.String, LeadDays: record.LeadDays, GenerateFrom: record.GenerateFrom,
		TagIDs: record.TagIDs, Postings: make([]db.RecurringTemplatePostingSpec, 0, len(record.Postings))}
	if record.PayeeID.Valid {
		value := record.PayeeID.Int64
		spec.PayeeID = &value
	}
	if record.ByWeekday.Valid {
		value := int(record.ByWeekday.Int64)
		spec.ByWeekday = &value
	}
	if record.DayOfMonth.Valid {
		value := int(record.DayOfMonth.Int64)
		spec.DayOfMonth = &value
	}
	if record.MonthOfYear.Valid {
		value := int(record.MonthOfYear.Int64)
		spec.MonthOfYear = &value
	}
	if record.MaxOccurrences.Valid {
		value := int(record.MaxOccurrences.Int64)
		spec.MaxOccurrences = &value
	}
	for _, posting := range record.Postings {
		spec.Postings = append(spec.Postings, db.RecurringTemplatePostingSpec{LineKey: posting.LineKey, AccountID: posting.AccountID,
			QuantityValue: posting.QuantityValue, QuantityScale: posting.QuantityScale, CommodityID: posting.CommodityID, Memo: posting.Memo})
	}
	result := RecurringTemplate{ID: record.ID, Revision: record.Revision, Spec: spec,
		ArchivedAt: record.ArchivedAt.String, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
	if spec.Enabled && result.ArchivedAt == "" {
		from, err := time.Parse(time.DateOnly, max(spec.StartsOn, spec.GenerateFrom))
		if err != nil {
			return RecurringTemplate{}, fmt.Errorf("read recurring watermark: %w", err)
		}
		next, _, err := recur.Next(recurringSchedule(spec), from.AddDate(0, 0, -1).Format(time.DateOnly))
		if err != nil {
			return RecurringTemplate{}, fmt.Errorf("read next recurring date: %w", err)
		}
		result.NextDueOn = next
	}
	return result, nil
}

func mapRecurringError(err error) error {
	switch {
	case errors.Is(err, db.ErrRecurringTemplateNotFound):
		return ErrRecurringTemplateNotFound
	case errors.Is(err, db.ErrRecurringTemplateArchived):
		return ErrRecurringTemplateArchived
	case errors.Is(err, db.ErrRecurringTemplateConflict):
		return ErrRecurringTemplateConflict
	default:
		return err
	}
}
