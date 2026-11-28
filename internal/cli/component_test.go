package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/ps2build/ps2build/internal/update"
	"github.com/ps2build/ps2build/internal/utils"
)

type updateCall struct{ opts update.Options }

// fakeSDK points the cli at an empty sdk root holding the given versions.yaml components and replaces runUpdate.
func fakeSDK(t *testing.T, installed map[string]string, fn func(update.Options) (update.Result, error)) *[]updateCall {
	t.Helper()
	root := t.TempDir()
	var yaml strings.Builder
	yaml.WriteString("suite: \"\"\ncomponents:\n")
	for name, tag := range installed {
		yaml.WriteString("    " + name + ": " + tag + "\n")
	}
	if err := os.WriteFile(filepath.Join(root, "versions.yaml"), []byte(yaml.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	reset := utils.SetSDKRootOverride(root)
	t.Cleanup(reset)

	var calls []updateCall
	orig := runUpdate
	runUpdate = func(o update.Options) (update.Result, error) {
		calls = append(calls, updateCall{o})
		return fn(o)
	}
	t.Cleanup(func() { runUpdate = orig })
	return &calls
}

// installs reports every requested component as freshly installed at tag.
func installs(tag string) func(update.Options) (update.Result, error) {
	return func(o update.Options) (update.Result, error) {
		return update.Result{Components: []update.ComponentResult{{Name: o.Component, NewTag: tag, Updated: true}}}, nil
	}
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	for _, c := range []*cobra.Command{updateCmd, componentAddCmd, componentListCmd} {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs(args)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil); rootCmd.SetArgs(nil) })
	err = rootCmd.Execute()
	return out.String(), errOut.String(), err
}

func TestComponentAdd_InstallsThroughUpdateRun(t *testing.T) {
	calls := fakeSDK(t, nil, installs("v2026.10.13"))
	out, errOut, err := runCLI(t, "component", "add", "ogg")
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].opts.Component != "ogg" || (*calls)[0].opts.Check || (*calls)[0].opts.Force || (*calls)[0].opts.Version != "" {
		t.Fatalf("update.Run calls = %+v, want one plain install of ogg", *calls)
	}
	if !strings.Contains(out, "ogg") || !strings.Contains(out, "v2026.10.13") {
		t.Errorf("stdout = %q, want the installed tag", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing: component add is the supported path", errOut)
	}
}

func TestComponentAdd_VersionIsPassedVerbatim(t *testing.T) {
	calls := fakeSDK(t, nil, installs("2026.10.13"))
	if _, _, err := runCLI(t, "component", "add", "ogg", "--version", "2026.10.13"); err != nil {
		t.Fatal(err)
	}
	if got := (*calls)[0].opts.Version; got != "2026.10.13" {
		t.Errorf("Version = %q, want %q unchanged (no v prefix added)", got, "2026.10.13")
	}
}

func TestComponentAdd_AlreadyInstalledSaysSoAndSucceeds(t *testing.T) {
	calls := fakeSDK(t, map[string]string{"ogg": "v2026.10.13"}, installs("v2026.11.01"))
	out, _, err := runCLI(t, "component", "add", "ogg")
	if err != nil {
		t.Fatalf("err = %v, want exit 0", err)
	}
	if want := "ogg is already installed; use \"ps2build update ogg\"\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if len(*calls) != 0 {
		t.Errorf("update.Run called %d times for an installed component, want 0", len(*calls))
	}
}

func TestComponentAdd_ForceReinstalls(t *testing.T) {
	calls := fakeSDK(t, map[string]string{"ogg": "v2026.10.13"}, installs("v2026.10.13"))
	if _, _, err := runCLI(t, "component", "add", "ogg", "--force"); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || !(*calls)[0].opts.Force {
		t.Errorf("update.Run calls = %+v, want one forced install", *calls)
	}
}

func TestComponentAdd_SeveralNames(t *testing.T) {
	calls := fakeSDK(t, map[string]string{"vorbis": "v1"}, installs("v2"))
	out, _, err := runCLI(t, "component", "add", "ogg", "vorbis", "llvm-toolchain", "ogg")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range *calls {
		got = append(got, c.opts.Component)
	}
	if strings.Join(got, ",") != "ogg,llvm-toolchain" {
		t.Errorf("installed %v, want ogg and llvm-toolchain once each; vorbis is already installed", got)
	}
	if !strings.Contains(out, "vorbis is already installed") {
		t.Errorf("stdout = %q, want the already-installed note for vorbis", out)
	}
}

func TestComponentAdd_Errors(t *testing.T) {
	cases := map[string][]string{
		"ps2build is not addable":    {"component", "add", "ps2build"},
		"ps2build among others":      {"component", "add", "ogg", "ps2build"},
		"unknown component":          {"component", "add", "nope"},
		"version with several names": {"component", "add", "ogg", "vorbis", "--version", "v1"},
		"no names":                   {"component", "add"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			calls := fakeSDK(t, nil, installs("v1"))
			if _, _, err := runCLI(t, args...); err == nil {
				t.Fatal("err = nil, want an error")
			}
			if len(*calls) != 0 {
				t.Errorf("update.Run called %d times, want 0: nothing installs when the request is invalid", len(*calls))
			}
		})
	}
}

func TestComponentAdd_FailureIsReturned(t *testing.T) {
	boom := errors.New("download failed")
	fakeSDK(t, nil, func(update.Options) (update.Result, error) { return update.Result{}, boom })
	if _, _, err := runCLI(t, "component", "add", "ogg"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestComponentList_ShowsInstalledTagAndOptional(t *testing.T) {
	calls := fakeSDK(t, map[string]string{"ps2build": "v2026.11.11", "llvm-toolchain": "v2026.11.05"}, installs("unused"))
	out, _, err := runCLI(t, "component", "list")
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("list made %d update.Run calls without --available, want 0", len(*calls))
	}
	want := map[string][]string{
		"ps2build":       {"v2026.11.11"},
		"llvm-toolchain": {"v2026.11.05", "optional"},
		"ogg":            {"not installed", "optional"},
		"vorbis":         {"not installed", "optional"},
		"toolchain":      {"not installed"},
	}
	lines := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		lines[strings.Fields(l)[0]] = l
	}
	if len(lines) != len(update.Components()) {
		t.Errorf("listed %d components, want %d:\n%s", len(lines), len(update.Components()), out)
	}
	for name, parts := range want {
		for _, p := range parts {
			if !strings.Contains(lines[name], p) {
				t.Errorf("line for %s = %q, want it to contain %q", name, lines[name], p)
			}
		}
	}
	if strings.Contains(lines["toolchain"], "optional") || strings.Contains(lines["ps2build"], "optional") {
		t.Errorf("only llvm-toolchain, ogg and vorbis are optional:\n%s", out)
	}
}

func TestComponentList_AvailableFetchesEachLatestTag(t *testing.T) {
	calls := fakeSDK(t, map[string]string{"ninja": "v1.13.2"}, func(o update.Options) (update.Result, error) {
		if o.Component == "vorbis" {
			return update.Result{}, errors.New("no release")
		}
		return update.Result{Components: []update.ComponentResult{{Name: o.Component, NewTag: "v9-" + o.Component}}}, nil
	})
	out, _, err := runCLI(t, "component", "list", "--available")
	if err != nil {
		t.Fatalf("err = %v: one component without a release must not fail the listing", err)
	}
	if len(*calls) != len(update.Components()) {
		t.Fatalf("update.Run called %d times, want once per component", len(*calls))
	}
	for _, c := range *calls {
		if !c.opts.Check || c.opts.Version != "" {
			t.Errorf("call %+v, want a --check against the latest release", c.opts)
		}
	}
	if !strings.Contains(out, "v9-ogg") || !strings.Contains(out, "latest unavailable") {
		t.Errorf("stdout = %q, want latest tags and the unavailable note", out)
	}
}

func TestComponent_HasNoRemove(t *testing.T) {
	var names []string
	for _, c := range componentCmd.Commands() {
		names = append(names, c.Name())
	}
	if strings.Join(names, ",") != "add,list" {
		t.Errorf("component subcommands = %v, want add and list only; remove is reserved", names)
	}
	if _, _, err := runCLI(t, "component", "remove", "ogg"); err == nil {
		t.Error("component remove succeeded, want an unknown-command error")
	}
}

const deprecation = "warning: \"ps2build update ogg\" installed ogg, but installing with update is deprecated; use \"ps2build component add ogg\".\n"

func TestUpdate_NameNotInstalledBeforeWarnsOnStderr(t *testing.T) {
	fakeSDK(t, nil, installs("v2026.10.13"))
	out, errOut, err := runCLI(t, "update", "ogg")
	if err != nil {
		t.Fatal(err)
	}
	if errOut != deprecation {
		t.Errorf("stderr = %q, want %q", errOut, deprecation)
	}
	if !strings.Contains(out, "(not installed) -> v2026.10.13") || strings.Contains(out, "deprecated") {
		t.Errorf("stdout = %q, want the usual update line and no warning", out)
	}
}

func TestUpdate_NoWarningWhenAlreadyInstalledChecking_OrNothingChanged(t *testing.T) {
	updated := func(o update.Options) (update.Result, error) {
		return update.Result{Components: []update.ComponentResult{{Name: o.Component, PreviousTag: "v1", NewTag: "v2", Updated: true}}}, nil
	}
	current := func(o update.Options) (update.Result, error) {
		return update.Result{Components: []update.ComponentResult{{Name: o.Component, PreviousTag: "v2", NewTag: "v2"}}}, nil
	}
	checking := func(o update.Options) (update.Result, error) {
		return update.Result{Components: []update.ComponentResult{{Name: o.Component, NewTag: "v2"}}}, nil
	}
	for name, tc := range map[string]struct {
		fn   func(update.Options) (update.Result, error)
		args []string
	}{
		"was installed":   {updated, []string{"update", "ogg"}},
		"already current": {current, []string{"update", "ogg"}},
		"--check":         {checking, []string{"update", "ogg", "--check"}},
		"--version":       {updated, []string{"update", "ogg", "--version", "v2"}},
	} {
		t.Run(name, func(t *testing.T) {
			fakeSDK(t, nil, tc.fn)
			_, errOut, err := runCLI(t, tc.args...)
			if err != nil || errOut != "" {
				t.Errorf("err = %v, stderr = %q, want success and no warning", err, errOut)
			}
		})
	}
}

func TestUpdate_NoWarningForBareUpdateOrPS2Build(t *testing.T) {
	many := func(update.Options) (update.Result, error) {
		return update.Result{Components: []update.ComponentResult{{Name: "toolchain", NewTag: "v1", Updated: true}, {Name: "ninja", NewTag: "v1", Updated: true}}}, nil
	}
	fakeSDK(t, nil, many)
	if _, errOut, err := runCLI(t, "update"); err != nil || errOut != "" {
		t.Errorf("bare update: err = %v, stderr = %q, want success and no warning", err, errOut)
	}
	fakeSDK(t, nil, installs("v1"))
	if _, errOut, err := runCLI(t, "update", "ps2build"); err != nil || errOut != "" {
		t.Errorf("update ps2build: err = %v, stderr = %q, want success and no warning (component add cannot install it)", err, errOut)
	}
}

func TestUpdate_FailedInstallKeepsItsErrorAndSaysNothingElse(t *testing.T) {
	boom := errors.New("download failed")
	fakeSDK(t, nil, func(o update.Options) (update.Result, error) {
		return update.Result{Components: []update.ComponentResult{{Name: o.Component, NewTag: "v1", Updated: true}}}, boom
	})
	_, errOut, err := runCLI(t, "update", "ogg")
	if !errors.Is(err, boom) || errOut != "" {
		t.Errorf("err = %v, stderr = %q, want the original error and no warning", err, errOut)
	}
}
