package hoststats

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/shirou/gopsutil/v4/disk"
	gnet "github.com/shirou/gopsutil/v4/net"
)

func TestNormalizeDisksPreservesIndependentCapacitiesAndLegacyMounts(t *testing.T) {
	input := []DiskInfo{
		{Path: "/System/Volumes/VM", Filesystem: "apfs", Used: 900, Total: 1000, Percent: 90, CapacityID: "aaaaaaaa-1111-4111-8111-111111111111", CapacityKind: "apfs-container", MountPoints: []string{"/System/Volumes/VM"}},
		{Path: "/System/Volumes/Data", Filesystem: "apfs", Used: 800, Total: 1000, Percent: 80, CapacityID: "AAAAAAAA-1111-4111-8111-111111111111", CapacityKind: "apfs-container"},
		{Path: "/", Filesystem: "apfs", Used: 19, Total: 20, Percent: 95, CapacityID: "AAAAAAAA-1111-4111-8111-111111111111", CapacityKind: "apfs-container", MountPoints: []string{"/", "/System/Volumes/VM"}},
		{Path: "/Volumes/External", Filesystem: "apfs", Used: 19, Total: 20, Percent: 95, CapacityID: "BBBBBBBB-1111-4111-8111-111111111111", CapacityKind: "apfs-container"},
		{Path: "/System/Volumes/Data", Filesystem: "apfs", Used: 19, Total: 20, Percent: 95, CapacityID: "AAAAAAAA-1111-4111-8111-111111111111", CapacityKind: "apfs-quota"},
		{Path: "/legacy-1", Filesystem: "apfs", Used: 19, Total: 20, Percent: 95},
		{Path: "/legacy-2", Filesystem: "apfs", Used: 19, Total: 20, Percent: 95},
	}
	before := make([]DiskInfo, len(input))
	copy(before, input)
	for i := range before {
		before[i].MountPoints = append([]string(nil), input[i].MountPoints...)
	}
	got := NormalizeDisks(input)
	if len(got) != 5 || got[0].Path != "/" || got[0].Percent != 95 || got[0].Used != 19 || got[0].Total != 20 || !reflect.DeepEqual(got[0].MountPoints, []string{"/", "/System/Volumes/Data", "/System/Volumes/VM"}) {
		t.Fatalf("normalized: %+v", got)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("normalization modified its input")
	}
	got[0].MountPoints[0] = "changed"
	if !reflect.DeepEqual(input, before) {
		t.Fatal("normalized mount list aliases input")
	}
	for i, j := 0, len(input)-1; i < j; i, j = i+1, j-1 {
		input[i], input[j] = input[j], input[i]
	}
	for _, d := range NormalizeDisks(input) {
		if d.CapacityKind == "apfs-container" && d.CapacityID == "AAAAAAAA-1111-4111-8111-111111111111" && (d.Path != "/" || d.Percent != 95 || d.Used != 19 || d.Total != 20) {
			t.Fatalf("sample depends on order: %+v", d)
		}
	}
}

func TestNormalizeDisksKeepsAllMountsForCallerToValidate(t *testing.T) {
	var input []DiskInfo
	for i := 39; i >= 0; i-- {
		input = append(input, DiskInfo{Path: fmt.Sprintf("/mount-%02d", i), CapacityID: "container", CapacityKind: "apfs-container", Percent: 50, Used: 50, Total: 100})
	}
	got := NormalizeDisks(input)
	if len(got) != 1 || len(got[0].MountPoints) != 40 || got[0].Path != "/mount-00" {
		t.Fatalf("lost mount data: %+v", got)
	}
	input = []DiskInfo{{Path: "/b", CapacityID: "same", CapacityKind: "unknown"}, {Path: "/a", CapacityID: "same", CapacityKind: "unknown"}}
	if len(NormalizeDisks(input)) != 2 {
		t.Fatal("unsupported identity kind was merged")
	}
}

func TestCollectorDiskIncompleteForFailedInvalidAndTruncatedSamples(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage func(context.Context, string) (*disk.UsageStat, error)
	}{
		{"error", func(context.Context, string) (*disk.UsageStat, error) { return nil, errors.New("unavailable") }},
		{"nil", func(context.Context, string) (*disk.UsageStat, error) { return nil, nil }},
		{"zero", func(context.Context, string) (*disk.UsageStat, error) { return &disk.UsageStat{}, nil }},
		{"overfull", func(context.Context, string) (*disk.UsageStat, error) {
			return &disk.UsageStat{Total: 10, Used: 11, UsedPercent: 100}, nil
		}},
		{"nan", func(context.Context, string) (*disk.UsageStat, error) {
			return &disk.UsageStat{Total: 10, Used: 5, UsedPercent: math.NaN()}, nil
		}},
		{"infinite", func(context.Context, string) (*disk.UsageStat, error) {
			return &disk.UsageStat{Total: 10, Used: 5, UsedPercent: math.Inf(1)}, nil
		}},
		{"negative", func(context.Context, string) (*disk.UsageStat, error) {
			return &disk.UsageStat{Total: 10, Used: 5, UsedPercent: -1}, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := fakeDependencies()
			deps.partitions = func(context.Context, bool) ([]disk.PartitionStat, error) {
				return []disk.PartitionStat{{Mountpoint: "/", Fstype: "ext4"}}, nil
			}
			deps.usage = tc.usage
			snap := Snapshot{Disks: []DiskInfo{}, Errors: []string{}}
			collectDisks(context.Background(), deps, &snap)
			if !snap.DiskIncomplete || len(snap.Disks) != 0 || len(snap.Errors) != 1 {
				t.Fatalf("failed sample appeared healthy: %+v", snap)
			}
		})
	}
	deps := fakeDependencies()
	deps.partitions = func(context.Context, bool) ([]disk.PartitionStat, error) {
		return []disk.PartitionStat{{Mountpoint: "/", Fstype: "ext4"}}, errors.New("partial partition list")
	}
	snap := Snapshot{Disks: []DiskInfo{}, Errors: []string{}}
	collectDisks(context.Background(), deps, &snap)
	if !snap.DiskIncomplete || len(snap.Disks) != 1 {
		t.Fatalf("partial enumeration lost healthy sample or completeness: %+v", snap)
	}
}

func TestCollectorBoundsDiagnosticsAndMountPathsWithoutLosingOtherMetrics(t *testing.T) {
	deps := fakeDependencies()
	deps.partitions = func(context.Context, bool) ([]disk.PartitionStat, error) {
		out := []disk.PartitionStat{{Mountpoint: "/" + strings.Repeat("日", 200), Fstype: "ext4"}, {Mountpoint: "/invalid\x00path", Fstype: "ext4"}, {Mountpoint: "/invalid\xff", Fstype: "ext4"}}
		for i := 0; i < 25; i++ {
			out = append(out, disk.PartitionStat{Mountpoint: fmt.Sprintf("/%02d/", i) + strings.Repeat("日", 169), Fstype: "ext4"})
		}
		return out, nil
	}
	deps.usage = func(context.Context, string) (*disk.UsageStat, error) { return nil, errors.New("missing") }
	deps.counters = func(context.Context, bool) ([]gnet.IOCountersStat, error) { return nil, errors.New("network") }
	collector := Collector{dependencies: &deps}
	report, err := collector.Collect(context.Background())
	if err != nil || !report.Snapshot.DiskIncomplete || len(report.Snapshot.Errors) != 16 || report.Snapshot.CPU == nil || report.Snapshot.Memory == nil {
		t.Fatalf("partial metrics: %+v %v", report, err)
	}
	for _, message := range report.Snapshot.Errors {
		if len(message) > 512 || !utf8.ValidString(message) {
			t.Fatalf("invalid diagnostic: %q", message)
		}
	}
}
