package app

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func versionFourDatabase(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;" + schema + migration2 + migration3 + migration4); err != nil {
		t.Fatal(err)
	}
	return dir, db
}

func TestMigrationFiveQueueIndexAndHistory(t *testing.T) {
	dir, old := versionFourDatabase(t)
	if _, err := old.Exec(`
		INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) VALUES
		(1,'first','TCP','example.com:443','test',60,5),
		(2,'second','TCP','example.com:443','test',60,5);
		WITH RECURSIVE history(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM history WHERE n<10000)
		INSERT INTO jobs(id,monitor_id,state,created_at,finished_at) SELECT n,1,'succeeded',1,2 FROM history;
		INSERT INTO jobs(id,monitor_id,state,created_at) VALUES(10001,1,'queued',3),(10002,2,'queued',4);
		INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,1,'healthy',12,'preserved');
	`); err != nil {
		t.Fatal(err)
	}
	old.Close()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version, indexes, jobs, results int
	for query, dest := range map[string]*int{
		"SELECT max(version) FROM schema_migrations":                                  &version,
		"SELECT count(*) FROM sqlite_master WHERE type='index' AND name='jobs_queue'": &indexes,
		"SELECT count(*) FROM jobs":                                                   &jobs,
		"SELECT count(*) FROM check_results WHERE detail='preserved'":                 &results,
	} {
		if err := s.DB.QueryRow(query).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if version != currentSchemaVersion || indexes != 1 || jobs != 10002 || results != 1 {
		t.Fatalf("migration changed history or missed index: version=%d indexes=%d jobs=%d results=%d", version, indexes, jobs, results)
	}
	rows, err := s.DB.Query("EXPLAIN QUERY PLAN " + nextQueuedJobQuery)
	if err != nil {
		t.Fatal(err)
	}
	var plans []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plans = append(plans, detail)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || !strings.Contains(strings.Join(plans, "; "), "jobs_queue") {
		t.Fatalf("queue query does not use its partial index: %v, %v", plans, err)
	}
	t.Logf("queue query plan: %s", strings.Join(plans, "; "))
	for _, want := range []int{10001, 10002} {
		var got int
		if err := s.DB.QueryRow(nextQueuedJobQuery).Scan(&got); err != nil || got != want {
			t.Fatalf("queue order: got %d want %d, %v", got, want, err)
		}
		if _, err := s.DB.Exec("UPDATE jobs SET state='succeeded',finished_at=5 WHERE id=?", got); err != nil {
			t.Fatal(err)
		}
	}
	var id int
	if err := s.DB.QueryRow(nextQueuedJobQuery).Scan(&id); err != sql.ErrNoRows {
		t.Fatalf("drained queue: %v", err)
	}
	s.Close()
	reopened, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen after migration: %v", err)
	}
	reopened.Close()
}

func TestMigrationFiveFailureRollsBackAndReleasesLock(t *testing.T) {
	dir, old := versionFourDatabase(t)
	if _, err := old.Exec(`CREATE TRIGGER reject_five BEFORE INSERT ON schema_migrations WHEN NEW.version=5 BEGIN SELECT RAISE(ABORT,'test migration failure'); END`); err != nil {
		t.Fatal(err)
	}
	old.Close()
	if s, err := OpenStore(dir); err == nil {
		s.Close()
		t.Fatal("migration should fail")
	}
	check, err := sql.Open("sqlite", filepath.Join(dir, "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version, indexes int
	if err := check.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := check.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='jobs_queue'").Scan(&indexes); err != nil {
		t.Fatal(err)
	}
	if version != 4 || indexes != 0 {
		t.Fatalf("failed migration was not rolled back: version=%d indexes=%d", version, indexes)
	}
	if _, err := check.Exec("DROP TRIGGER reject_five"); err != nil {
		t.Fatal(err)
	}
	check.Close()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("retry after failed migration: %v", err)
	}
	s.Close()
}
