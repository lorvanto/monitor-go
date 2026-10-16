package app

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

func TestPerformanceImagePeak(t *testing.T) {
	if os.Getenv("MONITOR_PERF") != "1" {
		t.Skip("set MONITOR_PERF=1 for the 24-megapixel image workload")
	}
	_, ts, c := fixture(t)
	t.Logf("HTTP address: %s", ts.URL)
	var encoded bytes.Buffer
	source := image.NewNRGBA(image.Rect(0, 0, 6000, 4000))
	if e := png.Encode(&encoded, source); e != nil {
		t.Fatal(e)
	}
	f := uploadForTest(t, c, "2", "synthetic-24mp.png", encoded.Bytes(), 200)
	source = nil
	encoded.Reset()
	runtime.GC()
	proc, e := process.NewProcess(int32(os.Getpid()))
	if e != nil {
		t.Fatal(e)
	}
	stop, stopped := make(chan struct{}), make(chan struct{})
	var peak uint64
	go func() {
		defer close(stopped)
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if m, e := proc.MemoryInfo(); e == nil {
					peak = max(peak, m.RSS)
				}
			}
		}
	}()
	started := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, e := c.c.Get(ts.URL + "/api/v1/files/" + f.ID + "/image")
			if e != nil {
				errs <- e
				return
			}
			defer res.Body.Close()
			raw, e := io.ReadAll(res.Body)
			if e != nil {
				errs <- e
				return
			}
			config, _, e := image.DecodeConfig(bytes.NewReader(raw))
			if e != nil || res.StatusCode != 200 || config.Width != 1600 || config.Height != 1066 {
				errs <- fmt.Errorf("image status/dimensions %d %+v: %v", res.StatusCode, config, e)
			}
		}()
	}
	wg.Wait()
	close(stop)
	<-stopped
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	t.Logf("Two concurrent requests, 24MP source, serialized generation: wall=%s process_peak_rss_mib=%.2f (includes fixture/client)", time.Since(started), float64(peak)/(1024*1024))
}
