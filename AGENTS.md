# Repository Instructions

## Design Priorities

- Keep the codebase minimal. Every new line should justify itself.
- Prefer idiomatic Go and standard library patterns over custom abstractions.
- Before adding ACP-specific helper code, check `github.com/coder/acp-go-sdk/helpers.go` and reuse the SDK helper when it already models the same content or update shape.
- Choose clear control flow over cleverness.
- Do not introduce layers, interfaces, or generics unless they are pulling real weight now.
- Optimize for maintainability first, then performance where measurement or protocol needs justify it.

## Type Safety

- Do not use `map[string]any`.
- Model protocol payloads, config, metadata, and internal state with explicit Go types.
- Prefer existing ACP SDK types over creating internal duplicates. Do not introduce internal wrapper structs when an upstream protocol type already fits.
- If data is structurally unknown at a boundary, contain that uncertainty at the boundary and convert it into typed values immediately.
- Avoid `any` in public or core internal APIs unless the upstream library requires it.

## Naming And Identity

- Centralize runtime identity values such as project name, binary name, agent name, log paths, and version in one place.
- Do not scatter product-name literals through the codebase.
- Versions must be normal build/runtime versions such as `dev`, semver, or injected build metadata. Do not use phase names as versions.

## Testing

- Keep repo-wide statement coverage at or above 80% at all times.
- Add tests for new behavior as part of the same change.
- Do not satisfy coverage with shallow tests that ignore meaningful behavior or error paths.

## Documentation

- Keep docs aligned with the implemented code and the current upstream ACP SDK surface.
- When implementation constraints change the practical design, update the docs in the same change.
