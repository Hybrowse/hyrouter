# Hyrouter

Hyrouter is a highly flexible, plugin-driven Layer 7 referral router and QUIC entrypoint for Hytale.

It gives you a clean “front door” for your Hytale backend fleet: Hyrouter decides where a client should go and redirects it instantly.

It accepts an incoming QUIC connection from a Hytale client, inspects minimal metadata (TLS SNI + the first `Connect` packet), applies optional plugins, and then either:

- **Denies** the connection with a Hytale `Disconnect` packet, or
- **Redirects** the client to a backend using a Hytale `ClientReferral` packet.

Hyrouter is **not** a reverse proxy and does **not** forward gameplay traffic.

## What you get

- Hostname routing via TLS SNI (`match.hostname` / `match.hostnames`)
- Built-in load balancing per route (`round_robin`, `random`, `weighted`)
- Plugin hooks (gRPC or WASM) to deny connections, influence backend selection, and attach referral data
- Stateless data plane: no session storage required, no gameplay proxying

## Why

Hyrouter is useful when you want a lightweight, stateless entrypoint in front of one or many Hytale servers:

- **Traffic steering** based on hostname (SNI) via routing rules.
- **Fail-safe behavior**: deny, redirect, or fall back to a default target.
- **Extensible policies** via plugins (gRPC or WASM): deny connections, influence backend selection, attach referral data.
- **No gameplay proxying**: lower cost and less operational complexity.

## How it works (high level)

- Client opens a QUIC connection and negotiates ALPN (default: `hytale/1`).
- Hyrouter reads the first framed packet (`Connect`, packet ID `0`).
- Hyrouter selects a backend based on SNI routing rules, load balancing strategy, and optional plugins.
- Hyrouter sends either:
  - `ClientReferral` (packet ID `18`) to redirect the client, or
  - `Disconnect` (packet ID `1`) if a plugin denies the connection.
- The stream is closed.

## Example configuration

```yaml
listen: ":5520"

tls:
  alpn:
    - hytale/1

routing:
  default:
    strategy: round_robin
    backends:
      - host: play.hyvane.com
        port: 5520

  routes:
    - match:
        hostname: "alpha.example.com"
      pool:
        strategy: weighted
        backends:
          - host: alpha-backend-a.internal
            port: 5520
            weight: 1
          - host: alpha-backend-b.internal
            port: 5520
            weight: 3
```

## Quickstart

### Local

Run with the development config:

```bash
go run ./cmd/hyrouter -config dev/config.dev.yaml
```

Or using `go-task`:

```bash
task run
```

Enable debug logging:

```bash
task run:debug
```

### Local (plugins demo)

In separate terminals:

```bash
task plugin:grpc:run
```

```bash
task plugin:wasm:build
```

Then run Hyrouter with a config that enables both plugins:

```bash
task run:plugins
```

If you need detailed logs:

```bash
task run:plugins:debug
```

### Docker

Build:

```bash
docker build -t hyrouter:local .
```

Run (mount your config):

```bash
docker run --rm \
  -p 5520:5520/udp \
  -v $(pwd)/config.yaml:/app/config.yaml \
  hyrouter:local
```

## Configuration

Hyrouter supports YAML and JSON config files.

See:

- [`docs/configuration.md`](docs/configuration.md)
- [`docs/plugin-configuration.md`](docs/plugin-configuration.md)

## Documentation

- [`docs/README.md`](docs/README.md)
- [`docs/architecture.md`](docs/architecture.md)
- [`docs/configuration.md`](docs/configuration.md)
- [`docs/plugin-configuration.md`](docs/plugin-configuration.md)
- [`docs/plugin-development.md`](docs/plugin-development.md)
- [`docs/troubleshooting.md`](docs/troubleshooting.md)

## Logging

Use `-log-level` to control verbosity:

```bash
hyrouter -config config.yaml -log-level debug
```

Default is `info`.

## Development

Common tasks:

- `task fmt`
- `task tidy`
- `task test`
- `task cover`

## Status / roadmap

Hyrouter implements:

- QUIC intake and TLS/ALPN handling
- SNI-based routing rules with load balancing pools (`round_robin`, `random`, `weighted`)
- `ClientReferral` and `Disconnect` packet handling
- Plugin system (gRPC + WASM) with deterministic ordering

Planned work includes Kubernetes/Agones discovery backends and more advanced selection/sorting.
