package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/ps2build/ps2build/internal/buildgraph"
	"github.com/ps2build/ps2build/internal/compiledb"
	"github.com/ps2build/ps2build/internal/test"
	"github.com/ps2build/ps2build/internal/utils"
)

const (
	projectRootPlaceholder = "<PROJECT_ROOT>"
	outDirPlaceholder      = "<OUT_DIR>"
	ps2devPlaceholder      = "<PS2DEV>"
)

var fixtures = []string{"simple-ee", "simple-iop", "embed-irx", "embed-irx-ee", "overrides", "static-lib", "cxx-lib", "vu-microcode"}

func escapeNinjaColon(s string) string {
	return strings.ReplaceAll(s, ":", "$:")
}

func TestGenerateGolden(t *testing.T) {
	// The pipeline reads package.yaml and headers from disk, so PS2DEV must be a writable directory; redact maps it to a placeholder.
	ps2devAbs, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatalf("resolving ps2dev dir: %v", err)
	}
	t.Cleanup(utils.SetSDKRootOverride(ps2devAbs))

	oldExeSuffix := utils.ExeSuffixValue
	utils.ExeSuffixValue = ""
	t.Cleanup(func() { utils.ExeSuffixValue = oldExeSuffix })

	test.SetupCommonIncludeDir()

	llvmRoot := filepath.Join(ps2devAbs, "llvm-toolchain")
	if err := os.MkdirAll(filepath.Join(llvmRoot, "ee", "bin"), 0o755); err != nil {
		t.Fatalf("creating fake llvm toolchain: %v", err)
	}
	t.Setenv(buildgraph.LLVMRootEnvVar, llvmRoot)

	for _, tc := range []struct{ toolchain, goldenDir string }{
		{"gcc", "golden"},
		{"llvm", "golden-llvm"},
	} {
		for _, name := range fixtures {
			t.Run(tc.toolchain+"/"+name, func(t *testing.T) {
				fixtureDir := filepath.Join("..", "explain", "testdata", "fixtures", name)
				configPath := filepath.Join(fixtureDir, "ps2.yaml")

				projectRootAbs, err := filepath.Abs(fixtureDir)
				if err != nil {
					t.Fatalf("resolving fixture dir: %v", err)
				}

				outDir := t.TempDir()
				outDirAbs, err := filepath.Abs(outDir)
				if err != nil {
					t.Fatalf("resolving out dir: %v", err)
				}

				if err := Run(Options{
					ConfigPath: configPath,
					OutDir:     outDir,
					Toolchain:  tc.toolchain,
				}); err != nil {
					t.Fatalf("Run: %v", err)
				}

				redact := func(s string) string {
					s = strings.ReplaceAll(s, escapeNinjaColon(filepath.ToSlash(projectRootAbs)), projectRootPlaceholder)
					s = strings.ReplaceAll(s, escapeNinjaColon(filepath.ToSlash(outDirAbs)), outDirPlaceholder)
					s = strings.ReplaceAll(s, escapeNinjaColon(filepath.ToSlash(ps2devAbs)), ps2devPlaceholder)
					s = strings.ReplaceAll(s, filepath.ToSlash(projectRootAbs), projectRootPlaceholder)
					s = strings.ReplaceAll(s, filepath.ToSlash(outDirAbs), outDirPlaceholder)
					s = strings.ReplaceAll(s, filepath.ToSlash(ps2devAbs), ps2devPlaceholder)
					s = strings.ReplaceAll(s, outDirAbs, outDirPlaceholder)
					s = strings.ReplaceAll(s, ps2devAbs, ps2devPlaceholder)
					return s
				}

				checkGolden(t, filepath.Join(fixtureDir, tc.goldenDir, "build.ninja"),
					readAndRedact(t, filepath.Join(outDir, "build.ninja"), redact))
				checkGolden(t, filepath.Join(fixtureDir, tc.goldenDir, "compile_commands.json"),
					readAndRedact(t, filepath.Join(outDir, "compile_commands.json"), redact))

				entries, err := compiledb.Read(strings.NewReader(readAndRedact(t, filepath.Join(outDir, "compile_commands.json"), redact)))
				if err != nil {
					t.Fatalf("parsing compile_commands.json: %v", err)
				}
				if problems := compiledb.Validate(entries, compiledb.ValidateOptions{}); len(problems) > 0 {
					t.Errorf("compile_commands.json invalid:\n%s", strings.Join(problems, "\n"))
				}
			})
		}
	}
}

func readAndRedact(t *testing.T, path string, redact func(string) string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading generated %s: %v", path, err)
	}
	return redact(string(data))
}

func checkGolden(t *testing.T, goldenPath, got string) {
	t.Helper()

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("creating golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden file %s (run with UPDATE_GOLDEN=1 to create it): %v", goldenPath, err)
	}

	if diff := cmp.Diff(string(want), got); diff != "" {
		t.Errorf("%s mismatch (-want +got):\n%s", goldenPath, diff)
	}
}

func TestWriteEmbedHeaders(t *testing.T) {
	dir := t.TempDir()
	hdr := buildgraph.NewGeneratedPath(filepath.ToSlash(filepath.Join(dir, "generated", "audsrv_irx.h")))
	g := &buildgraph.Graph{Bin2Cs: []buildgraph.Bin2CStep{{VarName: "audsrv_irx", HeaderPath: hdr}}}

	if err := writeEmbedHeaders(g); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "generated", "audsrv_irx.h"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"extern unsigned char audsrv_irx[];", "extern unsigned int size_audsrv_irx;", "#ifndef AUDSRV_IRX_H"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("header missing %q:\n%s", want, got)
		}
	}
}
