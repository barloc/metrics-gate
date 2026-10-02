package gate

import (
	"context"
	"errors"
	"log/slog"

	"github.com/barloc/metrics-gate/app/index"
)

const apiVersion = 1

type (
	// Service exposes metrics tools over the in-memory index.
	Service struct {
		store *index.Store
		log   *slog.Logger
	}

	// Envelope is the public JSON response shape.
	Envelope struct {
		OK                  bool    `json:"ok"`
		APIVersion          int     `json:"api_version"`
		Product             string  `json:"product,omitempty"`
		Checksum            string  `json:"checksum,omitempty"`
		ManifestGeneratedAt string  `json:"manifest_generated_at,omitempty"`
		DocsSQLDialect      string  `json:"docs_sql_dialect,omitempty"`
		ApplyWarehouse      string  `json:"apply_warehouse,omitempty"`
		Data                any     `json:"data"`
		Paging              *Paging `json:"paging"`
		Error               *Error  `json:"error"`
	}

	// Paging signals truncated lists (list_metrics / search caps).
	Paging struct {
		HasMore    bool `json:"has_more"`
		NextOffset int  `json:"next_offset,omitempty"`
	}

	// Error is a structured API error.
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message,omitempty"`
	}

	SearchRequest struct {
		Query        string `json:"query"`
		Category     string `json:"category"`
		ResourceType string `json:"resource_type"`
		Limit        int    `json:"limit"`
	}

	GetMetricRequest struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Alias string `json:"alias"`
	}

	GetDimensionRequest struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Alias string `json:"alias"`
	}

	ListMetricsRequest struct {
		Category string `json:"category"`
		Limit    int    `json:"limit"`
		Offset   int    `json:"offset"`
	}
)

// NewService constructs the gate Service.
func NewService(store *index.Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store: store,
		log:   log.With(slog.String("component", "gate")),
	}
}

func (s *Service) withMeta(env Envelope) Envelope {
	idx := s.store.Get()
	env.APIVersion = apiVersion
	if idx != nil {
		env.Product = idx.Meta.Product
		env.Checksum = idx.Meta.Checksum
		env.ManifestGeneratedAt = idx.Meta.ManifestGeneratedAt
		env.DocsSQLDialect = idx.Meta.DocsSQLDialect
		env.ApplyWarehouse = idx.Meta.ApplyWarehouse
	} else {
		env.Product = s.store.Product()
		env.DocsSQLDialect = "vertica"
		env.ApplyWarehouse = "starrocks"
	}
	return env
}

func (s *Service) unavailable() Envelope {
	s.log.Info("index unavailable")
	return s.withMeta(Envelope{
		OK:    false,
		Error: &Error{Code: "index_unavailable", Message: "manifest index not loaded yet"},
	})
}

func (s *Service) ok(data any) Envelope {
	return s.withMeta(Envelope{OK: true, Data: data, Paging: &Paging{HasMore: false}})
}

func (s *Service) okPaging(data any, hasMore bool, nextOffset int) Envelope {
	p := &Paging{HasMore: hasMore}
	if hasMore {
		p.NextOffset = nextOffset
	}
	return s.withMeta(Envelope{OK: true, Data: data, Paging: p})
}

func (s *Service) fail(code, msg string) Envelope {
	return s.withMeta(Envelope{OK: false, Error: &Error{Code: code, Message: msg}})
}

// Search handles POST /v1/search.
func (s *Service) Search(_ context.Context, req SearchRequest) Envelope {
	idx := s.store.Get()
	if idx == nil {
		return s.unavailable()
	}
	limit := req.Limit
	if limit <= 0 {
		limit = s.store.MaxHits()
	}
	hits, hasMore, err := idx.Search(index.SearchRequest{
		Query:        req.Query,
		Category:     req.Category,
		ResourceType: req.ResourceType,
		Limit:        limit,
	})
	if err != nil {
		if errors.Is(err, index.ErrInvalidQuery) || errors.Is(err, index.ErrInvalidResourceType) {
			return s.fail("invalid_argument", err.Error())
		}
		return s.fail("internal", err.Error())
	}
	return s.okPaging(map[string]any{"hits": hits, "count": len(hits)}, hasMore, 0)
}

// GetMetric handles POST /v1/get_metric.
func (s *Service) GetMetric(_ context.Context, req GetMetricRequest) Envelope {
	idx := s.store.Get()
	if idx == nil {
		return s.unavailable()
	}
	key := firstNonEmpty(req.ID, req.Name, req.Alias)
	if key == "" {
		return s.fail("invalid_argument", "id, name, or alias is required")
	}
	card, err := idx.Resolve(key)
	if err != nil {
		if errors.Is(err, index.ErrNotFound) {
			return s.fail("not_found", "metric not found")
		}
		return s.fail("internal", err.Error())
	}
	return s.ok(metricCardData(card))
}

// GetDimension handles POST /v1/get_dimension.
func (s *Service) GetDimension(_ context.Context, req GetDimensionRequest) Envelope {
	idx := s.store.Get()
	if idx == nil {
		return s.unavailable()
	}
	key := firstNonEmpty(req.ID, req.Name, req.Alias)
	if key == "" {
		return s.fail("invalid_argument", "id, name, or alias is required")
	}
	card, err := idx.ResolveDimension(key)
	if err != nil {
		if errors.Is(err, index.ErrNotFound) {
			return s.fail("not_found", "dimension not found")
		}
		return s.fail("internal", err.Error())
	}
	return s.ok(metricCardData(card))
}

// ListMetrics handles POST /v1/list_metrics.
func (s *Service) ListMetrics(_ context.Context, req ListMetricsRequest) Envelope {
	idx := s.store.Get()
	if idx == nil {
		return s.unavailable()
	}
	limit := req.Limit
	if limit <= 0 {
		limit = s.store.MaxHits()
	}
	hits, hasMore := idx.ListMetrics(req.Category, limit, req.Offset)
	next := req.Offset + len(hits)
	return s.okPaging(map[string]any{"hits": hits, "count": len(hits)}, hasMore, next)
}

func metricCardData(card *index.MetricCard) map[string]any {
	rt := card.ResourceType
	if rt == "" {
		rt = index.ResourceMetric
	}
	return map[string]any{
		"id":            card.ID,
		"unique_id":     card.UniqueID,
		"name":          card.Name,
		"aliases":       card.Aliases,
		"apply_column":  card.ApplyColumn,
		"category":      card.Category,
		"methodology":   card.Methodology,
		"example_sql":   card.ExampleSQL,
		"caveat":        card.Caveat,
		"status":        card.Status,
		"path":          card.Path,
		"resource_type": rt,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
