# Troubleshooting

## Enable debug logging

Hyrouter defaults to `info` logging. For detailed inspection of packet flow and stream behavior, run with:

```bash
hyrouter -config config.yaml -log-level debug
```

Or:

```bash
task run:debug
```

## “No packages found” in the `examples/` folder

The `examples/` directory uses Go build tags (`-tags=examples`). Many editors will show diagnostics unless the tag is enabled.

Example build/run commands in this repository already pass `-tags=examples`.

## Client stuck on “Connecting…”

Common causes:

- A plugin denies the connection but the stream is not closed.
- A plugin blocks and the router never sends `Disconnect` or `ClientReferral`.

Current Hyrouter behavior:

- If a plugin denies, Hyrouter sends `Disconnect` (packet ID `1`) and closes the stream.
- Each plugin call runs with a fixed timeout. If the plugin does not respond, Hyrouter logs an error and continues.

## Plugin errors

If a plugin returns an error (or times out), Hyrouter continues with the next plugin.

When debugging:

- Run with `-log-level debug`.
- Confirm that your plugin process is reachable (gRPC) or that the `.wasm` file exists and exports the required functions.

## Docker build fails with missing config

The Docker image expects a config file at `/app/config.yaml`.

Recommended pattern:

- Mount your config into the container.
- Or bake a config into the image by editing the Dockerfile.

## Hytale Update 7

Update 7 widened `Connect.Language` to 100 characters. Hyrouter accepts this
field when choosing routes and calling plugins, while retaining support for the
older hash-based Connect format. The regression tests use synthetic packets and
exercise the referral over a local QUIC connection.

The [official Update 7 notes](https://hytale.com/news/2026/10/modder-facing-patch-notes-update-7)
explain the protocol change. These checks do not replace joining through Hyrouter
with the matching Hytale client and server build.
