# Mica - ACP-Native Agentic Coding Harness

## Overview

Mica is an Agent Client Protocol (ACP) server implementation in Go, designed as a reusable SDK for building coding agents that integrate seamlessly with any ACP-compatible client (Zed, JetBrains, etc.).

## Design Philosophy

Mica embraces radical simplicity and efficiency:

- **Idiomatic Go**: Clean, readable Go code following standard conventions and best practices
- **Extremely Simple**: Minimal abstractions, straightforward control flow, easy to understand
- **Minimal Codebase**: Aggressive focus on keeping line count low - every line must justify its existence
- **Fast**: Optimized for speed and low latency in agent responses
- **Token-Efficient**: Minimizes LLM token usage through smart context management and prompting
- **Open Weights Optimized**: Designed to work excellently with open source models (not just frontier models)
- **Tiny Binary**: Small build artifacts, fast compilation, minimal dependencies
- **Beautiful Code**: Code that is a pleasure to read and maintain - clarity over cleverness

**Guiding Principle**: If there's a simpler way to do it, do it that way.

## Core Goals

### Architecture Principles

- **ACP Server (Agent Side)**: Implements the agent interface from the Agent Client Protocol
- **Zero Direct Capabilities**: The agent orchestrates LLM interactions but has NO direct file system or terminal access
- **Client-Provided Tools**: ALL file operations (`fs/read_text_file`, `fs/write_text_file`) and terminal operations (`terminal/*`) are provided by the client
- **JSON-RPC Communication**: Agent calls client methods via JSON-RPC, client executes them and returns results
- **SDK-First Design**: Exportable Go library (`github.com/carsonfarmer/mica`) that others can extend

### Technical Stack

- **ACP Protocol**: `github.com/coder/acp-go-sdk` for protocol implementation, tracked against current `main` during early development
- **LLM Providers**: `github.com/charmbracelet/fantasy` for model provider layer
- **Inspiration**: Design patterns from `github.com/charmbracelet/crush` and `github.com/MiniMax-AI/Mini-Agent`

### Current Foundation

1. **ACP-Native Session Persistence**
   - One append-only JSONL session file per session under `.mica/sessions/`
   - The session file is the single durable source of truth for that session
   - Stores one ACP-shaped header plus session-scoped `SessionUpdate` records
   - Supports restart-safe replay for `session/load` and state rebuild for `session/resume`

2. **Minimal Session Configuration**
   - Response format, mode, and model selection flow through `session/set_config_option`
   - Stable `SessionModelState` is returned on stable `session/new` and `session/load`
   - Direct `session/set_mode` and unstable `session/set_model` are intentionally unsupported in Phase 1

3. **Streaming ACP Surface**
   - Streaming-native responses via `session/update` notifications
   - Stable and unstable ACP session methods implemented against the current SDK surface
   - Echo-agent behavior kept isolated as scaffolding for later phases

4. **Universal Client Support**
   - Works with any ACP-compatible client
   - Primary target: Zed editor
   - CLI verification with `acpx`

5. **Future Expansion Areas**
   - Multi-provider model support
   - Agent skills
   - Client tool execution
   - Additional ACP clients

## Libraries

- https://github.com/coder/acp-go-sdk
  - Go SDK for the Agent Client Protocol (ACP), offering typed requests, responses, and helpers so Go applications can build ACP-compliant agents, clients, and integrations
  - This will be our primary agent SDK layer (our SDK is ACP native)
  - The types here represent our primary agent SDK types (our SDK is ACP native)
  - The types here represent our primary persistence types (we only persist ACP-compatible data/types)
  - We will start with a pure agent-side design/implementation that supports streaming
  - We will only expose ACP-compatible fs/ APIs (read, write), and the full terminal/* set of APIs but we don't implement these tools directly
  - We will expose agent skills via the available commands messages, and the builtin tools via the core API to clients that support them

- https://github.com/charmbracelet/fantasy
  - Multiple providers, multiple models, one API
  - This should be our primary "provider" layer
  - Focus only on anthropic and openaicompat providers
  - We will use their built-in agent to start, see https://github.com/charmbracelet/fantasy/blob/main/examples/stream/main.go
  - We will only support streaming, we want our tools to run in parallel

- https://github.com/charmbracelet/catwalk
  - A collection of LLM inference providers and models
  - Based on the types from https://charm.land/crush.json
  - For now, maybe just use this to fetch providers and models "manually"

- https://charm.land/crush.json
  - Useful json schema definition to explore
  - We could use the main Config type (mostly limited to the models, providers, options, permissions fields)
  - Focus mostly on the ProviderConfig, ModelOptions, Model, Permissions, SelectedModel, and Options (mostly limited to thge context_paths, skills_paths, debug, data_directory fields ) types

- https://github.com/charmbracelet/crush
  - Go-based full coding agent harness
  - Use this for ideas and inspiration
  - Look at handling and testing skills https://github.com/charmbracelet/crush/blob/main/internal/skills/skills.go
  - Look at handling config options (maybe overly complicated) https://github.com/charmbracelet/crush/blob/main/internal/config/config.go
  - Look for how crush handles AGENTS.md and CRUSH.md files

- https://github.com/charmbracelet/x
  - Experimental utility packages, possibly useful, possibly not

- Others updated as needed

## Development Phases

### Phase 1: Core Protocol & Echo Agent

**Goal**: Prove the ACP plumbing works

**Deliverables**:
- Implement `coder/acp-go-sdk` agent interface
- Stay aligned with the current ACP Go SDK `main` branch
- Stdio transport (JSON-RPC over stdin/stdout)
- Initialize handshake with capabilities
  - Required ACP lifecycle methods (`authenticate`, `new`, `prompt`, `cancel`, `set_config_option`)
  - Implement the additional stable and unstable agent methods exposed by the current SDK surface, even if behavior is initially minimal
  - Echo agent: receives prompt, streams back "Echo: {prompt}"
  - Per-session ACP-native JSONL persistence with `session/load` replay support

**Manual Test**: Run with `acpx` CLI or Zed, verify echoing works

---

### Phase 2: LLM Integration

**Goal**: Real LLM responses with no tools

**Deliverables**:
- Integrate `charmbracelet/fantasy` for model providers
- Simple prompt → LLM → stream response chunks via `session/update`
- Model selection changes via `session/set_config_option`
- Support Anthropic + OpenAI providers
- Optional: Either hardcode a set of available models to choose from, or support something like the model config setup mentioned above

**Manual Test**: Again using `acpx`, have conversation with a live model, and switch models mid-session

---

### Phase 3: Client Tool Calls

**Goal**: Agent requests file/terminal operations from client

**Deliverables**:
- Implement client method calls: `fs/read_text_file`, `fs/write_text_file`
- Implement terminal methods: `terminal/create`, `terminal/output`, etc.
- Tool call status tracking (pending → in_progress → success/error)
- Permission requests via `session/request_permission`

**Manual Test**: Agent reads files, writes code, runs terminal commands via Zed

---

### Phase 4: Agent Skills System

**Goal**: Extensible agent behaviors

**Deliverables**:
- Borrow skill pattern from crush/Mini-Agent
- Skills as composable prompt templates + tool orchestrations
- Built-in skills: code_review, debug, refactor, test_generation
- Custom skill loading from config

**Manual Test**: Use coding agent with different skills in Zed

---

### Phase 5: Email Client

**Goal**: Demonstrate ACP client flexibility

**Deliverables**:
- Go email client implementing ACP client interface
- Maps emails to prompts, responses to email replies
- Limited tools: read/write local files, simple terminal

**Manual Test**: Send/receive coding requests via email

## Project Structure

```
mica/
├── go.mod                    # Module: github.com/carsonfarmer/mica
├── pkg/
│   ├── agent/              # ACP protocol adapter and echo-specific config helpers
│   └── session/            # Session ownership, replay, and storage backends
│       └── store/
│           └── file.go     # File-backed SessionStore
├── cmd/
│   └── mica-echo/          # Echo agent executable (Phase 1)
└── internal/
    └── app/                # Shared app metadata and paths
```

## Testing Strategy

- **Unit Tests**: Core functionality in each package
- **Integration Tests**: Full protocol flows
- **Manual Testing**: Primary verification method
  - Use `acpx` CLI for rapid iteration
  - Use Zed for real-world coding scenarios
  - Use custom clients to verify protocol compliance

## Success Criteria

1. **Works with any ACP client**: Zed, JetBrains, custom clients
2. **Zero vendor lock-in**: Model providers are swappable
3. **SDK reusability**: Others can build their own versions
4. **Production-ready persistence**: per-session ACP header/update streams enable replay and future backend changes
5. **Extensible**: Skills system allows customization without core changes
6. **Stays minimal**: Core codebase remains under 2000 lines of Go
7. **Fast builds**: Binary compiles in seconds, size under 10MB
8. **Token efficient**: Demonstrably lower token usage than comparable agents
9. **Open weights ready**: Works well with Llama, Qwen, and other OSS models

## Future Extensions

- Additional client implementations (Slack bot, Discord bot, web UI)
- Advanced agent orchestration (multi-agent collaboration)
- MCP server integration for external tool access
- Cloud deployment support for remote agents
