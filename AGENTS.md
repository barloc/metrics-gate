# Agent notes (metrics-gate)

Read-only metrics methodology gate: indexes a dbt `manifest.json`, exposes HTTP `/v1` + MCP `/mcp`.

## Run locally

```bash
make test
make up          # or: make run
make smoke
```

Ports: API/MCP `:8080`, health/metrics `:8090`.

MCP: `http://127.0.0.1:8080/mcp` — tools `search`, `get_metric`, `get_dimension`, `list_metrics`.

## Config

- `index.file` or `index.url` (raw `manifest.json` required)
- `docs_sql_dialect` / `apply_warehouse` (envelope + caveat)
- `overlay.file` for aliases / `apply_column`
- optional `catalog.*` for macro path prefixes and exposure names

Authoring metrics/dimensions in dbt: see [README](README.md#authoring-metrics--dimensions-in-dbt).

Example skill for Cursor: [`examples/skills/metrics-gate/SKILL.md`](examples/skills/metrics-gate/SKILL.md).

## Do not

- Execute `example_sql` against the warehouse (`executable=false`)
- Invent neighboring metrics when search misses
- Treat this service as a SQL runner
- Expect `catalog.json`, git Jinja, or MetricFlow runtime inside the gate
