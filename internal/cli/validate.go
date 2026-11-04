package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/compiledb"
)

var validateCmd = &cobra.Command{
	Use:   "validate [compile_commands.json]",
	Short: "Check a compile_commands.json for absolute paths, target triple, GCC-only flags and missing include dirs",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		file := "compile_commands.json"
		if len(args) == 1 {
			file = args[0]
		}
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		entries, err := compiledb.Read(f)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", file, err)
		}
		problems := compiledb.Validate(entries, compiledb.ValidateOptions{CheckDirs: true})
		for _, p := range problems {
			fmt.Fprintln(cmd.ErrOrStderr(), p)
		}
		if len(problems) > 0 {
			return fmt.Errorf("%d problem(s) in %s", len(problems), file)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %d entries OK\n", file, len(entries))
		return nil
	},
}

func init() { rootCmd.AddCommand(validateCmd) }
