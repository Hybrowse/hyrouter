# Plugin configuration

Hyrouter supports plugins that can influence connection handling after the client sends the first Hytale `Connect` packet (packet ID `0`).

Plugins can:

- Deny a connection (Hyrouter replies with `Disconnect`, packet ID `1`).
- Override the selected routing backend.
- Attach referral data (forwarded into `ClientReferral`, packet ID `18`).

Plugins are configured under the top-level `plugins` list in the Hyrouter config file.

## Plugin ordering

Plugins are executed in two dimensions:

- **Stage order**: `deny` -> `route` -> `mutate`
- **Within a stage**: deterministic ordering using a topological sort over `before` / `after` constraints.

If `stage` is omitted, it defaults to `route`.

## Common fields

Each plugin entry supports:

- `name` (string, required)
- `type` (string, required): `grpc` or `wasm`
- `stage` (string, optional): `deny`, `route`, `mutate`
- `before` (list of string, optional): plugin names that should run after this plugin
- `after` (list of string, optional): plugin names that should run before this plugin

## gRPC plugin

### Fields

- `grpc.address` (string, required): address of the plugin server (for example `127.0.0.1:7777`)

### Example

```yaml
plugins:
  - name: deny-grpc
    type: grpc
    stage: deny
    grpc:
      address: 127.0.0.1:7777
```

## WASM plugin

### Fields

- `wasm.path` (string, required): path to a `.wasm` file

### Example

```yaml
plugins:
  - name: mutate-wasm
    type: wasm
    stage: mutate
    wasm:
      path: examples/wasm-plugin/plugin.wasm
```

## Behavior details

### Deny

If any plugin returns `deny: true`, Hyrouter:

- Sends a Hytale `Disconnect` packet.
- Closes the QUIC stream.

### Target override

A plugin may influence backend selection by setting one of:

- `backend` (explicit host/port)
- `selected_index` (pick an entry from the current `candidates` list)

### Referral data

A plugin may set `referral_data` in its response. If non-nil, Hyrouter includes it in the `ClientReferral` packet.

## Timeouts and errors

- Each plugin call runs with a fixed timeout.
- If a plugin call returns an error or times out, Hyrouter logs it and continues with the next plugin.
