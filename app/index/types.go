package index

import "strings"

const (
	ResourceMetric    = "metric"
	ResourceDimension = "dimension"

	defaultDocsSQLDialect = "vertica"
	defaultApplyWarehouse = "starrocks"

	defaultMetricsMacroPrefix    = "macros/metrics/"
	defaultDimensionsMacroPrefix = "macros/dimensions/"
	defaultMetricsExposure       = "analytics_metrics_core"
	defaultDimensionsExposure    = "analytics_dimensions"

	methodologyCapBytes = 64 * 1024
	descHitCap          = 500
)

// BuildOpts controls envelope dialect/warehouse and catalog path conventions.
type BuildOpts struct {
	DocsSQLDialect        string
	ApplyWarehouse        string
	MetricsMacroPrefix    string
	DimensionsMacroPrefix string
	MetricsExposure       string
	DimensionsExposure    string
}

func (o BuildOpts) withDefaults() BuildOpts {
	if strings.TrimSpace(o.DocsSQLDialect) == "" {
		o.DocsSQLDialect = defaultDocsSQLDialect
	}
	if strings.TrimSpace(o.ApplyWarehouse) == "" {
		o.ApplyWarehouse = defaultApplyWarehouse
	}
	if strings.TrimSpace(o.MetricsMacroPrefix) == "" {
		o.MetricsMacroPrefix = defaultMetricsMacroPrefix
	}
	if strings.TrimSpace(o.DimensionsMacroPrefix) == "" {
		o.DimensionsMacroPrefix = defaultDimensionsMacroPrefix
	}
	if strings.TrimSpace(o.MetricsExposure) == "" {
		o.MetricsExposure = defaultMetricsExposure
	}
	if strings.TrimSpace(o.DimensionsExposure) == "" {
		o.DimensionsExposure = defaultDimensionsExposure
	}
	return o
}

func (o BuildOpts) caveat() string {
	return "docs SQL is " + o.DocsSQLDialect + " dialect; apply on " + o.ApplyWarehouse +
		"; example_sql.executable=false"
}

// ExampleSQL is docs SQL marked non-executable on the apply warehouse.
type ExampleSQL struct {
	Dialect    string `json:"dialect"`
	SQL        string `json:"sql,omitempty"`
	Executable bool   `json:"executable"`
}

// MetricCard is the internal methodology card for a metric or dimension
// (not a dbt unique_id passthrough). ResourceType distinguishes the two.
type MetricCard struct {
	ResourceType string
	ID           string
	UniqueID     string
	Name         string
	Aliases      []string
	ApplyColumn  string
	Category     string
	Methodology  string
	ExampleSQL   ExampleSQL
	Caveat       string
	Status       string
	Path         string
	Short        string // search hit blurb without full methodology
}

// Meta is loaded-manifest envelope fields.
type Meta struct {
	Product             string
	Checksum            string
	ManifestGeneratedAt string
	ETag                string
	DocsSQLDialect      string
	ApplyWarehouse      string
}

// Index is an immutable snapshot of metrics + dimensions for one product instance.
type Index struct {
	Meta       Meta
	ByID       map[string]*MetricCard // metric id → card
	ByAlias    map[string]string      // normalized metric alias → id
	Cards      []*MetricCard          // metrics only (list_metrics)
	DimByID    map[string]*MetricCard // dimension id → card
	DimByAlias map[string]string      // normalized dimension alias → id
	DimCards   []*MetricCard
	SearchDocs []SearchDoc
}

// SearchDoc is a compact searchable document for one metric or dimension.
type SearchDoc struct {
	ResourceType string
	ID           string
	UniqueID     string
	Name         string
	Aliases      []string
	Category     string
	ApplyColumn  string
	Short        string
	SearchText   string // lowercased corpus for substring match
}
