package index

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

type (
	configFile struct {
		Product        string        `yaml:"product"`
		DocsSQLDialect string        `yaml:"docs_sql_dialect"`
		ApplyWarehouse string        `yaml:"apply_warehouse"`
		Index          IndexConfig   `yaml:"index"`
		Overlay        OverlayConfig `yaml:"overlay"`
		Search         SearchConfig  `yaml:"search"`
		Catalog        CatalogConfig `yaml:"catalog"`
	}

	// IndexConfig is YAML index block.
	IndexConfig struct {
		URL             string `yaml:"url"`
		File            string `yaml:"file"`
		RefreshInterval string `yaml:"refresh_interval"`
	}

	// OverlayConfig is YAML overlay block.
	OverlayConfig struct {
		File string `yaml:"file"`
	}

	// SearchConfig is YAML search block.
	SearchConfig struct {
		MaxHits int `yaml:"max_hits"`
	}

	// CatalogConfig overrides manifest path prefixes and exposure names.
	CatalogConfig struct {
		MetricsMacroPrefix     string `yaml:"metrics_macro_prefix"`
		DimensionsMacroPrefix  string `yaml:"dimensions_macro_prefix"`
		MetricsExposure        string `yaml:"metrics_exposure"`
		DimensionsExposure     string `yaml:"dimensions_exposure"`
	}

	// Config is runtime index settings.
	Config struct {
		Product         string
		DocsSQLDialect  string
		ApplyWarehouse  string
		URL             string
		File            string
		OverlayFile     string
		Overlay         Overlay
		RefreshInterval time.Duration
		MaxHits         int
		Catalog         CatalogConfig
	}

	// Store holds the atomic index pointer and refresh loop.
	Store struct {
		log   *slog.Logger
		cfg   Config
		fetch Fetcher
		cur   atomic.Pointer[Index]
		etag  atomic.Value // string
	}
)

// ParseConfig unmarshals product/index/overlay/search/catalog from YAML.
func ParseConfig(raw []byte) (Config, error) {
	var cf configFile
	if err := yaml.Unmarshal(raw, &cf); err != nil {
		return Config{}, fmt.Errorf("index config: %w", err)
	}
	cfg := Config{
		Product:        cf.Product,
		DocsSQLDialect: cf.DocsSQLDialect,
		ApplyWarehouse: cf.ApplyWarehouse,
		URL:            cf.Index.URL,
		File:           cf.Index.File,
		OverlayFile:    cf.Overlay.File,
		MaxHits:        cf.Search.MaxHits,
		Catalog:        cf.Catalog,
	}
	if cfg.Product == "" {
		cfg.Product = "example"
	}
	if cfg.DocsSQLDialect == "" {
		cfg.DocsSQLDialect = defaultDocsSQLDialect
	}
	if cfg.ApplyWarehouse == "" {
		cfg.ApplyWarehouse = defaultApplyWarehouse
	}
	if cfg.URL == "" && cfg.File == "" {
		return Config{}, fmt.Errorf("index: set index.url or index.file")
	}
	cfg.RefreshInterval = 5 * time.Minute
	if cf.Index.RefreshInterval != "" {
		d, err := time.ParseDuration(cf.Index.RefreshInterval)
		if err != nil {
			return Config{}, fmt.Errorf("index.refresh_interval: %w", err)
		}
		cfg.RefreshInterval = d
	}
	if cfg.MaxHits <= 0 {
		cfg.MaxHits = 20
	}
	cfg.Catalog = cfg.Catalog.withDefaults()
	ov, err := LoadOverlay(cfg.OverlayFile)
	if err != nil {
		return Config{}, err
	}
	cfg.Overlay = ov
	return cfg, nil
}

func (c CatalogConfig) withDefaults() CatalogConfig {
	if strings.TrimSpace(c.MetricsMacroPrefix) == "" {
		c.MetricsMacroPrefix = defaultMetricsMacroPrefix
	}
	if strings.TrimSpace(c.DimensionsMacroPrefix) == "" {
		c.DimensionsMacroPrefix = defaultDimensionsMacroPrefix
	}
	if strings.TrimSpace(c.MetricsExposure) == "" {
		c.MetricsExposure = defaultMetricsExposure
	}
	if strings.TrimSpace(c.DimensionsExposure) == "" {
		c.DimensionsExposure = defaultDimensionsExposure
	}
	return c
}

// NewStore constructs an empty Store (load via Start).
func NewStore(cfg Config, log *slog.Logger) *Store {
	return newStore(cfg, log)
}

// NewTestStore builds a Store for unit tests.
func NewTestStore(product string, overlay Overlay) *Store {
	if product == "" {
		product = "example"
	}
	return newStore(Config{
		Product:         product,
		DocsSQLDialect:  defaultDocsSQLDialect,
		ApplyWarehouse:  defaultApplyWarehouse,
		MaxHits:         20,
		RefreshInterval: time.Minute,
		Overlay:         overlay,
		Catalog:         CatalogConfig{}.withDefaults(),
	}, slog.Default())
}

func newStore(cfg Config, log *slog.Logger) *Store {
	if log == nil {
		log = slog.Default()
	}
	cfg.Catalog = cfg.Catalog.withDefaults()
	s := &Store{
		log: log.With(slog.String("component", "index")),
		cfg: cfg,
		fetch: Fetcher{
			URL:  cfg.URL,
			File: cfg.File,
		},
	}
	s.etag.Store("")
	s.setReady(0)
	return s
}

func (s *Store) buildOpts() BuildOpts {
	return BuildOpts{
		DocsSQLDialect:        s.cfg.DocsSQLDialect,
		ApplyWarehouse:        s.cfg.ApplyWarehouse,
		MetricsMacroPrefix:    s.cfg.Catalog.MetricsMacroPrefix,
		DimensionsMacroPrefix: s.cfg.Catalog.DimensionsMacroPrefix,
		MetricsExposure:       s.cfg.Catalog.MetricsExposure,
		DimensionsExposure:    s.cfg.Catalog.DimensionsExposure,
	}
}

// Get returns the current index or nil if not ready.
func (s *Store) Get() *Index {
	return s.cur.Load()
}

// Product returns configured product.
func (s *Store) Product() string { return s.cfg.Product }

// MaxHits returns configured search cap.
func (s *Store) MaxHits() int { return s.cfg.MaxHits }

// Start kicks off initial load + periodic refresh in the background.
// /v1 returns 503 index_unavailable until the first successful load.
func (s *Store) Start(ctx context.Context) {
	go s.loop(ctx)
}

func (s *Store) loop(ctx context.Context) {
	s.refresh(ctx)
	t := time.NewTicker(s.cfg.RefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refresh(ctx)
		}
	}
}

func (s *Store) refresh(ctx context.Context) {
	prev, ok := s.etag.Load().(string)
	if !ok {
		prev = ""
	}
	res, err := s.fetch.HeadOrGet(prev)
	if err != nil {
		s.log.Error("index refresh failed", slog.String("error", err.Error()))
		s.observeRefreshError()
		return
	}
	if res.NotModified {
		s.log.Debug("index unchanged", slog.String("etag", prev))
		if s.Get() != nil {
			s.observeLoaded()
		}
		return
	}
	idx, err := Build(res.Body, s.cfg.Product, res.Checksum, res.ETag, s.cfg.Overlay, s.buildOpts())
	if err != nil {
		s.log.Error("index build failed", slog.String("error", err.Error()))
		s.observeRefreshError()
		return
	}
	select {
	case <-ctx.Done():
		return
	default:
	}
	s.cur.Store(idx)
	s.etag.Store(res.ETag)
	s.observeIndex(idx)
	s.log.Info("index loaded",
		slog.String("product", idx.Meta.Product),
		slog.String("checksum", idx.Meta.Checksum),
		slog.String("generated_at", idx.Meta.ManifestGeneratedAt),
		slog.Int("metrics", len(idx.ByID)),
		slog.Int("search_docs", len(idx.SearchDocs)),
	)
}

// LoadBytes builds and swaps an index from raw bytes (tests).
func (s *Store) LoadBytes(body []byte, checksum string) error {
	idx, err := Build(body, s.cfg.Product, checksum, checksum, s.cfg.Overlay, s.buildOpts())
	if err != nil {
		return err
	}
	s.cur.Store(idx)
	s.etag.Store(checksum)
	s.observeIndex(idx)
	return nil
}
