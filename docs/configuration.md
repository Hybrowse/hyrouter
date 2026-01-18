# Configuration

Hyrouter reads configuration from a YAML (`.yaml`/`.yml`) or JSON (`.json`) file.

Development examples are provided under `dev/`:

- [`dev/config.dev.yaml`](dev/config.dev.yaml) (no plugins)
- [`dev/config.plugins.dev.yaml`](dev/config.plugins.dev.yaml) (plugins enabled)

## CLI flags
 
- `-config` (default: `dev/config.dev.yaml`)
- `-log-level` (default: `info`): `debug|info|warn|error`

## Top-level fields

### `listen`

UDP listen address.

Example:

```yaml
listen: ":5520"
```

### `tls`

TLS configuration for QUIC.

Fields:

- `cert_file`: path to PEM certificate (optional)
- `key_file`: path to PEM private key (optional)
- `alpn`: list of ALPN identifiers (required)

Notes:

- `cert_file` and `key_file` must be set together.
- If both are omitted, Hyrouter generates a short-lived self-signed certificate for development.
- Hyrouter requests (but does not require) a client certificate. If present, the certificate fingerprint is exposed to plugins.

Example:

```yaml
tls:
  cert_file: "/etc/hyrouter/tls.crt"
  key_file: "/etc/hyrouter/tls.key"
  alpn:
    - hytale/1
```

### `quic`

QUIC transport configuration.

Fields:

- `max_idle_timeout`: Go duration string (`30s`, `1m`, ...)

Example:

```yaml
quic:
  max_idle_timeout: 30s
```

### `routing`

Static routing rules based on the TLS SNI (hostname) observed during the QUIC handshake.

Fields:

- `default`: fallback pool (optional)
  - `strategy` (string): `round_robin|random|weighted`
  - `backends` (list)
    - `host` (string)
    - `port` (int)
    - `weight` (int, only for `weighted`)
- `routes`: ordered list of routing rules (optional)
  - `match.hostname` (string) or `match.hostnames` (list of string)
  - `pool.strategy`
  - `pool.backends` (same schema as `default.backends`)

Routing notes:

- Routes are evaluated in order; the first match wins.
- Hostname matching supports wildcard patterns via Go's `path.Match` semantics (for example `*.example.com`).

Example:

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
        strategy: weighted
        backends:
          - host: alpha-backend-a.internal
            port: 5520
            weight: 1
          - host: alpha-backend-b.internal
            port: 5520
            weight: 3
    - match:
        hostname: "*.example.com"
      pool:
        strategy: random
        backends:
          - host: wildcard-backend.internal
            port: 5520
```

See also: [`docs/routing.md`](docs/routing.md).

### `plugins`

Optional plugins that can deny connections or mutate the routing decision.

Plugins are executed after Hyrouter decodes the first Hytale `Connect` packet.

See:

- [`docs/plugin-configuration.md`](docs/plugin-configuration.md)
- [`docs/plugin-development.md`](docs/plugin-development.md)

Example:

```yaml
plugins:
  - name: deny-grpc
    type: grpc
    stage: deny
    grpc:
      address: 127.0.0.1:7777

  - name: mutate-wasm
    type: wasm
    stage: mutate
    wasm:
      path: examples/wasm-plugin/plugin.wasm
```

## Notes on `examples/`

The `examples/` directory uses Go build tags (`-tags=examples`).

Repository tasks like `task test` and `task cover` exclude `examples/` packages by default.
