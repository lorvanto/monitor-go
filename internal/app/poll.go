package app

import "net/http"

// Authenticate once per refresh, then authorize every included resource against
// that fresh identity. No authorization or incident data is cached across calls.
func (s *Server) poll(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	page := r.URL.Query().Get("page")
	switch page {
	case "overview", "host", "monitors", "alerts", "applications", "releases", "codes", "roles", "files", "menus", "audit", "settings":
	default:
		return nil, bad("请求字段不正确")
	}
	nav, err := s.navigation(w, r, u)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"user": u, "navigation": nav, "hosts": []Host{}, "monitors": []Monitor{}, "alerts": []Alert{}, "openAlertCount": 0}
	if u.Can("host:read") && (page == "overview" || page == "host") {
		out["hosts"], err = s.hosts(r.Context())
		if err != nil {
			return nil, err
		}
	}
	if u.Can("monitor:read") && (page == "overview" || page == "monitors") {
		out["monitors"], err = s.monitors(r.Context())
		if err != nil {
			return nil, err
		}
	}
	if u.Can("alert:read") {
		summary, e := s.alertSummary(w, r, u)
		if e != nil {
			return nil, e
		}
		out["openAlertCount"] = summary.(map[string]int)["open"]
		if page == "overview" || page == "host" || page == "alerts" {
			compact := r.Clone(r.Context())
			urlCopy := *r.URL
			compact.URL = &urlCopy
			q := compact.URL.Query()
			q.Set("compact", "1")
			compact.URL.RawQuery = q.Encode()
			out["alerts"], err = s.listAlerts(w, compact, u)
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
