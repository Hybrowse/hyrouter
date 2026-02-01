# Hyrouter

Hyrouter is a highly flexible, plugin-driven Layer 7 referral router and QUIC entrypoint for Hytale.

It gives you a clean “front door” for your Hytale backend fleet: Hyrouter decides where a client should go and redirects it instantly.

It accepts an incoming QUIC connection from a Hytale client, inspects minimal metadata (TLS SNI + the first `Connect` packet), applies optional plugins, and then either:

- **Denies** the connection with a Hytale `Disconnect` packet, or
- **Redirects** the client to a backend using a Hytale `ClientReferral` packet.

Hyrouter is **not** a reverse proxy and does **not** forward gameplay traffic.

## Hybrowse Server Stack

Hyrouter is part of the **Hybrowse Server Stack** — production-grade building blocks for running Hytale at scale:

- [Hybrowse/hytale-server-docker](https://github.com/Hybrowse/hytale-server-docker) — hardened Docker image for dedicated servers (mods, auto-download, Kubernetes assets)
- [Hybrowse/hytale-session-token-broker](https://github.com/Hybrowse/hytale-session-token-broker) — non-interactive server authentication for providers/fleets

> [!IMPORTANT]
> Due to a client-side bug, connection redirects via `ClientReferral` only work correctly starting with Hytale client version `pre-release/2026.01.29`.
> We expect the next regular client release (Patch 3) to roll this fix out to all players.

## What you get

- Hostname routing via TLS SNI (`match.hostname` / `match.hostnames`)
- Built-in load balancing per route (`round_robin`, `random`, `weighted`, `least_loaded`, `p2c`)
- Filtering, sorting, and candidate limiting for backend selection (pre-selection controls)
- Discovery providers for dynamic backend lists (Kubernetes, Agones)
- Plugin hooks (gRPC or WASM) to deny connections, influence backend selection, and attach referral data
- Optional signed referral envelope (HMAC) for backend verification
- Stateless data plane: no session storage required, no gameplay proxying

## Why

Hyrouter is useful when you want a lightweight, stateless entrypoint in front of one or many Hytale servers:

- **Traffic steering** based on hostname (SNI) via routing rules.
- **Fail-safe behavior**: deny, redirect, or fall back to a default target.
- **Extensible policies** via plugins (gRPC or WASM): deny connections, influence backend selection, attach referral data.
- **No gameplay proxying**: lower cost and less operational complexity.

## How it works (high level)

- Client opens a QUIC connection and negotiates ALPN (configured via `tls.alpn`; recommended: `hytale/*` to pick the highest `hytale/<n>` offered by the client, currently typically `hytale/2`).
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
    - hytale/*

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

The default config file used when `-config` is omitted is `config.yaml`.

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

Or pull a published image:

```bash
docker pull hybrowse/hyrouter:latest
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

Development examples are provided under `dev/`:

- `dev/config.dev.yaml` (no plugins)
- `dev/config.plugins.dev.yaml` (plugins enabled)
- `dev/config.kubernetes.pods.dev.yaml` (Kubernetes discovery via Pods)
- `dev/config.kubernetes.endpointslices.dev.yaml` (Kubernetes discovery via EndpointSlices)
- `dev/config.agones.observe.dev.yaml` (Agones discovery observe mode)
- `dev/config.agones.allocate.dev.yaml` (Agones discovery allocate mode)

See:

- [`docs/configuration.md`](docs/configuration.md)
- [`docs/routing.md`](docs/routing.md)
- [`docs/plugin-configuration.md`](docs/plugin-configuration.md)
- [`docs/referral-envelope.md`](docs/referral-envelope.md)

## Documentation

- [`docs/README.md`](docs/README.md)
- [`docs/architecture.md`](docs/architecture.md)
- [`docs/configuration.md`](docs/configuration.md)
- [`docs/routing.md`](docs/routing.md)
- [`docs/plugin-configuration.md`](docs/plugin-configuration.md)
- [`docs/referral-envelope.md`](docs/referral-envelope.md)
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
- SNI-based routing rules with load balancing pools (`round_robin`, `random`, `weighted`, `least_loaded`, `p2c`)
- `ClientReferral` and `Disconnect` packet handling
- Plugin system (gRPC + WASM) with deterministic ordering

Hyrouter also includes discovery providers and examples under `dev/`:

- Kubernetes discovery (Pods, EndpointSlices)
- Agones discovery (observe mode, allocate mode)

Planned work includes additional discovery providers and more advanced selection/sorting.

## Contributing & Security
 
- [`CONTRIBUTING.md`](CONTRIBUTING.md)
- [`LICENSING.md`](LICENSING.md)
- [`SECURITY.md`](SECURITY.md)

## Legal and policy notes

This is an **unofficial** community project and is not affiliated with or endorsed by Hypixel Studios Canada Inc.

This repository and image do not redistribute proprietary Hytale game/server files.
Server operators are responsible for complying with the Hytale EULA, Terms of Service, and Server Operator Policies (including monetization and branding rules): https://hytale.com/server-policies

## License
 
Current repository license: [`LICENSE`](LICENSE)

See also: [`NOTICE`](NOTICE).

For an overview (including commercial agreements and trademarks), see:

- [`LICENSING.md`](LICENSING.md)
- [`COMMERCIAL_LICENSE.md`](COMMERCIAL_LICENSE.md)
- [`TRADEMARKS.md`](TRADEMARKS.md)
