package tui

// providers.go — screen 10, "Providers" (Build 1 A): one health card per
// runtime backend with real health, runtime/mode, sandbox count + latency spark.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type providerDetailMsg providerDetailData

func (m Model) fetchProviderDetail(name string) tea.Cmd {
	return func() tea.Msg {
		d, err := m.client.providerDetail(name)
		if err != nil {
			return providerDetailMsg{Name: name}
		}
		return providerDetailMsg(d)
	}
}

func (m *Model) handleProvidersKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h", "k", "up":
		if m.providerCursor > 0 {
			m.providerCursor--
		}
	case "right", "l", "j", "down":
		if m.providerCursor < len(m.providerList)-1 {
			m.providerCursor++
		}
	case "enter":
		if m.providerCursor < len(m.providerList) {
			p := m.providerList[m.providerCursor]
			return m, m.patchConfigCmd(map[string]interface{}{"providers.default": p.Name})
		}
	}
	return m, nil
}

func (m Model) viewProviders(height, width int) string {
	if len(m.providerList) == 0 {
		return panel("PROVIDERS", "", stFaint.Render("no providers configured"), width, false)
	}

	n := len(m.providerList)
	gap := n - 1
	cardW := (width - gap) / n

	cards := make([]string, 0, n)
	for i, p := range m.providerList {
		cards = append(cards, m.providerCard(p, cardW, i == m.providerCursor))
	}
	row := joinRow(cards, width)

	hint := stFaint.Render("set a default with ↵") + "   " +
		keyHints([]hint{{"←/→", "select"}, {"r", "refresh"}, {glyphEnter, "set default"}})
	return row + "\n\n" + hint
}

func (m Model) providerCard(p providerData, width int, selected bool) string {
	detail := m.providerDetails[p.Name]

	// Health line.
	var health string
	if p.Healthy {
		health = stOK.Render(glyphDotRun + " healthy")
	} else {
		health = stDim.Render(glyphDotIdle + " standby")
	}

	// Sandbox count: prefer the detail's live count, else runtime_count.
	count := detail.SandboxCount
	if count == 0 && p.RuntimeCount != nil {
		count = *p.RuntimeCount
	}

	lines := []string{
		health,
		m.providerDescriptor(p, detail),
		stDim.Render(padRight("sandboxes", 12)) + stInk.Render(itoa(count)),
		stDim.Render(padRight("latency", 12)) + stInk.Render(fmt.Sprintf("%dms", p.LatencyMS)) +
			"  " + spark(m.provLatency[p.Name].slice(), 7, stOK),
	}

	hint := ""
	if p.IsDefault {
		hint = "DEFAULT"
	}
	accent := p.IsDefault || selected
	title := strings.ToUpper(p.Name)
	if selected {
		title = stHi.Render("▸ ") + title
	}
	return panel(title, hint, strings.Join(lines, "\n"), width, accent)
}

// providerDescriptor renders the type-specific config line for a provider.
func (m Model) providerDescriptor(p providerData, d providerDetailData) string {
	kv := func(k, v string) string { return stDim.Render(padRight(k, 12)) + stInk.Render(v) }
	switch d.Config["type"] {
	case "docker":
		rt := d.Config["runtime"]
		if rt == "" {
			rt = "runc"
		}
		return kv("runtime", rt)
	case "firecracker":
		return kv("kernel", "microVM · vsock")
	case "proot":
		return kv("mode", "userspace")
	default:
		if len(p.Capabilities) > 0 {
			return kv("caps", truncate(strings.Join(p.Capabilities, " "), 26))
		}
		return stFaint.Render("—")
	}
}
