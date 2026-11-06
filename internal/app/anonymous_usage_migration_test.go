package app

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestAnonymousUsageMigrationPreservesConsentIdentityAndActivity(t *testing.T) {
	central, _, _, _ := fakeUsageConsole(t)
	s, c, _ := usageFixture(t, central.URL)
	enableUsage(t, c)
	before := usageStateForTest(t, s)
	day := time.Now().UTC().Format("2006-01-02")
	if _, err := s.Store.DB.Exec(`ALTER TABLE usage_settings ADD COLUMN account_login TEXT NOT NULL DEFAULT '';
		UPDATE usage_settings SET account_login='legacy-console-account';
		DELETE FROM schema_migrations WHERE version>=11;
		ALTER TABLE monitors DROP COLUMN certificate_expires_at;
		ALTER TABLE check_results DROP COLUMN certificate_expires_at;
		INSERT INTO usage_activity(day) VALUES(?)`, day); err != nil {
		t.Fatal(err)
	}
	var notice string
	var consent, activated int64
	if err := s.Store.DB.QueryRow("SELECT notice_version,consent_at,activated_at FROM usage_settings").Scan(&notice, &consent, &activated); err != nil {
		t.Fatal(err)
	}
	dir := s.Store.Dir
	s.Store.Close()
	upgraded, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	after, err := readUsageState(context.Background(), upgraded.DB)
	if err != nil || after != before {
		t.Fatalf("anonymous migration changed identity, credential or reporting state: %v", err)
	}
	var columns, days, schema int
	if err = upgraded.DB.QueryRow("SELECT count(*) FROM pragma_table_info('usage_settings') WHERE name='account_login'").Scan(&columns); err != nil {
		t.Fatal(err)
	}
	upgraded.DB.QueryRow("SELECT count(*) FROM usage_activity WHERE day=?", day).Scan(&days)
	upgraded.DB.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&schema)
	var nextNotice string
	var nextConsent, nextActivated int64
	upgraded.DB.QueryRow("SELECT notice_version,consent_at,activated_at FROM usage_settings").Scan(&nextNotice, &nextConsent, &nextActivated)
	if columns != 0 || days != 1 || schema != currentSchemaVersion || nextNotice != notice || nextConsent != consent || nextActivated != activated {
		t.Fatal("migration retained the account snapshot or changed existing consent/activity")
	}
}

func TestAnonymousUsageMigrationFailureRollsBackOnRealSQLite(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(`ALTER TABLE usage_settings ADD COLUMN account_login TEXT NOT NULL DEFAULT '';
		UPDATE usage_settings SET account_login='legacy-account';
		DELETE FROM schema_migrations WHERE version>=11;
		ALTER TABLE monitors DROP COLUMN certificate_expires_at;
		ALTER TABLE check_results DROP COLUMN certificate_expires_at;
		CREATE TRIGGER reject_anonymous BEFORE INSERT ON schema_migrations WHEN NEW.version=11
		BEGIN SELECT RAISE(ABORT,'test anonymous migration rollback'); END;`); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if unexpected, err := OpenStore(dir); err == nil {
		unexpected.Close()
		t.Fatal("rejected migration unexpectedly succeeded")
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	var schema int
	var account string
	if err = db.QueryRow("SELECT account_login FROM usage_settings").Scan(&account); err != nil {
		t.Fatal("failed migration partially dropped the previous schema")
	}
	db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&schema)
	if schema != 10 || account != "legacy-account" {
		t.Fatal("migration rollback did not preserve the previous data")
	}
	if _, err = db.Exec("DROP TRIGGER reject_anonymous"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	recovered, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	recovered.Close()
}
