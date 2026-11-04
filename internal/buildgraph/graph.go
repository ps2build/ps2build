package buildgraph

import (
	"fmt"
	"path/filepath"

	"github.com/ps2build/ps2build/internal/utils"
)

// GeneratedPath is always absolute: ninja matches nodes by exact path string, not file identity.
type GeneratedPath string

type CompileStep struct {
	TargetName string
	Toolchain  string
	FlagArgs   []string
	Source     string
	Object     string
	Assembler  bool
	// VCL marks a .vcl step: the vcl driver takes -I/-D, writes a depfile, and is not a gcc frontend.
	VCL bool
}

type LinkStep struct {
	TargetName      string
	Toolchain       string
	Linkfile        string
	FlagArgs        []string
	Objects         []string
	Libs            []string
	LibDeps         []string
	LocalLibDirs    []string
	ExternalLibDeps []string
	Output          string
	// NoDefaultLibs is set by -nostdlib or -nodefaultlibs in the target's ldflags.
	NoDefaultLibs bool
}

type StripStep struct {
	TargetName string
	Toolchain  string
	FlagArgs   []string
	Input      string
	Output     string
}

type FixupStep struct {
	TargetName string
	FlagArgs   []string
	Input      string
	Output     string
}

type Bin2CStep struct {
	IOPTargetName string
	IRXPath       string
	COutPath      GeneratedPath
	HeaderPath    GeneratedPath
	VarName       string
}

type IETableStubStep struct {
	TargetName string
	SourceFile string
	Header     string
	COutPath   GeneratedPath
}

type ObjcopyStep struct {
	TargetName string
	Toolchain  string
	Input      string
	Output     string
}

type PackStep struct {
	TargetName string
	Toolchain  string
	Input      string
	Output     string
}

type ArchiveStep struct {
	TargetName string
	Toolchain  string
	Objects    []string
	Output     string
}

type Graph struct {
	ProjectRoot string
	OutDir      string

	Compiles     []CompileStep
	Links        []LinkStep
	Strips       []StripStep
	Fixups       []FixupStep
	Bin2Cs       []Bin2CStep
	Objcopies    []ObjcopyStep
	Packs        []PackStep
	Archives     []ArchiveStep
	IETableStubs []IETableStubStep

	TargetOutputs map[string]string
	TargetOrder   []string

	Toolchain      Toolchain
	ToolchainPaths map[string]string

	DeprecationWarnings []string
}

func NewGeneratedPath(p string) GeneratedPath {
	abs, err := filepath.Abs(p)
	if err != nil {
		panic(fmt.Sprintf("buildgraph.NewGeneratedPath(%q): %v", p, err))
	}
	return GeneratedPath(filepath.ToSlash(abs))
}

func (p GeneratedPath) String() string { return string(p) }

// RebaseGraph prefixes this member's own output paths; ExternalLibDeps is skipped since it names another member's/package's file, not this one's.
func RebaseGraph(g *Graph, prefix string) {
	rebase := func(s string) string {
		return utils.Join(prefix, s)
	}
	rebaseAll := func(ss []string) {
		for i := range ss {
			ss[i] = rebase(ss[i])
		}
	}

	for i := range g.Compiles {
		g.Compiles[i].Object = rebase(g.Compiles[i].Object)
	}
	for i := range g.Links {
		rebaseAll(g.Links[i].Objects)
		rebaseAll(g.Links[i].LibDeps)
		rebaseAll(g.Links[i].LocalLibDirs)
		g.Links[i].Output = rebase(g.Links[i].Output)
	}
	for i := range g.Strips {
		g.Strips[i].Input = rebase(g.Strips[i].Input)
		g.Strips[i].Output = rebase(g.Strips[i].Output)
	}
	for i := range g.Fixups {
		g.Fixups[i].Input = rebase(g.Fixups[i].Input)
		g.Fixups[i].Output = rebase(g.Fixups[i].Output)
	}
	for i := range g.Bin2Cs {
		g.Bin2Cs[i].IRXPath = rebase(g.Bin2Cs[i].IRXPath)
	}
	for i := range g.Archives {
		rebaseAll(g.Archives[i].Objects)
		g.Archives[i].Output = rebase(g.Archives[i].Output)
	}
	for i := range g.Objcopies {
		g.Objcopies[i].Input = rebase(g.Objcopies[i].Input)
		g.Objcopies[i].Output = rebase(g.Objcopies[i].Output)
	}
	for i := range g.Packs {
		g.Packs[i].Input = rebase(g.Packs[i].Input)
		g.Packs[i].Output = rebase(g.Packs[i].Output)
	}
	for name, out := range g.TargetOutputs {
		g.TargetOutputs[name] = rebase(out)
	}
}
