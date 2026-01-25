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
- `least_loaded` (requires `key`)
- `p2c` (power-of-two-choices; requires `key`, optional `sample`)

Notes:

- `round_robin` cycles deterministically through the candidate list. This tends to distribute load evenly across backends.
- `random` picks a backend uniformly at random from the candidate list. This is non-deterministic and may produce streaks.
- `p2c` samples `sample` random candidates (default: 2) and chooses the one with the smallest numeric value for `key`.

Pool selection controls:

- `pool.sort` sorts candidates before load balancing.
- `pool.limit` optionally caps the candidate set before load balancing.
- `pool.filters` can filter candidates before sorting and selection.
- `pool.fallback` defines additional selection attempts if the initial filters yield no candidates.

## Filters

Filters are evaluated against backend metadata and the decoded `Connect` request.

### `whitelist`

The `whitelist` filter is enabled per-backend via a metadata key and then checks whether the client is included in a backend-provided allowlist.

Fields:

- `enabled_key`: backend meta key that toggles whitelist behavior (`true`/`1`/`yes` enables it)
- `list_key`: backend meta key holding the allowlist (JSON array or comma-separated list)
- `subject`: which client field to match against the allowlist (`uuid` or `username`, default: `uuid`)

Example:

```yaml
filters:
  - type: whitelist
    subject: uuid
    enabled_key: annotation.agones.dev/sdk-whitelist-enabled
    list_key: list.whitelistedPlayers.values
```

### Discovery-backed pools

A pool can optionally reference a discovery provider instead of (or in addition to) static `backends`.

- `pool.discovery.provider` selects a configured provider (see `docs/configuration.md`).
- `pool.discovery.mode` controls how discovered backends interact with static backends:
  - `prefer`: use discovered backends if any exist, otherwise fall back to static backends
  - `union`: merge static + discovered backends

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

    - match:
        hostname: "play.example.com"
      pool:
        strategy: round_robin
        sort:
          - key: counter:players.count
            type: number
            order: asc
        limit: 10
        discovery:
          provider: agones
          mode: prefer
```

## Interaction with plugins

- Plugins run after the first `Connect` packet has been decoded.
- A plugin may return `backend` to override the routing decision.
- A plugin may return `selected_index` to pick a backend from `candidates`.

See:

- [`docs/plugin-configuration.md`](docs/plugin-configuration.md)
