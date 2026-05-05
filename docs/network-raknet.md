# RakNet Decision

## Decision

Use `github.com/sandertv/go-raknet` as the initial RakNet transport implementation. Keep it behind `internal/network/raknet` so the server core depends on a local boundary instead of third-party API details.

This package is transport only. MCPE packet batch IO lives in `internal/network/mcpe`; MCPE login, packet routing, resource packs, StartGame, world sync and gameplay logic live in `internal/server`.

## Rationale

- The library implements the RakNet protocol used by Minecraft: Bedrock Edition and exposes server-side `Listen`, `Accept` and `PongData` APIs.
- It also exposes `Ping` helpers, which gives the project a local smoke test for M1 without needing a full Bedrock client in CI.
- Starting with a transport adapter avoids spending the first milestone on reliable packet resend, MTU negotiation and unconnected ping plumbing before MCPE packet work can begin.

## Initial Scope

- Bind the configured UDP address.
- Respond to unconnected ping with the BetterAltay-style `MCPE;...;` server list payload.
- Accept RakNet sessions and pass their reliable payload stream to the local MCPE session state machine.

Runtime startup builds the server MCPE handler first, then passes a packet-client factory into `internal/network/mcpe`. The network layer configures the RakNet adapter and forwards decoded MCPE packets to that client. The project no longer keeps a gophertunnel `minecraft.Listener` runtime path.

## Boundaries

- MCPE batch compression and encryption belong in `internal/network/mcpe`, not in the RakNet adapter.
- MCPE packet dispatch and login state belong in `internal/server`.
- `gophertunnel` is used for MCPE packet parsing/packing helpers only, not for listener or connection ownership.
- Query protocol support is separate from RakNet ping and can be implemented later if needed.

## Source Baseline

BetterAltay's `RakLibInterface::setName()` formats ping data as:

```text
MCPE;motd;protocol;version;players;maxPlayers;serverId;serverName;gamemode;
```
