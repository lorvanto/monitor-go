package hoststats

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	maxDisks           = 16
	maxDiskMountPoints = 32
)

// NormalizeDisks merges only reports with an explicit, supported capacity identity.
// Equal capacities or percentages do not establish that two disks share space.
// Conflicting samples retain the highest utilization and its corresponding bytes.
// The returned slice and its mount lists do not alias the caller's slices.
func NormalizeDisks(disks []DiskInfo) []DiskInfo {
	out := make([]DiskInfo, 0, len(disks))
	groups := make(map[string]int)
	for _, original := range disks {
		d := original
		d.MountPoints = append([]string(nil), d.MountPoints...)
		if d.CapacityID == "" || (d.CapacityKind != "apfs-container" && d.CapacityKind != "apfs-quota") {
			out = append(out, d)
			continue
		}
		if id := canonicalUUID(d.CapacityID); id != "" {
			d.CapacityID = id
		}
		d.MountPoints = canonicalMountPoints(append(d.MountPoints, d.Path))
		if len(d.MountPoints) > 0 {
			d.Path = d.MountPoints[0]
		}
		key := d.CapacityKind + "\x00" + d.CapacityID
		if index, ok := groups[key]; ok {
			previous := &out[index]
			mounts := canonicalMountPoints(append(previous.MountPoints, d.MountPoints...))
			if higherDiskPressure(d, *previous) {
				*previous = d
			}
			previous.MountPoints = mounts
			if len(mounts) > 0 {
				previous.Path = mounts[0]
			}
			continue
		}
		groups[key] = len(out)
		out = append(out, d)
	}
	return out
}

func higherDiskPressure(candidate, previous DiskInfo) bool {
	if math.IsNaN(previous.Percent) {
		return !math.IsNaN(candidate.Percent)
	}
	if math.IsNaN(candidate.Percent) {
		return false
	}
	if candidate.Percent != previous.Percent {
		return candidate.Percent > previous.Percent
	}
	// A deterministic tie break prevents iteration order from changing byte counts.
	if candidate.Used != previous.Used {
		return candidate.Used > previous.Used
	}
	return candidate.Total < previous.Total
}

func mountOrder(path string) int {
	switch path {
	case "/":
		return 0
	case "/System/Volumes/Data":
		return 1
	default:
		return 2
	}
}

func canonicalMountPoints(paths []string) []string {
	unique := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != "" && !unique[path] {
			unique[path] = true
			out = append(out, path)
		}
	}
	slices.SortFunc(out, func(a, b string) int {
		if order := mountOrder(a) - mountOrder(b); order != 0 {
			return order
		}
		return strings.Compare(a, b)
	})
	return out
}

func collectDisks(ctx context.Context, deps collectorDependencies, snap *Snapshot) {
	parts, err := deps.partitions(ctx, false)
	if err != nil {
		snap.DiskIncomplete = true
		appendCollectionError(snap, "磁盘分区不可用")
	}
	var topology *apfsTopology
	if deps.apfsCommand != nil {
		for _, part := range parts {
			if strings.EqualFold(part.Fstype, "apfs") {
				raw, commandErr := deps.apfsCommand(ctx, "apfs", "list", "-plist")
				if commandErr == nil {
					topology, commandErr = parseAPFSTopology(raw)
				}
				if commandErr != nil {
					snap.DiskIncomplete = true
					appendCollectionError(snap, "APFS container metadata unavailable: "+commandErr.Error())
				}
				break
			}
		}
	}
	seen := make(map[string]bool)
	for _, part := range parts {
		if slices.Contains([]string{"devfs", "proc", "sysfs", "tmpfs", "devtmpfs", "squashfs", "autofs", "cgroup", "cgroup2", "debugfs", "tracefs", "securityfs"}, part.Fstype) || seen[part.Mountpoint] {
			continue
		}
		seen[part.Mountpoint] = true
		if len(part.Mountpoint) == 0 || len(part.Mountpoint) > 512 || !utf8.ValidString(part.Mountpoint) || strings.ContainsRune(part.Mountpoint, 0) {
			snap.DiskIncomplete = true
			appendCollectionError(snap, "Disk mount path is invalid or exceeds 512 bytes")
			continue
		}
		if topology != nil && strings.EqualFold(part.Fstype, "apfs") {
			disks, resolveErr := topology.disksForMount(ctx, part, deps.apfsCommand)
			if resolveErr == nil {
				snap.Disks = NormalizeDisks(append(snap.Disks, disks...))
				if len(snap.Disks) > maxDisks {
					snap.Disks = snap.Disks[:maxDisks]
					snap.DiskIncomplete = true
					appendCollectionError(snap, "Disk collection truncated at 16 capacity entries")
					break
				}
				continue
			}
			snap.DiskIncomplete = true
			appendCollectionError(snap, fmt.Sprintf("APFS capacity unavailable for %s: %v", part.Mountpoint, resolveErr))
		}
		if len(snap.Disks) >= maxDisks {
			snap.DiskIncomplete = true
			appendCollectionError(snap, "Disk collection truncated at 16 capacity entries")
			break
		}
		usage, usageErr := deps.usage(ctx, part.Mountpoint)
		if usageErr != nil || usage == nil || usage.Total == 0 || usage.Used > usage.Total || math.IsNaN(usage.UsedPercent) || math.IsInf(usage.UsedPercent, 0) || usage.UsedPercent < 0 || usage.UsedPercent > 100 {
			snap.DiskIncomplete = true
			appendCollectionError(snap, "Disk usage unavailable for "+part.Mountpoint)
			continue
		}
		snap.Disks = append(snap.Disks, DiskInfo{Path: part.Mountpoint, Filesystem: part.Fstype, Used: usage.Used, Total: usage.Total, Percent: usage.UsedPercent})
	}
	for index := range snap.Disks {
		if len(snap.Disks[index].MountPoints) > maxDiskMountPoints {
			snap.Disks[index].MountPoints = snap.Disks[index].MountPoints[:maxDiskMountPoints]
			snap.DiskIncomplete = true
			appendCollectionError(snap, "APFS mount list truncated at 32 paths")
		}
	}
}

func appendCollectionError(snap *Snapshot, message string) {
	if len(snap.Errors) >= 16 {
		return
	}
	message = strings.ToValidUTF8(message, "�")
	if len(message) > 512 {
		message = message[:512]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
	}
	snap.Errors = append(snap.Errors, message)
}
