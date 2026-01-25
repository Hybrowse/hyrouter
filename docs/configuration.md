# Configuration

Hyrouter reads configuration from a YAML (`.yaml`/`.yml`) or JSON (`.json`) file.

Development examples are provided under `dev/`:

- [`dev/config.dev.yaml`](dev/config.dev.yaml) (no plugins)
- [`dev/config.plugins.dev.yaml`](dev/config.plugins.dev.yaml) (plugins enabled)
- [`dev/config.kubernetes.pods.dev.yaml`](dev/config.kubernetes.pods.dev.yaml) (Kubernetes discovery via Pods)
- [`dev/config.kubernetes.endpointslices.dev.yaml`](dev/config.kubernetes.endpointslices.dev.yaml) (Kubernetes discovery via EndpointSlices)
- [`dev/config.agones.observe.dev.yaml`](dev/config.agones.observe.dev.yaml) (Agones discovery observe mode)
- [`dev/config.agones.allocate.dev.yaml`](dev/config.agones.allocate.dev.yaml) (Agones discovery allocate mode)

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
- Default: `hytale/*` (accept any `hytale/<n>` ALPN offered by the client).
- To restrict to specific protocol versions, set `alpn` explicitly (for example `hytale/2`).

Example:

```yaml
tls:
  cert_file: "/etc/hyrouter/tls.crt"
  key_file: "/etc/hyrouter/tls.key"
  alpn:
    - hytale/*
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

### `logging`

Logging configuration.

Fields:

- `log_client_ip` (bool): include the client socket address (`remote_addr`) in logs.

Example:

```yaml
logging:
  log_client_ip: false
```

### `routing`

Static routing rules based on the TLS SNI (hostname) observed during the QUIC handshake.

Fields:

- `default`: fallback pool (optional)
  - `strategy` (string): `round_robin|random|weighted|least_loaded|p2c`
  - `key` (string, required for `least_loaded` and `p2c`)
  - `sample` (int, optional; `p2c` only)
  - `sort` (list, optional): sorting rules applied before load balancing
  - `limit` (int, optional): optional maximum number of candidates
  - `filters` (list, optional): candidate filters
  - `fallback` (list, optional): additional selection attempts applied if filters yield no candidates
  - `backends` (list)
    - `host` (string)
    - `port` (int)
    - `weight` (int, only for `weighted`)
- `routes`: ordered list of routing rules (optional)
  - `match.hostname` (string) or `match.hostnames` (list of string)
  - `pool.strategy`
  - `pool.key` / `pool.sample` / `pool.sort` / `pool.limit` / `pool.filters` / `pool.fallback`
  - `pool.backends` (same schema as `default.backends`)
  - `pool.discovery` (optional)
    - `provider` (string): reference to a configured discovery provider
    - `mode` (string): `union|prefer`
  - Sort/limit/filtering are controlled at the pool level (not under `pool.discovery`).

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

    - match:
        hostname: "play.example.com"
      pool:
        strategy: p2c
        key: counter:onlinePlayers.count
        sample: 2
        filters:
          - type: compare
            left: counter:onlinePlayers.count
            op: lt
            right: counter:onlinePlayers.capacity
          - type: game_start_not_past
            key: annotation.agones.dev/sdk-game-start
          - type: whitelist
            enabled_key: annotation.agones.dev/sdk-whitelist-enabled
            list_key: list.whitelistedPlayers.values
        fallback:
          - strategy: round_robin
            filters: []
        discovery:
          provider: agones
          mode: prefer
```

### `referral`

Referral envelope configuration.

Hyrouter always wraps plugin-provided referral content into a fixed, versioned envelope.

Fields:

- `key_id` (int): key identifier included in the envelope (0-255)
- `hmac_secret` (string, optional): shared secret used to sign the envelope with HMAC-SHA256

`hmac_secret` formats:

- raw string (used as UTF-8 bytes)
- `base64:<...>`
- `hex:<...>`

Example:

```yaml
referral:
  key_id: 1
  hmac_secret: "base64:REPLACE_ME_WITH_A_SECRET"
```

### `discovery`

Optional dynamic backend discovery.

If `discovery` is omitted, Hyrouter uses only statically configured `routing.*.backends`.

Discovery is enabled per pool via `pool.discovery.provider`.

#### Providers

`discovery.providers` is a list of named providers.

Each provider has:

- `name` (string)
- `type` (string): `kubernetes|agones`

##### Kubernetes provider

Selector semantics:

- `resources[].selector.labels` is a Kubernetes label selector expression (for example `app=my-game,region in (eu,us)`).
- `resources[].selector.annotations` is a simple comma-separated `k=v` matcher.

Metadata:

- `metadata.include_labels` copies the selected label keys into backend metadata under `label.<key>`.
- `metadata.include_annotations` copies the selected annotation keys into backend metadata under `annotation.<key>`.

RBAC (in-cluster):

Hyrouter needs read permissions for the watched resources.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: hyrouter-discovery
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["discovery.k8s.io"]
    resources: ["endpointslices"]
    verbs: ["get", "list", "watch"]
```

```yaml
discovery:
  providers:
    - name: k8s
      type: kubernetes
      kubernetes:
        kubeconfig: "" # empty => in-cluster
        namespaces: ["hytale"]
        resources:
          - kind: endpointslices
            service:
              name: hytale-backend
              namespace: hytale
            port:
              name: game
        filters:
          require_pod_ready: true
          require_pod_phase: ["Running"]
          require_endpoint_ready: true
        metadata:
          include_labels: ["region", "fleet"]
          include_annotations: ["hyrouter/weight"]
```

##### Agones provider

RBAC (in-cluster):

Observe mode requires read access to `gameservers.agones.dev`. Allocate mode additionally requires create access to `gameserverallocations.allocation.agones.dev`.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: hyrouter-agones
rules:
  - apiGroups: ["agones.dev"]
    resources: ["gameservers"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["allocation.agones.dev"]
    resources: ["gameserverallocations"]
    verbs: ["create"]
```

```yaml
discovery:
  providers:
    - name: agones
      type: agones
      agones:
        kubeconfig: "" # empty => in-cluster
        namespaces: ["hytale"]
        mode: observe # observe|allocate
        allocate_min_interval: "250ms" # optional; allocate mode only
        state: ["Ready", "Reserved"]
        selector:
          labels: "agones.dev/fleet=hytale,region=eu"
          annotations: "hyrouter/enabled=true"
        metadata:
          include_labels: ["agones.dev/fleet", "hytale.hyvane.com/region", "hytale.hyvane.com/server-tag"]
          include_annotations: ["agones.dev/sdk-whitelist-enabled", "agones.dev/sdk-game-start"]
        address_source: "addresses" # address|addresses
        address_preference: ["ExternalIP", "PublicIP", "NodeExternalIP"]
        port:
          name: gameport
```

Allocate mode notes:

- `allocate_min_interval` throttles allocation requests to avoid hammering the Kubernetes API.
- If multiple `namespaces` or `state` entries are configured, allocate mode uses the first entry.

#### Using a provider from routing

```yaml
routing:
  routes:
    - match:
        hostname: "play.example.com"
      pool:
        strategy: p2c
        key: counter:players.count
        sample: 2
        sort:
          - key: counter:players.count
            type: number
            order: asc
        limit: 10
        discovery:
          provider: agones
          mode: prefer
```

See also: [`docs/routing.md`](docs/routing.md).

### `messages`

Optional user-facing messages.

#### `messages.disconnect`

Hyrouter may send a Hytale `Disconnect` packet if it cannot select a backend (for example because no route matched, there are no backends, or discovery failed).

Fields:

- `no_route`: used if no route/default matched
- `no_backends`: used if a route matched but there are no backends
- `routing_error`: generic routing error
- `discovery_error`: discovery-related error

Optional:

- `disconnect_locales`: map of language tags to overrides. Hyrouter first tries an exact match (e.g. `de-DE`), then falls back to the base language (e.g. `de`), otherwise it uses `messages.disconnect`.

All disconnect messages support the following template variables:

- `${sni}`: the SNI hostname
- `${error}`: the internal error string (usually leave this out for player-friendly messages)

Example:

```yaml
messages:
  disconnect:
    no_route: "The server is currently unavailable."
    no_backends: "The server is full or restarting. Please try again in a moment."
    routing_error: "The server is currently unreachable. Please try again later."
    discovery_error: "The server is looking for an available instance. Please try again in a moment."
  disconnect_locales:
    de:
      no_route: "Der Server ist aktuell nicht verfügbar."
      no_backends: "Der Server ist gerade voll oder startet neu. Bitte versuche es gleich erneut."
      routing_error: "Der Server ist aktuell nicht erreichbar. Bitte versuche es später erneut."
      discovery_error: "Der Server sucht gerade eine freie Instanz. Bitte versuche es gleich erneut."
```

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
