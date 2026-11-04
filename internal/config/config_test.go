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

func TestLoadUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ps2.yaml")
	if err := os.WriteFile(path, []byte(`
project: test
targets:
  - name: main
    type: ee
    sources: [src/main.c]
    typo_field: oops
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load: expected an error for an unknown field, got nil")
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
