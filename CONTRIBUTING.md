# Contributing

## Workflow

1. Open an issue for any change that adds a feature or alters `ps2.yaml` behavior.
2. Fork, branch from `master`, and keep each pull request to one change.
3. Run `gofmt -l .`, `go vet ./...` and `go test ./...` before pushing; CI runs the same checks plus a build against a real SDK.
4. If output changes intentionally, regenerate golden files with `UPDATE_GOLDEN=1 go test ./internal/generate/...` and review the diff.
5. If `go.mod` changes, run `.github/scripts/generate-licenses.sh` and commit `licenses/`.

## Code style

- Comments are single lines and only for tool, compiler or linker behavior that is not obvious from the code. Tests are documented by their names.
- CLI and MCP help text states what a command accomplishes, not how it works internally.
- Flag assembly belongs in `internal/buildgraph/flags.go`. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## AI-assisted contributions

AI-generated code is accepted. Contributions that the author cannot explain without AI assistance are discouraged and may be closed.

- State in the pull request which AI tool and model were used, or that none was.
- Write the pull request description yourself, without AI.
- You are responsible for every line you submit: be prepared to explain and defend each change in review.

## License

By submitting a contribution, you agree that it is licensed under the same license as this repository (see [LICENSE](LICENSE)).
