package hoststats

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

func fakeDependencies() collectorDependencies {
	return collectorDependencies{
		now: func() time.Time { return time.Unix(100, 0) },
		hostInfo: func(context.Context) (*host.InfoStat, error) {
			return &host.InfoStat{Hostname: "test-host", Platform: "test", PlatformVersion: "1", Uptime: 42}, nil
		},
		cpuInfo:    func(context.Context) ([]cpu.InfoStat, error) { return []cpu.InfoStat{{ModelName: "fake CPU"}}, nil },
		cpuPercent: func(context.Context, time.Duration, bool) ([]float64, error) { return []float64{12}, nil },
		memory: func(context.Context) (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 100, Used: 30, UsedPercent: 30}, nil
		},
		interfaces: func(context.Context) (gnet.InterfaceStatList, error) {
			return gnet.InterfaceStatList{{Addrs: []gnet.InterfaceAddr{{Addr: "127.0.0.1/8"}, {Addr: "10.0.0.2/24"}, {Addr: "fe80::1/64"}, {Addr: "invalid"}}}}, nil
		},
		partitions: func(context.Context, bool) ([]disk.PartitionStat, error) { return nil, nil },
		usage: func(context.Context, string) (*disk.UsageStat, error) {
			return &disk.UsageStat{Total: 100, Used: 50, UsedPercent: 50}, nil
		},
		counters: func(context.Context, bool) ([]gnet.IOCountersStat, error) {
			return []gnet.IOCountersStat{{Name: "en0", BytesRecv: 100, BytesSent: 200}, {Name: "lo0", BytesRecv: 10000, BytesSent: 20000}}, nil
		},
	}
}

func TestCollectorOperatingSystemProductNameAndVersion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform string
		version  string
		kernel   string
		wantOS   string
	}{
		{"macOS product version", "darwin", "27.0.1", "27.0.0", "macOS 27.0.1"},
		{"Darwin case", "Darwin", "15.7", "24.6.0", "macOS 15.7"},
		{"macOS version unavailable", "darwin", "", "27.0.0", "macOS"},
		{"Linux distribution", "ubuntu", "24.04", "6.8.0-90-generic", "ubuntu 24.04"},
		{"Windows product", "Microsoft Windows 11 Pro", "10.0.26100", "10.0.26100.4652", "Microsoft Windows 11 Pro 10.0.26100"},
		{"FreeBSD", "freebsd", "14.3-RELEASE", "14.3-RELEASE-p1", "freebsd 14.3-RELEASE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := fakeDependencies()
			info := &host.InfoStat{OS: "unchanged", Platform: tc.platform, PlatformVersion: tc.version, KernelVersion: tc.kernel}
			deps.hostInfo = func(context.Context) (*host.InfoStat, error) { return info, nil }
			collector := Collector{dependencies: &deps}
			report, err := collector.Collect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if report.Inventory.OS != tc.wantOS || report.Inventory.Kernel != tc.kernel {
				t.Fatalf("product/kernel display: got %q / %q, want %q / %q", report.Inventory.OS, report.Inventory.Kernel, tc.wantOS, tc.kernel)
			}
			if info.OS != "unchanged" || info.Platform != tc.platform || info.PlatformVersion != tc.version || info.KernelVersion != tc.kernel {
				t.Fatalf("source platform information was modified: %+v", info)
			}
			if report.Protocol != 1 || report.Inventory.Arch != runtime.GOARCH {
				t.Fatalf("report protocol or architecture changed: %+v", report)
			}
		})
	}
}

func TestCollectorDiskFilteringLimitAndPartialErrors(t *testing.T) {
	deps := fakeDependencies()
	calls := map[string]int{}
	deps.partitions = func(context.Context, bool) ([]disk.PartitionStat, error) {
		out := []disk.PartitionStat{{Mountpoint: "/proc", Fstype: "proc"}, {Mountpoint: "/missing", Fstype: "ext4"}, {Mountpoint: "/empty", Fstype: "ext4"}, {Mountpoint: "/disk0", Fstype: "ext4"}}
		for i := 0; i < 30; i++ {
			out = append(out, disk.PartitionStat{Mountpoint: fmt.Sprintf("/disk%d", i), Fstype: "ext4"})
		}
		return out, nil
	}
	deps.usage = func(_ context.Context, path string) (*disk.UsageStat, error) {
		calls[path]++
		if path == "/missing" {
			return nil, errors.New("unavailable")
		}
		if path == "/empty" {
			return &disk.UsageStat{}, nil
		}
		return &disk.UsageStat{Total: 100, Used: 50, UsedPercent: 50}, nil
	}
	collector := Collector{dependencies: &deps}
	report, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Snapshot.Disks) != 16 || calls["/proc"] != 0 || calls["/disk0"] != 1 || calls["/disk16"] != 0 {
		t.Fatalf("partitions: %d calls=%v", len(report.Snapshot.Disks), calls)
	}
	if report.AgentVersion != buildinfo.Version || report.CollectedAt != 100000 || report.Hostname != "test-host" || len(report.Inventory.IPs) != 1 || report.Inventory.IPs[0] != "10.0.0.2/24" {
		t.Fatalf("inventory: %+v", report)
	}
	deps.cpuPercent = func(context.Context, time.Duration, bool) ([]float64, error) { return nil, errors.New("cpu") }
	deps.memory = func(context.Context) (*mem.VirtualMemoryStat, error) { return nil, errors.New("memory") }
	deps.partitions = func(context.Context, bool) ([]disk.PartitionStat, error) { return nil, errors.New("partitions") }
	deps.counters = func(context.Context, bool) ([]gnet.IOCountersStat, error) { return nil, errors.New("network") }
	report, err = collector.Collect(context.Background())
	if err != nil || len(report.Snapshot.Errors) != 4 || report.Snapshot.CPU != nil || report.Snapshot.Memory != nil || report.Snapshot.NetIn != nil {
		t.Fatalf("partial metrics: %+v %v", report, err)
	}
	deps.hostInfo = func(context.Context) (*host.InfoStat, error) { return nil, errors.New("host unavailable") }
	if _, err = collector.Collect(context.Background()); err == nil {
		t.Fatal("missing host info accepted")
	}
}
func TestCollectorNetworkDeltasClockAndCounterReset(t *testing.T) {
	deps := fakeDependencies()
	at := time.Unix(100, 0)
	counter := uint64(100)
	deps.now = func() time.Time { return at }
	deps.counters = func(context.Context, bool) ([]gnet.IOCountersStat, error) {
		return []gnet.IOCountersStat{{Name: "en0", BytesRecv: counter, BytesSent: counter * 2}}, nil
	}
	collector := Collector{dependencies: &deps}
	report, _ := collector.Collect(context.Background())
	if report.Snapshot.NetIn != nil {
		t.Fatal("first sample is a rate")
	}
	counter = 200
	at = at.Add(10 * time.Second)
	report, _ = collector.Collect(context.Background())
	if report.Snapshot.NetIn == nil || *report.Snapshot.NetIn != 10 || *report.Snapshot.NetOut != 20 {
		t.Fatalf("rate: %+v", report.Snapshot)
	}
	for _, reset := range []struct {
		at      time.Time
		counter uint64
	}{{at, 300}, {at.Add(-time.Second), 400}, {at.Add(time.Second), 1}} {
		at, counter = reset.at, reset.counter
		report, _ = collector.Collect(context.Background())
		if report.Snapshot.NetIn != nil || report.Snapshot.NetOut != nil {
			t.Fatal("invalid time or reset produced a rate")
		}
	}
}
func TestCollectorConcurrentCalls(t *testing.T) {
	deps := fakeDependencies()
	collector := Collector{dependencies: &deps}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			if _, err := collector.Collect(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}
