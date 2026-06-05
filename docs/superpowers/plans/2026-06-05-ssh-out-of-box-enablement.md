# SSH Out-of-the-Box Enablement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an end user enable SSH-into-sandboxes through `stacyvm setup` with no file editing, and guarantee that `ssh.enabled: true` makes `stacyvm serve` *run* (not crash) for a normal non-root user.

**Architecture:** Move the SSH key defaults from the root-only `/var/lib/stacyvm/` to the per-user `~/.stacyvm/` config dir (fixes the startup crash at its source), add an opt-in confirm step to the Go setup wizard that writes a complete `ssh:` block, and improve the serve-time error message for any remaining unwritable-path case. The npx script is unchanged — it already delegates to `stacyvm setup`.

**Tech Stack:** Go, `spf13/viper` (config), `charmbracelet/huh` (wizard forms), `golang.org/x/crypto/ssh` (gateway). Spec: `docs/superpowers/specs/2026-06-05-ssh-out-of-box-enablement-design.md`.

---

## File Structure

- `internal/config/config.go` — add `defaultStateDir()` helper; point the three `ssh.*` path defaults at it. (Owns config defaults.)
- `internal/config/config_test.go` — add tests for the home-based defaults, env override precedence, and the no-home fallback.
- `cmd/stacyvm/cmd_setup.go` — add `applySSHConfig()` helper, an opt-in SSH confirm step, and a post-setup connection hint. (Owns the interactive wizard.)
- `cmd/stacyvm/cmd_setup_test.go` — **new** — unit tests for `applySSHConfig()`.
- `cmd/stacyvm/cmd_serve.go` — clearer error wrapping when SSH key creation fails. (Owns serve wiring.)
- `scripts/smoke-ssh-enabled.sh` — **new** — smoke test proving `serve` runs with `ssh.enabled: true` as a non-root user.

---

## Task 1: Writable default SSH key paths

Fixes the crash at its root: the built-in defaults become a directory the running user can write to. A hand-edited `ssh.enabled: true` then works without the wizard.

**Files:**
- Modify: `internal/config/config.go` (add helper near line 222; edit `ssh.*` path defaults at lines 245-248)
- Test: `internal/config/config_test.go` (add three test funcs after `TestSSHDefaults`, line 313)

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go` (imports already include `os`, `path/filepath`, `testing`):

```go
func TestSSHKeyPathsDefaultToUserConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir()) // empty dir → no ./stacyvm.yaml, pure defaults

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if want := filepath.Join(home, ".stacyvm", "ssh_host_ed25519_key"); cfg.SSH.HostKeyPath != want {
		t.Fatalf("host_key_path = %q, want %q", cfg.SSH.HostKeyPath, want)
	}
	if want := filepath.Join(home, ".stacyvm", "ssh_user_ca_key"); cfg.SSH.UserCAPath != want {
		t.Fatalf("user_ca_path = %q, want %q", cfg.SSH.UserCAPath, want)
	}
	if want := filepath.Join(home, ".stacyvm", "ssh-recordings"); cfg.SSH.RecordingDir != want {
		t.Fatalf("recording_dir = %q, want %q", cfg.SSH.RecordingDir, want)
	}
}

func TestSSHKeyPathExplicitOverrideWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("STACYVM_SSH_HOST_KEY_PATH", "/custom/host_key")
	t.Chdir(t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SSH.HostKeyPath != "/custom/host_key" {
		t.Fatalf("host_key_path = %q, want /custom/host_key (env override must win)", cfg.SSH.HostKeyPath)
	}
}

func TestSSHKeyPathsFallBackWithoutHome(t *testing.T) {
	t.Setenv("HOME", "") // os.UserHomeDir() errors → system fallback
	t.Chdir(t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SSH.HostKeyPath != "/var/lib/stacyvm/ssh_host_ed25519_key" {
		t.Fatalf("host_key_path = %q, want /var/lib/stacyvm fallback", cfg.SSH.HostKeyPath)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestSSHKeyPaths|TestSSHKeyPathExplicit' -v`
Expected: FAIL — `host_key_path` still equals `/var/lib/stacyvm/ssh_host_ed25519_key` (the old default), so the first and second tests fail. (`TestSSHKeyPathsFallBackWithoutHome` will already pass since the old default is the fallback value — that's fine.)

- [ ] **Step 3: Add the `defaultStateDir` helper**

In `internal/config/config.go`, add immediately before `func setDefaults(v *viper.Viper) {` (line 222):

```go
// defaultStateDir is where StacyVM keeps per-user state such as SSH keys. It
// mirrors the config-file location (~/.stacyvm) so a normal user can write to
// it; when the home dir cannot be resolved it falls back to the system path
// used by packaged/root deployments.
func defaultStateDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".stacyvm")
	}
	return "/var/lib/stacyvm"
}
```

- [ ] **Step 4: Point the SSH path defaults at it**

In `internal/config/config.go`, replace the SSH defaults block (lines 243-249):

```go
	v.SetDefault("ssh.enabled", false)
	v.SetDefault("ssh.listen_addr", ":2222")
	v.SetDefault("ssh.host_key_path", "/var/lib/stacyvm/ssh_host_ed25519_key")
	v.SetDefault("ssh.user_ca_path", "/var/lib/stacyvm/ssh_user_ca_key")
	v.SetDefault("ssh.session_recording", false)
	v.SetDefault("ssh.recording_dir", "/var/lib/stacyvm/ssh-recordings")
	v.SetDefault("ssh.allow_port_forward", true)
```

with:

```go
	sshStateDir := defaultStateDir()
	v.SetDefault("ssh.enabled", false)
	v.SetDefault("ssh.listen_addr", ":2222")
	v.SetDefault("ssh.host_key_path", filepath.Join(sshStateDir, "ssh_host_ed25519_key"))
	v.SetDefault("ssh.user_ca_path", filepath.Join(sshStateDir, "ssh_user_ca_key"))
	v.SetDefault("ssh.session_recording", false)
	v.SetDefault("ssh.recording_dir", filepath.Join(sshStateDir, "ssh-recordings"))
	v.SetDefault("ssh.allow_port_forward", true)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/config/ -run 'TestSSH' -v`
Expected: PASS for `TestSSHDefaults`, `TestSSHKeyPathsDefaultToUserConfigDir`, `TestSSHKeyPathExplicitOverrideWins`, `TestSSHKeyPathsFallBackWithoutHome`.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "fix(ssh): default SSH key paths to ~/.stacyvm so enabling SSH doesn't need root"
```

---

## Task 2: Opt-in SSH step in the setup wizard

Adds the prompt and the config-writing helper. The helper is pure and unit-tested; the interactive form is exercised by the Task 4 smoke test.

**Files:**
- Create: `cmd/stacyvm/cmd_setup_test.go`
- Modify: `cmd/stacyvm/cmd_setup.go` (add helper; add confirm form after the domain form ~line 130; call helper before `WriteConfigAs` ~line 144; add hint after the success message ~line 163)

- [ ] **Step 1: Write the failing test**

Create `cmd/stacyvm/cmd_setup_test.go`:

```go
package main

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestApplySSHConfigEnabledWritesBlock(t *testing.T) {
	v := viper.New()
	dir := filepath.Join("home", "user", ".stacyvm")

	applySSHConfig(v, dir, true)

	if !v.GetBool("ssh.enabled") {
		t.Fatal("ssh.enabled should be true when enabled")
	}
	if got, want := v.GetString("ssh.listen_addr"), ":2222"; got != want {
		t.Fatalf("listen_addr = %q, want %q", got, want)
	}
	if got, want := v.GetString("ssh.host_key_path"), filepath.Join(dir, "ssh_host_ed25519_key"); got != want {
		t.Fatalf("host_key_path = %q, want %q", got, want)
	}
	if got, want := v.GetString("ssh.user_ca_path"), filepath.Join(dir, "ssh_user_ca_key"); got != want {
		t.Fatalf("user_ca_path = %q, want %q", got, want)
	}
}

func TestApplySSHConfigDisabledWritesNothing(t *testing.T) {
	v := viper.New()
	applySSHConfig(v, "/anything", false)
	if v.IsSet("ssh.enabled") {
		t.Fatal("applySSHConfig(enabled=false) must not set any ssh keys")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/stacyvm/ -run TestApplySSHConfig -v`
Expected: FAIL to compile — `undefined: applySSHConfig`.

- [ ] **Step 3: Add the `applySSHConfig` helper**

In `cmd/stacyvm/cmd_setup.go`, add after the `runSetup` function (after its closing `}` at line 177). `viper` and `path/filepath` are already imported.

```go
// applySSHConfig writes the ssh.* keys for an enabled gateway into v, using
// stateDir for the host-key and user-CA file paths. It is a no-op when enabled
// is false, so the wizard never persists an ssh block the user declined.
func applySSHConfig(v *viper.Viper, stateDir string, enabled bool) {
	if !enabled {
		return
	}
	v.Set("ssh.enabled", true)
	v.Set("ssh.listen_addr", ":2222")
	v.Set("ssh.host_key_path", filepath.Join(stateDir, "ssh_host_ed25519_key"))
	v.Set("ssh.user_ca_path", filepath.Join(stateDir, "ssh_user_ca_key"))
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/stacyvm/ -run TestApplySSHConfig -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Commit the tested helper**

```bash
git add cmd/stacyvm/cmd_setup_test.go cmd/stacyvm/cmd_setup.go
git commit -m "feat(setup): add applySSHConfig helper to write the ssh config block"
```

- [ ] **Step 6: Add the opt-in confirm form to the wizard**

In `cmd/stacyvm/cmd_setup.go`, insert after the domain form's error check (after line 130, `}` closing `if err = domainForm.Run(); ...`) and before the `// Save Config` comment (line 132):

```go
	// SSH access (opt-in). Enabling it writes a complete ssh block so the
	// gateway starts on the next `serve`, with keys in the user-writable
	// config dir. Defaults to Yes: it is the headline capability, and this is
	// still an explicit prompt.
	enableSSH := true
	sshForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable SSH access to your sandboxes?").
				Description("Lets you `ssh` and VS Code Remote into running sandboxes.").
				Value(&enableSSH),
		),
	).WithTheme(t)
	if err = sshForm.Run(); err != nil {
		return err
	}
```

- [ ] **Step 7: Persist the SSH block before writing the file**

In `cmd/stacyvm/cmd_setup.go`, the existing provider block ends at line 142 with `}` and line 144 is `viper.WriteConfigAs(...)`. Insert between them (after line 142, before line 144):

```go
	applySSHConfig(viper.GetViper(), configDir, enableSSH)
```

(`configDir` is defined at line 134 as `filepath.Join(home, ".stacyvm")`, so the keys land next to `config.yaml`.)

- [ ] **Step 8: Add the post-setup connection hint**

In `cmd/stacyvm/cmd_setup.go`, after the existing `fmt.Println("Config saved to ~/.stacyvm/config.yaml")` (line 163), insert:

```go
	if enableSSH {
		fmt.Println("\n" + successStyle.Render("SSH enabled.") + " Once the server is running, connect with:")
		fmt.Println("  stacyvm ssh <sandbox-id>")
		fmt.Println("  (Direct ssh / VS Code Remote: port 2222.)")
	}
```

- [ ] **Step 9: Verify it builds and existing tests pass**

Run: `go build ./cmd/stacyvm/ && go test ./cmd/stacyvm/ -run TestApplySSHConfig -v`
Expected: build succeeds; tests PASS.

- [ ] **Step 10: Commit**

```bash
git add cmd/stacyvm/cmd_setup.go
git commit -m "feat(setup): opt-in SSH prompt that enables the gateway and prints connect hint"
```

---

## Task 3: Clearer serve-time error for unwritable key paths

Defense-in-depth: if someone still points a key path somewhere unwritable, serve names the path and the fix instead of emitting a raw `mkdir ... permission denied`.

**Files:**
- Modify: `cmd/stacyvm/cmd_serve.go` (lines 266-273)

- [ ] **Step 1: Replace the two error wrappers**

In `cmd/stacyvm/cmd_serve.go`, replace:

```go
		sshUserCA, err = stacyssh.LoadOrCreateHostKey(cfg.SSH.UserCAPath)
		if err != nil {
			return fmt.Errorf("ssh user CA: %w", err)
		}
		hostKey, herr := stacyssh.LoadOrCreateHostKey(cfg.SSH.HostKeyPath)
		if herr != nil {
			return fmt.Errorf("ssh gateway host key: %w", herr)
		}
```

with:

```go
		sshUserCA, err = stacyssh.LoadOrCreateHostKey(cfg.SSH.UserCAPath)
		if err != nil {
			return fmt.Errorf("ssh user CA: could not create key at %s: %w (set ssh.user_ca_path to a writable location)", cfg.SSH.UserCAPath, err)
		}
		hostKey, herr := stacyssh.LoadOrCreateHostKey(cfg.SSH.HostKeyPath)
		if herr != nil {
			return fmt.Errorf("ssh gateway host key: could not create key at %s: %w (set ssh.host_key_path to a writable location)", cfg.SSH.HostKeyPath, herr)
		}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./cmd/stacyvm/`
Expected: success. (This path is validated end-to-end by the Task 4 smoke test; `runServe` is not independently unit-tested.)

- [ ] **Step 3: Commit**

```bash
git add cmd/stacyvm/cmd_serve.go
git commit -m "fix(ssh): clearer serve error when SSH key path is not writable"
```

---

## Task 4: Smoke test — serve runs with `ssh.enabled: true`

Directly proves acceptance criteria 2 & 3 from the spec: as a non-root user, with SSH enabled and no `/var/lib/stacyvm`, `serve` stays up and the gateway listens.

**Files:**
- Create: `scripts/smoke-ssh-enabled.sh`

- [ ] **Step 1: Write the smoke script**

Create `scripts/smoke-ssh-enabled.sh`:

```bash
#!/usr/bin/env bash
# Proves `stacyvm serve` RUNS (does not crash) with ssh.enabled=true as a
# non-root user with no /var/lib/stacyvm. Uses the mock provider and isolated
# temp HOME/db/ports so it never touches a real install.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$(mktemp -d)/stacyvm"
go build -o "$BIN" "$ROOT/cmd/stacyvm"

TMP="$(mktemp -d)"
SSH_PORT=22422
LOG="$TMP/serve.log"

HOME="$TMP" \
STACYVM_SSH_ENABLED=true \
STACYVM_SSH_LISTEN_ADDR=":$SSH_PORT" \
STACYVM_PROVIDERS_DEFAULT=mock \
STACYVM_PROVIDERS_MOCK_ENABLED=true \
STACYVM_DATABASE_PATH="$TMP/test.db" \
STACYVM_SERVER_PORT=27423 \
  "$BIN" serve >"$LOG" 2>&1 &
PID=$!

cleanup() { kill "$PID" 2>/dev/null || true; }
trap cleanup EXIT

# Give it a moment to boot and create keys.
for _ in $(seq 1 20); do
  kill -0 "$PID" 2>/dev/null || { echo "FAIL: serve exited at startup"; cat "$LOG"; exit 1; }
  if (exec 3<>/dev/tcp/127.0.0.1/"$SSH_PORT") 2>/dev/null; then break; fi
  sleep 0.5
done

if ! kill -0 "$PID" 2>/dev/null; then
  echo "FAIL: serve is not running"; cat "$LOG"; exit 1
fi
if ! (exec 3<>/dev/tcp/127.0.0.1/"$SSH_PORT") 2>/dev/null; then
  echo "FAIL: SSH gateway not listening on :$SSH_PORT"; cat "$LOG"; exit 1
fi
test -f "$TMP/.stacyvm/ssh_host_ed25519_key" || { echo "FAIL: host key not created under \$HOME/.stacyvm"; exit 1; }
test -f "$TMP/.stacyvm/ssh_user_ca_key" || { echo "FAIL: user CA not created under \$HOME/.stacyvm"; exit 1; }

echo "PASS: serve runs with ssh.enabled=true; gateway listening on :$SSH_PORT; keys created under \$HOME/.stacyvm"
```

- [ ] **Step 2: Make it executable and run it**

Run: `chmod +x scripts/smoke-ssh-enabled.sh && ./scripts/smoke-ssh-enabled.sh`
Expected: final line `PASS: serve runs with ssh.enabled=true; ...`. If the mock provider's `Healthy()` check or another startup step fails, the script prints the captured `serve.log` — read it and fix before proceeding.

- [ ] **Step 3: Commit**

```bash
git add scripts/smoke-ssh-enabled.sh
git commit -m "test(ssh): smoke test proving serve runs with ssh.enabled=true as non-root"
```

---

## Task 5: Full verification

Matches the project's standing verification bar (race tests, vet, Windows cross-compile).

- [ ] **Step 1: Run the focused and full test suites**

Run: `go test ./internal/config/ ./cmd/stacyvm/ && go test -race ./internal/...`
Expected: all PASS.

- [ ] **Step 2: Vet and Windows cross-compile**

Run: `go vet ./... && GOOS=windows go build ./...`
Expected: no output / exit 0 for both.

- [ ] **Step 3: Final commit if anything changed**

```bash
git add -A
git commit -m "chore(ssh): verification pass for out-of-the-box SSH enablement" || echo "nothing to commit"
```

---

## Self-Review Notes

- **Spec coverage:** Piece 1 → Task 1; Piece 2 → Task 2; Piece 3 → Task 3; Piece 4 → Task 2 Step 8; Acceptance criteria 2/3 → Task 4; criterion 5 (root/`/var/lib` override) → `TestSSHKeyPathExplicitOverrideWins` (Task 1) + the override note. Criterion 1 (wizard writes valid block) → `TestApplySSHConfigEnabledWritesBlock`. Criterion 4 (`stacyvm ssh` opens a shell) is covered by the pre-existing SSH gateway e2e referenced in the spec, not re-implemented here.
- **Type/name consistency:** `applySSHConfig(v *viper.Viper, stateDir string, enabled bool)` and `defaultStateDir() string` are used identically wherever referenced. Config keys (`ssh.host_key_path`, `ssh.user_ca_path`, `ssh.recording_dir`, `ssh.listen_addr`, `ssh.enabled`) match the `SSHConfig` mapstructure tags in `internal/config/config.go:29-37`.
- **No placeholders:** every code and command step is concrete.
