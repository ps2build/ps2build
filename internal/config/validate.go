package config

import (
	"fmt"
	"strings"
)

func Validate(cfg *Config) error {
	if strings.TrimSpace(cfg.Project) == "" {
		return fmt.Errorf("project: must not be empty")
	}
	switch cfg.Toolchain {
	case "", ToolchainGCC, ToolchainLLVM:
	default:
		return fmt.Errorf("toolchain: must be %q or %q, got %q", ToolchainGCC, ToolchainLLVM, cfg.Toolchain)
	}
	if len(cfg.Targets) == 0 {
		return fmt.Errorf("targets: at least one target is required")
	}

	byName := make(map[string]*Target, len(cfg.Targets))
	seen := make(map[string]bool, len(cfg.Targets))
	for i := range cfg.Targets {
		t := &cfg.Targets[i]
		if t.Name == "" {
			return fmt.Errorf("targets[%d]: name must not be empty", i)
		}
		if seen[t.Name] {
			return fmt.Errorf("targets: duplicate target name %q", t.Name)
		}
		seen[t.Name] = true
		byName[t.Name] = t

		switch t.Type {
		case TargetEE, TargetIOP, TargetEELib, TargetIOPLib:
		default:
			return fmt.Errorf("target %q: type must be one of %q, %q, %q, %q, got %q", t.Name, TargetEE, TargetIOP, TargetEELib, TargetIOPLib, t.Type)
		}

		if len(t.Sources) == 0 {
			return fmt.Errorf("target %q: sources must not be empty", t.Name)
		}

		if t.Type == TargetIOP {
			if len(t.EmbedIRX) > 0 {
				return fmt.Errorf("target %q: embed_irx is only valid on ee targets", t.Name)
			}
			if t.OutputBin {
				return fmt.Errorf("target %q: output_bin is only valid on ee targets", t.Name)
			}
			if t.Pack {
				return fmt.Errorf("target %q: pack is only valid on ee targets", t.Name)
			}
		}

		if t.Type != TargetIOP {
			if t.ImportsLst != "" {
				return fmt.Errorf("target %q: imports_lst is only valid on iop targets", t.Name)
			}
			if t.ExportsTab != "" {
				return fmt.Errorf("target %q: exports_tab is only valid on iop targets", t.Name)
			}
		}

		if t.Type == TargetEELib || t.Type == TargetIOPLib {
			if len(t.EmbedIRX) > 0 {
				return fmt.Errorf("target %q: embed_irx is only valid on ee targets", t.Name)
			}
			if t.OutputBin {
				return fmt.Errorf("target %q: output_bin is only valid on ee targets", t.Name)
			}
			if t.Pack {
				return fmt.Errorf("target %q: pack is only valid on ee targets", t.Name)
			}
			if len(t.Libs) > 0 {
				return fmt.Errorf("target %q: libs is not valid on %q targets - a library doesn't itself link against other libraries", t.Name, t.Type)
			}
			if len(t.LDFlags) > 0 {
				return fmt.Errorf("target %q: ldflags is not valid on %q targets - a library isn't linked, only archived", t.Name, t.Type)
			}
			if t.Linkfile != "" {
				return fmt.Errorf("target %q: linkfile is not valid on %q targets - a library isn't linked, only archived", t.Name, t.Type)
			}
		}
	}

	for i := range cfg.Targets {
		t := &cfg.Targets[i]
		for _, entry := range t.EmbedIRX {
			embedName, _ := ParseEmbedIRXEntry(entry)
			if embedded, ok := byName[embedName]; ok && embedded.Type != TargetIOP && embedded.Type != TargetEE {
				return fmt.Errorf("target %q: embed_irx references %q, which is type %q, not %q or %q", t.Name, embedName, embedded.Type, TargetIOP, TargetEE)
			}
		}

		var wantLibType TargetType
		switch t.Type {
		case TargetEE:
			wantLibType = TargetEELib
		case TargetIOP:
			wantLibType = TargetIOPLib
		default:
			continue
		}
		for _, libName := range t.Libs {
			lib, ok := byName[libName]
			if !ok {
				continue
			}
			if lib.Type != wantLibType {
				return fmt.Errorf("target %q: libs references %q, which is type %q, not %q", t.Name, libName, lib.Type, wantLibType)
			}
		}
	}

	return nil
}
