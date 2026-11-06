package app

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	activationNoticeVersion      = "2"
	activationConsentRequired    = "Activation consent is required"
	activationNoticeChanged      = "Activation notice has changed; review it again"
	activationDestinationChanged = "Activation destination has changed; review it again"
	activationNotConfigured      = "Activation service is not configured"
	activationUnavailable        = "Activation service is unavailable; try again"
	activationRejected           = "Activation request was rejected; try again"
)

type activationStatus struct {
	Required      bool   `json:"required"`
	Configured    bool   `json:"configured"`
	ConsoleURL    string `json:"consoleUrl"`
	NoticeVersion string `json:"noticeVersion"`
}

type setupActivation struct {
	instanceID string
	origin     string
}

func (s *Server) validateActivationConsent(consent bool, noticeVersion, consoleURL string) error {
	if !consent {
		return bad(activationConsentRequired)
	}
	if noticeVersion != activationNoticeVersion {
		return bad(activationNoticeChanged)
	}
	if s.usage.origin == "" {
		return &apiError{503, activationNotConfigured}
	}
	if consoleURL != s.usage.origin {
		return bad(activationDestinationChanged)
	}
	return nil
}

// activateInstallation is called only after validating setup access or settings
// permissions and the current notice. A durable random identity makes registration
// idempotent across retries and restarts. It is not consent to background reports:
// enabled, registered and consent records remain unchanged until the setup or
// settings transaction commits. No administrator fields or setup token are sent.
func (s *Server) activateInstallation(ctx context.Context) (*setupActivation, error) {
	p := s.usage
	var st usageState
	err := p.write(ctx, func(tx *sql.Tx) error {
		var err error
		st, err = readUsageState(ctx, tx)
		if err != nil {
			return err
		}
		if st.InstanceID != "" {
			return nil
		}
		st.InstanceID = randomToken()[:32]
		st.Credential, err = s.Store.encrypt(randomToken())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE usage_settings SET instance_id=?,credential=? WHERE id=1 AND enabled=0", st.InstanceID, st.Credential)
		return err
	})
	if err != nil {
		return nil, err
	}
	credential, err := s.Store.decrypt(st.Credential)
	if err != nil {
		return nil, &apiError{502, activationUnavailable}
	}
	// Never hold a SQLite transaction while waiting for a remote service. The
	// reporter's HTTP client also rejects redirects and bounds each request to 5s.
	result, err := p.post(ctx, "register", "", p.registration(st.InstanceID, credential))
	if err != nil {
		var remote *usageRemoteError
		if errors.As(err, &remote) {
			return nil, &apiError{502, activationRejected}
		}
		return nil, &apiError{502, activationUnavailable}
	}
	if result.InstanceID != st.InstanceID {
		return nil, &apiError{502, activationRejected}
	}
	return &setupActivation{instanceID: st.InstanceID, origin: p.origin}, nil
}

// commit belongs to the same transaction as the administrator or settings audit.
// A failed transaction can leave a remote anonymous registration, but never a
// partially confirmed local state or an enabled background reporter.
func (a *setupActivation) commit(ctx context.Context, tx *sql.Tx) error {
	at := time.Now().Unix()
	result, err := tx.ExecContext(ctx, `UPDATE usage_settings SET enabled=1,registered=1,notice_version=?,consent_at=?,activated_at=?,next_report_at=?,last_error='',failures=0 WHERE id=1 AND console_url=? AND instance_id=?`, activationNoticeVersion, at, at, at, a.origin, a.instanceID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return &apiError{409, usageChanged}
	}
	return nil
}
