# Testing

## Automated

```bash
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

Coverage requirement: keep repo-wide statement coverage at or above 80% at all times.

The test suite covers:

- ACP initialize/new-session/prompt flow through the Phase 1 agent surface
- stable ACP session APIs including load-time replay plus stable `SessionModelState`
- unstable ACP session APIs including fork, list, and resume
- session config selectors for response format, mode, and model
- streamed echo updates
- per-session JSONL files with one ACP-shaped header plus typed `SessionUpdate` records using UUID `eventId` values
- `session.Log` / `session.Logs` replay and state rehydration over a swappable `SessionStore`

## Manual with `acpx`

```bash
go build -o mica ./cmd/mica-echo
npx --yes acpx --agent ./mica exec "hello from acpx"
npx --yes acpx --agent ./mica sessions new
npx --yes acpx --agent ./mica prompt "hello again"
```

Repeatable smoke run:

Run these commands sequentially. Do not fire multiple `acpx` commands in
parallel against the same active session; `acpx` itself writes local
session metadata under `~/.acpx/sessions/` and can race there.

1. Build the agent.

```bash
go build -o mica ./cmd/mica-echo
```

2. Run a one-shot exec and confirm the response is prefixed with `Echo:`.

```bash
npx --yes acpx --agent ./mica exec "smoke one"
```

3. Find the newest session file and inspect it. It should contain one header record with `sessionEvent: "new"`, then `session_info_update`, user message, and agent message update records. Every line should carry a UUID `eventId`.

```bash
ls -1t .mica/sessions | head -n 1
cat .mica/sessions/<latest-session>.jsonl
```

4. Create a persistent session for the current working directory.

```bash
npx --yes acpx --agent ./mica sessions new
```

5. Prompt that session once and confirm the default output is still `Echo: <prompt>`.

```bash
npx --yes acpx --agent ./mica prompt "named session prompt 1"
```

6. Change the response format through `session/set_config_option`, then prompt again. The next response should be the raw prompt text without the `Echo:` prefix.

```bash
npx --yes acpx --agent ./mica set response_format raw
npx --yes acpx --agent ./mica prompt "raw probe"
```

7. Change the mode and model through config selectors and confirm the commands succeed.

```bash
npx --yes acpx --agent ./mica set mode default
npx --yes acpx --agent ./mica set model echo-v1
```

8. Confirm the direct method wrappers are intentionally unsupported.

```bash
npx --yes acpx --agent ./mica set-mode default
# expected: Method not found
```

9. Re-open the newest session log and confirm later entries include `config_option_update` for `response_format`, `mode`, and `model`, plus `current_mode_update` for the mode change.

```bash
ls -1t .mica/sessions | head -n 1
cat .mica/sessions/<latest-session>.jsonl
```

Suggested checks:

- send a normal prompt and verify the streamed reply is `Echo: <prompt>`
- note that `acpx prompt` expects an existing session, while `exec` creates a one-shot session automatically
- send an empty prompt
- create a second session and verify both turns work
- inspect `.mica/sessions/<session-id>.jsonl` and confirm it contains one ACP-shaped header followed by ordered `SessionUpdate` records
- confirm each persisted line includes an `eventId` UUID rather than a synthetic sequence number
- confirm reconnect-heavy `acpx` commands do not append synthetic `session/load` lifecycle entries to the log
- do not parallelize `acpx set` or `acpx prompt` commands for the same session during smoke tests; keep the script strictly sequential

## Known Limitations

- session state is reconstructed by `session.Log` replaying persisted ACP header/update records from a `SessionStore`
- session config support is intentionally minimal
- stable `SessionModelState` is returned on `session/new` and `session/load`
- `session/set_mode` and unstable `session/set_model` are not currently supported; mode/model changes flow through `session/set_config_option`
- prompt flattening is text-first and does not attempt rich rendering for non-text content
- concurrent multi-process writes to the same session log are not coordinated in Phase 1; use sequential `acpx` commands per session
