package install

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ps2build/ps2build/internal/config"
	"github.com/ps2build/ps2build/internal/pkgmanifest"
	"github.com/ps2build/ps2build/internal/workspace"
)

// Options configures Run.
type Options struct {
	// ConfigPath is a ps2.yaml or ps2workspace.yaml - same dispatch-by-
	// filename rule as "ps2build build"/"ps2build generate".
	ConfigPath string
	// BuildOutDir is where the build already wrote its output (the same
	// -o given to "ps2build build"), not the packages destination.
	BuildOutDir  string
	PackagesRoot string
}

// Result summarizes what Run installed.
type Result struct {
	// Installed lists "<tier>/<name>" for every package copied into
	// place, in the order processed.
	Installed []string
}

type meta struct {
	Tier string `yaml:"tier"`
}

func readTier(packageYAMLPath string) (string, error) {
	data, err := os.ReadFile(packageYAMLPath)
	if err != nil {
		return "", err
	}
	var m meta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return "", err
	}
	if m.Tier != "core" && m.Tier != "world" {
		return "", fmt.Errorf("%s: tier must be \"core\" or \"world\", got %q", packageYAMLPath, m.Tier)
	}
	return m.Tier, nil
}

// member is one thing to install: a workspace entry, or (for a plain
// ps2.yaml, no workspace) the single project itself.
type member struct {
	relPath string // relative to configRoot, forward-slash ("" for a plain ps2.yaml project)
	root    string // this member's own source directory, absolute
}

func Run(opts Options, packagesRoot string) (Result, error) {
	configRoot, err := filepath.Abs(filepath.Dir(opts.ConfigPath))
	if err != nil {
		return Result{}, fmt.Errorf("resolving config root: %w", err)
	}
	buildOutDir, err := filepath.Abs(opts.BuildOutDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolving build out-dir: %w", err)
	}

	var members []member
	if filepath.Base(opts.ConfigPath) == "ps2workspace.yaml" {
		wsCfg, err := workspace.Load(opts.ConfigPath)
		if err != nil {
			return Result{}, err
		}
		for _, m := range wsCfg.Members {
			members = append(members, member{
				relPath: filepath.ToSlash(m),
				root:    filepath.ToSlash(filepath.Join(configRoot, filepath.FromSlash(m))),
			})
		}
	} else {
		members = []member{{relPath: "", root: filepath.ToSlash(configRoot)}}
	}

	var result Result
	for _, m := range members {
		manifestPath := path.Join(m.root, "package.yaml")
		if _, err := os.Stat(filepath.FromSlash(manifestPath)); err != nil {
			continue // no package.yaml - nothing to install for this member
		}

		manifest, err := pkgmanifest.Load(manifestPath)
		if err != nil {
			return result, fmt.Errorf("member %q: %w", m.relPath, err)
		}
		tier, err := readTier(manifestPath)
		if err != nil {
			return result, fmt.Errorf("member %q: %w", m.relPath, err)
		}

		memberCfgPath := filepath.Join(filepath.FromSlash(m.root), "ps2.yaml")
		memberCfg, err := config.Load(memberCfgPath)
		if err != nil {
			return result, fmt.Errorf("member %q: %w", m.relPath, err)
		}

		memberBuildDir := path.Join(filepath.ToSlash(buildOutDir), m.relPath)
		destDir := filepath.Join(packagesRoot, tier, manifest.Name)

		if err := os.RemoveAll(destDir); err != nil {
			return result, fmt.Errorf("installing %s/%s: %w", tier, manifest.Name, err)
		}
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return result, fmt.Errorf("installing %s/%s: %w", tier, manifest.Name, err)
		}

		if err := copyFile(filepath.FromSlash(manifestPath), filepath.Join(destDir, "package.yaml")); err != nil {
			return result, fmt.Errorf("installing %s/%s: %w", tier, manifest.Name, err)
		}

		if licenseFile, ok := findLicenseFile(filepath.FromSlash(m.root)); ok {
			if err := copyFile(licenseFile, filepath.Join(destDir, filepath.Base(licenseFile))); err != nil {
				return result, fmt.Errorf("installing %s/%s: copying license file: %w", tier, manifest.Name, err)
			}
		}

		for _, a := range manifest.Artifacts {
			// Headers are real source files, checked into the member's
			// own tree - copy from there, not from the build output.
			for i, rel := range a.IncludeDirs {
				src := filepath.FromSlash(manifest.AbsIncludeDirs(a)[i])
				dst := filepath.Join(destDir, filepath.FromSlash(rel))
				if err := copyDir(src, dst); err != nil {
					return result, fmt.Errorf("installing %s/%s: copying include dir %q: %w", tier, manifest.Name, rel, err)
				}
			}
			// Linkfile is a real source file too, same as IncludeDirs -
			// not a build output like Lib/IRX below.
			if a.Linkfile != "" {
				src := filepath.FromSlash(manifest.AbsLinkfile(a))
				dst := filepath.Join(destDir, filepath.FromSlash(a.Linkfile))
				if err := copyFile(src, dst); err != nil {
					return result, fmt.Errorf("installing %s/%s: copying linkfile %q: %w", tier, manifest.Name, a.Linkfile, err)
				}
			}
			if a.Lib != "" {
				targetName, err := libTargetName(memberCfg, a.Target)
				if err != nil {
					return result, fmt.Errorf("installing %s/%s: %w", tier, manifest.Name, err)
				}
				src := filepath.FromSlash(path.Join(memberBuildDir, "lib", "lib"+targetName+".a"))
				dst := filepath.Join(destDir, filepath.FromSlash(a.Lib))
				if err := copyFile(src, dst); err != nil {
					return result, fmt.Errorf("installing %s/%s: copying lib %q: %w", tier, manifest.Name, a.Lib, err)
				}
			}
			if a.IRX != "" {
				targetName, err := iopTargetName(memberCfg)
				if err != nil {
					return result, fmt.Errorf("installing %s/%s: %w", tier, manifest.Name, err)
				}
				src := filepath.FromSlash(path.Join(memberBuildDir, "bin", targetName+".irx"))
				dst := filepath.Join(destDir, filepath.FromSlash(a.IRX))
				if err := copyFile(src, dst); err != nil {
					return result, fmt.Errorf("installing %s/%s: copying irx %q: %w", tier, manifest.Name, a.IRX, err)
				}
			}
		}

		result.Installed = append(result.Installed, tier+"/"+manifest.Name)
	}

	return result, nil
}

func libTargetName(cfg *config.Config, artifactTarget []string) (string, error) {
	var want config.TargetType
	switch {
	case contains(artifactTarget, "ee"):
		want = config.TargetEELib
	case contains(artifactTarget, "iop"):
		want = config.TargetIOPLib
	default:
		return "", fmt.Errorf("library artifact has no ee/iop in target: %v", artifactTarget)
	}
	var found []string
	for _, t := range cfg.Targets {
		if t.Type == want {
			found = append(found, t.Name)
		}
	}
	if len(found) != 1 {
		return "", fmt.Errorf("expected exactly one %s target, found %v", want, found)
	}
	return found[0], nil
}

func iopTargetName(cfg *config.Config) (string, error) {
	var found []string
	for _, t := range cfg.Targets {
		if t.Type == config.TargetIOP {
			found = append(found, t.Name)
		}
	}
	if len(found) != 1 {
		return "", fmt.Errorf("expected exactly one iop target, found %v", found)
	}
	return found[0], nil
}

var licenseFileNames = []string{"LICENSE", "LICENSE.TXT", "LICENSE.md", "COPYING", "COPYING.LIB", "COPYRIGHT"}

func findLicenseFile(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, want := range licenseFileNames {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.EqualFold(e.Name(), want) {
				return filepath.Join(dir, e.Name()), true
			}
		}
	}
	return "", false
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target)
	})
}
