package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRollupCommitsBoundedBatchesAndResumesAfterFailure(t *testing.T) {
	s, _, _ := fixture(t)
	mustSQL(t, s, "INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) VALUES(1,'batch','TCP','localhost:80','',60,3)")
	at := now() / hourMillis * hourMillis
	mustSQL(t, s, `WITH RECURSIVE n(i) AS(VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<12345)
 INSERT INTO check_results(monitor_id,at,state,latency,detail) SELECT 1,?,'healthy',10,'' FROM n`, at-1)
	mustSQL(t, s, `CREATE TRIGGER reject_batch BEFORE UPDATE OF rolled_up ON check_results WHEN NEW.id=6001 BEGIN SELECT RAISE(ABORT,'batch test'); END`)
	if e := s.rollupChecks(context.Background(), at); e == nil {
		t.Fatal("expected interrupted batch")
	}
	var marked, total int
	s.Store.DB.QueryRow("SELECT sum(rolled_up) FROM check_results").Scan(&marked)
	s.Store.DB.QueryRow("SELECT sum(total) FROM check_hourly").Scan(&total)
	if marked != 6000 || total != 6000 {
		t.Fatalf("committed progress marked=%d summarized=%d", marked, total)
	}
	mustSQL(t, s, "DROP TRIGGER reject_batch")
	for i := 0; i < 2; i++ {
		if e := s.rollupChecks(context.Background(), at); e != nil {
			t.Fatal(e)
		}
	}
	s.Store.DB.QueryRow("SELECT sum(rolled_up) FROM check_results").Scan(&marked)
	s.Store.DB.QueryRow("SELECT sum(total) FROM check_hourly").Scan(&total)
	if marked != 12345 || total != 12345 {
		t.Fatalf("resume duplicated/lost rows %d %d", marked, total)
	}
}

type gatedImageResponse struct {
	http.ResponseWriter
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (w *gatedImageResponse) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered); <-w.release })
	return w.ResponseWriter.Write(p)
}
func TestSlowImageResponseDoesNotBlockCachedReadsOrCacheClear(t *testing.T) {
	s, _, c := fixture(t)
	var encoded bytes.Buffer
	png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 80, 40)))
	file := uploadForTest(t, c, "2", "performance.png", encoded.Bytes(), 200)
	imagePath := "/files/" + file.ID + "/image"
	expected, _ := binaryForTest(t, c, imagePath, 200)
	entered, release := make(chan struct{}), make(chan struct{})
	handler := s.Handler()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Slow") == "1" {
			handler.ServeHTTP(&gatedImageResponse{ResponseWriter: w, entered: entered, release: release}, r)
		} else {
			handler.ServeHTTP(w, r)
		}
	}))
	defer ts.Close()
	t.Logf("HTTP address: %s", ts.URL)
	// Cookies are scoped to the same loopback host, independent of the test port.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1"+imagePath, nil)
	req.Header.Set("X-Test-Slow", "1")
	done := make(chan error, 1)
	go func() {
		res, e := c.c.Do(req)
		if e != nil {
			done <- e
			return
		}
		defer res.Body.Close()
		raw, e := io.ReadAll(res.Body)
		if e == nil && !bytes.Equal(raw, expected) {
			e = fmt.Errorf("slow response content changed")
		}
		done <- e
	}()
	defer func() {
		close(release)
		if e := <-done; e != nil {
			t.Error(e)
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("slow response not reached")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	fast, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/v1"+imagePath, nil)
	started := time.Now()
	res, e := c.c.Do(fast)
	if e != nil {
		t.Fatal("cached read blocked by client", e)
	}
	raw, e := io.ReadAll(res.Body)
	res.Body.Close()
	if e != nil || res.StatusCode != 200 || !bytes.Equal(raw, expected) {
		t.Fatal("cached content changed", e)
	}
	t.Logf("parallel cached image: %s", time.Since(started))
	c.call("POST", "/files/cache/clear", map[string]any{}, 200)
	regenerated, _ := binaryForTest(t, c, imagePath, 200)
	if !bytes.Equal(expected, regenerated) {
		t.Fatal("cache regeneration changed image")
	}
	// release is closed by the defer before the server waits for connections.
}
