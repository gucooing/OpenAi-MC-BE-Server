# Protocol

## Current Approach

The protocol layer uses `github.com/sandertv/gophertunnel` as the source of truth for packet encoding, batch compression, packet pools, and login parsing.

## Local Adapters

- `internal/protocol/codec.go`: packet encode/decode and batch compression.
- `internal/protocol/login.go`: login request parsing.
- `internal/protocol/dispatcher.go`: packet ID routing.
- `internal/protocol/session.go`: debug packet logging and a small login-stage smoke path.

## Debug Logs

When `log-level=debug`, packet flow logs include:

- remote address
- direction
- packet ID
- packet Go type
- login summary fields such as display name, UUID, XUID, device OS, and game version

Raw JWT contents are not logged.
