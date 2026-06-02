package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newUninstallCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Completely uninstall StacyVM configuration, database, and binaries",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("could not locate user home directory: %w", err)
			}
			configDir := filepath.Join(home, ".stacyvm")

			if !force {
				fmt.Printf("⚠️ This will permanently delete all StacyVM configuration, database files, and clean up the application registry.\n")
				fmt.Printf("Are you sure you want to proceed? [y/N]: ")
				reader := bufio.NewReader(os.Stdin)
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(strings.ToLower(input))
				if input != "y" && input != "yes" {
					fmt.Println("Uninstall cancelled.")
					return nil
				}
			}

			// Resolve the running binary first so we can avoid trying to
			// delete it out from under ourselves. On Windows the OS holds a
			// lock on a running executable, so it must be deleted only after
			// this process exits (handled by removeRunningBinary below).
			exe, exeErr := os.Executable()
			if exeErr == nil {
				if resolved, err := filepath.EvalSymlinks(exe); err == nil {
					exe = resolved
				}
			}

			// Clean up the config directory, skipping the running binary if it
			// happens to live inside it (the default install layout puts the
			// binary at <configDir>/bin/stacyvm[.exe]).
			if _, err := os.Stat(configDir); err == nil {
				fmt.Printf("Removing configuration directory: %s...\n", configDir)
				if err := removeAllExcept(configDir, exe); err != nil {
					fmt.Printf("⚠️ Failed to remove configuration directory: %v\n", err)
				} else {
					fmt.Println("✓ Removed configuration directory.")
				}
			}

			// Clean up local database if any
			localDB := "stacyvm.db"
			if _, err := os.Stat(localDB); err == nil {
				fmt.Printf("Removing local database file: %s...\n", localDB)
				if err := os.Remove(localDB); err != nil {
					fmt.Printf("⚠️ Failed to remove local database file: %v\n", err)
				} else {
					fmt.Println("✓ Removed local database file.")
				}
			}

			// Undo the shell integration that `stacyvm setup` / the installer
			// added (PATH entries and tab-completion blocks). Without this a new
			// shell still tries to source completion for a binary that's gone,
			// which is the "stacyvm: command not found" noise in ~/.bashrc.
			cleanupShellIntegration()

			// Clean up the running binary itself. The mechanism differs per OS
			// (see removeRunningBinary), but the user-facing behaviour is the
			// same: the binary and its directory are removed.
			if exeErr != nil {
				fmt.Printf("Note: could not locate running binary to remove: %v\n", exeErr)
			} else {
				removeRunningBinary(exe, configDir)
			}

			fmt.Println("✨ Uninstall completed.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force uninstall without confirmation prompt")
	return cmd
}

// removeAllExcept removes everything under dir except the file at the absolute
// path `except` (and the ancestor directories needed to reach it). If `except`
// is empty or outside dir, it behaves like os.RemoveAll(dir).
func removeAllExcept(dir, except string) error {
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		dirAbs = dir
	}

	// Fast path: nothing to preserve inside this directory.
	if except == "" || !isWithin(dirAbs, except) {
		return os.RemoveAll(dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := filepath.Join(dirAbs, entry.Name())
		if entry.IsDir() {
			if isWithin(child, except) {
				// Recurse so we preserve `except` but drop everything else.
				if err := removeAllExcept(child, except); err != nil {
					return err
				}
				continue
			}
			if err := os.RemoveAll(child); err != nil {
				return err
			}
			continue
		}
		if sameFile(child, except) {
			continue
		}
		if err := os.Remove(child); err != nil {
			return err
		}
	}
	return nil
}

// isWithin reports whether target is the directory dir itself or lives beneath
// it. Both are compared as cleaned absolute paths.
func isWithin(dir, target string) bool {
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		dirAbs = filepath.Clean(dir)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		targetAbs = filepath.Clean(target)
	}
	rel, err := filepath.Rel(dirAbs, targetAbs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

// autocompleteMarker is the comment line that `stacyvm setup` writes above the
// shell completion block in every supported rc/profile file.
const autocompleteMarker = "# StacyVM Autocomplete"

// removeAutocompleteFromFile strips the StacyVM autocomplete block from the
// given shell rc/profile file, leaving the rest of the file untouched. Missing
// or unreadable files are silently ignored. terminators are the closing tokens
// for the host shell ("fi", "end", "}").
func removeAutocompleteFromFile(path string, terminators ...string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	// removeAutocompleteBlock splits/joins on "\n", which leaves any trailing
	// "\r" attached to each surviving line, so CRLF profiles stay CRLF.
	updated, changed := removeAutocompleteBlock(string(b), terminators...)
	if !changed {
		return
	}
	// Preserve the existing file mode so we don't widen permissions (e.g. an
	// 0600 profile becoming 0644).
	_ = os.WriteFile(path, []byte(updated), info.Mode().Perm())
}

// removeAutocompleteBlock removes the contiguous "# StacyVM Autocomplete" block
// (and a single blank line preceding it) from content. The block runs from the
// marker line through the first matching terminator; if no terminator is found
// it extends only across lines that look like part of our block, so we never
// eat unrelated user configuration. Returns the new content and whether a block
// was removed.
func removeAutocompleteBlock(content string, terminators ...string) (string, bool) {
	lines := strings.Split(content, "\n")

	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == autocompleteMarker {
			start = i
			break
		}
	}
	if start == -1 {
		return content, false
	}

	termSet := make(map[string]bool, len(terminators))
	for _, t := range terminators {
		termSet[t] = true
	}

	end := start // last index to remove (inclusive); defaults to marker only
	for i := start + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if termSet[t] {
			end = i
			break
		}
		if !isAutocompleteBlockLine(t) {
			break
		}
		end = i
	}

	// Also drop a single blank separator line immediately before the marker.
	delStart := start
	if delStart > 0 && strings.TrimSpace(lines[delStart-1]) == "" {
		delStart--
	}

	remaining := append([]string{}, lines[:delStart]...)
	remaining = append(remaining, lines[end+1:]...)
	return strings.Join(remaining, "\n"), true
}

// isAutocompleteBlockLine reports whether a trimmed line looks like part of the
// StacyVM autocomplete block (PATH/path tweaks plus the guarded completion
// sourcing), across bash, zsh, fish and PowerShell.
func isAutocompleteBlockLine(t string) bool {
	if t == "" {
		return false
	}
	for _, tok := range []string{
		"stacyvm",
		"export PATH",
		"fish_add_path",
		"Get-Command",
		"type -q",
		"source",
		"Invoke-Expression",
	} {
		if strings.Contains(t, tok) {
			return true
		}
	}
	return false
}

// sameFile reports whether two paths refer to the same file location.
func sameFile(a, b string) bool {
	aAbs, err := filepath.Abs(a)
	if err != nil {
		aAbs = filepath.Clean(a)
	}
	bAbs, err := filepath.Abs(b)
	if err != nil {
		bAbs = filepath.Clean(b)
	}
	return filepath.Clean(aAbs) == filepath.Clean(bAbs)
}
