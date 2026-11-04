package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ps2build/ps2build/internal/buildrun"
	"github.com/ps2build/ps2build/internal/generate"
)

type buildProjectArgs struct {
	Path      string `json:"path" jsonschema:"Path to a ps2.yaml or ps2workspace.yaml."`
	OutDir    string `json:"out_dir,omitempty" jsonschema:"Build output directory. Defaults to 'build' next to the config file, matching the CLI's own default."`
	Toolchain string `json:"toolchain,omitempty" jsonschema:"Compiler toolchain to build with: gcc or llvm. Defaults to PS2BUILD_TOOLCHAIN, then the project file's toolchain: field, then gcc."`
}

type diagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type buildProjectResult struct {
	Success         bool         `json:"success"`
	Diagnostics     []diagnostic `json:"diagnostics,omitempty"`
	Stdout          string       `json:"stdout"`
	Stderr          string       `json:"stderr"`
	PS2BuildVersion string       `json:"ps2build_version"`
}

var diagRe = regexp.MustCompile(`^(.+?):(\d+):(?:(\d+):)?\s*(error|warning|fatal error):\s*(.*)$`)

func parseDiagnostics(text string) []diagnostic {
	var out []diagnostic
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimRight(line, "\r")
		m := diagRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		lineNo, _ := strconv.Atoi(m[2])
		col := 0
		if m[3] != "" {
			col, _ = strconv.Atoi(m[3])
		}
		out = append(out, diagnostic{
			File:     m[1],
			Line:     lineNo,
			Column:   col,
			Severity: m[4],
			Message:  m[5],
		})
	}
	return out
}

func defaultOutDir(configPath, outDir string) string {
	if outDir != "" {
		return outDir
	}
	return filepath.Join(filepath.Dir(configPath), "build")
}

func registerBuildProject(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "build_project",
		Description: "Build a ps2.yaml/ps2workspace.yaml project - same as \"ps2build build\", returning structured pass/fail with file:line:message diagnostics.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args buildProjectArgs) (*mcp.CallToolResult, buildProjectResult, error) {
		outDir := defaultOutDir(args.Path, args.OutDir)
		if err := generate.RunAuto(args.Path, outDir, args.Toolchain); err != nil {
			return nil, buildProjectResult{Success: false, Stderr: err.Error(), PS2BuildVersion: version}, nil
		}

		selfExe, _ := os.Executable()
		result, err := buildrun.Run(selfExe, outDir, nil, nil)
		if err != nil {
			return nil, buildProjectResult{Success: false, Stderr: err.Error(), PS2BuildVersion: version}, nil
		}

		return nil, buildProjectResult{
			Success:         result.Success,
			Diagnostics:     parseDiagnostics(result.Stdout + "\n" + result.Stderr),
			Stdout:          result.Stdout,
			Stderr:          result.Stderr,
			PS2BuildVersion: version,
		}, nil
	})
}
