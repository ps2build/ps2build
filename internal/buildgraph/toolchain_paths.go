package buildgraph

import (
	"os"
	"path"
	"path/filepath"

	"github.com/ps2build/ps2build/internal/utils"
)

// Host-built utilities buildgraph invokes that are not cross-compilers.
const (
	iopfixupTool = "iopfixup"
	bin2cTool    = "bin2c"
	ps2packTool  = "ps2pack"
	// ps2buildTool is ps2build's own binary, self-invoked by the iop_ietable_stub ninja rule.
	ps2buildTool = "ps2build"
)

// addHostTools adds the tools both toolchains share: the standalone assemblers, packers and ps2build itself.
func addHostTools(paths map[string]string, sdkRoot string) {
	toolsDir := path.Join(sdkRoot, "tools")
	paths[dvpAS] = path.Join(toolsDir, utils.ExeSuffix(dvpAS))
	paths[vclTool] = path.Join(sdkRoot, "vcl", utils.ExeSuffix(vclTool))
	paths[iopfixupTool] = path.Join(toolsDir, utils.ExeSuffix(iopfixupTool))
	paths[bin2cTool] = path.Join(toolsDir, utils.ExeSuffix(bin2cTool))
	paths[ps2packTool] = path.Join(toolsDir, utils.ExeSuffix(ps2packTool))

	if exe, err := os.Executable(); err == nil {
		paths[ps2buildTool] = filepath.ToSlash(exe)
	}
}
