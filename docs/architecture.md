# Architecture

## Layers

- `cmd/bds`: entrypoint.
- `internal/bootstrap`: flags, config, logging, shutdown, listener startup.
- `internal/network/raknet`: RakNet listener/session adapter.
- `internal/protocol`: packet codec, login parsing, dispatcher, debug session logging.
- `internal/config`: `server.properties` loading.

## Rules

- Prefer mature open-source protocol and transport libraries first.
- Keep third-party APIs behind thin local adapters.
- Log packet flow at `debug` level only.
- Do not log raw login secrets or JWT payloads.

## Flow

1. RakNet accepts a connection.
2. The protocol session handler decodes batch packets.
3. The handler logs parsed packets and login summaries.
4. Bootstrap controls shutdown and listener lifetime.
