package app

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestReadPoolConnectionsRejectWritesAndCloseWithStore(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if store.DB.Stats().MaxOpenConnections != 1 || store.readDB.Stats().MaxOpenConnections != 2 {
		t.Fatal("unexpected pool limits")
	}
	connections := []*sql.Conn{}
	for i := 0; i < 2; i++ {
		conn, err := store.readDB.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, conn)
		var queryOnly, foreignKeys int
		if err = conn.QueryRowContext(ctx, "PRAGMA query_only").Scan(&queryOnly); err != nil || queryOnly != 1 {
			t.Fatalf("query_only: %d %v", queryOnly, err)
		}
		if err = conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
			t.Fatalf("foreign_keys: %d %v", foreignKeys, err)
		}
		if _, err = conn.ExecContext(ctx, "UPDATE system_settings SET instance_name='forbidden' WHERE id=1"); err == nil {
			t.Fatal("read pool wrote data")
		}
		// URI mode=ro remains a second barrier even if a caller disables query_only.
		if _, err = conn.ExecContext(ctx, "PRAGMA query_only=0"); err != nil {
			t.Fatal(err)
		}
		if _, err = conn.ExecContext(ctx, "UPDATE system_settings SET instance_name='forbidden' WHERE id=1"); err == nil {
			t.Fatal("URI read-only mode was not enforced")
		}
		if _, err = conn.ExecContext(ctx, "PRAGMA query_only=1"); err != nil {
			t.Fatal(err)
		}
	}
	for _, conn := range connections {
		conn.Close()
	}
	// Force a replacement connection; DSN pragmas must also protect new connections.
	store.readDB.SetMaxIdleConns(0)
	var protected int
	if err = store.readDB.QueryRowContext(ctx, "PRAGMA query_only").Scan(&protected); err != nil || protected != 1 {
		t.Fatal("replacement reader not protected", err)
	}
	readers := store.readDB
	store.Close()
	if err = readers.PingContext(ctx); err == nil {
		t.Fatal("readers survived store close")
	}
	reopened, err := OpenStore(dir)
	if err != nil {
		t.Fatal("instance lock not released", err)
	}
	reopened.Close()
}

func TestWriteTransactionReadsOwnWritesAndReadersSeeCommittedSnapshots(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Hold a read snapshot while a production write transaction commits.
	reader, err := store.readDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Rollback()
	var original string
	if err = reader.QueryRowContext(ctx, "SELECT instance_name FROM system_settings WHERE id=1").Scan(&original); err != nil {
		t.Fatal(err)
	}
	err = store.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE system_settings SET instance_name='committed name' WHERE id=1"); err != nil {
			return err
		}
		var own string
		if err := tx.QueryRow("SELECT instance_name FROM system_settings WHERE id=1").Scan(&own); err != nil {
			return err
		}
		if own != "committed name" {
			return errors.New("transaction did not read its own write")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot string
	if err = reader.QueryRowContext(ctx, "SELECT instance_name FROM system_settings WHERE id=1").Scan(&snapshot); err != nil || snapshot != original {
		t.Fatalf("reader lost snapshot: %q %v", snapshot, err)
	}
	if err = reader.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = store.reader().QueryRowContext(ctx, "SELECT instance_name FROM system_settings WHERE id=1").Scan(&snapshot); err != nil || snapshot != "committed name" {
		t.Fatal("new read missed commit", err)
	}
	rollback := errors.New("rollback")
	if err = store.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE system_settings SET instance_name='rolled back' WHERE id=1")
		if err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if err = store.reader().QueryRowContext(ctx, "SELECT instance_name FROM system_settings WHERE id=1").Scan(&snapshot); err != nil || snapshot != "committed name" {
		t.Fatal("rollback leaked to reader", err)
	}
}

func TestReadPoolPollingAndConcurrentWrites(t *testing.T) {
	s, _, _ := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustSQL(t, s, "INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) VALUES(1,'concurrent','HTTP','http://example.com','test',60,5)")
	var workers sync.WaitGroup
	failures := make(chan error, 5)
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 25; i++ {
				if _, err := s.monitors(ctx); err != nil {
					failures <- err
					return
				}
				if _, err := s.availability(ctx, 720, now(), ""); err != nil {
					failures <- err
					return
				}
			}
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		for i := 0; i < 50; i++ {
			if err := s.Store.write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec("INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,?,'healthy',10,'test')", now()-hourMillis)
				return err
			}); err != nil {
				failures <- err
				return
			}
			if err := s.rollupChecks(ctx, now()); err != nil {
				failures <- err
				return
			}
		}
	}()
	workers.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	report, err := s.availability(ctx, 720, now(), "")
	if err != nil || report.Samples != 50 || *report.Uptime != 100 {
		t.Fatalf("concurrent rollup: %+v %v", report, err)
	}
}
