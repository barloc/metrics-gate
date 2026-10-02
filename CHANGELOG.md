# Changelog

## [1.0.0] — 2026-10-02

First public release.

### Added

- Read-only metrics methodology gate over a standard dbt `manifest.json` (HTTP file or URL)
- HTTP API on `:8080`: `POST /v1/search`, `/v1/get_metric`, `/v1/get_dimension`, `/v1/list_metrics`
- Streamable HTTP MCP on `/mcp` with the same four tools
- System endpoints on `:8090`: `/health`, `/metrics` (index freshness)
- Macro-docs metrics (`macros/metrics/`) and semantic MetricFlow metrics (exposure allow-list)
- Dimensions from exposure markdown tables; optional `macros/dimensions/` enrichment
- Overlay YAML for aliases + `apply_column`
- Configurable `catalog` prefixes and exposure names
- Local workflow: `make test`, `make run`, `make smoke`, `docker compose`
- Example Cursor skill: `examples/skills/metrics-gate/SKILL.md`
- Multi-stage Docker image (`barloc/metrics-gate`), Apache-2.0
