package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/ps2build/ps2build/internal/utils"
	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/update"
)

var (
	updateVersion string
	updateCheck   bool
	updateForce   bool
)

const reexecEnvVar = "PS2BUILD_UPDATE_REEXEC"

var updateCmd = &cobra.Command{
	Use:   "update [component]",
	Short: "Update an installed ps2build sdk in place, or bootstrap one from nothing",
	Long: "With no arguments, updates every component (toolchain, ps2build, sdk-core, sdk-world) to its own latest release. Given a component name, updates just that one, optionally pinned with --version.\n\n" +
		"Run against a directory with nothing installed yet, this bootstraps a full install instead.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := ""
		if len(args) == 1 {
			component = args[0]
		}
		if updateVersion != "" && component == "" {
			return fmt.Errorf("--version requires a specific component, e.g. \"ps2build update sdk-world --version v2026.09.02.3\"")
		}

		sdkRoot := utils.DetectSDKRoot()

		result, runErr := update.Run(update.Options{
			SDKRoot:   sdkRoot,
			Component: component,
			Version:   updateVersion,
			Check:     updateCheck,
			Force:     updateForce,
			SkipSelf:  os.Getenv(reexecEnvVar) == "1",
		})

		out := cmd.OutOrStdout()
		width := 0
		for _, cr := range result.Components {
			if len(cr.Name) > width {
				width = len(cr.Name)
			}
		}
		for _, cr := range result.Components {
			prev := cr.PreviousTag
			if prev == "" {
				prev = "(not installed)"
			}
			switch {
			case updateCheck:
				if cr.NewTag == cr.PreviousTag {
					fmt.Fprintf(out, "%-*s  up to date: %s\n", width, cr.Name, prev)
				} else {
					fmt.Fprintf(out, "%-*s  installed: %-20s available: %s\n", width, cr.Name, prev, cr.NewTag)
				}
			case cr.Updated:
				fmt.Fprintf(out, "%-*s  %s -> %s\n", width, cr.Name, prev, cr.NewTag)
			default:
				fmt.Fprintf(out, "%-*s  already up to date: %s\n", width, cr.Name, cr.PreviousTag)
			}
		}

		if runErr == nil && result.SelfUpdated {
			fmt.Fprintf(out, "\nps2build updated itself - re-running to finish updating the rest...\n\n")
			newExe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("locating the just-updated ps2build: %w", err)
			}
			childArgs := []string{"update"}
			if updateForce {
				childArgs = append(childArgs, "--force")
			}
			child := exec.Command(newExe, childArgs...)
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			child.Stdin = os.Stdin
			child.Env = append(os.Environ(), reexecEnvVar+"=1")
			if err := child.Run(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					os.Exit(exitErr.ExitCode())
				}
				return fmt.Errorf("re-running the updated ps2build: %w", err)
			}
			os.Exit(0)
		}

		return runErr
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)

	updateCmd.Flags().StringVar(&updateVersion, "version", "", "release tag to update to (only valid with a specific component; defaults to latest)")
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "only report installed vs. available versions, don't apply")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "reinstall even if already on the resolved version")
}
