package mcp

import (
	"context"
	"fmt"
	"os"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
)

// Run starts the MCP server over stdio. Project resolution uses the process cwd
// on each tool call; .hc is not required to start the server.
func Run() error {
	s := server.NewMCPServer("hybrid-coco", "0.3.0", server.WithToolCapabilities(false))

	s.AddTool(mcpgo.NewTool("hc_search",
		mcpgo.WithDescription("FTS5 search over symbol names, signatures and docstrings. Requires .hc in the project."),
		mcpgo.WithString("query", mcpgo.Required(), mcpgo.Description("Search query")),
		mcpgo.WithNumber("limit", mcpgo.Description("Max results")),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return withStore(func(st *store.Store, _ string) (*mcpgo.CallToolResult, error) {
			query, err := req.RequireString("query")
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			limit := 20
			if args := req.GetArguments(); args != nil {
				if raw, ok := args["limit"]; ok {
					switch n := raw.(type) {
					case float64:
						limit = int(n)
					case int:
						limit = n
					}
				}
			}
			if limit <= 0 {
				return mcpgo.NewToolResultError("limit must be > 0"), nil
			}
			results, err := st.FTSSearch(query, limit)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			return mcpgo.NewToolResultText(FormatSearch(query, results)), nil
		})
	})

	s.AddTool(mcpgo.NewTool("hc_symbol",
		mcpgo.WithDescription("Exact (then prefix) symbol lookup by name, with fan-in and call neighbors. Requires .hc."),
		mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("Symbol name")),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return withStore(func(st *store.Store, _ string) (*mcpgo.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			results, err := st.LookupSymbol(name)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			enrich, err := EnrichSymbols(st, results)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			return mcpgo.NewToolResultText(FormatSymbol(name, results, enrich)), nil
		})
	})

	s.AddTool(mcpgo.NewTool("hc_file_context",
		mcpgo.WithDescription("All symbols in a file (path relative to project root), with Read offset/limit hints. Requires .hc."),
		mcpgo.WithString("path", mcpgo.Required(), mcpgo.Description("Relative file path")),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return withStore(func(st *store.Store, _ string) (*mcpgo.CallToolResult, error) {
			path, err := req.RequireString("path")
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			data, err := st.FileContext(path)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			text, err := FormatFileContext(path, data)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			return mcpgo.NewToolResultText(text), nil
		})
	})

	s.AddTool(mcpgo.NewTool("hc_package",
		mcpgo.WithDescription("Directory/package outline with fan-in and Read range hints. Requires .hc."),
		mcpgo.WithString("dir", mcpgo.Required(), mcpgo.Description("Relative directory (e.g. internal/store)")),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return withStore(func(st *store.Store, _ string) (*mcpgo.CallToolResult, error) {
			dir, err := req.RequireString("dir")
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			data, err := st.PackageOutline(dir)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			text, err := FormatPackage(dir, data)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			return mcpgo.NewToolResultText(text), nil
		})
	})

	s.AddTool(mcpgo.NewTool("hc_explore",
		mcpgo.WithDescription("Structural neighborhood: callers, callees, containment, imports, fan-in/out. Requires .hc."),
		mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("Symbol name")),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return withStore(func(st *store.Store, _ string) (*mcpgo.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			data, err := st.Explore(name)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			text, err := FormatExplore(name, data)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			return mcpgo.NewToolResultText(text), nil
		})
	})

	s.AddTool(mcpgo.NewTool("hc_impact",
		mcpgo.WithDescription("Blast radius: direct callers and files that import this symbol's file. Requires .hc."),
		mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("Symbol name")),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return withStore(func(st *store.Store, _ string) (*mcpgo.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			data, err := st.Impact(name)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			return mcpgo.NewToolResultText(FormatImpact(name, data)), nil
		})
	})

	s.AddTool(mcpgo.NewTool("hc_path",
		mcpgo.WithDescription("Call path between two symbols (BFS, max depth 6). Requires .hc."),
		mcpgo.WithString("from", mcpgo.Required(), mcpgo.Description("Start symbol name")),
		mcpgo.WithString("to", mcpgo.Required(), mcpgo.Description("End symbol name")),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return withStore(func(st *store.Store, _ string) (*mcpgo.CallToolResult, error) {
			from, err := req.RequireString("from")
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			to, err := req.RequireString("to")
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			hops, err := st.Path(from, to)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			return mcpgo.NewToolResultText(FormatPath(from, to, hops)), nil
		})
	})

	s.AddTool(mcpgo.NewTool("hc_status",
		mcpgo.WithDescription("Index status: file/symbol/edge counts, hotspots, last update. Requires .hc."),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return withStore(func(st *store.Store, dbPath string) (*mcpgo.CallToolResult, error) {
			stats, err := st.Stats()
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			return mcpgo.NewToolResultText(FormatStatus(stats, dbPath)), nil
		})
	})

	if err := server.ServeStdio(s); err != nil {
		return fmt.Errorf("mcp server: %w", err)
	}
	return nil
}

// withStore opens the index for the process cwd when .hc is present.
// Without marker / version mismatch / missing index → tool error (server stays up).
func withStore(fn func(st *store.Store, dbPath string) (*mcpgo.CallToolResult, error)) (*mcpgo.CallToolResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	st, dbPath, err := openProjectStore(cwd)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	defer st.Close()
	return fn(st, dbPath)
}

func openProjectStore(start string) (*store.Store, string, error) {
	_, id, dbPath, err := config.ResolveProject(start)
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("shared index not found at %s. Run: hc setup", dbPath)
		}
		return nil, "", err
	}
	st, err := store.OpenForProject(dbPath, id)
	if err != nil {
		return nil, "", err
	}
	return st, dbPath, nil
}
