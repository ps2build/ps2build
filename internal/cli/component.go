package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/update"
	"github.com/ps2build/ps2build/internal/utils"
	"github.com/ps2build/ps2build/internal/versions"
)

// componentPS2Build is the one component "component add" cannot install: it replaces the running executable.
const componentPS2Build = "ps2build"

var (
	componentAddVersion    string
	componentAddForce      bool
	componentListAvailable bool
)

var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Install and list the components of the sdk",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var componentAddCmd = &cobra.Command{
	Use:   "add NAME...",
	Short: "Install components that are not installed yet",
	Long:  "Installs each named component, such as llvm-toolchain, ogg or vorbis. A component that is already installed is left alone; use \"ps2build update NAME\" to update it.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		known := map[string]bool{}
		var names []string
		for _, c := range update.Components() {
			names = append(names, c.Name)
			known[c.Name] = true
		}
		var wanted []string
		seen := map[string]bool{}
		for _, name := range args {
			switch {
			case name == componentPS2Build:
				return fmt.Errorf("%s cannot be added; use \"ps2build update %s\"", name, name)
			case !known[name]:
				return fmt.Errorf("unknown component %q - one of: %s", name, strings.Join(names, ", "))
			case !seen[name]:
				seen[name] = true
				wanted = append(wanted, name)
			}
		}
		if componentAddVersion != "" && len(wanted) != 1 {
			return fmt.Errorf("--version needs exactly one component name")
		}

		root := utils.DetectSDKRoot()
		installed := installedTags(root)
		out := cmd.OutOrStdout()
		for _, name := range wanted {
			if installed[name] != "" && !componentAddForce {
				fmt.Fprintf(out, "%s is already installed; use \"ps2build update %s\"\n", name, name)
				continue
			}
			result, err := runUpdate(update.Options{SDKRoot: root, Component: name, Version: componentAddVersion, Force: componentAddForce})
			for _, cr := range result.Components {
				prev := cr.PreviousTag
				if prev == "" {
					prev = "(not installed)"
				}
				if cr.Updated {
					fmt.Fprintf(out, "%s  %s -> %s\n", cr.Name, prev, cr.NewTag)
				} else {
					fmt.Fprintf(out, "%s  already up to date: %s\n", cr.Name, cr.PreviousTag)
				}
			}
			if err != nil {
				return err
			}
		}
		return nil
	},
}

var componentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every component and its installed version",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root := utils.DetectSDKRoot()
		installed := installedTags(root)
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		for _, c := range update.Components() {
			tag := installed[c.Name]
			if tag == "" {
				tag = "not installed"
			}
			cols := []string{c.Name, tag}
			if componentListAvailable {
				latest := "latest unavailable"
				if res, err := runUpdate(update.Options{SDKRoot: root, Component: c.Name, Check: true}); err == nil && len(res.Components) == 1 {
					latest = "latest " + res.Components[0].NewTag
				}
				cols = append(cols, latest)
			}
			if c.Optional {
				cols = append(cols, "optional")
			}
			fmt.Fprintln(w, strings.Join(cols, "\t"))
		}
		return w.Flush()
	},
}

// installedTags reads the tag recorded for each installed component; a missing versions.yaml means nothing is installed.
func installedTags(sdkRoot string) map[string]string {
	vf, err := versions.Load(sdkRoot)
	if err != nil || vf.Components == nil {
		return map[string]string{}
	}
	return vf.Components
}

func init() {
	rootCmd.AddCommand(componentCmd)
	componentCmd.AddCommand(componentAddCmd, componentListCmd)

	componentAddCmd.Flags().StringVar(&componentAddVersion, "version", "", "release tag to install (one component only; defaults to latest)")
	componentAddCmd.Flags().BoolVar(&componentAddForce, "force", false, "reinstall the latest release even if already installed")
	componentListCmd.Flags().BoolVar(&componentListAvailable, "available", false, "also show the latest release of each component")
}
