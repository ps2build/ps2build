package mcpserver

import (
	"context"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ps2build/ps2build/internal/buildrun"
	"github.com/ps2build/ps2build/internal/generate"
	"github.com/ps2build/ps2build/internal/install"
)

type installProjectArgs struct {
	Path         string `json:"path" jsonschema:"Path to a ps2.yaml or ps2workspace.yaml."`
	OutDir       string `json:"out_dir,omitempty" jsonschema:"Scratch build output directory (not the packages destination). Defaults to 'build' next to the config file, matching the CLI's own default."`
	PackagesRoot string `json:"packages_root,omitempty" jsonschema:"Destination packages root. Defaults to this server's own resolved $PS2DEV/packages."`
	Toolchain    string `json:"toolchain,omitempty" jsonschema:"Compiler toolchain to build with: gcc or llvm. Defaults to PS2BUILD_TOOLCHAIN, then the project file's toolchain: field, then gcc."`
}

type installProjectResult struct {
	Success     bool         `json:"success"`
	Installed   []string     `json:"installed,omitempty"`
	Diagnostics []diagnostic `json:"diagnostics,omitempty"`
	Stdout      string       `json:"stdout"`
	Stderr      string       `json:"stderr"`
}

func registerInstallProject(s *mcp.Server, packageRoot string) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "install_project",
		Description: "Build a project, then copy each member's package.yaml-declared artifacts into $PS2DEV/packages - the same as \"ps2build install\".",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args installProjectArgs) (*mcp.CallToolResult, installProjectResult, error) {
		outDir := defaultOutDir(args.Path, args.OutDir)
		packagesRoot := args.PackagesRoot
		if packagesRoot == "" {
			packagesRoot = packageRoot
		}

		if err := generate.RunAuto(args.Path, outDir, args.Toolchain); err != nil {
			return nil, installProjectResult{Success: false, Stderr: err.Error()}, nil
		}

		selfExe, _ := os.Executable()
		buildResult, err := buildrun.Run(selfExe, outDir, nil, nil)
		if err != nil {
			return nil, installProjectResult{Success: false, Stderr: err.Error()}, nil
		}
		if !buildResult.Success {
			return nil, installProjectResult{
				Success:     false,
				Diagnostics: parseDiagnostics(buildResult.Stdout + "\n" + buildResult.Stderr),
				Stdout:      buildResult.Stdout,
				Stderr:      buildResult.Stderr,
			}, nil
		}

		result, err := install.Run(install.Options{
			ConfigPath:  args.Path,
			BuildOutDir: outDir,
		}, packagesRoot)
		if err != nil {
			return nil, installProjectResult{
				Success: false,
				Stdout:  buildResult.Stdout,
				Stderr:  buildResult.Stderr + "\n" + err.Error(),
			}, nil
		}

		return nil, installProjectResult{
			Success:   true,
			Installed: result.Installed,
			Stdout:    buildResult.Stdout,
			Stderr:    buildResult.Stderr,
		}, nil
	})
}
