package update

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ps2build/ps2build/internal/utils"
	"github.com/ps2build/ps2build/internal/versions"
)

const defaultHost = "github.com"

// Options configures Run.
type Options struct {
	SDKRoot   string
	Component string
	Version   string
	Check     bool // report only, don't apply
	Force     bool // reinstall even if already on the resolved tag
	SkipSelf  bool // leave ps2build itself alone (a re-run by a freshly updated ps2build)
}

// ComponentResult summarizes what Run found/did for one component.
type ComponentResult struct {
	Name        string
	PreviousTag string // "" if not previously recorded (unknown, or a fresh bootstrap)
	NewTag      string
	Updated     bool
}

type Result struct {
	Components  []ComponentResult
	SelfUpdated bool // ps2build replaced itself and the remaining components were left for the new binary
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Run resolves and - unless opts.Check - applies the requested
// component(s) against opts.PS2Dev, updating each independently.
func Run(opts Options) (Result, error) {
	sdkRoot := utils.DetectSDKRoot()

	if runtime.GOOS == "windows" {
		removeAside(filepath.Join(sdkRoot, utils.ExeSuffix("ps2build")))
	}

	targets := defaultTargets(sdkRoot)
	if opts.Component != "" {
		c, ok := findComponent(opts.Component)
		if !ok {
			return Result{}, fmt.Errorf("unknown component %q - one of: %s", opts.Component, strings.Join(componentNames(), ", "))
		}
		targets = []component{c}
	}
	if opts.SkipSelf {
		targets = withoutSelfReplace(targets)
	}

	vf, err := versions.Load(sdkRoot)
	if err != nil {
		if freshErr := requireFreshTarget(sdkRoot); freshErr != nil {
			return Result{}, freshErr
		}
		vf = versions.File{}
	}
	if vf.Components == nil {
		vf.Components = map[string]string{}
	}

	var result Result
	for _, c := range targets {
		version := "latest"
		if c.PinnedVersion != "" {
			version = c.PinnedVersion
		}
		if opts.Component != "" && opts.Version != "" {
			version = opts.Version // explicit override always wins
		}
		cr, err := updateOne(sdkRoot, c, vf.Components[c.Name], version, opts.Check, opts.Force)
		result.Components = append(result.Components, cr)
		if err != nil {
			return result, fmt.Errorf("updating %s: %w", c.Name, err)
		}
		if c.SelfReplace && cr.Updated && opts.Component == "" && !opts.Check {
			result.SelfUpdated = true
			break
		}
	}

	return result, nil
}

func requireFreshTarget(sdkRoot string) error {
	entries, err := os.ReadDir(filepath.Join(sdkRoot, "packages"))
	if err == nil && len(entries) > 0 {
		return fmt.Errorf("no ps2build install detected at %s (missing versions.yaml), but packages/ already has real content - refusing to guess what this is; if this is meant to be a fresh install, start from an empty directory", sdkRoot)
	}
	return nil
}

// updateOne resolves, and - unless check or already up to date -
// downloads and applies, one component.
func updateOne(ps2dev string, c component, previousTag, version string, check, force bool) (ComponentResult, error) {
	release, err := fetchRelease(c.Host, c.Repo, version)
	if err != nil {
		return ComponentResult{Name: c.Name, PreviousTag: previousTag}, err
	}
	if release.TagName == "" {
		return ComponentResult{Name: c.Name, PreviousTag: previousTag}, fmt.Errorf("could not resolve a %s release for %q", c.Name, version)
	}

	result := ComponentResult{Name: c.Name, PreviousTag: previousTag, NewTag: release.TagName}
	if check {
		return result, nil
	}
	if release.TagName == previousTag && !force && componentInstalled(ps2dev, c) {
		return result, nil
	}

	platformSuffix := ""
	platformGated := c.WindowsSuffix != "" || c.LinuxSuffix != "" || c.DarwinSuffix != ""

	if platformGated {
		switch runtime.GOOS {
		case "windows":
			platformSuffix = c.WindowsSuffix
		case "linux":
			platformSuffix = c.LinuxSuffix
		case "darwin":
			platformSuffix = c.DarwinSuffix
		}
		if platformSuffix == "" {
			// No release asset for this platform; keep an existing install rather than failing the run.
			if componentInstalled(ps2dev, c) {
				return ComponentResult{Name: c.Name, PreviousTag: previousTag, NewTag: previousTag}, nil
			}
			return result, fmt.Errorf("%s has no %s release yet - install it manually under %s", c.Name, runtime.GOOS, filepath.Join(ps2dev, c.Test))
		}
	}

	suffix := platformSuffix + c.AssetSuffix

	var applyErr error
	if c.Tier != "" {
		applyErr = applySDKTier(ps2dev, c, release)
	} else {
		asset := findAsset(release, c.AssetPrefix, suffix)
		if asset == nil {
			return result, fmt.Errorf("no matching asset found on %s release %s", c.Name, release.TagName)
		}
		if c.SelfReplace {
			applyErr = applyPS2Build(asset.BrowserDownloadURL)
		} else {
			applyErr = applyLayer(ps2dev, c.Name, asset.BrowserDownloadURL)
		}
	}
	if applyErr != nil {
		return result, applyErr
	}

	if err := versions.SaveComponent(ps2dev, c.Name, release.TagName); err != nil {
		return result, fmt.Errorf("recording version: %w", err)
	}

	result.Updated = true
	return result, nil
}

func fetchRelease(apiHost, repo, version string) (releaseInfo, error) {
	var url string
	if apiHost == "" {
		apiHost = defaultHost
	}
	if apiHost == "github.com" {
		if version == "" || version == "latest" {
			url = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
		} else {
			url = fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, version)
		}
	} else {
		h := apiHost
		if version == "" || version == "latest" {
			url = fmt.Sprintf("https://%s/api/v1/repos/%s/releases/latest", h, repo)
		} else {
			url = fmt.Sprintf("https://%s/api/v1/repos/%s/releases/tags/%s", h, repo, version)
		}
	}

	resp, err := http.Get(url)
	if err != nil {
		return releaseInfo{}, fmt.Errorf("resolving %s release: %w", repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseInfo{}, fmt.Errorf("resolving %s release %q: %s", repo, version, resp.Status)
	}

	var rel releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return releaseInfo{}, fmt.Errorf("parsing release response: %w", err)
	}
	return rel, nil
}

// applySDKTier installs a tier release's gcc and llvm package sets. A release that predates the split ships one
// unsuffixed zip holding both, which stands in for the gcc zip.
func applySDKTier(ps2dev string, c component, rel releaseInfo) error {
	gcc := findAsset(rel, c.AssetPrefix, "-gcc"+c.AssetSuffix)
	if gcc == nil {
		gcc = findUnsuffixedAsset(rel, c.AssetPrefix, c.AssetSuffix)
	}
	if gcc == nil {
		return fmt.Errorf("no matching asset found on %s release %s", c.Name, rel.TagName)
	}
	if err := applyLayer(ps2dev, c.Name, gcc.BrowserDownloadURL); err != nil {
		return err
	}
	if llvm := findAsset(rel, c.AssetPrefix, "-llvm"+c.AssetSuffix); llvm != nil {
		return applyLayer(ps2dev, c.Name+"-llvm", llvm.BrowserDownloadURL)
	}
	return nil
}

func findUnsuffixedAsset(rel releaseInfo, prefix, suffix string) *releaseAsset {
	for i, a := range rel.Assets {
		if strings.HasPrefix(a.Name, prefix) && strings.HasSuffix(a.Name, suffix) &&
			!strings.HasSuffix(a.Name, "-gcc"+suffix) && !strings.HasSuffix(a.Name, "-llvm"+suffix) {
			return &rel.Assets[i]
		}
	}
	return nil
}

func findAsset(rel releaseInfo, prefix, suffix string) *releaseAsset {
	for i, a := range rel.Assets {
		if strings.HasPrefix(a.Name, prefix) && strings.HasSuffix(a.Name, suffix) {
			return &rel.Assets[i]
		}
	}
	return nil
}

// downloadToTemp downloads url to a new OS-temp file, rendering a
// progress bar labeled with the given component name as it goes.
func downloadToTemp(label, url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: %s", url, resp.Status)
	}

	out, err := os.CreateTemp("", "ps2dev-update-*.zip")
	if err != nil {
		return "", err
	}
	defer out.Close()

	pr := newProgressReader(resp.Body, label, resp.ContentLength)
	if _, err := io.Copy(out, pr); err != nil {
		os.Remove(out.Name())
		return "", fmt.Errorf("saving download: %w", err)
	}
	return out.Name(), nil
}

func downloadToFile(label, url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: %s", url, resp.Status)
	}

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	pr := newProgressReader(resp.Body, label, resp.ContentLength)
	_, err = io.Copy(out, pr)
	return err
}

// RemoveReplacedBinary deletes the ps2build.exe.old that a self-update on Windows leaves next to the new executable.
// It is a no-op while the old process still runs, so any later invocation finishes the job.
func RemoveReplacedBinary() {
	if runtime.GOOS != "windows" {
		return
	}
	if execPath, err := os.Executable(); err == nil {
		removeAside(execPath)
	}
}

func removeAside(execPath string) {
	_ = os.Remove(execPath + ".old")
	leftovers, _ := filepath.Glob(execPath + ".old.*")
	for _, p := range leftovers {
		_ = os.Remove(p)
	}
}

func applyPS2Build(url string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating running ps2build: %w", err)
	}

	newPath := execPath + ".new"
	if err := downloadToFile("ps2build", url, newPath); err != nil {
		os.Remove(newPath)
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(newPath, 0o755); err != nil {
			os.Remove(newPath)
			return fmt.Errorf("marking new ps2build executable: %w", err)
		}
	}

	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		if os.Remove(oldPath) != nil && fileExists(oldPath) {
			oldPath = fmt.Sprintf("%s.old.%d", execPath, os.Getpid())
		}
		if err := os.Rename(execPath, oldPath); err != nil {
			os.Remove(newPath)
			return fmt.Errorf("renaming running ps2build aside: %w", err)
		}
	}

	if err := os.Rename(newPath, execPath); err != nil {
		return fmt.Errorf("installing new ps2build: %w", err)
	}
	return nil
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("archive entry %q escapes destination", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractOne(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractOne(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}
