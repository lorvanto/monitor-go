package app

import (
	"context"
	"database/sql"
	"testing"
)

func TestMigrationSixThresholdConstraints(t *testing.T) {
	dir, db := versionFourDatabase(t)
	if _, err := db.Exec(migration5); err != nil {
		t.Fatal(err)
	}
	db.Close()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	settings, err := readSettings(store.DB)
	if err != nil || settings.HostCPUPercent != 90 || settings.HostMemoryPercent != 90 || settings.HostDiskPercent != 85 || settings.HostAlertMinutes != 5 {
		t.Fatalf("defaults: %+v %v", settings, err)
	}
	for _, statement := range []string{
		"UPDATE system_settings SET host_cpu_percent=-1", "UPDATE system_settings SET host_cpu_percent=101",
		"UPDATE system_settings SET host_memory_percent=101", "UPDATE system_settings SET host_disk_percent=-1",
		"UPDATE system_settings SET host_alert_minutes=0", "UPDATE system_settings SET host_alert_minutes=61",
	} {
		if _, err := store.DB.Exec(statement); err == nil {
			t.Fatalf("accepted invalid threshold: %s", statement)
		}
	}
	var version int
	if err := store.DB.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version); err != nil || version != currentSchemaVersion {
		t.Fatalf("version %d: %v", version, err)
	}
}

func TestHostThresholdContinuityRecoveryAndDisable(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		low, missing, gap, short bool
		want                     int
	}{
		{name: "sustained", want: 2}, {name: "middle dip", low: true}, {name: "missing value", missing: true}, {name: "gap", gap: true}, {name: "short window", short: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := OpenStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			at := now()
			value := 95.0
			err = store.write(context.Background(), func(tx *sql.Tx) error {
				if _, err := tx.Exec("INSERT INTO hosts(id,hostname) VALUES(1,'test-host')"); err != nil {
					return err
				}
				if _, err := tx.Exec("UPDATE system_settings SET host_alert_minutes=1"); err != nil {
					return err
				}
				for i := 0; i <= 4; i++ {
					if (tc.gap && (i == 1 || i == 2 || i == 3)) || (tc.short && i == 0) {
						continue
					}
					var v any = value
					if tc.low && i == 2 {
						v = 50.0
					}
					if tc.missing && i == 2 {
						v = nil
					}
					if _, err := tx.Exec("INSERT INTO host_samples(host_id,at,cpu,memory) VALUES(1,?,?,?)", at-60000+int64(i)*15000, v, v); err != nil {
						return err
					}
				}
				return evaluateHostAlerts(tx, 1, "test-host", snapshot{CPU: &value, Memory: &value}, at)
			})
			if err != nil {
				t.Fatal(err)
			}
			var active int
			store.DB.QueryRow("SELECT count(*) FROM alerts WHERE state<>'resolved'").Scan(&active)
			if active != tc.want {
				t.Fatalf("active=%d want %d", active, tc.want)
			}
			if tc.want == 0 {
				return
			}
			// A missing reading must leave active incidents intact.
			err = store.write(context.Background(), func(tx *sql.Tx) error { return evaluateHostAlerts(tx, 1, "test-host", snapshot{}, at+15000) })
			if err != nil {
				t.Fatal(err)
			}
			store.DB.QueryRow("SELECT count(*) FROM alerts WHERE state<>'resolved'").Scan(&active)
			if active != 2 {
				t.Fatal("missing sample resolved alert")
			}
			// Low CPU recovers immediately; disabling memory resolves on the next report.
			low := 20.0
			err = store.write(context.Background(), func(tx *sql.Tx) error {
				if _, err := tx.Exec("UPDATE system_settings SET host_memory_percent=0"); err != nil {
					return err
				}
				return evaluateHostAlerts(tx, 1, "test-host", snapshot{CPU: &low}, at+30000)
			})
			if err != nil {
				t.Fatal(err)
			}
			store.DB.QueryRow("SELECT count(*) FROM alerts WHERE state<>'resolved'").Scan(&active)
			if active != 0 {
				t.Fatal("recovery/disable did not resolve")
			}
		})
	}
}

func TestHostThresholdSettingsApplyNextReport(t *testing.T) {
	s, _, c := fixture(t)
	settings := decodeValue[Settings](t, c.call("GET", "/settings", nil, 200))
	settings.HostDiskPercent = 70
	c.call("PATCH", "/settings", settings, 200)
	c.call("PATCH", "/settings", settings, 409)
	apply := func() {
		t.Helper()
		if err := s.Store.write(context.Background(), func(tx *sql.Tx) error {
			if _, err := tx.Exec("INSERT OR IGNORE INTO hosts(id,hostname) VALUES(1,'test-host')"); err != nil {
				return err
			}
			return evaluateHostAlerts(tx, 1, "test-host", snapshot{Disks: []diskInfo{{Path: "/", Percent: 80}}}, now())
		}); err != nil {
			t.Fatal(err)
		}
	}
	apply()
	var active int
	s.Store.DB.QueryRow("SELECT count(*) FROM alerts WHERE fingerprint='host:1:disk' AND state<>'resolved'").Scan(&active)
	if active != 1 {
		t.Fatal("new threshold not applied")
	}
	settings = decodeValue[Settings](t, c.call("GET", "/settings", nil, 200))
	settings.HostDiskPercent = 0
	c.call("PATCH", "/settings", settings, 200)
	apply()
	s.Store.DB.QueryRow("SELECT count(*) FROM alerts WHERE fingerprint='host:1:disk' AND state<>'resolved'").Scan(&active)
	if active != 0 {
		t.Fatal("disabled disk threshold not applied")
	}
	settings.Version++
	settings.HostCPUPercent = 101
	c.call("PATCH", "/settings", settings, 422)
}
