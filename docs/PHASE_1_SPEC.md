# Phase 1: Core Protocol & Echo Agent - Detailed Specification

## Objectives

1. Establish Go module structure as reusable SDK
2. Implement ACP agent interface using the current `main` branch of `coder/acp-go-sdk`
3. Build echo agent that demonstrates protocol compliance
4. Implement per-session ACP-native JSONL persistence
5. Test with `acpx` CLI client

The echo agent in this phase is scaffolding only.

It exists to validate protocol compliance, session persistence shape,
and the basic package boundaries needed for later phases.

Implementers should not overfit the long-term architecture to the echo
agent's behavior, defaults, or narrow feature set. Later phases are
expected to replace most of the echo-specific application logic.

## Code Quality Expectations

**Keep it ruthlessly simple:**

- **Idiomatic Go**: Use standard library patterns, follow Go conventions
- **Minimal abstractions**: Avoid unnecessary interfaces, generics, or complexity
- **Small codebase**: Keep the implementation as small as practical for the required protocol surface
- **Clear over clever**: Readable code beats clever code
- **No premature optimization**: But write naturally efficient code
- **Beautiful structure**: Code should be easy to navigate and understand

**Before adding any code, ask**: "Is there a simpler way to do this?"

Also ask: "Is this a Phase 1 protocol scaffold, or am I accidentally
turning the echo agent into the long-term design?"

## Architecture

```
mica/
├── go.mod (module: github.com/carsonfarmer/mica)
├── pkg/
│   ├── agent/          # ACP protocol adapter
│   │   ├── agent.go
│   │   ├── config.go
│   │   └── helpers.go
│   ├── session/        # Session ownership and replay
│   │   ├── session.go
│   │   └── store/
│   │       └── file.go # File-backed SessionStore
│   ├── provider/       # LLM provider abstraction (Phase 2)
│   └── skill/          # Skill system (Phase 4)
├── cmd/
│   ├── mica-echo/      # Echo agent executable (Phase 1)
│   │   └── main.go
│   └── mica/           # Full agent (Phase 2+)
│       └── main.go
├── internal/
│   └── testutil/       # Testing utilities
└── examples/
    └── client/         # Example Go client (future)
```

## Dependencies

- `github.com/coder/acp-go-sdk` - ACP protocol implementation
- Use the current `main` branch/pseudo-version rather than lagging behind a tagged release during protocol exploration
- Standard library for JSONL persistence

## Implementation Details

### 1. Agent Interface (`pkg/agent/agent.go`)

- Implement `acp.Agent` interface from SDK
- Implement `acp.AgentLoader`
- Implement the unstable agent methods currently exposed on `main` of `coder/acp-go-sdk`
- Handle `Authenticate()` with a no-op success response unless auth is later configured
- Handle `Initialize()` - return agent capabilities (no fs/terminal in Phase 1)
- Handle `NewSession()` - create new session with ID
- Handle `Prompt()` - echo the prompt back
- Handle `LoadSession()` with minimal persisted-session support
- Replay persisted session history back to the client during `LoadSession()` via `session/update` notifications, as required by ACP
- Handle `SetSessionConfigOption()` with a minimal typed response
- Handle `Cancel()` - graceful stop for in-flight prompt work
- Leave `SetSessionMode()` unsupported in Phase 1; mode changes should flow through `session/set_config_option`
- Handle unstable session fork/list/resume requests with minimal but valid responses to start
- Leave unstable `session/set_model` unsupported in Phase 1; model changes should flow through `session/set_config_option`

### 2. Session Management (`pkg/session/session.go`)

- `Log` with runtime state derived from one persisted header plus persisted `SessionUpdate` records
- Track replayable session updates rather than synthetic lifecycle envelopes
- `Logs` owner for cached session lookup, creation, fork, and list/load orchestration over a `SessionStore`
- Keep prompt cancellation in the protocol adapter layer rather than the durable session owner

### 3. JSONL Persistence (`pkg/session/store/file.go`)

- Write one JSONL file per session under `.mica/sessions/{sessionID}.jsonl`
- The session file is the single durable source of truth for that session
- Persist only session-scoped ACP records needed to replay and rehydrate session state
- The first line is a typed ACP-shaped session header as defined in `docs/SESSION_PERSISTENCE_SPEC.md`
- All later lines are typed ACP `SessionUpdate` records with durable metadata
- Append-only, survives restarts

### 4. Echo Agent (`cmd/mica-echo/main.go`)

- Minimal main() that instantiates agent and starts connection
- On prompt: stream back via `session/update` with properly formatted chunks.
- Then return `PromptResponse{StopReason: end_turn}`

Important constraint:

- The echo agent is not the product architecture.
- Do not treat echo-specific config values, model IDs, prompt behavior,
  or UX decisions as canonical for later phases.
- Keep echo-specific behavior isolated so later phases can replace it
  without restructuring the persistence or session ownership model.

## Critical: Exploration with `acpx`

**Before finalizing the implementation, you MUST experiment with `acpx` to understand the protocol:**

### 1. Install and explore acpx

```bash
# Install acpx (check latest installation method)
# Run it to see available commands/options
acpx --help
```

### 2. Test with existing ACP agents

```bash
# Try connecting to a known agent (if available) to see expected behavior
# Observe the JSON-RPC message flow
# Pay attention to session lifecycle, streaming, etc.
```

### 3. Experiment with your echo agent during development

```bash
# Build and test iteratively
go build -o mica ./cmd/mica-echo
acpx --agent ./mica

# Try different scenarios:
# - Send simple prompts
# - Send very long prompts
# - Send multiple prompts in same session
# - Try to cancel mid-response (Ctrl+C)
# - Start new sessions
# - Send special characters, unicode, emojis
# - Send empty prompts
# - Rapid-fire multiple prompts
```

### 4. Inspect the JSONL logs

```bash
# After each experiment, look at the logs
cat .mica/sessions/{session-id}.jsonl | jq

# Verify:
# - All messages captured
# - Timestamps correct
# - Message structure matches SESSION_PERSISTENCE_SPEC.md
```

### 5. Edge cases to explore

- What happens if you kill `acpx` mid-session?
- What happens if you send malformed JSON-RPC?
- What happens if session directory doesn't exist?
- What happens with concurrent sessions (if acpx supports it)?
- What happens on agent restart - can it load old sessions?

### 6. Compare your implementation

- Look at `coder/acp-go-sdk` examples
- Compare your JSONL output to expected format
- Verify streaming behavior matches protocol spec

### Document your findings

Create a `TESTING.md` file with:
- How to use acpx with mica
- Interesting edge cases discovered
- Known limitations
- Example session transcripts

## Acceptance Criteria

### 1. Protocol Compliance

- `acpx` can connect to agent via stdio
- Handshake succeeds with proper capabilities
- Session creation returns valid session ID
- Agent responds to prompts with streamed updates

### 2. Persistence

- JSONL file created in `.mica/sessions/`
- Session-owned messages logged according to SESSION_PERSISTENCE_SPEC.md
- File survives agent restart
- Can replay session from JSONL

### 3. Testing Commands

```bash
# Build agent
go build -o mica ./cmd/mica-echo

# Test with acpx
acpx --agent ./mica
# Should see prompt, type message, get "Echo: {message}" back

# Verify JSONL created
cat .mica/sessions/{session-id}.jsonl
```

### 4. Manual Verification in Zed

- Configure Zed to use `./mica` as agent
- Send prompts, verify echo responses
- Check JSONL logging works

## Error Handling

- Graceful shutdown on SIGINT/SIGTERM
- Close JSONL files properly
- Invalid JSON-RPC requests return proper error responses
- Persistence failures should surface clearly rather than silently dropping ACP session records

## Documentation

Required documentation files:

### README.md

- Project overview
- Build/run instructions
- Example `acpx` usage
- How to configure in Zed
- ACP header/update JSONL format documentation

### TESTING.md

- How to test with `acpx`
- Exploration findings and insights
- Known edge cases
- Example session transcripts
- Debugging tips

### Examples in README

```bash
# Build
go build -o mica-echo ./cmd/mica-echo

# Run with acpx
acpx --agent ./mica-echo

# Configure in Zed (add to settings.json)
{
  "agents": {
    "mica-echo": {
      "command": "/path/to/mica-echo"
    }
  }
}
```

## Non-Goals for Phase 1

These are explicitly OUT OF SCOPE:

- ❌ No LLM integration yet
- ❌ No file system operations
- ❌ No terminal operations
- ❌ No permission requests
- ❌ No agent skills
- ❌ No model provider abstraction
- ❌ No direct `session/set_mode` API support beyond minimal config-driven mode state

## Success Definition

Phase 1 is complete when:

1. ✅ Echo agent successfully runs with `acpx`
2. ✅ Each session persists as one ACP-shaped header plus ordered `SessionUpdate` records in JSONL
3. ✅ Agent can be configured and used in Zed
4. ✅ Comprehensive testing documented in `TESTING.md`
5. ✅ Code is well-structured and ready for Phase 2 LLM integration
6. ✅ Code remains minimal, readable, and easy to replace in later phases
7. ✅ Binary builds cleanly and the code passes standard formatting and test checks
8. ✅ No unnecessary dependencies beyond `coder/acp-go-sdk`

## Alignment Notes

- Phase 1 should implement the full current `acp.Agent` interface for protocol compatibility, even where behavior is intentionally minimal.
- Phase 1 should also implement the current `acp.AgentLoader` and unstable agent methods exposed by the SDK `main` branch so the project tracks the real protocol surface, not a stale tagged subset.
- Persistence in Phase 1 is per-session ACP header/update storage only as per the separate spec document.

## Key Learning Goals

This phase should teach you:

- How ACP protocol works in practice
- The JSON-RPC message flow and lifecycle
- How agents and clients communicate
- What streaming looks like in the protocol
- How to structure a reusable Go SDK
- How to test ACP agents effectively

## Next Steps After Phase 1

Once Phase 1 is complete and you've thoroughly explored the protocol with `acpx`:

1. Review the JSONL logs to understand header/update patterns
2. Identify which parts of the code will need to change for LLM integration
3. Document any protocol quirks or surprises discovered
4. Prepare for Phase 2 by understanding where `charmbracelet/fantasy` will fit in

---

**Remember**: This phase is about learning the protocol deeply through hands-on experimentation with `acpx`. Don't rush to "completion" - take time to explore, experiment, and understand how ACP works in practice.
