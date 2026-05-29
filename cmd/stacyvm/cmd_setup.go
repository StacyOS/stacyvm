package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	setupTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFA60C")).MarginBottom(1)
	successStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Bold(true)
	warningStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA60C"))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF3333"))
)

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup wizard for StacyVM",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup()
		},
	}
	return cmd
}

func runSetup() error {
	fmt.Println(setupTitleStyle.Render("StacyVM Interactive Setup"))

	_, dockerErr := exec.LookPath("docker")
	_, runscErr := exec.LookPath("runsc")
	_, kataErr := exec.LookPath("kata-runtime")
	
	kvmAvailable := false
	if _, err := os.Stat("/dev/kvm"); err == nil {
		kvmAvailable = true
	}

	var provider string
	var runtime string
	var domain string = "localhost"

	// Define theme
	t := huh.ThemeCharm()
	t.Focused.Base = t.Focused.Base.BorderForeground(lipgloss.Color("#FFA60C"))
	t.Focused.Title = t.Focused.Title.Foreground(lipgloss.Color("#FFA60C")).Bold(true)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(lipgloss.Color("#22C55E"))
	t.Focused.Description = t.Focused.Description.Foreground(lipgloss.Color("#888888"))

	dockerLabel := "Docker (Recommended) - Works on macOS, Windows (WSL), and Linux."
	if dockerErr != nil {
		dockerLabel += " ⚠️ Docker was not found in your PATH!"
	}
	firecrackerLabel := "Firecracker (Advanced) - Requires native Linux host with KVM enabled. True microVMs."
	if !kvmAvailable {
		firecrackerLabel += " ⚠️ KVM is not available!"
	}
	prootLabel := "PRoot (Userspace) - Zero privileges needed. Fallback option."

	// Create Form
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select isolation provider:").
				Options(
					huh.NewOption(dockerLabel, "docker"),
					huh.NewOption(firecrackerLabel, "firecracker"),
					huh.NewOption(prootLabel, "proot"),
				).
				Value(&provider),
		),
	).WithTheme(t)

	err := form.Run()
	if err != nil {
		return err
	}

	// If Docker, ask for runtime
	if provider == "docker" {
		runcLabel := "Standard container isolation (default)."
		runscLabel := "runsc (gVisor) - Requires manual installation. Strong sandboxing."
		if runscErr != nil {
			runscLabel += " ⚠️ 'runsc' binary not found!"
		}
		kataLabel := "kata (Kata Containers) - Requires manual installation. VM-level isolation."
		if kataErr != nil {
			kataLabel += " ⚠️ 'kata-runtime' binary not found!"
		}

		runtimeForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Select Docker Runtime Security:").
					Options(
						huh.NewOption(runcLabel, "runc"),
						huh.NewOption(runscLabel, "runsc"),
						huh.NewOption(kataLabel, "kata"),
					).
					Value(&runtime),
			),
		).WithTheme(t)
		
		err = runtimeForm.Run()
		if err != nil {
			return err
		}
	}

	// Domain
	domainForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Setup preview domain").
				Value(&domain),
		),
	).WithTheme(t)
	
	err = domainForm.Run()
	if err != nil {
		return err
	}

	// Save Config
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".stacyvm")
	os.MkdirAll(configDir, 0755)

	viper.SetConfigFile(filepath.Join(configDir, "config.yaml"))
	viper.Set("server.preview_domain", domain)
	viper.Set("providers.default", provider)
	if provider == "docker" {
		viper.Set("providers.docker.runtime", runtime)
	}

	viper.WriteConfigAs(filepath.Join(configDir, "config.yaml"))

	fmt.Println("\n" + successStyle.Render("✨ StacyVM Setup Complete!"))
	fmt.Println("Config saved to ~/.stacyvm/config.yaml")

	return nil
}
