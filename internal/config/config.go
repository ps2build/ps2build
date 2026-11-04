package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	TargetEE     TargetType = "ee"
	TargetIOP    TargetType = "iop"
	TargetEELib  TargetType = "ee_lib"
	TargetIOPLib TargetType = "iop_lib"
)

type TargetType string

// ToolchainKind names a compiler toolchain.
type ToolchainKind string

const (
	ToolchainGCC  ToolchainKind = "gcc"
	ToolchainLLVM ToolchainKind = "llvm"
)

type Config struct {
	Project   string        `yaml:"project" jsonschema:"required,description=Project name, used for documentation purposes only"`
	PS2SDK    string        `yaml:"ps2sdk,omitempty" jsonschema:"description=Ignored. Kept so existing ps2.yaml files still load"`
	Toolchain ToolchainKind `yaml:"toolchain,omitempty" jsonschema:"enum=gcc,enum=llvm,description=Compiler toolchain to build with. The --toolchain flag and the PS2BUILD_TOOLCHAIN environment variable take precedence; the default is gcc"`
	Targets   []Target      `yaml:"targets" jsonschema:"required,minItems=1,description=The list of EE and IOP build targets in this project"`
}

type Target struct {
	Name string     `yaml:"name" jsonschema:"required,description=Unique target name. Used as the ninja phony alias and to reference this target from other targets' embed_irx/libs lists"`
	Type TargetType `yaml:"type" jsonschema:"required,enum=ee,enum=iop,enum=ee_lib,enum=iop_lib,description=ee builds a bootable Emotion Engine .elf; iop builds an IOP .irx module; ee_lib/iop_lib build a static library for other ee/iop targets to link against"`

	Sources []string `yaml:"sources" jsonschema:"required,minItems=1,description=Source file paths, relative to the ps2.yaml directory. .c/.cpp/.s/.S supported. .vsm (VU microprogram assembly) is also supported on ee targets - assembled with dvp-as and linked directly into the EE binary; .vcl (VU source for the vcl compiler) is also supported on ee targets - compiled with vcl, honoring the target's include dirs and defines, and linked the same way; unlike other extensions its object keeps the .vsm/.vcl suffix, so it can share a basename with the .c that submits it. An entry containing *, ?, or [ is matched as a glob (e.g. src/*.c) against the real filesystem at build/generate time - matches are sorted; a glob matching nothing is an error. No recursive ** matching. Every existing literal path keeps working unchanged"`

	IncludeDirs []string `yaml:"include_dirs,omitempty" jsonschema:"description=Extra -I include directories, relative to the ps2.yaml directory"`
	Defines     []string `yaml:"defines,omitempty" jsonschema:"description=Extra preprocessor defines, e.g. ['DEBUG=1', 'FOO']"`
	CFlags      []string `yaml:"cflags,omitempty" jsonschema:"description=Extra flags appended to the compile command"`
	LDFlags     []string `yaml:"ldflags,omitempty" jsonschema:"description=Extra flags appended to the link command. ee/iop targets only"`
	Libs        []string `yaml:"libs,omitempty" jsonschema:"description=Extra libraries to link, without the -l prefix (e.g. 'png' not '-lpng'). Also accepts the name of an ee_lib (for ee targets) or iop_lib (for iop targets) target defined in this same file - ps2build builds it and links it in automatically. EE targets already link the standard kernel/cglue/cdvd libraries by default via the compiler's own specs - only list additional libraries here. ee/iop targets only"`
	Headers     []string `yaml:"headers,omitempty" jsonschema:"description=Names of installed packages (under $PS2DEV/packages) whose include directories should reach this target's compile step - headers only, no -L/-l. Also accepts the name of an ee_lib/iop_lib target, whether defined in this same file or in another member of the same ps2workspace.yaml. Valid on every target type, including ee_lib/iop_lib, which can't use libs (a static library isn't linked) but can still need another package's headers to compile against"`

	Linkfile string `yaml:"linkfile,omitempty" jsonschema:"description=Override the default linker script. Defaults to the linker script shipped in the startup package. ee/iop targets only"`
	Opt      string `yaml:"opt,omitempty" jsonschema:"description=Override the optimization flag. Defaults to -O2 (ee) or -Os (iop)"`
	Debug    bool   `yaml:"debug,omitempty" jsonschema:"description=Add -gdwarf-2 -gz debug info flags to the compile command"`

	EmbedIRX  []string `yaml:"embed_irx,omitempty" jsonschema:"description=EE targets only. Names of iop/ee targets in this same file (or installed packages exposing a driver artifact) to embed as C byte arrays via bin2c and link into this EE binary. An entry may be 'name' or 'name:symbol' - the latter generates <symbol>_irx/size_<symbol>_irx instead of the default <name>_irx/size_<name>_irx, for matching hand-picked symbol names a ported application's own source already expects."`
	OutputBin bool     `yaml:"output_bin,omitempty" jsonschema:"description=EE targets only. Also run objcopy -O binary on the linked .elf to produce a flat .bin"`
	Pack      bool     `yaml:"pack,omitempty" jsonschema:"description=EE targets only. Also run ps2pack on the linked (stripped, unless debug:) .elf to produce a compressed, self-extracting <name>-packed.elf - the real deliverable most PS2 homebrew releases actually ship"`

	ImportsLst string `yaml:"imports_lst,omitempty" jsonschema:"description=IOP targets only. Path to an imports.lst file, relative to the ps2.yaml directory. Generates and compiles a #include \"irx_imports.h\" + imports.lst wrapper (so an existing module's imports.lst works unmodified) with -fno-toplevel-reorder, required for its DECLARE_IMPORT_TABLE macros to lay out correctly - your project must supply its own irx_imports.h (see the sdk-core iop modules for examples) aggregating whichever library headers imports.lst references, findable via include_dirs"`
	ExportsTab string `yaml:"exports_tab,omitempty" jsonschema:"description=IOP targets only. Path to an exports.tab file, relative to the ps2.yaml directory. Generates and compiles a #include \"irx.h\" + exports.tab wrapper the same way as imports_lst, links it first, and disables the default --allow-zero-text iopfixup flag since the module now has a real export table"`
}

func ParseEmbedIRXEntry(entry string) (resolveName, symbolName string) {
	if before, after, ok := strings.Cut(entry, ":"); ok {
		return before, after
	}
	return entry, entry
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &cfg, nil
}

// LoadValidated reads, validates, and applies defaults to the ps2.yaml at
// path in one call - the sequence every single-project caller needs.
func LoadValidated(path string) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	ApplyDefaults(cfg)
	return cfg, nil
}

func ApplyDefaults(cfg *Config) {
	for i := range cfg.Targets {
		t := &cfg.Targets[i]
		if t.Opt == "" {
			switch t.Type {
			case TargetEE, TargetEELib:
				t.Opt = "-O2"
			case TargetIOP, TargetIOPLib:
				t.Opt = "-Os"
			}
		}
	}
}
