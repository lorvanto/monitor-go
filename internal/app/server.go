package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"github.com/lorvanto/monitor-go/internal/hoststats"
	"github.com/lorvanto/monitor-go/locales"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Listen, DataDir, PublicURL, TrustedProxy string
	SecureCookie                             bool
	Assets                                   fs.FS
	Collect                                  bool
}
type Server struct {
	static         http.Handler
	Store          *Store
	cfg            Config
	setupToken     string
	limitMu        sync.Mutex
	attempts       map[string]attempt
	trustedProxies []netip.Prefix
	work           sync.WaitGroup
	collector      hoststats.Collector
	fileMu         sync.Mutex
	imageMu        sync.Mutex
	uploadSlots    chan struct{}
	codeSlots      chan struct{}
}
type attempt struct {
	Count   int
	Pending int
	Until   time.Time
}
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return e.Message }
func bad(s string) error          { return &apiError{422, s} }
func conflict() error             { return &apiError{409, "内容已被其他成员修改，请刷新后重试"} }
func missing() error              { return &apiError{404, "记录不存在"} }
func randomToken() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func digest(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func New(cfg Config) (*Server, error) {
	proxies, e := parseTrustedProxies(cfg.TrustedProxy)
	if e != nil {
		return nil, e
	}
	var static http.Handler
	if cfg.Assets != nil {
		static, e = newStaticHandler(cfg.Assets)
		if e != nil {
			return nil, e
		}
	}
	st, e := OpenStore(cfg.DataDir)
	if e != nil {
		return nil, e
	}
	s := &Server{static: static, Store: st, cfg: cfg, trustedProxies: proxies, attempts: map[string]attempt{}, uploadSlots: make(chan struct{}, 2), codeSlots: make(chan struct{}, 2)}
	var count int
	e = st.DB.QueryRow("SELECT count(*) FROM users").Scan(&count)
	if e != nil {
		st.Close()
		return nil, e
	}
	if count == 0 {
		path := filepath.Join(st.Dir, "setup-token")
		b, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			b = []byte(randomToken())
			err = os.WriteFile(path, b, 0600)
		}
		if err != nil {
			st.Close()
			return nil, err
		}
		s.setupToken = strings.TrimSpace(string(b))
		if len(s.setupToken) != 64 {
			st.Close()
			return nil, errors.New("初始化码文件无效")
		}
	}
	return s, nil
}

type handler func(http.ResponseWriter, *http.Request, *User) (any, error)

func decode(r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return &apiError{415, "请使用 application/json"}
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 65537))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return bad("请求字段不正确")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return bad("请求仅允许一个 JSON 对象")
	}
	return nil
}
func output(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
func pathID(r *http.Request) (int64, error) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		return 0, bad("无效的资源编号")
	}
	return id, nil
}
func (s *Server) originOK(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, e := url.Parse(origin)
	if e != nil {
		return false
	}
	scheme := "http"
	if r.TLS != nil || s.cfg.SecureCookie {
		scheme = "https"
	}
	return origin == scheme+"://"+r.Host || origin == s.cfg.PublicURL || (strings.HasPrefix(s.cfg.Listen, "127.0.0.1:") && (u.Host == "127.0.0.1:5188" || u.Host == "localhost:5188") && u.Scheme == "http")
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	add := func(pattern, perm string, public bool, fn handler) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			language := requestLocale(w, r)
			requestID := randomToken()[:16]
			r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID))
			w.Header().Set("X-Request-ID", requestID)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			var user *User
			var err error
			if r.Method != "GET" && r.Method != "HEAD" && !s.originOK(r) {
				err = &apiError{403, "请求来源不受信任"}
			}
			if err == nil && !public {
				user, err = s.identity(r)
				if err == nil && perm != "" && !user.Can(perm) {
					err = &apiError{403, "当前账号没有操作权限"}
				}
				if err == nil && r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("X-CSRF-Token") != user.CSRF {
					err = &apiError{403, "请求校验失效，请刷新页面"}
				}
			}
			var data any
			if err == nil {
				if pattern != "POST /api/v1/files/uploads" {
					r.Body = http.MaxBytesReader(w, r.Body, 65536)
				}
				data, err = fn(w, r, user)
			}
			if err != nil {
				status := 500
				message := "操作失败，请稍后重试"
				var ae *apiError
				var fe *formattedError
				if errors.As(err, &ae) {
					status = ae.Status
					message = locales.Text(language, ae.Message)
				} else if errors.As(err, &fe) {
					status = 422
					message = locales.Format(language, fe.key, fe.args...)
				} else {
					message = locales.Text(language, message)
					slog.Error("API 请求失败", "request_id", requestID, "path", r.URL.Path, "error", err)
				}
				output(w, status, map[string]any{"error": map[string]any{"message": message}, "request_id": requestID})
				return
			}
			if _, ok := data.(streamed); ok {
				return
			}
			status := 200
			if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/checks") {
				status = 202
			} else if r.Method == "POST" && (r.URL.Path == "/api/v1/monitors" || r.URL.Path == "/api/v1/users" || r.URL.Path == "/api/v1/roles") {
				status = 201
			}
			output(w, status, map[string]any{"data": data, "request_id": requestID})
		})
	}
	add("GET /api/v1/applications", "application:catalog", false, s.listApplications)
	add("POST /api/v1/applications", "application:manage", false, s.saveApplication)
	add("PATCH /api/v1/applications/{id}", "application:manage", false, s.saveApplication)
	add("DELETE /api/v1/applications/{id}", "application:manage", false, s.archiveApplication)
	add("GET /api/v1/applications/{id}/keys", "application:keys", false, s.listKeys)
	add("POST /api/v1/applications/{id}/keys", "application:keys", false, s.createKey)
	add("DELETE /api/v1/application-keys/{id}", "application:keys", false, s.revokeKey)
	add("GET /api/v1/releases", "release:access", false, s.listReleases)
	add("POST /api/v1/releases", "release:manage", false, s.saveRelease)
	add("PATCH /api/v1/releases/{id}", "release:manage", false, s.saveRelease)
	add("DELETE /api/v1/releases/{id}", "release:manage", false, s.archiveRelease)
	add("GET /api/v1/releases/{id}/download", "release:download", false, s.downloadRelease)
	add("POST /api/v1/tools/codes", "tool:generate", false, s.generateCode)
	add("GET /api/v1/integrations/me", "", true, s.machine("", func(w http.ResponseWriter, r *http.Request, k machineIdentity) (any, error) { return k, nil }))
	add("GET /api/v1/integrations/hosts", "", true, s.machine("host:read", s.machineHosts))
	add("GET /api/v1/integrations/monitors", "", true, s.machine("monitor:read", s.machineMonitors))
	add("GET /api/v1/integrations/alerts", "", true, s.machine("alert:read", s.machineAlerts))
	add("GET /api/v1/integrations/releases", "", true, s.machine("release:read", s.machineReleases))
	add("GET /api/v1/integrations/releases/{id}/download", "", true, s.machine("release:download", s.machineDownload))
	add("GET /api/v1/me/navigation", "", false, s.navigation)
	add("GET /api/v1/menus", "menu:manage", false, s.listMenus)
	add("POST /api/v1/menus", "menu:manage", false, s.saveMenu)
	add("PATCH /api/v1/menus/{id}", "menu:manage", false, s.saveMenu)
	add("DELETE /api/v1/menus/{id}", "menu:manage", false, s.deleteMenu)
	add("POST /api/v1/menus/reset", "menu:manage", false, s.resetMenus)
	add("GET /api/v1/settings", "system:access", false, s.getSettings)
	add("PATCH /api/v1/settings", "system:settings", false, s.saveSettings)
	add("GET /api/v1/system", "system:access", false, s.systemInfo)
	add("GET /api/v1/audit", "audit:read", false, s.listAudit)
	add("GET /api/v1/files", "file:access", false, s.listFiles)
	add("GET /api/v1/files/tree", "file:access", false, s.fileTree)
	add("POST /api/v1/files/folders", "file:manage", false, s.createFolder)
	add("POST /api/v1/files/uploads", "file:upload", false, s.uploadFile)
	add("PATCH /api/v1/files/{id}", "file:manage", false, s.updateFile)
	add("DELETE /api/v1/files/{id}", "file:manage", false, s.trashFile)
	add("POST /api/v1/files/{id}/restore", "file:manage", false, s.restoreFile)
	add("POST /api/v1/files/{id}/purge", "file:manage", false, s.purgeFile)
	add("GET /api/v1/files/{id}/download", "file:download", false, s.downloadFile)
	add("GET /api/v1/files/{id}/preview", "file:read", false, s.previewFile)
	add("GET /api/v1/files/{id}/image", "file:read", false, s.fileImage)
	add("GET /api/v1/image-styles", "file:access", false, s.listImageStyles)
	add("POST /api/v1/image-styles", "file:styles", false, s.saveImageStyle)
	add("PATCH /api/v1/image-styles/{id}", "file:styles", false, s.saveImageStyle)
	add("DELETE /api/v1/image-styles/{id}", "file:styles", false, s.deleteImageStyle)
	add("POST /api/v1/files/cache/clear", "system:settings", false, s.clearImageCache)
	add("GET /api/v1/status", "", true, s.status)
	add("POST /api/v1/setup", "", true, s.setup)
	add("POST /api/v1/auth/login", "", true, s.login)
	add("GET /api/v1/me", "", false, func(w http.ResponseWriter, r *http.Request, u *User) (any, error) { return u, nil })
	add("POST /api/v1/auth/logout", "", false, s.logout)
	add("GET /api/v1/roles", "team:read", false, s.listRoles)
	add("POST /api/v1/roles", "role:manage", false, s.createRole)
	add("PATCH /api/v1/roles/{id}", "role:manage", false, s.updateRole)
	add("GET /api/v1/permissions", "team:read", false, func(w http.ResponseWriter, r *http.Request, u *User) (any, error) { return permissions, nil })
	add("GET /api/v1/users", "team:read", false, s.listUsers)
	add("POST /api/v1/users", "user:manage", false, s.createUser)
	add("PATCH /api/v1/users/{id}", "user:manage", false, s.updateUser)
	add("GET /api/v1/hosts", "host:read", false, s.listHosts)
	add("PATCH /api/v1/hosts/{id}", "host:update", false, s.updateHost)
	add("GET /api/v1/hosts/{id}/metrics", "host:read", false, s.hostMetrics)
	add("GET /api/v1/monitors/availability", "monitor:read", false, s.monitorAvailability)
	add("GET /api/v1/monitors", "monitor:read", false, s.listMonitors)
	add("POST /api/v1/monitors", "monitor:update", false, s.createMonitor)
	add("PATCH /api/v1/monitors/{id}", "monitor:update", false, s.updateMonitor)
	add("PUT /api/v1/monitors/{id}", "monitor:update", false, s.editMonitor)
	add("DELETE /api/v1/monitors/{id}", "monitor:update", false, s.deleteMonitor)
	add("POST /api/v1/monitors/{id}/checks", "monitor:run", false, s.manualCheck)
	add("GET /api/v1/alerts/summary", "alert:read", false, s.alertSummary)
	add("GET /api/v1/alerts", "alert:read", false, s.listAlerts)
	add("POST /api/v1/alerts/{id}/acknowledgements", "alert:handle", false, s.ackAlert)
	add("POST /api/v1/alerts/{id}/silences", "alert:handle", false, s.silenceAlert)
	add("POST /api/v1/hosts/agents", "host:agents", false, s.createAgent)
	add("POST /api/v1/hosts/{id}/agent", "host:agents", false, s.changeAgent)
	add("POST /api/v1/agent/enroll", "", true, s.enrollAgent)
	add("POST /api/v1/agent/reports", "", true, s.reportAgent)
	add("GET /api/v1/notification-channels", "notification:manage", false, s.listChannels)
	add("POST /api/v1/notification-channels", "notification:manage", false, s.saveChannel)
	add("PATCH /api/v1/notification-channels/{id}", "notification:manage", false, s.saveChannel)
	add("DELETE /api/v1/notification-channels/{id}", "notification:manage", false, s.deleteChannel)
	add("POST /api/v1/notification-channels/{id}/tests", "notification:manage", false, s.testChannel)
	add("GET /api/v1/notification-deliveries", "alert:access", false, s.listDeliveries)
	add("POST /api/v1/notification-deliveries/{id}/retry", "notification:manage", false, s.retryDelivery)
	add("GET /api/v1/overview", "", false, s.overview)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { output(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if e := s.Store.DB.PingContext(r.Context()); e != nil {
			output(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		output(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		output(w, 404, map[string]any{"error": map[string]string{"message": locales.Text(requestLocale(w, r), "接口不存在")}})
	})
	if s.static != nil {
		mux.Handle("/", s.static)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		mux.ServeHTTP(w, r)
	})
}
func (s *Server) Start(ctx context.Context) {
	s.work.Add(3)
	go func() { defer s.work.Done(); s.scheduler(ctx) }()
	go func() { defer s.work.Done(); s.agentLoop(ctx) }()
	go func() { defer s.work.Done(); s.notificationLoop(ctx) }()
	if s.cfg.Collect {
		s.work.Add(1)
		go func() { defer s.work.Done(); s.collectLoop(ctx) }()
	}
}
func (s *Server) Wait() { s.work.Wait() }
func (s *Server) status(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var n int
	e := s.Store.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&n)
	return map[string]any{"initialized": n > 0, "version": buildinfo.Version, "storage": "SQLite"}, e
}
func actor(u *User) string {
	if u == nil {
		return "系统"
	}
	return u.Name
}
func idText(id int64) string { return fmt.Sprint(id) }
