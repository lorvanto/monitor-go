package hoststats

import (
	"context"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"net/netip"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

type netSample struct {
	in, out uint64
	at      time.Time
}
type DiskInfo struct {
	Path       string  `json:"path"`
	Filesystem string  `json:"filesystem"`
	Used       uint64  `json:"used"`
	Total      uint64  `json:"total"`
	Percent    float64 `json:"percent"`
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
	CPU        *float64   `json:"cpu"`
	Memory     *float64   `json:"memory"`
	MemoryUsed uint64     `json:"memoryUsed"`
	Uptime     uint64     `json:"uptime"`
	NetIn      *float64   `json:"netIn"`
	NetOut     *float64   `json:"netOut"`
	Disks      []DiskInfo `json:"disks"`
	Errors     []string   `json:"errors"`
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
type Collector struct {
	mu          sync.Mutex
	previousNet *netSample
}

func (c *Collector) Collect(ctx context.Context) (Report, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	info, e := host.InfoWithContext(ctx)
	if e != nil {
		return Report{}, e
	}
	inv := Inventory{OS: strings.TrimSpace(info.Platform + " " + info.PlatformVersion), Kernel: info.KernelVersion, Arch: runtime.GOARCH, Cores: runtime.NumCPU(), BootTime: info.BootTime, Source: "服务端本机采集", IPs: []string{}}
	snap := Snapshot{Uptime: info.Uptime, Disks: []DiskInfo{}, Errors: []string{}}
	if cpus, e := cpu.InfoWithContext(ctx); e == nil && len(cpus) > 0 {
		inv.CPU = cpus[0].ModelName
	}
	if v, e := cpu.PercentWithContext(ctx, time.Second, false); e == nil && len(v) > 0 {
		snap.CPU = &v[0]
	} else {
		snap.Errors = append(snap.Errors, "CPU 采样不可用")
	}
	if v, e := mem.VirtualMemoryWithContext(ctx); e == nil {
		inv.Memory = v.Total
		snap.MemoryUsed = v.Used
		p := v.UsedPercent
		snap.Memory = &p
	} else {
		snap.Errors = append(snap.Errors, "内存采样不可用")
	}
	if interfaces, e := gnet.InterfacesWithContext(ctx); e == nil {
		for _, i := range interfaces {
			for _, a := range i.Addrs {
				if prefix, err := netip.ParsePrefix(a.Addr); err == nil && prefix.Addr().IsGlobalUnicast() && !prefix.Addr().IsLoopback() {
					inv.IPs = append(inv.IPs, a.Addr)
				}
			}
		}
	}
	parts, e := disk.PartitionsWithContext(ctx, false)
	if e != nil {
		snap.Errors = append(snap.Errors, "磁盘分区不可用")
	}
	seen := map[string]bool{}
	for _, p := range parts {
		if slices.Contains([]string{"devfs", "proc", "sysfs", "tmpfs", "devtmpfs", "squashfs", "autofs", "cgroup", "cgroup2", "debugfs", "tracefs", "securityfs"}, p.Fstype) {
			continue
		}
		if len(snap.Disks) >= 16 {
			break
		}
		if seen[p.Mountpoint] {
			continue
		}
		seen[p.Mountpoint] = true
		usage, e := disk.UsageWithContext(ctx, p.Mountpoint)
		if e != nil || usage.Total == 0 {
			continue
		}
		snap.Disks = append(snap.Disks, DiskInfo{p.Mountpoint, p.Fstype, usage.Used, usage.Total, usage.UsedPercent})
	}
	if counters, e := gnet.IOCountersWithContext(ctx, true); e == nil {
		current := &netSample{at: time.Now()}
		for _, v := range counters {
			if strings.HasPrefix(v.Name, "lo") {
				continue
			}
			current.in += v.BytesRecv
			current.out += v.BytesSent
		}
		if prev := c.previousNet; prev != nil && current.in >= prev.in && current.out >= prev.out {
			dt := current.at.Sub(prev.at).Seconds()
			vin := float64(current.in-prev.in) / dt
			vout := float64(current.out-prev.out) / dt
			snap.NetIn = &vin
			snap.NetOut = &vout
		}
		c.previousNet = current
	} else {
		snap.Errors = append(snap.Errors, "网络流量不可用")
	}

	return Report{Protocol: 1, AgentVersion: "0.2.0", CollectedAt: time.Now().UnixMilli(), Hostname: info.Hostname, Inventory: inv, Snapshot: snap}, nil
}
