package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testOverlay(t *testing.T) Overlay {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "overlay.mini.yaml"))
	require.NoError(t, err)
	ov, err := ParseOverlay(body)
	require.NoError(t, err)
	return ov
}

func loadMini(t *testing.T) *Index {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "manifest.mini.json"))
	require.NoError(t, err)
	idx, err := Build(body, "example", "sha1mini", "sha1mini", testOverlay(t), BuildOpts{})
	require.NoError(t, err)
	return idx
}

func TestUnitBuildMetricsFromMacros(t *testing.T) {
	idx := loadMini(t)
	require.Contains(t, idx.ByID, "gp")
	require.Contains(t, idx.ByID, "gmp")
	require.Contains(t, idx.ByID, "spend")
	require.Contains(t, idx.ByID, "turnover_fx")
	require.NotContains(t, idx.ByID, "orphan_metric", "exposure core set must drop macros outside metrics exposure allow-list")

	gp := idx.ByID["gp"]
	require.Equal(t, "GP", gp.Name)
	require.Equal(t, "GMP", gp.Category)
	require.Contains(t, gp.Methodology, "Валовая прибыль")
	require.Equal(t, "vertica", gp.ExampleSQL.Dialect)
	require.False(t, gp.ExampleSQL.Executable)
	require.Equal(t, "gp", gp.ApplyColumn)
}

func TestUnitBuildDimensionsFromExposure(t *testing.T) {
	idx := loadMini(t)
	require.Contains(t, idx.DimByID, "channel")
	require.Contains(t, idx.DimByID, "nrfm_group")
	require.Contains(t, idx.DimByID, "country")
	require.NotContains(t, idx.DimByID, "get_helper_app_ids")
	require.NotContains(t, idx.ByID, "channel", "dimensions must not land in metric ByID")

	ch := idx.DimByID["channel"]
	require.Equal(t, ResourceDimension, ch.ResourceType)
	require.Equal(t, "channel", ch.ID)
	require.Contains(t, ch.Short, "Канал регистрации")
	require.Equal(t, "vertica", ch.ExampleSQL.Dialect)
	require.False(t, ch.ExampleSQL.Executable)

	nrfm := idx.DimByID["nrfm_group"]
	require.Equal(t, "nrfm_group", nrfm.Name, "name must stay catalog id, not ### from long docs")
	require.Contains(t, nrfm.Methodology, "nRFM-сегмент")
	require.Contains(t, nrfm.Methodology, "активной и неактивной")
	require.NotEmpty(t, nrfm.ExampleSQL.SQL)
	require.Equal(t, "macros/dimensions/get_dim_nrfm_group.sql", nrfm.Path)
}

func TestUnitCoreExposureFallbackAllMacros(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "manifest.mini.json"))
	require.NoError(t, err)
	var top map[string]any
	require.NoError(t, json.Unmarshal(body, &top))
	delete(top, "exposures")
	stripped, err := json.Marshal(top)
	require.NoError(t, err)

	idx, err := Build(stripped, "example", "sha1", "sha1", Overlay{}, BuildOpts{})
	require.NoError(t, err)
	require.Contains(t, idx.ByID, "orphan_metric")
	require.Contains(t, idx.ByID, "gp")
}

func TestUnitBuildLowercaseMetricsPath(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "manifest.mini.json"))
	require.NoError(t, err)
	// Case-insensitive prefix: Macros/Metrics must still match macros/metrics/
	cased := strings.ReplaceAll(string(body), "macros/metrics/", "macros/Metrics/")
	cased = strings.ReplaceAll(cased, "macros/dimensions/", "macros/Dimensions/")
	idx, err := Build([]byte(cased), "example", "sha1", "sha1", testOverlay(t), BuildOpts{})
	require.NoError(t, err)
	require.Contains(t, idx.ByID, "gp")
	require.Equal(t, "GMP", idx.ByID["gp"].Category)
	require.Contains(t, idx.DimByID, "nrfm_group")
}

func TestUnitBuildSemanticMetrics(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "manifest.semantic.mini.json"))
	require.NoError(t, err)
	idx, err := Build(body, "demo-semantic", "sha1semantic", "sha1semantic", Overlay{}, BuildOpts{
		DocsSQLDialect: "spark",
		ApplyWarehouse: "starrocks",
	})
	require.NoError(t, err)
	require.Contains(t, idx.ByID, "volume")
	require.Contains(t, idx.ByID, "pnl")
	require.NotContains(t, idx.ByID, "orphan_metric")
	require.Equal(t, "spark", idx.Meta.DocsSQLDialect)
	require.Equal(t, "starrocks", idx.Meta.ApplyWarehouse)

	vol := idx.ByID["volume"]
	require.Equal(t, "Volume", vol.Name)
	require.Equal(t, "spark", vol.ExampleSQL.Dialect)
	require.False(t, vol.ExampleSQL.Executable)
	require.Contains(t, vol.Caveat, "apply on starrocks")
	require.NotContains(t, vol.Caveat, "via apply_column")
	require.Contains(t, vol.Methodology, "объём")
	require.Contains(t, vol.Methodology, "amount × leverage")
	require.NotEmpty(t, vol.ExampleSQL.SQL)

	require.Contains(t, idx.DimByID, "dt")
	require.Contains(t, idx.DimByID, "deal_type")
	dt := idx.DimByID["dt"]
	require.Contains(t, dt.Short, "Дата строки")
	require.NotEqual(t, "—", dt.Short)
	deal := idx.DimByID["deal_type"]
	require.Contains(t, deal.Short, "продукте")
}

func TestUnitDimTableLayoutFromHeader(t *testing.T) {
	require.Equal(t, dimLayoutValuesDesc, detectDimTableLayout("| разрез | значения | описание | источник |\n|---|---|---|---|\n"))
	require.Equal(t, dimLayoutShortDesc, detectDimTableLayout("| dimension | description | detailed description |\n|---|---|---|\n"))

	// Short-desc layout with backticks must stay col2 (not flipped to col3).
	shortRows := parseDimensionTableRows(`| dimension | description | detailed |
| --- | --- | --- |
|**channel**|Канал с ` + "`web`" + `|link without macro|
`)
	require.Len(t, shortRows, 1)
	require.Contains(t, shortRows[0].Short, "Канал")
	require.Contains(t, shortRows[0].Short, "web")

	valuesRows := parseDimensionTableRows(`| разрез | значения | описание | источник |
| --- | --- | --- | --- |
|**dt**|—|Дата строки витрины.|src|
`)
	require.Len(t, valuesRows, 1)
	require.Contains(t, valuesRows[0].Short, "Дата строки")
}
