// Package sysstats samples host-level and process-level runtime metrics
// (CPU, memory, disk, load average, uptime) for the dashboard System card.
//
// CPU percentage is a delta measurement: gopsutil's cpu.Percent(0, ...) needs
// a previous sample to diff against, so the Sampler primes itself once at
// construction and exposes Samples so clients can show a placeholder until a
// real delta exists.
package sysstats

import (
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

// CPUStats is host CPU utilization and load. Load fields are nil on platforms
// without load average (Windows).
type CPUStats struct {
	Percent float64  `json:"percent"`
	Cores   int      `json:"cores"`
	Load1   *float64 `json:"load1,omitempty"`
	Load5   *float64 `json:"load5,omitempty"`
	Load15  *float64 `json:"load15,omitempty"`
}

// MemoryStats is host RAM plus swap. Swap fields are nil when unavailable.
type MemoryStats struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"used_percent"`
	SwapTotal   *uint64 `json:"swap_total,omitempty"`
	SwapUsed    *uint64 `json:"swap_used,omitempty"`
}

// DiskStats is filesystem usage for one path.
type DiskStats struct {
	Path        string  `json:"path"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
}

// HostStats identifies the host OS and its uptime.
type HostStats struct {
	OS             string  `json:"os"`
	Platform       *string `json:"platform,omitempty"`
	HostUptimeSecs uint64  `json:"host_uptime_secs"`
}

// ProcStats is gateway-process runtime state. RSSBytes is nil on error.
type ProcStats struct {
	PID            int32   `json:"pid"`
	Goroutines     int     `json:"goroutines"`
	HeapAllocBytes uint64  `json:"heap_alloc_bytes"`
	RSSBytes       *uint64 `json:"rss_bytes,omitempty"`
	GoVersion      string  `json:"go_version"`
}

// Stats is the full /v1/system/stats payload. Disk is nil when diskPath is
// empty or the stat fails.
type Stats struct {
	CPU     CPUStats    `json:"cpu"`
	Memory  MemoryStats `json:"memory"`
	Disk    *DiskStats  `json:"disk,omitempty"`
	Host    HostStats   `json:"host"`
	Proc    ProcStats   `json:"proc"`
	Samples int         `json:"samples"`
}

// Sampler keeps CPU delta state between snapshots. Safe for concurrent use.
type Sampler struct {
	mu      sync.Mutex
	percent float64
	samples int
}

// NewSampler creates a Sampler and primes the CPU delta in the background so
// startup is not blocked by the initial 200ms measurement.
func NewSampler() *Sampler {
	s := &Sampler{}
	go func() {
		if p, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(p) > 0 {
			s.mu.Lock()
			s.percent = p[0]
			s.samples++
			s.mu.Unlock()
		}
	}()
	return s
}

// Snapshot collects all metrics. Each sub-system is sampled independently: a
// failure in one (e.g. load average on Windows) never fails the rest. The CPU
// delta read is serialized under the mutex so concurrent snapshots never
// diff against a just-stored sub-millisecond sample.
func (s *Sampler) Snapshot(diskPath string) Stats {
	var st Stats

	s.mu.Lock()
	// CPU: interval 0 diffs against the previous call inside gopsutil.
	if p, err := cpu.Percent(0, false); err == nil && len(p) > 0 {
		s.percent = p[0]
		s.samples++
		st.CPU.Percent = p[0]
		st.Samples = s.samples
	} else {
		st.CPU.Percent = s.percent
		st.Samples = s.samples
	}
	s.mu.Unlock()
	st.CPU.Cores = runtime.NumCPU()
	if avg, err := load.Avg(); err == nil {
		st.CPU.Load1 = &avg.Load1
		st.CPU.Load5 = &avg.Load5
		st.CPU.Load15 = &avg.Load15
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		st.Memory.Total = vm.Total
		st.Memory.Used = vm.Used
		st.Memory.Available = vm.Available
		st.Memory.UsedPercent = vm.UsedPercent
	}
	if sw, err := mem.SwapMemory(); err == nil {
		total, used := sw.Total, sw.Used
		st.Memory.SwapTotal = &total
		st.Memory.SwapUsed = &used
	}

	if diskPath != "" {
		if du, err := disk.Usage(diskPath); err == nil {
			st.Disk = &DiskStats{
				Path:        du.Path,
				Total:       du.Total,
				Used:        du.Used,
				UsedPercent: du.UsedPercent,
			}
		}
	}

	st.Host.OS = runtime.GOOS
	if hi, err := host.Info(); err == nil {
		platform := hi.Platform + " " + hi.PlatformVersion
		st.Host.Platform = &platform
		st.Host.HostUptimeSecs = hi.Uptime
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	st.Proc = ProcStats{
		PID:            int32(os.Getpid()),
		Goroutines:     runtime.NumGoroutine(),
		HeapAllocBytes: ms.HeapAlloc,
		GoVersion:      runtime.Version(),
	}
	if p, err := process.NewProcess(st.Proc.PID); err == nil {
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			rss := mi.RSS
			st.Proc.RSSBytes = &rss
		}
	}

	return st
}
