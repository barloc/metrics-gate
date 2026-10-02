package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/barloc/metrics-gate/app/gate"
)

func (c *Controller) buildMCPHandler() http.Handler {
	s := server.NewMCPServer("metrics-gate", "1.0.0",
		server.WithToolCapabilities(true),
	)

	s.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Lexical search over metric and dimension methodology cards (id, aliases, text)"),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithString("category", mcp.Description("Optional category filter (path folder under metrics)")),
		mcp.WithString("resource_type", mcp.Description("Optional filter: metric or dimension (empty = both)")),
		mcp.WithNumber("limit", mcp.Description("Max hits (capped by server at 20)")),
	), c.mcpSearch)

	s.AddTool(mcp.NewTool("get_metric",
		mcp.WithDescription("Full metric card by id, name, or alias"),
		mcp.WithString("id", mcp.Description("Metric id (macro name)")),
		mcp.WithString("name", mcp.Description("Metric display name")),
		mcp.WithString("alias", mcp.Description("Alias (e.g. to_fx)")),
	), c.mcpGetMetric)

	s.AddTool(mcp.NewTool("get_dimension",
		mcp.WithDescription("Full dimension card by id, name, or alias"),
		mcp.WithString("id", mcp.Description("Dimension id (e.g. channel)")),
		mcp.WithString("name", mcp.Description("Dimension display name")),
		mcp.WithString("alias", mcp.Description("Alias")),
	), c.mcpGetDimension)

	s.AddTool(mcp.NewTool("list_metrics",
		mcp.WithDescription("List metric short hits; optional category; offset paging; paging.has_more when truncated"),
		mcp.WithString("category", mcp.Description("Optional category filter")),
		mcp.WithNumber("limit", mcp.Description("Max hits (capped at 20)")),
		mcp.WithNumber("offset", mcp.Description("Skip first N hits for pagination")),
	), c.mcpListMetrics)

	return server.NewStreamableHTTPServer(s,
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return r.Context()
		}),
	)
}

func (c *Controller) mcpSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	env := c.gate.Search(ctx, gate.SearchRequest{
		Query:        argString(args, "query"),
		Category:     argString(args, "category"),
		ResourceType: argString(args, "resource_type"),
		Limit:        argInt(args, "limit"),
	})
	return mcpResult(env), nil
}

func (c *Controller) mcpGetMetric(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	env := c.gate.GetMetric(ctx, gate.GetMetricRequest{
		ID:    argString(args, "id"),
		Name:  argString(args, "name"),
		Alias: argString(args, "alias"),
	})
	return mcpResult(env), nil
}

func (c *Controller) mcpGetDimension(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	env := c.gate.GetDimension(ctx, gate.GetDimensionRequest{
		ID:    argString(args, "id"),
		Name:  argString(args, "name"),
		Alias: argString(args, "alias"),
	})
	return mcpResult(env), nil
}

func (c *Controller) mcpListMetrics(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	env := c.gate.ListMetrics(ctx, gate.ListMetricsRequest{
		Category: argString(args, "category"),
		Limit:    argInt(args, "limit"),
		Offset:   argInt(args, "offset"),
	})
	return mcpResult(env), nil
}

func mcpResult(env gate.Envelope) *mcp.CallToolResult {
	b, err := json.Marshal(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error())
	}
	if !env.OK {
		return mcp.NewToolResultError(string(b))
	}
	return mcp.NewToolResultText(string(b))
}

func argString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func argInt(args map[string]any, key string) int {
	v, ok := args[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0
		}
		return int(i)
	default:
		return 0
	}
}
