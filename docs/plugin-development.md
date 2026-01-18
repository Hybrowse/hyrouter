# Plugin development

Hyrouter plugins run during the initial connection phase (after the client sends the first `Connect` packet).

There are two plugin backends:

- gRPC (out-of-process)
- WASM (in-process, executed via wazero)

Both use the same request/response model: JSON-encoded `ConnectRequest` and `ConnectResponse`.

## Data model

The JSON types are defined in `internal/plugins/types.go`.

### ConnectRequest

- `event`
  - `sni`
  - `client_cert_fingerprint` (optional)
  - `protocol_hash` (optional)
  - `client_type` (optional)
  - `uuid` (optional)
  - `username` (optional)
  - `language` (optional)
  - `identity_token_present` (optional)
- `strategy` – selected strategy (`round_robin|random|weighted`)
- `candidates` – candidate backends for the current route/pool
- `selected_index` – index chosen by Hyrouter
- `backend` – the selected backend (`host`, `port`, `weight`, `meta`)
- `referral_data` – current referral payload (optional)

### ConnectResponse

- `deny` (bool)
- `deny_reason` (string, optional)
- `candidates` (list, optional)
- `selected_index` (int, optional)
- `backend` (object, optional)
- `referral_data` (bytes, optional)

## gRPC plugins

### Protocol

Hyrouter dials a gRPC server and invokes:

- Service: `hyrouter.Plugin`
- Method: `OnConnect`

Both request and response are JSON-encoded.

### Implementing a plugin server

If your plugin is written in Go, you can reuse Hyrouter’s service descriptor:

- `internal/plugins.RegisterGRPCServer`

See `examples/grpc-plugin` for a minimal runnable plugin.

### Running the example plugin

```bash
task plugin:grpc:run
```

## WASM plugins

WASM plugins are executed in-process via wazero.

### Required exports

Your module must export:

- `alloc(len: u32) -> u32`
- `on_connect(ptr: u32, len: u32) -> u64`

The `on_connect` return value packs `resp_ptr` and `resp_len`:

- high 32 bits: `resp_ptr`
- low 32 bits: `resp_len`

Hyrouter will read `resp_len` bytes from module memory starting at `resp_ptr` and interpret them as JSON for `ConnectResponse`.

### Building the example plugin

The repository contains a Go-based WASM plugin example under `examples/wasm-plugin`.

Build it using:

```bash
task plugin:wasm:build
```

Or directly:

```bash
GOOS=wasip1 GOARCH=wasm go build -tags=examples -buildmode=c-shared -o examples/wasm-plugin/plugin.wasm ./examples/wasm-plugin
```

### Limitations and notes

- The module is expected to run under WASI.
- Hyrouter instantiates WASI (`wasi_snapshot_preview1`) and tries to use the reactor entrypoint (`_initialize`) when present.
- The plugin interface is synchronous; keep `on_connect` fast.

## Testing locally

Use the provided development configs:

- [`dev/config.dev.yaml`](dev/config.dev.yaml) (no plugins)
- [`dev/config.plugins.dev.yaml`](dev/config.plugins.dev.yaml) (gRPC deny + WASM mutate)

Run:

```bash
task run:plugins
```

Enable debug logs:

```bash
task run:plugins:debug
```
