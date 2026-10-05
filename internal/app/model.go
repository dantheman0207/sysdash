package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	help "charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/daniel/sysdash/internal/actions"
	"github.com/daniel/sysdash/internal/collect"
	"github.com/daniel/sysdash/internal/diskscan"
	"github.com/daniel/sysdash/internal/domain"
	"github.com/daniel/sysdash/internal/ui"
)

type focusWidget int
type processSortKey string

const (
	focusCPU focusWidget = iota
	focusMemory
	focusDiskIO
	focusNetwork
	focusDiskBrowser
	focusCount

	sortCPU  processSortKey = "cpu"
	sortRSS  processSortKey = "rss"
	sortPID  processSortKey = "pid"
	sortName processSortKey = "name"
)

type tickMsg struct{}
type diskProgressMsg struct {
	path     string
	progress domain.DiskScanProgress
	ok       bool
}
type cleanupProgressMsg struct {
	progress domain.CleanupProgress
}
type snapshotMsg struct {
	data domain.DashboardData
	err  error
}
type diskScanMsg struct {
	path string
	node domain.DiskNode
	err  error
}
type actionMsg struct {
	text string
	err  error
}

const (
	heartbeatInterval        = 1 * time.Second
	focusedRefreshInterval   = 2 * time.Second
	unfocusedRefreshInterval = 30 * time.Second
	sparklineHistoryLimit    = 20
)

type keyMap struct {
	Tab     key.Binding
	BackTab key.Binding
	Left    key.Binding
	Right   key.Binding
	Enter   key.Binding
	Esc     key.Binding
	Up      key.Binding
	Down    key.Binding
	Top     key.Binding
	Bottom  key.Binding
	Refresh key.Binding
	Cleanup key.Binding
	Quit    key.Binding
	Back    key.Binding
	Yes     key.Binding
	No      key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Tab:     key.NewBinding(key.WithKeys("tab")),
		BackTab: key.NewBinding(key.WithKeys("shift+tab")),
		Left:    key.NewBinding(key.WithKeys("left")),
		Right:   key.NewBinding(key.WithKeys("right")),
		Enter:   key.NewBinding(key.WithKeys("enter")),
		Esc:     key.NewBinding(key.WithKeys("esc")),
		Up:      key.NewBinding(key.WithKeys("up")),
		Down:    key.NewBinding(key.WithKeys("down")),
		Top:     key.NewBinding(key.WithKeys("home")),
		Bottom:  key.NewBinding(key.WithKeys("end")),
		Refresh: key.NewBinding(key.WithKeys("f5")),
		Cleanup: key.NewBinding(key.WithKeys("f6")),
		Quit:    key.NewBinding(key.WithKeys("ctrl+c")),
		Back:    key.NewBinding(key.WithKeys("backspace", "ctrl+h")),
		Yes:     key.NewBinding(key.WithKeys("y")),
		No:      key.NewBinding(key.WithKeys("n")),
	}
}

type Model struct {
	width          int
	height         int
	hasFocus       bool
	view           domain.View
	focus          focusWidget
	data           domain.DashboardData
	sampler        *collect.Sampler
	scanner        *diskscan.Scanner
	cleanupRunner  *actions.CleanupRunner
	styles         ui.Styles
	keys           keyMap
	help           help.Model
	status         string
	nextRefreshAt  time.Time
	cpuHistory     []float64
	memHistory     []float64
	diskReadHist   []float64
	diskWriteHist  []float64
	netRXHist      []float64
	netTXHist      []float64
	selectedCPU    int
	selectedMem    int
	selectedConn   int
	selectedDisk   int
	cpuSort        processSortKey
	memSort        processSortKey
	processFilter  string
	diskPath       string
	diskNode       domain.DiskNode
	diskLoading    bool
	diskProgress   domain.DiskScanProgress
	cleanupConfirm bool
	cleanupState   domain.CleanupProgress
	detailOpen     bool
	detailProcess  domain.ProcessInfo
	actionMenuPID  int32
	actionMenuName string
	confirmPID     int32
	confirmName    string
	confirmSignal  syscall.Signal
	confirmLabel   string
}

func New() Model {
	h := help.New()
	h.ShowAll = false
	return Model{
		hasFocus:      true,
		view:          domain.ViewDashboard,
		sampler:       collect.NewSampler(),
		scanner:       diskscan.New(),
		cleanupRunner: actions.NewCleanupRunner(),
		styles:        ui.NewStyles(),
		keys:          newKeyMap(),
		help:          h,
		status:        "loading...",
		cpuSort:       sortCPU,
		memSort:       sortRSS,
		diskPath:      "/",
		nextRefreshAt: time.Now().Add(focusedRefreshInterval),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.tickCmd(), m.refreshCmd(), m.scanDirCmd(m.diskPath), m.diskProgressCmd(m.diskPath))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		if time.Now().Before(m.nextRefreshAt) {
			return m, m.tickCmd()
		}
		m.scheduleNextRefresh()
		return m, tea.Batch(m.tickCmd(), m.refreshCmd())
	case tea.FocusMsg:
		m.hasFocus = true
		m.status = "focus regained"
		m.scheduleNextRefresh()
		return m, m.tickCmd()
	case tea.BlurMsg:
		m.hasFocus = false
		m.status = "paused while unfocused"
		m.scheduleNextRefresh()
		return m, m.tickCmd()
	case snapshotMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.data = msg.data
		m.recordSparklineData(msg.data)
		m.status = fmt.Sprintf("updated %s", msg.data.Timestamp.Format("15:04:05"))
		return m, nil
	case diskScanMsg:
		if msg.err != nil && !diskscan.IsCanceled(msg.err) {
			m.status = msg.err.Error()
		}
		if msg.path == m.diskPath && msg.err == nil {
			if len(msg.node.Children) > 0 || !m.scanner.IsInflight(msg.path) {
				m.diskNode = msg.node
				m.selectedDisk = clamp(m.selectedDisk, len(m.diskNode.Children))
			}
		}
		if msg.path == m.diskPath {
			m.diskLoading = m.scanner.IsInflight(msg.path)
			if !m.diskLoading {
				if p, ok := m.scanner.Progress(msg.path); ok {
					m.diskProgress = p
				}
			}
		}
		return m, nil
	case diskProgressMsg:
		if msg.path == m.diskPath && msg.ok {
			m.diskProgress = msg.progress
			m.diskLoading = !msg.progress.Done
			if !msg.progress.Done {
				return m, m.diskProgressCmd(msg.path)
			}
		}
		return m, nil
	case cleanupProgressMsg:
		m.cleanupState = msg.progress
		if msg.progress.Running {
			return m, m.cleanupProgressCmd()
		}
		if msg.progress.Done {
			m.cleanupConfirm = false
			status := fmt.Sprintf("cleanup finished, saved %s", ui.ByteString(msg.progress.SavedBytes))
			failures := cleanupFailures(msg.progress.Results)
			if failures > 0 {
				status += fmt.Sprintf(" (%d step failures)", failures)
			}
			m.status = status
			return m, tea.Batch(m.refreshCmd(), m.rescanDirCmd(m.diskPath), m.diskProgressCmd(m.diskPath))
		}
		return m, nil
	case actionMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.status = msg.text
		}
		m.confirmPID = 0
		m.confirmName = ""
		m.confirmSignal = 0
		m.confirmLabel = ""
		return m, nil
	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
		if m.detailOpen {
			switch {
			case key.Matches(msg, m.keys.Enter):
				m.actionMenuPID = m.detailProcess.PID
				m.actionMenuName = m.detailProcess.Name
				m.detailOpen = false
				m.status = fmt.Sprintf("selected pid %d", m.actionMenuPID)
				return m, nil
			case key.Matches(msg, m.keys.Esc):
				m.detailOpen = false
				m.status = "closed process details"
				return m, nil
			default:
				return m, nil
			}
		}
		if m.cleanupConfirm {
			switch {
			case key.Matches(msg, m.keys.Yes):
				m.cleanupConfirm = false
				if err := m.cleanupRunner.Start(context.Background()); err != nil {
					m.status = err.Error()
					return m, nil
				}
				m.cleanupState = m.cleanupRunner.Progress()
				m.status = "cleanup started"
				return m, m.cleanupProgressCmd()
			case key.Matches(msg, m.keys.No), key.Matches(msg, m.keys.Esc):
				m.cleanupConfirm = false
				m.status = "cleanup canceled"
				return m, nil
			default:
				return m, nil
			}
		}
		if m.actionMenuPID != 0 {
			return m.handleActionMenu(msg)
		}
		if m.confirmPID != 0 {
			switch {
			case key.Matches(msg, m.keys.Yes):
				pid := m.confirmPID
				sig := m.confirmSignal
				text := m.confirmLabel + " sent"
				return m, tea.Batch(m.actionCmd(pid, sig, text), m.refreshCmd())
			case key.Matches(msg, m.keys.No), key.Matches(msg, m.keys.Esc):
				m.confirmPID = 0
				m.confirmName = ""
				m.confirmSignal = 0
				m.confirmLabel = ""
				m.status = "action canceled"
				return m, nil
			default:
				return m, nil
			}
		}
		if handled := m.handleProcessFilterKey(msg); handled {
			return m, nil
		}
		switch {
		case key.Matches(msg, m.keys.Refresh):
			m.scheduleNextRefresh()
			if m.view == domain.ViewDiskBrowser {
				m.diskLoading = true
				m.diskNode = domain.DiskNode{Path: m.diskPath}
				m.selectedDisk = 0
				return m, tea.Batch(m.refreshCmd(), m.rescanDirCmd(m.diskPath), m.diskProgressCmd(m.diskPath))
			}
			return m, tea.Batch(m.refreshCmd(), m.scanDirCmd(m.diskPath))
		case key.Matches(msg, m.keys.Cleanup):
			if m.view == domain.ViewDiskBrowser {
				if m.cleanupState.Running {
					m.status = "cleanup already running"
					return m, nil
				}
				m.cleanupConfirm = true
				return m, nil
			}
			return m, nil
		case key.Matches(msg, m.keys.Tab):
			if m.view == domain.ViewDashboard {
				m.focus = (m.focus + 1) % focusCount
			} else if m.view == domain.ViewCPU || m.view == domain.ViewMemory {
				m.cycleProcessSort(1)
			}
			return m, nil
		case key.Matches(msg, m.keys.BackTab):
			if m.view == domain.ViewDashboard {
				m.focus = (m.focus - 1 + focusCount) % focusCount
			} else if m.view == domain.ViewCPU || m.view == domain.ViewMemory {
				m.cycleProcessSort(-1)
			}
			return m, nil
		case key.Matches(msg, m.keys.Left):
			if m.view == domain.ViewDashboard {
				m.focus = moveDashboardFocus(m.focus, "left")
			} else if m.view == domain.ViewDiskBrowser {
				return m.handleBack()
			}
			return m, nil
		case key.Matches(msg, m.keys.Right):
			if m.view == domain.ViewDashboard {
				m.focus = moveDashboardFocus(m.focus, "right")
			} else if m.view == domain.ViewDiskBrowser {
				return m.handleEnter()
			}
			return m, nil
		case key.Matches(msg, m.keys.Esc):
			m.processFilter = ""
			m.view = domain.ViewDashboard
			return m, nil
		case key.Matches(msg, m.keys.Enter):
			return m.handleEnter()
		case key.Matches(msg, m.keys.Back):
			return m.handleBack()
		case key.Matches(msg, m.keys.Down):
			if m.view == domain.ViewDashboard {
				m.focus = moveDashboardFocus(m.focus, "down")
			} else {
				m.move(1)
			}
			return m, nil
		case key.Matches(msg, m.keys.Up):
			if m.view == domain.ViewDashboard {
				m.focus = moveDashboardFocus(m.focus, "up")
			} else {
				m.move(-1)
			}
			return m, nil
		case key.Matches(msg, m.keys.Top):
			m.moveTo(0)
			return m, nil
		case key.Matches(msg, m.keys.Bottom):
			m.moveToBottom()
			return m, nil
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	body := ""
	diskState := ui.DiskBrowserState{
		Path:     m.diskPath,
		Node:     m.diskNode,
		Selected: m.selectedDisk,
		Loading:  m.diskLoading,
		Progress: m.diskProgress,
		Cleanup:  m.cleanupState,
	}
	history := ui.DashboardHistory{
		CPU:       m.cpuHistory,
		Memory:    m.memHistory,
		DiskRead:  m.diskReadHist,
		DiskWrite: m.diskWriteHist,
		NetRX:     m.netRXHist,
		NetTX:     m.netTXHist,
	}
	switch m.view {
	case domain.ViewCPU:
		body = ui.RenderProcessTable(m.styles, m.width, m.height, "CPU Processes", m.filteredCPU(), m.selectedCPU, m.processFilter, processSortLabel(m.cpuSort))
	case domain.ViewMemory:
		body = ui.RenderProcessTable(m.styles, m.width, m.height, "Memory Processes", m.filteredMemory(), m.selectedMem, m.processFilter, processSortLabel(m.memSort))
	case domain.ViewNetwork:
		body = ui.RenderConnections(m.styles, m.width, m.height, m.data.Connections, m.selectedConn)
	case domain.ViewDiskBrowser:
		body = ui.RenderDiskBrowser(m.styles, m.width, m.height, m.data.DiskUsage, diskState)
	default:
		body = ui.RenderDashboard(m.styles, m.width, int(m.focus), m.data, diskState, history)
	}

	if m.actionMenuPID != 0 {
		menu := m.styles.Focused.Width(min(max(44, m.width-12), 72)).Render(
			m.styles.Warn.Render("Process Actions") + "\n\n" +
				fmt.Sprintf("%s (pid %d)\n\n", m.actionMenuName, m.actionMenuPID) +
				"1. SIGTERM   2. SIGKILL   3. SIGSTOP   4. SIGCONT\n\n" +
				"Press 1-4 to choose, or esc to cancel.",
		)
		body = lipgloss.JoinVertical(lipgloss.Left, menu, "", body)
	}
	if m.detailOpen {
		detail := m.styles.Focused.Width(min(max(52, m.width-12), 96)).Render(
			m.styles.Warn.Render("Process Details") + "\n\n" +
				fmt.Sprintf("Name     %s\n", m.detailProcess.Name) +
				fmt.Sprintf("PID      %d\n", m.detailProcess.PID) +
				fmt.Sprintf("User     %s\n", blankIfEmpty(m.detailProcess.User)) +
				fmt.Sprintf("State    %s\n", blankIfEmpty(m.detailProcess.Status)) +
				fmt.Sprintf("CPU      %s\n", ui.CPUString(m.detailProcess.CPU)) +
				fmt.Sprintf("RSS      %s\n", ui.ByteString(m.detailProcess.RSS)) +
				fmt.Sprintf("Threads  %d\n\n", m.detailProcess.Threads) +
				fmt.Sprintf("Command  %s\n\n", blankIfEmpty(m.detailProcess.Cmdline)) +
				"Press enter for actions, or esc to close.",
		)
		body = lipgloss.JoinVertical(lipgloss.Left, detail, "", body)
	}
	if m.confirmPID != 0 {
		confirm := m.styles.Focused.Width(min(max(44, m.width-12), 72)).Render(
			m.styles.Warn.Render("Confirm "+m.confirmLabel) + "\n\n" +
				fmt.Sprintf("%s (pid %d)\n\n", m.confirmName, m.confirmPID) +
				fmt.Sprintf("This will send %s immediately.\n\n", m.confirmLabel) +
				"Press y to confirm, n to cancel, or esc to back out.",
		)
		body = lipgloss.JoinVertical(lipgloss.Left, confirm, "", body)
	}
	if m.cleanupConfirm {
		confirm := m.styles.Focused.Width(min(max(44, m.width-12), 72)).Render(
			m.styles.Warn.Render("Confirm Cleanup") + "\n\n" +
				"This will run:\n" +
				"go clean -cache -modcache\n" +
				"remove Xcode and simulator caches\n" +
				"delete ~/Library/Caches and ~/.cache\n" +
				"empty trash\n" +
				"brew cleanup -s\n" +
				"docker system prune -f (best effort)\n\n" +
				"Press y to confirm, n to cancel, or esc to back out.",
		)
		body = lipgloss.JoinVertical(lipgloss.Left, confirm, "", body)
	}
	header := m.topBar()
	if header != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, header, "", body)
	}
	body += "\n\n" + m.styles.Dim.Render(m.status)
	v := tea.NewView(m.styles.App.Width(m.width).Render(body))
	v.AltScreen = true
	v.ReportFocus = true
	return v
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(heartbeatInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) refreshCmd() tea.Cmd {
	opts := collect.SnapshotOptions{
		FullProcessDetails: m.view == domain.ViewCPU || m.view == domain.ViewMemory,
		IncludeConnections: m.view == domain.ViewNetwork,
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		data, err := m.sampler.Snapshot(ctx, opts)
		return snapshotMsg{data: data, err: err}
	}
}

func (m Model) scanDirCmd(path string) tea.Cmd {
	return func() tea.Msg {
		node, err := m.scanner.Scan(context.Background(), path)
		return diskScanMsg{path: path, node: node, err: err}
	}
}

func (m Model) rescanDirCmd(path string) tea.Cmd {
	return func() tea.Msg {
		node, err := m.scanner.ForceScan(context.Background(), path)
		return diskScanMsg{path: path, node: node, err: err}
	}
}

func (m Model) diskProgressCmd(path string) tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		progress, ok := m.scanner.Progress(path)
		return diskProgressMsg{path: path, progress: progress, ok: ok}
	})
}

func (m Model) cleanupProgressCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return cleanupProgressMsg{progress: m.cleanupRunner.Progress()}
	})
}

func (m Model) actionCmd(pid int32, sig syscall.Signal, text string) tea.Cmd {
	return func() tea.Msg {
		err := actions.Signal(pid, sig)
		return actionMsg{text: text, err: err}
	}
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.view {
	case domain.ViewDashboard:
		switch m.focus {
		case focusCPU:
			m.processFilter = ""
			m.view = domain.ViewCPU
		case focusMemory:
			m.processFilter = ""
			m.view = domain.ViewMemory
		case focusNetwork:
			m.view = domain.ViewNetwork
		case focusDiskBrowser:
			m.view = domain.ViewDiskBrowser
		}
		return m, nil
	case domain.ViewCPU, domain.ViewMemory:
		if proc, ok := m.currentProcess(); ok {
			m.detailOpen = true
			m.detailProcess = proc
			m.status = fmt.Sprintf("selected pid %d", proc.PID)
		}
		return m, nil
	case domain.ViewDiskBrowser:
		if len(m.diskNode.Children) == 0 {
			return m, nil
		}
		child := m.diskNode.Children[m.selectedDisk]
		if !child.IsDir {
			return m, nil
		}
		m.diskPath = child.Path
		m.selectedDisk = 0
		m.diskLoading = true
		m.diskProgress = domain.DiskScanProgress{}
		if cached, ok := m.scanner.Cached(m.diskPath); ok {
			m.diskNode = cached
			m.diskLoading = false
			return m, nil
		}
		m.diskNode = domain.DiskNode{Path: child.Path}
		return m, tea.Batch(m.scanDirCmd(child.Path), m.diskProgressCmd(child.Path))
	default:
		return m, nil
	}
}

func (m Model) handleBack() (tea.Model, tea.Cmd) {
	if m.view != domain.ViewDiskBrowser {
		return m, nil
	}
	parent := filepath.Dir(m.diskPath)
	if parent == "." || parent == m.diskPath {
		parent = "/"
	}
	m.diskPath = parent
	m.selectedDisk = 0
	m.diskProgress = domain.DiskScanProgress{}
	if cached, ok := m.scanner.Cached(parent); ok {
		m.diskNode = cached
		m.diskLoading = false
		return m, nil
	}
	m.diskLoading = true
	m.diskNode = domain.DiskNode{Path: parent}
	return m, tea.Batch(m.scanDirCmd(parent), m.diskProgressCmd(parent))
}

func (m *Model) move(delta int) {
	switch m.view {
	case domain.ViewCPU:
		m.selectedCPU = clamp(m.selectedCPU+delta, len(m.filteredCPU()))
	case domain.ViewMemory:
		m.selectedMem = clamp(m.selectedMem+delta, len(m.filteredMemory()))
	case domain.ViewNetwork:
		m.selectedConn = clamp(m.selectedConn+delta, len(m.data.Connections))
	case domain.ViewDiskBrowser:
		m.selectedDisk = clamp(m.selectedDisk+delta, len(m.diskNode.Children))
	}
}

func (m *Model) moveTo(index int) {
	switch m.view {
	case domain.ViewCPU:
		m.selectedCPU = clamp(index, len(m.filteredCPU()))
	case domain.ViewMemory:
		m.selectedMem = clamp(index, len(m.filteredMemory()))
	case domain.ViewNetwork:
		m.selectedConn = clamp(index, len(m.data.Connections))
	case domain.ViewDiskBrowser:
		m.selectedDisk = clamp(index, len(m.diskNode.Children))
	}
}

func (m *Model) moveToBottom() {
	switch m.view {
	case domain.ViewCPU:
		m.selectedCPU = bottom(len(m.filteredCPU()))
	case domain.ViewMemory:
		m.selectedMem = bottom(len(m.filteredMemory()))
	case domain.ViewNetwork:
		m.selectedConn = bottom(len(m.data.Connections))
	case domain.ViewDiskBrowser:
		m.selectedDisk = bottom(len(m.diskNode.Children))
	}
}

func (m Model) currentPID() (int32, bool) {
	if proc, ok := m.currentProcess(); ok {
		return proc.PID, true
	}
	return 0, false
}

func (m Model) currentProcess() (domain.ProcessInfo, bool) {
	switch m.view {
	case domain.ViewCPU:
		rows := m.filteredCPU()
		if len(rows) == 0 {
			return domain.ProcessInfo{}, false
		}
		return rows[m.selectedCPU], true
	case domain.ViewMemory:
		rows := m.filteredMemory()
		if len(rows) == 0 {
			return domain.ProcessInfo{}, false
		}
		return rows[m.selectedMem], true
	default:
		return domain.ProcessInfo{}, false
	}
}

func clamp(index, length int) int {
	if length <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func bottom(length int) int {
	if length <= 0 {
		return 0
	}
	return length - 1
}

func moveDashboardFocus(curr focusWidget, dir string) focusWidget {
	switch curr {
	case focusCPU:
		switch dir {
		case "right":
			return focusMemory
		case "up":
			return focusDiskBrowser
		case "down":
			return focusDiskIO
		}
	case focusMemory:
		switch dir {
		case "left":
			return focusCPU
		case "up":
			return focusDiskBrowser
		case "down":
			return focusNetwork
		}
	case focusDiskIO:
		switch dir {
		case "up":
			return focusCPU
		case "right":
			return focusNetwork
		}
	case focusNetwork:
		switch dir {
		case "up":
			return focusMemory
		case "left":
			return focusDiskIO
		}
	case focusDiskBrowser:
		switch dir {
		case "down":
			return focusMemory
		}
	}
	return curr
}

func (m *Model) handleProcessFilterKey(msg tea.KeyPressMsg) bool {
	if m.view != domain.ViewCPU && m.view != domain.ViewMemory {
		return false
	}
	switch {
	case key.Matches(msg, m.keys.Back):
		if m.processFilter == "" {
			return false
		}
		r := []rune(m.processFilter)
		m.processFilter = string(r[:len(r)-1])
		m.resetProcessSelection()
		m.status = processFilterStatus(m.processFilter, m.filteredProcessCount())
		return true
	case isPrintableKey(msg) && !m.isReservedProcessKey(msg):
		m.processFilter += msg.Text
		m.resetProcessSelection()
		m.status = processFilterStatus(m.processFilter, m.filteredProcessCount())
		return true
	default:
		return false
	}
}

func (m *Model) resetProcessSelection() {
	switch m.view {
	case domain.ViewCPU:
		m.selectedCPU = 0
	case domain.ViewMemory:
		m.selectedMem = 0
	}
}

func (m Model) filteredProcessCount() int {
	switch m.view {
	case domain.ViewCPU:
		return len(m.filteredCPU())
	case domain.ViewMemory:
		return len(m.filteredMemory())
	default:
		return 0
	}
}

func (m Model) filteredCPU() []domain.ProcessInfo {
	return sortProcesses(filterProcesses(m.data.TopCPU, m.processFilter), m.cpuSort)
}

func (m Model) filteredMemory() []domain.ProcessInfo {
	return sortProcesses(filterProcesses(m.data.TopMemory, m.processFilter), m.memSort)
}

func filterProcesses(in []domain.ProcessInfo, query string) []domain.ProcessInfo {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return in
	}
	out := make([]domain.ProcessInfo, 0, len(in))
	for _, p := range in {
		haystack := strings.ToLower(strings.Join([]string{
			p.Name,
			p.Cmdline,
			p.User,
			p.Status,
			strconv.Itoa(int(p.PID)),
		}, " "))
		if strings.Contains(haystack, query) {
			out = append(out, p)
		}
	}
	return out
}

func processFilterStatus(query string, matches int) string {
	if query == "" {
		return "filter cleared"
	}
	return fmt.Sprintf("filter: %q (%d matches)", query, matches)
}

func isPrintableKey(msg tea.KeyPressMsg) bool {
	return msg.Text != "" && msg.Mod == 0
}

func (m Model) isReservedProcessKey(msg tea.KeyPressMsg) bool {
	return key.Matches(msg,
		m.keys.Up,
		m.keys.Down,
		m.keys.Top,
		m.keys.Bottom,
		m.keys.Refresh,
		m.keys.Enter,
		m.keys.Esc,
		m.keys.Quit,
	)
}

func (m *Model) cycleProcessSort(delta int) {
	order := []processSortKey{sortCPU, sortRSS, sortPID, sortName}
	switch m.view {
	case domain.ViewCPU:
		m.cpuSort = nextProcessSort(m.cpuSort, order, delta)
		m.selectedCPU = clamp(m.selectedCPU, len(m.filteredCPU()))
		m.status = "sort: " + processSortLabel(m.cpuSort)
	case domain.ViewMemory:
		m.memSort = nextProcessSort(m.memSort, order, delta)
		m.selectedMem = clamp(m.selectedMem, len(m.filteredMemory()))
		m.status = "sort: " + processSortLabel(m.memSort)
	}
}

func (m Model) handleActionMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.Text {
	case "1":
		return m.beginConfirmation(syscall.SIGTERM, "SIGTERM")
	case "2":
		return m.beginConfirmation(syscall.SIGKILL, "SIGKILL")
	case "3":
		return m.beginConfirmation(syscall.SIGSTOP, "SIGSTOP")
	case "4":
		return m.beginConfirmation(syscall.SIGCONT, "SIGCONT")
	}
	if key.Matches(msg, m.keys.Esc) {
		m.actionMenuPID = 0
		m.actionMenuName = ""
		m.status = "action menu canceled"
		return m, nil
	}
	return m, nil
}

func (m Model) beginConfirmation(sig syscall.Signal, label string) (tea.Model, tea.Cmd) {
	pid := m.actionMenuPID
	name := m.actionMenuName
	m.actionMenuPID = 0
	m.actionMenuName = ""
	m.confirmPID = pid
	m.confirmName = name
	m.confirmSignal = sig
	m.confirmLabel = label
	m.status = fmt.Sprintf("pending %s confirmation for pid %d", label, pid)
	return m, nil
}

func (m *Model) scheduleNextRefresh() {
	m.nextRefreshAt = time.Now().Add(m.refreshInterval())
}

func (m Model) refreshInterval() time.Duration {
	if m.hasFocus {
		return focusedRefreshInterval
	}
	return unfocusedRefreshInterval
}

func (m Model) topBar() string {
	if m.view == domain.ViewDiskBrowser {
		return "sysdash"
	}
	left := "sysdash"
	if !m.hasFocus {
		left += " [background]"
	}
	right := "next update " + m.countdownText()
	width := max(20, m.width-6)
	if width <= len(left)+len(right)+1 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", width-len(left)-len(right)) + right
}

func (m Model) countdownText() string {
	remaining := time.Until(m.nextRefreshAt)
	if remaining <= 0 {
		return "now"
	}
	seconds := int((remaining + time.Second - 1) / time.Second)
	return fmt.Sprintf("%ds", seconds)
}

func init() {
	if _, err := os.Stat("/"); err != nil {
		panic(err)
	}
}

func topChildren(node domain.DiskNode, limit int) domain.DiskNode {
	out := node
	out.Children = append([]domain.DiskEntry(nil), node.Children...)
	sort.Slice(out.Children, func(i, j int) bool { return out.Children[i].Size > out.Children[j].Size })
	if len(out.Children) > limit {
		out.Children = out.Children[:limit]
	}
	return out
}

func cleanupFailures(results []domain.CleanupStepResult) int {
	var failures int
	for _, result := range results {
		if !result.OK {
			failures++
		}
	}
	return failures
}

func (m *Model) recordSparklineData(data domain.DashboardData) {
	m.cpuHistory = appendSparklinePoint(m.cpuHistory, data.CPUPercent)
	m.memHistory = appendSparklinePoint(m.memHistory, data.Memory.UsedPercent)
	m.diskReadHist = appendSparklinePoint(m.diskReadHist, float64(data.DiskIO.ReadBytesPerSec))
	m.diskWriteHist = appendSparklinePoint(m.diskWriteHist, float64(data.DiskIO.WriteBytesPerSec))
	m.netRXHist = appendSparklinePoint(m.netRXHist, float64(data.Network.ReadBytesPerSec))
	m.netTXHist = appendSparklinePoint(m.netTXHist, float64(data.Network.WriteBytesPerSec))
}

func appendSparklinePoint(history []float64, value float64) []float64 {
	history = append(history, value)
	if len(history) > sparklineHistoryLimit {
		history = history[len(history)-sparklineHistoryLimit:]
	}
	return history
}

func sortProcesses(in []domain.ProcessInfo, key processSortKey) []domain.ProcessInfo {
	out := append([]domain.ProcessInfo(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		switch key {
		case sortRSS:
			if out[i].RSS == out[j].RSS {
				return out[i].PID < out[j].PID
			}
			return out[i].RSS > out[j].RSS
		case sortPID:
			return out[i].PID < out[j].PID
		case sortName:
			if out[i].Name == out[j].Name {
				return out[i].PID < out[j].PID
			}
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		default:
			if out[i].CPU == out[j].CPU {
				return out[i].PID < out[j].PID
			}
			return out[i].CPU > out[j].CPU
		}
	})
	return out
}

func nextProcessSort(current processSortKey, order []processSortKey, delta int) processSortKey {
	for i, key := range order {
		if key == current {
			next := (i + delta + len(order)) % len(order)
			return order[next]
		}
	}
	return order[0]
}

func processSortLabel(key processSortKey) string {
	switch key {
	case sortRSS:
		return "RSS desc"
	case sortPID:
		return "PID asc"
	case sortName:
		return "Name asc"
	default:
		return "CPU desc"
	}
}

func blankIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
