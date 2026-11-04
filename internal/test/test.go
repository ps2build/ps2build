package test

import (
	"os"
	"path/filepath"

	"github.com/ps2build/ps2build/internal/utils"
)

func SetupCommonIncludeDir() string {
	packageRoot := utils.PackageRoot()
	dir := filepath.Join(packageRoot, "core", "common")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	manifest := filepath.Join(dir, "package.yaml")
	os.WriteFile(manifest, []byte("name: common\nartifacts:\n  - kind: headers\n    target: [ee, iop]\n    include_dirs: [include]"), 0o644)
	return dir
}
