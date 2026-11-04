package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/schema"
)

var schemaIndent int

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Print a JSON Schema for the ps2.yaml format",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := schema.Generate()

		var data []byte
		var err error
		if schemaIndent > 0 {
			data, err = json.MarshalIndent(s, "", strings.Repeat(" ", schemaIndent))
		} else {
			data, err = json.Marshal(s)
		}
		if err != nil {
			return fmt.Errorf("marshaling schema: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)
	schemaCmd.Flags().IntVar(&schemaIndent, "indent", 2, "pretty-print indent width in spaces, 0 for compact single-line output")
}
