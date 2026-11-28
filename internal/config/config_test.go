package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ps2.yaml")
	if err := os.WriteFile(path, []byte(`
project: test
targets:
  - name: main
    type: ee
    sources: [src/main.c]
  - name: mod
    type: iop
    sources: [src/mod.c]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Project != "test" {
		t.Errorf("Project = %q, want %q", cfg.Project, "test")
	}
	if len(cfg.Targets) != 2 {
		t.Fatalf("len(Targets) = %d, want 2", len(cfg.Targets))
	}

	ApplyDefaults(cfg)
	if cfg.Targets[0].Opt != "-O2" {
		t.Errorf("ee target Opt = %q, want -O2", cfg.Targets[0].Opt)
	}
	if cfg.Targets[1].Opt != "-Os" {
		t.Errorf("iop target Opt = %q, want -Os", cfg.Targets[1].Opt)
	}

	if err := Validate(cfg); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ps2.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const oneTarget = `targets:
  - name: main
    type: ee
    sources: [src/main.c]
`

func TestSchemaVersion(t *testing.T) {
	old := BuildVersion
	BuildVersion = "v2026.11.11"
	t.Cleanup(func() { BuildVersion = old })

	cases := []struct {
		name, header, wantErr string
	}{
		{"missing means 1", "project: t\n", ""},
		{"1 loads", "schema_version: 1\nproject: t\n", ""},
		{"2 needs a newer ps2build", "schema_version: 2\nproject: t\n",
			`schema_version 2 needs a newer ps2build (this is v2026.11.11, which reads up to 1); run "ps2build update"`},
		{"0 is an error", "schema_version: 0\nproject: t\n", "schema_version must be 1 or higher, got 0"},
		{"negative is an error", "schema_version: -3\nproject: t\n", "schema_version must be 1 or higher, got -3"},
		{"not an integer", "schema_version: one\nproject: t\n", "cannot unmarshal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, tc.header+oneTarget)
			cfg, err := Load(path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				if err := Validate(cfg); err != nil {
					t.Errorf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tc.wantErr)
			}
			if strings.Contains(tc.name, "needs") && err.Error() != path+": "+tc.wantErr {
				t.Errorf("Load error = %q, want %q", err, path+": "+tc.wantErr)
			}
		})
	}
}

func TestMissingSchemaVersionIsOne(t *testing.T) {
	cfg, err := Load(writeConfig(t, "project: t\n"+oneTarget))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SchemaVersion != nil {
		t.Errorf("SchemaVersion = %v, want nil (missing) - callers treat that as 1", *cfg.SchemaVersion)
	}
	if err := CheckSchemaVersion("x", cfg.SchemaVersion); err != nil {
		t.Errorf("CheckSchemaVersion(nil) = %v, want nil", err)
	}
}

func TestUnknownKeysWarnAndAreIgnored(t *testing.T) {
	path := writeConfig(t, `project: t
foo: 1
targets:
  - name: main
    type: ee
    sources: [src/main.c]
    foo: oops
  - name: mod
    type: iop
    sources: [src/mod.c]
    nested: {a: 1}
`)
	cfg, warnings, err := LoadWithWarnings(path)
	if err != nil {
		t.Fatalf("Load: %v - unknown keys must not be errors", err)
	}
	want := []string{
		path + `: unknown key "foo" ignored`,
		path + `: unknown key "targets[0].foo" ignored`,
		path + `: unknown key "targets[1].nested" ignored`,
	}
	if strings.Join(warnings, "\n") != strings.Join(want, "\n") {
		t.Errorf("warnings = %q, want %q", warnings, want)
	}
	if cfg.Project != "t" || len(cfg.Targets) != 2 || cfg.Targets[1].Name != "mod" {
		t.Errorf("known keys were not loaded: %+v", cfg)
	}
}

func TestLoadPrintsWarningsOnStderrOnce(t *testing.T) {
	path := writeConfig(t, "project: t\n"+oneTarget+"    foo: 1\n")
	warnedMu.Lock()
	warned = map[string]bool{}
	warnedMu.Unlock()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	for i := 0; i < 2; i++ {
		if _, err := Load(path); err != nil {
			os.Stderr = orig
			t.Fatal(err)
		}
	}
	os.Stderr = orig
	w.Close()
	var got strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		got.Write(buf[:n])
		if err != nil {
			break
		}
	}
	want := "warning: " + path + ": unknown key \"targets[0].foo\" ignored\n"
	if got.String() != want {
		t.Errorf("stderr = %q, want %q (once, though the file was loaded twice)", got.String(), want)
	}
}

func TestAnchorsAndMergeKeysAreNotUnknownKeys(t *testing.T) {
	path := writeConfig(t, `project: t
targets:
  - &base
    name: a
    type: iop
    sources: [a.c]
    headers: &hdrs [common, stdio]
  - <<: *base
    name: b
    headers: *hdrs
`)
	cfg, warnings, err := LoadWithWarnings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}
	if len(cfg.Targets) != 2 || cfg.Targets[1].Name != "b" || len(cfg.Targets[1].Sources) != 1 {
		t.Errorf("merge key not applied: %+v", cfg.Targets)
	}
}

func TestExistingFixturesLoadWithoutWarnings(t *testing.T) {
	var files []string
	for _, pattern := range []string{"../../testdata/invalid/*.yaml", "../explain/testdata/fixtures/*/ps2.yaml"} {
		m, err := filepath.Glob(pattern)
		if err != nil || len(m) == 0 {
			t.Fatalf("glob %s: %v, %d files", pattern, err, len(m))
		}
		files = append(files, m...)
	}
	for _, f := range files {
		cfg, warnings, err := LoadWithWarnings(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if len(warnings) != 0 || cfg.SchemaVersion != nil {
			t.Errorf("%s: warnings %q, schema_version %v - existing files must load exactly as before", f, warnings, cfg.SchemaVersion)
		}
	}
}

func TestValidateInvalidFixtures(t *testing.T) {
	cases := []struct {
		file      string
		wantError string
	}{
		{"embed-lib-target.yaml", `embed_irx references "other", which is type "ee_lib", not "iop" or "ee"`},
		{"bad-type.yaml", `type must be one of "ee", "iop", "ee_lib", "iop_lib", got "dvp"`},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join("..", "..", "testdata", "invalid", tc.file)
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			ApplyDefaults(cfg)

			err = Validate(cfg)
			if err == nil {
				t.Fatalf("Validate: expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Errorf("Validate error = %q, want it to contain %q", err.Error(), tc.wantError)
			}
		})
	}
}

func TestValidateEmbedIRXUnresolvedNameIsNotAnError(t *testing.T) {
	cfg := &Config{
		Project: "p",
		Targets: []Target{
			{Name: "main", Type: TargetEE, Sources: []string{"main.c"}, EmbedIRX: []string{"some-installed-driver-package"}},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("Validate() = %v, want nil - an unresolved embed_irx name defers to build time now", err)
	}
}

func TestValidateEmbedIRXAcceptsEETargetWithSymbolName(t *testing.T) {
	cfg := &Config{
		Project: "p",
		Targets: []Target{
			{Name: "loader", Type: TargetEE, Sources: []string{"loader.c"}},
			{Name: "main", Type: TargetEE, Sources: []string{"main.c"}, EmbedIRX: []string{"loader:loader_elf"}},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("Validate() = %v, want nil - embed_irx of an ee target with an explicit symbol name must validate", err)
	}
}

func TestValidateRejectsEEOnlyFieldsOnIOP(t *testing.T) {
	cfg := &Config{
		Project: "p",
		Targets: []Target{
			{Name: "helper", Type: TargetIOP, Sources: []string{"a.c"}, EmbedIRX: []string{"x"}},
		},
	}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "embed_irx is only valid on ee targets") {
		t.Errorf("Validate error = %v, want embed_irx-on-iop rejection", err)
	}

	cfg2 := &Config{
		Project: "p",
		Targets: []Target{
			{Name: "helper", Type: TargetIOP, Sources: []string{"a.c"}, OutputBin: true},
		},
	}
	if err := Validate(cfg2); err == nil || !strings.Contains(err.Error(), "output_bin is only valid on ee targets") {
		t.Errorf("Validate error = %v, want output_bin-on-iop rejection", err)
	}

	cfg3 := &Config{
		Project: "p",
		Targets: []Target{
			{Name: "helper", Type: TargetIOP, Sources: []string{"a.c"}, Pack: true},
		},
	}
	if err := Validate(cfg3); err == nil || !strings.Contains(err.Error(), "pack is only valid on ee targets") {
		t.Errorf("Validate error = %v, want pack-on-iop rejection", err)
	}
}

func TestValidateRejectsLinkOnlyFieldsOnLibTargets(t *testing.T) {
	cases := []struct {
		name    string
		target  Target
		wantErr string
	}{
		{"embed_irx", Target{Name: "l", Type: TargetEELib, Sources: []string{"a.c"}, EmbedIRX: []string{"x"}}, "embed_irx is only valid on ee targets"},
		{"output_bin", Target{Name: "l", Type: TargetEELib, Sources: []string{"a.c"}, OutputBin: true}, "output_bin is only valid on ee targets"},
		{"pack", Target{Name: "l", Type: TargetEELib, Sources: []string{"a.c"}, Pack: true}, "pack is only valid on ee targets"},
		{"libs", Target{Name: "l", Type: TargetEELib, Sources: []string{"a.c"}, Libs: []string{"m"}}, `libs is not valid on "ee_lib" targets`},
		{"ldflags", Target{Name: "l", Type: TargetIOPLib, Sources: []string{"a.c"}, LDFlags: []string{"-Wl,--gc-sections"}}, `ldflags is not valid on "iop_lib" targets`},
		{"linkfile", Target{Name: "l", Type: TargetIOPLib, Sources: []string{"a.c"}, Linkfile: "custom"}, `linkfile is not valid on "iop_lib" targets`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Project: "p", Targets: []Target{tc.target}}
			if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateLibsCrossReference(t *testing.T) {
	t.Run("ee target referencing an iop_lib is rejected", func(t *testing.T) {
		cfg := &Config{
			Project: "p",
			Targets: []Target{
				{Name: "mylib", Type: TargetIOPLib, Sources: []string{"a.c"}},
				{Name: "main", Type: TargetEE, Sources: []string{"b.c"}, Libs: []string{"mylib"}},
			},
		}
		want := `libs references "mylib", which is type "iop_lib", not "ee_lib"`
		if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate error = %v, want it to contain %q", err, want)
		}
	})

	t.Run("ee target referencing a matching ee_lib is accepted", func(t *testing.T) {
		cfg := &Config{
			Project: "p",
			Targets: []Target{
				{Name: "mylib", Type: TargetEELib, Sources: []string{"a.c"}},
				{Name: "main", Type: TargetEE, Sources: []string{"b.c"}, Libs: []string{"mylib"}},
			},
		}
		if err := Validate(cfg); err != nil {
			t.Errorf("Validate: %v, want no error", err)
		}
	})

	t.Run("a libs name matching no target is left as an external lib", func(t *testing.T) {
		cfg := &Config{
			Project: "p",
			Targets: []Target{
				{Name: "main", Type: TargetEE, Sources: []string{"b.c"}, Libs: []string{"png"}},
			},
		}
		if err := Validate(cfg); err != nil {
			t.Errorf("Validate: %v, want no error for an external library name", err)
		}
	})
}

func TestValidateDuplicateTargetName(t *testing.T) {
	cfg := &Config{
		Project: "p",
		Targets: []Target{
			{Name: "main", Type: TargetEE, Sources: []string{"a.c"}},
			{Name: "main", Type: TargetEE, Sources: []string{"b.c"}},
		},
	}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), `duplicate target name "main"`) {
		t.Errorf("Validate error = %v, want duplicate-name rejection", err)
	}
}

func TestParseEmbedIRXEntry(t *testing.T) {
	cases := []struct {
		entry       string
		wantResolve string
		wantSymbol  string
	}{
		{"hdd", "hdd", "hdd"},
		{"hdd:ps2hdd", "hdd", "ps2hdd"},
		{"mcman-old:mcman", "mcman-old", "mcman"},
	}
	for _, c := range cases {
		gotResolve, gotSymbol := ParseEmbedIRXEntry(c.entry)
		if gotResolve != c.wantResolve || gotSymbol != c.wantSymbol {
			t.Errorf("ParseEmbedIRXEntry(%q) = (%q, %q), want (%q, %q)", c.entry, gotResolve, gotSymbol, c.wantResolve, c.wantSymbol)
		}
	}
}

func TestValidateToolchain(t *testing.T) {
	base := Config{Project: "p", Targets: []Target{{Name: "a", Type: TargetEE, Sources: []string{"a.c"}}}}
	for _, tc := range []struct {
		kind    ToolchainKind
		wantErr bool
	}{
		{"", false}, {ToolchainGCC, false}, {ToolchainLLVM, false}, {"msvc", true},
	} {
		cfg := base
		cfg.Toolchain = tc.kind
		err := Validate(&cfg)
		if (err != nil) != tc.wantErr {
			t.Errorf("Validate(toolchain=%q) error = %v, wantErr %v", tc.kind, err, tc.wantErr)
		}
	}
}
