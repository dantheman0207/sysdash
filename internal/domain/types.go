package domain

import "time"

type View string

const (
	ViewDashboard   View = "dashboard"
	ViewCPU         View = "cpu"
	ViewMemory      View = "memory"
	ViewNetwork     View = "network"
	ViewDiskBrowser View = "disk"
)

type DashboardData struct {
	Timestamp   time.Time
	CPUPercent  float64
	Memory      MemoryStats
	DiskIO      Throughput
	Network     Throughput
	DiskUsage   DiskUsage
	TopCPU      []ProcessInfo
	TopMemory   []ProcessInfo
	Connections []ConnectionInfo
	LastError   string
}

type MemoryStats struct {
	Used        uint64
	Total       uint64
	UsedPercent float64
}

type Throughput struct {
	ReadBytesPerSec  uint64
	WriteBytesPerSec uint64
}

type DiskUsage struct {
	Path        string
	Used        uint64
	Total       uint64
	UsedPercent float64
}

type ProcessInfo struct {
	PID     int32
	Name    string
	CPU     float64
	RSS     uint64
	Status  string
	User    string
	Threads int32
	Cmdline string
}

type ConnectionInfo struct {
	Command string
	PID     int32
	User    string
	Proto   string
	Local   string
	Remote  string
	State   string
}

type DiskEntry struct {
	Name       string
	Path       string
	Size       uint64
	IsDir      bool
	Percent    float64
	ScannedAt  time.Time
	Incomplete bool
}

type DiskNode struct {
	Path      string
	Size      uint64
	Children  []DiskEntry
	ScannedAt time.Time
}

type DiskScanProgress struct {
	Root      string
	Current   string
	Completed int
	Total     int
	StartedAt time.Time
	Done      bool
}

type CleanupStepResult struct {
	Name    string
	OK      bool
	Detail  string
	Started time.Time
	Ended   time.Time
}

type CleanupRunSummary struct {
	FinishedAt time.Time
	SavedBytes uint64
	Failures   int
}

type CleanupProgress struct {
	Running     bool
	Done        bool
	Completed   int
	Total       int
	CurrentStep string
	StartedAt   time.Time
	FinishedAt  time.Time
	BeforeFree  uint64
	AfterFree   uint64
	SavedBytes  uint64
	Results     []CleanupStepResult
	History     []CleanupRunSummary
}
