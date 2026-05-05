# Architecture

## Layers

- `cmd/bds`: entrypoint.
- `internal/bootstrap`: flags, config, logging, shutdown, listener startup.
- `internal/network/raknet`: RakNet transport adapter only.
- `internal/network/mcpe`: server-owned MCPE session state machine.
- `internal/protocol`: packet codec, batch compression/encryption, login parsing, handshake key derivation and packet dispatch primitives.
- `internal/world`: world-facing chunk provider and the default flat-world generator.
- `internal/config`: `server.properties` loading.

## Rules

- `gophertunnel` may provide packet structs, packet pools, login-chain parsing and compression/JWT helper types.
- Server behaviour stays local: login state, resource-pack flow, StartGame, player lifecycle, world sync and gameplay packets are not delegated to `minecraft.Listener` or `minecraft.Conn`.
- Keep RakNet details behind `internal/network/raknet`.
- Log packet flow at `debug` level only.
- Do not log raw login secrets or JWT payloads.

## Flow

1. Bootstrap starts `internal/network/mcpe` on the configured RakNet address.
2. `internal/network/mcpe` configures MOTD/pong data and accepts sessions through `internal/network/raknet`.
3. Each MCPE session decodes batches with `internal/protocol`, advances the local login/encryption/resource-pack/spawn state machine, and writes packets back through RakNet.
4. `internal/world` provides spawn data and generated chunks through a replaceable `ChunkProvider`.
5. `internal/network/mcpe` adapts world chunks to `NetworkChunkPublisherUpdate` and `LevelChunk` packets.
6. Bootstrap controls shutdown and listener lifetime.
