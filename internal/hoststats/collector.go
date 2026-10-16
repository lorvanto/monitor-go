package hoststats

import (
	"context"
	"fmt"
	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"net/netip"
	"runtime"
	"strings"
	"sync"
	"time"
)

type netSample struct {
	in, out uint64
	at      time.Time
}
type DiskInfo struct {
	Path         string   `json:"path"`
	Filesystem   string   `json:"filesystem"`
	Used         uint64   `json:"used"`
	Total        uint64   `json:"total"`
	Percent      float64  `json:"percent"`
	CapacityID   string   `json:"capacityId,omitempty"`
	CapacityKind string   `json:"capacityKind,omitempty"`
	MountPoints  []string `json:"mountPoints,omitempty"`
}
type Inventory struct {
	OS       string   `json:"os"`
	Kernel   string   `json:"kernel"`
	Arch     string   `json:"arch"`
	CPU      string   `json:"cpu"`
	Cores    int      `json:"cores"`
	Memory   uint64   `json:"memory"`
	IPs      []string `json:"ips"`
	BootTime uint64   `json:"bootTime"`
	Source   string   `json:"source"`
}
type Snapshot struct {
	CPU            *float64   `json:"cpu"`
	Memory         *float64   `json:"memory"`
	MemoryUsed     uint64     `json:"memoryUsed"`
	Uptime         uint64     `json:"uptime"`
	NetIn          *float64   `json:"netIn"`
	NetOut         *float64   `json:"netOut"`
	Disks          []DiskInfo `json:"disks"`
	DiskIncomplete bool       `json:"diskIncomplete,omitempty"`
	Errors         []string   `json:"errors"`
}

type Report struct {
	Protocol     int       `json:"protocol"`
	ReportID     string    `json:"reportId"`
	AgentVersion string    `json:"agentVersion"`
	CollectedAt  int64     `json:"collectedAt"`
	Hostname     string    `json:"hostname"`
	Inventory    Inventory `json:"inventory"`
	Snapshot     Snapshot  `json:"snapshot"`
}
type collectorDependencies struct {
	now         func() time.Time
	hostInfo    func(context.Context) (*host.InfoStat, error)
	cpuInfo     func(context.Context) ([]cpu.InfoStat, error)
	cpuPercent  func(context.Context, time.Duration, bool) ([]float64, error)
	memory      func(context.Context) (*mem.VirtualMemoryStat, error)
	interfaces  func(context.Context) (gnet.InterfaceStatList, error)
	partitions  func(context.Context, bool) ([]disk.PartitionStat, error)
	usage       func(context.Context, string) (*disk.UsageStat, error)
	counters    func(context.Context, bool) ([]gnet.IOCountersStat, error)
	apfsCommand apfsCommandFunc
}

func systemDependencies() collectorDependencies {
	return collectorDependencies{now: time.Now, hostInfo: host.InfoWithContext, cpuInfo: cpu.InfoWithContext, cpuPercent: cpu.PercentWithContext, memory: mem.VirtualMemoryWithContext, interfaces: gnet.InterfacesWithContext, partitions: disk.PartitionsWithContext, usage: disk.UsageWithContext, counters: gnet.IOCountersWithContext, apfsCommand: platformAPFSCommand()}
}

type Collector struct {
	dependencies *collectorDependencies
	mu           sync.Mutex
	previousNet  *netSample
	staticInfo   *host.InfoStat
	staticCPU    string
	staticAt     time.Time
}

func (c *Collector) Collect(ctx context.Context) (Report, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	deps := systemDependencies()
	if c.dependencies != nil {
		deps = *c.dependencies
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	sampledAt := deps.now()
	if c.staticInfo == nil || sampledAt.Before(c.staticAt) || sampledAt.Sub(c.staticAt) >= 5*time.Minute {
		info, err := deps.hostInfo(ctx)
		if err != nil {
			return Report{}, err
		}
		if info == nil {
			return Report{}, fmt.Errorf("host inventory unavailable")
		}
		copy := *info
		c.staticInfo, c.staticAt, c.staticCPU = &copy, sampledAt, ""
	}
	info := *c.staticInfo
	info.Uptime += uint64(max(0, sampledAt.Sub(c.staticAt).Seconds()))
	if c.staticCPU == "" {
		if cpus, err := deps.cpuInfo(ctx); err == nil && len(cpus) > 0 {
			c.staticCPU = cpus[0].ModelName
		}
	}
	platform := info.Platform
	if strings.EqualFold(platform, "darwin") {
		// gopsutil reports the kernel name as the platform and sw_vers as its version.
		platform = "macOS"
	}
	inv := Inventory{OS: strings.TrimSpace(platform + " " + info.PlatformVersion), Kernel: info.KernelVersion, Arch: runtime.GOARCH, Cores: runtime.NumCPU(), BootTime: info.BootTime, Source: "服务端本机采集", IPs: []string{}}
	snap := Snapshot{Uptime: info.Uptime, Disks: []DiskInfo{}, Errors: []string{}}
	inv.CPU = c.staticCPU
	if v, e := deps.cpuPercent(ctx, time.Second, false); e == nil && len(v) > 0 {
		snap.CPU = &v[0]
	} else {
		appendCollectionError(&snap, "CPU 采样不可用")
	}
	if v, e := deps.memory(ctx); e == nil {
		inv.Memory = v.Total
		snap.MemoryUsed = v.Used
		p := v.UsedPercent
		snap.Memory = &p
	} else {
		appendCollectionError(&snap, "内存采样不可用")
	}
	if interfaces, e := deps.interfaces(ctx); e == nil {
		for _, i := range interfaces {
			for _, a := range i.Addrs {
				if prefix, err := netip.ParsePrefix(a.Addr); err == nil && prefix.Addr().IsGlobalUnicast() && !prefix.Addr().IsLoopback() {
					inv.IPs = append(inv.IPs, a.Addr)
				}
			}
		}
	}
	collectDisks(ctx, deps, &snap)
	if counters, e := deps.counters(ctx, true); e == nil {
		current := &netSample{at: deps.now()}
		for _, v := range counters {
			if strings.HasPrefix(v.Name, "lo") {
				continue
			}
			current.in += v.BytesRecv
			current.out += v.BytesSent
		}
		if prev := c.previousNet; prev != nil && current.at.After(prev.at) && current.in >= prev.in && current.out >= prev.out {
			dt := current.at.Sub(prev.at).Seconds()
			vin := float64(current.in-prev.in) / dt
			vout := float64(current.out-prev.out) / dt
			snap.NetIn = &vin
			snap.NetOut = &vout
		}
		c.previousNet = current
	} else {
		appendCollectionError(&snap, "网络流量不可用")
	}

	return Report{Protocol: 1, AgentVersion: buildinfo.Version, CollectedAt: deps.now().UnixMilli(), Hostname: info.Hostname, Inventory: inv, Snapshot: snap}, nil
}
