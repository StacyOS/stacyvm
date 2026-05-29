package main

import (
	"context"
	"image/color"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	version = "dev"

	serverURL string
	apiKey    string
)

func main() {
	root := &cobra.Command{
		Use:   "stacyvm",
		Short: "StacyVM — microVM sandbox orchestrator for LLMs",
		Long:  "StacyVM gives any LLM the ability to spawn isolated sandboxes, execute code, manage files, and tear it all down.",
	}

	root.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:7423", "StacyVM server URL")
	root.PersistentFlags().StringVar(&apiKey, "api-key", os.Getenv("STACYVM_API_KEY"), "API key for authentication")

	root.AddCommand(
		newServeCmd(),
		newSetupCmd(),
		newWorkerCmd(),
		newSpawnCmd(),
		newExecCmd(),
		newKillCmd(),
		newListCmd(),
		newVersionCmd(),
		newTUICmd(),
		newWebUICmd(),
		newBuildImageCmd(),
		newDoctorCmd(),
		newConfigCmd(),
		newDBCmd(),
		newUpgradeCmd(),
		newUpdateCmd(),
		newUninstallCmd(),
		newSupportCmd(),
	)

	theme := getBrandTheme()
	if err := fang.Execute(context.Background(), root, fang.WithTheme(theme)); err != nil {
		os.Exit(1)
	}
}

func getBrandTheme() fang.ColorScheme {
	return fang.ColorScheme{
		Base:           lipgloss.Color("#F9F7F3"), // Off White
		Title:          lipgloss.Color("#FFA60C"), // Brand Orange
		Description:    lipgloss.Color("#F9F7F3"), // Off White
		Codeblock:      lipgloss.Color("#D7F6E2"), // Mint
		Program:        lipgloss.Color("#FFA60C"), // Orange
		DimmedArgument: lipgloss.Color("#888888"), // Dim
		Comment:        lipgloss.Color("#888888"),
		Flag:           lipgloss.Color("#22C55E"), // Green
		FlagDefault:    lipgloss.Color("#888888"),
		Command:        lipgloss.Color("#FFA60C"), // Orange
		QuotedString:   lipgloss.Color("#D7F6E2"), // Mint
		Argument:       lipgloss.Color("#FDF6D6"), // Cream
		Help:           lipgloss.Color("#888888"),
		Dash:           lipgloss.Color("#888888"),
		ErrorHeader:    [2]color.Color{lipgloss.Color("#0C0C0C"), lipgloss.Color("#FFA60C")}, // Black on Orange
		ErrorDetails:   lipgloss.Color("#FF3333"),
	}
}

func getClient() *httpClient {
	return newHTTPClient(serverURL, apiKey)
}
