# Mica

Mica is an ACP-native coding agent harness in Go. The current implementation provides a minimal echo agent over stdio plus per-session ACP-native persistence.

The ACP dependency is intentionally tracked against the current `main` branch of [`github.com/coder/acp-go-sdk`](https://github.com/coder/acp-go-sdk), not the latest tagged release, so the agent stays aligned with the newest stable and unstable protocol surfaces.

## Build

```bash
go build -o mica ./cmd/mica-echo
```

## Run

```bash
./mica
```

The agent speaks line-delimited JSON-RPC over stdin/stdout and writes one append-only JSONL session file per session to `.mica/sessions/<session-id>.jsonl`.

## ACP Support

The agent currently implements:

- all stable agent methods exposed by the current ACP Go SDK `main` branch
- all unstable agent methods currently exposed by the same SDK branch

The unstable methods are implemented minimally but intentionally. Stable `SessionModelState` is returned on stable session setup/load flows. Mode and model selection are exposed as session config options; `session/set_mode` and unstable `session/set_model` are not currently supported.

## Manual Testing

With `acpx` installed:

```bash
npx --yes acpx --agent ./mica exec "hello"
```

Send a prompt and the agent will stream back `Echo: <prompt>`.

## Zed

Add the built binary as an ACP agent command in Zed's agent settings. The exact settings shape depends on the Zed version, but the command should point at the built `mica` binary.

## JSONL Format

Each session file contains:

- one ACP-shaped session header
- zero or more ACP `SessionUpdate` records

Every record carries a UUID `eventId` plus a timestamp. The header is the durable source of truth for initial session state; later `SessionUpdate` records are the durable source of truth for conversation replay and subsequent state changes.
