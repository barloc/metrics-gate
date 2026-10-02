# metrics-gate

Read-only **metrics methodology gate** for IDE agents: search metrics and dimensions, fetch full methodology cards (including docs SQL marked non-executable), and resolve aliases.

Indexes a standard dbt `manifest.json` — no warehouse client. Supports two authoring styles in the dbt project (macro-docs stubs and Semantic Layer metrics). Use overlay YAML for aliases and warehouse `apply_column`.

License: [Apache-2.0](LICENSE). Current release: **[v1.0.0](CHANGELOG.md#100--2026-10-02)**.

## Features

- HTTP `/v1` tools: `search`, `get_metric`, `get_dimension`, `list_metrics`
- Streamable HTTP MCP on `/mcp` (same four tools)
- Index from local file or any HTTP URL to raw `manifest.json`
- Macro-docs and/or MetricFlow semantic metrics (allow-listed via exposure)
- Dimensions from a markdown table in an exposure; optional enrich macros
- Overlay YAML for aliases + `apply_column`
- Configurable catalog path prefixes and exposure names

Not supported: AuthN beyond optional static identity stub, MetricFlow runtime, dual-write back to dbt, executing docs SQL.

## Quick start

```bash
make test
make run          # uses config.example.yaml + fixture manifest
# in another terminal:
make smoke
```

Or with Docker Compose:

```bash
make up
make smoke
make down
```

Cursor MCP (local):

```json
{
  "mcpServers": {
    "metrics-gate": {
      "url": "http://127.0.0.1:8080/mcp"
    }
  }
}
```

Example Cursor skill: [`examples/skills/metrics-gate/SKILL.md`](examples/skills/metrics-gate/SKILL.md). Short agent notes: [`AGENTS.md`](AGENTS.md).

| Port | Role |
| --- | --- |
| `:8090` | `/health`, `/metrics` |
| `:8080` | API `/v1/*` and MCP `/mcp` |

## Configuration

See [`config.example.yaml`](config.example.yaml).

```yaml
product: example

docs_sql_dialect: vertica      # dialect of example SQL in docs
apply_warehouse: starrocks    # where agents should apply numbers

index:
  file: "path/to/manifest.json"   # or:
  # url: "https://docs.example.com/manifest.json"
  refresh_interval: 5m

overlay:
  file: overlay.example.yaml

catalog:                          # optional; defaults shown
  metrics_macro_prefix: "macros/metrics/"
  dimensions_macro_prefix: "macros/dimensions/"
  metrics_exposure: analytics_metrics_core
  dimensions_exposure: analytics_dimensions

auth:
  mode: disabled                  # or static — see config.example.yaml
```

`index.url` / `index.file`: one is required. The published image ships the example config, overlay, and a tiny fixture so `docker run -p 8080:8080 -p 8090:8090 barloc/metrics-gate:v1.0.0` answers `/v1/get_metric` out of the box.

## Authoring metrics & dimensions in dbt

Gate reads only `manifest.json`. The catalog is built from **named exposures** plus one or both metric sources (macro-docs and semantic). On id clash, **macros win**.

### Shared: registry exposures

| Exposure (default name) | Role |
| --- | --- |
| `analytics_metrics_core` | Allow-list of published metrics |
| `analytics_dimensions` | Dimension catalog = markdown table in `description` |

Without `analytics_metrics_core`, semantic metrics are **not** loaded; macros under `macros/metrics/` load all. Without `analytics_dimensions`, there are no dimensions (dimension macros alone do not create the catalog).

Override names/prefixes via the `catalog` config block if your repo uses different conventions.

### Variant A — macro-docs (stub macros)

For projects without dbt Semantic Layer:

1. Stub macros under `macros/metrics/**` (path case-insensitive; subdirectory → `category`).
2. Macro body may be empty; **methodology = resolved `description`** in the manifest (`{% docs %}` / docs blocks).
3. In description: `### Name`, prose, optional fenced ` ```sql ` (becomes `example_sql` with `executable=false`).
4. Exposure `analytics_metrics_core`:
   - `depends_on.macros: [macro.<pkg>.<id>, …]` **or**
   - links `#!/macro/macro.<pkg>.<id>` in the description
5. Orphan macros outside a non-empty allow-list are not indexed.

```yaml
# exposures.yml
version: 2
exposures:
  - name: analytics_metrics_core
    type: dashboard
    owner: { name: analytics }
    depends_on:
      macros:
        - macro.my_project.revenue
```

```sql
-- macros/metrics/Finance/revenue.sql
{% macro revenue() %}{% endmacro %}
```

### Variant B — semantic metrics (MetricFlow / dbt metrics)

1. Declare dbt `metrics:` in YAML → they appear in manifest top-level `metrics`.
2. Exposure `analytics_metrics_core` **must** list them in `depends_on.nodes: [metric.<pkg>.<id>, …]`.
   Empty `nodes` → semantic metrics are **not** indexed (never “load all metrics”).
3. Card fields: `label` / description; from `meta`: `formula`, `unit`, `pitfalls`, `short_name` (alias).

```yaml
exposures:
  - name: analytics_metrics_core
    type: dashboard
    owner: { name: analytics }
    depends_on:
      nodes:
        - metric.my_project.volume
```

### Dimensions

Catalog = **markdown table rows** in exposure `analytics_dimensions`. Id cell: `**snake_case_id**`.

Two layouts (auto-detected from header):

| Layout | Header cues | Short text |
| --- | --- | --- |
| short-desc (3 col) | `dimension` / `description` | col2 (or link in col3) |
| values-desc (4 col) | `values`+`description` or localized equivalents | col3 (description), col2 = values |

Optional methodology enrichment (does not create ids by itself):

- macros under `macros/dimensions/`
- `#!/macro/...` link in the row **or** macro named `get_dim_<id>` with non-empty description

```markdown
| dimension | description | detailed description |
| --- | --- | --- |
| **channel** | Acquisition channel | [docs](#!/macro/macro.my_project.get_dim_channel) |
```

### Outside dbt: overlay YAML

Aliases (“how users ask”) and warehouse `apply_column` live in the gate overlay file, not in the manifest. Only for metric ids that already exist.

## Docker Hub

```bash
docker login
make docker-push                 # barloc/metrics-gate:latest
make docker-push VERSION=v1.0.0  # also tags VERSION
```

Image: [`barloc/metrics-gate`](https://hub.docker.com/r/barloc/metrics-gate).

## Make targets

| Target | Action |
| --- | --- |
| `make test` | `go test ./...` |
| `make run` | build + run with `config.example.yaml` |
| `make smoke` | curl health + `/v1/get_metric` + `/v1/search` |
| `make up` / `make down` | docker compose |
| `make docker-build` / `make docker-push` | local image publish |
