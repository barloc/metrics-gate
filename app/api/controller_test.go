package api_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/barloc/metrics-gate/app/api"
	"github.com/barloc/metrics-gate/app/gate"
	"github.com/barloc/metrics-gate/app/index"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "index", "testdata", "manifest.mini.json"))
	require.NoError(t, err)
	ovBody, err := os.ReadFile(filepath.Join("..", "index", "testdata", "overlay.mini.yaml"))
	require.NoError(t, err)
	ov, err := index.ParseOverlay(ovBody)
	require.NoError(t, err)

	store := index.NewTestStore("example", ov)
	require.NoError(t, store.LoadBytes(body, "sha1mini"))
	svc := gate.NewService(store, slog.Default())
	return api.NewController(svc, nil, slog.Default()).HandlerForTest()
}

func postJSON(t *testing.T, h http.Handler, path string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestUnitGetMetricGP(t *testing.T) {
	h := testHandler(t)
	rr := postJSON(t, h, "/v1/get_metric", map[string]any{"id": "gp"})

	require.Equal(t, http.StatusOK, rr.Code)
	var env gate.Envelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	require.True(t, env.OK)
	require.Equal(t, "example", env.Product)
	require.Equal(t, "sha1mini", env.Checksum)
	require.Equal(t, "vertica", env.DocsSQLDialect)
	require.Equal(t, "starrocks", env.ApplyWarehouse)
	data, ok := env.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "gp", data["id"])
	assert.Equal(t, "gp", data["apply_column"])
	method, ok := data["methodology"].(string)
	require.True(t, ok)
	assert.Contains(t, method, "Валовая прибыль")
	ex, ok := data["example_sql"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, ex["executable"])
}

func TestUnitSearchToFx(t *testing.T) {
	h := testHandler(t)
	rr := postJSON(t, h, "/v1/search", map[string]any{"query": "to_fx"})

	require.Equal(t, http.StatusOK, rr.Code)
	var env gate.Envelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	require.True(t, env.OK)
	require.NotNil(t, env.Paging)
	assert.False(t, env.Paging.HasMore)
	data := env.Data.(map[string]any)
	hits := data["hits"].([]any)
	require.NotEmpty(t, hits)
	hit := hits[0].(map[string]any)
	assert.Equal(t, "turnover_fx", hit["id"])
	assert.Equal(t, "fx_to", hit["apply_column"])
}

func TestUnitGetMetricMiss(t *testing.T) {
	h := testHandler(t)
	rr := postJSON(t, h, "/v1/get_metric", map[string]any{"id": "no_such_metric_xyz"})

	require.Equal(t, http.StatusNotFound, rr.Code)
	var env gate.Envelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	require.False(t, env.OK)
	require.Equal(t, "not_found", env.Error.Code)
}

func TestUnitGetDimensionChannel(t *testing.T) {
	h := testHandler(t)
	rr := postJSON(t, h, "/v1/get_dimension", map[string]any{"id": "channel"})

	require.Equal(t, http.StatusOK, rr.Code)
	var env gate.Envelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	require.True(t, env.OK)
	require.Equal(t, "vertica", env.DocsSQLDialect)
	require.Equal(t, "starrocks", env.ApplyWarehouse)
	data, ok := env.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "channel", data["id"])
	assert.Equal(t, "dimension", data["resource_type"])
	short, _ := data["methodology"].(string)
	assert.Contains(t, short, "Канал регистрации")
}

func TestUnitListMetricsHasMore(t *testing.T) {
	h := testHandler(t)
	rr := postJSON(t, h, "/v1/list_metrics", map[string]any{"limit": 2})

	require.Equal(t, http.StatusOK, rr.Code)
	var env gate.Envelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	require.True(t, env.OK)
	require.NotNil(t, env.Paging)
	assert.True(t, env.Paging.HasMore)
	assert.Equal(t, 2, env.Paging.NextOffset)

	rr2 := postJSON(t, h, "/v1/list_metrics", map[string]any{"limit": 2, "offset": env.Paging.NextOffset})
	require.Equal(t, http.StatusOK, rr2.Code)
	var env2 gate.Envelope
	require.NoError(t, json.Unmarshal(rr2.Body.Bytes(), &env2))
	require.True(t, env2.OK)
	assert.False(t, env2.Paging.HasMore)
}
