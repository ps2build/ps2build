package explain

import (
	_ "embed"
)

//go:embed testdata/fixtures/simple-ee/ps2.yaml
var exampleSimpleEE string

//go:embed testdata/fixtures/simple-iop/ps2.yaml
var exampleSimpleIOP string

//go:embed testdata/fixtures/embed-irx/ps2.yaml
var exampleEmbedIRX string

//go:embed testdata/fixtures/overrides/ps2.yaml
var exampleOverrides string

//go:embed testdata/fixtures/static-lib/ps2.yaml
var exampleStaticLib string

//go:embed testdata/fixtures/cxx-lib/ps2.yaml
var exampleCXXLib string

//go:embed testdata/fixtures/vu-microcode/ps2.yaml
var exampleVUMicrocode string

var examples = []struct {
	Name        string
	Description string
	YAML        string
}{
	{"simple-ee", "Minimal single EE target.", exampleSimpleEE},
	{"simple-iop", "Minimal single IOP module target.", exampleSimpleIOP},
	{"embed-irx", "An EE target embedding a built IOP module via bin2c (embed_irx).", exampleEmbedIRX},
	{"overrides", "An EE target using every optional override field.", exampleOverrides},
	{"static-lib", "An ee_lib target built as a static library and linked into an ee target via libs.", exampleStaticLib},
	{"cxx-lib", "A plain-C ee target linking a C++ ee_lib - ps2build automatically switches its link driver to g++ since it needs libstdc++.", exampleCXXLib},
	{"vu-microcode", "An EE target with a VU microprogram source (.vsm), assembled with dvp-as and linked directly into the EE binary.", exampleVUMicrocode},
}

const overview = `ps2build reads a ps2.yaml build definition and builds Sony PlayStation 2
Emotion Engine (EE) and IOP targets with the ps2toolchain or
LLVM cross-compilers.

Typical workflow:
  1. Write ps2.yaml describing your targets (or "ps2build init" to
     scaffold one).
  2. Run "ps2build build" - generates build.ninja + compile_commands.json
     in the output directory (default: ./build), then invokes ninja.
     Requires a real ps2dev-suite install; toolchain binaries resolve to
     absolute paths automatically, no PATH setup needed.

A ps2workspace.yaml builds several projects (each its own directory with
a ps2.yaml, listed under members:) into one shared build.ninja and
compile_commands.json, so targets in different members can link against
each other. "ps2build build"/"generate" detect it automatically by
filename.

"ps2build generate" only regenerates build.ninja/compile_commands.json
without compiling. "ps2build build" always regenerates first, so it's
the only command guaranteed to pick up a newly-added file matching an
existing glob in sources.`

const targetTypes = `Every target has type: ee, iop, ee_lib, or iop_lib.

sources accepts glob patterns (*, ?, [) alongside literal paths, e.g.
sources: [src/*.c, src/*.cpp] - matched against the filesystem at
generate/build time, sorted for deterministic output. A glob matching
zero files is an error. No recursive ** matching.

EE targets (type: ee) produce a bootable .elf:
  - Compiled with mips64r5900el-ps2-elf-gcc (or -g++ for .cpp sources).
  - Linked against the installed core packages with the startup package's
    linker script (override with linkfile).
  - kernel/cglue/cdvd link automatically - only list extra libraries in
    libs (e.g. libs: [png, m]).
  - output_bin: true also runs "objcopy -O binary", producing a flat .bin.
  - pack: true also runs "ps2pack" on the linked (stripped, unless
    debug: true) .elf, producing a compressed, self-extracting
    <name>-packed.elf. Independent of output_bin - a target can set
    either, both, or neither.
  - .vsm sources (VU0/VU1 microprogram assembly) are assembled with
    dvp-as into a plain relocatable object and linked into the EE binary
    alongside its other objects - no separate step or field needed, just
    list the .vsm file in sources like any other. Symbols declared
    .global in the .vsm (e.g. a microprogram's start/end labels) come out
    as ordinary linked symbols, referenceable from C/C++ as
    extern unsigned char. dvp-as ignores this target's C compile flags
    (-D_EE, -I, ...) - it's an assembler, not a C compiler. Only valid on
    ee targets, not ee_lib (never itself linked). Unlike .c/.cpp/.s/.S, its
    object keeps the .vsm extension (src/foo.vsm -> obj/<target>/src/foo.vsm.o)
    so a VU microprogram can share a basename with the ordinary .c source that
    submits it - a common pattern - without both collapsing onto the same
    ninja output node. See the vu-microcode example.

IOP targets (type: iop) produce a relocatable .irx module:
  - Compiled with mipsel-none-elf-gcc, with -D_IOP -fno-builtin and the
    soft-float/no-explicit-relocs flags modern IOP gcc needs.
  - "Linked" specially (-nostdlib -dc -r), then stripped, then passed
    through the "iopfixup" host tool to produce the final .irx.
  - imports_lst/exports_tab generate and compile an
    imports.lst/exports.tab wrapper with -fno-toplevel-reorder; a target
    with exports_tab also links it first and drops iopfixup's default
    --allow-zero-text flag.

Embedding an IOP module into an EE binary (embed_irx):
  An EE target lists other iop (or ee) targets by name in embed_irx. For
  each one, ps2build runs the "bin2c" host tool on the target's
  built .irx/.elf, compiles the result, and links it into the EE binary:

    #include "<name>_irx.h"
    SifExecModuleBuffer(<name>_irx, size_<name>_irx, 0, NULL, NULL);

  The header declares both symbols (for "name:symbol" entries, the
  header is <symbol>_irx.h) and its directory is already on the include
  path of the EE target.

  See the embed-irx example. Two EE targets embedding the same module
  share one build/embed, not two.

Static libraries (type: ee_lib / type: iop_lib):
  Compiles like an ee/iop target and archives the objects into
  lib<name>.a with the matching "ar" tool - never itself linked, so
  ldflags/linkfile/libs/embed_irx/output_bin/pack aren't valid on it.
  An ee target links an ee_lib (and an iop target an iop_lib) by listing
  its target name in libs, alongside ordinary external library names -
  order is preserved as written, since static link order matters. See
  the static-lib example. A lib target can't itself list libs.

  An ee target's link automatically switches to the C++ driver (g++)
  whenever C++ runtime support is needed - its own sources, or a linked
  ee_lib's sources, include a .cpp/.cc/.cxx file. See the cxx-lib
  example.`

const cliCommands = `ps2build init [directory] [--name] [--package] [--workspace] [--force]
    Scaffolds a new project: ps2.yaml, a starter EE target, .gitignore,
    and .clangd. --package scaffolds a linkable ee_lib instead of a
    bootable executable; --workspace scaffolds ps2workspace.yaml instead.

ps2build build [-c ps2.yaml] [-o build]
    Runs generate (below), then invokes ninja.

ps2build generate [-c ps2.yaml] [-o build]
    Writes <out-dir>/build.ninja and <out-dir>/compile_commands.json
    without compiling. "ps2build build" is the normal path and the only
    one guaranteed to re-evaluate glob sources before building.

ps2build install [-c ps2.yaml] [-o build] [--packages DIR]
    Builds, then copies each package's declared artifacts into
    $PS2DEV/packages.

ps2build update [component] [--version V] [--check] [--force]
    Updates an installed suite in place, or bootstraps one from nothing.

ps2build schema [--indent N]
    Prints a JSON Schema (draft 2020-12) for the ps2.yaml format.

ps2build explain [--json] [--section NAME]
    Prints this documentation. --json for a structured document intended
    for LLM agents. --section filters to one section (overview,
    target_types, cli_commands, env_vars, v1_scope, ps2_yaml_format,
    examples).

ps2build mcp [--listen ADDR]
    Runs an MCP server exposing package lookup, docs, validation, and
    building/installing as tools. stdio by default; --listen serves
    streamable HTTP instead.

ps2build version
    Prints ps2build's version and the installed suite's component
    versions.

ps2build --version
    Prints just the ps2build version.`

const envVars = `The SDK root is always the directory containing the running ps2build
executable - true for any real ps2dev-suite install (that's what
"ps2build update" produces). There is no flag or environment variable
to override it.

Every toolchain binary (mips64r5900el-ps2-elf-gcc, mipsel-none-elf-gcc,
dvp-as, bin2c, iopfixup, ninja, ps2pack) and every installed package's
headers/libraries resolve to absolute paths under the SDK root at
generate time - the generated build.ninja needs nothing on PATH to build.`

const v1Scope = `Explicitly NOT supported:
  - A dedicated dvp/vu target type - only ee, iop, ee_lib, and iop_lib
    target types exist. (.vsm VU microprogram sources ARE supported within
    ee targets - see target_types.)
  - A lib target (ee_lib/iop_lib) linking against other libs - only ee/iop
    targets link, against any number of libs each.

These may be added later; ps2.yaml files that don't use them are expected
to keep working unchanged.`
