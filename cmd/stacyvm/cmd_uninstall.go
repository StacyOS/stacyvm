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

			// Clean up config directory
			if _, err := os.Stat(configDir); err == nil {
				fmt.Printf("Removing configuration directory: %s...\n", configDir)
				if err := os.RemoveAll(configDir); err != nil {
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

			// Clean up running binary itself
			exe, err := os.Executable()
			if err == nil {
				fmt.Printf("Removing stacyvm binary at: %s...\n", exe)
				
				// On Unix-like systems, we can unlink the running executable
				// On Windows, the binary is locked while running
				err := os.Remove(exe)
				if err != nil {
					if os.PathSeparator == '\\' {
						fmt.Printf("Note: On Windows, the running binary is locked. Please manually delete this file: %s\n", exe)
					} else {
						fmt.Printf("⚠️ Failed to delete binary: %v. You may need to run: sudo rm %s\n", err, exe)
					}
				} else {
					fmt.Println("✓ StacyVM binary removed successfully.")
				}
			}

			fmt.Println("✨ Uninstall completed.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force uninstall without confirmation prompt")
	return cmd
}
