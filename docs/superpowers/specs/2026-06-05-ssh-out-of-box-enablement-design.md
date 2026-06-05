# SSH Out-of-the-Box Enablement — Design

**Date:** 2026-06-05
**Branch:** feat/ssh-pty-access
**Status:** Approved (pending spec review)

## Problem

The SSH/PTY gateway works (Phase-1 tests green), but an end user installing via
`npx stacyvm-setup` cannot actually use it:

1. **It is off by default.** `ssh.enabled` defaults to `false`
   (`internal/config/config.go:243`), and nothing in the install flow turns it
   on. The npx script (`scripts/npm-setup.mjs`) writes no config; it delegates to
   `stacyvm setup`, and that wizard (`cmd/stacyvm/cmd_setup.go`) only writes
   `server.preview_domain`, `providers.default`, and `providers.docker.runtime` —
   never an `ssh:` block.
2. **Turning it on crashes serve for non-root users.** When `ssh.enabled` is
   true, `runServe` calls `LoadOrCreateHostKey(cfg.SSH.UserCAPath)`
   (`cmd/stacyvm/cmd_serve.go:266`), which `os.MkdirAll`s the key's parent dir
   (`internal/ssh/hostkey.go:36`). The default paths live under
   `/var/lib/stacyvm/`, which is `root:root` and not writable by a normal user,
   so serve aborts at startup with `ssh user CA: ... permission denied`.

End users have no access to the codebase and will not hand-edit YAML, so the fix
must make SSH enablement a first-class, prompt-driven part of `stacyvm setup`,
and must make enablement safe (no crash) regardless of how it is turned on.

## Decisions

- **Posture:** Opt-in. SSH stays off in pure defaults; `stacyvm setup` asks the
  user whether to enable it and writes the config when they accept.
- **Key location:** Default the SSH key files to the per-user config dir
  (`~/.stacyvm/`, where `config.yaml` already lives) instead of
  `/var/lib/stacyvm/`. This fixes the crash for *both* the wizard path and a
  manual `ssh.enabled: true`, since the default itself becomes writable.
- **Direct port:** When enabled, keep the native `:2222` listener on all
  interfaces (current behavior), so `ssh`/scp/VS Code Remote from other machines
  work. No decoupling of the WS tunnel from the native listener.
- **Wizard prompt default (open for review):** The confirm step defaults to
  **Yes**. It is still a visible prompt (consent), but pressing Enter enables the
  product's headline feature rather than silently skipping it. Trivial to flip to
  default-No if preferred.
- **npx script:** Untouched. It already correctly delegates to `stacyvm setup`;
  all new logic lives in the Go wizard and config defaults.

## Implementation

### Piece 1 — Writable default key paths (`internal/config/config.go`)

In `setDefaults`, compute a per-user state dir and use it for the SSH file paths:

- Add a helper that returns the default SSH state dir:
  - `home, err := os.UserHomeDir()`; on success return `filepath.Join(home, ".stacyvm")`.
  - On error (home unresolvable), fall back to `/var/lib/stacyvm` so nothing
    regresses on headless/system installs.
- Change the three defaults to sit under that dir:
  - `ssh.host_key_path` → `<stateDir>/ssh_host_ed25519_key`
  - `ssh.user_ca_path` → `<stateDir>/ssh_user_ca_key`
  - `ssh.recording_dir` → `<stateDir>/ssh-recordings`

Because `SetDefault` is the lowest-priority source in viper, an explicit value in
`config.yaml` or a `STACYVM_SSH_*` env var still wins — so packaged/systemd/root
deployments that want `/var/lib/stacyvm` simply set the path explicitly.

### Piece 2 — Wizard SSH prompt (`cmd/stacyvm/cmd_setup.go`)

- Add a `huh.NewConfirm()` step after the existing provider/runtime/domain
  questions: *"Enable SSH access to your sandboxes? (lets you ssh / VS Code into
  them)"*, default value `true`.
- Extract the SSH config-writing logic into a small, pure, testable helper rather
  than inlining viper calls in the wizard:

  ```go
  // applySSHConfig records the ssh.* keys for an enabled gateway into v,
  // using stateDir for the host key / user CA paths. It is a no-op when
  // enabled is false.
  func applySSHConfig(v *viper.Viper, stateDir string, enabled bool)
  ```

  When `enabled` is true it sets:
  - `ssh.enabled = true`
  - `ssh.listen_addr = ":2222"`
  - `ssh.host_key_path = filepath.Join(stateDir, "ssh_host_ed25519_key")`
  - `ssh.user_ca_path  = filepath.Join(stateDir, "ssh_user_ca_key")`

- The wizard calls `applySSHConfig(viper.GetViper(), configDir, sshEnabled)`
  before `viper.WriteConfigAs(...)`, where `configDir` is the existing
  `~/.stacyvm` value. `WriteConfigAs` only persists explicitly-set keys, so the
  written file gains a clean `ssh:` block and nothing else.

### Piece 3 — Clearer startup error (`cmd/stacyvm/cmd_serve.go`)

Wrap the two `LoadOrCreateHostKey` failures (host key and user CA, lines ~266 and
~270) so the message names the path and the remedy, e.g.:

> `ssh gateway host key: could not create key at <path>: <err>. Set ssh.host_key_path to a writable location.`

This converts the cryptic `mkdir ... permission denied` abort into an actionable
message for anyone who points the paths somewhere unwritable.

### Piece 4 — Post-setup connection hint (`cmd/stacyvm/cmd_setup.go`)

When SSH was enabled, after writing the config print guidance:

> `SSH enabled. Once the server is running, connect with:  stacyvm ssh <sandbox-id>`
> `(Direct ssh / VS Code: port 2222.)`

## Components / boundaries

- `defaultSSHStateDir()` (config) — pure, returns a dir string; depends only on
  `os.UserHomeDir`. Testable by setting `HOME`.
- `applySSHConfig(v, stateDir, enabled)` (setup) — pure mutation of a viper
  instance; no I/O. Directly unit-testable.
- The wizard's interactive `huh` forms remain untested (UI), but all the logic
  that matters (which keys get written, to which paths) moves into the two pure
  helpers above.

## Acceptance criteria

The headline criterion: **with `ssh.enabled: true`, `stacyvm serve` runs.**
Concretely, all of the following must hold for a normal (non-root) user with no
pre-existing `/var/lib/stacyvm`:

1. Running `stacyvm setup` and answering "yes" to the SSH prompt writes a valid
   `ssh:` block (enabled + key paths + listen_addr) into `~/.stacyvm/config.yaml`.
2. `stacyvm serve` then starts successfully — it does **not** abort with a
   permission error — generates the host key and user CA under `~/.stacyvm/`, and
   binds the gateway on `:2222`.
3. A hand-edited `ssh.enabled: true` (no wizard, defaults for everything else)
   produces the same result: serve starts and listens, no crash.
4. `stacyvm ssh <sandbox-id>` against that server opens an interactive shell.
5. A deployment that explicitly sets `ssh.host_key_path` to `/var/lib/stacyvm/...`
   while running as root still works (no regression for the system-service case).

### Verification

- Automated: the config + helper unit tests below, plus the existing SSH gateway
  e2e (temp writable key path → gateway builds, serves, real client gets output).
- Smoke (manual/scripted), proving criterion 2/3 directly: as a non-root user,
  `ssh.enabled: true` with key paths under a temp `HOME`, start `stacyvm serve`,
  and assert the process stays up and `:2222` is listening (e.g. the gateway
  listener accepts a TCP connection) rather than exiting with an error.

## Testing (TDD)

- `config_test.go`: with `HOME` set to a temp dir and no config file, assert
  `cfg.SSH.HostKeyPath`, `cfg.SSH.UserCAPath`, and `cfg.SSH.RecordingDir` resolve
  under `<tmp>/.stacyvm/`. Assert that an explicit env/config override still wins.
- `config_test.go`: with `HOME` unset/unresolvable, assert the `/var/lib/stacyvm`
  fallback.
- New `cmd_setup` test: `applySSHConfig` on a fresh `viper.New()` with
  `enabled=true` sets the four expected keys to the expected values; with
  `enabled=false` sets none of them.
- Existing SSH gateway e2e (per project notes) still passes: with a temp writable
  key path and `ssh.enabled`, the gateway builds and serves.

## Out of scope (YAGNI)

- Auto-registering the user's `~/.ssh/*.pub` so plain `ssh` works without the
  cert flow. (`stacyvm ssh <sb>` already works via the cert + WS flow.)
- Splitting `ssh.enabled` into separate "gateway on" vs "native :2222 on" flags.
- Any change to `scripts/npm-setup.mjs`.
- Changing the `:2222` bind to localhost-only.

## Deployment note

Default key paths now follow the invoking user's home dir. Packaged/systemd
deployments that intentionally store state in `/var/lib/stacyvm` must set
`ssh.host_key_path`, `ssh.user_ca_path`, and (if used) `ssh.recording_dir`
explicitly in their config — the same way they already pin other paths.
