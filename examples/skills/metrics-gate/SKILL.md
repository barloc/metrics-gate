---
name: metrics-gate
description: >-
  Metric/dimension methodology via metrics-gate MCP/HTTP: search, get_metric,
  get_dimension, list_metrics. Cite id + checksum; respect docs_sql_dialect /
  apply_warehouse; example_sql is not executable. Point MCP at a running
  metrics-gate instance.
---

# metrics-gate (example Cursor skill)

Use this skill when the user asks how a metric or dimension is defined, aliased,
or applied — from a **metrics-gate** instance (local Docker/binary or a deployed URL).

Upstream project: [github.com/barloc/metrics-gate](https://github.com/barloc/metrics-gate).

How to author metrics/dimensions in the dbt repo (macro-docs vs semantic, exposures, overlay): see the upstream [README authoring section](https://github.com/barloc/metrics-gate/blob/main/README.md#authoring-metrics--dimensions-in-dbt).

## Install (Cursor)

1. Copy or symlink this directory into Cursor skills, e.g.:

```bash
mkdir -p ~/.cursor/skills
ln -sfn /path/to/metrics-gate/examples/skills/metrics-gate ~/.cursor/skills/metrics-gate
```

2. Register MCP (Streamable HTTP) in `~/.cursor/mcp.json` against a running gate:

```json
{
  "mcpServers": {
    "metrics-gate": {
      "url": "http://127.0.0.1:8080/mcp"
    }
  }
}
```

For a remote deployment, replace the URL (one MCP entry = one product/instance).

3. Reload the Cursor window. Tools: `search`, `get_metric`, `get_dimension`, `list_metrics`.

Local smoke without MCP:

```bash
# from the metrics-gate repo
make up && make smoke
# or: docker run -p 8080:8080 -p 8090:8090 barloc/metrics-gate:latest
```

## Hard rules

- Prefer MCP tools over inventing metric ids. If `search` / `get_metric` misses, say not found (do not guess neighbors).
- Cite **`id`** (or `unique_id`) and envelope **`checksum`** plus `docs_sql_dialect` / `apply_warehouse`.
- Do **not** send `example_sql` to a query tool as executable SQL (`executable=false`).
- One MCP URL = one indexed manifest (one product instance).
- Methodology → this gate; warehouse schema/lineage and live numbers → other tools.

## Suggested flow

1. `search` with a short query (metric alias, name fragment, or dimension id).
2. Optionally filter with `resource_type`: `metric` or `dimension`.
3. `get_metric` / `get_dimension` for full methodology, `apply_column`, `example_sql`.
4. `list_metrics` when the user wants a category listing.

## HTTP equivalent

Same tools as JSON POST on the API port (default `:8080`):

| Tool | Method |
| --- | --- |
| `search` | `POST /v1/search` |
| `get_metric` | `POST /v1/get_metric` |
| `get_dimension` | `POST /v1/get_dimension` |
| `list_metrics` | `POST /v1/list_metrics` |

Example:

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/get_metric \
  -H 'Content-Type: application/json' \
  -d '{"id":"gp"}'
```
