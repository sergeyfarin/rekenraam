package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/recur"
)

const maxOccurrencesPerTick = 50

type GenerateRecurringInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	// Zero sweeps all enabled templates; a positive ID is the service-only
	// run-now entry point. Neither is exposed publicly before slice 5.
	TemplateID int64
}

type RecurringGenerationResult struct {
	Generated int
	Blocked   int
}

// GenerateDue creates only drafts. Validation failures become terminal blocked
// occurrences; infrastructure errors leave the date unrecorded for a later tick.
func (s *RecurringService) GenerateDue(ctx context.Context, input GenerateRecurringInput) (RecurringGenerationResult, error) {
	var result RecurringGenerationResult
	today, err := s.localToday(ctx, input.OwnerUserID)
	if err != nil {
		return result, err
	}
	var templates []RecurringTemplate
	if input.TemplateID > 0 {
		template, err := s.Template(ctx, input.OwnerUserID, input.TemplateID)
		if err != nil {
			return result, err
		}
		templates = []RecurringTemplate{template}
	} else {
		templates, err = s.ListTemplates(ctx, input.OwnerUserID, false)
		if err != nil {
			return result, err
		}
	}
	localDate, err := time.Parse(time.DateOnly, today)
	if err != nil {
		return result, err
	}
	for _, template := range templates {
		if !template.Spec.Enabled || template.ArchivedAt != "" {
			continue
		}
		end := localDate.AddDate(0, 0, template.Spec.LeadDays)
		if end.Year() > 9999 {
			end = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		}
		to := end.Format(time.DateOnly)
		if template.Spec.GenerateFrom > to {
			continue
		}
		dates, err := recur.Occurrences(recurringSchedule(template.Spec), template.Spec.GenerateFrom, to)
		if err != nil {
			return result, fmt.Errorf("enumerate recurring dates: %w", err)
		}
		existing, err := s.repository.RecurringOccurrenceDates(ctx, BookID, template.ID, template.Spec.GenerateFrom, to)
		if err != nil {
			return result, err
		}
		stale := false
		for _, date := range dates {
			if _, ok := existing[date]; ok {
				continue
			}
			if result.Generated+result.Blocked >= maxOccurrencesPerTick {
				return result, nil
			}
			blocked, err := s.materializeOccurrence(ctx, input, template, date)
			if errors.Is(err, db.ErrRecurringOccurrenceExists) {
				continue
			}
			if errors.Is(err, db.ErrRecurringTemplateConflict) {
				// The next tick reads the edited template afresh. Never move a
				// new schedule's watermark based on the old snapshot.
				stale = true
				break
			}
			if err != nil {
				return result, err
			}
			if blocked {
				result.Blocked++
			} else {
				result.Generated++
			}
		}
		if stale {
			continue
		}
		next := end.AddDate(0, 0, 1)
		// The last representable date is safe to re-read: its occurrence
		// identity still makes it idempotent without a five-digit watermark.
		if next.Year() > 9999 {
			next = end
		}
		err = s.repository.AdvanceRecurringGeneration(ctx, BookID, template.ID, template.Revision, next.Format(time.DateOnly), s.now().UTC().Format(time.RFC3339))
		if err != nil && !errors.Is(err, db.ErrRecurringTemplateConflict) {
			return result, err
		}
	}
	return result, nil
}

func (s *RecurringService) materializeOccurrence(ctx context.Context, input GenerateRecurringInput, template RecurringTemplate, date string) (bool, error) {
	return s.materializeOccurrenceAttempt(ctx, input, template, date, nil)
}

func (s *RecurringService) materializeOccurrenceAttempt(ctx context.Context, input GenerateRecurringInput, template RecurringTemplate, date string, expectedAuditID *int64) (bool, error) {
	at := s.now().UTC().Format(time.RFC3339)
	occurrence := db.CreateRecurringOccurrenceParams{BookID: BookID, TemplateID: template.ID, OccurrenceDate: date, MaterializedAt: at}
	// Reuse the full template and transaction validators at the actual date,
	// including exact balancing (ordinary incomplete drafts need not balance).
	spec, err := s.cleanTemplate(ctx, template.Spec, date)
	var draft *db.CreateTransactionParams
	if err == nil {
		postings := make([]PostingInput, 0, len(spec.Postings))
		for _, p := range spec.Postings {
			postings = append(postings, PostingInput{LineKey: p.LineKey, AccountID: p.AccountID, CommodityID: p.CommodityID, QuantityValue: p.QuantityValue, QuantityScale: p.QuantityScale, Memo: p.Memo})
		}
		params, prepareErr := s.transactions.prepareCreateTransactionForWrite(ctx, CreateTransactionInput{
			OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
			OriginType: "scheduled", Operation: "recurring.occurrence.generate", ChangeReason: "generated recurring draft",
			Spec: TransactionInput{Status: "draft", TransactionKind: spec.TransactionKind, TransactionDate: date,
				PayeeID: spec.PayeeID, PayeeName: spec.PayeeName, Description: spec.Description, NoteMarkdown: spec.NoteMarkdown, TagIDs: spec.TagIDs,
				JournalEntries: []JournalEntryInput{{EntryKind: "ordinary", EntryDate: date, Postings: postings}}},
		})
		err = prepareErr
		params.CreatedAt = at
		draft = &params
	}
	if err != nil {
		var validation ValidationError
		var overflow LedgerOverflowError
		if !errors.As(err, &validation) && !errors.As(err, &overflow) && !errors.Is(err, ErrTransactionTag) &&
			!errors.Is(err, ErrRecurringTemplateUnbalanced) && !errors.Is(err, ErrRecurringScheduleInvalid) {
			return false, err
		}
		occurrence.ErrorSummary = err.Error()
		draft = nil
	}
	audit := db.AuditEventParams{
		BookID: BookID, ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		OccurredAt: at, OriginType: "scheduled", Operation: "recurring.occurrence.block", Reason: "recurring occurrence failed validation",
		MetadataJSON: fmt.Sprintf(`{"template_id":%d,"occurrence_date":%q}`, template.ID, date),
	}
	if expectedAuditID != nil {
		audit.OriginType, audit.Operation, audit.Reason = "browser_api", "recurring.occurrence.retry", "retry blocked recurring occurrence"
		if draft != nil {
			draft.Operation, draft.ChangeReason = audit.Operation, audit.Reason
		}
		return draft == nil, s.repository.RetryRecurringOccurrence(ctx, template.Revision, occurrence, draft, audit, *expectedAuditID)
	}
	return draft == nil, s.repository.MaterializeRecurringOccurrence(ctx, template.Revision, occurrence, draft, audit)
}

// StartScheduler is intentionally NOT wired in command.go. Activation waits
// for the dedicated review/discard UI in R9 slice 5.
func (s *RecurringService) StartScheduler(ctx context.Context, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	go func() {
		s.scheduleRecurringIfDue(ctx, logger)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.scheduleRecurringIfDue(ctx, logger)
			}
		}
	}()
}

func (s *RecurringService) scheduleRecurringIfDue(ctx context.Context, logger *slog.Logger) {
	ownerID, err := s.repository.CurrentBookOwnerID(ctx, BookID)
	if errors.Is(err, db.ErrNotFound) || ctx.Err() != nil {
		return
	}
	if err == nil {
		_, err = s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: ownerID})
	}
	if err != nil && ctx.Err() == nil {
		// Underlying errors can contain financial input. Log no such content.
		logger.WarnContext(ctx, "recurring generation could not complete")
	}
}
