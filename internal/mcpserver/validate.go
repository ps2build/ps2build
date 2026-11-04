package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ps2build/ps2build/internal/config"
)

type validateTargetArgs struct {
	Path string `json:"path" jsonschema:"Path to a ps2.yaml (or ps2workspace.yaml - only top-level parse/validate is checked, not per-member) file."`
}

type validateTargetResult struct {
	Valid           bool   `json:"valid"`
	Error           string `json:"error,omitempty"`
	PS2BuildVersion string `json:"ps2build_version"`
}

func registerValidateTarget(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "validate_target",
		Description: "Validate a ps2.yaml without running a full generate/build - fast feedback while authoring a build definition.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args validateTargetArgs) (*mcp.CallToolResult, validateTargetResult, error) {
		cfg, err := config.Load(args.Path)
		if err != nil {
			return nil, validateTargetResult{Valid: false, Error: err.Error(), PS2BuildVersion: version}, nil
		}
		if err := config.Validate(cfg); err != nil {
			return nil, validateTargetResult{Valid: false, Error: err.Error(), PS2BuildVersion: version}, nil
		}
		config.ApplyDefaults(cfg)
		return nil, validateTargetResult{Valid: true, PS2BuildVersion: version}, nil
	})
}
