package app

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
)

const hourMillis int64 = 3600000

// Marking consumed rows in the same transaction makes accumulation idempotent,
// including late results for an already summarized hour and reused SQLite IDs.
func (s *Server) rollupChecks(ctx context.Context, at int64) error {
	end := at / hourMillis * hourMillis
	return s.Store.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO check_hourly(monitor_id,hour,total,good,latency_avg,latency_max)
 SELECT monitor_id,(at/3600000)*3600000,count(*),sum(state IN ('healthy','warning')),avg(latency),max(latency)
 FROM check_results WHERE rolled_up=0 AND at<? GROUP BY monitor_id,at/3600000
 ON CONFLICT(monitor_id,hour) DO UPDATE SET
 latency_avg=(check_hourly.latency_avg*check_hourly.total+excluded.latency_avg*excluded.total)/(check_hourly.total+excluded.total),
 latency_max=max(check_hourly.latency_max,excluded.latency_max),
 total=check_hourly.total+excluded.total,good=check_hourly.good+excluded.good`, end)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE check_results SET rolled_up=1 WHERE rolled_up=0 AND at<?", end)
		return err
	})
}

type Availability struct {
	MonitorID string   `json:"monitorId"`
	Samples   int64    `json:"samples"`
	Good      int64    `json:"good"`
	Uptime    *float64 `json:"uptime"`
	Latency   *float64 `json:"latency"`
	Maximum   *float64 `json:"maximum"`
}
type AvailabilityReport struct {
	From    int64          `json:"from"`
	To      int64          `json:"to"`
	Hours   int64          `json:"hours"`
	Items   []Availability `json:"items"`
	Points  []Point        `json:"points"`
	Samples int64          `json:"samples"`
	Uptime  *float64       `json:"uptime"`
	Latency *float64       `json:"latency"`
}

// One SQLite statement sees either side of a committed rollup, never both.
// Long windows begin at a UTC hour boundary because summaries have hourly precision.
const availabilityQuery = `WITH samples AS (
 SELECT monitor_id,hour AS at,total,good,latency_avg*total AS latency_sum,latency_max FROM check_hourly WHERE hour>=? AND hour<?
 UNION ALL
 SELECT monitor_id,(at/3600000)*3600000,count(*),sum(state IN ('healthy','warning')),sum(latency),max(latency)
 FROM check_results WHERE rolled_up=0 AND at>=? AND at<? GROUP BY monitor_id,at/3600000
)
SELECT s.monitor_id,s.at,sum(s.total),sum(s.good),sum(s.latency_sum),max(s.latency_max)
FROM samples s JOIN monitors m ON m.id=s.monitor_id WHERE m.deleted_at IS NULL AND (?='' OR m.group_name=?)
GROUP BY s.monitor_id,s.at ORDER BY s.monitor_id,s.at`

func (s *Server) availability(ctx context.Context, hours, at int64, group string) (AvailabilityReport, error) {
	report := AvailabilityReport{From: (at - hours*hourMillis) / hourMillis * hourMillis, To: at, Hours: hours, Items: []Availability{}, Points: []Point{}}
	rows, err := s.Store.DB.QueryContext(ctx, availabilityQuery, report.From, at, report.From, at, group, group)
	if err != nil {
		return report, err
	}
	defer rows.Close()
	type counts struct {
		samples, good    int64
		latency, maximum float64
	}
	monitors := map[int64]*counts{}
	points := map[int64]*counts{}
	total := counts{}
	bucket := max(hourMillis, hours*hourMillis/180)
	for rows.Next() {
		var id, hour int64
		var v counts
		if err = rows.Scan(&id, &hour, &v.samples, &v.good, &v.latency, &v.maximum); err != nil {
			return report, err
		}
		if monitors[id] == nil {
			monitors[id] = &counts{}
		}
		m := monitors[id]
		m.samples += v.samples
		m.good += v.good
		m.latency += v.latency
		m.maximum = max(m.maximum, v.maximum)
		at := report.From + (hour-report.From)/bucket*bucket
		if points[at] == nil {
			points[at] = &counts{}
		}
		p := points[at]
		p.samples += v.samples
		p.latency += v.latency
		total.samples += v.samples
		total.good += v.good
		total.latency += v.latency
	}
	if err = rows.Err(); err != nil {
		return report, err
	}
	for id, m := range monitors {
		uptime := float64(m.good) * 100 / float64(m.samples)
		latency := m.latency / float64(m.samples)
		maximum := m.maximum
		report.Items = append(report.Items, Availability{MonitorID: idText(id), Samples: m.samples, Good: m.good, Uptime: &uptime, Latency: &latency, Maximum: &maximum})
	}
	sort.Slice(report.Items, func(i, j int) bool { return report.Items[i].MonitorID < report.Items[j].MonitorID })
	for at, p := range points {
		latency := p.latency / float64(p.samples)
		report.Points = append(report.Points, Point{At: at, Value: &latency})
	}
	sort.Slice(report.Points, func(i, j int) bool { return report.Points[i].At < report.Points[j].At })
	report.Samples = total.samples
	if total.samples > 0 {
		uptime := float64(total.good) * 100 / float64(total.samples)
		latency := total.latency / float64(total.samples)
		report.Uptime = &uptime
		report.Latency = &latency
	}
	return report, nil
}
func (s *Server) monitorAvailability(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	hours := rangeHours(r)
	if hours != 720 && hours != 2160 {
		return nil, bad("长期可用率仅支持 30 天或 90 天")
	}
	return s.availability(r.Context(), hours, now(), r.URL.Query().Get("group"))
}
