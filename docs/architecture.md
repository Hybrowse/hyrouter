# Architecture

Hyrouter is a stateless QUIC entrypoint and referral router for Hytale.

It accepts an incoming QUIC connection, extracts minimal metadata (TLS SNI + the first `Connect` packet), chooses a backend target using routing rules and optional plugins, sends a referral/redirect to the client, and closes the stream.

Hyrouter is **not** a reverse proxy and does **not** forward gameplay traffic.

## Core principles

- Stateless data plane (no required DB/Redis)
- Fail-safe routing (fallbacks or explicit deny)
- No gameplay proxying / forwarding
- Extensible policies via plugins (gRPC or WASM)

## Components

- CLI entrypoint: `cmd/hyrouter`
- Config loading + validation: `internal/config`
- Static routing engine (SNI-based): `internal/routing`
- Plugin system (ordering + backends): `internal/plugins`
- QUIC server + packet handling: `internal/server`

## Connection flow

1. Client connects via QUIC and negotiates ALPN (configured via `tls.alpn`; typically `hytale/2`).
2. Hyrouter obtains TLS metadata:
   - remote address
   - SNI (hostname)
   - negotiated ALPN
   - optional client certificate fingerprint
3. Hyrouter decides an initial target using the routing engine (based on SNI).
4. Hyrouter accepts a bidirectional QUIC stream and reads Hytale framed packets.
   - Framing: `uint32le payloadLen` + `uint32le packetID` + `payload`
5. When the first `Connect` packet (packet ID `0`) is decoded:
   - Hyrouter extracts identity fields (username, uuid, language, ...).
   - Hyrouter calls plugins (if configured).
6. Result:
   - If any plugin denies: Hyrouter sends `Disconnect` (packet ID `1`) and closes the stream.
   - Otherwise, if a routing target is available: Hyrouter sends `ClientReferral` (packet ID `18`) and closes the stream.

## Plugin execution model

Plugins run sequentially with deterministic ordering:

- Stage order: `deny` -> `route` -> `mutate`
- Within each stage: `before` / `after` constraints (topological sort)

Plugins can deny, override the target, or attach referral data.

See:

- [`plugin-configuration.md`](plugin-configuration.md)
- [`plugin-development.md`](plugin-development.md)
