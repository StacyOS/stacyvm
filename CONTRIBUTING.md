# Contributing to StacyVM

Thanks for considering a contribution. StacyVM is MIT-licensed, vendor-neutral, and built in the open. The most useful contributions are:

1. **New providers** (Kata, Podman, K3s, Nomad, …)
2. **SDK improvements** (more languages, ergonomics, error handling)
3. **Documentation** (examples, tutorials, fixing inaccuracies)
4. **Bug fixes and tests** (anything that improves reliability)
5. **Performance work** (benchmarks live in `scripts/benchmark.sh`)

This document covers everything you need: how to set up, what we expect from PRs, where things live, and how reviews work.

---

## Table of contents

- [Code of conduct](#code-of-conduct)
- [Quick contributor setup](#quick-contributor-setup)
- [Project layout](#project-layout)
- [Where to put what](#where-to-put-what)
- [Development workflow](#development-workflow)
- [Testing](#testing)
- [Coding style](#coding-style)
- [Commit messages](#commit-messages)
- [Pull requests](#pull-requests)
- [Adding a new provider](#adding-a-new-provider)
- [Adding a new SDK language](#adding-a-new-sdk-language)
- [Updating the API surface](#updating-the-api-surface)
- [Documentation contributions](#documentation-contributions)
- [Reporting bugs](#reporting-bugs)
- [Reporting security issues](#reporting-security-issues)
- [Licensing](#licensing)

---

## Code of conduct

Be kind. Assume good faith. Critique code, not people. Disagreement is fine; condescension and personal attacks are not. Maintainers reserve the right to remove comments, close issues, and block contributors who don't follow this.

---

## Quick contributor setup

Prerequisites:

- **Go** 1.25+
- **Node** 18+ (for the JS SDK and the web dashboard)
- **Python** 3.9+ (for the Python SDK)
- **Docker** 24+ (most provider tests use it)
- **Make** (build orchestration)
- *(Optional)* **KVM** + **Firecracker** binary if you're touching the Firecracker provider

Get a working tree:

```bash
git clone https://github.com/StacyOs/stacyvm
cd stacyvm
./scripts/setup.sh        # checks toolchain, downloads kernel + Firecracker
make build-all            # builds the server + agent
./stacyvm serve           # smoke-test
```

Run the test suite:

```bash
make test                 # Go tests
make web                  # build the React dashboard
cd sdk/python && pytest   # Python SDK tests
cd sdk/js     && npm test # TypeScript SDK tests
```

If `./scripts/setup.sh` fails, copy the failure output into a new issue tagged `setup` — that's a high-priority fix for us.

---

## Project layout

```
stacyvm/
├── cmd/                    # CLI entrypoints
│   ├── stacyvm/            # The main CLI binary
│   └── stacyvm-agent/      # Guest agent that runs inside Firecracker VMs
├── internal/
│   ├── api/                # HTTP handlers (chi router) — match docs/api.md
│   ├── orchestrator/       # Lifecycle, TTL, templates, pool, event bus
│   ├── providers/          # docker, firecracker, e2b, custom, proot, mock
│   ├── config/             # Viper-based config loader
│   └── auth/               # Optional API-key auth middleware
├── sdk/
│   ├── js/                 # TypeScript SDK
│   └── python/             # Python SDK (sync + async)
├── web/                    # React + Vite dashboard
├── tui/                    # Terminal UI (bubbletea)
├── docs/                   # Architecture write-ups, OpenAPI spec, API reference
├── scripts/                # setup.sh, build-rootfs.sh, install.sh, benchmarks
├── examples/               # Working code samples (js, python)
├── tests/                  # Integration tests (live against a real server)
├── docker-compose.yml      # StacyVM + Traefik
└── Makefile                # build, test, web, release-build
```

---

## Where to put what

| Change type | Goes in | Notes |
|---|---|---|
| New REST endpoint | `internal/api/` | Update `docs/api.md` and `docs/swagger.yaml` in the same PR |
| New provider | `internal/providers/{name}/` | Implement the `Provider` interface; register in config |
| Provider config option | `internal/config/` | Add to schema + document in main README |
| SDK method (Python) | `sdk/python/stacyvm/sandbox.py` (+ `async_sandbox.py`) | Mirror sync/async signatures |
| SDK method (TS) | `sdk/js/src/sandbox.ts` | Add to `index.ts` exports |
| New error class | `errors.ts` / `exceptions.py` | Mirror across both SDKs; document in API reference |
| Architecture decision | `docs/{topic}.md` | One markdown file per topic, linked from the README |
| One-off script | `scripts/` | Shell or Python; needs a top comment explaining usage |
| Integration test | `tests/` | Spawns a real server; document any external deps |

If your change doesn't fit any of these, open an issue first to discuss placement.

---

## Development workflow

Most contributions follow this loop:

```bash
# 1. Branch from main
git checkout -b feat/my-feature

# 2. Make the change
$EDITOR internal/api/handlers.go

# 3. Verify locally
make lint                                # go vet
make test                                # Go unit tests
go test ./internal/api/...               # focused tests
./stacyvm serve &                        # spin up server
curl http://localhost:7423/api/v1/health # smoke test

# 4. Update related artifacts (see "Where to put what")
$EDITOR docs/api.md
$EDITOR sdk/python/stacyvm/sandbox.py
$EDITOR sdk/js/src/sandbox.ts

# 5. Commit + push + open a PR
git commit -m "api: add /v1/sandboxes/{id}/snapshot"
git push -u origin feat/my-feature
gh pr create
```

For larger features (new provider, SDK rewrite, breaking API change), open a discussion or draft PR early so we can align on the approach before you write a thousand lines.

---

## Testing

We don't enforce 100% coverage, but every change should include tests where it's reasonable. Three layers:

### Go unit tests
```bash
make test                              # run everything
go test ./internal/orchestrator/...    # one package
go test -run TestSpawn ./...           # one test
go test -race ./...                    # race detector
```

### SDK tests
```bash
# TypeScript
cd sdk/js && npm test                  # vitest

# Python
cd sdk/python && pytest                # uses pytest-httpx for fakes
cd sdk/python && pytest -k pool        # filter
```

### Integration tests
```bash
# tests/ runs against a real server
./stacyvm serve &
SERVER_URL=http://localhost:7423 go test ./tests/...
```

If you're adding behaviour that depends on Docker or Firecracker, mark the test so it skips cleanly when the dependency is missing (`testing.Skip`).

---

## Coding style

### Go
- `gofmt` everything (most editors do this on save).
- `go vet` must pass — `make lint` runs it.
- Errors wrap with `fmt.Errorf("doing X: %w", err)`. Don't swallow, don't `log.Fatal` outside `main`.
- Name HTTP handlers `handleVerb` (e.g. `handleSpawn`, `handleListFiles`) for grep-ability.
- Provider methods receive a `context.Context` first; use it for cancellation.

### TypeScript
- TypeScript strict mode is on. Don't add `any` without a comment explaining why.
- Two-space indent, semicolons, double quotes — Prettier defaults.
- Public API uses `camelCase`. Wire-format types match the server (`snake_case`).
- Re-export everything users need from `src/index.ts`.

### Python
- Type hints required on public APIs.
- 4-space indent, `black`-style formatting (line length 100).
- Sync (`Client`) and async (`AsyncClient`) APIs must stay in sync — same method names, same signatures.
- `snake_case` for methods and module-level functions.

### Markdown
- One sentence per line in long-form prose makes diffs cleaner.
- Code blocks always specify a language fence (` ```python ` not ` ``` `).
- Link to source files using relative paths so they resolve on GitHub and in IDEs.

---

## Commit messages

We don't enforce a strict convention, but follow this rough format and reviewers will love you:

```
{area}: {short imperative summary}

{Optional longer body explaining the why, not the what.
The diff already shows the what.}

Fixes #123
```

`{area}` is one of: `api`, `orchestrator`, `providers`, `sdk-js`, `sdk-py`, `web`, `tui`, `docs`, `scripts`, `ci`.

Examples:
```
api: add POST /sandboxes/{id}/snapshot endpoint
providers/firecracker: handle missing kernel gracefully
sdk-py: fix AsyncSandbox.exec_stream cancellation
docs: clarify pool mode user-id header is mandatory
```

Squash commits before merge if your branch has noise (`fixup: typo`, `wip`).

---

## Pull requests

Before you open one:

- [ ] Branch is rebased on latest `main`
- [ ] `make lint` and `make test` pass
- [ ] SDK tests pass for any SDK you touched
- [ ] Docs (`README.md`, `docs/api.md`, SDK READMEs) reflect the change
- [ ] OpenAPI spec (`docs/swagger.yaml`) updated for API changes
- [ ] New behaviour is covered by at least one test
- [ ] PR description explains the **why** and links any related issue

PR template — copy this into the description:

```markdown
## What
One-line summary of the change.

## Why
The motivation. Bug? Feature? Performance? Link issues if any.

## How
Brief notes on the approach, especially anything non-obvious or
controversial. Reviewers shouldn't have to reverse-engineer your design.

## Testing
What you ran. Any manual verification (commands, screenshots, recordings).

## Breaking changes
None / List of breaking changes with migration notes.
```

We aim to give first-pass review within **3 business days**. Ping a maintainer in the PR if it stalls.

---

## Adding a new provider

Providers are the most common contribution. Here's the recipe:

1. **Implement the interface.** Create `internal/providers/{name}/provider.go` and implement every method on the `orchestrator.Provider` interface (Spawn, Exec, ExecStream, file ops, Status, Destroy, Healthy, etc.). Mock provider (`internal/providers/mock/`) is the smallest reference implementation; Docker is the most complete.
2. **Add config schema.** Extend `internal/config/config.go` with a `{Name}Config` struct, default values, and YAML tags.
3. **Register the provider.** Wire it into the provider registry in `internal/orchestrator/registry.go`.
4. **Tests.** At minimum: spawn → exec → destroy. Mark Docker/KVM-dependent tests with `testing.Skip` if the dependency is missing.
5. **Docs.** Update the providers table in the main README, add a config example, and mention any host requirements.
6. **(Bonus)** Live preview support — if your provider exposes ports, document how it integrates with Traefik (see `docs/live-preview-architecture.md`).

Open an issue before starting if your provider needs new orchestrator-level features (e.g. GPU passthrough). It's faster to align on the API than to debate it after the code is written.

---

## Adding a new SDK language

We have Python and TypeScript. Other contenders that come up: Go (client), Rust, Ruby, Java/Kotlin.

Process:
1. Open a discussion describing your target audience and ergonomic goals.
2. Mirror the existing SDKs' surface area — every method documented in `docs/api.md`, named idiomatically for the language.
3. Tests against the real server using a docker-compose fixture.
4. Publishing pipeline (PyPI / npm / crates.io / etc.) and version-bump checklist.
5. SDK README following the same shape as `sdk/python/README.md` and `sdk/js/README.md`.

Don't auto-generate from OpenAPI without a manual ergonomics pass — generated SDKs feel generic and we'd rather have fewer good ones than many mediocre ones.

---

## Updating the API surface

A few things must move together:

| When you change | Update |
|---|---|
| Add an HTTP endpoint | `internal/api/`, `docs/api.md`, `docs/swagger.yaml`, both SDKs, both SDK READMEs |
| Change a request/response field | `docs/api.md`, `docs/swagger.yaml`, both SDK type definitions, both SDK READMEs |
| Add an error code | `docs/api.md` error table, error classes in both SDKs |
| Add a sandbox-level method | `sdk/python/stacyvm/sandbox.py` AND `async_sandbox.py`, `sdk/js/src/sandbox.ts`, both READMEs |

Reviewers will ask for these. Saving them up for "a follow-up PR" almost always means they don't happen.

Breaking changes need a migration note in the PR description and a callout in the next release notes.

---

## Documentation contributions

Docs PRs are welcome and usually merge fast. A few specifics:

- **README** is the front door. Aim for clarity over completeness.
- **`docs/api.md`** is the source of truth for the REST API. Every endpoint, every field.
- **SDK READMEs** mirror each other in structure — change one, consider whether the other needs the same change.
- **Architecture docs** in `docs/` are one-topic-per-file (`live-preview-architecture.md`, `snapshot-restore.md`, etc.). Add new ones as needed; link them from the README.
- **Examples** in `examples/{js,python}/` should be runnable. If they require setup (`docker compose up`), say so in a top comment.

When you fix a doc inaccuracy, link to the commit or PR that introduced the drift in your PR body — helps us spot patterns.

---

## Reporting bugs

Open an issue with:

1. **What you did** — exact commands, request bodies, config snippets.
2. **What you expected.**
3. **What actually happened** — full error message, relevant logs, version (`stacyvm version`).
4. **Environment** — OS, kernel, Docker version, KVM availability if relevant.

Reproductions in `docker compose` or a tiny script are gold.

For feature requests, describe the use case before the proposed API. The "why" usually leads to a better "what" than the one in the original ticket.

---

## Reporting security issues

**Do not open a public issue.** Follow [SECURITY.md](SECURITY.md) — it routes to a private GitHub security advisory. We acknowledge within 48 hours and aim for a fix within 7 days for critical issues.

---

## Licensing

By submitting a contribution, you agree it's licensed under [MIT](LICENSE), the same as the rest of the project. No CLA, no paperwork — the act of opening a PR is the agreement.

If you're contributing on behalf of an employer, make sure they're okay with that. Check your employment agreement before contributing significant code.

---

## Maintainers

The current maintainers are the [@StacyOs](https://github.com/StacyOs) team. Reach us via:

- **GitHub Issues** — bugs, features, questions
- **GitHub Discussions** — design proposals, broader topics
- **Security advisories** — vulnerability reports (see SECURITY.md)

Thanks for making StacyVM better.
