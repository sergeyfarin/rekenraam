package app

import (
	"context"
	"time"

	"rekenraam/backend/internal/db"
)

type attentionReader interface {
	Read(context.Context, int64, string) (db.AttentionRecord, error)
}
type attentionPricing interface {
	Workspace(context.Context, string) (PricingWorkspace, error)
}

type AttentionService struct {
	repository attentionReader
	pricing    attentionPricing
	now        func() time.Time
}

func NewAttentionService(repository attentionReader, pricing attentionPricing) *AttentionService {
	return &AttentionService{repository: repository, pricing: pricing, now: time.Now}
}

type AttentionSource struct {
	DividendReviewCount     *int
	ID, Unit, State, Status string
	Count                   *int
	Failed                  bool
	Err                     error
}

type Attention struct {
	CheckedAt   time.Time
	TotalKnown  int
	Complete    bool
	HasFailures bool
	Sources     []AttentionSource
}

var attentionSources = []struct{ id, unit string }{
	{"drafts", "transaction"}, {"transaction_review", "transaction"},
	{"recurring_blocked", "occurrence"}, {"imports", "batch"},
	{"suggestions", "provider_event"}, {"connections", "connection"},
	{"refresh", "market_source"}, {"backups", "latest_run"}, {"self_check", "latest_run"},
}

// Read composes existing durable queues and the shared current price coverage.
// It never runs a provider, materializes a draft, or starts a self-check.
// Unknown counts stay nil. A running check is not a successful empty check.
func (s *AttentionService) Read(ctx context.Context) Attention {
	result := Attention{CheckedAt: s.now().UTC(), Complete: true, Sources: []AttentionSource{}}
	appendSource := func(source AttentionSource) {
		if source.Count != nil {
			result.TotalKnown += *source.Count
		}
		result.Complete = result.Complete && source.State == "ready"
		result.HasFailures = result.HasFailures || source.Failed || source.State == "error"
		result.Sources = append(result.Sources, source)
	}
	for _, definition := range attentionSources {
		source := AttentionSource{ID: definition.id, Unit: definition.unit, State: "ready"}
		record, err := s.repository.Read(ctx, BookID, source.ID)
		if err != nil {
			source.State, source.Err = "error", err
		} else {
			source.DividendReviewCount = record.DividendReviewCount
			source.Status = record.Status
			source.Failed = record.Status == "failed"
			switch source.ID {
			case "backups", "self_check":
				switch record.Status {
				case "running", "pending":
					source.State = "loading"
				case "failed", "errored", "warning", "incomplete":
					record.Count = 1
					source.Failed = record.Status == "failed" || record.Status == "errored"
				}
			case "connections", "refresh":
				source.Failed = record.Count > 0
				if record.Status == "running" {
					source.State = "loading"
				}
			}
			if source.State == "ready" || record.Count > 0 {
				source.Count = &record.Count
			}
		}
		appendSource(source)
	}
	workspace, err := s.pricing.Workspace(ctx, "")
	for _, definition := range []struct{ id, kind string }{{"prices", "security"}, {"rates", "currency"}} {
		source := AttentionSource{ID: definition.id, Unit: "pair", State: "ready"}
		if err != nil {
			source.State, source.Err = "error", err
		} else {
			kinds := map[int64]string{}
			for _, commodity := range workspace.Commodities {
				kinds[commodity.ID] = commodity.Kind
			}
			count := 0
			seen := map[[2]int64]bool{}
			for _, coverage := range workspace.Coverage {
				pair := [2]int64{coverage.BaseCommodityID, coverage.QuoteCommodityID}
				isCurrency := kinds[coverage.BaseCommodityID] == "currency"
				if (definition.kind == "currency") == isCurrency && coverage.Status != "usable" && !seen[pair] {
					count++
					seen[pair] = true
				}
			}
			source.Count = &count
		}
		appendSource(source)
	}
	return result
}
