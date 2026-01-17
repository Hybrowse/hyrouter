# Routing

Hyrouter uses the TLS SNI (server name / hostname) observed during the QUIC handshake to choose a backend target.

Routing is evaluated *before* plugins run. Plugins may still override the chosen target.

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

## Targets

A target is a host/port pair:

- `host` must be non-empty.
- `port` must be between 1 and 65535.

## Example

```yaml
routing:
  default:
    host: play.hyvane.com
    port: 5520

  routes:
    - match:
        hostname: "alpha.example.com"
      target:
        host: alpha-backend.internal
        port: 5520

    - match:
        hostnames:
          - "*.example.com"
          - "example.net"
      target:
        host: wildcard-backend.internal
        port: 5520
```

## Interaction with plugins

- Plugins run after the first `Connect` packet has been decoded.
- A plugin may return `target` to override the routing decision.

See:

- [`docs/plugin-configuration.md`](docs/plugin-configuration.md)
