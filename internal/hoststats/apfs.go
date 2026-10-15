package hoststats

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
)

type apfsCommandFunc func(context.Context, ...string) ([]byte, error)

type apfsContainer struct {
	id, reference string
	total, free   uint64
	volumes       []*apfsVolume
}

type apfsVolume struct {
	id, device  string
	used, quota uint64
	container   *apfsContainer
}

type apfsTopology struct {
	containers map[string]*apfsContainer
	volumes    map[string]*apfsVolume
	volumeIDs  map[string]*apfsVolume
}

// diskutil's plist output is parsed as XML rather than localized display text.
// No external entity resolution or new plist dependency is required.
type plistNode struct {
	XMLName  xml.Name
	Text     string      `xml:",chardata"`
	Children []plistNode `xml:",any"`
}

func parsePlist(raw []byte) (map[string]plistNode, error) {
	if len(raw) == 0 || len(raw) > 4<<20 {
		return nil, errors.New("invalid plist size")
	}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var root plistNode
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("invalid plist XML: %w", err)
	}
	if root.XMLName.Local != "plist" || len(root.Children) != 1 || strings.TrimSpace(root.Text) != "" {
		return nil, errors.New("invalid plist root")
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid plist suffix: %w", err)
		}
		switch token := token.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return nil, errors.New("unexpected plist suffix")
			}
		case xml.Comment:
		default:
			return nil, errors.New("unexpected plist suffix")
		}
	}
	return root.Children[0].dictionary()
}

func (node plistNode) dictionary() (map[string]plistNode, error) {
	if node.XMLName.Local != "dict" || len(node.Children)%2 != 0 || strings.TrimSpace(node.Text) != "" {
		return nil, errors.New("invalid plist dictionary")
	}
	out := make(map[string]plistNode, len(node.Children)/2)
	for index := 0; index < len(node.Children); index += 2 {
		key := node.Children[index]
		if key.XMLName.Local != "key" || len(key.Children) != 0 || key.Text == "" {
			return nil, errors.New("invalid plist dictionary key")
		}
		if _, exists := out[key.Text]; exists {
			return nil, errors.New("duplicate plist dictionary key")
		}
		out[key.Text] = node.Children[index+1]
	}
	return out, nil
}

func (node plistNode) array() ([]plistNode, error) {
	if node.XMLName.Local != "array" || strings.TrimSpace(node.Text) != "" {
		return nil, errors.New("invalid plist array")
	}
	return node.Children, nil
}

func (node plistNode) string() string {
	if node.XMLName.Local != "string" || len(node.Children) != 0 {
		return ""
	}
	return strings.TrimSpace(node.Text)
}

func (node plistNode) integer() (uint64, error) {
	if node.XMLName.Local != "integer" || len(node.Children) != 0 {
		return 0, errors.New("missing or invalid plist integer")
	}
	value, err := strconv.ParseUint(strings.TrimSpace(node.Text), 10, 64)
	if err != nil {
		return 0, errors.New("invalid plist integer")
	}
	return value, nil
}

func canonicalUUID(value string) string {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return ""
	}
	if _, err := hex.DecodeString(strings.ReplaceAll(value, "-", "")); err != nil {
		return ""
	}
	return strings.ToUpper(value)
}

func parseAPFSTopology(raw []byte) (*apfsTopology, error) {
	root, err := parsePlist(raw)
	if err != nil {
		return nil, err
	}
	containers, err := root["Containers"].array()
	if err != nil {
		return nil, err
	}
	topology := &apfsTopology{containers: make(map[string]*apfsContainer), volumes: make(map[string]*apfsVolume), volumeIDs: make(map[string]*apfsVolume)}
	containerIDs := make(map[string]bool)
	for _, node := range containers {
		dict, err := node.dictionary()
		if err != nil {
			return nil, err
		}
		container := &apfsContainer{id: canonicalUUID(dict["APFSContainerUUID"].string()), reference: dict["ContainerReference"].string()}
		container.total, err = dict["CapacityCeiling"].integer()
		if err != nil {
			return nil, err
		}
		container.free, err = dict["CapacityFree"].integer()
		if err != nil || container.id == "" || container.reference == "" || container.total == 0 || container.free > container.total || topology.containers[container.reference] != nil || containerIDs[container.id] {
			return nil, errors.New("invalid or duplicate APFS container")
		}
		topology.containers[container.reference] = container
		containerIDs[container.id] = true
		volumes, err := dict["Volumes"].array()
		if err != nil {
			return nil, err
		}
		for _, node := range volumes {
			volumeData, err := node.dictionary()
			if err != nil {
				return nil, err
			}
			volume := &apfsVolume{id: canonicalUUID(volumeData["APFSVolumeUUID"].string()), device: volumeData["DeviceIdentifier"].string(), container: container}
			volume.used, err = volumeData["CapacityInUse"].integer()
			if err != nil || volume.used > container.total {
				return nil, errors.New("invalid APFS volume usage")
			}
			// diskutil omits an unset quota on some system versions.
			if quota, ok := volumeData["CapacityQuota"]; ok {
				volume.quota, err = quota.integer()
				if err != nil {
					return nil, err
				}
			}
			if volume.id == "" || volume.device == "" || topology.volumes[volume.device] != nil || topology.volumeIDs[volume.id] != nil {
				return nil, errors.New("invalid or duplicate APFS volume")
			}
			topology.volumes[volume.device] = volume
			topology.volumeIDs[volume.id] = volume
			container.volumes = append(container.volumes, volume)
		}
	}
	return topology, nil
}

func (topology *apfsTopology) disksForMount(ctx context.Context, part disk.PartitionStat, command apfsCommandFunc) ([]DiskInfo, error) {
	device := strings.TrimPrefix(part.Device, "/dev/")
	volume := topology.volumes[device]
	var container *apfsContainer
	if volume != nil {
		container = volume.container
	} else {
		if command == nil {
			return nil, errors.New("APFS mount identity unavailable")
		}
		raw, err := command(ctx, "info", "-plist", part.Mountpoint)
		if err != nil {
			return nil, err
		}
		info, err := parsePlist(raw)
		if err != nil {
			return nil, err
		}
		container = topology.containers[info["APFSContainerReference"].string()]
		infoDevice := info["DeviceIdentifier"].string()
		if container == nil || (device != "" && infoDevice != device) || (info["FilesystemType"].string() != "" && info["FilesystemType"].string() != "apfs") {
			return nil, errors.New("APFS mount/container identity mismatch")
		}
		volume = topology.volumes[infoDevice]
		if volume == nil {
			volume = topology.volumeIDs[canonicalUUID(info["APFSVolumeUUID"].string())]
		}
		if volume == nil {
			volume = topology.volumeIDs[canonicalUUID(info["VolumeUUID"].string())]
		}
		if volume == nil && info["APFSSnapshot"].XMLName.Local == "true" {
			// The system root is a mounted snapshot (e.g. disk3s3s1), not
			// a separate entry in apfs list. Confirmed snapshot metadata
			// and a complete numeric suffix establish its listed base volume.
			for _, candidate := range container.volumes {
				if isAPFSSnapshotDevice(infoDevice, candidate.device) {
					if volume != nil {
						return nil, errors.New("ambiguous APFS snapshot volume")
					}
					volume = candidate
				}
			}
		}
		if volume != nil && volume.container != container {
			return nil, errors.New("APFS volume/container identity mismatch")
		}
		if volume == nil {
			// Without the actual volume identity, its quota cannot be ruled
			// out; keep the fallback sample and mark collection incomplete.
			return nil, errors.New("APFS volume identity unavailable")
		}
	}
	used := container.total - container.free
	out := []DiskInfo{{Path: part.Mountpoint, Filesystem: "apfs", Used: used, Total: container.total, Percent: float64(used) / float64(container.total) * 100, CapacityID: container.id, CapacityKind: "apfs-container", MountPoints: []string{part.Mountpoint}}}
	if volume.quota > 0 {
		// APFS can already exceed a quota after it is reduced. Cap pressure
		// at 100% and retain the bytes instead of discarding the full disk.
		percent := float64(volume.used) / float64(volume.quota) * 100
		if percent > 100 {
			percent = 100
		}
		out = append(out, DiskInfo{Path: part.Mountpoint, Filesystem: "apfs", Used: volume.used, Total: volume.quota, Percent: percent, CapacityID: volume.id, CapacityKind: "apfs-quota", MountPoints: []string{part.Mountpoint}})
	}
	return out, nil
}

func isAPFSSnapshotDevice(device, volume string) bool {
	suffix, ok := strings.CutPrefix(device, volume+"s")
	if !ok || suffix == "" {
		return false
	}
	for _, character := range suffix {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
