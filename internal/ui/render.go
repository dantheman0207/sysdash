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
	Progress domain.DiskScanProgress
	Cleanup  domain.CleanupProgress
}

type DashboardHistory struct {
	CPU       []float64
	Memory    []float64
	DiskRead  []float64
	DiskWrite []float64
	NetRX     []float64
	NetTX     []float64
}

func RenderDashboard(styles Styles, width int, focus int, data domain.DashboardData, disk DiskBrowserState, history DashboardHistory) string {
	fullWidth := max(60, width-6)
	halfWidth := max(28, (fullWidth-1)/2)
	top := panel(styles, 4 == focus, fullWidth, "Disk Space", withWidgetHint(styles, 4 == focus, renderDiskPreview(styles, fullWidth, data.DiskUsage, disk), "enter open  tab next  arrows move"))
	middle := lipgloss.JoinHorizontal(lipgloss.Top,
		panel(styles, 0 == focus, halfWidth, "CPU", withWidgetHint(styles, 0 == focus, renderCPUWidget(styles, halfWidth, data, history.CPU), "enter processes  tab next  arrows move")),
		panel(styles, 1 == focus, fullWidth-halfWidth, "Memory", withWidgetHint(styles, 1 == focus, renderMemoryWidget(styles, fullWidth-halfWidth, data, history.Memory), "enter processes  shift+tab back  arrows move")),
	)
	bottom := lipgloss.JoinHorizontal(lipgloss.Top,
		panel(styles, 2 == focus, halfWidth, "Disk I/O", withWidgetHint(styles, 2 == focus, renderThroughput(styles, halfWidth, data.DiskIO, "Read", "Write", history.DiskRead, history.DiskWrite), "tab next  arrows move")),
		panel(styles, 3 == focus, fullWidth-halfWidth, "Network", withWidgetHint(styles, 3 == focus, renderThroughput(styles, fullWidth-halfWidth, data.Network, "RX", "TX", history.NetRX, history.NetTX), "enter connections  tab next  arrows move")),
	)
	body := lipgloss.JoinVertical(lipgloss.Left, top, middle, bottom)

	status := "tab focus  shift+tab reverse  arrows move panes  enter drill  esc back  home/end top-bottom  f5 refresh  ctrl+c quit"
	if data.LastError != "" {
		status += "  error: " + data.LastError
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, "", styles.StatusLine.Render(status))
}

func RenderProcessTable(styles Styles, width, height int, title string, processes []domain.ProcessInfo, selected int, query, sortLabel string) string {
	var lines []string
	nameWidth, stateWidth, cmdWidth, ruleWidth := processTableWidths(width)
	start, end := windowBounds(len(processes), selected, visibleRows(height, 7))
	lines = append(lines, styles.Title.Render(title))
	lines = append(lines, styles.Dim.Render("Sort: "+sortLabel+"  tab/shift+tab change"))
	if query != "" {
		lines = append(lines, styles.Warn.Render("Filter: "+query))
	} else {
		lines = append(lines, styles.Dim.Render("Filter: type to search, backspace deletes"))
	}
	lines = append(lines, fmt.Sprintf("%-8s %-*s %-9s %-10s %-*s %s", "PID", nameWidth, "NAME", "CPU%", "RSS", stateWidth, "STATE", "CMD"))
	lines = append(lines, styles.Dim.Render(strings.Repeat("─", ruleWidth)))
	for i := start; i < end; i++ {
		p := processes[i]
		line := fmt.Sprintf(
			"%-8d %-*s %-9s %-10s %-*s %s",
			p.PID,
			nameWidth,
			truncate(p.Name, nameWidth),
			cpuString(p.CPU),
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
	lines = append(lines, styles.Dim.Render("type search  backspace delete  enter details  esc dashboard"))
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

func RenderDiskBrowser(styles Styles, width, height int, usage domain.DiskUsage, state DiskBrowserState) string {
	var lines []string
	nameWidth, barWidth, ruleWidth := diskBrowserWidths(width)
	fullBarWidth := contentBarWidth(width, 0)
	start, end := windowBounds(len(state.Node.Children), state.Selected, visibleRows(height, 7))
	title := "Disk Space Browser: " + state.Path
	if state.Loading {
		title += " [scanning]"
	}
	lines = append(lines, styles.Title.Render(title))
	lines = append(lines, styles.Dim.Render(fmt.Sprintf("Filesystem  %s used of %s  (%4.1f%%)", byteString(usage.Used), byteString(usage.Total), usage.UsedPercent)))
	lines = append(lines, styles.Dim.Render(pctBar(styles, fullBarWidth, usage.UsedPercent)))
	if state.Node.Size > 0 {
		lines = append(lines, fmt.Sprintf("Scanned path total  %s", byteString(state.Node.Size)))
	} else if state.Loading {
		lines = append(lines, "Scanned path total  calculating...")
	}
	if state.Cleanup.Running || state.Cleanup.Done {
		lines = append(lines, "")
		lines = append(lines, renderCleanupProgress(styles, width, state.Cleanup))
	}
	if state.Loading && state.Progress.Total > 0 {
		progressPct := (float64(state.Progress.Completed) / float64(state.Progress.Total)) * 100
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Scan progress  %d/%d  %s", state.Progress.Completed, state.Progress.Total, truncate(state.Progress.Current, max(20, width-36))))
		lines = append(lines, pctBar(styles, fullBarWidth, progressPct))
	}
	lines = append(lines, "")
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
	lines = append(lines, styles.Dim.Render("enter/right descend  backspace/left up  f5 rescan current folder  f6 cleanup  percentages are of scanned path total"))
	return strings.Join(lines, "\n")
}

func panel(styles Styles, focused bool, width int, title, body string) string {
	style := styles.Panel.Width(width)
	if focused {
		style = styles.Focused.Width(width)
	}
	return style.Render(styles.Title.Render(title) + "\n" + body)
}

func withWidgetHint(styles Styles, focused bool, body, hint string) string {
	if !focused {
		return body
	}
	return body + "\n\n" + styles.Dim.Render(hint)
}

func renderCPUWidget(styles Styles, width int, data domain.DashboardData, history []float64) string {
	barWidth := contentBarWidth(width, len("Usage   000.0% "))
	nameWidth := max(12, width-24)
	lines := []string{
		fmt.Sprintf("Usage   %5.1f%% %s", data.CPUPercent, pctBar(styles, barWidth, data.CPUPercent)),
		renderTrendLine(styles, width, "Trend", history, 100, 70, 85, func(v float64) string { return fmt.Sprintf("%4.0f", v) }),
		"",
		styles.Dim.Render("Top CPU"),
	}
	for i, p := range data.TopCPU {
		if i >= 5 {
			break
		}
		lines = append(lines, fmt.Sprintf("%6d  %-*s  %6.1f%%", p.PID, nameWidth, truncate(p.Name, nameWidth), p.CPU))
	}
	return strings.Join(lines, "\n")
}

func renderMemoryWidget(styles Styles, width int, data domain.DashboardData, history []float64) string {
	barWidth := contentBarWidth(width, len("Usage   000.0% "))
	nameWidth := max(12, width-25)
	lines := []string{
		fmt.Sprintf("Used    %s / %s", byteString(data.Memory.Used), byteString(data.Memory.Total)),
		fmt.Sprintf("Usage   %5.1f%% %s", data.Memory.UsedPercent, pctBar(styles, barWidth, data.Memory.UsedPercent)),
		renderTrendLine(styles, width, "Trend", history, 100, 70, 85, func(v float64) string { return fmt.Sprintf("%4.0f", v) }),
		"",
		styles.Dim.Render("Top RSS"),
	}
	for i, p := range data.TopMemory {
		if i >= 5 {
			break
		}
		lines = append(lines, fmt.Sprintf("%6d  %-*s  %8s", p.PID, nameWidth, truncate(p.Name, nameWidth), byteString(p.RSS)))
	}
	return strings.Join(lines, "\n")
}

func renderThroughput(styles Styles, width int, tp domain.Throughput, readLabel, writeLabel string, readHistory, writeHistory []float64) string {
	readP95 := percentile(readHistory, 0.95)
	writeP95 := percentile(writeHistory, 0.95)
	readStyle := styleForThreshold(styles, thresholdRatio(latestValue(readHistory), readP95), 1.0, 1.2)
	writeStyle := styleForThreshold(styles, thresholdRatio(latestValue(writeHistory), writeP95), 1.0, 1.2)
	return strings.Join([]string{
		readStyle.Render(fmt.Sprintf("%-6s %12s/s", readLabel, byteString(tp.ReadBytesPerSec))) + "  " + styles.Dim.Render("p95 "+shortByteString(uint64(readP95))+"/s"),
		renderTrendLine(styles, width, "Trend", readHistory, 0, 0.60, 0.85, func(v float64) string { return shortByteString(uint64(v)) }),
		writeStyle.Render(fmt.Sprintf("%-6s %12s/s", writeLabel, byteString(tp.WriteBytesPerSec))) + "  " + styles.Dim.Render("p95 "+shortByteString(uint64(writeP95))+"/s"),
		renderTrendLine(styles, width, "Trend", writeHistory, 0, 0.60, 0.85, func(v float64) string { return shortByteString(uint64(v)) }),
	}, "\n")
}

func renderDiskPreview(styles Styles, width int, usage domain.DiskUsage, disk DiskBrowserState) string {
	barWidth := contentBarWidth(width, 0)
	if disk.Loading && len(disk.Node.Children) == 0 {
		return strings.Join([]string{
			fmt.Sprintf("%s used of %s", byteString(usage.Used), byteString(usage.Total)),
			fmt.Sprintf("Usage    %5.1f%%", usage.UsedPercent),
			pctBar(styles, barWidth, usage.UsedPercent),
			"",
			"Scanning...",
		}, "\n")
	}
	lines := []string{
		fmt.Sprintf("%s used of %s", byteString(usage.Used), byteString(usage.Total)),
		fmt.Sprintf("Usage    %5.1f%%", usage.UsedPercent),
		pctBar(styles, barWidth, usage.UsedPercent),
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

func cpuString(v float64) string {
	return fmt.Sprintf("%5.1f%%", v)
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

func ByteString(v uint64) string {
	return byteString(v)
}

func renderCleanupProgress(styles Styles, width int, progress domain.CleanupProgress) string {
	lines := []string{
		styles.Warn.Render("Cleanup"),
	}
	total := max(1, progress.Total)
	pct := (float64(progress.Completed) / float64(total)) * 100
	status := fmt.Sprintf("%d/%d", progress.Completed, progress.Total)
	if progress.CurrentStep != "" {
		status += "  " + progress.CurrentStep
	}
	if progress.Done {
		status = fmt.Sprintf("saved %s", byteString(progress.SavedBytes))
		if progress.AfterFree > 0 {
			status += fmt.Sprintf("  free %s -> %s", byteString(progress.BeforeFree), byteString(progress.AfterFree))
		}
	}
	lines = append(lines, status)
	lines = append(lines, pctBar(styles, contentBarWidth(width, 0), pct))
	for _, result := range progress.Results {
		label := "ok"
		if !result.OK {
			label = "failed"
		}
		line := fmt.Sprintf("%-26s %s", truncate(result.Name, 26), label)
		if !result.OK && result.Detail != "" {
			line += "  " + truncate(result.Detail, max(20, width-42))
		}
		lines = append(lines, line)
	}
	if len(progress.History) > 0 {
		lines = append(lines, "")
		lines = append(lines, styles.Dim.Render("Recent cleanup runs"))
		for i, run := range progress.History {
			if i >= 3 {
				break
			}
			line := fmt.Sprintf("%s  saved %s", run.FinishedAt.Format("15:04:05"), byteString(run.SavedBytes))
			if run.Failures > 0 {
				line += fmt.Sprintf("  %d failures", run.Failures)
			}
			lines = append(lines, styles.Dim.Render(line))
		}
	}
	return strings.Join(lines, "\n")
}

func contentBarWidth(containerWidth, prefixWidth int) int {
	usable := containerWidth - 8 - prefixWidth
	return max(10, usable)
}

func renderTrendLine(styles Styles, width int, label string, history []float64, maxValue, warnThreshold, highThreshold float64, format func(float64) string) string {
	sparkWidth := max(8, contentBarWidth(width, len(label)+20))
	if len(history) == 0 {
		return fmt.Sprintf("%-6s %s", label, styles.Dim.Render(strings.Repeat("·", sparkWidth)))
	}
	values := resample(history, sparkWidth)
	minV, maxV := rangeMinMax(history)
	current := history[len(history)-1]
	arrow := trendArrow(history)
	colorLevel := current
	scaleMax := maxValue
	if scaleMax <= 0 {
		scaleMax = maxV
	}
	if scaleMax > 0 && maxValue <= 0 {
		colorLevel = current / scaleMax
	}
	spark := sparklineBlocks(values, sparkWidth, scaleMax)
	spark = styleForThreshold(styles, colorLevel, warnThreshold, highThreshold).Render(spark)
	labelText := fmt.Sprintf("%s-%s", format(minV), format(maxV))
	arrowText := styleForThreshold(styles, colorLevel, warnThreshold, highThreshold).Render(arrow)
	return fmt.Sprintf("%-6s %s %s %s", label, arrowText, spark, styles.Dim.Render(labelText))
}

func sparklineBlocks(values []float64, width int, maxValue float64) string {
	const blocks = "▁▂▃▄▅▆▇█"
	if width <= 0 {
		return ""
	}
	if len(values) == 0 {
		return strings.Repeat("·", width)
	}
	values = resample(values, width)
	if maxValue <= 0 {
		for _, v := range values {
			if v > maxValue {
				maxValue = v
			}
		}
	}
	if maxValue <= 0 {
		return strings.Repeat("▁", width)
	}
	levels := []rune(blocks)
	var b strings.Builder
	for _, v := range values {
		if v < 0 {
			v = 0
		}
		idx := int((v / maxValue) * float64(len(levels)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(levels) {
			idx = len(levels) - 1
		}
		b.WriteRune(levels[idx])
	}
	return b.String()
}

func resample(values []float64, width int) []float64 {
	if width <= 0 {
		return nil
	}
	if len(values) == width {
		return append([]float64(nil), values...)
	}
	out := make([]float64, width)
	if len(values) == 1 {
		for i := range out {
			out[i] = values[0]
		}
		return out
	}
	for i := 0; i < width; i++ {
		pos := float64(i) * float64(len(values)-1) / float64(max(1, width-1))
		lo := int(pos)
		hi := minInt(lo+1, len(values)-1)
		frac := pos - float64(lo)
		out[i] = values[lo] + (values[hi]-values[lo])*frac
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func rangeMinMax(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	minV := values[0]
	maxV := values[0]
	for _, v := range values[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return minV, maxV
}

func trendArrow(values []float64) string {
	if len(values) < 2 {
		return "→"
	}
	prev := values[len(values)-2]
	curr := values[len(values)-1]
	delta := curr - prev
	switch {
	case delta > 0:
		return "↗"
	case delta < 0:
		return "↘"
	default:
		return "→"
	}
}

func styleForThreshold(styles Styles, value, warnThreshold, highThreshold float64) lipgloss.Style {
	switch {
	case value >= highThreshold:
		return styles.Error
	case value >= warnThreshold:
		return styles.Warn
	default:
		return styles.BarFull
	}
}

func shortByteString(v uint64) string {
	units := []string{"B", "K", "M", "G", "T"}
	f := float64(v)
	unit := 0
	for f >= 1024 && unit < len(units)-1 {
		f /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d%s", v, units[unit])
	}
	if f >= 10 {
		return fmt.Sprintf("%.0f%s", f, units[unit])
	}
	return fmt.Sprintf("%.1f%s", f, units[unit])
}

func CPUString(v float64) string {
	return cpuString(v)
}

func latestValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sortFloat64s(sorted)
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}

func thresholdRatio(value, baseline float64) float64 {
	if baseline <= 0 {
		return 0
	}
	return value / baseline
}

func sortFloat64s(values []float64) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
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
