package cli

import (
	"fmt"

	"github.com/ps2build/ps2build/internal/utils"
	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/install"
)

var (
	installConfigPath   string
	installBuildOutDir  string
	installPackagesRoot string
	installToolchain    string
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Build the project and install its packages",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runBuild(installConfigPath, installBuildOutDir, installToolchain, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return err
		}

		packagesRoot := installPackagesRoot
		if packagesRoot == "" {
			packagesRoot = utils.InstallPackageRoot()
			if packagesRoot == "" {
				return fmt.Errorf("could not resolve $PS2DEV/packages - pass --packages explicitly")
			}
		}

		result, err := install.Run(install.Options{
			ConfigPath:  installConfigPath,
			BuildOutDir: installBuildOutDir,
		}, packagesRoot)
		if err != nil {
			return err
		}

		for _, name := range result.Installed {
			fmt.Fprintf(cmd.OutOrStdout(), "installed %s\n", name)
		}
		if len(result.Installed) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "nothing to install (no package.yaml found)")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)

	addConfigFlag(installCmd, &installConfigPath)
	installCmd.Flags().StringVarP(&installBuildOutDir, "out-dir", "o", "build", "build output directory (a scratch dir - not the packages destination)")
	installCmd.Flags().StringVar(&installPackagesRoot, "packages", "", "destination packages root (default: $PS2DEV/packages, or $PS2DEV/packages-llvm with the llvm toolchain)")
	addToolchainFlag(installCmd, &installToolchain)
}
