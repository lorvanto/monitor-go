package app

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// Run explicitly with -run '^$' -bench BenchmarkConcurrentPollingWrites -benchtime=1x.
// Uses the production polling query and write transaction against a real WAL file.
func BenchmarkConcurrentPollingWrites(b *testing.B) {
	for _, size := range []int{0, 2, 4} {
		b.Run(fmt.Sprintf("readers-%d", size), func(b *testing.B) { benchmarkPollingWrites(b, size) })
	}
}
func benchmarkPollingWrites(b *testing.B, readers int) {
	for iteration := 0; iteration < b.N; iteration++ {
		b.StopTimer()
		store, err := OpenStore(b.TempDir())
		if err != nil {
			b.Fatal(err)
		}
		if readers == 0 {
			store.readDB.Close()
			store.readDB = nil
		} else {
			store.readDB.SetMaxOpenConns(readers)
			store.readDB.SetMaxIdleConns(readers)
		}
		s := &Server{Store: store}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		at := now()
		err = store.write(ctx, func(tx *sql.Tx) error {
			if _, err := tx.Exec(`WITH RECURSIVE ids(id) AS (VALUES(1) UNION ALL SELECT id+1 FROM ids WHERE id<500)
INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds,enabled) SELECT id,'Monitor '||id,'HTTP','https://example.com','benchmark',60,5,0 FROM ids`); err != nil {
				return err
			}
			_, err := tx.Exec(`WITH RECURSIVE samples(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM samples WHERE n<1439)
INSERT INTO check_results(monitor_id,at,state,latency,detail) SELECT m.id,?-n*60000,CASE WHEN n%10=0 THEN 'failed' ELSE 'healthy' END,10+n%100,'Synthetic benchmark result' FROM monitors m CROSS JOIN samples`, at)
			return err
		})
		if err == nil {
			err = s.rollupChecks(ctx, at)
		}
		if err != nil {
			cancel()
			store.Close()
			b.Fatal(err)
		}
		if _, err = s.monitors(ctx); err != nil {
			cancel()
			store.Close()
			b.Fatal(err)
		}
		var mutex sync.Mutex
		var reads, writes []float64
		errors := make(chan error, 5)
		start := make(chan struct{})
		var workers sync.WaitGroup
		before := store.DB.Stats()
		b.StartTimer()
		begun := time.Now()
		for worker := 0; worker < 4; worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				for n := 0; n < 5; n++ {
					at := time.Now()
					items, err := s.monitors(ctx)
					elapsed := float64(time.Since(at).Microseconds()) / 1000
					if err != nil {
						errors <- err
						return
					}
					if len(items) != 500 {
						errors <- fmt.Errorf("monitors=%d", len(items))
						return
					}
					mutex.Lock()
					reads = append(reads, elapsed)
					mutex.Unlock()
				}
			}()
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for n := 0; n < 80; n++ {
				at := time.Now()
				err := store.write(ctx, func(tx *sql.Tx) error {
					if _, err := tx.Exec("INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,?,'healthy',12,'Concurrent write')", now()); err != nil {
						return err
					}
					_, err := tx.Exec("UPDATE monitors SET checked_at=?,latency=12 WHERE id=1", now())
					return err
				})
				elapsed := float64(time.Since(at).Microseconds()) / 1000
				if err != nil {
					errors <- err
					return
				}
				mutex.Lock()
				writes = append(writes, elapsed)
				mutex.Unlock()
				time.Sleep(20 * time.Millisecond)
			}
		}()
		close(start)
		workers.Wait()
		wall := time.Since(begun)
		b.StopTimer()
		close(errors)
		for err := range errors {
			if err != nil {
				cancel()
				store.Close()
				b.Fatal(err)
			}
		}
		after := store.DB.Stats()
		cancel()
		store.Close()
		sort.Float64s(reads)
		sort.Float64s(writes)
		percentile := func(v []float64, p int) float64 { return v[(len(v)-1)*p/100] }
		b.ReportMetric(percentile(reads, 50), "read-p50-ms")
		b.ReportMetric(percentile(reads, 95), "read-p95-ms")
		b.ReportMetric(percentile(writes, 50), "write-p50-ms")
		b.ReportMetric(percentile(writes, 95), "write-p95-ms")
		b.ReportMetric(wall.Seconds(), "wall-s")
		b.ReportMetric(float64(after.WaitCount-before.WaitCount), "writer-waits")
		b.ReportMetric((after.WaitDuration - before.WaitDuration).Seconds(), "writer-wait-s")
	}
}
