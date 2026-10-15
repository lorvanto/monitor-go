package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/lorvanto/monitor-go/internal/hoststats"
	"strings"
	"testing"
)

func capacityDisk(path, id, kind string, percent float64) diskInfo {
	return diskInfo{Path: path, Filesystem: "apfs", Used: uint64(percent), Total: 100, Percent: percent, CapacityID: id, CapacityKind: kind}
}

func TestDiskAlertCapacityGroupingPreservesIncidentAndRecovery(t *testing.T) {
	s, _, c := fixture(t)
	mustSQL(t, s, "INSERT INTO hosts(id,hostname) VALUES(1,'disk-test')")
	mustSQL(t, s, "UPDATE system_settings SET host_cpu_percent=0,host_memory_percent=0,host_disk_percent=80")
	makeChannel(t, c, "https://example.invalid/disposable-test")
	apply := func(disks []diskInfo, incomplete bool) {
		t.Helper()
		if err := s.Store.write(context.Background(), func(tx *sql.Tx) error {
			return evaluateHostAlerts(tx, 1, "disk-test", snapshot{Disks: disks, DiskIncomplete: incomplete}, now())
		}); err != nil {
			t.Fatal(err)
		}
	}
	incident := func() Alert {
		t.Helper()
		alerts := decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200))
		if len(alerts) == 0 {
			t.Fatal("missing disk incident")
		}
		return alerts[0]
	}
	count := func(query string, want int) {
		t.Helper()
		var got int
		if err := s.Store.DB.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: got %d want %d: %v", query, got, want, err)
		}
	}
	// Legacy Agents without capacity identities must not be deduplicated by
	// coincidentally equal percentages or byte counts.
	legacy := []diskInfo{{Path: "/", Filesystem: "apfs", Used: 80, Total: 100, Percent: 80}, {Path: "/System/Volumes/Data", Filesystem: "apfs", Used: 80, Total: 100, Percent: 80}}
	apply(legacy, false)
	original := incident()
	if strings.Count(original.Description, "使用率 80.0%") != 2 {
		t.Fatalf("legacy mounts were guessed to be one disk: %s", original.Description)
	}
	c.call("POST", "/alerts/"+idText(original.ID)+"/acknowledgements", map[string]any{"version": original.Version}, 200)
	c.call("POST", "/alerts/"+idText(original.ID)+"/silences", map[string]any{"version": original.Version + 1, "minutes": 60}, 200)
	acknowledged := incident()
	grouped := []diskInfo{
		capacityDisk("/", "CONTAINER-A", "apfs-container", 80),
		capacityDisk("/System/Volumes/Data", "CONTAINER-A", "apfs-container", 80),
		capacityDisk("/Volumes/Independent", "CONTAINER-B", "apfs-container", 80),
		capacityDisk("/", "VOLUME-QUOTA", "apfs-quota", 80),
	}
	apply(grouped, false)
	apply(grouped, false)
	updated := incident()
	if updated.ID != original.ID || updated.State != "acknowledged" || updated.Version != acknowledged.Version || updated.SilenceUntil != acknowledged.SilenceUntil {
		t.Fatalf("grouping changed incident ownership or silence: before=%+v after=%+v", acknowledged, updated)
	}
	if strings.Count(updated.Description, "使用率 80.0%") != 3 || strings.Count(updated.Description, "APFS 容器 / 使用率") != 1 || !strings.Contains(updated.Description, "APFS 容器 /Volumes/Independent 使用率") || !strings.Contains(updated.Description, "APFS 卷配额 / 使用率") {
		t.Fatalf("incorrect capacity grouping or scope labels: %s", updated.Description)
	}
	count("SELECT count(*) FROM alerts WHERE fingerprint='host:1:disk'", 1)
	count("SELECT count(*) FROM notification_deliveries WHERE event='opened'", 1)
	count("SELECT count(*) FROM alert_activity WHERE message LIKE '达到阈值，创建告警：%'", 1)
	// A partial healthy subset, or no data at all, cannot resolve the existing
	// incident. This also retains the last confirmed diagnostic description.
	apply([]diskInfo{capacityDisk("/Volumes/Independent", "CONTAINER-B", "apfs-container", 20)}, true)
	apply(nil, false)
	unknown := incident()
	if unknown.ID != updated.ID || unknown.State != updated.State || unknown.SilenceUntil != updated.SilenceUntil || unknown.Description != updated.Description {
		t.Fatalf("missing disk readings altered incident: %+v", unknown)
	}
	// A confirmed high reading remains actionable even when another disk fails.
	apply([]diskInfo{capacityDisk("/", "CONTAINER-A", "apfs-container", 90)}, true)
	if got := incident(); got.State != "acknowledged" || !strings.Contains(got.Description, "90.0%") {
		t.Fatalf("confirmed high partial report ignored: %+v", got)
	}
	apply([]diskInfo{capacityDisk("/", "CONTAINER-A", "apfs-container", 20)}, false)
	if got := incident(); got.ID != original.ID || got.State != "resolved" {
		t.Fatalf("complete healthy report did not resolve: %+v", got)
	}
	count("SELECT count(*) FROM notification_deliveries WHERE event='resolved'", 1)
	// A fresh partial high report opens a new incident; disabling the rule
	// resolves it even if its next report has no readable disks.
	apply(grouped, true)
	if got := incident(); got.ID == original.ID || got.State != "open" {
		t.Fatalf("new high reading did not open incident: %+v", got)
	}
	mustSQL(t, s, "UPDATE system_settings SET host_disk_percent=0")
	apply(nil, true)
	count("SELECT count(*) FROM alerts WHERE fingerprint='host:1:disk' AND state<>'resolved'", 0)
	count("SELECT count(*) FROM notification_deliveries WHERE event='opened'", 2)
}

func TestAgentDiskIdentityValidationAndNormalizedSQLiteSnapshot(t *testing.T) {
	s, ts, c := fixture(t)
	registered := decodeValue[map[string]any](t, c.call("POST", "/hosts/agents", map[string]string{"alias": "disposable-disk-agent"}, 200))
	credential := randomToken()
	agentRequest(t, ts.URL, "enroll", "", map[string]any{"token": registered["token"], "credential": credential}, 200)
	base := capacityDisk("/", "CONTAINER-A", "apfs-container", 90)
	invalid := []struct {
		name string
		edit func(*diskInfo)
	}{
		{"identity without kind", func(d *diskInfo) { d.CapacityKind = "" }},
		{"kind without identity", func(d *diskInfo) { d.CapacityID = "" }},
		{"unknown kind", func(d *diskInfo) { d.CapacityKind = "filesystem" }},
		{"non APFS identity", func(d *diskInfo) { d.Filesystem = "ext4" }},
		{"long identity", func(d *diskInfo) { d.CapacityID = strings.Repeat("a", 181) }},
		{"control identity", func(d *diskInfo) { d.CapacityID = "CONTAINER\tA" }},
		{"padded identity", func(d *diskInfo) { d.CapacityID = " CONTAINER-A" }},
		{"zero capacity", func(d *diskInfo) { d.Used, d.Total = 0, 0 }},
		{"over quota with low percentage", func(d *diskInfo) { d.CapacityKind, d.Used = "apfs-quota", 101 }},
		{"empty path", func(d *diskInfo) { d.Path = "" }},
		{"NUL path", func(d *diskInfo) { d.Path = "/\x00" }},
		{"long mount", func(d *diskInfo) { d.MountPoints = []string{strings.Repeat("x", 513)} }},
		{"empty mount", func(d *diskInfo) { d.MountPoints = []string{""} }},
		{"NUL mount", func(d *diskInfo) { d.MountPoints = []string{"/\x00"} }},
		{"too many mounts", func(d *diskInfo) { d.MountPoints = make([]string, 33) }},
		{"mounts without identity", func(d *diskInfo) { d.CapacityID, d.CapacityKind, d.MountPoints = "", "", []string{"/"} }},
	}
	newReport := func(disks []diskInfo) hoststats.Report {
		return hoststats.Report{Protocol: 1, ReportID: randomToken(), CollectedAt: now(), Hostname: "disposable-disk-agent", Inventory: inventory{OS: "macOS"}, Snapshot: snapshot{Disks: disks}}
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			disk := base
			tc.edit(&disk)
			agentRequest(t, ts.URL, "reports", credential, newReport([]diskInfo{disk}), 422)
		})
	}
	// Per-row limits must also apply after a group merges its mount lists.
	one, two := base, base
	for i := 0; i < 17; i++ {
		one.MountPoints = append(one.MountPoints, fmt.Sprintf("/first/%d", i))
		two.MountPoints = append(two.MountPoints, fmt.Sprintf("/second/%d", i))
	}
	agentRequest(t, ts.URL, "reports", credential, newReport([]diskInfo{one, two}), 422)
	// UUID case variants still refer to one capacity. Canonicalizing them must
	// not bypass the group's mount limit or persist an oversized snapshot.
	one.CapacityID = "ABCDEF12-3456-4789-ABCD-0123456789AB"
	two.CapacityID = strings.ToLower(one.CapacityID)
	agentRequest(t, ts.URL, "reports", credential, newReport([]diskInfo{one, two}), 422)
	var samples int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM host_samples").Scan(&samples); err != nil || samples != 0 {
		t.Fatalf("invalid disk report persisted: %d %v", samples, err)
	}
	legacy := diskInfo{Path: "/Volumes/Legacy", Filesystem: "apfs", Used: 90, Total: 100, Percent: 90}
	legacyOther := legacy
	legacyOther.Path = "/Volumes/LegacyOther"
	overQuota := capacityDisk("/Volumes/Quota", "VOLUME-OVER-QUOTA", "apfs-quota", 100)
	overQuota.Used = 120
	report := newReport([]diskInfo{
		base,
		capacityDisk("/System/Volumes/Data", "CONTAINER-A", "apfs-container", 90),
		capacityDisk("/Volumes/Independent", "CONTAINER-B", "apfs-container", 90),
		capacityDisk("/", "CONTAINER-A", "apfs-quota", 90),
		legacy, legacyOther, overQuota,
	})
	report.Snapshot.DiskIncomplete = true
	agentRequest(t, ts.URL, "reports", credential, report, 200)
	agentRequest(t, ts.URL, "reports", credential, report, 200)
	var stored string
	if err := s.Store.DB.QueryRow("SELECT snapshot FROM hosts WHERE id=2").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var snap snapshot
	if err := json.Unmarshal([]byte(stored), &snap); err != nil || len(snap.Disks) != 6 || !snap.DiskIncomplete {
		t.Fatalf("normalized SQLite snapshot: %s %v", stored, err)
	}
	var grouped diskInfo
	for _, disk := range snap.Disks {
		if disk.CapacityID == "CONTAINER-A" && disk.CapacityKind == "apfs-container" {
			grouped = disk
		}
	}
	if grouped.Path != "/" || strings.Join(grouped.MountPoints, ",") != "/,/System/Volumes/Data" {
		t.Fatalf("mount identities not retained: %+v", grouped)
	}
	hosts := decodeValue[[]Host](t, c.call("GET", "/hosts", nil, 200))
	if len(hosts) != 1 || string(hosts[0].Snapshot) != stored {
		t.Fatalf("host API does not return the stored normalized snapshot: %+v", hosts)
	}
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM host_samples WHERE host_id=2").Scan(&samples); err != nil || samples != 1 {
		t.Fatalf("retry duplicated disk sample: %d %v", samples, err)
	}
	var alerts int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM alerts WHERE fingerprint='host:2:disk' AND state<>'resolved'").Scan(&alerts); err != nil || alerts != 1 {
		t.Fatalf("normalized report did not produce one host incident: %d %v", alerts, err)
	}
}

func TestHostReaderSeesEffectiveDiskThresholdWithoutSettingsAccess(t *testing.T) {
	s, ts, c := fixture(t)
	mustSQL(t, s, "INSERT INTO hosts(id,hostname) VALUES(1,'local'),(2,'remote')")
	role := decodeValue[map[string]string](t, c.call("POST", "/roles", map[string]string{"name": "Host-only reader"}, 201))
	c.call("PATCH", "/roles/"+role["id"], map[string]any{"version": 1, "permissions": []string{"host:read"}}, 200)
	c.call("POST", "/users", userInput{Username: "hostonly", Name: "Host reader", Password: "Host-reader-test-123!", Roles: []string{role["id"]}}, 201)
	reader := client(t, ts.URL)
	reader.login("hostonly", "Host-reader-test-123!")
	for _, threshold := range []int{71, 0} {
		settings := decodeValue[Settings](t, c.call("GET", "/settings", nil, 200))
		settings.HostDiskPercent = threshold
		c.call("PATCH", "/settings", settings, 200)
		reader.call("GET", "/settings", nil, 403)
		raw := reader.call("GET", "/hosts", nil, 200)
		hosts := decodeValue[[]Host](t, raw)
		if len(hosts) != 2 {
			t.Fatalf("host reader lost hosts: %+v", hosts)
		}
		for _, host := range hosts {
			if host.DiskAlertPercent != threshold {
				t.Fatalf("host %d threshold=%d want %d", host.ID, host.DiskAlertPercent, threshold)
			}
		}
		if strings.Count(string(raw), fmt.Sprintf(`"diskAlertPercent":%d`, threshold)) != 2 || strings.Contains(string(raw), "uploadLimitMB") || strings.Contains(string(raw), "instanceName") {
			t.Fatalf("missing threshold or leaked settings: %s", raw)
		}
	}
}
