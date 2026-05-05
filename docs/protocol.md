# Protocol

## Current Approach

The project uses `github.com/sandertv/gophertunnel` directly for packet structs, packet pools, packet field encode/decode, compression algorithms and login-chain parsing.

Runtime protocol logic is local code. `gophertunnel` must not own the listener, connection lifecycle, login state machine, resource-pack flow, StartGame flow, player lifecycle, world sync or gameplay handling.

There is no local `internal/protocol` package while gophertunnel owns packet definitions. MCPE wire helpers live with the MCPE network adapter, and login/session helpers live with the server session code.

## Local Adapters

- `internal/network/raknet`: RakNet transport, unconnected ping/pong and accepted reliable sessions.
- `internal/network/mcpe/server.go`: MCPE listener boundary that wires RakNet sessions to a server-provided packet client.
- `internal/network/mcpe/codec.go`: MCPE packet encode/decode, batch framing, compression and encrypted batch checksum/cipher state.
- `internal/network/mcpe/session.go`: connection adapter for MCPE batch IO, compression and encryption.
- `internal/server/mcpe_*.go`: local NetworkSettings, Login, resource-pack, StartGame, spawn and world-sync state.
- `internal/server/mcpe_login.go`: login request parsing.
- `internal/server/mcpe_handshake.go`: ServerToClientHandshake JWT salt and P-384 ECDH encryption key derivation.
- `internal/server/mcpe_router.go`: server-owned packet ID routing.
- `internal/server/mcpe_player_sync.go`: local player list, AddPlayer spawn packets, actor metadata/motion packets and movement input routing.
- `internal/server/chunk_publisher.go`: adapter from world chunks to MCPE chunk publisher/update packets.
- `internal/world`: runtime block state lookup and flat chunk generation.

## Login Flow

`internal/server` owns the server-side MCPE state machine:

1. Read `RequestNetworkSettings` and reply with `NetworkSettings`.
2. Enable batch compression for following MCPE packets.
3. Read `Login`, parse the login chain through `internal/server/mcpe_login.go`, and apply local online/offline policy.
4. Send a locally built `ServerToClientHandshake` containing a signed JWT salt, derive the shared encryption key from the client login public key and enable encrypted batch IO.
5. Read the encrypted `ClientToServerHandshake`.
6. Send encrypted `PlayStatusLoginSuccess` and an empty `ResourcePacksInfo`.
7. Handle empty resource-pack stack responses.
8. Send locally built `StartGame`, player list data, actor metadata/motion and follow-up spawn packets.

The local encryption path is covered by network/server smoke tests. M2 still needs real Bedrock client validation before it can be marked complete.

## Chunk Flow

The runtime world path is split so world generation and packet construction can evolve separately:

- `internal/world.ChunkProvider` owns spawn position and chunk generation.
- The default provider creates a flat overworld using Dragonfly's block-state and chunk palette encoder.
- `internal/server` converts generated chunks to `NetworkChunkPublisherUpdate`, `SetTime`, `SetSpawnPosition`, `LevelChunk` and `SubChunk` packets for the session.

## Player Flow

`internal/server` keeps the current MCPE players on the handler and mirrors the BetterAltay ordering at a minimal level:

- A player is registered when `StartGame` is sent.
- Existing clients receive a `PlayerList` add entry for the new player.
- The joining client receives a full `PlayerList` snapshot plus its own `SetActorData` and `SetActorMotion`.
- After `SetLocalPlayerAsInitialised`, spawned peers exchange `AddPlayer`, `SetActorData` and `SetActorMotion`.
- `PlayerAuthInput` and legacy `MovePlayer` packets update local position/rotation state and broadcast `MovePlayer`; metadata and motion changes broadcast `SetActorData` and `SetActorMotion`.

## Debug Logs

When `log-level=debug`, packet flow logs include remote address, packet ID, packet Go type and session state. Login summary logs include display name, UUID, XUID, device OS and game version.

Raw JWT contents are not logged.
