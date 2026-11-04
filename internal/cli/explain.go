package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/explain"
)

var (
	explainJSON    bool
	explainSection string
)

var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Print documentation on ps2build and the ps2.yaml format",
	Long:  "Prints an overview, the CLI surface, the full ps2.yaml JSON Schema, and worked examples.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		doc := explain.BuildDoc()

		if explainJSON {
			data, err := explain.RenderJSON(doc, explainSection)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		}

		text, err := explain.RenderText(doc, explainSection)
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), text)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(explainCmd)
	explainCmd.Flags().BoolVar(&explainJSON, "json", false, "structured JSON output, intended for LLM agents")
	explainCmd.Flags().StringVar(&explainSection, "section", "", fmt.Sprintf("show only one section: %s", strings.Join(explain.ValidSections(), ", ")))
}
