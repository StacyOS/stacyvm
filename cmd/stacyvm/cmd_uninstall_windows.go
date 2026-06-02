//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// cleanupShellIntegration removes the StacyVM autocomplete block from the
// PowerShell profile(s) and strips the .stacyvm\bin entry from the User PATH,
// mirroring what the installer / `stacyvm setup` added.
func cleanupShellIntegration() {
	for _, shell := range []string{"powershell", "pwsh"} {
		if profile := powershellProfilePath(shell); profile != "" {
			// PowerShell blocks close with "}".
			removeAutocompleteFromFile(profile, "}")
		}
	}
	removeStacyFromUserPath()
}

// removeStacyFromUserPath removes <home>\.stacyvm\bin from the persistent User
// PATH so an uninstalled StacyVM leaves no dangling PATH entry behind. Best
// effort: failures are ignored since the directory itself is being deleted.
func removeStacyFromUserPath() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	installDir := filepath.Join(home, ".stacyvm", "bin")
	// Escape single quotes so a username with an apostrophe can't break out of
	// the PowerShell string literal.
	escaped := strings.ReplaceAll(installDir, "'", "''")
	psCommand := fmt.Sprintf(
		`$parts = @([Environment]::GetEnvironmentVariable('Path','User') -split ';' | `+
			`Where-Object { $_ -ne '' -and $_ -ne '%s' }); `+
			`[Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')`,
		escaped,
	)
	_ = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCommand).Run()
}

// Windows process creation flags (see CreateProcess docs). DETACHED_PROCESS
// makes the helper fully independent of this process so it survives our exit.
const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

// removeRunningBinary schedules deletion of the currently running executable.
//
// On Windows the kernel holds a mandatory lock on the image file of any running
// .exe, so it cannot be deleted while this process is alive (this is why a
// plain os.Remove fails with "Access is denied"). Instead we spawn a detached
// helper batch script that waits for this process to exit — at which point the
// lock is released — then deletes the binary and the (now otherwise empty)
// config directory.
func removeRunningBinary(exe, configDir string) {
	fmt.Printf("Removing stacyvm binary at: %s...\n", exe)

	if err := scheduleSelfDelete(exe, configDir); err != nil {
		fmt.Printf("⚠️ Could not schedule binary removal: %v\n", err)
		fmt.Printf("Note: please manually delete this file once StacyVM exits: %s\n", exe)
		return
	}
	fmt.Println("✓ StacyVM binary will be removed once this process exits.")
}

// scheduleSelfDelete writes a batch script to the temp directory and launches it
// fully detached. The script polls (with a bounded number of retries) until the
// executable is unlocked, deletes it, then removes the config directory.
func scheduleSelfDelete(exe, configDir string) error {
	// NOTE: paths are written with plain backslashes inside double quotes.
	// Do NOT use %q here — it Go-escapes backslashes (C:\\Users\\...), which
	// is invalid in a .bat file and makes del/rmdir silently fail.
	script := fmt.Sprintf(`@echo off
setlocal
set "target=%s"
set "cfg=%s"
set /a tries=0
:waitloop
del "%%target%%" >nul 2>&1
if not exist "%%target%%" goto cleanup
set /a tries+=1
if %%tries%% geq 30 goto cleanup
ping -n 2 127.0.0.1 >nul
goto waitloop
:cleanup
rmdir /s /q "%%cfg%%" >nul 2>&1
del "%%~f0" >nul 2>&1
`, exe, configDir)

	batPath := filepath.Join(os.TempDir(), "stacyvm_uninstall.bat")
	if err := os.WriteFile(batPath, []byte(script), 0o644); err != nil {
		return fmt.Errorf("writing cleanup script: %w", err)
	}

	// Launch the batch directly as a detached, window-less process so it keeps
	// running after this process exits and can delete the now-unlocked binary.
	cmd := exec.Command("cmd", "/C", batPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching cleanup script: %w", err)
	}
	// Don't Wait — we want it to keep running after we exit.
	return nil
}
