// Command ps2build generates a build.ninja and compile_commands.json from a
// ps2.yaml PS2 EE/IOP build definition.
package main

import (
	"fmt"
	"os"

	"github.com/ps2build/ps2build/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
