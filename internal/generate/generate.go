// Package generate orchestrates the full ps2.yaml -> build.ninja +
// compile_commands.json pipeline.
package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ps2build/ps2build/internal/buildgraph"
	"github.com/ps2build/ps2build/internal/compiledb"
	"github.com/ps2build/ps2build/internal/config"
	"github.com/ps2build/ps2build/internal/ninja"
	"github.com/ps2build/ps2build/internal/utils"
	"github.com/ps2build/ps2build/internal/workspace"
)

// Options configures a Run.
type Options struct {
	// ConfigPath is the path to the ps2.yaml file to read.
	ConfigPath string
	// OutDir is the build output directory: build.ninja,
	// compile_commands.json, and every compiled artifact are written here.
	OutDir            string
	WriteClangdConfig bool
	// Toolchain is the --toolchain flag value; "" defers to PS2BUILD_TOOLCHAIN, then ps2.yaml, then the user default, then gcc.
	Toolchain string
	// VerifyToolchain fails with a *buildgraph.NotInstalledError when the selected toolchain is not installed.
	VerifyToolchain bool
}

// resolveToolchain picks the toolchain by the standard precedence and, when verify is set, checks it is installed.
func resolveToolchain(flagValue string, projectValue config.ToolchainKind, verify bool) (buildgraph.Toolchain, error) {
	kind, err := buildgraph.SelectToolchain(flagValue, projectValue)
	if err != nil {
		return nil, err
	}
	tc, err := buildgraph.NewToolchain(kind)
	if err != nil {
		return nil, err
	}
	utils.UseLLVMPackages(kind == buildgraph.ToolchainLLVM)
	if v, ok := tc.(interface{ Verify() error }); ok && verify {
		if err := v.Verify(); err != nil {
			return nil, err
		}
	}
	return tc, nil
}

func Run(opts Options) error {
	cfg, err := config.LoadValidated(opts.ConfigPath)
	if err != nil {
		return err
	}

	projectRootNative, err := filepath.Abs(filepath.Dir(opts.ConfigPath))
	if err != nil {
		return fmt.Errorf("resolving project root: %w", err)
	}
	outDir, err := filepath.Abs(opts.OutDir)
	if err != nil {
		return fmt.Errorf("resolving out-dir: %w", err)
	}

	tc, err := resolveToolchain(opts.Toolchain, cfg.Toolchain, opts.VerifyToolchain)
	if err != nil {
		return err
	}

	graph, err := buildgraph.Build(cfg, filepath.ToSlash(projectRootNative), filepath.ToSlash(outDir), nil, tc)
	if err != nil {
		return err
	}
	printDeprecationWarnings(graph)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating out-dir %s: %w", outDir, err)
	}

	if err := writeEmbedHeaders(graph); err != nil {
		return err
	}

	if err := writeFile(filepath.Join(outDir, "build.ninja"), func(f *os.File) error {
		return ninja.Emit(graph, f)
	}); err != nil {
		return err
	}

	if err := writeFile(filepath.Join(outDir, "compile_commands.json"), func(f *os.File) error {
		return compiledb.Write(compiledb.Entries(graph), f)
	}); err != nil {
		return err
	}

	if opts.WriteClangdConfig && tc.Kind() == config.ToolchainGCC {
		if err := writeFile(filepath.Join(projectRootNative, ".clangd"), func(f *os.File) error {
			_, err := f.WriteString(compiledb.ClangdConfig(graph))
			return err
		}); err != nil {
			return err
		}
	}

	return nil
}

// WorkspaceOptions configures a RunWorkspace.
type WorkspaceOptions struct {
	WorkspaceConfigPath string
	OutDir              string
	Toolchain           string
	VerifyToolchain     bool
}

func RunWorkspace(opts WorkspaceOptions) error {

	outDir, err := filepath.Abs(opts.OutDir)
	if err != nil {
		return fmt.Errorf("resolving out-dir: %w", err)
	}

	wsCfg, err := workspace.Load(opts.WorkspaceConfigPath)
	if err != nil {
		return err
	}
	tc, err := resolveToolchain(opts.Toolchain, wsCfg.Toolchain, opts.VerifyToolchain)
	if err != nil {
		return err
	}

	graph, err := workspace.Build(opts.WorkspaceConfigPath, filepath.ToSlash(outDir), tc)
	if err != nil {
		return err
	}
	printDeprecationWarnings(graph)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating out-dir %s: %w", outDir, err)
	}

	if err := writeEmbedHeaders(graph); err != nil {
		return err
	}

	if err := writeFile(filepath.Join(outDir, "build.ninja"), func(f *os.File) error {
		return ninja.Emit(graph, f)
	}); err != nil {
		return err
	}

	if err := writeFile(filepath.Join(outDir, "compile_commands.json"), func(f *os.File) error {
		return compiledb.Write(compiledb.Entries(graph), f)
	}); err != nil {
		return err
	}

	return nil
}

// IsWorkspaceConfig reports whether configPath names a ps2workspace.yaml
// rather than a ps2.yaml.
func IsWorkspaceConfig(configPath string) bool {
	return filepath.Base(configPath) == "ps2workspace.yaml"
}

// RunAuto runs RunWorkspace or Run depending on configPath's filename.
func RunAuto(configPath, outDir, toolchain string) error {
	if IsWorkspaceConfig(configPath) {
		return RunWorkspace(WorkspaceOptions{WorkspaceConfigPath: configPath, OutDir: outDir, Toolchain: toolchain, VerifyToolchain: true})
	}
	return Run(Options{ConfigPath: configPath, OutDir: outDir, WriteClangdConfig: true, Toolchain: toolchain, VerifyToolchain: true})
}

func printDeprecationWarnings(graph *buildgraph.Graph) {
	for _, w := range graph.DeprecationWarnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
}

func writeFile(path string, fn func(*os.File) error) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}

	if err := fn(f); err != nil {
		f.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", path, err)
	}

	return nil
}

func writeEmbedHeaders(g *buildgraph.Graph) error {
	for _, b := range g.Bin2Cs {
		path := filepath.FromSlash(b.HeaderPath.String())
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
		}
		guard := strings.ToUpper(b.VarName) + "_H"
		content := fmt.Sprintf(`#ifndef %[1]s
#define %[1]s

#ifdef __cplusplus
extern "C" {
#endif

extern unsigned char %[2]s[];
extern unsigned int size_%[2]s;

#ifdef __cplusplus
}
#endif

#endif
`, guard, b.VarName)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	return nil
}
