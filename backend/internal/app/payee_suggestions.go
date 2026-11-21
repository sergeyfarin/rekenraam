package app

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"rekenraam/backend/internal/db"
)

const payeeSuggestionLimit = 5

type PayeeSuggestion struct {
	ID    int64
	Name  string
	Exact bool
}

func (s *PayeeService) PayeeSuggestions(ctx context.Context, query string) ([]PayeeSuggestion, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > payeeNameMaxBytes {
		return nil, ValidationError{Message: "payee suggestion query must contain 1 to 200 bytes"}
	}
	ctx, err := s.settings.referenceContext(ctx, s.now())
	if err != nil {
		return nil, err
	}
	names, err := s.repository.ListAllActivePayeeNames(ctx, BookID)
	if err != nil {
		return nil, err
	}

	normalized := normalizePayeeNameForMatch(query)
	terms := payeeNameTerms(normalized)
	best := make([]rankedPayee, 0, payeeSuggestionLimit+1)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		match := rankedPayee{name: name, exact: name.NormalizedName == normalized}
		if !match.exact {
			match.score = payeeSuggestionScore(terms, payeeNameTerms(name.NormalizedName))
			if match.score == 0 {
				continue
			}
		}
		best = append(best, match)
		slices.SortFunc(best, compareRankedPayees)
		if len(best) > payeeSuggestionLimit {
			best = best[:payeeSuggestionLimit]
		}
	}

	result := make([]PayeeSuggestion, 0, len(best))
	for _, match := range best {
		result = append(result, PayeeSuggestion{ID: match.name.ID, Name: match.name.Name, Exact: match.exact})
	}
	return result, nil
}

type rankedPayee struct {
	name  db.PayeeName
	exact bool
	score int
}

func compareRankedPayees(a, b rankedPayee) int {
	if a.exact != b.exact {
		if a.exact {
			return -1
		}
		return 1
	}
	return cmp.Or(
		cmp.Compare(b.score, a.score),
		cmp.Compare(a.name.NormalizedName, b.name.NormalizedName),
		cmp.Compare(a.name.ID, b.name.ID),
	)
}

func payeeNameTerms(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
}

func payeeSuggestionScore(query, name []string) int {
	score := 0
	for _, term := range query {
		best := 0
		for _, word := range name {
			switch {
			case word == term:
				best = 3
			case strings.HasPrefix(word, term):
				best = max(best, 2)
			case best == 0 && payeeTermsNear(term, word):
				best = 1
			}
		}
		score += best
	}
	return score
}

func payeeTermsNear(term, word string) bool {
	// Retain the editor's tolerance of about 30% edits per typed term.
	length := utf8.RuneCountInString(term)
	limit := (3*length + 5) / 10
	wordLength := utf8.RuneCountInString(word)
	if limit == 0 || wordLength < length-limit || wordLength > length+limit {
		return false
	}

	// Levenshtein distance with one row, bounded by the request's term length.
	typed := []rune(term)
	row := make([]int, len(typed)+1)
	for i := range row {
		row[i] = i
	}
	for _, letter := range word {
		diagonal := row[0]
		row[0]++
		for i, target := range typed {
			above := row[i+1]
			cost := 0
			if letter != target {
				cost = 1
			}
			row[i+1] = min(row[i]+1, above+1, diagonal+cost)
			diagonal = above
		}
	}
	return row[len(typed)] <= limit
}
