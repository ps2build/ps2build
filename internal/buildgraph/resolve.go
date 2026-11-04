package buildgraph

import (
	"fmt"
	"path"
	"strings"

	"github.com/ps2build/ps2build/internal/pkgmanifest"
	"github.com/ps2build/ps2build/internal/utils"
)

type libInfo struct {
	Archive     string
	NeedsCXX    bool
	IncludeDirs []string
	Local       bool
}

// gcc's *startfile spec always prepends its own crt0.o ahead of -lstartup; don't "fix" with -nostartfiles, it drops crtbegin.o's __dso_handle too.
func implicitStartupLibs(libsByName map[string]libInfo, target string) []string {
	if resolvesAsLibrary(libsByName, target, "startup") {
		return []string{"startup"}
	}
	return nil
}

// kernel.h is available to every EE target without listing kernel. The library itself is not
// added to the link line: the toolchain's default library group already links it after libc.
func implicitKernelHeaders(headers []string, libsByName map[string]libInfo) []string {
	for _, h := range headers {
		if h == "kernel" {
			return headers
		}
	}
	if !resolvesAsLibrary(libsByName, "ee", "kernel") {
		return headers
	}
	return append(append([]string{}, headers...), "kernel")
}

func resolvesAsLibrary(libsByName map[string]libInfo, target, name string) bool {
	if _, ok := libsByName[name]; ok {
		return true
	}
	packageRoot := utils.PackageRoot()
	if packageRoot == "" {
		return false
	}
	m, ok, err := pkgmanifest.Find(packageRoot, name)
	if err != nil || !ok {
		return false
	}
	_, ok = m.LibraryArtifact(target)
	return ok
}

func resolveLibDeps(libs []string, libsByName map[string]libInfo) (localDirs, localArchives, externalDirs, externalArchives, includeDirs []string, needsCXX bool) {
	seenLocalDir := make(map[string]bool)
	seenExternalDir := make(map[string]bool)
	for _, name := range libs {
		lib, ok := libsByName[name]
		if !ok {
			continue
		}
		dir := path.Dir(lib.Archive)
		if lib.Local {
			localArchives = append(localArchives, lib.Archive)
			if !seenLocalDir[dir] {
				localDirs = append(localDirs, dir)
				seenLocalDir[dir] = true
			}
		} else {
			externalArchives = append(externalArchives, lib.Archive)
			if !seenExternalDir[dir] {
				externalDirs = append(externalDirs, dir)
				seenExternalDir[dir] = true
			}
		}
		includeDirs = append(includeDirs, lib.IncludeDirs...)
		if lib.NeedsCXX {
			needsCXX = true
		}
	}
	return localDirs, localArchives, externalDirs, externalArchives, includeDirs, needsCXX
}

func ResolvePackageLibs(libs []string, libsByName map[string]libInfo, target, targetName string) (dirs, archives, includeDirs, deprecations []string, needsCXX bool, err error) {
	packageRoot := utils.PackageRoot()
	seenDir := make(map[string]bool)
	seenInclude := make(map[string]bool)
	addInclude := func(d string) {
		if !seenInclude[d] {
			includeDirs = append(includeDirs, d)
			seenInclude[d] = true
		}
	}
	visited := make(map[string]bool)
	for _, name := range libs {
		if _, ok := libsByName[name]; ok {
			continue
		}
		m, ok, findErr := pkgmanifest.Find(packageRoot, name)
		if findErr != nil {
			return nil, nil, nil, nil, false, fmt.Errorf("resolving package %q: %w", name, findErr)
		}
		if !ok {
			continue
		}
		a, ok := m.LibraryArtifact(target)
		if !ok {
			if da, dok := m.DriverArtifact(target); dok {
				if da.Deprecated != "" {
					deprecations = append(deprecations, fmt.Sprintf(
						"%s: package %q (driver) is deprecated: %s", targetName, name, da.Deprecated))
				}
				for _, d := range m.AbsIncludeDirs(da) {
					addInclude(d)
				}
				visited[name] = true
				hdrDirs, err := resolvePackageHeaders(da.Headers, packageRoot, target, visited)
				if err != nil {
					return nil, nil, nil, nil, false, err
				}
				for _, d := range hdrDirs {
					addInclude(d)
				}
			}
			continue
		}
		if a.Deprecated != "" {
			deprecations = append(deprecations, fmt.Sprintf(
				"%s: package %q (library) is deprecated: %s", targetName, name, a.Deprecated))
		}
		archivePath := m.AbsLib(a)
		archives = append(archives, archivePath)
		for _, d := range m.AbsIncludeDirs(a) {
			addInclude(d)
		}
		if a.CXX {
			needsCXX = true
		}
		dir := path.Dir(archivePath)
		if !seenDir[dir] {
			dirs = append(dirs, dir)
			seenDir[dir] = true
		}

		visited[name] = true
		hdrDirs, err := resolvePackageHeaders(a.Headers, packageRoot, target, visited)
		if err != nil {
			return nil, nil, nil, nil, false, err
		}
		for _, d := range hdrDirs {
			addInclude(d)
		}
	}
	return dirs, archives, includeDirs, deprecations, needsCXX, nil
}

// resolveHeaderIncludeDirs resolves a target's headers: field against workspace sibling library targets and installed packages.
func resolveHeaderIncludeDirs(headers []string, libsByName map[string]libInfo, target, targetName string) (includeDirs, deprecations []string, err error) {
	packageRoot := utils.PackageRoot()
	for _, name := range headers {
		if _, ok := libsByName[name]; ok {
			continue
		}
		_, found, findErr := pkgmanifest.Find(packageRoot, name)
		if findErr != nil {
			return nil, nil, fmt.Errorf("resolving package %q: %w", name, findErr)
		}
		if !found {
			return nil, nil, fmt.Errorf("%s: headers: %q is not an installed package or a library target in this workspace", targetName, name)
		}
	}
	_, _, _, _, localIncludeDirs, _ := resolveLibDeps(headers, libsByName)
	_, _, pkgIncludeDirs, deprecations, _, err := ResolvePackageLibs(headers, libsByName, target, targetName)
	if err != nil {
		return nil, nil, err
	}
	return append(localIncludeDirs, pkgIncludeDirs...), deprecations, nil
}

var toolchainLibs = map[string]bool{"c": true, "g": true, "m": true, "gcc": true, "stdc++": true, "supc++": true, "nosys": true}

// A bare -l<name> with no matching package and no -L to find it fails at link time with no pointer to the cause.
func unresolvedLibWarnings(libs, ldflags []string, libsByName map[string]libInfo, targetName string) []string {
	for _, f := range ldflags {
		if strings.HasPrefix(f, "-L") {
			return nil
		}
	}
	packageRoot := utils.PackageRoot()
	var warnings []string
	for _, name := range libs {
		if _, ok := libsByName[name]; ok || toolchainLibs[name] {
			continue
		}
		if _, found, err := pkgmanifest.Find(packageRoot, name); err != nil || found {
			continue
		}
		if siblingLibraryDeclared(libs, packageRoot, name) {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"%s: libs: %q is not an installed package or a library target, so -l%s is emitted without a -L path", targetName, name, name))
	}
	return warnings
}

// A package can ship several archives (vorbis ships libvorbis.a and libvorbisfile.a); listing the package puts its lib directory on the link path, so the other archives resolve too.
func siblingLibraryDeclared(libs []string, packageRoot, name string) bool {
	archive := "lib" + name + ".a"
	for _, other := range libs {
		m, found, err := pkgmanifest.Find(packageRoot, other)
		if err != nil || !found {
			continue
		}
		for _, a := range m.Artifacts {
			if a.Kind == "library" && path.Base(a.Lib) == archive {
				return true
			}
		}
	}
	return false
}

func resolvePackageHeaders(names []string, packageRoot, target string, visited map[string]bool) ([]string, error) {
	var dirs []string
	for _, name := range names {
		if visited[name] {
			continue
		}
		visited[name] = true
		m, ok, err := pkgmanifest.Find(packageRoot, name)
		if err != nil {
			return nil, fmt.Errorf("resolving header dependency %q: %w", name, err)
		}
		if !ok {
			continue
		}
		a, ok := m.LibraryArtifact(target)
		if !ok {
			if a, ok = m.DriverArtifact(target); !ok {
				continue
			}
		}
		d, err := artifactHeaderDirs(m, a, packageRoot, target, visited)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, d...)
	}
	return dirs, nil
}

func artifactHeaderDirs(m *pkgmanifest.Manifest, a pkgmanifest.Artifact, packageRoot, target string, visited map[string]bool) ([]string, error) {
	dirs := m.AbsIncludeDirs(a)
	nested, err := resolvePackageHeaders(a.Headers, packageRoot, target, visited)
	if err != nil {
		return nil, err
	}
	return append(dirs, nested...), nil
}

func resolvePackageDriverIRX(name string, target string) (irxPath string, found bool, deprecated string, err error) {
	packageRoot := utils.PackageRoot()
	if packageRoot == "" {
		return "", false, "", nil
	}
	m, ok, findErr := pkgmanifest.Find(packageRoot, name)
	if findErr != nil {
		return "", false, "", fmt.Errorf("resolving driver package %q: %w", name, findErr)
	}
	if !ok {
		return "", false, "", nil
	}
	a, ok := m.DriverArtifact(target)
	if !ok {
		return "", false, "", nil
	}
	return m.AbsIRX(a), true, a.Deprecated, nil
}

func resolveLinkOrder(libs []string, libsByName map[string]libInfo, target string) ([]string, error) {
	packageRoot := utils.PackageRoot()

	visited := make(map[string]bool, len(libs))
	for _, name := range libs {
		visited[name] = true
	}

	var deps []string
	var visitLinksOf func(name string) error
	visitLinksOf = func(name string) error {
		if _, ok := libsByName[name]; ok {
			return nil
		}
		m, ok, err := pkgmanifest.Find(packageRoot, name)
		if err != nil {
			return fmt.Errorf("resolving link dependency %q: %w", name, err)
		}
		if !ok {
			return nil
		}
		a, ok := m.LibraryArtifact(target)
		if !ok {
			return nil
		}
		for _, link := range a.Links {
			if visited[link] {
				continue
			}
			visited[link] = true
			if err := visitLinksOf(link); err != nil {
				return err
			}
			deps = append(deps, link)
		}
		return nil
	}

	for _, name := range libs {
		if err := visitLinksOf(name); err != nil {
			return nil, err
		}
	}

	// Reversed so dependencies land after their dependents - a static
	// linker resolves symbols in one left-to-right pass.
	for i, j := 0, len(deps)-1; i < j; i, j = i+1, j-1 {
		deps[i], deps[j] = deps[j], deps[i]
	}

	return append(append([]string{}, libs...), deps...), nil
}
