package hoststats

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
)

const (
	fixtureContainerA     = "AAAAAAAA-1111-4111-8111-111111111111"
	fixtureContainerB     = "BBBBBBBB-2222-4222-8222-222222222222"
	fixtureVolumeData     = "DDDDDDDD-1111-4111-8111-111111111111"
	fixtureVolumeSystem   = "EEEEEEEE-1111-4111-8111-111111111111"
	fixtureVolumeVM       = "FFFFFFFF-1111-4111-8111-111111111111"
	fixtureVolumeExternal = "CCCCCCCC-2222-4222-8222-222222222222"
)

func fixtureAPFSVolume(device, id string, used, quota uint64) string {
	return fmt.Sprintf(`<dict><key>DeviceIdentifier</key><string>%s</string><key>APFSVolumeUUID</key><string>%s</string><key>CapacityInUse</key><integer>%d</integer><key>CapacityQuota</key><integer>%d</integer><key>CapacityReserve</key><integer>0</integer></dict>`, device, id, used, quota)
}

func fixtureAPFSContainer(reference, id string, total, free uint64, volumes ...string) string {
	return fmt.Sprintf(`<dict><key>ContainerReference</key><string>%s</string><key>APFSContainerUUID</key><string>%s</string><key>CapacityCeiling</key><integer>%d</integer><key>CapacityFree</key><integer>%d</integer><key>Volumes</key><array>%s</array></dict>`, reference, id, total, free, strings.Join(volumes, ""))
}

func fixtureAPFSList(containers ...string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Containers</key><array>` + strings.Join(containers, "") + `</array></dict></plist>`)
}

func fixtureAPFSInfo(reference, device, uuid string, snapshot bool) []byte {
	flag := "false"
	if snapshot {
		flag = "true"
	}
	return []byte(fmt.Sprintf(`<plist version="1.0"><dict><key>APFSContainerReference</key><string>%s</string><key>DeviceIdentifier</key><string>%s</string><key>FilesystemType</key><string>apfs</string><key>APFSSnapshot</key><%s/><key>VolumeUUID</key><string>%s</string></dict></plist>`, reference, device, flag, uuid))
}

func TestCollectorAPFSSharedContainersSnapshotsAndIndependentQuota(t *testing.T) {
	deps := fakeDependencies()
	deps.partitions = func(context.Context, bool) ([]disk.PartitionStat, error) {
		return []disk.PartitionStat{
			{Device: "/dev/disk3s6", Mountpoint: "/System/Volumes/VM", Fstype: "apfs"},
			{Device: "/dev/disk3s3s1", Mountpoint: "/", Fstype: "apfs"},
			{Device: "/dev/disk3s1", Mountpoint: "/System/Volumes/Data", Fstype: "apfs"},
			{Device: "/dev/disk4s1", Mountpoint: "/Volumes/External", Fstype: "apfs"},
		}, nil
	}
	list := fixtureAPFSList(
		fixtureAPFSContainer("disk3", fixtureContainerA, 1000, 100,
			fixtureAPFSVolume("disk3s1", fixtureVolumeData, 600, 0),
			fixtureAPFSVolume("disk3s3", fixtureVolumeSystem, 100, 0),
			fixtureAPFSVolume("disk3s6", fixtureVolumeVM, 200, 0)),
		fixtureAPFSContainer("disk4", fixtureContainerB, 1000, 100, fixtureAPFSVolume("disk4s1", fixtureVolumeExternal, 900, 850)),
	)
	calls := map[string]int{}
	deps.apfsCommand = func(_ context.Context, args ...string) ([]byte, error) {
		key := strings.Join(args, " ")
		calls[key]++
		switch key {
		case "apfs list -plist":
			return list, nil
		case "info -plist /":
			return fixtureAPFSInfo("disk3", "disk3s3s1", "99999999-9999-4999-8999-999999999999", true), nil
		default:
			t.Fatalf("unexpected command: %v", args)
			return nil, nil
		}
	}
	deps.usage = func(context.Context, string) (*disk.UsageStat, error) {
		t.Fatal("identified APFS space must use container metadata, not repeated mount statfs")
		return nil, nil
	}
	snap := Snapshot{Disks: []DiskInfo{}, Errors: []string{}}
	collectDisks(context.Background(), deps, &snap)
	if snap.DiskIncomplete || len(snap.Errors) != 0 || len(snap.Disks) != 3 || calls["apfs list -plist"] != 1 || calls["info -plist /"] != 1 {
		t.Fatalf("APFS collection: %+v calls=%v", snap, calls)
	}
	shared := snap.Disks[0]
	if shared.Path != "/" || shared.CapacityID != fixtureContainerA || shared.CapacityKind != "apfs-container" || shared.Percent != 90 || shared.Used != 900 || shared.Total != 1000 || !reflect.DeepEqual(shared.MountPoints, []string{"/", "/System/Volumes/Data", "/System/Volumes/VM"}) {
		t.Fatalf("shared capacity: %+v", shared)
	}
	if snap.Disks[1].CapacityID != fixtureContainerB || snap.Disks[1].Percent != shared.Percent || snap.Disks[2].CapacityKind != "apfs-quota" || snap.Disks[2].CapacityID != fixtureVolumeExternal || snap.Disks[2].Used != 900 || snap.Disks[2].Total != 850 || snap.Disks[2].Percent != 100 {
		t.Fatalf("independent container and over-quota volume: %+v", snap.Disks)
	}
}

func TestCollectorAPFSRequeriesTopologyAndDoesNotReuseDeviceNumbers(t *testing.T) {
	deps := fakeDependencies()
	deps.partitions = func(context.Context, bool) ([]disk.PartitionStat, error) {
		return []disk.PartitionStat{{Device: "/dev/disk3s1", Mountpoint: "/Volumes/Disk", Fstype: "apfs"}}, nil
	}
	call := 0
	deps.apfsCommand = func(context.Context, ...string) ([]byte, error) {
		call++
		id := fixtureContainerA
		free := uint64(100)
		if call == 2 {
			id = fixtureContainerB
			free = 500
		}
		return fixtureAPFSList(fixtureAPFSContainer("disk3", id, 1000, free, fixtureAPFSVolume("disk3s1", fixtureVolumeData, 100, 0))), nil
	}
	collector := Collector{dependencies: &deps}
	one, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	two, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if call != 2 || one.Snapshot.Disks[0].CapacityID != fixtureContainerA || two.Snapshot.Disks[0].CapacityID != fixtureContainerB || two.Snapshot.Disks[0].Percent != 50 {
		t.Fatalf("stale APFS device mapping reused: one=%+v two=%+v calls=%d", one.Snapshot, two.Snapshot, call)
	}
}

func TestCollectorAPFSFailuresKeepOriginalMountsAndMarkIncomplete(t *testing.T) {
	valid := fixtureAPFSList(fixtureAPFSContainer("disk3", fixtureContainerA, 1000, 100, fixtureAPFSVolume("disk3s1", fixtureVolumeData, 600, 0)))
	for _, tc := range []struct {
		name       string
		raw        []byte
		commandErr error
	}{
		{"command failure", nil, errors.New("diskutil unavailable")},
		{"deadline", nil, context.DeadlineExceeded},
		{"malformed", []byte(`<plist><dict>`), nil},
		{"missing container UUID", []byte(strings.ReplaceAll(string(valid), fixtureContainerA, "")), nil},
		{"free exceeds capacity", fixtureAPFSList(fixtureAPFSContainer("disk3", fixtureContainerA, 100, 101, fixtureAPFSVolume("disk3s1", fixtureVolumeData, 60, 0))), nil},
		{"used exceeds container", fixtureAPFSList(fixtureAPFSContainer("disk3", fixtureContainerA, 100, 1, fixtureAPFSVolume("disk3s1", fixtureVolumeData, 101, 0))), nil},
		{"negative quota", []byte(strings.ReplaceAll(string(valid), `<key>CapacityQuota</key><integer>0</integer>`, `<key>CapacityQuota</key><integer>-1</integer>`)), nil},
		{"duplicate plist key", []byte(strings.Replace(string(valid), `<key>CapacityFree</key>`, `<key>CapacityCeiling</key>`, 1)), nil},
		{"unexpected trailing root", append(append([]byte(nil), valid...), []byte(`<plist><dict/></plist>`)...), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := fakeDependencies()
			deps.partitions = func(context.Context, bool) ([]disk.PartitionStat, error) {
				return []disk.PartitionStat{{Device: "/dev/disk3s1", Mountpoint: "/one", Fstype: "apfs"}, {Device: "/dev/disk3s2", Mountpoint: "/two", Fstype: "apfs"}}, nil
			}
			deps.apfsCommand = func(context.Context, ...string) ([]byte, error) { return tc.raw, tc.commandErr }
			snap := Snapshot{Disks: []DiskInfo{}, Errors: []string{}}
			collectDisks(context.Background(), deps, &snap)
			if !snap.DiskIncomplete || len(snap.Errors) == 0 || len(snap.Disks) != 2 || snap.Disks[0].CapacityID != "" || snap.Disks[1].CapacityID != "" || snap.Disks[0].Percent != 50 || snap.Disks[1].Percent != 50 {
				t.Fatalf("fallback lost original mounts or pretended health: %+v", snap)
			}
		})
	}
}

func TestAPFSInfoMustConfirmSnapshotOrActualVolumeIdentity(t *testing.T) {
	list := fixtureAPFSList(fixtureAPFSContainer("disk3", fixtureContainerA, 1000, 100,
		fixtureAPFSVolume("disk3s1", fixtureVolumeData, 600, 650),
		fixtureAPFSVolume("disk3s10", fixtureVolumeSystem, 100, 120)))
	topology, err := parseAPFSTopology(list)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, device, reference, infoDevice, volumeUUID string
		snapshot, wantError                             bool
		wantQuota                                       string
	}{
		{"confirmed snapshot preserves base quota", "disk3s10s1", "disk3", "disk3s10s1", "", true, false, fixtureVolumeSystem},
		{"not a snapshot", "disk3s10s1", "disk3", "disk3s10s1", "", false, true, ""},
		{"prefix collision", "disk3s100", "disk3", "disk3s100", "", true, true, ""},
		{"device changed during collection", "disk3s10s1", "disk3", "disk3s1s1", "", true, true, ""},
		{"unknown container", "disk3s10s1", "disk9", "disk3s10s1", "", true, true, ""},
		{"volume UUID resolves unknown device", "snapshot-device", "disk3", "snapshot-device", fixtureVolumeData, false, false, fixtureVolumeData},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := func(context.Context, ...string) ([]byte, error) {
				return fixtureAPFSInfo(tc.reference, tc.infoDevice, tc.volumeUUID, tc.snapshot), nil
			}
			disks, err := topology.disksForMount(context.Background(), disk.PartitionStat{Device: "/dev/" + tc.device, Mountpoint: "/", Fstype: "apfs"}, command)
			if (err != nil) != tc.wantError {
				t.Fatalf("identity result: %+v %v", disks, err)
			}
			if !tc.wantError && (len(disks) != 2 || disks[1].CapacityID != tc.wantQuota) {
				t.Fatalf("wrong quota mapping: %+v", disks)
			}
		})
	}
	for _, invalid := range [][]byte{[]byte(`not XML`), []byte(`<plist><array/></plist>`)} {
		_, err := topology.disksForMount(context.Background(), disk.PartitionStat{Device: "/dev/unknown", Mountpoint: "/", Fstype: "apfs"}, func(context.Context, ...string) ([]byte, error) { return invalid, nil })
		if err == nil {
			t.Fatal("invalid info plist accepted")
		}
	}
}

func TestCollectorAPFSLimitsCountCapacitiesInsteadOfAliases(t *testing.T) {
	deps := fakeDependencies()
	list := fixtureAPFSList(fixtureAPFSContainer("disk3", fixtureContainerA, 1000, 100, fixtureAPFSVolume("disk3s1", fixtureVolumeData, 600, 0)))
	deps.apfsCommand = func(context.Context, ...string) ([]byte, error) { return list, nil }
	var parts []disk.PartitionStat
	for i := 0; i < 15; i++ {
		parts = append(parts, disk.PartitionStat{Mountpoint: fmt.Sprintf("/normal-%02d", i), Fstype: "ext4"})
	}
	parts = append(parts, disk.PartitionStat{Mountpoint: "/System/Volumes/Data", Device: "/dev/disk3s1", Fstype: "apfs"}, disk.PartitionStat{Mountpoint: "/alias", Device: "/dev/disk3s1", Fstype: "apfs"})
	deps.partitions = func(context.Context, bool) ([]disk.PartitionStat, error) { return parts, nil }
	snap := Snapshot{Disks: []DiskInfo{}, Errors: []string{}}
	collectDisks(context.Background(), deps, &snap)
	if snap.DiskIncomplete || len(snap.Disks) != 16 || !reflect.DeepEqual(snap.Disks[15].MountPoints, []string{"/System/Volumes/Data", "/alias"}) {
		t.Fatalf("alias falsely counted as new disk: %+v", snap)
	}
	parts = append(parts, disk.PartitionStat{Mountpoint: "/one-too-many", Fstype: "ext4"})
	snap = Snapshot{Disks: []DiskInfo{}, Errors: []string{}}
	collectDisks(context.Background(), deps, &snap)
	if !snap.DiskIncomplete || len(snap.Disks) != 16 {
		t.Fatalf("truncated sample marked complete: %+v", snap)
	}
	parts = nil
	for i := 0; i < 40; i++ {
		parts = append(parts, disk.PartitionStat{Mountpoint: fmt.Sprintf("/alias-%02d", i), Device: "/dev/disk3s1", Fstype: "apfs"})
	}
	snap = Snapshot{Disks: []DiskInfo{}, Errors: []string{}}
	collectDisks(context.Background(), deps, &snap)
	if !snap.DiskIncomplete || len(snap.Disks) != 1 || len(snap.Disks[0].MountPoints) != 32 {
		t.Fatalf("unbounded mount union: %+v", snap)
	}
}
