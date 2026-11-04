package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	genirxStubHeader string
	genirxStubIn     string
	genirxStubOut    string
)

var genirxStubCmd = &cobra.Command{
	Use:    "genirx-stub",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(genirxStubIn)
		if err != nil {
			return fmt.Errorf("reading %s: %w", genirxStubIn, err)
		}

		out, err := os.Create(genirxStubOut)
		if err != nil {
			return fmt.Errorf("creating %s: %w", genirxStubOut, err)
		}

		if _, err := fmt.Fprintf(out, "#include \"%s\"\n", genirxStubHeader); err != nil {
			out.Close()
			return fmt.Errorf("writing %s: %w", genirxStubOut, err)
		}
		if _, err := out.Write(data); err != nil {
			out.Close()
			return fmt.Errorf("writing %s: %w", genirxStubOut, err)
		}

		if err := out.Close(); err != nil {
			return fmt.Errorf("closing %s: %w", genirxStubOut, err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(genirxStubCmd)

	genirxStubCmd.Flags().StringVar(&genirxStubHeader, "header", "", "header to #include at the top of the generated file")
	genirxStubCmd.Flags().StringVar(&genirxStubIn, "in", "", "input imports.lst/exports.tab file")
	genirxStubCmd.Flags().StringVar(&genirxStubOut, "out", "", "output .c file path")
	_ = genirxStubCmd.MarkFlagRequired("header")
	_ = genirxStubCmd.MarkFlagRequired("in")
	_ = genirxStubCmd.MarkFlagRequired("out")
}
