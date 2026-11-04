package buildgraph

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ps2build/ps2build/internal/config"
	"github.com/ps2build/ps2build/internal/utils"
)

type ExternalLib struct {
	Archive     string
	NeedsCXX    bool
	IncludeDirs []string
}

func Build(cfg *config.Config, projectRoot, outDir string, externalLibs map[string]ExternalLib, tc Toolchain) (*Graph, error) {
	g := &Graph{
		ProjectRoot:    projectRoot,
		OutDir:         outDir,
		TargetOutputs:  make(map[string]string),
		Toolchain:      tc,
		ToolchainPaths: tc.Paths(),
	}

	targets := make([]config.Target, len(cfg.Targets))
	copy(targets, cfg.Targets)
	for i := range targets {
		resolved, err := expandSources(projectRoot, targets[i].Sources)
		if err != nil {
			return nil, fmt.Errorf("target %q: %w", targets[i].Name, err)
		}
		targets[i].Sources = resolved
	}

	byName := make(map[string]config.Target, len(targets))
	for _, t := range targets {
		byName[t.Name] = t
		g.TargetOrder = append(g.TargetOrder, t.Name)
	}

	libsByName := make(map[string]libInfo, len(externalLibs))
	for name, ext := range externalLibs {
		libsByName[name] = libInfo{Archive: ext.Archive, NeedsCXX: ext.NeedsCXX, IncludeDirs: ext.IncludeDirs, Local: false}
	}
	for _, t := range targets {
		if t.Type != config.TargetEELib && t.Type != config.TargetIOPLib {
			continue
		}
		archivePath, err := buildLibTarget(g, t, projectRoot, libsByName)
		if err != nil {
			return nil, fmt.Errorf("target %q: %w", t.Name, err)
		}
		libsByName[t.Name] = libInfo{
			Archive:     archivePath,
			NeedsCXX:    hasCXXSource(t.Sources),
			IncludeDirs: utils.AbsPaths(projectRoot, t.IncludeDirs),
			Local:       true,
		}
		g.TargetOutputs[t.Name] = archivePath
	}

	irxByIOPTarget := make(map[string]string)
	for _, t := range targets {
		if t.Type != config.TargetIOP {
			continue
		}
		irxPath, err := buildIOPTarget(g, t, projectRoot, libsByName)
		if err != nil {
			return nil, fmt.Errorf("target %q: %w", t.Name, err)
		}
		irxByIOPTarget[t.Name] = irxPath
		g.TargetOutputs[t.Name] = irxPath
	}

	embedObjByTarget := make(map[string]string)
	elfByEETarget := make(map[string]string)

	// Embedded EE targets must build before the targets that embed them.
	eeTargets := make([]config.Target, 0, len(targets))
	for _, t := range targets {
		if t.Type == config.TargetEE {
			eeTargets = append(eeTargets, t)
		}
	}
	embeddedByOthers := make(map[string]bool)
	for _, t := range eeTargets {
		for _, name := range t.EmbedIRX {
			if embedded, ok := byName[name]; ok && embedded.Type == config.TargetEE {
				embeddedByOthers[name] = true
			}
		}
	}
	orderedEETargets := make([]config.Target, 0, len(eeTargets))
	for _, t := range eeTargets {
		if embeddedByOthers[t.Name] {
			orderedEETargets = append(orderedEETargets, t)
		}
	}
	for _, t := range eeTargets {
		if !embeddedByOthers[t.Name] {
			orderedEETargets = append(orderedEETargets, t)
		}
	}

	for _, t := range orderedEETargets {
		elfPath, err := buildEETarget(g, t, projectRoot, irxByIOPTarget, elfByEETarget, embedObjByTarget, libsByName)
		if err != nil {
			return nil, fmt.Errorf("target %q: %w", t.Name, err)
		}
		elfByEETarget[t.Name] = elfPath

		output := elfPath
		if t.OutputBin {
			binPath := strings.TrimSuffix(elfPath, ".elf") + ".bin"
			g.Objcopies = append(g.Objcopies, ObjcopyStep{
				TargetName: t.Name,
				Toolchain:  eeObjcopy,
				Input:      elfPath,
				Output:     binPath,
			})
			output = binPath
		}
		if t.Pack {
			packedPath := strings.TrimSuffix(elfPath, ".elf") + "-packed.elf"
			g.Packs = append(g.Packs, PackStep{
				TargetName: t.Name,
				Toolchain:  ps2packTool,
				Input:      elfPath,
				Output:     packedPath,
			})
			output = packedPath
		}
		g.TargetOutputs[t.Name] = output
	}

	return g, nil
}

func expandSources(projectRoot string, sources []string) ([]string, error) {
	var out []string
	for _, s := range sources {
		if !strings.ContainsAny(s, "*?[") {
			out = append(out, s)
			continue
		}
		pattern := s
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(projectRoot, pattern)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("sources: invalid glob %q: %w", s, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("sources: glob %q matched no files", s)
		}
		for _, m := range matches {
			rel, err := filepath.Rel(projectRoot, m)
			if err != nil {
				return nil, fmt.Errorf("sources: resolving glob match %q: %w", m, err)
			}
			out = append(out, filepath.ToSlash(rel))
		}
	}
	return out, nil
}

func buildIOPTarget(g *Graph, t config.Target, projectRoot string, libsByName map[string]libInfo) (string, error) {
	objDir := utils.Join("obj", t.Name)

	libs := append(implicitStartupLibs(libsByName, "iop"), t.Libs...)
	expandedLibs, err := resolveLinkOrder(libs, libsByName, "iop")
	if err != nil {
		return "", err
	}

	localLibDirs, libDeps, externalLibDirs, externalLibDeps, libIncludeDirs, _ := resolveLibDeps(expandedLibs, libsByName)
	pkgDirs, pkgArchives, pkgIncludeDirs, pkgDeprecations, _, err := ResolvePackageLibs(expandedLibs, libsByName, "iop", t.Name)
	if err != nil {
		return "", err
	}
	pkgDirs = append(pkgDirs, externalLibDirs...)
	externalLibDeps = append(externalLibDeps, pkgArchives...)
	pkgIncludeDirs = append(libIncludeDirs, pkgIncludeDirs...)
	g.DeprecationWarnings = append(g.DeprecationWarnings, pkgDeprecations...)
	g.DeprecationWarnings = append(g.DeprecationWarnings, unresolvedLibWarnings(expandedLibs, t.LDFlags, libsByName, t.Name)...)

	headerIncludeDirs, headerDeprecations, err := resolveHeaderIncludeDirs(t.Headers, libsByName, "iop", t.Name)
	if err != nil {
		return "", err
	}
	pkgIncludeDirs = append(pkgIncludeDirs, headerIncludeDirs...)
	g.DeprecationWarnings = append(g.DeprecationWarnings, headerDeprecations...)

	var objects []string

	// exports.o must link first - the IOP loader expects to find the
	// real export table before any other data that might resemble one.
	if t.ExportsTab != "" {
		obj, err := addIETableStub(g, t, projectRoot, t.ExportsTab, "irx.h", "exports", pkgIncludeDirs)
		if err != nil {
			return "", err
		}
		objects = append(objects, obj)
	}

	for _, src := range t.Sources {
		obj, err := compileSource(g, t, projectRoot, src, objDir, iopCompileFlags(g, t, projectRoot, pkgIncludeDirs))
		if err != nil {
			return "", err
		}
		objects = append(objects, obj)
	}

	if t.ImportsLst != "" {
		obj, err := addIETableStub(g, t, projectRoot, t.ImportsLst, "irx_imports.h", "imports", pkgIncludeDirs)
		if err != nil {
			return "", err
		}
		objects = append(objects, obj)
	}

	linkfile := resolveLinkfile(g, t, projectRoot)
	linkedELF := utils.Join(objDir, t.Name+".notiopmod.elf")
	g.Links = append(g.Links, LinkStep{
		TargetName:      t.Name,
		Toolchain:       iopGCC,
		Linkfile:        linkfile,
		FlagArgs:        iopLinkFlags(g, t, pkgDirs),
		Objects:         objects,
		Libs:            g.Toolchain.MapLibs(expandedLibs, "iop"),
		LibDeps:         libDeps,
		LocalLibDirs:    localLibDirs,
		ExternalLibDeps: externalLibDeps,
		Output:          linkedELF,
	})

	strippedELF := utils.Join(objDir, t.Name+".notiopmod.stripped.elf")
	g.Strips = append(g.Strips, StripStep{
		TargetName: t.Name,
		Toolchain:  iopStrip,
		FlagArgs:   iopStripFlags(),
		Input:      linkedELF,
		Output:     strippedELF,
	})

	irxPath := utils.Join("bin", t.Name+".irx")
	g.Fixups = append(g.Fixups, FixupStep{
		TargetName: t.Name,
		FlagArgs:   g.Toolchain.IOPFixupFlags(t.ExportsTab != ""),
		Input:      strippedELF,
		Output:     irxPath,
	})

	return irxPath, nil
}

func addIETableStub(g *Graph, t config.Target, projectRoot, sourceFile, header, kind string, pkgIncludeDirs []string) (string, error) {
	absSrc := utils.AbsPath(projectRoot, sourceFile)
	cPath := NewGeneratedPath(utils.Join(g.OutDir, "generated", t.Name+"_"+kind+".c"))

	g.IETableStubs = append(g.IETableStubs, IETableStubStep{
		TargetName: t.Name,
		SourceFile: absSrc,
		Header:     header,
		COutPath:   cPath,
	})

	flags := append(append([]string{}, iopCompileFlags(g, t, projectRoot, pkgIncludeDirs)...), g.Toolchain.IETableStubFlags()...)
	objPath := utils.Join("obj", t.Name, kind+".o")
	g.Compiles = append(g.Compiles, CompileStep{
		TargetName: t.Name,
		Toolchain:  iopGCC,
		FlagArgs:   flags,
		Source:     cPath.String(),
		Object:     objPath,
	})

	return objPath, nil
}

func buildEETarget(g *Graph, t config.Target, projectRoot string, irxByIOPTarget, elfByEETarget, embedObjByTarget map[string]string, libsByName map[string]libInfo) (string, error) {
	objDir := utils.Join("obj", t.Name)

	libs := append(implicitStartupLibs(libsByName, "ee"), t.Libs...)
	expandedLibs, err := resolveLinkOrder(libs, libsByName, "ee")
	if err != nil {
		return "", err
	}

	localLibDirs, libDeps, externalLibDirs, externalLibDeps, libIncludeDirs, libsNeedCXX := resolveLibDeps(expandedLibs, libsByName)
	pkgDirs, pkgArchives, pkgIncludeDirs, pkgDeprecations, pkgNeedsCXX, err := ResolvePackageLibs(expandedLibs, libsByName, "ee", t.Name)
	if err != nil {
		return "", err
	}
	pkgDirs = append(pkgDirs, externalLibDirs...)
	externalLibDeps = append(externalLibDeps, pkgArchives...)
	pkgIncludeDirs = append(libIncludeDirs, pkgIncludeDirs...)
	g.DeprecationWarnings = append(g.DeprecationWarnings, pkgDeprecations...)
	g.DeprecationWarnings = append(g.DeprecationWarnings, unresolvedLibWarnings(expandedLibs, t.LDFlags, libsByName, t.Name)...)
	libsNeedCXX = libsNeedCXX || pkgNeedsCXX

	headerIncludeDirs, headerDeprecations, err := resolveHeaderIncludeDirs(implicitKernelHeaders(t.Headers, libsByName), libsByName, "ee", t.Name)
	if err != nil {
		return "", err
	}
	pkgIncludeDirs = append(pkgIncludeDirs, headerIncludeDirs...)
	g.DeprecationWarnings = append(g.DeprecationWarnings, headerDeprecations...)
	if len(t.EmbedIRX) > 0 {
		pkgIncludeDirs = append(pkgIncludeDirs, utils.Join(g.OutDir, "generated"))
	}

	var objects []string
	for _, src := range t.Sources {
		obj, err := compileSource(g, t, projectRoot, src, objDir, eeCompileFlags(g, t, projectRoot, pkgIncludeDirs))
		if err != nil {
			return "", err
		}
		objects = append(objects, obj)
	}

	for _, entry := range t.EmbedIRX {
		resolveName, symbolName := config.ParseEmbedIRXEntry(entry)
		obj, ok := embedObjByTarget[symbolName]
		if !ok {
			binPath, ok := irxByIOPTarget[resolveName]
			if !ok {
				binPath, ok = elfByEETarget[resolveName]
			}
			if !ok {
				pkgIRX, found, deprecated, findErr := resolvePackageDriverIRX(resolveName, "iop")
				if findErr != nil {
					return "", findErr
				}
				if found {
					binPath = pkgIRX
					if deprecated != "" {
						g.DeprecationWarnings = append(g.DeprecationWarnings, fmt.Sprintf(
							"%s: package %q (driver) is deprecated: %s", t.Name, resolveName, deprecated))
					}
				}
			}
			var err error
			obj, err = addEmbedIRX(g, symbolName, binPath)
			if err != nil {
				return "", err
			}
			embedObjByTarget[symbolName] = obj
		}
		objects = append(objects, obj)
	}

	// Plain gcc doesn't add -lstdc++ the way g++ does.
	linkToolchain := eeGCC
	if hasCXXSource(t.Sources) || libsNeedCXX {
		linkToolchain = eeGXX
	}

	linkfile := resolveLinkfile(g, t, projectRoot)
	elfPath := utils.Join("bin", t.Name+".elf")

	linkOutput := elfPath
	if !t.Debug {
		linkOutput = utils.Join(objDir, t.Name+".unstripped.elf")
	}
	g.Links = append(g.Links, LinkStep{
		TargetName:      t.Name,
		Toolchain:       linkToolchain,
		Linkfile:        linkfile,
		FlagArgs:        eeLinkFlags(g, t, pkgDirs, linkToolchain == eeGXX),
		Objects:         objects,
		Libs:            g.Toolchain.MapLibs(expandedLibs, "ee"),
		LibDeps:         libDeps,
		LocalLibDirs:    localLibDirs,
		ExternalLibDeps: externalLibDeps,
		Output:          linkOutput,
		NoDefaultLibs:   hasLDFlag(t.LDFlags, "-nostdlib") || hasLDFlag(t.LDFlags, "-nodefaultlibs"),
	})

	if !t.Debug {
		g.Strips = append(g.Strips, StripStep{
			TargetName: t.Name,
			Toolchain:  eeStrip,
			FlagArgs:   eeStripFlags(),
			Input:      linkOutput,
			Output:     elfPath,
		})
	}

	return elfPath, nil
}

func buildLibTarget(g *Graph, t config.Target, projectRoot string, libsByName map[string]libInfo) (string, error) {
	objDir := utils.Join("obj", t.Name)

	var compileFlags []string
	var arToolchain string
	switch t.Type {
	case config.TargetEELib:
		headerIncludeDirs, headerDeprecations, err := resolveHeaderIncludeDirs(t.Headers, libsByName, "ee", t.Name)
		if err != nil {
			return "", err
		}
		g.DeprecationWarnings = append(g.DeprecationWarnings, headerDeprecations...)
		compileFlags = eeCompileFlags(g, t, projectRoot, headerIncludeDirs)
		arToolchain = eeAR
	case config.TargetIOPLib:
		headerIncludeDirs, headerDeprecations, err := resolveHeaderIncludeDirs(t.Headers, libsByName, "iop", t.Name)
		if err != nil {
			return "", err
		}
		g.DeprecationWarnings = append(g.DeprecationWarnings, headerDeprecations...)
		compileFlags = iopCompileFlags(g, t, projectRoot, headerIncludeDirs)
		arToolchain = iopAR
	default:
		panic(fmt.Sprintf("buildLibTarget: not a lib target type %q", t.Type))
	}

	var objects []string
	for _, src := range t.Sources {
		obj, err := compileSource(g, t, projectRoot, src, objDir, compileFlags)
		if err != nil {
			return "", err
		}
		objects = append(objects, obj)
	}

	archivePath := utils.Join("lib", "lib"+t.Name+".a")
	g.Archives = append(g.Archives, ArchiveStep{
		TargetName: t.Name,
		Toolchain:  arToolchain,
		Objects:    objects,
		Output:     archivePath,
	})

	return archivePath, nil
}

func hasCXXSource(sources []string) bool {
	for _, s := range sources {
		switch filepath.Ext(s) {
		case ".cpp", ".cc", ".cxx":
			return true
		}
	}
	return false
}
