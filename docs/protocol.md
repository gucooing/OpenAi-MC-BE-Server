# Protocol

## Current Approach

The protocol layer uses `github.com/sandertv/gophertunnel` for packet structs, packet pools, packet field encode/decode, compression algorithms and login-chain parsing.

Runtime protocol logic is local code. `gophertunnel` must not own the listener, connection lifecycle, login state machine, resource-pack flow, StartGame flow, player lifecycle, world sync or gameplay handling.

## Local Adapters

- `internal/protocol/codec.go`: packet encode/decode, batch framing, compression and encrypted batch checksum/cipher state.
- `internal/protocol/handshake.go`: ServerToClientHandshake JWT salt and P-384 ECDH encryption key derivation.
- `internal/protocol/login.go`: login request parsing.
- `internal/protocol/dispatcher.go`: packet ID routing.
- `internal/network/raknet`: RakNet transport, unconnected ping/pong and accepted reliable sessions.
- `internal/network/mcpe/server.go`: MCPE listener boundary that wires RakNet sessions to local MCPE sessions.
- `internal/network/mcpe/session.go`: local NetworkSettings, Login, resource-pack and spawn state machine.
- `internal/network/mcpe/chunk_publisher.go`: adapter from world chunks to MCPE chunk publisher/update packets.
- `internal/world`: runtime block state lookup and flat chunk generation.

## Login Flow

`internal/network/mcpe` owns the server-side state machine:

1. Read `RequestNetworkSettings` and reply with `NetworkSettings`.
2. Enable batch compression for following MCPE packets.
3. Read `Login`, parse the login chain through `internal/protocol/login.go`, and apply local online/offline policy.
4. Send a locally built `ServerToClientHandshake` containing a signed JWT salt, derive the shared encryption key from the client login public key and enable encrypted batch IO.
5. Read the encrypted `ClientToServerHandshake`.
6. Send encrypted `PlayStatusLoginSuccess` and an empty `ResourcePacksInfo`.
7. Handle empty resource-pack stack responses.
8. Send locally built `StartGame` and follow-up spawn packets.

The local encryption path is covered by protocol/session smoke tests. M2 still needs real Bedrock client validation before it can be marked complete.

## Chunk Flow

The runtime world path is split so world generation and packet construction can evolve separately:

- `internal/world.ChunkProvider` owns spawn position and chunk generation.
- The default provider creates a flat overworld using Dragonfly's block-state and chunk palette encoder.
- `internal/network/mcpe` converts generated chunks to `NetworkChunkPublisherUpdate` and `LevelChunk` packets for the session.

## Debug Logs

When `log-level=debug`, packet flow logs include remote address, packet ID, packet Go type and session state. Login summary logs include display name, UUID, XUID, device OS and game version.

Raw JWT contents are not logged.
