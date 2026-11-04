package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ps2build/ps2build/internal/buildgraph"
)

type resolveLibsArgs struct {
	Libs   []string `json:"libs" jsonschema:"A hypothetical libs: list, exactly as it would appear in a ps2.yaml target."`
	Target string   `json:"target" jsonschema:"Which processor's package artifacts to resolve against: 'ee' or 'iop'."`
}

type resolveLibsResult struct {
	IncludeDirs     []string `json:"include_dirs"`
	LibDirs         []string `json:"lib_dirs"`
	Archives        []string `json:"archives"`
	NeedsCXX        bool     `json:"needs_cxx"`
	Deprecations    []string `json:"deprecations,omitempty"`
	UnresolvedNames []string `json:"unresolved_names,omitempty"`
}

func registerResolveLibs(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "resolve_libs",
		Description: "Resolve a hypothetical libs: list against installed packages, exactly as a real build would - use it to sanity-check a dependency list before writing it into ps2.yaml.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args resolveLibsArgs) (*mcp.CallToolResult, resolveLibsResult, error) {
		dirs, archives, includeDirs, deprecations, needsCXX, err := buildgraph.ResolvePackageLibs(args.Libs, nil, args.Target, "mcp")
		if err != nil {
			return nil, resolveLibsResult{}, err
		}

		resolved := map[string]bool{}
		for _, a := range archives {
			resolved[a] = true
		}
		var unresolved []string
		for _, name := range args.Libs {
			m, ok, _ := findQuiet(name)
			if !ok || m == nil {
				unresolved = append(unresolved, name)
			}
		}

		return nil, resolveLibsResult{
			IncludeDirs:     includeDirs,
			LibDirs:         dirs,
			Archives:        archives,
			NeedsCXX:        needsCXX,
			Deprecations:    deprecations,
			UnresolvedNames: unresolved,
		}, nil
	})
}
