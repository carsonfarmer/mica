# ACP Session Persistence: Minimal JSONL Event Log

## Motivation

The Agent Client Protocol (ACP) defines how clients and agents
communicate using JSON‑RPC, but it intentionally does **not** define how
sessions are persisted.\
Agents that support `session/load` must replay the entire conversation
history as `session/update` notifications, but the storage format is
left to the implementation.

This document specifies a **minimal, file‑based persistence model**
designed to:

-   Support all ACP session lifecycle methods (`new`, `load`, `resume`,
    `fork`, `stop`, `delete`, `list`).
-   Preserve events in a shape that is **as close as possible to ACP
    payloads**.
-   Enable **simple replay** of session history without transformation.
-   Remain **simple above all else**, avoiding complex crash‑recovery or
    maintenance mechanisms.
-   Allow easy migration to a database backend without changing the
    agent-facing API.

The guiding principle is:

> Persistence should be simple, append-only, and structurally aligned
> with ACP messages.

------------------------------------------------------------------------

# Storage Layout

Sessions are stored inside the working directory associated with the
session (`cwd`).

    <cwd>/
      .mica/
        sessions/
          <session-id>.jsonl
        index.json        (optional cache)

Each session corresponds to **one append‑only `.jsonl` file**.

Each line is one event represented as a JSON object.

The optional `index.json` file is a derived cache for `session/list` and
is not authoritative.

------------------------------------------------------------------------

# Event Log Model

Each session file should contain:

1.  one session header record
2.  zero or more ACP `SessionUpdate` records

------------------------------------------------------------------------

## Session Header

The first record in the file should be a single header describing how
the session began.

The header is the durable source of truth for initial session state.

It should include:

-   a session-origin discriminator such as `new` or `fork`
-   `cwd` as `string`
-   `mcpServers` as `[]McpServer`
-   `_meta` as `map[string]any`
-   initial `configOptions` as `[]SessionConfigOption`
-   initial `modes` as `SessionModeState`
-   initial `models` as `UnstableSessionModelState`
-   `sessionId` as `SessionId`
-   optional `parentSessionId` as `SessionId` for forked sessions

The header should be written once and should not be replayed as a
`session/update` notification.

If the header already captures the durable initial session facts needed
for reconstruction, implementations should not persist repeated
synthetic request and response envelopes for lifecycle methods.

These fields should reuse the existing ACP types directly rather than
introducing parallel durable-only header types.

------------------------------------------------------------------------

## Session Update Records

All subsequent records should persist ACP `SessionUpdate` values.

Examples include:

    user_message_chunk
    agent_message_chunk
    agent_thought_chunk
    tool_call
    tool_call_update
    plan
    current_mode_update
    config_option_update
    session_info_update

Session update records should mirror the original ACP `SessionUpdate`
payload shape.

This is sufficient for both:

-   replaying conversation history to the client
-   reconstructing later session state changes such as config or mode
    updates

Implementations should avoid introducing intermediate durable schemas or
parallel event formats when `SessionUpdate` already models the durable
fact.

------------------------------------------------------------------------

# Minimal Metadata

A small number of additional fields may be stored alongside ACP
payloads.

  Field            Purpose
  ---------------- ------------------------------------------------
  `eventId`        Per-event UUID for durable identity
  `ts`             Timestamp of event creation
  `sessionEvent`   Header discriminator when used on the first record

No additional structural metadata is required.

------------------------------------------------------------------------

# Design Principles

## Single Owner

Exactly one runtime component should own session rehydration, append,
replayable updates, and derived session state.

The ACP agent or protocol layer must not orchestrate persistence
directly beyond calling that session owner.

Persistence backends are storage adapters only.

------------------------------------------------------------------------

## Runtime vs Durable State

This persistence model governs durable session state only.

Transient execution state such as in-flight prompt cancellation, active
goroutines, or streaming coordination must not be treated as persisted
session state.

------------------------------------------------------------------------

## Canonical State Source

The session header is authoritative for initial session state.

Persisted `SessionUpdate` records are authoritative for subsequent
session state changes.

Implementations should not introduce separate request-side and
response-side reconstruction paths for the same derived state.

## Append‑Only Log

Session files are **append‑only**.

The header and each update record are appended as single JSON lines.

Benefits:

-   Constant‑time writes
-   Simple crash recovery
-   Natural chronological ordering
-   Easy replay

------------------------------------------------------------------------

## Crash Handling

Implementations **SHOULD NOT introduce complex crash‑recovery
mechanisms**.

If a crash occurs during a write:

1.  Read the file line by line.
2.  Ignore a trailing incomplete JSON value caused by a truncated final write.
3.  Surface other JSON syntax errors as corruption rather than silently continuing.

This approach is sufficient for typical agent workflows and preserves
the simplicity of the system.

------------------------------------------------------------------------

## Self‑Contained Sessions

Each session file must contain all information required to reconstruct
the session.

Forked sessions should write a new header and then copy the parent
session's persisted `SessionUpdate` records into the forked file rather
than referencing the parent file. This prevents dependency chains and
ensures sessions remain portable and shareable.

------------------------------------------------------------------------

# Mapping to ACP Session Methods

## session/new

Create a new session file and write a single header describing the new
session and its initial ACP-shaped state.

------------------------------------------------------------------------

## session/load

1.  Read the session log sequentially.
2.  Skip the header record.
3.  For each persisted `SessionUpdate`, emit a `session/update`
    notification.

------------------------------------------------------------------------

## session/resume

Load the session header and replay persisted `SessionUpdate` records
into the session runtime state.

Do **not** replay conversation history to the client.

------------------------------------------------------------------------

## session/fork

Create a new session file.

Write a new fork header including `parentSessionId`, then copy persisted
`SessionUpdate` records from the parent session in file order.

------------------------------------------------------------------------

## session/list

Implementations may:

-   scan `.mica/sessions/*.jsonl`
-   read `index.json` if present

The index is an optimization and must not be authoritative.

------------------------------------------------------------------------

# Persistence Interface

**Suggestion to Implementors**

Agents should access persistence through a minimal `SessionStore`
abstraction, while a single session owner is responsible for
rehydrating records into runtime session state.

``` go
type SessionStore interface {
    Create(header Header) error
    Append(cwd string, sessionId SessionId, rec UpdateRecord) error
    Load(cwd string, sessionId SessionId) (Header, []UpdateRecord, error)
    List(cwd string) ([]SessionId, error)
}
```

Possible implementations include:

-   File-based JSONL backend
-   SQLite backend
-   PostgreSQL backend

Agent logic should remain independent of the chosen backend.

Package boundaries should follow ownership boundaries:

-   protocol adapter
-   session owner
-   storage backend

If a package split is used, the protocol adapter should not import the
concrete store backend. Concrete store construction belongs in the
composition root.

------------------------------------------------------------------------

# Session Lookup Semantics

Methods such as `session/prompt`, `session/cancel`, and
`session/set_config_option` should operate on an already loaded or
in-memory session unless the protocol explicitly permits implicit load
behavior.

Implementations should not guess missing context such as `cwd` when
reloading a session for these methods.

------------------------------------------------------------------------

# Shared Session Layer Boundaries

The shared session persistence or runtime layer should not embed
agent-specific model IDs, response-format values, display labels, or
other product-specific defaults.

Such values belong in the agent or application layer and may be
projected into persisted ACP payloads there.

Known ACP header fields and `SessionUpdate` payloads should remain typed
and ACP-shaped where practical.

Implementations should avoid replacing known ACP payloads with
`map[string]any`, opaque JSON blobs, or ad hoc untyped maps merely to
reduce wrapper code.

------------------------------------------------------------------------

# Non-Goals

This persistence spec does not require, and implementations should
avoid introducing:

-   A persistence backend that also owns ACP method handling.
    The storage layer should store and retrieve session records; it
    should not implement `session/load`, `session/list`,
    `session/fork`, or other protocol behavior.

-   Multiple runtime owners for the same session.
    There should not be separate components independently
    rehydrating, mutating, or deriving session state from the same
    persisted records.

-   Separate store-level and session-level replay logic.
    Rehydration should happen in one place only.

-   Agent-specific defaults or product behavior inside the shared
    session runtime layer.
    Model IDs, response-format values, display labels, and other
    application defaults belong in the agent layer, not in the
    generic session persistence layer.

-   Unstructured persistence payloads when the ACP payload shape is
    known.
    Implementations should not replace known ACP header fields or
    `SessionUpdate` payloads with `map[string]any`, ad hoc maps, or
    opaque JSON blobs merely to reduce type definitions.

-   Runtime-only execution state in durable session structures.
    In-flight cancellation handles, goroutine coordination,
    temporary streaming state, and similar ephemeral execution
    details are not part of persisted session state.

-   Implicit session reloads for methods that lack enough context to
    reload safely.
    If a method such as `session/prompt` or `session/cancel` does
    not provide the context needed to locate durable state,
    implementations should operate on an already loaded session
    rather than guessing.

-   Additional durable schemas layered on top of ACP-shaped records
    unless strictly necessary.
    The persisted record stream should stay as close as possible to a
    single ACP-shaped header plus ACP `SessionUpdate` events.

------------------------------------------------------------------------

# Sharing and Portability

Because sessions are stored as plain JSONL files:

-   Sessions can be copied between machines.
-   Sessions can be archived or transferred easily.
-   Sessions may optionally be committed to version control.

No specialized export tooling is required.

------------------------------------------------------------------------

# Summary

This persistence model provides a **minimal, robust, and ACP-aligned
session storage format**.

Key properties:

-   Append-only JSONL event logs
-   Event structures aligned with ACP payloads
-   One self-contained file per session
-   Simple crash handling
-   Pluggable backend architecture

The system intentionally prioritizes **simplicity and portability** over
sophisticated durability features.
