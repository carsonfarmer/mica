# Mica — Agent Guidelines

## Hard Rules

### 0. No commits without explicit approval.

Never run `git commit` or `git push` unless the user has explicitly instructed
you to do so in the current turn. A prior instruction to commit does not carry
forward. If in doubt, ask.

## References

- [ACP schema](https://github.com/agentclientprotocol/agent-client-protocol/tree/main/schema) — authoritative JSON Schema definitions for the protocol
- [ACP documentation](https://agentclientprotocol.com) — protocol overview and specification
- [`go-acp-sdk`](https://github.com/carsonfarmer/go-acp-sdk) — the ACP Go SDK that provides all protocol types, transport, connection, and helper utilities

## Design Principles

These principles govern every decision in this codebase. Ranked by priority.

### 0. Pre-1.0 — breaking changes are always fine.

This project has not reached 1.0. All APIs are unstable. Breaking changes
require no migration path, no deprecation cycle, no backward-compatibility
shims. Change the interface, rename the function, delete the type — callers
update or they don't. We optimize for the right design, not for existing
consumers.

### 1. Minimalism over features.

The least code that does the job, no more, no less. Every line must justify its
existence. If a feature isn't needed _right now_, it doesn't go in. "Future
flexibility" abstractions are banned — wait for the concrete need.

### 2. Lean on the SDK.

Mica is an agent harness, not a protocol library. All ACP types, transport,
connection management, and protocol helpers come from `go-acp-sdk`. Never
duplicate types or interfaces that the SDK already provides. If the SDK is
missing something, add it there — not here.

### 3. Correct-by-construction, not defense-in-depth.

Code must be correct at the type level and at the contract level. The caller
trusts the callee's guarantees; the callee does not re-verify them. "Harmless
defense" (nil checks after a function that can't return nil, empty-data skips
after a transport that filters them) is an antipattern — it hides bugs, bloats
code, and erodes trust in contracts. Write code such that invalid states are
unrepresentable.

### 4. Strong typing, minimal `map[string]any`.

Use explicit structs for every domain type. Use the SDK's protocol types for
ACP payloads. `any` is tolerated only where the spec or SDK demands it.

### 5. Minimal dependencies.

The core agent should remain standard-library only beyond `go-acp-sdk`.
Optional provider or tool packages may use focused, mature dependencies when
justified. No external JSON libraries, logging frameworks, or utility packages.

### 6. Modern Go idioms.

Use generics where they reduce boilerplate. Use type aliases for domain
scalars. Use interfaces for optional capabilities, not feature flags.

### 7. Borrow the best, leave the rest.

Study existing ACP agents (Beaver, coder/acp-go examples) and take what works.
Discard what doesn't. No allegiance to any one project's design. Mica should
develop its own design identity.

## What NOT to Do

- **No code generation.** Period.
- **No defense-in-depth.** Don't nil-check a value that can't be nil by
  construction. Don't re-validate a guarantee the callee already provides.
- **No future-proofing abstractions.** Don't add interfaces or indirection
  for hypothetical use cases.
- **No unnecessary external dependencies.** Keep the core stdlib-only + SDK;
  optional packages may use focused dependencies when justified.
- **No `map[string]any`** where a typed struct can exist.
- **No feature flags or capability checks inside the library.** Optional
  features are surfaced as optional Go interfaces — the type system does the
  gating.
- **Never duplicate SDK types.** If `go-acp-sdk` has it, use it. If it's
  missing, add it to the SDK first.
- **Never write migration code or backward-compatibility shims.** No format
  version checks, no "read old shape, convert to new," no dual-read paths, no
  deprecated-field aliases. When a format or type changes, the new code reads
  only the new shape. Old data is discarded. This project has no users yet;
  treat every breaking change as free.

## Build

- **Output to `bin/`.** Always use `-o bin/<name>` when building binaries:
  ```sh
  go build -o bin/mica ./cmd/mica
  ```
  Never run bare `go build` at the repo root — it drops artifacts in-tree.
- `bin/` is gitignored.

## Testing

- **Coverage target: 80%.** Every new feature ships with tests. Untestable
  goroutine-coordination code (Close, request/response correlation, cancel
  flows) MUST have dedicated tests — these are the most failure-prone areas.
- Run with `-race` on every change. Agent coordination involves concurrent
  goroutines — races are real.
- If code is legitimately hard to test (spawned processes, OS-level I/O),
  flag it for review rather than writing a worthless test to pad coverage.

## Style

- Files are organized by domain: agent, session, tools, provider, config,
  context, skills.
- Constructors start with `New` for core types.
- Constants for all protocol and domain string literals.

## Maintenance

- **Format and vet on every change.** `go fmt ./... && go vet ./...` before
  committing.
- **Race detector always.** `go test -race -count=1 ./...` — agent
  coordination is concurrent and correctness depends on goroutine coordination.
- **Test before commit.** Full test suite with coverage check:
  `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | grep total`.
  Aim for ≥80% on core packages.
