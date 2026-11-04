package workspace

import (
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ps2build/ps2build/internal/buildgraph"
	"github.com/ps2build/ps2build/internal/config"
)

// Config is one ps2workspace.yaml.
type Config struct {
	Workspace string               `yaml:"workspace"`
	Toolchain config.ToolchainKind `yaml:"toolchain,omitempty"` // applies to every member; a member that sets its own must agree
	Members   []string             `yaml:"members"`             // paths relative to the workspace file's own directory, each containing a ps2.yaml
}

// Load parses one ps2workspace.yaml at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(c.Members) == 0 {
		return nil, fmt.Errorf("%s: members must not be empty", path)
	}
	return &c, nil
}

type member struct {
	relPath string // relative to the workspace root, forward-slash - also the prefix RebaseGraph uses
	root    string // this member's own absolute project root, forward-slash
	cfg     *config.Config
}

func Build(workspaceConfigPath string, outDir string, tc buildgraph.Toolchain) (*buildgraph.Graph, error) {
	wsCfg, err := Load(workspaceConfigPath)
	if err != nil {
		return nil, err
	}

	workspaceRootNative, err := filepath.Abs(filepath.Dir(workspaceConfigPath))
	if err != nil {
		return nil, fmt.Errorf("resolving workspace root: %w", err)
	}

	var members []member
	for _, m := range wsCfg.Members {
		memberRootNative := filepath.Join(workspaceRootNative, filepath.FromSlash(m))
		cfgPath := filepath.Join(memberRootNative, "ps2.yaml")

		cfg, err := config.Load(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("member %q: %w", m, err)
		}
		if err := config.Validate(cfg); err != nil {
			return nil, fmt.Errorf("member %q: %w", m, err)
		}
		if cfg.Toolchain != "" && cfg.Toolchain != tc.Kind() {
			return nil, fmt.Errorf("member %q: toolchain: %s conflicts with the workspace build using %s (a workspace builds every member with one toolchain)", m, cfg.Toolchain, tc.Kind())
		}
		config.ApplyDefaults(cfg)

		members = append(members, member{
			relPath: filepath.ToSlash(m),
			root:    filepath.ToSlash(memberRootNative),
			cfg:     cfg,
		})
	}

	externalLibs := make(map[string]buildgraph.ExternalLib)
	for _, m := range members {
		for _, t := range m.cfg.Targets {
			if t.Type != config.TargetEELib && t.Type != config.TargetIOPLib {
				continue
			}
			externalLibs[t.Name] = buildgraph.ExternalLib{
				Archive:     path.Join(m.relPath, "lib", "lib"+t.Name+".a"),
				NeedsCXX:    hasCXXSource(t.Sources),
				IncludeDirs: absIncludeDirs(m.root, t.IncludeDirs),
			}
		}
	}

	merged := &buildgraph.Graph{
		ProjectRoot:   workspaceRootNative,
		OutDir:        outDir,
		TargetOutputs: make(map[string]string),
		Toolchain:     tc,
	}
	declaredBy := make(map[string]string) // target name -> which member declared it
	for _, m := range members {
		memberOutDir := path.Join(outDir, m.relPath)

		g, err := buildgraph.Build(m.cfg, m.root, memberOutDir, externalLibs, tc)
		if err != nil {
			return nil, fmt.Errorf("member %q: %w", m.relPath, err)
		}
		buildgraph.RebaseGraph(g, m.relPath)

		for _, name := range g.TargetOrder {
			if prev, ok := declaredBy[name]; ok {
				return nil, fmt.Errorf("target %q is declared by both %q and %q - target names must be unique across the whole workspace", name, prev, m.relPath)
			}
			declaredBy[name] = m.relPath
		}

		merged.Compiles = append(merged.Compiles, g.Compiles...)
		merged.Links = append(merged.Links, g.Links...)
		merged.Strips = append(merged.Strips, g.Strips...)
		merged.Fixups = append(merged.Fixups, g.Fixups...)
		merged.Bin2Cs = append(merged.Bin2Cs, g.Bin2Cs...)
		merged.Objcopies = append(merged.Objcopies, g.Objcopies...)
		merged.Archives = append(merged.Archives, g.Archives...)
		merged.IETableStubs = append(merged.IETableStubs, g.IETableStubs...)
		merged.TargetOrder = append(merged.TargetOrder, g.TargetOrder...)
		merged.DeprecationWarnings = append(merged.DeprecationWarnings, g.DeprecationWarnings...)
		maps.Copy(merged.TargetOutputs, g.TargetOutputs)
		if merged.ToolchainPaths == nil {
			merged.ToolchainPaths = g.ToolchainPaths
		}
	}

	return merged, nil
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

func absIncludeDirs(memberRoot string, dirs []string) []string {
	if dirs == nil {
		return nil
	}
	out := make([]string, len(dirs))
	for i, d := range dirs {
		if filepath.IsAbs(d) {
			out[i] = filepath.ToSlash(d)
		} else {
			out[i] = filepath.ToSlash(filepath.Join(memberRoot, d))
		}
	}
	return out
}
