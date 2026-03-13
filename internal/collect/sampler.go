package collect

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	gdisk "github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/daniel/sysdash/internal/domain"
)

type Sampler struct {
	mu              sync.Mutex
	lastNet         gnet.IOCountersStat
	lastDisk        gdisk.IOCountersStat
	lastNetSample   time.Time
	lastDiskSample  time.Time
	lastConnSample  time.Time
	netInitialized  bool
	diskInitialized bool
	cachedConns     []domain.ConnectionInfo
}

func NewSampler() *Sampler {
	return &Sampler{}
}

type SnapshotOptions struct {
	FullProcessDetails bool
	IncludeConnections bool
}

const (
	processDetailLimit   = 80
	connectionRefreshTTL = 10 * time.Second
)

func (s *Sampler) Snapshot(ctx context.Context, opts SnapshotOptions) (domain.DashboardData, error) {
	var out domain.DashboardData
	out.Timestamp = time.Now()

	percentages, err := cpu.PercentWithContext(ctx, 0, false)
	if err == nil && len(percentages) > 0 {
		out.CPUPercent = percentages[0]
	}

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err == nil {
		out.Memory = domain.MemoryStats{
			Used:        vm.Used,
			Total:       vm.Total,
			UsedPercent: vm.UsedPercent,
		}
	}

	usage, err := gdisk.UsageWithContext(ctx, "/")
	if err == nil {
		out.DiskUsage = domain.DiskUsage{
			Path:        "/",
			Used:        usage.Used,
			Total:       usage.Total,
			UsedPercent: usage.UsedPercent,
		}
	}

	s.captureThroughput(ctx, &out)

	processes, procErr := sampleProcessesBasic(ctx)
	if procErr == nil {
		topCPU := topByCPU(processes)
		topMemory := topByMemory(processes)
		if opts.FullProcessDetails {
			enrichTopProcesses(ctx, topCPU, processDetailLimit)
			enrichTopProcesses(ctx, topMemory, processDetailLimit)
		}
		out.TopCPU = topCPU
		out.TopMemory = topMemory
	} else {
		out.LastError = procErr.Error()
	}

	if opts.IncludeConnections {
		connections, connErr := s.connections(ctx)
		if connErr == nil {
			out.Connections = connections
		} else if out.LastError == "" {
			out.LastError = connErr.Error()
		}
	}

	return out, nil
}

func (s *Sampler) captureThroughput(ctx context.Context, out *domain.DashboardData) {
	netCounters, err := gnet.IOCountersWithContext(ctx, false)
	if err == nil && len(netCounters) > 0 {
		s.mu.Lock()
		if s.netInitialized {
			elapsed := time.Since(s.lastNetSample).Seconds()
			if elapsed > 0 {
				out.Network = domain.Throughput{
					ReadBytesPerSec:  rate(netCounters[0].BytesRecv, s.lastNet.BytesRecv, elapsed),
					WriteBytesPerSec: rate(netCounters[0].BytesSent, s.lastNet.BytesSent, elapsed),
				}
			}
		}
		s.lastNet = netCounters[0]
		s.lastNetSample = time.Now()
		s.netInitialized = true
		s.mu.Unlock()
	}

	diskCounters, err := gdisk.IOCountersWithContext(ctx)
	if err == nil {
		var totalRead uint64
		var totalWrite uint64
		for _, counter := range diskCounters {
			totalRead += counter.ReadBytes
			totalWrite += counter.WriteBytes
		}

		s.mu.Lock()
		if s.diskInitialized {
			elapsed := time.Since(s.lastDiskSample).Seconds()
			if elapsed > 0 {
				out.DiskIO = domain.Throughput{
					ReadBytesPerSec:  rate(totalRead, s.lastDisk.ReadBytes, elapsed),
					WriteBytesPerSec: rate(totalWrite, s.lastDisk.WriteBytes, elapsed),
				}
			}
		}
		s.lastDisk = gdisk.IOCountersStat{ReadBytes: totalRead, WriteBytes: totalWrite}
		s.lastDiskSample = time.Now()
		s.diskInitialized = true
		s.mu.Unlock()
	}
}

func sampleProcessesBasic(ctx context.Context) ([]domain.ProcessInfo, error) {
	ps, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]domain.ProcessInfo, 0, len(ps))
	for _, p := range ps {
		info := domain.ProcessInfo{PID: p.Pid}

		if name, err := p.NameWithContext(ctx); err == nil {
			info.Name = name
		}
		if info.Name == "" {
			continue
		}
		if cpuPercent, err := p.CPUPercentWithContext(ctx); err == nil {
			info.CPU = cpuPercent
		}
		if memInfo, err := p.MemoryInfoWithContext(ctx); err == nil && memInfo != nil {
			info.RSS = memInfo.RSS
		}
		result = append(result, info)
	}
	return result, nil
}

func enrichTopProcesses(ctx context.Context, processes []domain.ProcessInfo, limit int) {
	if len(processes) > limit {
		processes = processes[:limit]
	}
	for i := range processes {
		p, err := process.NewProcessWithContext(ctx, processes[i].PID)
		if err != nil {
			continue
		}
		if status, err := p.StatusWithContext(ctx); err == nil && len(status) > 0 {
			processes[i].Status = status[0]
		}
		if user, err := p.UsernameWithContext(ctx); err == nil {
			processes[i].User = user
		}
		if threads, err := p.NumThreadsWithContext(ctx); err == nil {
			processes[i].Threads = threads
		}
		if cmd, err := p.CmdlineWithContext(ctx); err == nil {
			processes[i].Cmdline = cmd
		}
	}
}

func topByCPU(in []domain.ProcessInfo) []domain.ProcessInfo {
	out := append([]domain.ProcessInfo(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].CPU == out[j].CPU {
			return out[i].PID < out[j].PID
		}
		return out[i].CPU > out[j].CPU
	})
	return out
}

func topByMemory(in []domain.ProcessInfo) []domain.ProcessInfo {
	out := append([]domain.ProcessInfo(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].RSS == out[j].RSS {
			return out[i].PID < out[j].PID
		}
		return out[i].RSS > out[j].RSS
	})
	return out
}

func rate(curr, prev uint64, seconds float64) uint64 {
	if curr < prev || seconds <= 0 {
		return 0
	}
	return uint64(float64(curr-prev) / seconds)
}

func (s *Sampler) connections(ctx context.Context) ([]domain.ConnectionInfo, error) {
	s.mu.Lock()
	if len(s.cachedConns) > 0 && time.Since(s.lastConnSample) < connectionRefreshTTL {
		out := append([]domain.ConnectionInfo(nil), s.cachedConns...)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()

	connections, err := ProcessConnections(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cachedConns = append([]domain.ConnectionInfo(nil), connections...)
	s.lastConnSample = time.Now()
	s.mu.Unlock()

	return connections, nil
}
