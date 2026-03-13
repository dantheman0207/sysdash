package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/daniel/sysdash/internal/domain"
)

type DiskBrowserState struct {
	Path     string
	Node     domain.DiskNode
	Selected int
	Loading  bool
}

func RenderDashboard(styles Styles, width int, focus int, data domain.DashboardData, disk DiskBrowserState) string {
	leftWidth := max(46, width/2-4)
	rightWidth := max(46, width-leftWidth-8)

	leftTop := lipgloss.JoinHorizontal(lipgloss.Top,
		panel(styles, 0 == focus, leftWidth/2, "CPU", renderCPUWidget(styles, data)),
		panel(styles, 1 == focus, leftWidth-leftWidth/2, "Memory", renderMemoryWidget(styles, data)),
	)
	leftBottom := lipgloss.JoinHorizontal(lipgloss.Top,
		panel(styles, 2 == focus, leftWidth/2, "Disk I/O", renderThroughput(data.DiskIO, "Read", "Write")),
		panel(styles, 3 == focus, leftWidth-leftWidth/2, "Network", renderThroughput(data.Network, "RX", "TX")),
	)

	right := panel(styles, 4 == focus, rightWidth, "Disk Browser", renderDiskPreview(styles, disk))

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Left, leftTop, leftBottom),
		right,
	)

	status := "tab focus  shift+tab reverse  arrows move panes  enter drill  esc back  home/end top-bottom  f5 refresh  ctrl+c quit"
	if data.LastError != "" {
		status += "  error: " + data.LastError
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, "", styles.StatusLine.Render(status))
}

func RenderProcessTable(styles Styles, width, height int, title string, processes []domain.ProcessInfo, selected int, sortBy, query string) string {
	var lines []string
	nameWidth, stateWidth, cmdWidth, ruleWidth := processTableWidths(width)
	start, end := windowBounds(len(processes), selected, visibleRows(height, 7))
	lines = append(lines, styles.Title.Render(title))
	if query != "" {
		lines = append(lines, styles.Warn.Render("Filter: "+query))
	} else {
		lines = append(lines, styles.Dim.Render("Filter: type to search, backspace deletes"))
	}
	lines = append(lines, fmt.Sprintf("%-8s %-*s %-9s %-10s %-*s %s", "PID", nameWidth, "NAME", sortBy, "RSS", stateWidth, "STATE", "CMD"))
	lines = append(lines, styles.Dim.Render(strings.Repeat("─", ruleWidth)))
	for i := start; i < end; i++ {
		p := processes[i]
		line := fmt.Sprintf(
			"%-8d %-*s %-9s %-10s %-*s %s",
			p.PID,
			nameWidth,
			truncate(p.Name, nameWidth),
			processSortValue(p, sortBy),
			byteString(p.RSS),
			stateWidth,
			truncate(p.Status, stateWidth),
			truncate(p.Cmdline, cmdWidth),
		)
		if i == selected {
			line = styles.Selected.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, styles.Dim.Render(fmt.Sprintf("rows %d-%d of %d", start+1, max(start+1, end), len(processes))))
	lines = append(lines, styles.Dim.Render("type search  backspace delete  enter actions  esc dashboard"))
	return strings.Join(lines, "\n")
}

func RenderConnections(styles Styles, width, height int, conns []domain.ConnectionInfo, selected int) string {
	var lines []string
	cmdWidth, localWidth, remoteWidth, ruleWidth := connectionTableWidths(width)
	start, end := windowBounds(len(conns), selected, visibleRows(height, 7))
	lines = append(lines, styles.Title.Render("Network Connections"))
	lines = append(lines, fmt.Sprintf("%-*s %-8s %-7s %-*s %-*s %s", cmdWidth, "COMMAND", "PID", "PROTO", localWidth, "LOCAL", remoteWidth, "REMOTE", "STATE"))
	lines = append(lines, styles.Dim.Render(strings.Repeat("─", ruleWidth)))
	for i := start; i < end; i++ {
		c := conns[i]
		line := fmt.Sprintf(
			"%-*s %-8d %-7s %-*s %-*s %s",
			cmdWidth,
			truncate(c.Command, cmdWidth),
			c.PID,
			c.Proto,
			localWidth,
			truncate(c.Local, localWidth),
			remoteWidth,
			truncate(c.Remote, remoteWidth),
			c.State,
		)
		if i == selected {
			line = styles.Selected.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, styles.Dim.Render(fmt.Sprintf("rows %d-%d of %d", start+1, max(start+1, end), len(conns))))
	lines = append(lines, styles.Dim.Render("Shows process-owned connections from lsof on macOS"))
	return strings.Join(lines, "\n")
}

func RenderDiskBrowser(styles Styles, width, height int, state DiskBrowserState) string {
	var lines []string
	nameWidth, barWidth, ruleWidth := diskBrowserWidths(width)
	start, end := windowBounds(len(state.Node.Children), state.Selected, visibleRows(height, 7))
	title := "Disk Browser: " + state.Path
	if state.Loading {
		title += " [scanning]"
	}
	lines = append(lines, styles.Title.Render(title))
	lines = append(lines, fmt.Sprintf("%-*s %10s %8s %s", nameWidth, "NAME", "SIZE", "PCT", "BAR"))
	lines = append(lines, styles.Dim.Render(strings.Repeat("─", ruleWidth)))
	for i := start; i < end; i++ {
		child := state.Node.Children[i]
		bar := pctBar(styles, barWidth, child.Percent)
		prefix := " "
		if child.IsDir {
			prefix = "/"
		}
		label := prefix + filepath.Base(child.Path)
		if filepath.Base(child.Path) == "." || filepath.Base(child.Path) == "/" {
			label = child.Path
		}
		if child.Incomplete {
			label += " *"
		}
		line := fmt.Sprintf("%-*s %10s %7.1f%% %s", nameWidth, truncate(label, nameWidth), byteString(child.Size), child.Percent, bar)
		if i == state.Selected {
			line = styles.Selected.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, styles.Dim.Render(fmt.Sprintf("rows %d-%d of %d", start+1, max(start+1, end), len(state.Node.Children))))
	lines = append(lines, styles.Dim.Render("enter descend  backspace up  * partial size due to permissions/device boundary"))
	return strings.Join(lines, "\n")
}

func panel(styles Styles, focused bool, width int, title, body string) string {
	style := styles.Panel.Width(width)
	if focused {
		style = styles.Focused.Width(width)
	}
	return style.Render(styles.Title.Render(title) + "\n" + body)
}

func renderCPUWidget(styles Styles, data domain.DashboardData) string {
	lines := []string{
		fmt.Sprintf("Usage   %5.1f%% %s", data.CPUPercent, pctBar(styles, 24, data.CPUPercent)),
		"",
		styles.Dim.Render("Top CPU"),
	}
	for i, p := range data.TopCPU {
		if i >= 5 {
			break
		}
		lines = append(lines, fmt.Sprintf("%6d  %-18s  %6.1f%%", p.PID, truncate(p.Name, 18), p.CPU))
	}
	return strings.Join(lines, "\n")
}

func renderMemoryWidget(styles Styles, data domain.DashboardData) string {
	lines := []string{
		fmt.Sprintf("Used    %s / %s", byteString(data.Memory.Used), byteString(data.Memory.Total)),
		fmt.Sprintf("Usage   %5.1f%% %s", data.Memory.UsedPercent, pctBar(styles, 24, data.Memory.UsedPercent)),
		"",
		styles.Dim.Render("Top RSS"),
	}
	for i, p := range data.TopMemory {
		if i >= 5 {
			break
		}
		lines = append(lines, fmt.Sprintf("%6d  %-18s  %8s", p.PID, truncate(p.Name, 18), byteString(p.RSS)))
	}
	return strings.Join(lines, "\n")
}

func renderThroughput(tp domain.Throughput, readLabel, writeLabel string) string {
	return strings.Join([]string{
		fmt.Sprintf("%-6s %12s/s", readLabel, byteString(tp.ReadBytesPerSec)),
		fmt.Sprintf("%-6s %12s/s", writeLabel, byteString(tp.WriteBytesPerSec)),
	}, "\n")
}

func renderDiskPreview(styles Styles, disk DiskBrowserState) string {
	if disk.Loading && len(disk.Node.Children) == 0 {
		return "Scanning..."
	}
	lines := []string{styles.Dim.Render(disk.Path)}
	for i, child := range disk.Node.Children {
		if i >= 8 {
			break
		}
		lines = append(lines, fmt.Sprintf("%-24s %8s %5.1f%%", truncate(filepath.Base(child.Path), 24), byteString(child.Size), child.Percent))
	}
	return strings.Join(lines, "\n")
}

func pctBar(styles Styles, width int, pct float64) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	full := int((pct / 100) * float64(width))
	return styles.BarFull.Render(strings.Repeat("█", full)) + styles.BarEmpty.Render(strings.Repeat("░", width-full))
}

func processSortValue(p domain.ProcessInfo, sortBy string) string {
	if sortBy == "CPU%" {
		return fmt.Sprintf("%5.1f%%", p.CPU)
	}
	return byteString(p.RSS)
}

func byteString(v uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f := float64(v)
	unit := 0
	for f >= 1024 && unit < len(units)-1 {
		f /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d%s", v, units[unit])
	}
	return fmt.Sprintf("%.1f%s", f, units[unit])
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func visibleRows(height, reserved int) int {
	rows := height - reserved
	if rows < 5 {
		return 5
	}
	return rows
}

func windowBounds(total, selected, visible int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if visible >= total {
		return 0, total
	}
	start := selected - visible/2
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > total {
		end = total
		start = end - visible
	}
	return start, end
}

func processTableWidths(width int) (nameWidth, stateWidth, cmdWidth, ruleWidth int) {
	usable := max(90, width-10)
	nameWidth = max(18, min(28, usable/5))
	stateWidth = 9
	fixed := 8 + 1 + nameWidth + 1 + 9 + 1 + 10 + 1 + stateWidth + 1
	cmdWidth = max(24, usable-fixed)
	ruleWidth = fixed + cmdWidth
	return
}

func connectionTableWidths(width int) (cmdWidth, localWidth, remoteWidth, ruleWidth int) {
	usable := max(100, width-10)
	cmdWidth = max(14, min(20, usable/7))
	stateWidth := 12
	fixed := cmdWidth + 1 + 8 + 1 + 7 + 1 + stateWidth + 2
	remaining := usable - fixed
	localWidth = max(20, remaining/2)
	remoteWidth = max(20, remaining-localWidth)
	ruleWidth = fixed + localWidth + 1 + remoteWidth
	return
}

func diskBrowserWidths(width int) (nameWidth, barWidth, ruleWidth int) {
	usable := max(88, width-10)
	nameWidth = max(24, min(48, usable/2))
	fixed := nameWidth + 1 + 10 + 1 + 8 + 1
	barWidth = max(18, usable-fixed)
	ruleWidth = fixed + barWidth
	return
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
