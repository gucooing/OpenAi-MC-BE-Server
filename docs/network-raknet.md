# RakNet Decision

## Decision

Use `github.com/sandertv/go-raknet` as the initial RakNet transport implementation. Keep it behind `internal/network/raknet` so the server core depends on a local boundary instead of third-party API details.

## Rationale

- The library implements the RakNet protocol used by Minecraft: Bedrock Edition and exposes server-side `Listen`, `Accept` and `PongData` APIs.
- It also exposes `Ping` helpers, which gives the project a local smoke test for M1 without needing a full Bedrock client in CI.
- Starting with an adapter avoids spending the first milestone on reliable packet resend, MTU negotiation and unconnected ping plumbing before MCPE packet work can begin.

## Initial Scope

- Bind the configured UDP address.
- Respond to unconnected ping with the BetterAltay-style `MCPE;...;` server list payload.
- Accept RakNet sessions and keep them open long enough for later MCPE login handling.

## Boundaries

- MCPE batch compression, encryption, packet dispatch and login state do not belong in the RakNet adapter.
- If M2 or M6 exposes a need for lower-level packet control, fork or replace the transport behind the adapter.
- Query protocol support is separate from RakNet ping and can be implemented later if needed.

## Source Baseline

BetterAltay's `RakLibInterface::setName()` formats ping data as:

```text
MCPE;motd;protocol;version;players;maxPlayers;serverId;serverName;gamemode;
```
