package cli

import (
	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/update"
)

// version is set at build time via -ldflags "-X .../internal/cli.version=...";
// left as "dev" for plain "go build"/"go run".
var version = "dev"

var rootCmd = &cobra.Command{
	Use:           "ps2build",
	Short:         "Build tool for PS2 homebrew projects",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

const thirdPartyNoticePointer = "Third-party licenses: see licenses/ in the source repository (github.com/ps2build/ps2build).\n"

func init() {
	rootCmd.SetVersionTemplate("ps2build {{.Version}}\n" + thirdPartyNoticePointer)
}

// Execute runs the root command.
func Execute() error {
	update.RemoveReplacedBinary()
	return rootCmd.Execute()
}

func addToolchainFlag(cmd *cobra.Command, p *string) {
	cmd.Flags().StringVar(p, "toolchain", "", "compiler toolchain to build with: gcc or llvm (default: PS2BUILD_TOOLCHAIN, then toolchain: in the project file, then gcc)")
}

func addConfigFlag(cmd *cobra.Command, p *string) {
	cmd.Flags().StringVarP(p, "config", "c", "ps2.yaml", "path to the ps2.yaml (or ps2workspace.yaml) build definition")
}
