package buildgraph

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/ps2build/ps2build/internal/utils"
)

func TestResolveToolchainPaths(t *testing.T) {
	t.Run("resolves a real toolchain under PS2DEV/PS2SDK", func(t *testing.T) {
		write(t, utils.SdkToolchainPath(path.Join("ee", "bin", utils.ExeSuffix(eeGCC))))
		write(t, utils.SdkToolPath(utils.ExeSuffix("bin2c")))

		paths := gccToolchain{}.Paths()

		wantGCC := utils.SdkToolchainPath(path.Join("ee/bin", utils.ExeSuffix(eeGCC)))
		if paths[eeGCC] != wantGCC {
			t.Errorf("gccToolchain{}.Paths()[eeGCC] = %q, want %q", paths[eeGCC], wantGCC)
		}
		wantBin2C := utils.SdkToolPath(utils.ExeSuffix("bin2c"))
		if paths["bin2c"] != wantBin2C {
			t.Errorf(`gccToolchain{}.Paths()["bin2c"] = %q, want %q`, paths["bin2c"], wantBin2C)
		}
	})

	t.Run("resolves dvp-as under tools, alongside bin2c/ps2pack", func(t *testing.T) {
		write(t, utils.SdkToolPath(utils.ExeSuffix(dvpAS)))

		paths := gccToolchain{}.Paths()

		want := utils.SdkToolPath(utils.ExeSuffix(dvpAS))
		if paths[dvpAS] != want {
			t.Errorf("gccToolchain{}.Paths()[dvpAS] = %q, want %q", paths[dvpAS], want)
		}
	})

	t.Run("iopfixup prefers PS2DEV/bin/tools over the legacy flat PS2SDK/bin", func(t *testing.T) {
		write(t, utils.SdkToolPath(utils.ExeSuffix("iopfixup")))

		paths := gccToolchain{}.Paths()

		want := utils.SdkToolPath(utils.ExeSuffix("iopfixup"))
		if paths["iopfixup"] != want {
			t.Errorf(`gccToolchain{}.Paths()["iopfixup"] = %q, want %q (bin/tools should win over the flat ps2sdk fallback)`, paths["iopfixup"], want)
		}
	})
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
