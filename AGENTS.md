# amux

**Generated:** 2026-07-23 · **Updated:** 2026-11-13

## Purpose
`amux` is a Go terminal UI that discovers AI coding agents running in tmux panes and Herdr sessions, plus supported headless daemon stores, groups them by project, and provides monitoring, notes, and tmux/Herdr actions.

## Key Files

| File | Description |
|------|-------------|
| `go.mod` | Module declaration, Go version, and direct dependencies. |
| `go.sum` | Dependency checksums. |
| `Makefile` | Build, install, test, vet, format, run, and clean targets. |
| `README.md` | User-facing behavior, installation, usage, and tmux integration documentation. |
| `.goreleaser.yaml` | GoReleaser distribution configuration. |
| `amux` | Locally built executable; regenerate with `make build`, do not edit. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `cmd/` | Cobra CLI entrypoint and subcommands (see `cmd/AGENTS.md`). |
| `internal/` | Non-public collection, Herdr access, notes, note generation, and Bubble Tea UI packages (see `internal/AGENTS.md`). |
| `.claude/` | Local Claude configuration and repository recovery tooling (see `.claude/AGENTS.md`). |

Git-ignored generated or operational directories are not documented and should not be hand-edited: `graft/` (Trail-Code repo map), `graphify-out/` (graphify knowledge graph), `.zvec-grep/` (search index), `.omc/`, `.omo/`, `.serena/`, `.ovis/`.

## For AI Agents

### Working In This Directory
- Keep the public command surface in `cmd/amux`; implementation belongs in the appropriate `internal` package.
- Treat the root `amux` executable as generated output; do not hand-edit it.
- Preserve platform support for macOS and Linux and the documented tmux version requirements.

### Testing Requirements
- Run `go test ./...` for behavior changes.
- Run `go vet ./...` when modifying Go packages.
- Use `make build` to verify the distributable binary builds with release linker flags.

### Common Patterns
- The project uses standard Go package layout and Go's internal-package boundary.
- Cobra defines commands; Bubble Tea and Lip Gloss implement the interactive UI.
- External process and filesystem observations are represented by `internal/collect` models before rendering.

## Dependencies

### Internal
- `cmd/amux` wires CLI commands to `internal/collect`, `internal/herdr`, `internal/notegen`, and `internal/ui`; `ui` owns its `internal/notes` integration.

### External
- `github.com/spf13/cobra` provides CLI parsing.
- `github.com/charmbracelet/bubbletea` and `github.com/charmbracelet/lipgloss` provide the TUI.
- Runtime tools on `PATH`: `tmux`, optionally `herdr` (0.9.5+ per README) and `codex` for note/title generation.

## Manual Notes

Notes under this heading are written by humans and preserved on regeneration.

### Build Installation
- After every successful build, always copy the built `./amux` to `/Users/ky-han/.local/bin/amux` as the final build step: `install -m 0755 ./amux /Users/ky-han/.local/bin/amux`.
- This also applies to verification builds that override `BUILD_INSTALL_PATH`; copying only to a temporary path does not complete the build.
