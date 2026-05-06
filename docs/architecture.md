# Architecture

## Layers

- `cmd/bds`: entrypoint.
- `internal/bootstrap`: flags, config, logging, shutdown, listener startup.
- `internal/network/raknet`: RakNet transport adapter only.
- `internal/network/mcpe`: MCPE listener and connection adapter: RakNet session wiring, packet batch read/write, compression and encryption.
- `internal/server`: server-owned MCPE login parsing, encryption handshake, resource-pack/spawn/world/player-sync/chat/command session logic and packet routing.
- `internal/command`: lightweight command registry, parsing, permission checks and AvailableCommands metadata.
- `internal/world`: world-facing chunk provider and the default flat-world generator.
- `internal/config`: `server.properties` loading.

## Rules

- `gophertunnel` may provide packet structs, packet pools, packet field encode/decode, compression and login-chain/JWT helper types.
- Server behaviour stays local in `internal/server`: login state, resource-pack flow, StartGame, player lifecycle, world sync and gameplay packets are not delegated to `minecraft.Listener` or `minecraft.Conn`.
- Do not keep a local `internal/protocol` package while packet definitions are provided by gophertunnel; put MCPE wire concerns in `internal/network/mcpe` and login/session concerns in `internal/server`.
- Keep RakNet details behind `internal/network/raknet`.
- Log packet flow at `debug` level only.
- Do not log raw login secrets or JWT payloads.

## Flow

1. Bootstrap builds the `internal/server` MCPE handler from config.
2. Bootstrap starts `internal/network/mcpe` on the configured RakNet address with a server-provided client factory.
3. `internal/network/mcpe` configures MOTD/pong data and accepts sessions through `internal/network/raknet`.
4. Each MCPE network session decodes batches through its local codec and passes gophertunnel packet structs to the `internal/server` MCPE client session.
5. `internal/world` provides spawn data and generated chunks through a replaceable `ChunkProvider`.
6. `internal/server` adapts world chunks to `NetworkChunkPublisherUpdate`, `SetTime`, `SetSpawnPosition`, `LevelChunk` and `SubChunk` packets, then writes them through the network connection interface.
7. `internal/server` owns local player list/spawn/movement state and writes `PlayerList`, `AddPlayer`, `MovePlayer`, `SetActorData` and `SetActorMotion` directly through the network connection interface.
8. `internal/server` routes chat and command packets through the local command registry; bootstrap routes console input to the same registry with a console sender.
9. Bootstrap controls shutdown and listener lifetime.
