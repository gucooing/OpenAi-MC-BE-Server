# Coding Standards

This project is a Go rewrite of BetterAltay. Code should keep the runtime easy to inspect, test and change.

## Core Rules

- Prefer direct, readable Go over generated ceremony.
- Do not scatter hardcoded values through gameplay, protocol or world logic. Protocol constants, version identifiers, documented defaults and golden test data are allowed when their source is clear.
- Do not add tiny pass-through functions that only rename, forward or wrap another call. Add a function only when it owns a real boundary: protocol encoding, error semantics, concurrency ownership, dependency isolation, test substitution or removal of meaningful duplication.
- Keep package boundaries aligned with runtime ownership. Network sessions, world state and player state must not mutate each other through unclear side paths.
- Return errors with enough context for operators and tests to understand the failing subsystem.
- Keep public API promises small until the plugin system is designed.

## Concurrency

- Treat the server tick loop as the owner of live world, entity and player state unless a task explicitly documents a different owner.
- Move data across goroutines by messages or immutable snapshots.
- Add race tests for code that crosses goroutine boundaries.

## Data Sources

- Prefer registries, generated tables or parsed resource files for block, item, packet and runtime ID data.
- When a constant comes from BetterAltay, Bedrock protocol documentation or a fixture, keep it near the related module and test it.

