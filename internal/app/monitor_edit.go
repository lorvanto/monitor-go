package app

import (
	"database/sql"
	"net/http"
)

type monitorEditInput struct {
	monitorInput
	Version       int  `json:"version"`
	ClearPassword bool `json:"clearPassword"`
}

func (s *Server) editMonitor(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var in monitorEditInput
	if err = decode(r, &in); err != nil {
		return nil, err
	}
	if err = in.validate(); err != nil {
		return nil, err
	}
	err = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var secret string
		var version int
		if err := tx.QueryRow("SELECT secret,version FROM monitors WHERE id=? AND deleted_at IS NULL", id).Scan(&secret, &version); err == sql.ErrNoRows {
			return conflict()
		} else if err != nil {
			return err
		}
		if version != in.Version {
			return conflict()
		}
		if in.Type != "MySQL" && in.Type != "Redis" {
			in.Username = ""
			in.ClearPassword = true
		}
		if in.ClearPassword {
			secret = ""
		} else if in.Password != "" {
			var err error
			secret, err = s.Store.encrypt(in.Password)
			if err != nil {
				return err
			}
		}
		_, err := tx.Exec(`UPDATE monitors SET name=?,type=?,target=?,group_name=?,interval_seconds=?,timeout_seconds=?,expected_status=?,failure_threshold=?,ssl_days=?,username=?,secret=?,tls=?,version=version+1,failures=0,state='unknown',next_check=0 WHERE id=?`, in.Name, in.Type, in.Target, in.Group, in.Interval, in.Timeout, in.Expected, in.Threshold, in.SSLDays, in.Username, secret, in.TLS, id)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE jobs SET state='cancelled',finished_at=? WHERE monitor_id=? AND state='queued'", now(), id); err != nil {
			return err
		}
		return auditRequest(tx, r, u.Name, "编辑检测", idText(id))
	})
	return true, err
}
