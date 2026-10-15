package app

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSuccessfulLoginsDoNotConsumeLimit(t *testing.T) {
	_, _, c := fixture(t)
	for i := 0; i < 20; i++ {
		c.call("POST", "/auth/login", map[string]string{"username": "admin", "password": "Test-password-123!"}, 200)
	}
}

func TestFullLimitTablePreservesExistingClients(t *testing.T) {
	s := &Server{attempts: map[string]attempt{}}
	for i := 0; i < 10001; i++ {
		s.attempts[fmt.Sprint(i)] = attempt{Count: 1, Until: time.Now().Add(time.Minute)}
	}
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	s.attempts["agent:192.0.2.1"] = attempt{Count: 1, Until: time.Now().Add(time.Minute)}
	if !s.allowedRequest(r, "agent:", 60) {
		t.Fatal("existing client rejected when table is full")
	}
	r.RemoteAddr = "192.0.2.2:1234"
	if s.allowedRequest(r, "agent:", 60) {
		t.Fatal("new client accepted when table is full")
	}
}

func TestClientIPTrustBoundary(t *testing.T) {
	proxies, err := parseTrustedProxies("10.0.0.0/8, 2001:db8:1::/48")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{trustedProxies: proxies}
	for _, tc := range []struct{ peer, forwarded, want string }{
		{"10.0.0.2:80", "192.0.2.5, 10.0.0.1", "192.0.2.5"},
		{"10.0.0.2:80", "198.51.100.8, 192.0.2.5", "192.0.2.5"},
		{"192.0.2.6:80", "192.0.2.5", "192.0.2.6"},
		{"10.0.0.2:80", "invalid, 192.0.2.5", "10.0.0.2"},
		{"10.0.0.2:80", "", "10.0.0.2"},
		{"[2001:db8:1::1]:80", "2001:db8:2::1, 10.0.0.1", "2001:db8:2::1"},
		{"[::ffff:10.0.0.2]:80", "::ffff:192.0.2.5", "192.0.2.5"},
		{"10.0.0.2:80", "10.0.0.1", "10.0.0.1"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Forwarded-For", tc.forwarded)
		if got := s.clientIP(r); got != tc.want {
			t.Errorf("%s / %s: got %s want %s", tc.peer, tc.forwarded, got, tc.want)
		}
	}
	if _, err := New(Config{DataDir: t.TempDir(), TrustedProxy: "not-a-cidr"}); err == nil {
		t.Fatal("invalid trusted proxy accepted")
	}
}

func TestLoginFailuresAreScopedAndExpire(t *testing.T) {
	s, _, c := fixture(t)
	c.call("POST", "/users", userInput{Username: "reader", Name: "Reader", Password: "Read-password-123!", Roles: []string{"3"}}, 201)
	for i := 0; i < loginUsernameLimit; i++ {
		c.call("POST", "/auth/login", map[string]string{"username": " ADMIN ", "password": "incorrect"}, 401)
	}
	c.call("POST", "/auth/login", map[string]string{"username": "admin", "password": "Test-password-123!"}, 429)
	c.login("reader", "Read-password-123!")
	// A different username may succeed; it must not reset the blocked account.
	c.call("POST", "/auth/login", map[string]string{"username": "admin", "password": "Test-password-123!"}, 429)
	s.limitMu.Lock()
	for k, a := range s.attempts {
		a.Until = time.Now().Add(-time.Second)
		s.attempts[k] = a
	}
	s.limitMu.Unlock()
	c.login("admin", "Test-password-123!")
}

func TestLoginReservationsAndIPBudget(t *testing.T) {
	s := &Server{attempts: map[string]attempt{}}
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "192.0.2.10:1234"
	var pending []func(bool)
	for i := 0; i < loginUsernameLimit; i++ {
		finish, ok := s.beginLogin(r, "member")
		if !ok {
			t.Fatal("reservation refused")
		}
		pending = append(pending, finish)
	}
	if _, ok := s.beginLogin(r, "member"); ok {
		t.Fatal("concurrent requests bypassed limit")
	}
	for _, finish := range pending {
		finish(false)
	}
	if len(s.attempts) != 0 {
		t.Fatal("successful requests consumed limit keys")
	}
	for i := 0; i < loginIPLimit; i++ {
		finish, ok := s.beginLogin(r, fmt.Sprint(i))
		if !ok {
			t.Fatal(i)
		}
		finish(true)
	}
	if _, ok := s.beginLogin(r, "new"); ok {
		t.Fatal("IP failure limit bypassed")
	}
	r.RemoteAddr = "192.0.2.11:1234"
	finish, ok := s.beginLogin(r, "new")
	if !ok {
		t.Fatal("other IP blocked")
	}
	finish(false)
}
