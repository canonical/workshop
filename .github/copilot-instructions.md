# GitHub Copilot Instructions

This file provides general project context for GitHub Copilot. For review-specific guidance, see:
- **Code Review**: `.github/agents/code-review.agent.md`
- **Documentation Review**: `.github/skills/documentation-review/SKILL.md`

## Project Overview

Workshop is a tool for defining and handling ephemeral development environments. It uses a client-server architecture where `workshopd` daemon exposes a RESTful API to clients. The project is written in Go and distributed as a Snap package.

## Tech Stack

- **Language**: Go 1.25+
- **Container Backend**: LXD (primary abstraction target)
- **Packaging**: Snap (Snapcraft)
- **CLI Framework**: Cobra
- **Testing**: Go unit tests (gocheck) + Spread (integration/e2e)
- **Documentation**: Sphinx (reStructuredText)
- **CI**: GitHub Actions

## Repository Structure

### Core Directories
- `cmd/` — CLI entry points (`workshop`, `workshopd`, `workshopctl`, `sdk`, `internal`)
- `client/` — Go client library for RESTful API communication
- `internal/` — Private packages (`daemon`, `overlord`, `workshop`, `interfaces`, `sdk`, etc.)
- `tests/` — E2E test suites using Spread (`main/`, `integration/`, `docs-*/`, `lib/`)
- `snap/` — Snap packaging configuration
- `docs/` — Sphinx documentation source and configuration (`Makefile`, `conf.py`, `contributing.rst`)

### Build Configuration
- `go.mod` / `go.sum` — Go module dependencies
- `.golangci.yaml` / `.golangci.incremental.yaml` — Linting configuration
- `.spread.yaml` — E2E testing with Spread framework
- `snap/snapcraft.yaml` — Snap package definition
- `docs/Makefile` — Sphinx documentation build script

## Coding Guidelines

See [`docs/coding-style-guide.md`](../docs/coding-style-guide.md) for detailed standards. Key points:

- **Error messages**: Lowercase, no trailing punctuation, actionable (`what was attempted: why it went wrong`)
- **Error handling**: Consistent `if err := f(); err != nil { return err }` pattern
- **Code organization**: Early returns over nested conditions; keep coupled elements adjacent
- **Testing**: Unit tests (`*_test.go`) adjacent to implementation; Spread tests for integration

## Common Tasks

### Running Tests
- **Unit Tests**: Run `go test ./...` in the root directory.
- **E2E Tests**: Use `spread` (requires external setup). Example: `spread tests/integration/`.

### Linting
- Run `golangci-lint run` to check for style and error handling issues.

### Building Documentation
- Navigate to `docs/` and run `make html` to build static HTML.
- Run `make run` in `docs/` to build and serve locally.

### Running Locally
- Quick path: `go tool try` — builds, starts `workshopd` against a temporary session, and drops into a pre-configured subshell. Exit to tear down; `--keep` retains the session.
- Manual: `go install ./cmd/...` then `workshopd run --create-dirs` (requires `WORKSHOP_DATA` and `WORKSHOP_CACHE` env vars set).

## Available Resources

- **Contributing Guide**: [`docs/contributing/development.rst`](../docs/contributing/development.rst) — Setup, testing, workflow
- **Documentation Style**: [`docs/doc-style-guide.md`](../docs/doc-style-guide.md) — reST/Markdown conventions
- **PR Template**: [`.github/pull_request_template.md`](.github/pull_request_template.md) — Self-review checklist
- **Code Review Agent**: [`.github/agents/code-review.agent.md`](.github/agents/code-review.agent.md) — For PR code reviews
- **Documentation Review Skill**: [`.github/skills/documentation-review/SKILL.md`](skills/documentation-review/SKILL.md) — For documentation reviews

## Related Repositories

These external repositories provide authoritative context for the Workshop project:

- https://github.com/canonical/sdkcraft — SDKcraft utility codebase for packaging and publishing SDKs
- Individual SDK implementations live as `canonical/<name>-sdk` repositories (e.g. `ollama-sdk`, `cuda-toolkit-sdk`, `zephyr-sdk`); see `docs/reference/reference-implementations.rst` for the curated themed list
- https://github.com/canonical/reference-workshops — Reference workshop implementations demonstrating SDK usage patterns

## GitHub Actions Workflows

- `lint.yaml` — golangci-lint on Go code
- `unit-tests.yaml` — Go unit tests
- `spread.yaml` — E2E tests with Spread
- `cover.yaml` — Coverage reports
- `automatic-doc-checks.yml` — Sphinx builds (fail on warnings)
- `doc-cover.yaml` — Documentation coverage map generation
- `doc-update-sdk-schema.yml` — Updates SDK schema in docs
- `fix-redirected-links.yml` — Updates selected redirecting documentation links
- `release.yaml` — Builds release snaps + generates CLI reference PR
- `fixup.yaml` — Commit message format validation
- `scanning.yml` — Security scanning
- `markdown-style-checks.yml` — Markdown linting
- `build-deps.yaml` — Build dependencies check
- `lxd-candidate-check.yaml` — Checks LXD candidate channel
- `staging.yaml` — Rejects staged test SDKs
- `update-sphinx-stack.yaml` — Updates Sphinx Stack files and documentation dependencies
- `zizmor.yaml` — Audits GitHub Actions workflows

## Evolution Note

These instructions are living documentation. When Copilot misbehaves:
1. Note the specific failure mode
2. Identify which instruction file should address it
3. Propose minimal, high-signal edits (avoid essay-style additions)
4. Test with focused prompts before committing changes
