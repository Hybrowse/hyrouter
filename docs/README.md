# Documentation

This folder contains the detailed documentation for Hyrouter.

## Hybrowse Server Stack

Hyrouter is part of the **Hybrowse Server Stack**:

- [Hybrowse/hytale-server-docker](https://github.com/Hybrowse/hytale-server-docker) — Docker image for dedicated servers
- [Hybrowse/hytale-session-token-broker](https://github.com/Hybrowse/hytale-session-token-broker) — non-interactive server authentication for providers/fleets

## Getting started

- [`configuration.md`](configuration.md) – configuration file schema (YAML / JSON)
- [`routing.md`](routing.md) – static routing rules (SNI-based)
- [`plugin-configuration.md`](plugin-configuration.md) – plugin configuration, ordering and stages
- [`referral-envelope.md`](referral-envelope.md) – referral envelope format and signing

## Internals

- [`architecture.md`](architecture.md) – high-level architecture and connection flow

## Plugin authoring

- [`plugin-development.md`](plugin-development.md) – how to build gRPC and WASM plugins

## Troubleshooting

- [`troubleshooting.md`](troubleshooting.md) – common issues and debugging tips
