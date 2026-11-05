<p align="center"><img src="branding/ps2build-logo.svg" alt="PS2BUILD" width="480"></p>

# ps2build

ps2build builds PlayStation 2 homebrew. A single `ps2.yaml` describes your EE and IOP targets; ps2build produces the build, IDE metadata and installable packages, and keeps the PS2 SDK and toolchain up to date.

Documentation: [ps2build.dev](https://ps2build.dev).

## Install

Windows (PowerShell):

```
irm https://ps2build.dev/install.ps1 | iex
```

Linux and macOS:

```
curl -sSfL https://ps2build.dev/install.sh | sh
```

The installer downloads `ps2build`, verifies it, adds it to your `PATH` and runs `ps2build update` to install the toolchain and SDK. Its source and options are in [ps2build/ps2build.dev](https://github.com/ps2build/ps2build.dev).

### Manual install

1. Download the binary for your platform and `SHA256SUMS` from the [latest release](https://github.com/ps2build/ps2build/releases/latest):

   | Platform | Asset |
   |---|---|
   | Windows x64 | `ps2build-<version>-windows-amd64.exe` |
   | Linux x64 (glibc) | `ps2build-<version>-linux-amd64` |
   | macOS Intel | `ps2build-<version>-darwin-amd64` |
   | macOS Apple silicon | `ps2build-<version>-darwin-arm64` |

2. Check the download against the checksum file:

   ```
   sha256sum --check --ignore-missing SHA256SUMS                 # Linux
   shasum -a 256 --check --ignore-missing SHA256SUMS             # macOS
   (Get-FileHash <asset>).Hash -eq (Select-String <asset> SHA256SUMS).Line.Split(' ')[0]   # PowerShell
   ```

3. Rename it to `ps2build` (`ps2build.exe` on Windows), mark it executable on Linux and macOS (`chmod +x ps2build`), and put its folder on your `PATH`. The installer uses `%LOCALAPPDATA%\ps2build` on Windows and `~/.local/share/ps2build` elsewhere.

4. Install the toolchain and SDK:

   ```
   ps2build update
   ```

## Usage

```
ps2build init my-game      # scaffold a project
cd my-game
ps2build build             # build it
ps2build explain           # ps2.yaml reference and examples
ps2build schema            # JSON Schema for editor completion
```

| Command | Purpose |
|---|---|
| `init` | Scaffold a new project. |
| `build` | Build the project. |
| `generate` | Prepare a project for building without building it. |
| `install` | Build the project and install its packages into the SDK. |
| `update` | Install or update the SDK, toolchains and tools. |
| `validate` | Check generated compile commands for common problems. |
| `mcp` | Run an MCP server exposing installed packages and `ps2.yaml` tooling. |
| `version` | Print the ps2build and installed component versions. |

Target types are `ee`, `iop`, `ee_lib` and `iop_lib`. Both the GCC and LLVM toolchains are supported.

## Development

```
go build ./...
go test ./...
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [CONTRIBUTING.md](CONTRIBUTING.md).

## License

CC0 1.0 Universal. See [LICENSE](LICENSE). Third-party licenses are in [licenses/](licenses/).
