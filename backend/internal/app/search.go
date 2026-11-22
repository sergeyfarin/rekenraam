package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
)

type SearchService struct {
	repository *db.SearchRepository
	settings   *SettingsService
	now        func() time.Time
}

func NewSearchService(repository *db.SearchRepository, settings *SettingsService) *SearchService {
	return &SearchService{repository: repository, settings: settings, now: time.Now}
}

func (s *SearchService) SetNowForTest(now func() time.Time) { s.now = now }

type SearchCatalog struct {
	Items      []db.SearchCatalogItem
	NextCursor string
}

func (s *SearchService) Catalog(ctx context.Context, cursor string, limit int) (SearchCatalog, error) {
	if limit < 1 || limit > 500 {
		return SearchCatalog{}, ValidationError{Message: "limit is invalid"}
	}
	kind := ""
	var id int64
	if cursor != "" {
		parts := strings.Split(cursor, ":")
		if len(parts) != 2 || (parts[0] != "account" && parts[0] != "category" && parts[0] != "instrument" && parts[0] != "payee") {
			return SearchCatalog{}, ValidationError{Message: "cursor is invalid"}
		}
		parsed, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != parts[1] {
			return SearchCatalog{}, ValidationError{Message: "cursor is invalid"}
		}
		kind, id = parts[0], parsed
	}
	ctx, err := s.settings.referenceContext(ctx, s.now())
	if err != nil {
		return SearchCatalog{}, err
	}
	items, err := s.repository.Catalog(ctx, BookID, kind, id, limit+1)
	if err != nil {
		return SearchCatalog{}, err
	}
	result := SearchCatalog{Items: items}
	if len(items) > limit {
		result.Items = items[:limit]
		last := result.Items[limit-1]
		result.NextCursor = fmt.Sprintf("%s:%d", last.Kind, last.ID)
	}
	return result, nil
}
