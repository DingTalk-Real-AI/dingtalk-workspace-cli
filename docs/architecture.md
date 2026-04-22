# Architecture

`dws` is a Go CLI that turns DingTalk MCP metadata into a command-line surface for both humans and AI agents.

## High-Level Flow

1. `internal/market` fetches the registry and server metadata.
2. `internal/discovery` resolves runtime server capabilities and caches results.
3. `internal/ir` normalizes discovery output into one canonical tool catalog.
4. `internal/cli` and `internal/app` mount that catalog into the public Cobra command tree.
5. `internal/transport` executes MCP JSON-RPC calls and `internal/output` formats responses.

## Repository Structure

- `cmd`: CLI entrypoint
- `internal/app`: root command wiring and static utility commands
- `internal/discovery`, `internal/market`, `internal/transport`: runtime discovery and execution
- `internal/ir`: canonical intermediate representation for discovered tools
- `internal/generator`: docs, schema, and skill generation pipeline
- `internal/compat`, `internal/helpers`: legacy-compatible overlays and helper commands
- `skills/`: bundled agent skills source and generated skill docs
- `test/`: CLI, compatibility, integration, contract, and script tests

## Public Repository Contract

This repository ships source, docs, tests, packaging templates, and install scripts. Generated or release-only artifacts are produced by repository scripts and are not required to exist in a clean checkout unless explicitly committed as part of a release workflow.
