package buildgraph

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ps2build/ps2build/internal/config"
	"github.com/ps2build/ps2build/internal/utils"
)

func addEmbedIRX(g *Graph, embedTargetName, binPath string) (string, error) {
	if binPath == "" {
		return "", fmt.Errorf("embed_irx: %q does not resolve to a built .irx/.elf (validate should have caught this)", embedTargetName)
	}

	varName := utils.SanitizeCIdent(embedTargetName) + "_irx"
	// ninja matches nodes by exact path string; the output and the compile input must spell cPath identically.
	cPath := NewGeneratedPath(utils.Join(g.OutDir, "generated", varName+".c"))

	g.Bin2Cs = append(g.Bin2Cs, Bin2CStep{
		IOPTargetName: embedTargetName,
		IRXPath:       binPath,
		COutPath:      cPath,
		HeaderPath:    NewGeneratedPath(utils.Join(g.OutDir, "generated", varName+".h")),
		VarName:       varName,
	})

	objPath := utils.Join("obj", "_embed", varName+".o")
	g.Compiles = append(g.Compiles, CompileStep{
		TargetName: embedTargetName,
		Toolchain:  eeGCC,
		FlagArgs:   append(append([]string{}, g.Toolchain.EECompileArgs()...), "-D_EE", "-G0"),
		Source:     cPath.String(),
		Object:     objPath,
	})

	return objPath, nil
}

func compileSource(g *Graph, t config.Target, projectRoot, src, objDir string, flags []string) (string, error) {
	absSrc := utils.AbsPath(projectRoot, src)

	ext := filepath.Ext(src)
	toolchain, extraFlags, err := compilerFor(t.Type, ext)
	if err != nil {
		return "", fmt.Errorf("source %q: %w", src, err)
	}

	relSrc := src
	if filepath.IsAbs(relSrc) {
		var err error
		relSrc, err = filepath.Rel(projectRoot, relSrc)
		if err != nil {
			return "", fmt.Errorf("source %q: could not make relative to project root: %w", src, err)
		}
	}
	isAssembler := toolchain == dvpAS
	compileFlags := append(append([]string{}, flags...), extraFlags...)
	compileFlags = append(compileFlags, g.Toolchain.CompileExtraFlags(ext)...)
	objBase := strings.TrimSuffix(relSrc, ext)
	isVCL := toolchain == vclTool
	if isVCL {
		compileFlags = vclFlags(flags)
		objBase = relSrc
	}
	if isAssembler {
		compileFlags = append([]string{}, extraFlags...)
		objBase = relSrc
	}
	objPath := utils.Join(objDir, objBase+".o")

	g.Compiles = append(g.Compiles, CompileStep{
		TargetName: t.Name,
		Toolchain:  toolchain,
		FlagArgs:   compileFlags,
		Source:     absSrc,
		Object:     objPath,
		Assembler:  isAssembler,
		VCL:        isVCL,
	})

	return objPath, nil
}

func compilerFor(t config.TargetType, ext string) (toolchain string, extraFlags []string, err error) {
	switch t {
	case config.TargetEE, config.TargetEELib:
		switch ext {
		case ".c":
			return eeGCC, nil, nil
		case ".cpp", ".cc", ".cxx":
			return eeGXX, nil, nil
		case ".s", ".S":
			return eeGCC, nil, nil // gcc dispatches to the assembler for .s/.S
		}
		if t == config.TargetEE && ext == ".vsm" {
			return dvpAS, nil, nil
		}
		if t == config.TargetEE && ext == ".vcl" {
			return vclTool, nil, nil
		}
	case config.TargetIOP, config.TargetIOPLib:
		switch ext {
		case ".c":
			return iopGCC, nil, nil
		case ".s", ".S":
			return iopGCC, nil, nil
		}
	}
	return "", nil, fmt.Errorf("unsupported source extension %q for target type %q", ext, t)
}

// vclFlags keeps the include and define options of a target's compile flags; the rest are gcc-only.
func vclFlags(flags []string) []string {
	var out []string
	for _, f := range flags {
		if strings.HasPrefix(f, "-I") || strings.HasPrefix(f, "-D") || strings.HasPrefix(f, "-U") {
			out = append(out, f)
		}
	}
	return out
}
