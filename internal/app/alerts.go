package app

import (
	"database/sql"
	"net/http"
)

func transitionAlert(tx *sql.Tx, fingerprint, kind string, resourceID int64, resource, title, description, severity string, active bool) error {
	var id int64
	e := tx.QueryRow("SELECT id FROM alerts WHERE fingerprint=? AND state <> 'resolved'", fingerprint).Scan(&id)
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	if active {
		if e == sql.ErrNoRows {
			res, e := tx.Exec("INSERT INTO alerts(fingerprint,resource_kind,resource_id,resource,title,description,severity,opened_at) VALUES(?,?,?,?,?,?,?,?)", fingerprint, kind, resourceID, resource, title, description, severity, now())
			if e != nil {
				return e
			}
			id, _ = res.LastInsertId()
			_, e = tx.Exec("INSERT INTO alert_activity(alert_id,at,actor,message) VALUES(?,?,?,?)", id, now(), "检测引擎", "达到阈值，创建告警："+description)
			if e != nil {
				return e
			}
			return queueAlertNotifications(tx, id, "opened", description)
		}
		_, e = tx.Exec("UPDATE alerts SET description=?,title=?,severity=? WHERE id=?", description, title, severity, id)
		return e
	}
	if e == sql.ErrNoRows {
		return nil
	}
	if _, e = tx.Exec("UPDATE alerts SET state='resolved',recovered_at=?,version=version+1 WHERE id=?", now(), id); e != nil {
		return e
	}
	_, e = tx.Exec("INSERT INTO alert_activity(alert_id,at,actor,message) VALUES(?,?,?,?)", id, now(), "检测引擎", "资源恢复正常："+description)
	if e != nil {
		return e
	}
	return queueAlertNotifications(tx, id, "resolved", description)
}
func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	out := []Alert{}
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT a.id,a.resource_kind,a.resource_id,a.resource,a.title,a.description,a.severity,a.state,a.opened_at,a.recovered_at,coalesce(u.display_name,''),a.silence_until,a.version FROM alerts a LEFT JOIN users u ON u.id=a.ack_by ORDER BY a.state='resolved',a.opened_at DESC LIMIT 200`)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		a := Alert{Activity: []Activity{}}
		if e = rows.Scan(&a.ID, &a.Kind, &a.ResourceID, &a.Resource, &a.Title, &a.Description, &a.Severity, &a.State, &a.Opened, &a.Recovered, &a.Owner, &a.SilenceUntil, &a.Version); e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, a)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for i := range out {
		rows, e = s.Store.DB.QueryContext(r.Context(), "SELECT at,actor,message FROM alert_activity WHERE alert_id=? ORDER BY id DESC LIMIT 100", out[i].ID)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var a Activity
			if e = rows.Scan(&a.At, &a.Actor, &a.Message); e != nil {
				rows.Close()
				return nil, e
			}
			out[i].Activity = append(out[i].Activity, a)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (s *Server) ackAlert(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	return s.changeAlert(r, u, false)
}
func (s *Server) silenceAlert(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	return s.changeAlert(r, u, true)
}
func (s *Server) changeAlert(r *http.Request, u *User, silence bool) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Version int `json:"version"`
		Minutes int `json:"minutes"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	if silence && (in.Minutes < 0 || in.Minutes > 1440) {
		return nil, bad("静默时间范围为 0–1440 分钟")
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var version int
		var state string
		if e := tx.QueryRow("SELECT state,version FROM alerts WHERE id=?", id).Scan(&state, &version); e == sql.ErrNoRows {
			return missing()
		} else if e != nil {
			return e
		}
		if state == "resolved" {
			return bad("事件已恢复")
		}
		if version != in.Version {
			return conflict()
		}
		message := "确认并接手告警"
		if silence {
			until := int64(0)
			if in.Minutes > 0 {
				until = now() + int64(in.Minutes)*60000
			}
			if _, e := tx.Exec("UPDATE alerts SET silence_until=?,version=version+1 WHERE id=?", until, id); e != nil {
				return e
			}
			message = "调整事件静默时间"
		} else {
			if state == "acknowledged" {
				return bad("该事件已被认领，请刷新查看")
			}
			if _, e := tx.Exec("UPDATE alerts SET state='acknowledged',ack_by=?,ack_at=?,version=version+1 WHERE id=?", u.ID, now(), id); e != nil {
				return e
			}
		}
		if _, e := tx.Exec("INSERT INTO alert_activity(alert_id,at,actor,message) VALUES(?,?,?,?)", id, now(), u.Name, message); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, message, idText(id))
	})
	return true, e
}
