package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/buildgraph"
	"github.com/ps2build/ps2build/internal/config"
	"github.com/ps2build/ps2build/internal/generate"
)

var (
	initProjectName string
	initOutDir      string
	initForce       bool
	initPackage     bool
	initWorkspace   bool
	initToolchain   string
)

var initCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Scaffold a new ps2build project",
	Long: "Creates ps2.yaml, a starter EE target, .gitignore, and .clangd.\n\n" +
		"--package scaffolds a linkable ee_lib instead of a bootable executable.\n" +
		"--workspace scaffolds ps2workspace.yaml instead, for a multi-project build.\n" +
		"--toolchain llvm sets toolchain: llvm in the project and skips .clangd, since the build's compile_commands.json already carries each target's flags.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if initPackage && initWorkspace {
			return fmt.Errorf("--package and --workspace are mutually exclusive")
		}
		toolchain, err := initToolchainKind()
		if err != nil {
			return err
		}

		dir := "."
		if len(args) == 1 {
			dir = args[0]
		}

		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}

		absDir, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("resolving %s: %w", dir, err)
		}

		name := initProjectName
		if name == "" {
			name = filepath.Base(absDir)
		}

		if initWorkspace {
			return initWorkspaceScaffold(cmd, dir, name)
		}

		configPath := filepath.Join(dir, "ps2.yaml")
		if _, err := os.Stat(configPath); err == nil && !initForce {
			return fmt.Errorf("%s already exists (pass --force to overwrite)", configPath)
		}

		files, err := initTargetFiles(dir, name, configPath, toolchain)
		if err != nil {
			return err
		}
		for path, content := range files {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", path)
		}

		if _, err := config.LoadValidated(configPath); err != nil {
			return err
		}

		outDir := filepath.Join(dir, initOutDir)
		if err := generate.Run(generate.Options{
			ConfigPath:        configPath,
			OutDir:            outDir,
			WriteClangdConfig: true,
			Toolchain:         initToolchain,
		}); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", outDir)

		return nil
	},
}

func initToolchainKind() (config.ToolchainKind, error) {
	if initToolchain == "" {
		return "", nil
	}
	kind, err := buildgraph.ParseToolchainKind(initToolchain)
	if err != nil {
		return "", fmt.Errorf("--toolchain: %w", err)
	}
	return kind, nil
}

func toolchainLine(kind config.ToolchainKind) string {
	if kind == "" {
		return ""
	}
	return fmt.Sprintf("toolchain: %s\n", kind)
}

func initTargetFiles(dir, name, configPath string, toolchain config.ToolchainKind) (map[string]string, error) {
	if !initPackage {
		if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
			return nil, fmt.Errorf("creating %s: %w", filepath.Join(dir, "src"), err)
		}
		files := map[string]string{
			configPath:                       ps2YamlTemplate(name, toolchain),
			filepath.Join(dir, "src/main.c"): mainCTemplate,
			filepath.Join(dir, ".gitignore"): gitignoreTemplate(initOutDir),
		}
		if toolchain != config.ToolchainLLVM {
			files[filepath.Join(dir, ".clangd")] = clangdTemplate
		}
		return files, nil
	}

	if err := os.MkdirAll(filepath.Join(dir, "include"), 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", filepath.Join(dir, "include"), err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", filepath.Join(dir, "src"), err)
	}

	ident := sanitizeCIdent(name)
	files := map[string]string{
		configPath:                               packageYamlTemplate(name, toolchain),
		filepath.Join(dir, "include", name+".h"): packageHeaderTemplate(ident),
		filepath.Join(dir, "src", name+".c"):     packageSourceTemplate(name, ident),
		filepath.Join(dir, ".gitignore"):         gitignoreTemplate(initOutDir),
	}
	if toolchain != config.ToolchainLLVM {
		files[filepath.Join(dir, ".clangd")] = clangdTemplate
	}
	return files, nil
}

func initWorkspaceScaffold(cmd *cobra.Command, dir, name string) error {
	path := filepath.Join(dir, "ps2workspace.yaml")
	if _, err := os.Stat(path); err == nil && !initForce {
		return fmt.Errorf("%s already exists (pass --force to overwrite)", path)
	}
	if err := os.WriteFile(path, []byte(workspaceYamlTemplate(name)), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", path)
	fmt.Fprintln(cmd.OutOrStdout(), "note: add each member (its own directory with a ps2.yaml) under members: before building")
	return nil
}

func ps2YamlTemplate(name string, toolchain config.ToolchainKind) string {
	return fmt.Sprintf(`project: %s
%s
targets:
  - name: %s
    type: ee
    sources:
      - src/main.c
`, name, toolchainLine(toolchain), name)
}

const mainCTemplate = `#include <stdio.h>

int main(int argc, char *argv[])
{
    printf("Hello World!\n");
    return 0;
}
`

func packageYamlTemplate(name string, toolchain config.ToolchainKind) string {
	return fmt.Sprintf(`project: %s
%s
targets:
  - name: %s
    type: ee_lib
    sources:
      - src/%s.c
    include_dirs:
      - include
`, name, toolchainLine(toolchain), name, name)
}

func packageHeaderTemplate(ident string) string {
	guard := strings.ToUpper(ident) + "_H"
	return fmt.Sprintf(`#ifndef %s
#define %s

#ifdef __cplusplus
extern "C" {
#endif

int %s_version(void);

#ifdef __cplusplus
}
#endif

#endif
`, guard, guard, ident)
}

func packageSourceTemplate(name, ident string) string {
	return fmt.Sprintf(`#include "%s.h"

int %s_version(void)
{
    return 1;
}
`, name, ident)
}

func workspaceYamlTemplate(name string) string {
	return fmt.Sprintf(`# ps2workspace.yaml - a multi-project build sharing one build.ninja
# and compile_commands.json. Add each member's directory below.
workspace: %s

members: []
`, name)
}

func sanitizeCIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" || (out[0] >= '0' && out[0] <= '9') {
		out = "_" + out
	}
	return out
}

func gitignoreTemplate(outDir string) string {
	return fmt.Sprintf("/%s/\n/.cache/\n", outDir)
}

const clangdTemplate = `CompileFlags:
  Remove:
    - -G*
    - -m*
    - -fno-toplevel-reorder
  Add:
    - -D_EE
`

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().StringVar(&initProjectName, "name", "", "project name (defaults to the directory name)")
	initCmd.Flags().StringVarP(&initOutDir, "out-dir", "o", "build", "build output directory, also written to .gitignore")
	initCmd.Flags().BoolVar(&initForce, "force", false, "overwrite an existing ps2.yaml")
	initCmd.Flags().BoolVar(&initPackage, "package", false, "scaffold an ee_lib package (include/+src/) instead of a bootable ee executable")
	initCmd.Flags().BoolVar(&initWorkspace, "workspace", false, "scaffold ps2workspace.yaml instead of a ps2.yaml project")
	initCmd.Flags().StringVar(&initToolchain, "toolchain", "", "compiler toolchain for the new project: gcc or llvm (default gcc)")
}
