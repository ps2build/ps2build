# Architecture

ps2build generates `build.ninja` and `compile_commands.json` from a `ps2.yaml` definition for PlayStation 2 Emotion Engine (EE) and IOP targets built with the PS2 SDK.

Supported target types: `ee`, `iop`, `ee_lib`, `iop_lib`. Library targets are archived into `lib<name>.a` and never linked on their own; `ee` and `iop` targets link any number of library targets or external libraries named in `libs`. `.vsm` and `.vcl` VU sources are ordinary `sources:` entries on an `ee` target.

## Commands

```
go build ./...
go vet ./...
gofmt -l .
go test ./...
UPDATE_GOLDEN=1 go test ./internal/generate/...
```

`UPDATE_GOLDEN=1` regenerates golden files after an intentional output change; review the diff before committing.

## Packages

| Package | Purpose |
|---|---|
| `cmd/ps2build` | Entrypoint; delegates to `internal/cli`. |
| `internal/cli` | Cobra command tree (`generate`, `build`, `install`, `update`, `version`, `init`, `schema`, `explain`, `mcp`). Flag parsing only. |
| `internal/update` | Updates an installed ps2dev suite one component at a time, or bootstraps one. Components release independently. Staging lives inside `$PS2DEV` so the swap is a same-volume rename. ps2build is the only self-replacing component. |
| `internal/versions` | Reads and writes `versions.yaml`, the per-component version manifest. |
| `internal/buildrun` | Locates and runs `ninja` against a generated build file. |
| `internal/install` | Copies `package.yaml`-declared artifacts into `$PS2DEV/packages/<tier>/<name>`. |
| `internal/mcpserver` | MCP server (stdio, or streamable HTTP with `--listen`) exposing package queries, schema, validation, build and install. Only `build_project` and `install_project` have side effects. |
| `internal/config` | `ps2.yaml` model: load, defaults, validation, environment resolution. |
| `internal/buildgraph` | Single source of truth for toolchain invocation: flags, ordering, dependency resolution. |
| `internal/ninja` | `build.ninja` writer. Consumes `buildgraph.Graph`. |
| `internal/compiledb` | `compile_commands.json` and `.clangd` emitters. |
| `internal/generate` | Orchestrates load, validate, resolve, build graph, emit. |
| `internal/schema` | JSON Schema reflected from `config.Config`. |
| `internal/explain` | Embedded documentation and example projects. |

## Flag assembly

`internal/buildgraph/flags.go` is the only place that assembles compiler and linker flags. `internal/ninja` and `internal/compiledb` read the resulting graph verbatim. A new flag or default belongs in `flags.go`.

## Toolchains

Two toolchains are supported: `gcc` (`$PS2DEV/toolchain`) and `llvm` (`$PS2DEV/llvm-toolchain`). Differences sit behind the `buildgraph.Toolchain` interface: flag assembly, tool paths, link driver, default libraries, linkfile. New toolchain-specific behavior is a new interface method implemented by both.

Selection precedence: `--toolchain`, `PS2BUILD_TOOLCHAIN`, `toolchain:` in `ps2.yaml` or `ps2workspace.yaml`, then `gcc`. `PS2BUILD_TOOLCHAIN_ROOT` points the llvm toolchain at a development tree. Each fixture has `golden/` (gcc) and `golden-llvm/` (llvm) outputs.

## Tool resolution

Compilers and host tools resolve to absolute paths under `$PS2DEV/{ee,iop}/bin` and are stored in `Graph.ToolchainPaths`; the bare name is used only when nothing is found on disk. Generated builds therefore need no `PATH` setup, and `compile_commands.json` consumers do not depend on shell state.

`ps2build build` looks for `ninja` in `tools/` beside the executable, then beside the executable, then on `PATH`. `tools/` is never added to `PATH`, so a bundled ninja does not shadow a system one.

`PS2SDK` defaults to `$PS2DEV/ps2sdk`; `PS2DEV` is the only required variable.

## VU sources

`.vsm` sources (`ee` targets only) assemble with `dvp-as` from `$PS2DEV/tools`. `dvp-as` is a standalone assembler: `compileSource` drops the C compile flags and emits `dvp-as $in -o $out`, with no depfile. `compilerFor` in `internal/buildgraph/embed.go` is the single dispatch point for source type.

`.vcl` sources (`ee` targets only) compile with `vcl` from `$PS2DEV/vcl`. Only `-I`, `-D` and `-U` options are forwarded. The rule uses a gcc-style depfile so `.include` edits trigger rebuilds.

Both keep their extension in the object name (`src/foo.vsm` becomes `obj/<target>/src/foo.vsm.o`) so a `.vsm` and a `.c` sharing a basename do not collide.

## Generated `.clangd`

clangd cannot discover a cross compiler's implicit system include directories without `--query-driver`. `internal/compiledb/isystem.go` runs the resolved compiler once (`-E -x c++ -v -`) and adds the reported directories as `-isystem` flags. `ClangdConfig` strips PS2-only flags and adds an `_EE` or `_IOP` fallback define for headers with no compile entry.

`generate.Run` writes `.clangd` only when `Options.WriteClangdConfig` is set, so golden tests do not modify fixtures. The CLI sets it on every `generate`, `build` and `init`.

## Path handling

1. Paths stored on a `buildgraph` step are forward-slash on every OS. `generate.Run` normalizes `projectRoot` and `outDir` before building the graph. `internal/compiledb` uses the `path` package, not `path/filepath`.
2. `ninja.Writer.Rule` writes its `command` template verbatim; it contains intentional single-`$` ninja variables. Escaping applies only to `Build` variable values and path tokens.
3. A bare `:` in a build-line path token is escaped as `$:`; a `:` inside a variable value is not. `escape` and `escapePath` in `internal/ninja/writer.go` are separate for this reason.
4. A file that is both a step output and a compile input must be spelled identically in both places, because ninja matches nodes by path string. `Bin2CStep.COutPath` is absolute for this reason; `TestBuild_EmbedIRXGeneratedFilePathsMatch` covers it.

## Static library link order

`libFlags` in `internal/ninja/emit.go` preserves the `libs` order from `ps2.yaml`. A static linker resolves symbols in one left-to-right pass, so that order is the dependency order.

## Extending

Adding a target type:

1. Add a `TargetType` constant in `internal/config/config.go`.
2. Extend `config.Target` and `internal/config/validate.go`.
3. Add step generation in `internal/buildgraph` (`build.go`, `resolve.go`, `flags.go`).
4. Add ninja rules in `internal/ninja/emit.go`.
5. Add a fixture under `internal/explain/testdata/fixtures/<name>/` and regenerate golden files.

Schema and `explain` output update automatically.

Adding a `ps2.yaml` field:

1. Add it to `config.Target` or `Config` with `yaml` and `jsonschema:"description=..."` tags.
2. Wire it into `internal/buildgraph/flags.go`.
3. Add or update a fixture and regenerate golden files.

## Testing

- `config`, `buildgraph`, `ninja`, `compiledb`: table-driven unit tests.
- `generate`: golden-file tests over each fixture, with absolute prefixes redacted to `<PROJECT_ROOT>` and `<OUT_DIR>`.
- Tests do not run the real toolchain or ninja. Golden tests compare text only, so changes to `internal/ninja` or the `embed_irx` path also need a real `ps2build generate && ninja` run against an installed SDK. CI does this in the `smoke-test-real-toolchain` job.

Fixtures live under `internal/explain/testdata/` because `go:embed` cannot reference paths outside the embedding package.
