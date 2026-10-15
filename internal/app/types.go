package app

import "encoding/json"

type Permission struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Sensitive bool   `json:"sensitive,omitempty"`
}
type PermissionGroup struct {
	Key         string       `json:"key"`
	Name        string       `json:"name"`
	Icon        string       `json:"icon"`
	Description string       `json:"description"`
	Items       []Permission `json:"items"`
}

var permissions = []PermissionGroup{
	{"applications", "应用与工具", "Box", "接入身份、版本分发与编码工具", []Permission{{"application:read", "查看应用", false}, {"application:manage", "管理应用", true}, {"application:keys", "管理应用密钥", true}, {"release:read", "查看安装包", false}, {"release:manage", "管理安装包", true}, {"release:download", "下载安装包", false}, {"tool:generate", "生成二维码与条码", false}}},
	{"files", "文件与资源", "FolderOpen", "文件内容、上传、管理与图片处理", []Permission{{"file:read", "查看与预览", false}, {"file:upload", "上传文件", false}, {"file:download", "下载原文件", false}, {"file:manage", "管理目录与回收站", true}, {"file:styles", "管理图片样式", false}}},
	{"host", "主机监控", "Server", "主机资料与资源指标", []Permission{{"host:read", "查看主机", false}, {"host:update", "编辑主机", false}, {"host:agents", "管理 Agent 接入", true}}},
	{"monitor", "服务检测", "Radar", "目标、检测记录与执行", []Permission{{"monitor:read", "查看检测", false}, {"monitor:update", "管理目标", false}, {"monitor:run", "执行检测", false}}},
	{"alert", "告警处理", "Bell", "故障认领与通知静默", []Permission{{"alert:read", "查看告警", false}, {"alert:handle", "处理告警", false}, {"notification:manage", "管理通知渠道", true}}},
	{"system", "系统与团队", "Settings2", "成员授权、导航、审计与实例配置", []Permission{{"user:manage", "管理用户", true}, {"role:manage", "角色授权", true}, {"menu:manage", "管理菜单", true}, {"audit:read", "查看操作审计", true}, {"system:read", "查看系统信息", false}, {"system:settings", "修改系统设置", true}}},
}

func allPermissions() []string {
	p := []string{}
	for _, g := range permissions {
		for _, i := range g.Items {
			p = append(p, i.Key)
		}
	}
	return p
}

type User struct {
	ID          int64    `json:"id,string"`
	Username    string   `json:"username"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Version     int      `json:"version"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	CSRF        string   `json:"csrf,omitempty"`
}

func (u *User) Can(p string) bool {
	if p == "application:access" {
		return u.Can("application:read") || u.Can("application:manage") || u.Can("application:keys")
	}
	if p == "release:access" {
		return u.Can("release:read") || u.Can("release:manage") || u.Can("release:download")
	}
	if p == "application:catalog" {
		return u.Can("application:access") || u.Can("release:access")
	}

	if p == "file:access" {
		for _, key := range []string{"file:read", "file:upload", "file:download", "file:manage", "file:styles"} {
			if u.Can(key) {
				return true
			}
		}
		return false
	}
	if p == "system:access" {
		return u.Can("system:read") || u.Can("system:settings")
	}

	if p == "alert:access" {
		return u.Can("alert:read") || u.Can("notification:manage")
	}
	if p == "team:read" {
		return u.Can("role:manage") || u.Can("user:manage")
	}
	for _, v := range u.Permissions {
		if v == p {
			return true
		}
	}
	return false
}

type Role struct {
	NameKey        string   `json:"nameKey,omitempty"`
	DescriptionKey string   `json:"descriptionKey,omitempty"`
	ID             int64    `json:"id,string"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Builtin        bool     `json:"builtin"`
	Version        int      `json:"version"`
	Members        int      `json:"members"`
	Permissions    []string `json:"permissions"`
}
type Monitor struct {
	ID        int64         `json:"id,string"`
	Name      string        `json:"name"`
	Type      string        `json:"type"`
	Target    string        `json:"target"`
	Group     string        `json:"group"`
	Interval  int           `json:"interval"`
	Timeout   int           `json:"timeout"`
	Expected  int           `json:"expected"`
	Threshold int           `json:"threshold"`
	SSLDays   int           `json:"sslDays"`
	Username  string        `json:"username"`
	HasSecret bool          `json:"hasSecret"`
	TLS       bool          `json:"tls"`
	Enabled   bool          `json:"enabled"`
	State     string        `json:"status"`
	Latency   *float64      `json:"latency"`
	Detail    string        `json:"detail"`
	Checked   int64         `json:"checkedAt"`
	NextCheck int64         `json:"nextCheck"`
	Version   int           `json:"version"`
	Uptime    *float64      `json:"uptime"`
	History   []CheckResult `json:"history"`
	Secret    string        `json:"-"`
}
type CheckResult struct {
	At      int64   `json:"at"`
	State   string  `json:"state"`
	Latency float64 `json:"latency"`
	Detail  string  `json:"detail"`
}
type Point struct {
	At    int64    `json:"at"`
	Value *float64 `json:"value"`
}
type Alert struct {
	ID           int64      `json:"id,string"`
	Kind         string     `json:"kind"`
	ResourceID   int64      `json:"resourceId,string"`
	Resource     string     `json:"resource"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Severity     string     `json:"severity"`
	State        string     `json:"state"`
	Opened       int64      `json:"openedAt"`
	Recovered    *int64     `json:"recoveredAt"`
	Owner        string     `json:"owner"`
	SilenceUntil int64      `json:"silenceUntil"`
	Version      int        `json:"version"`
	Activity     []Activity `json:"activity"`
}
type Activity struct {
	Action   string `json:"action,omitempty"`
	Resource string `json:"resource,omitempty"`
	At       int64  `json:"at"`
	Actor    string `json:"actor"`
	Message  string `json:"message"`
}
type Host struct {
	AliasKey          string          `json:"aliasKey,omitempty"`
	Source            string          `json:"source"`
	Status            string          `json:"status"`
	AgentEnabled      bool            `json:"agentEnabled"`
	AgentVersion      string          `json:"agentVersion"`
	EnrolledAt        int64           `json:"enrolledAt"`
	EnrollmentExpires int64           `json:"enrollmentExpires"`
	ID                int64           `json:"id,string"`
	Hostname          string          `json:"hostname"`
	Alias             string          `json:"alias"`
	Inventory         json.RawMessage `json:"inventory"`
	Snapshot          json.RawMessage `json:"snapshot"`
	LastSeen          int64           `json:"lastSeen"`
	Version           int             `json:"version"`
}
