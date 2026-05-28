package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleConfigKey(key string) (tea.Model, tea.Cmd) {
	opts := []string{
		"providers.default=docker",
		"providers.default=firecracker",
		"providers.default=proot",
		"providers.docker.runtime=runc",
		"providers.docker.runtime=runsc",
		"providers.docker.runtime=kata",
	}

	switch key {
	case "j", "down":
		if m.configCursor < len(opts)-1 {
			m.configCursor++
		}
	case "k", "up":
		if m.configCursor > 0 {
			m.configCursor--
		}
	case " ", "enter":
		opt := opts[m.configCursor]
		parts := strings.Split(opt, "=")
		if len(parts) == 2 {
			payload := map[string]interface{}{
				parts[0]: parts[1],
			}
			return m, m.patchConfigCmd(payload)
		}
	}
	return m, nil
}

func (m *Model) patchConfigCmd(payload map[string]interface{}) tea.Cmd {
	return func() tea.Msg {
		msg, err := m.client.patchConfig(payload)
		if err != nil {
			return errMsg(err)
		}
		return configPatchedMsg(msg)
	}
}

type configPatchedMsg string

func (m *Model) viewConfig(height, width int) string {
	var b strings.Builder
	b.WriteString(boldStyle.Render(" Configuration Management\n\n"))

	opts := []string{
		"providers.default=docker",
		"providers.default=firecracker",
		"providers.default=proot",
		"providers.docker.runtime=runc",
		"providers.docker.runtime=runsc",
		"providers.docker.runtime=kata",
	}

	descriptions := []string{
		"Set default provider to Docker (Recommended)",
		"Set default provider to Firecracker (Requires Linux KVM)",
		"Set default provider to PRoot (Userspace emulation)",
		"Set Docker runtime to standard (runc)",
		"Set Docker runtime to gVisor (runsc)",
		"Set Docker runtime to Kata Containers (kata)",
	}

	for i, opt := range opts {
		cursor := "  "
		style := normalRowStyle
		if m.configCursor == i {
			cursor = "> "
			style = selectedRowStyle
		}
		b.WriteString(fmt.Sprintf("%s%s - %s\n", cursor, style.Render(opt), dimStyle.Render(descriptions[i])))
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Press Space or Enter to apply the selected setting.\n"))

	return b.String()
}
