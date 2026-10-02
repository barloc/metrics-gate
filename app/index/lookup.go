package index

import (
	"errors"
	"strings"
)

var (
	// ErrNotFound is returned when id/alias does not resolve to a metric or dimension.
	ErrNotFound = errors.New("not found")
)

// Resolve looks up a metric by id, name, or alias. Unknown → ErrNotFound (never a neighbor).
func (idx *Index) Resolve(raw string) (*MetricCard, error) {
	return resolveIn(idx.ByAlias, idx.ByID, raw)
}

// ResolveDimension looks up a dimension by id, name, or alias.
// Unknown → ErrNotFound (never a neighbor metric or dimension).
func (idx *Index) ResolveDimension(raw string) (*MetricCard, error) {
	return resolveIn(idx.DimByAlias, idx.DimByID, raw)
}

func resolveIn(byAlias map[string]string, byID map[string]*MetricCard, raw string) (*MetricCard, error) {
	key := normalizeKey(raw)
	if key == "" {
		return nil, ErrNotFound
	}
	if id, ok := byAlias[key]; ok {
		if card, ok := byID[id]; ok {
			return card, nil
		}
	}
	if card, ok := byID[raw]; ok {
		return card, nil
	}
	if card, ok := byID[strings.ToLower(raw)]; ok {
		return card, nil
	}
	return nil, ErrNotFound
}

// ListMetrics returns short hits, optionally filtered by category, with offset pagination.
// hasMore is true when more cards exist beyond offset+limit (do not claim "all").
func (idx *Index) ListMetrics(category string, limit, offset int) (hits []SearchHit, hasMore bool) {
	limit = clampLimit(limit)
	if offset < 0 {
		offset = 0
	}
	wantCat := strings.TrimSpace(category)
	matched := make([]*MetricCard, 0, len(idx.Cards))
	for _, card := range idx.Cards {
		if wantCat != "" && !strings.EqualFold(card.Category, wantCat) {
			continue
		}
		matched = append(matched, card)
	}
	if offset >= len(matched) {
		return nil, false
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	page := matched[offset:end]
	out := make([]SearchHit, 0, len(page))
	for _, card := range page {
		out = append(out, SearchHit{
			ResourceType: ResourceMetric,
			ID:           card.ID,
			UniqueID:     card.UniqueID,
			Name:         card.Name,
			Aliases:      append([]string(nil), card.Aliases...),
			Category:     card.Category,
			ApplyColumn:  card.ApplyColumn,
			Description:  card.Short,
		})
	}
	return out, end < len(matched)
}
