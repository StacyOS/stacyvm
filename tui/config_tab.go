package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func (m Model) viewConfig(height, width int) string {
	leftW := (width - 1) / 2
	rightW := width - 1 - leftW
	return lipgloss.JoinHorizontal(lipgloss.Top, m.configProviders(leftW), " ", m.configServer(rightW))
}

// segmented renders a labelled segmented control; the option at cursorIdx
// (relative to base) is highlighted as the candidate to apply.
func (m Model) configSegmented(title string, opts []string, base int) string {
	var segs []string
	for i, o := range opts {
		idx := base + i
		if m.configCursor == idx {
			segs = append(segs, lipgloss.NewStyle().Foreground(colOrange).
				Border(lipgloss.NormalBorder(), false, true, false, true).
				BorderForeground(colOrange).Render(o))
		} else {
			segs = append(segs, stFaint.Render(" "+o+" "))
		}
	}
	return stDim.Render(title) + "\n" + strings.Join(segs, "  ")
}

func (m Model) configProviders(width int) string {
	body := strings.Join([]string{
		m.configSegmented("default provider", []string{"docker", "firecracker", "proot"}, 0),
		"",
		m.configSegmented("docker runtime", []string{"runc", "runsc", "kata"}, 3),
		"",
		keyHints([]hint{{"j/k", "move"}, {"space", "apply"}}),
	}, "\n")
	return panel(glyphPaneSpawn+" PROVIDERS", "", body, width, true)
}

func (m Model) configServer(width int) string {
	kv := func(k, v string) string { return stDim.Render(padRight(k, 16)) + stInk.Render(v) }
	body := strings.Join([]string{
		kv("address", m.client.baseURL),
		kv("log format", "pretty"),
		kv("preview domain", "*.stacy.dev"),
		"",
		stFaint.Render(glyphEnter + " edit any key · changes patch live"),
	}, "\n")
	return panel("SERVER", "", body, width, false)
}
