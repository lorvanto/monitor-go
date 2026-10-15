package app

import "testing"

func TestDefaultInstanceNameAndCustomNames(t *testing.T) {
	s, _, c := fixture(t)
	assertNavigation := func(name, key string) {
		t.Helper()
		nav := decodeValue[map[string]any](t, c.call("GET", "/me/navigation", nil, 200))
		if nav["instanceName"] != name || nav["instanceNameKey"] != key {
			t.Fatalf("name/key = %v / %v", nav["instanceName"], nav["instanceNameKey"])
		}
	}
	assertNavigation("", "我的监控中心")
	settings := decodeValue[Settings](t, c.call("GET", "/settings", nil, 200))
	if !settings.UseDefaultName || settings.InstanceName != "" {
		t.Fatal("legacy untouched default not recognized")
	}
	settings.AuditDays = 60
	c.call("PATCH", "/settings", settings, 200)
	assertNavigation("", "我的监控中心")
	var stored string
	if err := s.Store.DB.QueryRow("SELECT instance_name FROM system_settings WHERE id=1").Scan(&stored); err != nil || stored != "" {
		t.Fatalf("default was persisted as display text: %q %v", stored, err)
	}
	for _, name := range []string{"团队监控中心", "我的监控中心", "My monitoring center"} {
		settings = decodeValue[Settings](t, c.call("GET", "/settings", nil, 200))
		settings.UseDefaultName = false
		settings.InstanceName = name
		c.call("PATCH", "/settings", settings, 200)
		assertNavigation(name, "")
	}
	settings = decodeValue[Settings](t, c.call("GET", "/settings", nil, 200))
	settings.UseDefaultName = true
	c.call("PATCH", "/settings", settings, 200)
	assertNavigation("", "我的监控中心")
	// A legacy installation that has already saved settings remains user data.
	mustSQL(t, s, "UPDATE system_settings SET instance_name='我的监控中心',version=9 WHERE id=1")
	assertNavigation("我的监控中心", "")
}
