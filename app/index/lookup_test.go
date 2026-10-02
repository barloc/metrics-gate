package index

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnitAliasToFxResolvesApplyColumn(t *testing.T) {
	idx := loadMini(t)
	card, err := idx.Resolve("to_fx")
	require.NoError(t, err)
	require.Equal(t, "turnover_fx", card.ID)
	require.Equal(t, "fx_to", card.ApplyColumn)
}

func TestUnitUnknownAliasNotNeighbor(t *testing.T) {
	idx := loadMini(t)
	_, err := idx.Resolve("wd_pending_not_a_metric")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = idx.Resolve("no_such_metric_xyz")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestUnitResolveDimensionChannel(t *testing.T) {
	idx := loadMini(t)
	card, err := idx.ResolveDimension("channel")
	require.NoError(t, err)
	require.Equal(t, "channel", card.ID)
	require.Equal(t, ResourceDimension, card.ResourceType)
	require.Contains(t, card.Short, "Канал регистрации")

	_, err = idx.Resolve("channel")
	require.ErrorIs(t, err, ErrNotFound, "get_metric must not resolve dimensions")

	_, err = idx.ResolveDimension("no_such_dimension_xyz")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = idx.ResolveDimension("gp")
	require.ErrorIs(t, err, ErrNotFound, "get_dimension must not resolve metrics")
}

func TestUnitListMetricsPaging(t *testing.T) {
	idx := loadMini(t)
	hits, hasMore := idx.ListMetrics("", 2, 0)
	require.Len(t, hits, 2)
	require.True(t, hasMore)

	page2, hasMore := idx.ListMetrics("", 2, 2)
	require.Len(t, page2, 2)
	require.False(t, hasMore)
	require.NotEqual(t, hits[0].ID, page2[0].ID)

	all, hasMore := idx.ListMetrics("", 20, 0)
	require.Len(t, all, 4)
	require.False(t, hasMore)
	for _, h := range all {
		require.Equal(t, ResourceMetric, h.ResourceType)
	}
}
