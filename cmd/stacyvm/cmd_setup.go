package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	setupTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF6600")).MarginBottom(1)
	promptStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CCFF"))
	selectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	normalStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))
)

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup wizard for StacyVM",
		RunE: func(cmd *cobra.Command, args []string) error {
			m := initialSetupModel()
			p := tea.NewProgram(m)
			if _, err := p.Run(); err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

type setupStep int

const (
	stepProvider setupStep = iota
	stepRuntime
	stepPreviewDomain
	stepDone
)

type setupModel struct {
	step           setupStep
	providerOpts   []string
	providerCursor int
	runtimeOpts    []string
	runtimeCursor  int
	previewDomain  string

	// Host availability checks
	dockerAvailable bool
	kvmAvailable    bool
	runscAvailable  bool
	kataAvailable   bool
}

func initialSetupModel() setupModel {
	_, dockerErr := exec.LookPath("docker")
	_, runscErr := exec.LookPath("runsc")
	_, kataErr := exec.LookPath("kata-runtime")

	// Check if KVM is available
	kvmAvailable := false
	if _, err := os.Stat("/dev/kvm"); err == nil {
		kvmAvailable = true
	}

	return setupModel{
		step:            stepProvider,
		providerOpts:    []string{"Docker (Recommended)", "Firecracker (Advanced)", "PRoot (Userspace)"},
		runtimeOpts:     []string{"runc (Standard)", "runsc (gVisor)", "kata (Kata Containers)"},
		previewDomain:   "localhost",
		dockerAvailable: dockerErr == nil,
		kvmAvailable:    kvmAvailable,
		runscAvailable:  runscErr == nil,
		kataAvailable:   kataErr == nil,
	}
}

func (m setupModel) Init() tea.Cmd {
	return nil
}

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.step == stepProvider && m.providerCursor > 0 {
				m.providerCursor--
			} else if m.step == stepRuntime && m.runtimeCursor > 0 {
				m.runtimeCursor--
			}
		case "down", "j":
			if m.step == stepProvider && m.providerCursor < len(m.providerOpts)-1 {
				m.providerCursor++
			} else if m.step == stepRuntime && m.runtimeCursor < len(m.runtimeOpts)-1 {
				m.runtimeCursor++
			}
		case "enter":
			if m.step == stepProvider {
				if m.providerCursor == 0 {
					m.step = stepRuntime
				} else {
					m.step = stepPreviewDomain
				}
			} else if m.step == stepRuntime {
				m.step = stepPreviewDomain
			} else if m.step == stepPreviewDomain {
				m.saveConfig()
				m.step = stepDone
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m setupModel) saveConfig() {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".stacyvm")
	os.MkdirAll(configDir, 0755)

	viper.SetConfigFile(filepath.Join(configDir, "config.yaml"))

	// Set defaults
	viper.Set("server.preview_domain", m.previewDomain)

	providerSelection := m.providerOpts[m.providerCursor]
	if strings.Contains(providerSelection, "Docker") {
		viper.Set("providers.default", "docker")
		runtimeSelection := "runc"
		if m.runtimeCursor == 1 {
			runtimeSelection = "runsc"
		} else if m.runtimeCursor == 2 {
			runtimeSelection = "kata"
		}
		viper.Set("providers.docker.runtime", runtimeSelection)
	} else if strings.Contains(providerSelection, "Firecracker") {
		viper.Set("providers.default", "firecracker")
	} else if strings.Contains(providerSelection, "PRoot") {
		viper.Set("providers.default", "proot")
	}

	viper.WriteConfigAs(filepath.Join(configDir, "config.yaml"))
}

func (m setupModel) View() string {
	if m.step == stepDone {
		return setupTitleStyle.Render("✨ StacyVM Setup Complete!") + "\nConfig saved to ~/.stacyvm/config.yaml\n"
	}

	var b strings.Builder
	b.WriteString(setupTitleStyle.Render("StacyVM Interactive Setup"))
	b.WriteString("\n\n")

	if m.step == stepProvider {
		b.WriteString(promptStyle.Render("? Select isolation provider:"))
		b.WriteString("\n")
		providerDescs := []string{
			"Works on macOS, Windows (WSL), and Linux.",
			"Requires native Linux host with KVM enabled. True microVMs.",
			"Zero privileges needed. Fallback option if others fail.",
		}
		for i, opt := range m.providerOpts {
			cursor := "  "
			style := normalStyle
			warning := ""
			if m.providerCursor == i {
				cursor = "> "
				style = selectedStyle
				if i == 0 && !m.dockerAvailable {
					warning = "\n     " + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF3333")).Render("⚠️  Docker was not found in your PATH!")
				} else if i == 1 && !m.kvmAvailable {
					warning = "\n     " + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF3333")).Render("⚠️  KVM is not available! Firecracker requires native Linux with /dev/kvm.")
				}
			}
			b.WriteString(fmt.Sprintf("%s%s\n     %s%s\n", cursor, style.Render(opt), lipgloss.NewStyle().Foreground(lipgloss.Color("#777777")).Render(providerDescs[i]), warning))
		}
	} else if m.step == stepRuntime {
		b.WriteString(promptStyle.Render("? Select Docker Runtime Security:"))
		b.WriteString("\n")
		runtimeDescs := []string{
			"Standard container isolation (default).",
			"Requires manual 'runsc' installation & Docker config on host. Strong sandboxing.",
			"Requires manual 'kata-runtime' installation. VM-level isolation.",
		}
		for i, opt := range m.runtimeOpts {
			cursor := "  "
			style := normalStyle
			warning := ""
			if m.runtimeCursor == i {
				cursor = "> "
				style = selectedStyle
				if i == 1 && !m.runscAvailable {
					warning = "\n     " + lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00")).Render("⚠️  'runsc' (gVisor) binary not found! You must install it manually first.")
				} else if i == 2 && !m.kataAvailable {
					warning = "\n     " + lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00")).Render("⚠️  'kata-runtime' binary not found! You must install it manually first.")
				}
			}
			b.WriteString(fmt.Sprintf("%s%s\n     %s%s\n", cursor, style.Render(opt), lipgloss.NewStyle().Foreground(lipgloss.Color("#777777")).Render(runtimeDescs[i]), warning))
		}
	} else if m.step == stepPreviewDomain {
		b.WriteString(promptStyle.Render("? Setup preview domain [press enter]: "))
		b.WriteString(m.previewDomain)
		b.WriteString("\n")
	}

	b.WriteString("\n(Use arrow keys to move, Enter to select, q to quit)\n")
	return b.String()
}
