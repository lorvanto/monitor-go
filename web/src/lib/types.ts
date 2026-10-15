export type Page =
  | "applications"
  | "releases"
  | "codes"
  | "overview"
  | "host"
  | "monitors"
  | "alerts"
  | "roles"
  | "files"
  | "menus"
  | "audit"
  | "settings";
export type MonitorStatus =
  | "healthy"
  | "failed"
  | "warning"
  | "paused"
  | "unknown";
export interface Point {
  at: number;
  value: number | null;
}
export interface CheckResult {
  at: number;
  state: MonitorStatus;
  latency: number;
  detail: string;
}
export interface Monitor {
  keyword: string;
  keywordInvert: boolean;
  latencyWarnMs: number;
  id: string;
  name: string;
  type: string;
  target: string;
  group: string;
  interval: number;
  timeout: number;
  expected: number;
  threshold: number;
  sslDays: number;
  username: string;
  hasSecret: boolean;
  tls: boolean;
  enabled: boolean;
  status: MonitorStatus;
  latency: number | null;
  detail: string;
  checkedAt: number;
  nextCheck: number;
  version: number;
  uptime: number | null;
  history: CheckResult[];
}
export interface Activity {
  action?: string;
  resource?: string;
  at: number;
  actor: string;
  message: string;
}
export interface Alert {
  id: string;
  kind: "host" | "monitor";
  resourceId: string;
  resource: string;
  title: string;
  description: string;
  severity: "critical" | "warning";
  state: "open" | "acknowledged" | "resolved";
  openedAt: number;
  recoveredAt: number | null;
  owner: string;
  silenceUntil: number;
  version: number;
  activity: Activity[];
}
export interface Role {
  nameKey?: string;
  descriptionKey?: string;
  id: string;
  name: string;
  description: string;
  builtin: boolean;
  version: number;
  members: number;
  permissions: string[];
}
export interface User {
  id: string;
  username: string;
  name: string;
  enabled: boolean;
  version: number;
  roles: string[];
  permissions: string[];
  csrf?: string;
}
export interface PermissionGroup {
  key: string;
  name: string;
  description: string;
  icon: string;
  items: { key: string; label: string; sensitive?: boolean }[];
}
export interface Host {
  aliasKey?: string;
  source: "local" | "agent";
  status: "online" | "offline" | "pending" | "revoked";
  agentEnabled: boolean;
  agentVersion: string;
  enrolledAt: number;
  enrollmentExpires: number;
  id: string;
  hostname: string;
  alias: string;
  version: number;
  lastSeen: number;
  diskAlertPercent?: number;
  inventory: {
    os: string;
    kernel: string;
    arch: string;
    cpu: string;
    cores: number;
    memory: number;
    ips: string[];
    bootTime: number;
    source: string;
  };
  snapshot: {
    cpu: number | null;
    memory: number | null;
    memoryUsed: number;
    uptime: number;
    netIn: number | null;
    netOut: number | null;
    disks: {
      path: string;
      filesystem: string;
      used: number;
      total: number;
      percent: number;
      capacityId?: string;
      capacityKind?: "apfs-container" | "apfs-quota";
      mountPoints?: string[];
    }[];
    diskIncomplete?: boolean;
    errors: string[];
  };
}
export interface HostMetric {
  at: number;
  cpu: number | null;
  memory: number | null;
  netIn: number | null;
  netOut: number | null;
}
export interface Overview {
  from?: number;
  to?: number;
  hours?: number;
  at: number;
  points: Point[];
  activity: Activity[];
  samples?: number;
  latency?: number | null;
  uptime?: number | null;
}
export interface NavItem {
  nameKey?: string;
  id: string;
  parentId: string;
  name: string;
  kind: "group" | "page" | "link";
  pageKey: Page;
  url: string;
  icon: string;
  sort: number;
  visible: boolean;
  permission: string;
  builtin: boolean;
  version: number;
  children?: NavItem[];
}
