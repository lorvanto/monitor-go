package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// Opt-in: real HTTP, authentication and a disposable WAL database. Never uses
// the installation's data or sends probes to an external destination.
func TestPerformanceBaseline(t *testing.T) {
	if os.Getenv("MONITOR_PERF") != "1" {
		t.Skip("set MONITOR_PERF=1 to run the synthetic workload")
	}
	s, ts, c := fixture(t)
	t.Logf("HTTP address: %s; %s/%s %s; GOMAXPROCS=%d", ts.URL, runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.GOMAXPROCS(0))
	at := now()/hourMillis*hourMillis + hourMillis/2
	err := s.Store.write(context.Background(), func(tx *sql.Tx) error {
		queries := []struct {
			q    string
			args []any
		}{
			{`WITH RECURSIVE n(i) AS(VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<100) INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds,enabled) SELECT i,'Synthetic '||i,'HTTP','http://127.0.0.1','baseline',60,3,0 FROM n`, nil},
			{`WITH RECURSIVE n(i) AS(VALUES(0) UNION ALL SELECT i+1 FROM n WHERE i<10079) INSERT INTO check_results(monitor_id,at,state,latency,detail) SELECT m.id,?-i*60000,CASE WHEN i%20=0 THEN 'failed' ELSE 'healthy' END,10+i%100,'Synthetic sample' FROM monitors m CROSS JOIN n`, []any{at}},
			{`WITH RECURSIVE n(i) AS(VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<20) INSERT INTO hosts(id,hostname,alias,inventory,snapshot,last_seen) SELECT i,'host-'||i,'Synthetic host','{}','{}',? FROM n`, []any{at}},
			{`WITH RECURSIVE n(i) AS(VALUES(0) UNION ALL SELECT i+1 FROM n WHERE i<40319) INSERT INTO host_samples(host_id,at,cpu,memory,net_in,net_out) SELECT h.id,?-i*15000,20,40,1024,2048 FROM hosts h CROSS JOIN n`, []any{at}},
			{`WITH RECURSIVE n(i) AS(VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<100) INSERT INTO alerts(id,fingerprint,resource_kind,resource_id,resource,title,description,severity,opened_at) SELECT i,'synthetic:'||i,'monitor',i,'Synthetic monitor','Synthetic alert','Synthetic description','warning',? FROM n`, []any{at}},
			{`WITH RECURSIVE n(i) AS(VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<100) INSERT INTO alert_activity(alert_id,at,actor,message) SELECT a.id,?,'Synthetic actor','Synthetic activity' FROM alerts a CROSS JOIN n`, []any{at}},
		}
		for _, q := range queries {
			if _, e := tx.Exec(q.q, q.args...); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.rollupChecks(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer upstream.Close()
	mustSQL(t, s, "UPDATE monitors SET enabled=1,target=?,failure_threshold=1 WHERE id=1", upstream.URL)
	paths := []string{"/monitors", "/alerts", "/overview?hours=24", "/hosts/1/metrics?hours=24"}
	if os.Getenv("MONITOR_PERF_POLL") == "1" {
		paths = []string{"/poll?page=overview", "/overview?hours=24", "/hosts/1/metrics?hours=24"}
	}
	if extra := os.Getenv("MONITOR_PERF_EXTRA"); extra != "" {
		paths = append(paths, extra)
	}
	type sample struct {
		MS    float64
		Bytes int
	}
	results := map[string][]sample{}
	var mu sync.Mutex
	request := func(path string) error {
		begun := time.Now()
		res, e := c.c.Get(ts.URL + "/api/v1" + path)
		if e != nil {
			return e
		}
		raw, e := io.ReadAll(res.Body)
		res.Body.Close()
		if e != nil {
			return e
		}
		if res.StatusCode != 200 {
			return fmt.Errorf("%s: %d", path, res.StatusCode)
		}
		mu.Lock()
		results[path] = append(results[path], sample{float64(time.Since(begun).Microseconds()) / 1000, len(raw)})
		mu.Unlock()
		return nil
	}
	for _, p := range paths {
		if err = request(p); err != nil {
			t.Fatal(err)
		}
	}
	results = map[string][]sample{}
	runtime.GC()
	proc, _ := process.NewProcess(int32(os.Getpid()))
	cpuBefore, _ := proc.Times()
	var heapBefore runtime.MemStats
	runtime.ReadMemStats(&heapBefore)
	stop := make(chan struct{})
	sampleDone := make(chan struct{})
	var peakRSS uint64
	go func() {
		defer close(sampleDone)
		tick := time.NewTicker(25 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if m, e := proc.MemoryInfo(); e == nil {
					peakRSS = max(peakRSS, m.RSS)
				}
			}
		}
	}()
	before := s.Store.DB.Stats()
	begun := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 5; n++ {
				for _, p := range paths {
					if e := request(p); e != nil {
						errs <- e
						return
					}
				}
			}
		}()
	}
	var writes []float64
	wg.Add(1)
	go func() {
		defer wg.Done()
		for n := 0; n < 40; n++ {
			started := time.Now()
			job, e := s.enqueue(context.Background(), 1)
			if e == nil {
				e = s.execute(context.Background(), job)
			}
			if e != nil {
				errs <- e
				return
			}
			writes = append(writes, float64(time.Since(started).Microseconds())/1000)
			time.Sleep(25 * time.Millisecond)
		}
	}()
	wg.Wait()
	close(stop)
	<-sampleDone
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	wall := time.Since(begun).Seconds()
	cpuAfter, _ := proc.Times()
	var heapAfter runtime.MemStats
	runtime.ReadMemStats(&heapAfter)
	percentile := func(v []float64, p int) float64 { sort.Float64s(v); return v[(len(v)-1)*p/100] }
	report := map[string]any{"wall_seconds": wall, "peak_rss_mib": float64(peakRSS) / (1024 * 1024), "allocated_mib": float64(heapAfter.TotalAlloc-heapBefore.TotalAlloc) / (1024 * 1024), "gc_cycles": heapAfter.NumGC - heapBefore.NumGC, "writer_waits": s.Store.DB.Stats().WaitCount - before.WaitCount}
	if cpuAfter != nil && cpuBefore != nil {
		report["cpu_seconds"] = cpuAfter.User + cpuAfter.System - cpuBefore.User - cpuBefore.System
	}
	if len(writes) > 0 {
		report["probe_to_alert_p95_ms"] = percentile(writes, 95)
	}
	for p, ss := range results {
		values := []float64{}
		size := 0
		for _, v := range ss {
			values = append(values, v.MS)
			size += v.Bytes
		}
		report[p] = map[string]any{"requests": len(ss), "p50_ms": percentile(values, 50), "p95_ms": percentile(values, 95), "mean_bytes": size / len(ss)}
	}
	var alerts int
	if e := s.Store.DB.QueryRow("SELECT count(*) FROM alerts WHERE fingerprint='monitor:1' AND state='open'").Scan(&alerts); e != nil || alerts != 1 {
		t.Fatalf("alert persistence: %d %v", alerts, e)
	}
	var integrity string
	if e := s.Store.DB.QueryRow("PRAGMA quick_check").Scan(&integrity); e != nil || integrity != "ok" {
		t.Fatalf("integrity: %s %v", integrity, e)
	}
	raw, _ := json.MarshalIndent(report, "", "  ")
	t.Logf("PERFORMANCE REPORT\n%s", raw)
}
