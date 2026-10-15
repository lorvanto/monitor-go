package app

import (
	"database/sql"
	"fmt"
	"strings"
)

// Collection normally runs every 15 seconds. A gap longer than three periods
// breaks continuity; an anchor at/before the window start proves full coverage.
const hostSampleMaxGap int64 = 45000

type hostSample struct {
	at          int64
	cpu, memory sql.NullFloat64
}

func sustainedHostUsage(samples []hostSample, start, end int64, threshold int, memory bool) bool {
	anchor := -1
	for i, sample := range samples {
		if sample.at <= start {
			anchor = i
		}
	}
	if anchor < 0 || len(samples)-anchor < 2 || samples[len(samples)-1].at != end {
		return false
	}
	previous := samples[anchor].at
	for _, sample := range samples[anchor:] {
		value := sample.cpu
		if memory {
			value = sample.memory
		}
		if !value.Valid || value.Float64 < float64(threshold) || sample.at-previous > hostSampleMaxGap {
			return false
		}
		previous = sample.at
	}
	return true
}

// Called after inserting the current sample in the same transaction, for both
// local collection and remote reports. Unknown data never implies recovery.
func evaluateHostAlerts(tx *sql.Tx, id int64, hostname string, snap snapshot, at int64) error {
	settings, err := readSettings(tx)
	if err != nil {
		return err
	}
	start := at - int64(settings.HostAlertMinutes)*60000
	samples := []hostSample{}
	if settings.HostCPUPercent > 0 || settings.HostMemoryPercent > 0 {
		rows, err := tx.Query("SELECT at,cpu,memory FROM host_samples WHERE host_id=? AND at>=? AND at<=? ORDER BY at", id, start-hostSampleMaxGap, at)
		if err != nil {
			return err
		}
		for rows.Next() {
			var sample hostSample
			if err = rows.Scan(&sample.at, &sample.cpu, &sample.memory); err != nil {
				rows.Close()
				return err
			}
			samples = append(samples, sample)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	for _, rule := range []struct {
		metric, title string
		threshold     int
		value         *float64
		memory        bool
	}{
		{"cpu", "主机 CPU 使用率需要关注", settings.HostCPUPercent, snap.CPU, false},
		{"memory", "主机内存使用率需要关注", settings.HostMemoryPercent, snap.Memory, true},
	} {
		active := rule.threshold > 0 && rule.value != nil && *rule.value >= float64(rule.threshold)
		if rule.threshold > 0 && (rule.value == nil || (active && !sustainedHostUsage(samples, start, at, rule.threshold, rule.memory))) {
			continue
		}
		detail := "主机指标告警已关闭"
		if rule.threshold > 0 {
			detail = fmt.Sprintf("%s · %.1f%% / %d%% · %d min", strings.ToUpper(rule.metric), *rule.value, rule.threshold, settings.HostAlertMinutes)
		}
		if err = transitionAlert(tx, fmt.Sprintf("host:%d:%s", id, rule.metric), "host", id, hostname, rule.title, detail, "warning", active); err != nil {
			return err
		}
	}
	if settings.HostDiskPercent > 0 && len(snap.Disks) == 0 {
		return nil
	}
	details := []string{}
	if settings.HostDiskPercent > 0 {
		for _, disk := range snap.Disks {
			if disk.Percent >= float64(settings.HostDiskPercent) {
				details = append(details, fmt.Sprintf("%s 使用率 %.1f%%", disk.Path, disk.Percent))
			}
		}
	}
	detail := strings.Join(details, "；")
	if settings.HostDiskPercent == 0 {
		detail = "主机指标告警已关闭"
	}
	return transitionAlert(tx, fmt.Sprintf("host:%d:disk", id), "host", id, hostname, "主机磁盘空间需要关注", detail, "warning", len(details) > 0)
}
