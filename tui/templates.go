package tui

// templates.go — screen 9, "Templates" (Build 1 A): a TEMPLATES table on the
// left and a detail panel on the right, plus the create-template modal.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewTemplates(height, width int) string {
	if m.mode == modeCreateTemplate {
		return m.renderCreateTemplate(height, width)
	}

	leftW := (width - 1) * 13 / 23
	rightW := width - 1 - leftW
	left := m.templatesTable(leftW)
	right := m.templateDetail(rightW)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (m Model) templatesTable(width int) string {
	inner := width - 4
	wName, wMem, wCPU, wPool := 16, 6, 5, 5
	wImage := inner - wName - wMem - wCPU - wPool - 4
	if wImage < 8 {
		wImage = 8
	}
	if wImage > 22 {
		wImage = 22
	}

	header := stLabel.Render(
		padRight("NAME", wName) + " " + padRight("IMAGE", wImage) + " " +
			padRight("MEM", wMem) + " " + padRight("CPU", wCPU) + " " + padRight("POOL", wPool))
	rows := []string{header}
	if len(m.templateList) == 0 {
		rows = append(rows, stDim.Render("no templates — press n to create"))
	}
	for i, t := range m.templateList {
		poolStyle := stDim
		if t.PoolSize > 0 {
			poolStyle = stOK
		}
		name := stBold.Render(t.Name)
		if i == m.templateCursor {
			name = stHiB.Render(t.Name)
		}
		row := padRight(name, wName) + " " +
			padRight(stDim.Render(truncate(t.Image, wImage)), wImage) + " " +
			padRight(stDim.Render(itoa(t.MemoryMB)), wMem) + " " +
			padRight(stDim.Render(itoa(t.CPUCores)), wCPU) + " " +
			padRight(poolStyle.Render(itoa(t.PoolSize)), wPool)
		if i == m.templateCursor {
			rows = append(rows, selectedRow(row, inner))
		} else {
			rows = append(rows, row)
		}
	}
	return panel("TEMPLATES", fmt.Sprintf("%d total", len(m.templateList)), strings.Join(rows, "\n"), width, false)
}

func (m Model) templateDetail(width int) string {
	if len(m.templateList) == 0 || m.templateCursor >= len(m.templateList) {
		return panel("DETAIL", "", stFaint.Render("no template selected"), width, true)
	}
	t := m.templateList[m.templateCursor]
	inner := width - 4

	var lines []string
	lines = append(lines, stHiB.Render(t.Name))
	if t.Description != "" {
		desc := t.Description
		if out, err := glamour.Render(t.Description, "dark"); err == nil {
			desc = strings.TrimSpace(out)
		}
		for _, ln := range strings.Split(desc, "\n") {
			lines = append(lines, stDim.Render(truncate(ln, inner)))
		}
	}
	lines = append(lines, "")
	lines = append(lines, stDim.Render(fmt.Sprintf("mem %dMB · cpu %d · ttl %ds", t.MemoryMB, t.CPUCores, t.TTLSeconds)))
	lines = append(lines, stDim.Render("image "+truncate(t.Image, inner-6)))
	lines = append(lines, "")
	lines = append(lines, keyHints([]hint{{"s", "spawn"}, {"n", "new"}, {"d", "delete"}}))

	return panel(glyphInspect+" "+t.Name, "TEMPLATE", strings.Join(lines, "\n"), width, true)
}

func (m Model) renderCreateTemplate(height, width int) string {
	inner := 58
	focus := m.inputFocus
	labels := []string{"name", "image", "description", "memory", "cpu", "ttl"}
	field := func(idx int, k, v string) string {
		bar, kst := "  ", stDim
		if idx == focus {
			bar, kst = stHi.Render("▌ "), stHi
		}
		return bar + kst.Render(padRight(k, 12)) + v
	}
	var lines []string
	for i, lab := range labels {
		lines = append(lines, field(i, lab, m.inputs[5+i].View()))
	}
	lines = append(lines, "")
	lines = append(lines, keyHints([]hint{{glyphTab, "next field"}, {glyphEnter, "create"}, {"esc", "cancel"}}))
	box := panel(glyphPaneSpawn+" CREATE TEMPLATE", "", strings.Join(lines, "\n"), inner, true)
	return lipgloss.Place(width, height-1, lipgloss.Center, lipgloss.Center, box)
}
