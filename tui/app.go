package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0C0C0C")).
			Background(lipgloss.Color("#FFA60C")).
			Padding(0, 2)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F9F7F3")).
			Background(lipgloss.Color("#0C0C0C"))

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFA60C")).
			Underline(true).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888")).
				Padding(0, 1)

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFA60C"))

	normalRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F9F7F3"))

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFA60C")).
			Underline(true)

	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF3333"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA60C"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	boldStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F9F7F3"))
	outputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#D7F6E2"))
	cardStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FFA60C")).Padding(0, 1)
	gaugeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
)

type tab int

const (
	tabDashboard tab = iota
	tabSandboxes
	tabTemplates
	tabProviders
	tabLogs
	tabConfig
)

type mode int

const (
	modeNormal mode = iota
	modeInput
	modeConfirm
	modeExec
	modeSpawn
	modeSpawning
	modeSandboxAction
	modeSpawnTemplate
	modeCreateTemplate
	modeConfigEdit
)

// Messages
type tickMsg time.Time
type sandboxesMsg []sandboxData
type healthMsg *healthData
type providersMsg []providerData
type templatesMsg []templateData
type spawnedMsg *sandboxData
type destroyedMsg string
type execMsg *execResultData
type fileWrittenMsg struct{}
type fileReadMsg string
type templateCreatedMsg struct{}
type templateDeletedMsg string
type errMsg error
type frameMsg time.Time

type Model struct {
	client  *apiClient
	width   int
	height  int
	activeTab tab
	mode    mode

	// Sandbox list
	sandboxes []sandboxData
	cursor    int

	// Providers
	providerList []providerData

	// Templates
	templateList   []templateData
	templateCursor int

	// Inputs
	inputs     []textinput.Model
	inputFocus int

	// State
	health      *healthData
	lastOutput  string
	lastError   string
	statusMsg   string
	confirmMsg  string
	confirmFunc func() tea.Cmd
	logs        []string

	// Config
	configCursor int

	// Telemetry (real data → client-side ring buffers)
	host     hostSnapshot
	sbStats  map[string]sandboxStat
	teleCPU  *ring
	teleMEM  *ring
	teleLOAD *ring
	teleDISK *ring
	teleNET  *ring
	kpiSB    *ring
	kpiTmpl  *ring
	kpiProv  *ring
	clock    time.Time
	cursorOn bool

	// Event stream (SSE bus)
	events  []eventEntry
	eventCh chan eventEntry

	// Animation
	slideSpring   harmonica.Spring
	slidePos      float64
	slideVelocity float64

	modalPos      float64
	modalVelocity float64

	loaderSpring   harmonica.Spring
	loaderPos      float64
	loaderVelocity float64
	loaderTarget   float64
}

func NewModel(serverURL, apiKey string) Model {
	client := newAPIClient(serverURL, apiKey)

	// 0: image, 1: ttl, 2: command, 3: filepath, 4: filecontent
	// 5: tmpl name, 6: tmpl image, 7: tmpl desc, 8: tmpl memory, 9: tmpl cpus, 10: tmpl ttl
	imageInput := textinput.New()
	imageInput.Placeholder = "alpine:latest"
	imageInput.CharLimit = 100
	imageInput.Width = 40

	ttlInput := textinput.New()
	ttlInput.Placeholder = "30m"
	ttlInput.CharLimit = 20
	ttlInput.Width = 20

	cmdInput := textinput.New()
	cmdInput.Placeholder = "echo hello world"
	cmdInput.CharLimit = 500
	cmdInput.Width = 60

	filePathInput := textinput.New()
	filePathInput.Placeholder = "/workspace/file.txt"
	filePathInput.CharLimit = 200
	filePathInput.Width = 40

	fileContentInput := textinput.New()
	fileContentInput.Placeholder = "file contents..."
	fileContentInput.CharLimit = 5000
	fileContentInput.Width = 60

	tmplNameInput := textinput.New()
	tmplNameInput.Placeholder = "data-science"
	tmplNameInput.CharLimit = 50
	tmplNameInput.Width = 30

	tmplImageInput := textinput.New()
	tmplImageInput.Placeholder = "python:3.12-alpine"
	tmplImageInput.CharLimit = 100
	tmplImageInput.Width = 40

	tmplDescInput := textinput.New()
	tmplDescInput.Placeholder = "description..."
	tmplDescInput.CharLimit = 200
	tmplDescInput.Width = 50

	tmplMemInput := textinput.New()
	tmplMemInput.Placeholder = "512"
	tmplMemInput.CharLimit = 10
	tmplMemInput.Width = 10

	tmplCPUInput := textinput.New()
	tmplCPUInput.Placeholder = "1"
	tmplCPUInput.CharLimit = 5
	tmplCPUInput.Width = 10

	tmplTTLInput := textinput.New()
	tmplTTLInput.Placeholder = "300"
	tmplTTLInput.CharLimit = 10
	tmplTTLInput.Width = 10

	return Model{
		client: client,
		inputs: []textinput.Model{
			imageInput, ttlInput, cmdInput, filePathInput, fileContentInput,
			tmplNameInput, tmplImageInput, tmplDescInput, tmplMemInput, tmplCPUInput, tmplTTLInput,
		},
		logs: make([]string, 0, 100),
		slideSpring:  harmonica.NewSpring(harmonica.FPS(60), 12.0, 0.8),
		loaderSpring: harmonica.NewSpring(harmonica.FPS(60), 6.0, 0.2), // bouncy spring

		// Telemetry rings + event stream
		sbStats:  map[string]sandboxStat{},
		teleCPU:  newRing(8),
		teleMEM:  newRing(8),
		teleLOAD: newRing(8),
		teleDISK: newRing(8),
		teleNET:  newRing(8),
		kpiSB:    newRing(8),
		kpiTmpl:  newRing(8),
		kpiProv:  newRing(8),
		cursorOn: true,
		events:   make([]eventEntry, 0, 200),
		eventCh:  make(chan eventEntry, 64),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchSandboxes(),
		m.fetchHealth(),
		m.fetchProviders(),
		m.fetchTemplates(),
		m.tick(),
		m.fetchHostStats(),
		teleTick(),
		blinkTick(),
		m.streamEvents(),
		m.waitEvent(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		statusBarStyle = statusBarStyle.Width(m.width)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tickMsg:
		return m, tea.Batch(m.fetchSandboxes(), m.fetchHealth(), m.fetchProviders(), m.fetchTemplates(), m.tick())

	case teleTickMsg:
		m.clock = time.Time(msg)
		m.pushKPI()
		return m, tea.Batch(m.fetchHostStats(), m.fetchAllSandboxStats(), teleTick())

	case blinkMsg:
		m.cursorOn = !m.cursorOn
		return m, blinkTick()

	case hostStatsMsg:
		m.host = hostSnapshot(msg)
		m.pushTelemetry()
		return m, nil

	case sandboxStatsMsg:
		m.sbStats[msg.id] = msg.stat
		return m, nil

	case eventMsg:
		m.addEvent(eventEntry(msg))
		return m, m.waitEvent()

	case frameMsg:
		animating := false

		if math.Abs(m.modalPos) > 0.5 || math.Abs(m.modalVelocity) > 0.5 {
			m.modalPos, m.modalVelocity = m.slideSpring.Update(m.modalPos, m.modalVelocity, 0)
			animating = true
		} else {
			m.modalPos = 0
			m.modalVelocity = 0
		}

		if m.mode == modeSpawning {
			m.loaderPos, m.loaderVelocity = m.loaderSpring.Update(m.loaderPos, m.loaderVelocity, m.loaderTarget)
			if math.Abs(m.loaderPos-m.loaderTarget) < 1.0 {
				if m.loaderTarget == 20.0 {
					m.loaderTarget = 0.0
				} else {
					m.loaderTarget = 20.0
				}
			}
			animating = true
		}

		if animating {
			return m, tickFrame()
		}
		return m, nil

	case sandboxesMsg:
		m.sandboxes = []sandboxData(msg)
		if m.cursor >= len(m.sandboxes) {
			m.cursor = max(0, len(m.sandboxes)-1)
		}

	case healthMsg:
		m.health = (*healthData)(msg)

	case providersMsg:
		m.providerList = []providerData(msg)

	case templatesMsg:
		m.templateList = []templateData(msg)
		if m.templateCursor >= len(m.templateList) {
			m.templateCursor = max(0, len(m.templateList)-1)
		}

	case spawnedMsg:
		sb := (*sandboxData)(msg)
		m.statusMsg = fmt.Sprintf("Spawned %s (%s)", sb.ID, sb.Image)
		m.addLog("SPAWN", fmt.Sprintf("%s image=%s", sb.ID, sb.Image))
		m.activeTab = tabSandboxes
		m.mode = modeNormal
		return m, m.fetchSandboxes()

	case destroyedMsg:
		m.statusMsg = fmt.Sprintf("Destroyed %s", string(msg))
		m.addLog("KILL", string(msg))
		m.mode = modeNormal
		return m, m.fetchSandboxes()

	case configPatchedMsg:
		m.statusMsg = string(msg)
		m.addLog("CONFIG", string(msg))
		return m, nil

	case execMsg:
		r := (*execResultData)(msg)
		m.lastOutput = ""
		if r.Stdout != "" {
			m.lastOutput += r.Stdout
		}
		if r.Stderr != "" {
			m.lastOutput += errorStyle.Render(r.Stderr)
		}
		m.lastOutput += dimStyle.Render(fmt.Sprintf("\n[exit %d, %s]", r.ExitCode, r.Duration))
		m.lastError = ""
		m.addLog("EXEC", fmt.Sprintf("exit=%d dur=%s", r.ExitCode, r.Duration))

	case fileWrittenMsg:
		m.statusMsg = "File written successfully"
		m.lastError = ""
		m.addLog("WRITE", "file written")

	case fileReadMsg:
		m.lastOutput = string(msg)
		m.lastError = ""
		m.addLog("READ", fmt.Sprintf("%d bytes", len(msg)))

	case templateCreatedMsg:
		m.statusMsg = "Template created"
		m.mode = modeNormal
		m.addLog("TEMPLATE", "created")
		return m, m.fetchTemplates()

	case templateDeletedMsg:
		m.statusMsg = fmt.Sprintf("Template %s deleted", string(msg))
		m.mode = modeNormal
		m.addLog("TEMPLATE", fmt.Sprintf("deleted %s", string(msg)))
		return m, m.fetchTemplates()

	case errMsg:
		m.lastError = msg.Error()
		m.statusMsg = ""
		m.addLog("ERROR", msg.Error())
	}

	// Update active text input
	if m.mode == modeInput || m.mode == modeExec || m.mode == modeSpawn || m.mode == modeCreateTemplate {
		var cmd tea.Cmd
		idx := m.activeInputIndex()
		if idx >= 0 && idx < len(m.inputs) {
			m.inputs[idx], cmd = m.inputs[idx].Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		return m, tea.Quit
	}

	if m.mode == modeNormal {
		// Global keys
		if msg.String() >= "1" && msg.String() <= "6" {
			newTab := tab(msg.String()[0] - '1')
			if newTab != m.activeTab {
				m.activeTab = newTab
				return m, nil
			}
			return m, nil
		}
		
		if key == "left" || key == "right" {
			newTab := m.activeTab
			if key == "left" && m.activeTab > 0 {
				newTab--
			} else if key == "right" && m.activeTab < tabConfig {
				newTab++
			}
			if newTab != m.activeTab {
				m.activeTab = newTab
				return m, nil
			}
			return m, nil
		}
	}

	// Sandbox Action Mode
	if m.mode == modeSandboxAction {
		switch key {
		case "esc":
			m.mode = modeNormal
			return m, tickFrame()
		case "e":
			m.mode = modeExec
			m.modalPos = 20.0
			m.modalVelocity = 0
			m.inputs[2].SetValue("")
			m.inputs[2].Focus()
			return m, tickFrame()
		case "f":
			m.mode = modeInput
			m.modalPos = 20.0
			m.modalVelocity = 0
			m.inputs[3].SetValue("")
			m.inputs[4].SetValue("")
			m.inputFocus = 0
			m.inputs[3].Focus()
			return m, tickFrame()
		case "d":
			if len(m.sandboxes) > 0 && m.cursor < len(m.sandboxes) {
				sb := m.sandboxes[m.cursor]
				m.mode = modeConfirm
				m.confirmMsg = fmt.Sprintf(" Destroy sandbox %s? (y/N) ", sb.ID)
				m.confirmFunc = func() tea.Cmd {
					return m.destroySandbox(sb.ID)
				}
				return m, tickFrame()
			}
		}
		return m, nil
	}

	// Confirm mode
	if m.mode == modeConfirm {
		switch key {
		case "y", "Y":
			m.mode = modeNormal
			if m.confirmFunc != nil {
				return m, m.confirmFunc()
			}
		case "n", "N", "esc":
			m.mode = modeNormal
			m.confirmMsg = ""
		}
		return m, nil
	}

	// Input modes
	if m.mode == modeInput || m.mode == modeExec || m.mode == modeSpawn || m.mode == modeCreateTemplate || m.mode == modeSpawnTemplate {
		switch key {
		case "esc":
			m.mode = modeNormal
			m.blurAllInputs()
			return m, nil
		case "tab":
			m.cycleInputFocus(1)
			return m, nil
		case "shift+tab":
			m.cycleInputFocus(-1)
			return m, nil
		case "enter":
			return m.submitInput()
		}
		idx := m.activeInputIndex()
		if idx >= 0 && idx < len(m.inputs) {
			var cmd tea.Cmd
			m.inputs[idx], cmd = m.inputs[idx].Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Normal mode - global keys
	switch key {
	case "q":
		return m, tea.Quit
	case "r":
		return m, tea.Batch(m.fetchSandboxes(), m.fetchHealth(), m.fetchProviders(), m.fetchTemplates())
	}

	// Tab-specific keys
	switch m.activeTab {
	case tabDashboard:
		return m.handleDashboardKey(key)
	case tabSandboxes:
		return m.handleSandboxKey(key)
	case tabTemplates:
		return m.handleTemplateKey(key)
	case tabConfig:
		return m.handleConfigKey(key)
	}

	return m, nil
}

func (m *Model) handleDashboardKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		if m.mode == modeSpawn {
			m.mode = modeSpawning
			m.loaderPos = 0
			m.loaderVelocity = 0
			m.loaderTarget = 20.0
			return m, tea.Batch(m.spawnSandbox(m.inputs[0].Value(), m.inputs[1].Value()), tickFrame())
		}
		if m.mode == modeExec {
            return m, nil
        }
    }
	return m, nil
}

func (m *Model) handleSandboxKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		if len(m.sandboxes) > 0 {
			m.cursor = min(m.cursor+1, len(m.sandboxes)-1)
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "d", "delete":
		if len(m.sandboxes) > 0 {
			sb := m.sandboxes[m.cursor]
			m.confirmMsg = fmt.Sprintf("Destroy sandbox %s? (y/n)", sb.ID)
			m.mode = modeConfirm
			m.confirmFunc = func() tea.Cmd {
				return m.destroySandbox(sb.ID)
			}
		}
	case "enter":
		if m.activeTab == tabSandboxes && len(m.sandboxes) > 0 {
			m.mode = modeSandboxAction
			m.modalPos = 20.0
			m.modalVelocity = 0
			return m, tickFrame()
		}
	case "s":
		if m.activeTab == tabSandboxes {
			m.mode = modeSpawn
			m.modalPos = -20.0 // slide down from top
			m.modalVelocity = 0
			m.inputs[0].SetValue("")
			m.inputs[1].SetValue("")
			m.inputFocus = 0
			m.inputs[0].Focus()
			return m, tickFrame()
		}
	case "e":
		if m.activeTab == tabSandboxes && len(m.sandboxes) > 0 {
			m.mode = modeExec
			m.modalPos = 20.0
			m.modalVelocity = 0
			m.inputs[2].SetValue("")
			m.inputs[2].Focus()
			return m, tickFrame()
		}
	case "f":
		if m.activeTab == tabSandboxes && len(m.sandboxes) > 0 {
			m.mode = modeInput
			m.modalPos = 20.0
			m.modalVelocity = 0
			m.inputs[3].SetValue("")
			m.inputs[4].SetValue("")
			m.inputFocus = 0
			m.inputs[3].Focus()
			return m, tickFrame()
		}
	}
	return m, nil
}

func (m *Model) handleTemplateKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		if len(m.templateList) > 0 {
			m.templateCursor = min(m.templateCursor+1, len(m.templateList)-1)
		}
	case "k", "up":
		if m.templateCursor > 0 {
			m.templateCursor--
		}
	case "n", "c":
		m.mode = modeCreateTemplate
		m.inputFocus = 0
		m.inputs[5].Focus()
	case "d", "delete":
		if len(m.templateList) > 0 {
			t := m.templateList[m.templateCursor]
			m.confirmMsg = fmt.Sprintf("Delete template %s? (y/n)", t.Name)
			m.mode = modeConfirm
			m.confirmFunc = func() tea.Cmd {
				return m.deleteTemplate(t.Name)
			}
		}
	case "s", "enter":
		if len(m.templateList) > 0 {
			t := m.templateList[m.templateCursor]
			return m, m.spawnFromTemplate(t.Name)
		}
	}
	return m, nil
}

func (m *Model) submitInput() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSpawn:
		image := m.inputs[0].Value()
		if image == "" {
			image = "alpine:latest"
		}
		ttl := m.inputs[1].Value()
		if ttl == "" {
			ttl = "30m"
		}
		m.inputs[0].SetValue("")
		m.inputs[1].SetValue("")
		m.blurAllInputs()
		return m, m.spawnSandbox(image, ttl)

	case modeExec:
		cmd := m.inputs[2].Value()
		if cmd == "" || len(m.sandboxes) == 0 {
			return m, nil
		}
		sb := m.sandboxes[m.cursor]
		m.inputs[2].SetValue("")
		return m, m.execCommand(sb.ID, cmd)

	case modeInput:
		// File operations
		path := m.inputs[3].Value()
		content := m.inputs[4].Value()
		if path == "" || len(m.sandboxes) == 0 {
			return m, nil
		}
		sb := m.sandboxes[m.cursor]
		if content != "" {
			m.inputs[3].SetValue("")
			m.inputs[4].SetValue("")
			m.blurAllInputs()
			return m, m.writeFileCmd(sb.ID, path, content)
		}
		m.inputs[3].SetValue("")
		m.blurAllInputs()
		return m, m.readFileCmd(sb.ID, path)

	case modeCreateTemplate:
		name := m.inputs[5].Value()
		image := m.inputs[6].Value()
		if name == "" || image == "" {
			m.lastError = "Template name and image are required"
			return m, nil
		}
		desc := m.inputs[7].Value()
		mem := m.inputs[8].Value()
		cpus := m.inputs[9].Value()
		ttl := m.inputs[10].Value()
		for i := 5; i <= 10; i++ {
			m.inputs[i].SetValue("")
		}
		m.blurAllInputs()
		return m, m.createTemplate(name, image, desc, mem, cpus, ttl)
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	if m.width < 10 || m.height < 5 {
		return "Too small"
	}

	if m.width < 90 || m.height < 25 {
		msg := fmt.Sprintf("Terminal too small: %dx%d\nPlease resize to at least 90x25 to use StacyVM TUI.", m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, stErr.Render(msg))
	}

	w := m.width
	h := m.height

	// Shared chrome: full-width telemetry ribbon, horizontal nav, status footer.
	ribbon := m.renderRibbon(w)
	nav := m.renderNav(w)
	footer := m.renderStatusFooter(w)

	// Body fills the space between nav and footer (1 blank breathing row each side).
	overhead := lipgloss.Height(ribbon) + lipgloss.Height(nav) + lipgloss.Height(footer) + 2
	bodyH := h - overhead
	if bodyH < 5 {
		bodyH = 5
	}

	var content string
	switch m.activeTab {
	case tabDashboard:
		content = m.viewDashboard(bodyH, w)
	case tabSandboxes:
		content = m.viewSandboxes(bodyH, w)
	case tabTemplates:
		content = m.viewTemplates(bodyH, w)
	case tabProviders:
		content = m.viewProviders(bodyH, w)
	case tabLogs:
		content = m.viewLogs(bodyH, w)
	case tabConfig:
		content = m.viewConfig(bodyH, w)
	}

	// Clamp height so the footer stays pinned; content is already full-width.
	body := lipgloss.NewStyle().Height(bodyH).MaxHeight(bodyH).Render(content)

	return lipgloss.JoinVertical(lipgloss.Left, ribbon, "", nav, body, footer)
}

// viewDashboard lives in dashboard.go (Mission Control restyle).

// ── Sandboxes View ───────────────────────────────────────

func (m Model) viewSandboxes(height, width int) string {
	var b strings.Builder

	if m.mode == modeSpawning {
		b.WriteString(boldStyle.Render("\n  Spawning Sandbox...\n\n"))
		pos := int(m.loaderPos)
		if pos < 0 {
			pos = 0
		}
		if pos > 20 {
			pos = 20
		}
		padLeft := strings.Repeat(" ", pos)
		padRight := strings.Repeat(" ", 20-pos)
		b.WriteString(fmt.Sprintf("  [%s%s%s]\n", padLeft, successStyle.Render("●"), padRight))
		b.WriteString(dimStyle.Render("\n  Please wait...\n"))
		
		return b.String()
	}

	if m.mode == modeSpawn {
		content := boldStyle.Render("\n  Spawn New Sandbox\n\n")
		content += fmt.Sprintf("  Image: %s\n", m.inputs[0].View())
		content += fmt.Sprintf("  TTL:   %s\n", m.inputs[1].View())
		content += dimStyle.Render("\n  Press Enter to spawn, Esc to cancel\n")
		
		if math.Abs(m.modalPos) > 0.5 {
			offset := int(m.modalPos)
			if offset < 0 {
				content = strings.Repeat("\n", -offset) + content
			} else {
				content = lipgloss.NewStyle().MarginTop(offset).Render(content)
			}
		}
		return content
	}

	if m.mode == modeExec {
		target := "(none)"
		if len(m.sandboxes) > 0 && m.cursor < len(m.sandboxes) {
			sb := m.sandboxes[m.cursor]
			target = fmt.Sprintf("%s (%s)", sb.ID, sb.Image)
		}
		content := boldStyle.Render(fmt.Sprintf("\n  Execute in: %s\n\n", target))
		content += fmt.Sprintf("  Command: %s\n", m.inputs[2].View())
		if m.lastOutput != "" {
			content += boldStyle.Render("\n  Output:\n")
			lines := strings.Split(m.lastOutput, "\n")
			maxLines := height - 8
			for i, line := range lines {
				if i >= maxLines {
					content += dimStyle.Render(fmt.Sprintf("  ... (%d more lines)", len(lines)-i)) + "\n"
					break
				}
				content += "  " + outputStyle.Render(line) + "\n"
			}
		}
		
		if math.Abs(m.modalPos) > 0.5 {
			offset := int(m.modalPos)
			if offset > 0 {
				content = lipgloss.NewStyle().MarginTop(offset).Render(content)
			}
		}
		return content
	}

	if m.mode == modeSandboxAction {
		sb := m.sandboxes[m.cursor]
		content := boldStyle.Render(fmt.Sprintf("\n  Sandbox Details: %s\n\n", sb.ID))
		content += fmt.Sprintf("  Image:    %s\n", sb.Image)
		content += fmt.Sprintf("  Created:  %s\n", sb.CreatedAt.Format("2006-01-02 15:04:05"))
		content += fmt.Sprintf("  Expires:  %s\n", formatTTL(sb.ExpiresAt))
		content += fmt.Sprintf("  Memory:   %d MB\n", sb.MemoryMB)
		content += fmt.Sprintf("  CPUs:     %d\n", sb.VCPUs)
		content += fmt.Sprintf("  State:    %s\n", sb.State)
		content += dimStyle.Render("\n  Press Esc to close\n")
		return content
	}

	if m.mode == modeInput {
		target := "(none)"
		if len(m.sandboxes) > 0 && m.cursor < len(m.sandboxes) {
			target = m.sandboxes[m.cursor].ID
		}
		content := boldStyle.Render(fmt.Sprintf("\n  Manage Files in: %s\n\n", target))
		content += fmt.Sprintf("  Path:    %s\n", m.inputs[3].View())
		content += fmt.Sprintf("  Content: %s\n", m.inputs[4].View())
		content += dimStyle.Render("\n  Leave content empty to READ file.\n")
		content += dimStyle.Render("  Provide content to WRITE file.\n")
		
		if m.lastOutput != "" && m.inputs[4].Value() == "" {
			content += boldStyle.Render("\n  File Content:\n")
			lines := strings.Split(m.lastOutput, "\n")
			maxLines := height - 12
			for i, line := range lines {
				if i >= maxLines {
					content += dimStyle.Render(fmt.Sprintf("  ... (%d more lines)", len(lines)-i)) + "\n"
					break
				}
				content += "  " + outputStyle.Render(line) + "\n"
			}
		}
		
		if math.Abs(m.modalPos) > 0.5 {
			offset := int(m.modalPos)
			if offset > 0 {
				content = lipgloss.NewStyle().MarginTop(offset).Render(content)
			}
		}
		return content
	}

	// Normal list view
	if len(m.sandboxes) == 0 {
		b.WriteString(dimStyle.Render("\n  No sandboxes running. Press [s] to spawn one.\n"))
		return b.String()
	}

	var rows [][]string
	imageWidth := max(10, width-55) // Calculate dynamic width for Image column
	for i, sb := range m.sandboxes {
		ttlStr := formatTTL(sb.ExpiresAt)
		rows = append(rows, []string{
			sb.ID, sb.State, sb.Provider,
			truncate(sb.Image, imageWidth),
			sb.CreatedAt.Format("15:04:05"),
			ttlStr,
		})

		if i >= height-8 {
			remaining := len(m.sandboxes) - i - 1
			if remaining > 0 {
				rows = append(rows, []string{"...", "...", "...", fmt.Sprintf("and %d more", remaining), "...", "..."})
			}
			break
		}
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))).
		BorderRow(false).
		BorderColumn(true).
		Headers("ID", "STATE", "PROVIDER", "IMAGE", "CREATED", "TTL").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			if row == m.cursor {
				return selectedRowStyle
			}
			return normalRowStyle
		})

	b.WriteString("\n  " + strings.ReplaceAll(t.Render(), "\n", "\n  ") + "\n")

	return b.String()
}

// ── Templates View ───────────────────────────────────────

func (m Model) viewTemplates(height, width int) string {
	var b strings.Builder

	if m.mode == modeCreateTemplate {
		b.WriteString(boldStyle.Render("\n  Create Template\n\n"))
		b.WriteString(fmt.Sprintf("  Name:        %s\n", m.inputs[5].View()))
		b.WriteString(fmt.Sprintf("  Image:       %s\n", m.inputs[6].View()))
		b.WriteString(fmt.Sprintf("  Description: %s\n", m.inputs[7].View()))
		b.WriteString(fmt.Sprintf("  Memory (MB): %s\n", m.inputs[8].View()))
		b.WriteString(fmt.Sprintf("  CPU Cores:   %s\n", m.inputs[9].View()))
		b.WriteString(fmt.Sprintf("  TTL (secs):  %s\n", m.inputs[10].View()))
		b.WriteString(dimStyle.Render("\n  Press Enter to create, Esc to cancel\n"))
		return b.String()
	}

	if len(m.templateList) == 0 {
		b.WriteString(dimStyle.Render("\n  No templates configured. Press [n] to create one.\n"))
		return b.String()
	}

	var rows [][]string
	availableWidth := max(20, width-55) // Calculate dynamic width for Name and Image columns
	nameWidth := max(10, availableWidth*2/5)
	imageWidth := max(15, availableWidth*3/5)

	for i, tmpl := range m.templateList {
		rows = append(rows, []string{
			truncate(tmpl.Name, nameWidth),
			truncate(tmpl.Image, imageWidth),
			fmt.Sprintf("%d", tmpl.MemoryMB),
			fmt.Sprintf("%d", tmpl.CPUCores),
			fmt.Sprintf("%d", tmpl.TTLSeconds),
			fmt.Sprintf("%d", tmpl.PoolSize),
		})

		if i >= height-10 {
			break
		}
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))).
		BorderRow(false).
		BorderColumn(true).
		Headers("NAME", "IMAGE", "MEM(MB)", "CPUS", "TTL(s)", "POOL").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			if row == m.templateCursor {
				return selectedRowStyle
			}
			return normalRowStyle
		})

	b.WriteString("\n  " + strings.ReplaceAll(t.Render(), "\n", "\n  ") + "\n")

	// Detail panel for selected template
	if m.templateCursor < len(m.templateList) {
		t := m.templateList[m.templateCursor]
		b.WriteString("\n")
		b.WriteString(boldStyle.Render(fmt.Sprintf("  Template: %s", t.Name)) + "\n")
		if t.Description != "" {
			out, err := glamour.Render(t.Description, "dark")
			if err == nil {
				b.WriteString(strings.ReplaceAll(out, "\n", "\n  "))
			} else {
				b.WriteString(dimStyle.Render(fmt.Sprintf("  %s", t.Description)) + "\n")
			}
		}
		b.WriteString(fmt.Sprintf("  Image: %s  |  Memory: %dMB  |  CPUs: %d  |  TTL: %ds\n",
			t.Image, t.MemoryMB, t.CPUCores, t.TTLSeconds))
	}

	return b.String()
}

// ── Providers View ───────────────────────────────────────

func (m Model) viewProviders(height, width int) string {
	var b strings.Builder

	if len(m.providerList) == 0 {
		b.WriteString(dimStyle.Render("No providers configured.\n"))
		return b.String()
	}

	var rows [][]string
	for _, p := range m.providerList {
		defaultStr := ""
		if p.IsDefault {
			defaultStr = successStyle.Render("default")
		}
		healthStr := successStyle.Render("healthy")
		if !p.Healthy {
			healthStr = errorStyle.Render("unhealthy")
		}

		rows = append(rows, []string{
			p.Name, defaultStr, healthStr,
		})
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))).
		BorderRow(false).
		BorderColumn(true).
		Headers("NAME", "DEFAULT", "HEALTHY").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return normalRowStyle
		})

	b.WriteString("\n  " + strings.ReplaceAll(t.Render(), "\n", "\n  ") + "\n")
	return b.String()
}

// ── Logs View ────────────────────────────────────────────

func (m Model) viewLogs(height, width int) string {
	var b strings.Builder
	b.WriteString(boldStyle.Render("\n  Activity Log\n\n"))

	if len(m.logs) == 0 {
		b.WriteString(dimStyle.Render("  No activity yet.\n"))
		return b.String()
	}

	start := len(m.logs) - (height - 4)
	if start < 0 {
		start = 0
	}
	for _, line := range m.logs[start:] {
		b.WriteString("  " + truncate(line, width-4) + "\n")
	}

	return b.String()
}

// ── Status Bar ───────────────────────────────────────────

func (m Model) viewStatusBar(width int) string {
	left := fmt.Sprintf(" %d sandbox(es) | %d template(s) | %d provider(s)",
		len(m.sandboxes), len(m.templateList), len(m.providerList))
	right := ""
	if m.statusMsg != "" {
		right = successStyle.Render(m.statusMsg)
	} else if m.lastError != "" {
		right = errorStyle.Render(truncate(m.lastError, 50))
	}
	padding := width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 1 {
		padding = 1
	}
	return left + strings.Repeat(" ", padding) + right
}

// ── Commands ─────────────────────────────────────────────

func tickFrame() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return frameMsg(t)
	})
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) fetchSandboxes() tea.Cmd {
	return func() tea.Msg {
		sbs, err := m.client.listSandboxes()
		if err != nil {
			return errMsg(err)
		}
		return sandboxesMsg(sbs)
	}
}

func (m Model) fetchHealth() tea.Cmd {
	return func() tea.Msg {
		h, err := m.client.health()
		if err != nil {
			return errMsg(err)
		}
		return healthMsg(h)
	}
}

func (m Model) fetchProviders() tea.Cmd {
	return func() tea.Msg {
		p, err := m.client.listProviders()
		if err != nil {
			return errMsg(err)
		}
		return providersMsg(p)
	}
}

func (m Model) fetchTemplates() tea.Cmd {
	return func() tea.Msg {
		t, err := m.client.listTemplates()
		if err != nil {
			return errMsg(err)
		}
		return templatesMsg(t)
	}
}

func (m Model) spawnSandbox(image, ttl string) tea.Cmd {
	return func() tea.Msg {
		sb, err := m.client.spawn(image, ttl)
		if err != nil {
			return errMsg(err)
		}
		return spawnedMsg(sb)
	}
}

func (m Model) spawnFromTemplate(name string) tea.Cmd {
	return func() tea.Msg {
		sb, err := m.client.spawnTemplate(name)
		if err != nil {
			return errMsg(err)
		}
		return spawnedMsg(sb)
	}
}

func (m Model) destroySandbox(id string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.destroy(id); err != nil {
			return errMsg(err)
		}
		return destroyedMsg(id)
	}
}

func (m Model) execCommand(id, cmd string) tea.Cmd {
	return func() tea.Msg {
		r, err := m.client.exec(id, cmd)
		if err != nil {
			return errMsg(err)
		}
		return execMsg(r)
	}
}

func (m Model) writeFileCmd(id, path, content string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.writeFile(id, path, content); err != nil {
			return errMsg(err)
		}
		return fileWrittenMsg{}
	}
}

func (m Model) readFileCmd(id, path string) tea.Cmd {
	return func() tea.Msg {
		data, err := m.client.readFile(id, path)
		if err != nil {
			return errMsg(err)
		}
		return fileReadMsg(data)
	}
}

func (m Model) createTemplate(name, image, desc, mem, cpus, ttl string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.createTemplate(name, image, desc, mem, cpus, ttl); err != nil {
			return errMsg(err)
		}
		return templateCreatedMsg{}
	}
}

func (m Model) deleteTemplate(name string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.deleteTemplate(name); err != nil {
			return errMsg(err)
		}
		return templateDeletedMsg(name)
	}
}

// ── Helpers ──────────────────────────────────────────────

func (m Model) activeInputIndex() int {
	switch m.mode {
	case modeSpawn:
		return m.inputFocus // 0=image, 1=ttl
	case modeExec:
		return 2
	case modeInput:
		return 3 + m.inputFocus // 3=path, 4=content
	case modeCreateTemplate:
		return 5 + m.inputFocus // 5-10
	}
	return -1
}

func (m *Model) blurAllInputs() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
}

func (m *Model) cycleInputFocus(dir int) {
	m.blurAllInputs()
	switch m.mode {
	case modeSpawn:
		m.inputFocus = (m.inputFocus + dir + 2) % 2
		m.inputs[m.inputFocus].Focus()
	case modeInput:
		m.inputFocus = (m.inputFocus + dir + 2) % 2
		m.inputs[3+m.inputFocus].Focus()
	case modeCreateTemplate:
		m.inputFocus = (m.inputFocus + dir + 6) % 6
		m.inputs[5+m.inputFocus].Focus()
	}
}

func (m *Model) addLog(kind, msg string) {
	ts := time.Now().Format("15:04:05")
	entry := dimStyle.Render(ts) + " " + boldStyle.Render(kind) + " " + msg
	m.logs = append(m.logs, entry)
	if len(m.logs) > 100 {
		m.logs = m.logs[1:]
	}
}

func stateIcon(state string) string {
	switch state {
	case "running":
		return successStyle.Render("●")
	case "creating":
		return warnStyle.Render("◐")
	case "idle":
		return dimStyle.Render("○")
	case "error":
		return errorStyle.Render("✗")
	default:
		return dimStyle.Render("·")
	}
}

func formatTTL(expiresAt time.Time) string {
	ttl := time.Until(expiresAt).Round(time.Second)
	if ttl < 0 {
		return errorStyle.Render("expired")
	}
	if ttl < 5*time.Minute {
		return warnStyle.Render(ttl.String())
	}
	return dimStyle.Render(ttl.String())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
