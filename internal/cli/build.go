package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/buildrun"
	"github.com/ps2build/ps2build/internal/generate"
)

var (
	buildConfigPath string
	buildOutDir     string
	buildToolchain  string
)

func runBuild(configPath, outDir, toolchain string, stdout, stderr io.Writer) error {
	err := generate.RunAuto(configPath, outDir, toolchain)
	if offerToolchainInstall(err, os.Stdin, stdinIsTerminal(), stderr) {
		err = generate.RunAuto(configPath, outDir, toolchain)
	}
	if err != nil {
		return err
	}

	selfExe, _ := os.Executable()
	result, err := buildrun.Run(selfExe, outDir, stdout, stderr)
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("ninja: build failed")
	}
	return nil
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the project",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBuild(buildConfigPath, buildOutDir, buildToolchain, cmd.OutOrStdout(), cmd.ErrOrStderr())
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	addConfigFlag(buildCmd, &buildConfigPath)
	buildCmd.Flags().StringVarP(&buildOutDir, "out-dir", "o", "build", "build output directory")
	addToolchainFlag(buildCmd, &buildToolchain)
}
