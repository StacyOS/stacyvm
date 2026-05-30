package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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

	// Spawn modal (quick form) + animated spawn sequence
	spawnProviderIdx int // 0 docker · 1 firecracker · 2 proot
	spawnTemplateIdx int // -1 none, else index into templateList
	spawn            spawnState

	// Exec
	lastExec    *execResultData
	lastExecCmd string
	execHistory []string
	execHistIdx int

	// Files browser
	files fileState

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

	ins := []textinput.Model{
		imageInput, ttlInput, cmdInput, filePathInput, fileContentInput,
		tmplNameInput, tmplImageInput, tmplDescInput, tmplMemInput, tmplCPUInput, tmplTTLInput,
	}
	for i := range ins {
		ins[i].Prompt = "" // we render our own prompts (e.g. "$ ")
	}

	return Model{
		client: client,
		inputs: ins,
		logs:   make([]string, 0, 100),
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

		spawnProviderIdx: 0,
		spawnTemplateIdx: -1,
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

	case spawnTickMsg:
		if m.spawn.active && !m.spawn.done {
			m.advanceSpawn(time.Time(msg))
			if !m.spawn.done {
				return m, spawnTick()
			}
		}
		return m, nil

	case filesListedMsg:
		m.applyFilesListed(msg)
		return m, nil

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
		if m.spawn.active {
			// Feed the real result into the animated sequence; let it finish.
			m.spawn.result = sb
			return m, m.fetchSandboxes()
		}
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
		m.lastExec = r
		m.lastError = ""
		m.addLog("EXEC", fmt.Sprintf("exit=%d dur=%s", r.ExitCode, r.Duration))

	case fileWrittenMsg:
		m.statusMsg = "File written"
		m.lastError = ""
		if m.mode == modeInput {
			m.files.write = false
			m.files.editorOn = false
		}
		m.addLog("WRITE", "file written")

	case fileReadMsg:
		if m.mode == modeInput {
			m.files.content = string(msg)
			m.files.write = false
		} else {
			m.lastOutput = string(msg)
		}
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
		if m.spawn.active && !m.spawn.done {
			m.spawn.err = msg.Error()
		}
		m.addLog("ERROR", msg.Error())
	}

	// Keep the active text input live (cursor blink, etc.) for the text forms.
	if m.mode == modeExec || m.mode == modeSpawn || m.mode == modeCreateTemplate {
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

	// Animated spawn sequence: dismiss / replay / background (nav away).
	if m.mode == modeSpawning {
		switch {
		case key == "esc" || key == "enter":
			m.mode = modeNormal
			if m.spawn.done {
				m.spawn.active = false
			}
			return m, m.fetchSandboxes()
		case key == "r" || key == "↻":
			return m, m.startSpawn(m.spawn.req)
		case key >= "1" && key <= "6":
			m.activeTab = tab(key[0] - '1')
			m.mode = modeNormal // background; ribbon keeps showing progress
			return m, nil
		}
		return m, nil
	}

	// Spawn modal (quick form) — its own field/segmented handling.
	if m.mode == modeSpawn {
		return m.handleSpawnModalKey(msg)
	}

	// Files browser — tree navigation + READ/WRITE editor.
	if m.mode == modeInput {
		return m.handleFilesKey(msg)
	}

	// Text-input forms: exec + create-template.
	if m.mode == modeExec || m.mode == modeCreateTemplate {
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
		case "up":
			if m.mode == modeExec {
				m.execHistoryRecall(-1)
				return m, nil
			}
		case "down":
			if m.mode == modeExec {
				m.execHistoryRecall(1)
				return m, nil
			}
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

// handleDashboardKey: the dashboard's ACTIVE SANDBOXES table shares actions
// with the Sandboxes screen.
func (m *Model) handleDashboardKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		if len(m.sandboxes) > 0 {
			m.cursor = min(m.cursor+1, len(m.sandboxes)-1)
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "s":
		return m, m.openSpawnModal()
	case "e":
		return m, m.openExec()
	case "f":
		return m, m.openFiles()
	case "d":
		m.confirmKill()
	case "enter":
		// open workspace — built in Batch 2; no-op for now.
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
		m.confirmKill()
	case "s":
		return m, m.openSpawnModal()
	case "e":
		return m, m.openExec()
	case "f":
		return m, m.openFiles()
	case "l":
		m.activeTab = tabLogs
	case "enter":
		// inspect drawer is always shown; ↵ opens Workspace (Batch 2).
	}
	return m, nil
}

// ── shared sandbox actions ──────────────────────────────────────────────────

func (m *Model) openSpawnModal() tea.Cmd {
	m.mode = modeSpawn
	m.inputFocus = 0
	m.spawnTemplateIdx = -1
	m.spawnProviderIdx = 0
	m.inputs[0].SetValue("")
	m.inputs[1].SetValue("")
	m.blurAllInputs()
	m.inputs[0].Focus()
	return nil
}

func (m *Model) openExec() tea.Cmd {
	if len(m.sandboxes) == 0 {
		return nil
	}
	m.mode = modeExec
	m.inputs[2].SetValue("")
	m.execHistIdx = len(m.execHistory)
	m.inputs[2].Focus()
	return nil
}

func (m *Model) openFiles() tea.Cmd {
	if len(m.sandboxes) == 0 || m.cursor >= len(m.sandboxes) {
		return nil
	}
	return m.startFiles(m.sandboxes[m.cursor].ID)
}

func (m *Model) confirmKill() {
	if len(m.sandboxes) == 0 || m.cursor >= len(m.sandboxes) {
		return
	}
	sb := m.sandboxes[m.cursor]
	m.confirmMsg = fmt.Sprintf("Destroy sandbox %s? (y/N)", sb.ID)
	m.mode = modeConfirm
	m.confirmFunc = func() tea.Cmd { return m.destroySandbox(sb.ID) }
}

// execHistoryRecall steps through previously run commands (↑/↓).
func (m *Model) execHistoryRecall(dir int) {
	if len(m.execHistory) == 0 {
		return
	}
	m.execHistIdx += dir
	if m.execHistIdx < 0 {
		m.execHistIdx = 0
	}
	if m.execHistIdx >= len(m.execHistory) {
		m.execHistIdx = len(m.execHistory)
		m.inputs[2].SetValue("")
		return
	}
	m.inputs[2].SetValue(m.execHistory[m.execHistIdx])
	m.inputs[2].CursorEnd()
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
			m.activeTab = tabSandboxes
			return m, m.startSpawn(spawnReq{template: t.Name, image: t.Image})
		}
	}
	return m, nil
}

func (m *Model) submitInput() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeExec:
		cmd := m.inputs[2].Value()
		if cmd == "" || len(m.sandboxes) == 0 {
			return m, nil
		}
		sb := m.sandboxes[m.cursor]
		m.lastExecCmd = cmd
		m.execHistory = append(m.execHistory, cmd)
		m.execHistIdx = len(m.execHistory)
		m.inputs[2].SetValue("")
		return m, m.execCommand(sb.ID, cmd)

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

func (m Model) spawnSandbox(image, ttl, provider string) tea.Cmd {
	return func() tea.Msg {
		sb, err := m.client.spawn(image, ttl, provider)
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
		switch m.inputFocus { // 0=image, 1=template, 2=ttl, 3=provider
		case 0:
			return 0
		case 2:
			return 1
		}
		return -1
	case modeExec:
		return 2
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
