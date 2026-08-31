package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/recur"
)

var ErrRecurringOccurrenceMaterialized = errors.New("recurring occurrence already materialized or changed")

type RecurringAmount struct {
	CommodityID   int64
	CommodityCode string
	DebitValue    exact.Coefficient
	CreditValue   exact.Coefficient // signed, non-positive; retain imbalance in edited drafts
	QuantityScale int
}

type RecurringDueItem struct {
	db.RecurringDueRecord
	Amounts []RecurringAmount
}

type RecurringDuePage struct {
	Items      []RecurringDueItem
	NextCursor string
}

type recurringDueCursor struct {
	Date string
	ID   int64
}

func (s *RecurringService) Due(ctx context.Context, ownerID int64, cursor string, limit int) (RecurringDuePage, error) {
	if ownerID <= 0 {
		return RecurringDuePage{}, ValidationError{Message: "owner user is required"}
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 {
		return RecurringDuePage{}, ValidationError{Message: "limit must be between 1 and 200"}
	}
	var after recurringDueCursor
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(raw, &after) != nil || !validRecurringDate(after.Date) || after.ID <= 0 {
			return RecurringDuePage{}, ValidationError{Message: "invalid recurring cursor"}
		}
	}
	rows, err := s.repository.RecurringDue(ctx, BookID, after.Date, after.ID, limit+1)
	if err != nil {
		return RecurringDuePage{}, err
	}
	page := RecurringDuePage{Items: make([]RecurringDueItem, 0, min(len(rows), limit))}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		data, err := json.Marshal(recurringDueCursor{Date: last.OccurrenceDate, ID: last.ID})
		if err != nil {
			return RecurringDuePage{}, err
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString(data)
	}
	for _, row := range rows {
		amounts, err := recurringAmounts(row.Postings)
		if err != nil {
			return RecurringDuePage{}, err
		}
		page.Items = append(page.Items, RecurringDueItem{RecurringDueRecord: row, Amounts: amounts})
	}
	return page, nil
}

func recurringAmounts(postings []db.RecurringDuePosting) ([]RecurringAmount, error) {
	type total struct {
		code          string
		debit, credit *exact.ScaledInt
	}
	totals := map[int64]*total{}
	for _, p := range postings {
		if totals[p.CommodityID] == nil {
			totals[p.CommodityID] = &total{p.CommodityCode, exact.NewScaledInt(), exact.NewScaledInt()}
		}
		t := totals[p.CommodityID]
		if p.QuantityValue.Sign() >= 0 {
			t.debit.AddCoefficient(p.QuantityValue, p.QuantityScale)
		} else {
			t.credit.AddCoefficient(p.QuantityValue, p.QuantityScale)
		}
	}
	ids := make([]int64, 0, len(totals))
	for id := range totals {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	result := make([]RecurringAmount, 0, len(ids))
	for _, id := range ids {
		t := totals[id]
		scale := max(t.debit.Scale(), t.credit.Scale())
		t.debit.Align(scale)
		t.credit.Align(scale)
		debit, err := t.debit.Coefficient()
		if err != nil {
			return nil, LedgerOverflowError{CommodityID: id}
		}
		credit, err := t.credit.Coefficient()
		if err != nil {
			return nil, LedgerOverflowError{CommodityID: id}
		}
		result = append(result, RecurringAmount{CommodityID: id, CommodityCode: t.code, DebitValue: debit, CreditValue: credit, QuantityScale: scale})
	}
	return result, nil
}

type RecurringOccurrence struct {
	Date   string
	Status string // scheduled is computed, never persisted
	Record *db.RecurringOccurrenceRecord
}

func validRecurringDate(date string) bool {
	parsed, err := time.Parse(time.DateOnly, date)
	return err == nil && parsed.Year() >= 1 && parsed.Format(time.DateOnly) == date
}

// Occurrences merges the current schedule with historical identities, including
// occurrences left behind by schedule edits. Reads never create rows.
func (s *RecurringService) Occurrences(ctx context.Context, ownerID, templateID int64, from, to string) ([]RecurringOccurrence, error) {
	if !validRecurringDate(from) || !validRecurringDate(to) || from > to {
		return nil, ValidationError{Message: "invalid occurrence date range"}
	}
	start, _ := time.Parse(time.DateOnly, from)
	end, _ := time.Parse(time.DateOnly, to)
	if end.Sub(start) > 366*24*time.Hour {
		return nil, ValidationError{Message: "occurrence range cannot exceed 367 inclusive dates"}
	}
	template, err := s.Template(ctx, ownerID, templateID)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.RecurringOccurrencesInRange(ctx, BookID, templateID, from, to)
	if err != nil {
		return nil, err
	}
	merged := map[string]RecurringOccurrence{}
	var dates []string
	if max(from, template.Spec.GenerateFrom) <= to {
		dates, err = recur.Occurrences(recurringSchedule(template.Spec), max(from, template.Spec.GenerateFrom), to)
		if err != nil {
			return nil, err
		}
	}
	for _, date := range dates {
		merged[date] = RecurringOccurrence{Date: date, Status: "scheduled"}
	}
	for _, record := range records {
		merged[record.OccurrenceDate] = RecurringOccurrence{Date: record.OccurrenceDate, Status: record.Status, Record: &record}
	}
	result := make([]RecurringOccurrence, 0, len(merged))
	for _, item := range merged {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date < result[j].Date })
	return result, nil
}

type ReviewRecurringInput struct {
	OwnerUserID    int64
	AuthSessionID  int64
	RequestID      string
	TemplateID     int64
	OccurrenceDate string
	Reason         string
}

func (s *RecurringService) reviewOccurrence(ctx context.Context, input ReviewRecurringInput) (RecurringTemplate, *db.RecurringOccurrenceRecord, error) {
	if !validRecurringDate(input.OccurrenceDate) {
		return RecurringTemplate{}, nil, ErrRecurringScheduleInvalid
	}
	template, err := s.Template(ctx, input.OwnerUserID, input.TemplateID)
	if err != nil {
		return template, nil, err
	}
	records, err := s.repository.RecurringOccurrencesInRange(ctx, BookID, template.ID, input.OccurrenceDate, input.OccurrenceDate)
	if err != nil {
		return template, nil, err
	}
	if len(records) > 0 {
		return template, &records[0], nil
	}
	return template, nil, nil
}

func (s *RecurringService) SkipOccurrence(ctx context.Context, input ReviewRecurringInput) error {
	template, current, err := s.reviewOccurrence(ctx, input)
	if err != nil {
		return err
	}
	reason, err := cleanOptionalText(input.Reason, "skip reason", transactionTextMaxBytes)
	if err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return ValidationError{Message: "skip reason is required"}
	}
	var expected *int64
	if current != nil {
		if current.Status != "blocked" || !current.LastAuditEventID.Valid {
			return ErrRecurringOccurrenceMaterialized
		}
		expected = &current.LastAuditEventID.Int64
	} else {
		if template.ArchivedAt != "" {
			return ErrRecurringTemplateArchived
		}
		dates, err := recur.Occurrences(recurringSchedule(template.Spec), input.OccurrenceDate, input.OccurrenceDate)
		if err != nil || len(dates) != 1 || input.OccurrenceDate < template.Spec.GenerateFrom {
			return ErrRecurringScheduleInvalid
		}
	}
	at := s.now().UTC().Format(time.RFC3339)
	return mapRecurringError(s.repository.SkipRecurringOccurrence(ctx, template.Revision, db.CreateRecurringOccurrenceParams{
		BookID: BookID, TemplateID: template.ID, OccurrenceDate: input.OccurrenceDate, SkipReason: reason, MaterializedAt: at}, expected, db.AuditEventParams{
		BookID: BookID, ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		OccurredAt: at, OriginType: "browser_api", Operation: "recurring.occurrence.skip", Reason: reason,
		MetadataJSON: fmt.Sprintf(`{"template_id":%d,"occurrence_date":%q}`, template.ID, input.OccurrenceDate),
	}))
}

func (s *RecurringService) RetryOccurrence(ctx context.Context, input ReviewRecurringInput) (RecurringGenerationResult, error) {
	template, current, err := s.reviewOccurrence(ctx, input)
	if err != nil {
		return RecurringGenerationResult{}, err
	}
	if template.ArchivedAt != "" {
		return RecurringGenerationResult{}, ErrRecurringTemplateArchived
	}
	if !template.Spec.Enabled {
		return RecurringGenerationResult{}, ValidationError{Message: "enable the recurring template before retrying"}
	}
	if current == nil || current.Status != "blocked" || !current.LastAuditEventID.Valid {
		return RecurringGenerationResult{}, ErrRecurringOccurrenceMaterialized
	}
	blocked, err := s.materializeOccurrenceAttempt(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID, TemplateID: template.ID}, template, input.OccurrenceDate, &current.LastAuditEventID.Int64)
	if err != nil {
		return RecurringGenerationResult{}, mapRecurringError(err)
	}
	if blocked {
		return RecurringGenerationResult{Blocked: 1}, nil
	}
	return RecurringGenerationResult{Generated: 1}, nil
}
