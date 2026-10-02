package index

import (
	"sort"
	"strings"
	"unicode"
)

const maxHitsCap = 20 // ponytail: hard API cap; raise with search.max_hits + clients when catalog grows

// SearchRequest is the search tool input.
type SearchRequest struct {
	Query        string
	Category     string
	ResourceType string // "", "metric", or "dimension"
	Limit        int
}

// SearchHit is one ranked result (no full methodology).
type SearchHit struct {
	ResourceType string   `json:"resource_type"`
	ID           string   `json:"id"`
	UniqueID     string   `json:"unique_id,omitempty"`
	Name         string   `json:"name,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
	Category     string   `json:"category,omitempty"`
	ApplyColumn  string   `json:"apply_column,omitempty"`
	Description  string   `json:"description,omitempty"`
	Score        int      `json:"-"`
}

// Search runs lexical AND substring search with ranking over MetricCards.
// hasMore is true when more matches exist beyond limit (do not claim "all").
func (idx *Index) Search(req SearchRequest) (hits []SearchHit, hasMore bool, err error) {
	q := strings.TrimSpace(req.Query)
	if len([]rune(q)) < 2 {
		return nil, false, ErrInvalidQuery
	}
	wantRT := strings.TrimSpace(strings.ToLower(req.ResourceType))
	if wantRT != "" && wantRT != ResourceMetric && wantRT != ResourceDimension {
		return nil, false, ErrInvalidResourceType
	}
	limit := clampLimit(req.Limit)
	tokens := Tokenize(q)
	if len(tokens) == 0 {
		return nil, false, ErrInvalidQuery
	}

	wantCat := strings.TrimSpace(req.Category)
	hits = make([]SearchHit, 0, 32)

	for _, doc := range idx.SearchDocs {
		if wantRT != "" && doc.ResourceType != wantRT {
			continue
		}
		if wantCat != "" && !strings.EqualFold(doc.Category, wantCat) {
			continue
		}
		hit, ok := scoreMetricDoc(doc, tokens)
		if !ok {
			continue
		}
		hits = append(hits, hit)
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if len(hits[i].ID) != len(hits[j].ID) {
			return len(hits[i].ID) < len(hits[j].ID)
		}
		return hits[i].ID < hits[j].ID
	})

	if len(hits) > limit {
		return hits[:limit], true, nil
	}
	return hits, false, nil
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return maxHitsCap
	}
	if limit > maxHitsCap {
		return maxHitsCap
	}
	return limit
}

func scoreMetricDoc(doc SearchDoc, tokens []string) (SearchHit, bool) {
	score := 0
	idL := strings.ToLower(doc.ID)
	nameL := strings.ToLower(doc.Name)
	colL := strings.ToLower(doc.ApplyColumn)
	aliasesL := make([]string, len(doc.Aliases))
	for i, a := range doc.Aliases {
		aliasesL[i] = strings.ToLower(a)
	}

	for _, tok := range tokens {
		best := 0
		tokHit := false
		if idL == tok {
			best, tokHit = 100, true
		} else if strings.Contains(idL, tok) {
			best, tokHit = 40, true
		}
		if nameL == tok {
			best, tokHit = max(best, 90), true
		} else if strings.Contains(nameL, tok) {
			best, tokHit = max(best, 35), true
		}
		for _, a := range aliasesL {
			if a == tok {
				best, tokHit = max(best, 95), true
			} else if strings.Contains(a, tok) {
				best, tokHit = max(best, 50), true
			}
		}
		if colL == tok {
			best, tokHit = max(best, 80), true
		}
		if strings.Contains(doc.SearchText, tok) {
			best, tokHit = max(best, 12), true
		}
		if !tokHit {
			return SearchHit{}, false
		}
		score += best
	}

	return SearchHit{
		ResourceType: doc.ResourceType,
		ID:           doc.ID,
		UniqueID:     doc.UniqueID,
		Name:         doc.Name,
		Aliases:      append([]string(nil), doc.Aliases...),
		Category:     doc.Category,
		ApplyColumn:  doc.ApplyColumn,
		Description:  doc.Short,
		Score:        score,
	}, true
}

// Tokenize splits query into lowercase whitespace tokens.
func Tokenize(q string) []string {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	var tokens []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, b.String())
		b.Reset()
	}
	for _, r := range q {
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		b.WriteRune(r)
	}
	flush()
	return tokens
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ErrInvalidQuery is returned for empty / too-short queries.
var ErrInvalidQuery = errInvalidQuery{}

type errInvalidQuery struct{}

func (errInvalidQuery) Error() string { return "query must be at least 2 characters" }

// ErrInvalidResourceType is returned when resource_type is not metric|dimension|empty.
var ErrInvalidResourceType = errInvalidResourceType{}

type errInvalidResourceType struct{}

func (errInvalidResourceType) Error() string {
	return "resource_type must be metric, dimension, or empty"
}
