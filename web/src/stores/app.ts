import { create } from "zustand";
import { i18nStore, localizedLabels, metadataLabel, t, useI18n } from "../i18n";
import type {
  Alert,
  Host,
  Monitor,
  MonitorStatus,
  NavItem,
  Page,
  PermissionGroup,
  Role,
  User,
} from "../lib/types";
export {
  bytes,
  dateTime,
  hours,
  latencyParts,
  number,
  relative,
} from "../lib/format";
export type * from "../lib/types";

export const pageNames: Record<Page, string> = localizedLabels({
  applications: "应用与密钥",
  releases: "安装包管理",
  codes: "编码工具",
  overview: "监控总览",
  host: "主机详情",
  monitors: "服务检测",
  alerts: "告警中心",
  roles: "角色与权限",
  files: "文件管理",
  menus: "菜单管理",
  audit: "操作审计",
  settings: "系统设置",
});
function resolvePage(): Page {
  const key = location.hash.slice(1);
  return Object.hasOwn(pageNames, key) ? (key as Page) : "overview";
}
interface AppData {
  monitors: Monitor[];
  hosts: Host[];
  alerts: Alert[];
  roles: Role[];
  users: User[];
  permissions: PermissionGroup[];
}
interface AppState {
  navigation: NavItem[];
  instanceName: string;
  instanceNameKey: string;
  data: AppData;
  user: User | null;
  initialized: boolean;
  booting: boolean;
  connectionError: string;
  updatedAt: number;
  openAlertCount: number;
  page: Page;
  menuOpen: boolean;
  activeAlert: string;
  activeHost: string;
  activeMonitor: string;
  createMonitorRequested: boolean;
  toasts: { id: number; message: string; error: boolean }[];
  checking: string[];
}
const emptyData = (): AppData => ({
  monitors: [],
  hosts: [],
  alerts: [],
  roles: [],
  users: [],
  permissions: [],
});
export const appStore = create<AppState>(() => ({
  navigation: [],
  instanceName: "",
  instanceNameKey: "我的监控中心",
  data: emptyData(),
  user: null,
  initialized: true,
  booting: true,
  connectionError: "",
  updatedAt: 0,
  openAlertCount: 0,
  page: resolvePage(),
  menuOpen: false,
  activeAlert: "",
  activeHost: "",
  activeMonitor: "",
  createMonitorRequested: false,
  toasts: [],
  checking: [],
}));
export function displayNavName(item: NavItem): string {
  return item.nameKey ? metadataLabel(item.nameKey) : item.name;
}
function navLabel(items: NavItem[], key: Page): string {
  for (const item of items) {
    if (item.kind === "page" && item.pageKey === key)
      return displayNavName(item);
    const nested = navLabel(item.children || [], key);
    if (nested) return nested;
  }
  return "";
}
export function getAppView(state = appStore.getState()) {
  return {
    ...state,
    pageTitle: navLabel(state.navigation, state.page) || pageNames[state.page],
    openAlerts: state.data.alerts.filter((a) => a.state !== "resolved"),
  };
}
export function useApp() {
  const state = appStore();
  useI18n();
  return getAppView(state);
}
export const pagePermissions: Record<Page, string> = {
  applications: "application:access",
  releases: "release:access",
  codes: "tool:generate",
  overview: "",
  host: "host:read",
  monitors: "monitor:read",
  alerts: "alert:access",
  roles: "team:read",
  files: "file:access",
  menus: "menu:manage",
  audit: "audit:read",
  settings: "system:access",
};
export function can(permission: string): boolean {
  const groups: Record<string, string[]> = {
    "application:access": [
      "application:read",
      "application:manage",
      "application:keys",
    ],
    "release:access": ["release:read", "release:manage", "release:download"],
    "file:access": [
      "file:read",
      "file:upload",
      "file:download",
      "file:manage",
      "file:styles",
    ],
    "system:access": ["system:read", "system:settings"],
    "alert:access": ["alert:read", "notification:manage"],
    "team:read": ["role:manage", "user:manage"],
  };
  return groups[permission]
    ? groups[permission].some(can)
    : !permission ||
        !!appStore.getState().user?.permissions.includes(permission);
}
export function syncLocation() {
  appStore.setState({ page: resolvePage() });
}
export function navigate(target: Page) {
  if (!can(pagePermissions[target])) {
    notify(t("当前账号没有该页面的权限"), true);
    return;
  }
  appStore.setState({ page: target, menuOpen: false });
  location.hash = target;
  window.scrollTo({ top: 0 });
}
export function startMonitorCreate() {
  appStore.setState({ createMonitorRequested: true });
  navigate("monitors");
}
export function notify(message: string, error = false) {
  const id = Date.now() + Math.random();
  appStore.setState((state) => ({
    toasts: [...state.toasts, { id, message, error }],
  }));
  setTimeout(
    () =>
      appStore.setState((state) => ({
        toasts: state.toasts.filter((toast) => toast.id !== id),
      })),
    5000,
  );
}
export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
// Invalidate work from a previous login, including logout/login of the same account.
let sessionVersion = 0;
let authAttempt = 0;
appStore.subscribe((state, previous) => {
  if (
    state.user?.id !== previous.user?.id ||
    state.user?.csrf !== previous.user?.csrf
  )
    sessionVersion++;
});
class SessionChangedError extends Error {
  constructor() {
    super(t("登录已失效，请重新登录"));
  }
}
function clearSession() {
  sessionVersion++;
  appStore.setState({
    user: null,
    navigation: [],
    data: emptyData(),
    updatedAt: 0,
    openAlertCount: 0,
    instanceName: "",
    instanceNameKey: "我的监控中心",
    menuOpen: false,
    activeHost: "",
    activeMonitor: "",
    activeAlert: "",
    createMonitorRequested: false,
    checking: [],
    toasts: [],
    connectionError: "",
  });
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const session = sessionVersion;
  let response: Response;
  const user = appStore.getState().user;
  try {
    response = await fetch("/api/v1" + path, {
      method,
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        "Accept-Language": i18nStore.getState().locale,
        ...(user?.csrf ? { "X-CSRF-Token": user.csrf } : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(20000),
    });
  } catch {
    if (session !== sessionVersion) throw new SessionChangedError();
    throw new APIError(0, t("无法连接服务，请检查 Go 后端是否正在运行"));
  }
  if (session !== sessionVersion) throw new SessionChangedError();
  let json: { data: T; error?: { message?: string } };
  try {
    json = await response.json();
  } catch {
    throw new APIError(response.status, t("服务返回异常，请稍后重试"));
  }
  if (session !== sessionVersion) throw new SessionChangedError();
  if (!response.ok) {
    if (response.status === 401 && path !== "/auth/login") clearSession();
    throw new APIError(response.status, json.error?.message || t("请求失败"));
  }
  return json.data;
}
export async function refreshNavigation() {
  const navigation = await api<{
    items: NavItem[];
    instanceName: string;
    instanceNameKey?: string;
  }>("/me/navigation");
  appStore.setState({
    navigation: navigation.items,
    instanceName: navigation.instanceName,
    instanceNameKey: navigation.instanceNameKey || "",
  });
}
interface RefreshTask {
  session: number;
  page: Page;
  promise: Promise<void>;
}
let refreshing: RefreshTask | undefined;
let teamCache = "";
let lastRefreshPage: Page | undefined;
export function refreshAll(afterMutation = false): Promise<void> {
  if (!appStore.getState().user) return Promise.resolve();
  const session = sessionVersion;
  const page = appStore.getState().page;
  if (afterMutation) teamCache = "";
  const current = refreshing;
  if (current?.session === session) {
    return afterMutation || current.page !== page
      ? current.promise.then(() => {
          if (session === sessionVersion) return refreshAll(true);
        })
      : current.promise;
  }
  const task: RefreshTask = { session, page, promise: Promise.resolve() };
  task.promise = loadAll(session, page, afterMutation).finally(() => {
    if (refreshing === task) refreshing = undefined;
  });
  refreshing = task;
  return task.promise;
}
async function loadAll(session: number, page: Page, afterMutation: boolean) {
  try {
    const user = await api<User>("/me");
    if (session !== sessionVersion) return;
    appStore.setState({ user });
    const current = appStore.getState().data;
    const needHosts = page === "overview" || page === "host";
    const needMonitors = page === "overview" || page === "monitors";
    const needAlerts =
      page === "overview" || page === "host" || page === "alerts";
    const cacheKey = `${session}:${[...user.permissions].sort().join(",")}`;
    const needTeam =
      page === "roles" &&
      (afterMutation || lastRefreshPage !== "roles" || teamCache !== cacheKey);
    const [
      hosts,
      monitors,
      alerts,
      roles,
      users,
      permissions,
      navigation,
      summary,
    ] = await Promise.all([
      can("host:read")
        ? needHosts
          ? api<Host[]>("/hosts")
          : current.hosts
        : [],
      can("monitor:read")
        ? needMonitors
          ? api<Monitor[]>("/monitors")
          : current.monitors
        : [],
      can("alert:read")
        ? needAlerts
          ? api<Alert[]>("/alerts")
          : current.alerts
        : [],
      can("team:read")
        ? needTeam
          ? api<Role[]>("/roles")
          : current.roles
        : [],
      can("team:read")
        ? needTeam
          ? api<User[]>("/users")
          : current.users
        : [],
      can("team:read")
        ? needTeam
          ? api<PermissionGroup[]>("/permissions")
          : current.permissions
        : [],
      api<{
        items: NavItem[];
        instanceName: string;
        instanceNameKey?: string;
      }>("/me/navigation"),
      can("alert:read") && !needAlerts
        ? api<{ open: number }>("/alerts/summary")
        : null,
    ]);
    if (
      session !== sessionVersion ||
      !appStore.getState().user ||
      page !== appStore.getState().page
    )
      return;
    if (needTeam) teamCache = cacheKey;
    lastRefreshPage = page;
    appStore.setState({
      data: { hosts, monitors, alerts, roles, users, permissions },
      openAlertCount: can("alert:read")
        ? (summary?.open ?? alerts.filter((a) => a.state !== "resolved").length)
        : 0,
      navigation: navigation.items,
      instanceName: navigation.instanceName,
      instanceNameKey: navigation.instanceNameKey || "",
      updatedAt: Date.now(),
      connectionError: "",
    });
  } catch (e) {
    if (
      session !== sessionVersion ||
      page !== appStore.getState().page ||
      e instanceof SessionChangedError
    )
      return;
    const message = (e as Error).message;
    appStore.setState({ connectionError: message });
    if (!(e instanceof APIError && e.status === 401)) notify(message, true);
  }
}
export async function boot() {
  const attempt = ++authAttempt;
  sessionVersion++;
  appStore.setState({ booting: true });
  try {
    const status = await api<{ initialized: boolean }>("/status");
    appStore.setState({ initialized: status.initialized, connectionError: "" });
    if (status.initialized) {
      try {
        const user = await api<User>("/me");
        if (attempt !== authAttempt) return;
        appStore.setState({ user });
        await refreshAll();
      } catch (e) {
        if (!(e instanceof APIError && e.status === 401)) throw e;
      }
    }
  } catch (e) {
    if (attempt === authAttempt && !(e instanceof SessionChangedError))
      appStore.setState({ connectionError: (e as Error).message });
  } finally {
    if (attempt === authAttempt) appStore.setState({ booting: false });
  }
}
export async function signIn(input: unknown, setup = false) {
  const attempt = ++authAttempt;
  sessionVersion++;
  await api(setup ? "/setup" : "/auth/login", "POST", input);
  const user = await api<User>("/me");
  if (attempt !== authAttempt) return;
  appStore.setState({ initialized: true, user });
  navigate("overview");
  await refreshAll();
}
export async function logout() {
  const attempt = ++authAttempt;
  sessionVersion++;
  let error = "";
  try {
    await api("/auth/logout", "POST", {});
  } catch (e) {
    if (!(e instanceof SessionChangedError)) error = (e as Error).message;
  } finally {
    if (attempt === authAttempt) {
      clearSession();
      appStore.setState({ connectionError: error, booting: false });
    }
  }
}
export async function action(fn: () => Promise<unknown>, message?: string) {
  const session = sessionVersion;
  try {
    await fn();
    if (session !== sessionVersion) return false;
    await refreshAll(true);
    if (session !== sessionVersion) return false;
    if (message) notify(message);
    return true;
  } catch (e) {
    if (session === sessionVersion && !(e instanceof SessionChangedError))
      notify((e as Error).message, true);
    return false;
  }
}
export async function runCheck(ids: string[]) {
  const session = sessionVersion;
  const unique = [...new Set(ids)].filter(
    (id) => !appStore.getState().checking.includes(id),
  );
  if (!unique.length) return;
  appStore.setState((state) => ({ checking: [...state.checking, ...unique] }));
  try {
    const results = await Promise.allSettled(
      unique.map((id) => api(`/monitors/${id}/checks`, "POST", {})),
    );
    if (session !== sessionVersion) return;
    const ok = results.filter((r) => r.status === "fulfilled").length;
    const failed = results.find((r) => r.status === "rejected");
    if (ok) notify(t("已提交 {p0} 项检测，结果将自动刷新", { p0: ok }));
    if (failed?.status === "rejected") notify(failed.reason.message, true);
  } finally {
    if (session === sessionVersion)
      appStore.setState((state) => ({
        checking: state.checking.filter((id) => !unique.includes(id)),
      }));
  }
}
export function statusLabel(status: MonitorStatus) {
  return {
    healthy: t("正常"),
    failed: t("异常"),
    warning: t("需关注"),
    paused: t("已暂停"),
    unknown: t("待检测"),
  }[status];
}
