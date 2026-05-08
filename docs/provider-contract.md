# StacyVM Provider Contract

Providers are the runtime boundary for StacyVM. The orchestrator, API, SDKs, and
dashboard must be able to treat every provider the same way, whether the runtime
is Docker, Firecracker, PRoot, E2B, a custom HTTP service, or a test double.

The Go source of truth is `internal/providers/provider.go`. This document
spells out the behavioral contract expected by the shared provider conformance
tests.

## Lifecycle

- `Name` returns a stable unique identifier used in config, API responses, and
  persisted sandbox records.
- `Spawn` creates a running sandbox and returns a non-empty sandbox ID.
- `Status` returns `running` for an active sandbox.
- `Destroy` tears down runtime resources. Providers may return `ErrSandboxNotFound`
  if the runtime object is already gone.
- After destroy, exec and file operations must fail with either
  `ErrSandboxDestroyed` or `ErrSandboxNotFound`.
- Provider implementations should be best-effort idempotent around external
  cleanup. The orchestrator may call destroy during TTL reaping, explicit API
  requests, or recovery flows.

## Exec

- `Exec` runs the requested command and returns stdout, stderr, and exit code.
- Exec mode is explicit:
  - Empty mode and `shell` run `Command` through `/bin/sh -c`; `Args` are
    shell-quoted and appended for backwards compatibility.
  - `argv` runs `Command` directly with `Args` as literal process arguments.
    Providers must not invoke a shell in this mode.
- A nonzero command exit is not a provider error. It must return an `ExecResult`
  with the nonzero exit code.
- Provider errors are reserved for runtime failures: missing sandbox, provider
  unavailable, transport failure, timeout, or invalid provider state.
- Context cancellation and deadlines should be honored. Deadline expiration
  should map to `ErrExecTimeout` where the provider can detect it.
- `ExecStream` emits stdout/stderr chunks and closes its channel when the command
  finishes or the stream fails.

## Files

- File paths are interpreted inside the sandbox filesystem.
- Providers must support write, read, list, delete, move, chmod, stat, and glob.
- `WriteFile` should create missing parent directories when the runtime can do so.
- File reads return an `io.ReadCloser`; callers own closing it.
- Missing sandbox errors should use `ErrSandboxNotFound` or `ErrSandboxDestroyed`.
- Missing file behavior can remain provider-specific unless an API route maps it
  into a user-facing error.

## Health

- `Healthy` should be fast and side-effect free.
- It should return false when the runtime dependency is unreachable, for example
  Docker daemon unavailable, PRoot binary missing, Firecracker binary missing, or
  a custom HTTP backend failing its health endpoint.

## Typed Errors

Providers should use these sentinel errors from `internal/providers/errors.go`:

- `ErrSandboxNotFound`
- `ErrSandboxDestroyed`
- `ErrProviderNotFound`
- `ErrProviderUnavailable`
- `ErrExecTimeout`
- `ErrResourceLimit`

Wrapping is encouraged with `fmt.Errorf("context: %w", err)` so callers can use
`errors.Is`.

## Conformance Tests

Shared conformance tests live in
`internal/providers/provider_conformance_test.go`.

Current coverage:

- Mock provider
- Docker provider, when Docker is available
- Custom provider through an in-process fake HTTP backend

Future providers should be wired into the same harness whenever their runtime
dependencies are available.
