package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
