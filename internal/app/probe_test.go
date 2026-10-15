package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestProbesAndAddressBoundaries(t *testing.T) {
	for _, ip := range []string{"169.254.169.254", "100.100.100.200", "::ffff:169.254.169.254", "224.0.0.1", "::", "fe80::1"} {
		if !forbiddenIP(netip.MustParseAddr(ip)) {
			t.Fatalf("unsafe address allowed %s", ip)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "::1", "1.1.1.1"} {
		if forbiddenIP(netip.MustParseAddr(ip)) {
			t.Fatal("valid monitoring target rejected")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer up.Close()
	r := probe(ctx, Monitor{Type: "HTTP", Target: up.URL, Expected: 200, Timeout: 2}, "")
	if r.State != "healthy" {
		t.Fatal(r)
	}
	r = probe(ctx, Monitor{Type: "TCP", Target: up.Listener.Addr().String(), Timeout: 2}, "")
	if r.State != "healthy" {
		t.Fatal(r)
	}
	tlsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	tlsServer.Config.ErrorLog = log.New(io.Discard, "", 0)
	tlsServer.StartTLS()
	defer tlsServer.Close()
	r = probe(ctx, Monitor{Type: "SSL", Target: tlsServer.Listener.Addr().String(), Timeout: 2}, "")
	if r.State != "failed" {
		t.Fatal("untrusted certificate accepted")
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, e := listener.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(time.Second))
		reader := bufio.NewReader(c)
		for range 3 {
			reader.ReadString('\n')
		}
		io.WriteString(c, "+PONG\r\n")
	}()
	r = probe(ctx, Monitor{Type: "Redis", Target: listener.Addr().String(), Timeout: 2}, "")
	if r.State != "healthy" {
		t.Fatal(r)
	}
	<-done
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer redir.Close()
	r = probe(ctx, Monitor{Type: "HTTP", Target: redir.URL, Expected: 200, Timeout: 2}, "")
	if r.State != "failed" {
		t.Fatal("metadata redirect allowed")
	}
}
