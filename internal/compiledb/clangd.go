package compiledb

import (
	"fmt"
	"strings"

	"github.com/ps2build/ps2build/internal/buildgraph"
)

func ClangdConfig(g *buildgraph.Graph) string {
	define := ""
	for _, c := range g.Compiles {
		for _, f := range c.FlagArgs {
			if f == "-D_EE" || f == "-D_IOP" {
				define = f
				break
			}
		}
		if define != "" {
			break
		}
	}

	var b strings.Builder
	b.WriteString("CompileFlags:\n")
	b.WriteString("  Remove:\n")
	b.WriteString("    - -G*\n")
	b.WriteString("    - -m*\n")
	b.WriteString("    - -fno-toplevel-reorder\n")
	if define != "" {
		b.WriteString("  Add:\n")
		fmt.Fprintf(&b, "    - %s\n", define)
	}
	return b.String()
}
