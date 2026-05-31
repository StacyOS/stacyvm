package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	// Autocomplete
	var enableAutocomplete bool
	autocompleteForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Install shell auto-completion?").
				Description("Automatically configures autocomplete for Bash, Zsh, and Fish.").
				Value(&enableAutocomplete),
		),
	).WithTheme(t)
	_ = autocompleteForm.Run()

	if enableAutocomplete {
		installAutocomplete()
	}

	fmt.Println("\n" + successStyle.Render("✨ StacyVM Setup Complete!"))
	fmt.Println("Config saved to ~/.stacyvm/config.yaml")

	if enableAutocomplete {
		fmt.Println("\n" + warningStyle.Render("Auto-completion installed! Please restart your terminal or run:"))
		fmt.Println("  source ~/.zshrc  (or ~/.bashrc)")
	}

	return nil
}

func installAutocomplete() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	localBin := filepath.Join(home, ".local", "bin")

	// Zsh
	zshrc := filepath.Join(home, ".zshrc")
	installShellCompletion(
		zshrc,
		"stacyvm completion zsh",
		shellPathExport(localBin),
		"\n# StacyVM Autocomplete\n"+shellPathExport(localBin)+"if command -v stacyvm >/dev/null 2>&1; then\n  source <(stacyvm completion zsh)\nfi\n",
	)

	// Bash
	bashrc := filepath.Join(home, ".bashrc")
	installShellCompletion(
		bashrc,
		"stacyvm completion bash",
		shellPathExport(localBin),
		"\n# StacyVM Autocomplete\n"+shellPathExport(localBin)+"if command -v stacyvm >/dev/null 2>&1; then\n  source <(stacyvm completion bash)\nfi\n",
	)

	// Fish
	fishConfig := filepath.Join(home, ".config", "fish", "config.fish")
	installShellCompletion(
		fishConfig,
		"stacyvm completion fish",
		fishPathExport(localBin),
		"\n# StacyVM Autocomplete\n"+fishPathExport(localBin)+"if type -q stacyvm\n  stacyvm completion fish | source\nend\n",
	)
}

func installShellCompletion(configPath, completionMarker, pathSnippet, completionBlock string) {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	content := string(b)
	if strings.Contains(content, completionMarker) {
		if pathSnippet != "" && !shellConfigHasLocalBin(content) {
			content = insertBeforeCompletion(content, completionMarker, pathSnippet)
			_ = os.WriteFile(configPath, []byte(content), 0644)
		}
		return
	}

	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(completionBlock)
}

func shellPathExport(localBin string) string {
	return fmt.Sprintf("export PATH=%q:$PATH\n", localBin)
}

func fishPathExport(localBin string) string {
	return fmt.Sprintf("fish_add_path %q\n", localBin)
}

func shellConfigHasLocalBin(content string) bool {
	return strings.Contains(content, ".local/bin")
}

func insertBeforeCompletion(content, completionMarker, snippet string) string {
	idx := strings.Index(content, completionMarker)
	if idx == -1 {
		return content + snippet
	}
	lineStart := strings.LastIndex(content[:idx], "\n")
	if lineStart == -1 {
		return snippet + content
	}
	return content[:lineStart+1] + snippet + content[lineStart+1:]
}
