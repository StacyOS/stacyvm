package main

import (
	"strings"
	"testing"
)

func TestRemoveAutocompleteBlockBash(t *testing.T) {
	content := "export EDITOR=vim\n\n" +
		"# StacyVM Autocomplete\n" +
		"export PATH=\"/home/u/.local/bin\":$PATH\n" +
		"if command -v stacyvm >/dev/null 2>&1; then\n" +
		"  source <(stacyvm completion bash)\n" +
		"fi\n"

	got, changed := removeAutocompleteBlock(content, "fi")
	if !changed {
		t.Fatal("expected block to be removed")
	}
	if strings.Contains(got, "stacyvm") || strings.Contains(got, autocompleteMarker) {
		t.Fatalf("stacyvm block not fully removed:\n%q", got)
	}
	if got != "export EDITOR=vim\n" {
		t.Fatalf("unexpected leftover content:\n%q", got)
	}
}

func TestRemoveAutocompleteBlockFish(t *testing.T) {
	content := "set -x FOO bar\n\n" +
		"# StacyVM Autocomplete\n" +
		"fish_add_path \"/home/u/.local/bin\"\n" +
		"if type -q stacyvm\n" +
		"  stacyvm completion fish | source\n" +
		"end\n"

	got, changed := removeAutocompleteBlock(content, "end")
	if !changed {
		t.Fatal("expected block to be removed")
	}
	if got != "set -x FOO bar\n" {
		t.Fatalf("unexpected leftover content:\n%q", got)
	}
}

func TestRemoveAutocompleteBlockPowerShell(t *testing.T) {
	content := "Set-Alias ll Get-ChildItem\n\n" +
		"# StacyVM Autocomplete\n" +
		"if (Get-Command stacyvm -ErrorAction SilentlyContinue) {\n" +
		"    stacyvm completion powershell | Out-String | Invoke-Expression\n" +
		"}\n"

	got, changed := removeAutocompleteBlock(content, "}")
	if !changed {
		t.Fatal("expected block to be removed")
	}
	if got != "Set-Alias ll Get-ChildItem\n" {
		t.Fatalf("unexpected leftover content:\n%q", got)
	}
}

// Legacy installs wrote the block without a PATH export or guard/terminator.
func TestRemoveAutocompleteBlockLegacyNoTerminator(t *testing.T) {
	content := "alias g=git\n" +
		"# StacyVM Autocomplete\n" +
		"source <(stacyvm completion zsh)\n" +
		"alias k=kubectl\n"

	got, changed := removeAutocompleteBlock(content, "fi")
	if !changed {
		t.Fatal("expected block to be removed")
	}
	if strings.Contains(got, "stacyvm") {
		t.Fatalf("legacy block not removed:\n%q", got)
	}
	if !strings.Contains(got, "alias g=git") || !strings.Contains(got, "alias k=kubectl") {
		t.Fatalf("user config was clobbered:\n%q", got)
	}
}

func TestRemoveAutocompleteBlockAbsent(t *testing.T) {
	content := "export EDITOR=vim\nalias g=git\n"
	got, changed := removeAutocompleteBlock(content, "fi")
	if changed {
		t.Fatal("did not expect any change")
	}
	if got != content {
		t.Fatalf("content should be unchanged:\n%q", got)
	}
}
