package app

import (
	"fmt"
	"net/http"
)

func (s *Server) overview(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	out := map[string]any{"at": now(), "points": []Point{}, "activity": []Activity{}}
	if u.Can("monitor:read") {
		group := r.URL.Query().Get("group")
		filter := group != "" || r.URL.Query().Get("ungrouped") == "1"
		if rangeHours(r) >= 720 {
			report, err := s.availabilityFiltered(r.Context(), rangeHours(r), now(), group, filter)
			if err != nil {
				return nil, err
			}
			out["points"], out["samples"], out["latency"], out["uptime"] = report.Points, report.Samples, report.Latency, report.Uptime
			out["from"], out["to"], out["hours"] = report.From, report.To, report.Hours
		} else {
			bucket := rangeHours(r) * 3600000 / 180
			points, e := s.charts.get(r.Context(), fmt.Sprintf("%d:%t:%s", rangeHours(r), filter, group), func() ([]Point, error) {
				rows, e := s.Store.reader().QueryContext(r.Context(), `SELECT (c.at/?)*?,avg(c.latency) FROM monitors m CROSS JOIN check_results c ON c.monitor_id=m.id WHERE m.deleted_at IS NULL AND c.at>=? AND (?=0 OR m.group_name=?) GROUP BY c.at/? ORDER BY c.at/?`, bucket, bucket, since(r), filter, group, bucket, bucket)
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
				return points, nil
			})
			if e != nil {
				return nil, e
			}
			out["points"] = points
			var good, total int
			var latency *float64
			e = s.Store.reader().QueryRowContext(r.Context(), recentCheckTotals+`SELECT coalesce(sum(s.good),0),coalesce(sum(s.total),0),sum(s.latency_sum)/sum(s.total) FROM samples s JOIN monitors m ON m.id=s.monitor_id WHERE m.deleted_at IS NULL AND (?=0 OR m.group_name=?)`, now()-86400000, filter, group).Scan(&good, &total, &latency)
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
	}
	if u.Can("audit:read") {
		rows, e := s.Store.reader().QueryContext(r.Context(), "SELECT at,actor,action,resource FROM audit_logs ORDER BY id DESC LIMIT 8")
		if e != nil {
			return nil, e
		}
		activities := []Activity{}
		for rows.Next() {
			var a Activity
			if e = rows.Scan(&a.At, &a.Actor, &a.Action, &a.Resource); e != nil {
				rows.Close()
				return nil, e
			}
			a.Message = a.Action + " · " + a.Resource
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
