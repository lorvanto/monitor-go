package app

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func certificateLegacyStore(t *testing.T, reject bool) string {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := s.encrypt("synthetic-monitor-secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`ALTER TABLE monitors DROP COLUMN certificate_expires_at;
		ALTER TABLE check_results DROP COLUMN certificate_expires_at;
		DELETE FROM schema_migrations WHERE version=12;
		INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds,secret,state,checked_at,next_check,failures,version)
		VALUES(1,'Certificate','SSL','example.com:443','group',60,10,?,'warning',1234,61234,7,4);
		INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,1234,'warning',12,'legacy certificate diagnostic');`, secret)
	if err != nil {
		t.Fatal(err)
	}
	if reject {
		_, err = s.DB.Exec(`CREATE TRIGGER reject_certificate_schema BEFORE INSERT ON schema_migrations WHEN NEW.version=12 BEGIN SELECT RAISE(ABORT,'test certificate migration rollback'); END`)
		if err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	return dir
}

func TestCertificateMigrationPreservesSchedulesAndHistory(t *testing.T) {
	dir := certificateLegacyStore(t, false)
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m, err := scanMonitor(s.DB.QueryRow("SELECT " + monitorColumns + " FROM monitors WHERE id=1"))
	if err != nil {
		t.Fatal(err)
	}
	secret, err := s.decrypt(m.Secret)
	if err != nil || secret != "synthetic-monitor-secret" || m.Interval != 60 || m.Checked != 1234 || m.NextCheck != 61234 || m.Version != 4 || m.State != "warning" || m.CertificateExpiresAt != 0 {
		t.Fatal("migration changed existing schedule or credential", m, err)
	}
	var count, expiry, schema int
	if err = s.DB.QueryRow("SELECT count(*),certificate_expires_at FROM check_results WHERE detail='legacy certificate diagnostic'").Scan(&count, &expiry); err != nil || count != 1 || expiry != 0 {
		t.Fatal("legacy history lost or date fabricated", count, expiry, err)
	}
	if err = s.DB.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&schema); err != nil || schema != 12 {
		t.Fatal("schema version", schema, err)
	}
	for _, table := range []string{"monitors", "check_results"} {
		if _, err = s.DB.Exec("UPDATE " + table + " SET certificate_expires_at=-1"); err == nil {
			t.Fatal("negative certificate time accepted", table)
		}
	}
	rows, err := s.DB.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() || rows.Err() != nil {
		t.Fatal("foreign key violation after migration", rows.Err())
	}
}

func TestCertificateMigrationRollsBackAndCanRetry(t *testing.T) {
	dir := certificateLegacyStore(t, true)
	if s, err := OpenStore(dir); err == nil {
		s.Close()
		t.Fatal("rejected migration succeeded")
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	var schema, columns, history int
	err = db.QueryRow(`SELECT (SELECT max(version) FROM schema_migrations),
		(SELECT count(*) FROM pragma_table_info('monitors') WHERE name='certificate_expires_at')+
		(SELECT count(*) FROM pragma_table_info('check_results') WHERE name='certificate_expires_at'),
		(SELECT count(*) FROM check_results)`).Scan(&schema, &columns, &history)
	if err != nil || schema != 11 || columns != 0 || history != 1 {
		t.Fatal("migration partially committed", schema, columns, history, err)
	}
	if _, err = db.Exec("DROP TRIGGER reject_certificate_schema"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal("migration retry failed", err)
	}
	s.Close()
}
