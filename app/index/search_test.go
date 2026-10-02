package index

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnitSearchToFx(t *testing.T) {
	idx := loadMini(t)
	hits, hasMore, err := idx.Search(SearchRequest{Query: "to_fx", Limit: 5})
	require.NoError(t, err)
	require.False(t, hasMore)
	require.NotEmpty(t, hits)
	require.Equal(t, "turnover_fx", hits[0].ID)
	require.Equal(t, "fx_to", hits[0].ApplyColumn)
	require.Equal(t, ResourceMetric, hits[0].ResourceType)
	require.NotContains(t, hits[0].Description, "```sql") // short hit, not full methodology
}

func TestUnitSearchHasMore(t *testing.T) {
	idx := loadMini(t)
	// Shared caveat word matches every card → truncation must set has_more.
	hits, hasMore, err := idx.Search(SearchRequest{Query: "vertica", Limit: 1})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.True(t, hasMore)
}

func TestUnitSearchResourceTypeIsolation(t *testing.T) {
	idx := loadMini(t)

	dimHits, _, err := idx.Search(SearchRequest{Query: "channel", ResourceType: ResourceDimension, Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, dimHits)
	for _, h := range dimHits {
		require.Equal(t, ResourceDimension, h.ResourceType)
		require.NotEqual(t, "gp", h.ID)
		require.NotEqual(t, "turnover_fx", h.ID)
	}
	require.Equal(t, "channel", dimHits[0].ID)

	metricHits, _, err := idx.Search(SearchRequest{Query: "channel", ResourceType: ResourceMetric, Limit: 10})
	require.NoError(t, err)
	for _, h := range metricHits {
		require.Equal(t, ResourceMetric, h.ResourceType)
		require.NotEqual(t, "channel", h.ID)
	}

	// Metric-only filter must not return dimensions even when query matches a dim id.
	gpAsDim, _, err := idx.Search(SearchRequest{Query: "gp", ResourceType: ResourceDimension, Limit: 10})
	require.NoError(t, err)
	for _, h := range gpAsDim {
		require.NotEqual(t, "gp", h.ID)
	}
}
