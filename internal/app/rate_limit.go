package app

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const (
	attemptWindow      = 5 * time.Minute
	maxAttemptKeys     = 10000
	loginIPLimit       = 30
	loginUsernameLimit = 10
	setupLimit         = 10
)

func parseTrustedProxies(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var result []netip.Prefix
	for _, raw := range strings.Split(value, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid -trusted-proxy CIDR %q: %w", raw, err)
		}
		result = append(result, prefix.Masked())
	}
	return result, nil
}

func (s *Server) trustedProxy(addr netip.Addr) bool {
	for _, prefix := range s.trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap().WithZone("")
	if !s.trustedProxy(peer) {
		return peer.String()
	}
	values := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	if len(values) > 32 {
		return peer.String()
	}
	chain := make([]netip.Addr, 0, len(values))
	for _, value := range values {
		addr, e := netip.ParseAddr(strings.TrimSpace(value))
		if e != nil || addr.Zone() != "" {
			return peer.String()
		}
		chain = append(chain, addr.Unmap())
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if !s.trustedProxy(chain[i]) || i == 0 {
			return chain[i].String()
		}
	}
	return peer.String()
}

// Call under limitMu. Do not remove reservations still being authenticated.
func (s *Server) expireAttempts(t time.Time) {
	for key, a := range s.attempts {
		if !t.Before(a.Until) && a.Pending == 0 {
			delete(s.attempts, key)
		}
	}
}

func (s *Server) allowedRequest(r *http.Request, prefix string, limit int) bool {
	key := prefix + s.clientIP(r)
	s.limitMu.Lock()
	defer s.limitMu.Unlock()
	t := time.Now()
	s.expireAttempts(t)
	a, exists := s.attempts[key]
	if a.Count >= limit || (!exists && len(s.attempts) >= maxAttemptKeys) {
		return false
	}
	if !exists {
		a.Until = t.Add(attemptWindow)
	}
	a.Count++
	s.attempts[key] = a
	return true
}

// Reserve both dimensions atomically so concurrent attempts cannot bypass the
// failure budget. Release successful attempts without consuming/resetting it.
func (s *Server) beginLogin(r *http.Request, username string) (func(bool), bool) {
	keys := []string{"login-ip:" + s.clientIP(r), "login-user:" + digest(username)}
	limits := []int{loginIPLimit, loginUsernameLimit}
	s.limitMu.Lock()
	t := time.Now()
	s.expireAttempts(t)
	needed := 0
	for i, key := range keys {
		a, exists := s.attempts[key]
		if a.Count+a.Pending >= limits[i] {
			s.limitMu.Unlock()
			return nil, false
		}
		if !exists {
			needed++
		}
	}
	if needed > 0 && len(s.attempts)+needed > maxAttemptKeys {
		s.limitMu.Unlock()
		return nil, false
	}
	for _, key := range keys {
		a, exists := s.attempts[key]
		if !exists {
			a.Until = t.Add(attemptWindow)
		}
		a.Pending++
		s.attempts[key] = a
	}
	s.limitMu.Unlock()
	return func(failed bool) {
		s.limitMu.Lock()
		defer s.limitMu.Unlock()
		for _, key := range keys {
			a := s.attempts[key]
			a.Pending--
			if failed {
				a.Count++
			}
			if a.Count == 0 && a.Pending == 0 {
				delete(s.attempts, key)
			} else {
				s.attempts[key] = a
			}
		}
	}, true
}
