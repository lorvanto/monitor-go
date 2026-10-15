package app

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// An actual local SMTP conversation verifies TLS negotiation, DATA and
// acceptance. No real mail account or external recipient is used.
func smtpFixture(t *testing.T, implicit, startTLS bool) (channelConfig, *tls.Config, <-chan string) {
	t.Helper()
	certServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cert := certServer.TLS.Certificates[0]
	leaf := certServer.Certificate()
	certServer.Close()
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	if implicit {
		listener = tls.NewListener(listener, tlsConfig)
	}
	t.Cleanup(func() { listener.Close() })
	mail := make(chan string, 2)
	go func() {
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			go func() {
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(4 * time.Second))
				reader := bufio.NewReader(conn)
				if _, e := fmt.Fprint(conn, "220 localhost ESMTP\r\n"); e != nil {
					return
				}
				for {
					line, e := reader.ReadString('\n')
					if e != nil {
						return
					}
					verb := strings.Fields(line)
					if len(verb) == 0 {
						return
					}
					switch verb[0] {
					case "EHLO", "HELO":
						if startTLS {
							fmt.Fprint(conn, "250-localhost\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n")
						} else {
							fmt.Fprint(conn, "250-localhost\r\n250 AUTH PLAIN\r\n")
						}
					case "STARTTLS":
						fmt.Fprint(conn, "220 Ready\r\n")
						tc := tls.Server(conn, tlsConfig)
						if tc.Handshake() != nil {
							return
						}
						conn = tc
						reader = bufio.NewReader(tc)
					case "AUTH":
						fmt.Fprint(conn, "235 Authenticated\r\n")
					case "MAIL", "RCPT":
						fmt.Fprint(conn, "250 OK\r\n")
					case "DATA":
						fmt.Fprint(conn, "354 Continue\r\n")
						var b strings.Builder
						for {
							line, e = reader.ReadString('\n')
							if e != nil {
								return
							}
							if line == ".\r\n" {
								break
							}
							b.WriteString(line)
						}
						mail <- b.String()
						fmt.Fprint(conn, "250 Accepted\r\n")
					case "QUIT":
						fmt.Fprint(conn, "221 Bye\r\n")
						return
					default:
						fmt.Fprint(conn, "500 Unknown\r\n")
					}
				}
			}()
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	p, _ := strconv.Atoi(port)
	security := "starttls"
	if implicit {
		security = "tls"
	}
	return channelConfig{Host: host, Port: p, Security: security, Username: "test", Secret: "test-password", From: "monitor@example.test", To: []string{"oncall@example.test"}}, &tls.Config{RootCAs: pool}, mail
}
func TestSMTPTransportTLSAndSTARTTLS(t *testing.T) {
	for _, implicit := range []bool{true, false} {
		t.Run(fmt.Sprint(implicit), func(t *testing.T) {
			c, trust, mail := smtpFixture(t, implicit, true)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			payload := notificationPayload{DeliveryID: "101", Event: "opened", At: now(), AlertID: "22", Title: "测试异常", Description: "本地集成测试"}
			detail, e := sendSMTP(ctx, c, payload, trust)
			if e != nil {
				t.Fatalf("SMTP %s: %v", detail, e)
			}
			select {
			case body := <-mail:
				if !strings.Contains(body, "Message-ID: <monitor-go-101-") || !strings.Contains(body, "本地集成测试") || strings.Contains(body, c.Secret) {
					t.Fatal("bad mail body")
				}
			case <-ctx.Done():
				t.Fatal("mail not received")
			}
		})
	}
	c, _, _ := smtpFixture(t, false, false)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	detail, e := sendSMTP(ctx, c, notificationPayload{}, nil)
	if e == nil || !strings.Contains(detail, "拒绝明文") {
		t.Fatal("SMTP plaintext fallback allowed")
	}
	c, _, _ = smtpFixture(t, true, true)
	detail, e = sendSMTP(ctx, c, notificationPayload{}, nil)
	if e == nil || !strings.Contains(detail, "TLS") {
		t.Fatal("untrusted SMTP certificate accepted")
	}
}
func TestWebhookRedirectMetadataAndConfigValidation(t *testing.T) {
	var leaked atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1); w.WriteHeader(204) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	_, e := sendWebhook(context.Background(), channelConfig{URL: redirect.URL, Secret: "secret"}, notificationPayload{DeliveryID: "1"})
	if e == nil || leaked.Load() != 0 {
		t.Fatal("webhook followed redirect")
	}
	_, e = sendWebhook(context.Background(), channelConfig{URL: "http://169.254.169.254/latest"}, notificationPayload{})
	if e == nil {
		t.Fatal("metadata endpoint allowed")
	}
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer tlsServer.Close()
	_, e = sendWebhook(context.Background(), channelConfig{URL: tlsServer.URL}, notificationPayload{})
	if e == nil {
		t.Fatal("untrusted webhook TLS accepted")
	}
	for _, in := range []channelInput{
		{Name: "x", Kind: "webhook", Config: channelConfig{URL: "http://example.com/hook"}},
		{Name: "x", Kind: "smtp", Config: channelConfig{Host: "smtp.example.com", Port: 25, Security: "plain", From: "a@example.test", To: []string{"b@example.test"}}},
		{Name: "x", Kind: "smtp", Config: channelConfig{Host: "smtp.example.com", Port: 465, Security: "tls", From: "a@example.test\r\nBcc: evil@example.test", To: []string{"b@example.test"}}},
	} {
		if e = validateChannel(&in); e == nil {
			t.Fatal("unsafe channel config accepted")
		}
	}
}
