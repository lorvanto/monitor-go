package app

import (
	"net/http"
)

func (s *Server) overview(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	out := map[string]any{"at": now(), "points": []Point{}, "activity": []Activity{}}
	if u.Can("monitor:read") {
		group := r.URL.Query().Get("group")
		bucket := rangeHours(r) * 3600000 / 180
		rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT (c.at/?)*?,avg(c.latency) FROM check_results c JOIN monitors m ON m.id=c.monitor_id WHERE m.deleted_at IS NULL AND c.at>=? AND (?='' OR m.group_name=?) GROUP BY c.at/? ORDER BY c.at/?`, bucket, bucket, since(r), group, group, bucket, bucket)
		if e != nil {
			return nil, e
		}
		points := []Point{}
		for rows.Next() {
			var p Point
			if e = rows.Scan(&p.At, &p.Value); e != nil {
				rows.Close()
				return nil, e
			}
			points = append(points, p)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		out["points"] = points
		var good, total int
		var latency *float64
		e = s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(sum(c.state IN ('healthy','warning')),0),count(*),avg(c.latency) FROM check_results c JOIN monitors m ON m.id=c.monitor_id WHERE m.deleted_at IS NULL AND c.at>=? AND (?='' OR m.group_name=?)`, now()-86400000, group, group).Scan(&good, &total, &latency)
		if e != nil {
			return nil, e
		}
		out["samples"] = total
		out["latency"] = latency
		var uptime *float64
		if total > 0 {
			v := float64(good) * 100 / float64(total)
			uptime = &v
		}
		out["uptime"] = uptime
	}
	if u.Can("audit:read") {
		rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT at,actor,action || ' · ' || resource FROM audit_logs ORDER BY id DESC LIMIT 8")
		if e != nil {
			return nil, e
		}
		activities := []Activity{}
		for rows.Next() {
			var a Activity
			if e = rows.Scan(&a.At, &a.Actor, &a.Message); e != nil {
				rows.Close()
				return nil, e
			}
			activities = append(activities, a)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		out["activity"] = activities
	}
	return out, nil
}
