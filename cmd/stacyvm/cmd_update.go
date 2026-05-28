package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update StacyVM to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if version == "dev" {
				fmt.Println("You are running a development build (version 'dev').")
				fmt.Println("Self-update is disabled for development builds to prevent overwriting local changes.")
				return nil
			}
			
			fmt.Println("Checking for updates...")
			latest, found, err := selfupdate.DetectLatest("StacyOS/stacyvm")
			if err != nil {
				return fmt.Errorf("error occurred while checking for updates: %w", err)
			}
			
			if !found {
				return fmt.Errorf("no release found for StacyOS/stacyvm")
			}
			
			// Compare versions. The library parses semver nicely.
			if latest.Version.String() == strings.TrimPrefix(version, "v") {
				fmt.Printf("Current version (%s) is the latest.\n", version)
				return nil
			}
			
			fmt.Printf("Found latest version: %s\n", latest.Version)
			fmt.Printf("Downloading from %s...\n", latest.AssetURL)
			
			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("could not locate executable path: %w", err)
			}
			
			if err := selfupdate.UpdateTo(latest.AssetURL, exe); err != nil {
				return fmt.Errorf("error occurred while updating binary: %w", err)
			}
			
			fmt.Printf("Successfully updated to version %s\n", latest.Version)
			return nil
		},
	}
	return cmd
}
