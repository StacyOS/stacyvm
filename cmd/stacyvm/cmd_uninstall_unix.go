//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// cleanupShellIntegration removes the autocomplete block that `stacyvm setup`
// appended to the user's shell rc files. The PATH export inside the block is
// removed along with it; any standalone PATH line the user added themselves is
// left untouched.
func cleanupShellIntegration() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	// bash/zsh blocks close with "fi"; fish blocks close with "end".
	removeAutocompleteFromFile(filepath.Join(home, ".bashrc"), "fi")
	removeAutocompleteFromFile(filepath.Join(home, ".zshrc"), "fi")
	removeAutocompleteFromFile(filepath.Join(home, ".config", "fish", "config.fish"), "end")
}

// removeRunningBinary deletes the currently running executable. On Unix-like
// systems a running binary is just an open inode, so it can be unlinked
// immediately while the process is still executing.
func removeRunningBinary(exe, configDir string) {
	fmt.Printf("Removing stacyvm binary at: %s...\n", exe)
	if err := os.Remove(exe); err != nil {
		fmt.Printf("⚠️ Failed to delete binary: %v. You may need to run: sudo rm %s\n", err, exe)
		return
	}
	fmt.Println("✓ StacyVM binary removed successfully.")

	// The binary typically lives inside the config dir; now that it's gone,
	// clear out whatever directory shell is left behind.
	if err := os.RemoveAll(configDir); err != nil {
		fmt.Printf("⚠️ Failed to remove configuration directory: %v\n", err)
	}
}
