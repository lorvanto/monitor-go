import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { version } from "../package.json";
import BrandMark from "./components/BrandMark";
import Icon from "./components/Icon";
import LanguageSelect from "./components/LanguageSelect";
import Modal from "./components/Modal";
import NavTree from "./components/NavTree";
import PageBoundary from "./components/PageBoundary";
import StatusGlyph from "./components/StatusGlyph";
import { t } from "./i18n";

import { createStore, useStore } from "zustand";
import { useI18n } from "./i18n";
import { classNames } from "./lib/react";
import {
  appStore,
  boot,
  can,
  displayNavName,
  logout,
  navigate,
  pageNames,
  pagePermissions,
  refreshAll,
  syncLocation,
  useApp,
  type NavItem,
  type Page,
} from "./stores/app";
import AuthView from "./views/AuthView";
const OverviewView = lazy(() => import("./views/OverviewView"));
const HostView = lazy(() => import("./views/HostView"));
const MonitorsView = lazy(() => import("./views/MonitorsView"));
const AlertsView = lazy(() => import("./views/AlertsView"));
const RolesView = lazy(() => import("./views/RolesView"));
const FilesView = lazy(() => import("./views/FilesView"));
const MenusView = lazy(() => import("./views/MenusView"));
const AuditView = lazy(() => import("./views/AuditView"));
const SettingsView = lazy(() => import("./views/SettingsView"));
const ApplicationsView = lazy(() => import("./views/ApplicationsView"));
const ReleasesView = lazy(() => import("./views/ReleasesView"));
const CodesView = lazy(() => import("./views/CodesView"));
export default function App() {
  let stored: string | null = null;
  try {
    stored = localStorage.getItem("monitor-go-theme");
  } catch {
    /* Use the system preference when storage is blocked. */
  }
  const initialState = {
    dark: stored
      ? stored === "dark"
      : matchMedia("(prefers-color-scheme: dark)").matches,
    paletteOpen: false,
    query: "",
    cursor: 0,
    help: false,
    userOpen: false,
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const foot = useRef<HTMLElement | null>(null);
  const meButton = useRef<HTMLButtonElement | null>(null);
  const appState = useApp();
  useI18n();
  const views = {
    overview: OverviewView,
    host: HostView,
    monitors: MonitorsView,
    alerts: AlertsView,
    roles: RolesView,
    files: FilesView,
    menus: MenusView,
    audit: AuditView,
    settings: SettingsView,
    applications: ApplicationsView,
    releases: ReleasesView,
    codes: CodesView,
  };
  const pageIcons: Record<Page, string> = {
    overview: "LayoutDashboard",
    host: "Server",
    monitors: "Radar",
    alerts: "Bell",
    roles: "ShieldCheck",
    files: "FolderOpen",
    menus: "ListTree",
    audit: "ScrollText",
    settings: "Settings2",
    applications: "KeyRound",
    releases: "Package",
    codes: "QrCode",
  };
  const isMac = /Mac|iPhone|iPad/.test(navigator.platform);
  function toggleTheme() {
    store.setState({ dark: !store.getState().dark });
    try {
      localStorage.setItem(
        "monitor-go-theme",
        store.getState().dark ? "dark" : "light",
      );
    } catch {
      /* Keep the preference for this session. */
    }
  }
  const getDestinations = () => {
    const result: {
      key: Page;
      label: string;
      icon: string;
      group: string;
    }[] = [];
    function walk(items: NavItem[], group: string) {
      for (const item of items) {
        if (item.kind === "group")
          walk(item.children || [], displayNavName(item));
        else if (
          item.kind === "page" &&
          !result.some((r) => r.key === item.pageKey)
        )
          result.push({
            key: item.pageKey,
            label: displayNavName(item),
            icon: item.icon,
            group,
          });
      }
    }
    walk(appStore.getState().navigation, "");
    for (const key of Object.keys(pageNames) as Page[])
      if (!result.some((r) => r.key === key) && can(pagePermissions[key]))
        result.push({
          key,
          label: pageNames[key],
          icon: pageIcons[key],
          group: t("未在导航中显示"),
        });
    return result;
  };
  const getResults = () => {
    const q = store.getState().query.trim().toLowerCase();
    return getDestinations().filter(
      (d) =>
        !q ||
        `${d.label} ${pageNames[d.key]} ${d.group}`.toLowerCase().includes(q),
    );
  };
  function openPalette() {
    store.setState({ query: "" });
    store.setState({ cursor: 0 });
    store.setState({ paletteOpen: true });
    appStore.setState({ menuOpen: false });
  }
  function paletteKey(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const n = getResults().length;
      if (!n) return;
      store.setState({
        cursor:
          (store.getState().cursor + (e.key === "ArrowDown" ? 1 : n - 1)) % n,
      });
      queueMicrotask(() =>
        document
          .querySelector(".command-item.active")
          ?.scrollIntoView({ block: "nearest" }),
      );
    } else if (e.key === "Enter" && !e.nativeEvent.isComposing) {
      e.preventDefault();
      const target = getResults()[store.getState().cursor];
      if (target) go(target.key);
    }
  }
  const getInitial = () => appStore.getState().user?.name.slice(0, 1) || "?";
  function go(target: Page) {
    store.setState({ paletteOpen: false });
    store.setState({ userOpen: false });
    navigate(target);
  }
  function keydown(e: KeyboardEvent) {
    if (
      (e.metaKey || e.ctrlKey) &&
      e.key.toLowerCase() === "k" &&
      appStore.getState().user
    ) {
      e.preventDefault();
      if (store.getState().paletteOpen) store.setState({ paletteOpen: false });
      else openPalette();
    } else if (e.key === "Escape") {
      if (store.getState().userOpen) {
        store.setState({ userOpen: false });
        meButton.current?.focus();
      } else if (appStore.getState().menuOpen)
        appStore.setState({ menuOpen: false });
    }
  }
  function pointerdown(e: PointerEvent) {
    if (store.getState().userOpen && !foot.current?.contains(e.target as Node))
      store.setState({ userOpen: false });
  }
  async function exit() {
    store.setState({ userOpen: false });
    await logout();
  }
  const DynamicComponent = views[appState.page];
  useEffect(() => {
    store.setState({
      paletteOpen: false,
      userOpen: false,
      help: false,
      query: "",
      cursor: 0,
    });
  }, [appState.user?.id, appState.user?.csrf]);
  useEffect(() => {
    const v = viewState.dark;
    document.documentElement.dataset.theme = v ? "dark" : "light";
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute("content", v ? "#111316" : "#f1f2f4");
  }, [viewState.dark]);
  useEffect(() => {
    store.setState({ cursor: 0 });
  }, [viewState.query]);
  useEffect(() => {
    document.title = `${appState.pageTitle} · Monitor Go`;
  }, [appState.pageTitle]);
  useEffect(() => {
    void boot();
    document.addEventListener("keydown", keydown);
    document.addEventListener("pointerdown", pointerdown);
    window.addEventListener("hashchange", syncLocation);
    const interval = setInterval(() => {
      if (!document.hidden) void refreshAll();
    }, 10000);
    return () => {
      clearInterval(interval);
      document.removeEventListener("keydown", keydown);
      document.removeEventListener("pointerdown", pointerdown);
      window.removeEventListener("hashchange", syncLocation);
    };
  }, []);
  const results = getResults();
  const initial = getInitial();
  const instanceName =
    appState.instanceNameKey === "我的监控中心"
      ? t("我的监控中心")
      : appState.instanceName;
  return (
    <>
      {appState.booting ? (
        <div role="status" className="boot">
          <BrandMark size={28}></BrandMark>
          <span>{t("正在连接监控服务…")}</span>
        </div>
      ) : !appState.user ? (
        <AuthView></AuthView>
      ) : (
        <>
          <div className="shell">
            {appState.menuOpen ? (
              <button
                tabIndex={-1}
                aria-label={t("关闭导航")}
                className="scrim"
                onClick={() => {
                  appStore.setState({ menuOpen: false });
                }}
              ></button>
            ) : null}
            <aside className={classNames("rail", { open: appState.menuOpen })}>
              <div className="rail-head">
                <button
                  title={instanceName}
                  className="brand"
                  onClick={() => {
                    navigate("overview");
                  }}
                >
                  <BrandMark></BrandMark>
                  <span className="brand-text">
                    <strong>{instanceName}</strong>
                    <small>{"Monitor Go"}</small>
                  </span>
                </button>
              </div>
              <button
                className="rail-search"
                onClick={() => {
                  openPalette();
                }}
              >
                <Icon name="Search" size={15}></Icon>
                <span>{t("搜索页面")}</span>
                <kbd>{isMac ? "⌘K" : "Ctrl K"}</kbd>
              </button>
              <nav aria-label={t("主导航")} className="rail-nav">
                <NavTree items={appState.navigation}></NavTree>
              </nav>
              <div
                ref={(element) => {
                  foot.current = element;
                }}
                className="rail-foot"
              >
                <LanguageSelect></LanguageSelect>
                <div className="rail-status">
                  <StatusGlyph
                    tone={appState.connectionError ? "crit" : "ok"}
                    size={8}
                  ></StatusGlyph>
                  <span>
                    {appState.connectionError
                      ? t("服务连接中断")
                      : t("服务已连接")}
                  </span>
                  <button
                    aria-label={
                      viewState.dark ? t("切换到浅色主题") : t("切换到深色主题")
                    }
                    title={
                      viewState.dark ? t("切换到浅色主题") : t("切换到深色主题")
                    }
                    className="icon-btn sm"
                    onClick={() => {
                      toggleTheme();
                    }}
                  >
                    <Icon
                      name={viewState.dark ? "Sun" : "Moon"}
                      size={15}
                    ></Icon>
                  </button>
                </div>
                <button
                  ref={(element) => {
                    meButton.current = element;
                  }}
                  aria-haspopup="menu"
                  aria-expanded={viewState.userOpen}
                  className="me"
                  onClick={() => {
                    store.setState({ userOpen: !viewState.userOpen });
                  }}
                >
                  <span className="avatar">{initial}</span>
                  <span className="me-text">
                    <strong>{appState.user.name}</strong>
                    <small>{appState.user.username}</small>
                  </span>
                  <Icon name="ChevronsUpDown" size={14}></Icon>
                </button>
                {viewState.userOpen ? (
                  <div role="menu" aria-label={t("账号菜单")} className="menu">
                    {can("menu:manage") ? (
                      <button
                        role="menuitem"
                        onClick={() => {
                          go("menus");
                        }}
                      >
                        <Icon name="ListTree"></Icon>
                        {t("管理导航")}
                      </button>
                    ) : null}
                    {can("system:access") ? (
                      <button
                        role="menuitem"
                        onClick={() => {
                          go("settings");
                        }}
                      >
                        <Icon name="Settings2"></Icon>
                        {t("系统设置")}
                      </button>
                    ) : null}
                    <button
                      role="menuitem"
                      onClick={() => {
                        store.setState({ help: true, userOpen: false });
                      }}
                    >
                      <Icon name="CircleHelp"></Icon>
                      {t("使用说明")}
                    </button>
                    <hr />
                    <button
                      role="menuitem"
                      onClick={() => {
                        exit();
                      }}
                    >
                      <Icon name="LogOut"></Icon>
                      {t("退出登录")}
                    </button>
                    <p className="menu-note">
                      {t("会话有效期 12 小时，退出后立即失效")}
                    </p>
                  </div>
                ) : null}
              </div>
            </aside>
            <div className="main">
              <header className="mobile-bar">
                <button
                  aria-label={t("打开导航")}
                  className="icon-btn"
                  onClick={() => {
                    appStore.setState({ menuOpen: true });
                  }}
                >
                  <Icon name="Menu" size={18}></Icon>
                </button>
                <strong>{appState.pageTitle}</strong>
                <button
                  aria-label={t("搜索页面")}
                  className="icon-btn"
                  onClick={() => {
                    openPalette();
                  }}
                >
                  <Icon name="Search" size={17}></Icon>
                </button>
                {can("alert:access") ? (
                  <button
                    aria-label={t("告警中心，{p0} 条未恢复", {
                      p0: appState.openAlerts.length,
                    })}
                    className="icon-btn mobile-alerts"
                    onClick={() => {
                      navigate("alerts");
                    }}
                  >
                    <Icon name="Bell" size={17}></Icon>
                    {appState.openAlerts.length ? (
                      <span className="nav-count">
                        {appState.openAlerts.length}
                      </span>
                    ) : null}
                  </button>
                ) : null}
              </header>
              <main key={appState.page} className="page">
                {appState.connectionError ? (
                  <div data-tone="crit" role="alert" className="banner">
                    <StatusGlyph tone="crit"></StatusGlyph>
                    <div className="banner-text">
                      <strong>{appState.connectionError}</strong>
                      <p>{t("当前显示上次成功加载的数据。")}</p>
                    </div>
                    <button
                      className="link"
                      onClick={() => {
                        refreshAll();
                      }}
                    >
                      {t("重试")}
                    </button>
                  </div>
                ) : null}
                {can(pagePermissions[appState.page]) ? (
                  <PageBoundary key={appState.page}>
                    <Suspense
                      fallback={
                        <div role="status" className="page-loading">
                          {t("正在加载…")}
                        </div>
                      }
                    >
                      <DynamicComponent />
                    </Suspense>
                  </PageBoundary>
                ) : (
                  <div className="panel forbidden">
                    <div className="empty">
                      <Icon name="LockKeyhole" size={28}></Icon>
                      <h3>{t("当前账号没有此页面的访问权限")}</h3>
                      <p>{t("如需访问，请联系管理员调整角色授权。")}</p>
                      <button
                        className="btn"
                        onClick={() => {
                          navigate("overview");
                        }}
                      >
                        {t("返回总览")}
                      </button>
                    </div>
                  </div>
                )}
              </main>
            </div>
          </div>
          {viewState.paletteOpen ? (
            <Modal
              title={t("跳转到页面")}
              onClose={() => {
                store.setState({ paletteOpen: false });
              }}
            >
              <label className="search command-input">
                <Icon name="Search" size={17}></Icon>
                <input
                  placeholder={t("输入页面名称")}
                  aria-label={t("搜索页面")}
                  aria-controls="command-results"
                  value={viewState.query}
                  onKeyDown={(event) => {
                    paletteKey(event);
                  }}
                  onChange={(event) => {
                    store.setState({ query: event.currentTarget.value });
                  }}
                />
              </label>
              {results.length ? (
                <ul
                  id="command-results"
                  role="listbox"
                  aria-label={t("可访问的页面")}
                  className="command-list"
                >
                  <>
                    {results.map((item, i) => (
                      <li key={item.key}>
                        <button
                          role="option"
                          aria-selected={i === viewState.cursor}
                          className={classNames("command-item", {
                            active: i === viewState.cursor,
                          })}
                          onClick={() => {
                            go(item.key);
                          }}
                          onMouseMove={() => {
                            store.setState({ cursor: i });
                          }}
                        >
                          <Icon name={item.icon}></Icon>
                          <span>{item.label}</span>
                          <small>{item.group}</small>
                        </button>
                      </li>
                    ))}
                  </>
                </ul>
              ) : (
                <p className="muted">{t("没有可访问的匹配页面。")}</p>
              )}
              <div className="command-hint">
                <span>
                  <kbd>{"↑"}</kbd> <kbd>{"↓"}</kbd>
                  {t("选择")}
                </span>
                <span>
                  <kbd>{"Enter"}</kbd>
                  {t("打开")}
                </span>
                <span>
                  <kbd>{"Esc"}</kbd>
                  {t("关闭")}
                </span>
              </div>
            </Modal>
          ) : null}
          {viewState.help ? (
            <Modal
              title={t("使用说明")}
              subtitle={`Monitor Go v${version}`}
              onClose={() => {
                store.setState({ help: false });
              }}
            >
              <dl className="kv rows">
                <div>
                  <dt>{t("主机")}</dt>
                  <dd>
                    {t(
                      "每 15 秒采集服务器本机和远程 Agent 的 CPU、内存、磁盘与网络。页面每 10 秒自动刷新，尚无历史的时段留空显示。",
                    )}
                  </dd>
                </div>
                <div>
                  <dt>{t("检测")}</dt>
                  <dd>
                    {t(
                      "由当前服务器执行 HTTP、TCP、SSL、MySQL 和 Redis 检测。连续异常达到阈值后创建告警，一次成功检测后自动恢复。",
                    )}
                  </dd>
                </div>
                <div>
                  <dt>{t("告警")}</dt>
                  <dd>
                    {t(
                      "通过邮件或 Webhook 通知。确认和静默不会改变故障状态，静默期内未发送的通知会跳过。",
                    )}
                  </dd>
                </div>
                <div>
                  <dt>{t("文件")}</dt>
                  <dd>
                    {t(
                      "保存在实例数据目录中，上传、预览、下载和管理分别授权。删除先进入回收站，永久删除需再次确认。",
                    )}
                  </dd>
                </div>
                <div>
                  <dt>{t("权限")}</dt>
                  <dd>
                    {t(
                      "成员可拥有多个角色，权限取合集。修改授权会让受影响成员重新登录。",
                    )}
                  </dd>
                </div>
              </dl>
            </Modal>
          ) : null}
        </>
      )}
      <div role="status" aria-live="polite" className="toasts">
        <>
          {appState.toasts.map((t) => (
            <div
              key={t.id}
              data-tone={t.error ? "crit" : "ok"}
              className="toast"
            >
              <StatusGlyph tone={t.error ? "crit" : "ok"}></StatusGlyph>
              <span>{t.message}</span>
            </div>
          ))}
        </>
      </div>
    </>
  );
}
