package cli

import (
	"fmt"
	"sort"

	"github.com/ps2build/ps2build/internal/utils"
	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/versions"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print ps2build's version, and the installed suite's component versions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "ps2build %s\n", version)
		fmt.Fprint(out, thirdPartyNoticePointer)

		ps2dev := utils.DetectSDKRoot()
		v, err := versions.Load(ps2dev)
		if err != nil {
			return nil
		}

		fmt.Fprintf(out, "\nSDK suite: %s\n", v.Suite)
		names := make([]string, 0, len(v.Components))
		width := 0
		for name := range v.Components {
			names = append(names, name)
			if len(name) > width {
				width = len(name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(out, "  %-*s  %s\n", width, name, v.Components[name])
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
