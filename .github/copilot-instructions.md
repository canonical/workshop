# GitHub Copilot Instructions

For review-specific guidance, see:

- **Code Review**: `.github/agents/code-review.agent.md`
- **Documentation Review**: `.github/skills/documentation-review/SKILL.md`

## Project Overview

Workshop is a tool for defining and handling ephemeral development environments. It uses a client-server architecture where `workshopd` daemon exposes a RESTful API to clients. The project is written in Go, packaged as a Snap, and uses LXD as the container backend. Unit tests use gocheck; end-to-end tests use Spread.

Key directories: `cmd/` (CLI entry points), `client/` (Go client library), `internal/` (core packages: `daemon`, `overlord`, `workshop`, `interfaces`, `sdk`).

## Coding Guidelines

See [`docs/coding-style-guide.md`](../docs/coding-style-guide.md) for detailed standards.

## Development Workflow

See [`docs/contributing/development.rst`](../docs/contributing/development.rst) for setup, testing, and workflow details.

## Related Repositories

These external repositories provide authoritative context for the Workshop project:

- https://github.com/canonical/sdkcraft — SDKcraft utility codebase for packaging and publishing SDKs
- https://github.com/canonical/reference-workshops — Reference workshop implementations demonstrating SDK usage patterns
