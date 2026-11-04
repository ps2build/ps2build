package mcpserver

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ps2build/ps2build/internal/explain"
	"github.com/ps2build/ps2build/internal/schema"
)

// --- get_schema ---

type getSchemaResult struct {
	Schema any `json:"schema"`
}

func registerGetSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_schema",
		Description: "The full JSON Schema for ps2.yaml - same as \"ps2build schema\".",
		OutputSchema: &jsonschema.Schema{
			Type:     "object",
			Required: []string{"schema"},
			Properties: map[string]*jsonschema.Schema{
				"schema": {Type: "object"},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, getSchemaResult, error) {
		return nil, getSchemaResult{Schema: schema.Generate()}, nil
	})
}

// --- explain ---

type explainArgs struct {
	Section string `json:"section,omitempty" jsonschema:"One section to return (see ps2build explain --list for valid names). Omit for the full document."`
}

type explainResult struct {
	Text string `json:"text"`
}

func registerExplain(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "explain",
		Description: "Documentation for ps2build and ps2.yaml - same as \"ps2build explain\": target types, fields, and worked examples.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args explainArgs) (*mcp.CallToolResult, explainResult, error) {
		doc := explain.BuildDoc()
		text, err := explain.RenderText(doc, args.Section)
		if err != nil {
			return nil, explainResult{}, err
		}
		return nil, explainResult{Text: text}, nil
	})
}
