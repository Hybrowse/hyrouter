# Routing

Hyrouter uses the TLS SNI (server name / hostname) observed during the QUIC handshake to choose a backend.

Routing is evaluated *before* plugins run. Plugins may still influence the chosen backend.

## Matching

Routes support two match forms:

- `match.hostname` – a single hostname pattern
- `match.hostnames` – a list of hostname patterns

Patterns are matched using Go's `path.Match` semantics.

Examples:

- Exact host: `alpha.example.com`
- Wildcard subdomain: `*.example.com`

Notes:

- Matching is case-insensitive.
- A trailing dot is ignored.
- Routes are evaluated in order; the first match wins.

## Pools and backends

A backend is a host/port pair (plus optional metadata). A pool groups multiple backends with a selection strategy.

- `host` must be non-empty.
- `port` must be between 1 and 65535.

Pool strategies:

- `round_robin`
- `random`
- `weighted` (requires `weight > 0` on each backend)

## Example

```yaml
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
        strategy: round_robin
        backends:
          - host: alpha-backend.internal
            port: 5520

    - match:
        hostnames:
          - "*.example.com"
          - "example.net"
      pool:
        strategy: weighted
        backends:
          - host: wildcard-backend-a.internal
            port: 5520
            weight: 1
          - host: wildcard-backend-b.internal
            port: 5520
            weight: 2
```

## Interaction with plugins

- Plugins run after the first `Connect` packet has been decoded.
- A plugin may return `backend` to override the routing decision.
- A plugin may return `selected_index` to pick a backend from `candidates`.

See:

- [`docs/plugin-configuration.md`](docs/plugin-configuration.md)
