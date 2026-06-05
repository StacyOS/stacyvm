package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestInstallShellCompletionAddsPathBeforeZshCompletion(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(configPath, []byte("export EDITOR=vim\n"), 0644); err != nil {
		t.Fatal(err)
	}

	pathSnippet := shellPathExport("/Users/example/.local/bin")
	completionBlock := "\n# StacyVM Autocomplete\n" + pathSnippet + "if command -v stacyvm >/dev/null 2>&1; then\n  source <(stacyvm completion zsh)\nfi\n"
	installShellCompletion(configPath, "stacyvm completion zsh", pathSnippet, completionBlock)

	b, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	if !strings.Contains(content, `export PATH="/Users/example/.local/bin":$PATH`) {
		t.Fatalf("PATH export missing from zshrc:\n%s", content)
	}
	if !strings.Contains(content, "if command -v stacyvm >/dev/null 2>&1; then\n  source <(stacyvm completion zsh)\nfi") {
		t.Fatalf("guarded zsh completion missing from zshrc:\n%s", content)
	}
	if strings.Index(content, ".local/bin") > strings.Index(content, "stacyvm completion zsh") {
		t.Fatalf("PATH export must appear before completion:\n%s", content)
	}
}

func TestInstallShellCompletionRepairsExistingZshCompletion(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".zshrc")
	original := "# StacyVM Autocomplete\nsource <(stacyvm completion zsh)\n"
	if err := os.WriteFile(configPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	pathSnippet := shellPathExport("/Users/example/.local/bin")
	installShellCompletion(configPath, "stacyvm completion zsh", pathSnippet, "")

	b, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	if !strings.Contains(content, pathSnippet) {
		t.Fatalf("PATH export missing from repaired zshrc:\n%s", content)
	}
	if strings.Count(content, "stacyvm completion zsh") != 1 {
		t.Fatalf("completion should not be duplicated:\n%s", content)
	}
	if strings.Index(content, ".local/bin") > strings.Index(content, "stacyvm completion zsh") {
		t.Fatalf("PATH export must be inserted before existing completion:\n%s", content)
	}
}

func TestInstallShellCompletionSkipsWhenConfigMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".zshrc")

	installShellCompletion(configPath, "stacyvm completion zsh", shellPathExport("/Users/example/.local/bin"), "completion")

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("missing shell config should not be created, err=%v", err)
	}
}

func TestApplySSHConfigEnabledWritesBlock(t *testing.T) {
	v := viper.New()
	dir := filepath.Join(t.TempDir(), ".stacyvm")

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
	// The whole body is behind the !enabled guard; check the canonical key
	// plus the rest so a future refactor that moves a Set outside the guard
	// is caught.
	if v.IsSet("ssh.enabled") ||
		v.IsSet("ssh.listen_addr") ||
		v.IsSet("ssh.host_key_path") ||
		v.IsSet("ssh.user_ca_path") {
		t.Fatal("applySSHConfig(enabled=false) must not set any ssh keys")
	}
}
