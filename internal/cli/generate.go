package cli

import (
	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/generate"
)

var (
	generateConfigPath string
	generateOutDir     string
	generateToolchain  string
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Prepare a project to be built, without building it",
	Long:  "Supports a single project or every member of a ps2workspace.yaml, detected automatically from --config.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return generate.RunAuto(generateConfigPath, generateOutDir, generateToolchain)
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)

	addConfigFlag(generateCmd, &generateConfigPath)
	generateCmd.Flags().StringVarP(&generateOutDir, "out-dir", "o", "build", "build output directory")
	addToolchainFlag(generateCmd, &generateToolchain)
}
