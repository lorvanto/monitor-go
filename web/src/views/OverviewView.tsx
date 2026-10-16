import { alertTitle } from "../lib/incidentText";
import {
  groupName,
  groupOption,
  matchesGroup,
  groupQuery,
} from "../lib/monitorGroups";
import { errorMessage, type DisplayMessage } from "../lib/messages";
import { hostName } from "../lib/resourceLabels";
import { useEffect, useRef, useState, type CSSProperties } from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import LineChart from "../components/LineChart";
import SignalTrace from "../components/SignalTrace";
import StatusBadge from "../components/StatusBadge";
import StatusGlyph from "../components/StatusGlyph";
import { i18nStore, localizedLabels, metadataLabel, t, useI18n } from "../i18n";
import { readings, stamp } from "../lib/chart";
import { classNames } from "../lib/react";
import { severityRank, toneOf, type Tone } from "../lib/status";
import {
  api,
  appStore,
  can,
  getAppView,
  hours,
  latencyParts,
  navigate,
  number,
  relative,
  startMonitorCreate,
  useApp,
  type Host,
  type Overview,
} from "../stores/app";
export default function OverviewView() {
  const initialState = {
    period: "24h",
    scope: "",
    summary: { at: 0, points: [], activity: [] } as Overview,
    overall: null as Overview | null,
    error: "" as DisplayMessage,
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const sequence = useRef(0);
  const appState = useApp();
  const i18nState = useI18n();
  const getGroups = () => [
    ...new Set(appStore.getState().data.monitors.map((m) => m.group)),
  ];
  const getScoped = () =>
    appStore
      .getState()
      .data.monitors.filter((m) =>
        matchesGroup(m.group, store.getState().scope),
      );
  const getTotals = () => store.getState().overall || store.getState().summary;
  const count = (status: string) =>
    appStore.getState().data.monitors.filter((m) => m.status === status).length;
  const getOnline = () =>
    appStore.getState().data.hosts.filter((h) => h.status === "online").length;
  const getOffline = () =>
    appStore.getState().data.hosts.filter((h) => h.status === "offline").length;
  const getCritical = () =>
    getAppView().openAlerts.filter((a) => a.severity === "critical").length;
  const getState = () => {
    const failed = count("failed"),
      warning = count("warning"),
      problems = failed + getOffline();
    const incompleteDisks = appStore
      .getState()
      .data.hosts.filter(
        (h) => h.status === "online" && h.snapshot.diskIncomplete,
      ).length;
    const facts: {
      tone: Tone;
      text: string;
    }[] = [];
    if (failed)
      facts.push({ tone: "crit", text: t("{p0} 项检测异常", { p0: failed }) });
    if (getOffline())
      facts.push({
        tone: "crit",
        text: t("{p0} 台主机离线", { p0: getOffline() }),
      });
    if (warning)
      facts.push({
        tone: "warn",
        text: t("{p0} 项检测需关注", { p0: warning }),
      });
    if (getAppView().openAlerts.length)
      facts.push({
        tone: getCritical() ? "crit" : "warn",
        text: t("{p0} 条告警未恢复", { p0: getAppView().openAlerts.length }),
      });
    if (count("healthy"))
      facts.push({
        tone: "ok",
        text: t("{p0} 项检测正常", { p0: count("healthy") }),
      });
    if (getOnline())
      facts.push({
        tone: "ok",
        text: t("{p0} 台主机在线", { p0: getOnline() }),
      });
    if (incompleteDisks)
      facts.push({
        tone: "warn",
        text: t("{p0} 台主机磁盘采样不完整", { p0: incompleteDisks }),
      });
    if (count("paused"))
      facts.push({
        tone: "paused",
        text: t("{p0} 项检测已暂停", { p0: count("paused") }),
      });
    if (count("unknown"))
      facts.push({
        tone: "idle",
        text: t("{p0} 项等待首次检测", { p0: count("unknown") }),
      });
    if (!can("monitor:read") && !can("host:read") && !can("alert:read"))
      return {
        tone: "idle" as Tone,
        title: t("当前账号无法查看监控数据"),
        facts: [
          {
            tone: "idle" as Tone,
            text: t(
              "主机、检测和告警需要相应的查看权限，可从左侧进入有权限的页面",
            ),
          },
        ],
      };
    if (problems)
      return {
        tone: "crit" as Tone,
        title: t("{p0} 项异常需要处理", { p0: problems }),
        facts,
      };
    if (warning)
      return {
        tone: "warn" as Tone,
        title: t("{p0} 项检测需要关注", { p0: warning }),
        facts,
      };
    if (getAppView().openAlerts.length)
      return {
        tone: "warn" as Tone,
        title: t("{p0} 条告警尚未恢复", { p0: getAppView().openAlerts.length }),
        facts,
      };
    if (incompleteDisks)
      return {
        tone: "warn" as Tone,
        title: t("{p0} 台主机磁盘采样不完整", { p0: incompleteDisks }),
        facts,
      };
    if (
      !appStore.getState().data.monitors.length &&
      !appStore.getState().data.hosts.length
    )
      return {
        tone: "idle" as Tone,
        title: t("还没有监控数据"),
        facts: [
          {
            tone: "idle" as Tone,
            text: t("添加检测目标或接入远程主机后，状态会显示在这里"),
          },
        ],
      };
    return { tone: "ok" as Tone, title: t("全部运行正常"), facts };
  };
  const time = (at: number) =>
    at
      ? new Date(at).toLocaleTimeString(i18nStore.getState().locale, {
          hour12: false,
        })
      : "—";
  const getBoard = () =>
    [...getScoped()]
      .sort(
        (a, b) =>
          severityRank(a.status) - severityRank(b.status) ||
          a.name.localeCompare(b.name, i18nStore.getState().locale),
      )
      .slice(0, 8);
  const getLatency = () => readings(store.getState().summary.points);
  const getAlertRows = () =>
    [...getAppView().openAlerts]
      .sort(
        (a, b) =>
          Number(b.severity === "critical") -
            Number(a.severity === "critical") || b.openedAt - a.openedAt,
      )
      .slice(0, 6);
  const diskMax = (h: Host) =>
    h.snapshot.disks.length
      ? Math.max(...h.snapshot.disks.map((d) => d.percent))
      : null;
  const hostRank = (h: Host) =>
    ({ offline: 0, online: 1, pending: 2, revoked: 3 })[h.status];
  const getHostRows = () =>
    [...appStore.getState().data.hosts]
      .sort(
        (a, b) =>
          hostRank(a) - hostRank(b) ||
          Math.max(
            b.snapshot.cpu || 0,
            b.snapshot.memory || 0,
            diskMax(b) || 0,
          ) -
            Math.max(
              a.snapshot.cpu || 0,
              a.snapshot.memory || 0,
              diskMax(a) || 0,
            ),
      )
      .slice(0, 6);
  const usage = (h: Host) => {
    const pending = h.status === "pending";
    const disk = pending ? null : diskMax(h);
    return [
      { label: "CPU", value: pending ? null : h.snapshot.cpu, warn: false },
      {
        label: t("内存"),
        value: pending ? null : h.snapshot.memory,
        warn: false,
      },
      {
        label: t("磁盘"),
        value: disk,
        incomplete: h.status === "online" && h.snapshot.diskIncomplete,
        warn:
          disk != null &&
          (h.diskAlertPercent ?? 85) > 0 &&
          disk >= (h.diskAlertPercent ?? 85),
      },
    ];
  };
  const hostState: Record<Host["status"], string> = localizedLabels({
    online: "在线",
    offline: "离线",
    pending: "待接入",
    revoked: "已撤销",
  });
  const percent = (v: number | null | undefined) =>
    v == null ? "—" : `${number(v, v >= 99.95 || v < 10 ? 1 : 0)}%`;
  function openAlert(id: string) {
    appStore.setState({ activeAlert: id });
    navigate("alerts");
  }
  function openMonitor(id: string) {
    appStore.setState({ activeMonitor: id });
    navigate("monitors");
  }
  function openHost(id: string) {
    appStore.setState({ activeHost: id });
    navigate("host");
  }
  const minutes = (from: number) =>
    Math.max(1, Math.floor((Date.now() - from) / 60000));
  const duration = (from: number) => {
    const m = minutes(from);
    return m < 60
      ? t("持续 {p0} 分钟", { p0: m })
      : m < 1440
        ? t("持续 {p0} 小时", { p0: Math.floor(m / 60) })
        : t("持续 {p0} 天", { p0: Math.floor(m / 1440) });
  };
  useEffect(() => {
    void (async () => {
      const request = ++sequence.current;
      try {
        // Short-range availability stays at 24 hours; long ranges use hourly summaries.
        const [v, all] = await Promise.all([
          api<Overview>(
            `/overview?hours=${hours(store.getState().period)}${groupQuery(store.getState().scope)}`,
          ),
          store.getState().scope
            ? api<Overview>(
                `/overview?hours=${hours(store.getState().period) >= 720 ? hours(store.getState().period) : 1}`,
              )
            : null,
        ]);
        if (request === sequence.current) {
          store.setState({ summary: v });
          store.setState({ overall: all });
          store.setState({ error: "" });
        }
      } catch (e) {
        if (request === sequence.current)
          store.setState({ error: errorMessage(e) });
      }
    })();
  }, [viewState.period, viewState.scope, appState.updatedAt]);

  const groups = getGroups();
  const scoped = getScoped();
  const totals = getTotals();
  const online = getOnline();
  const offline = getOffline();
  const critical = getCritical();
  const state = getState();
  const board = getBoard();
  const latency = getLatency();
  const alertRows = getAlertRows();
  const hostRows = getHostRows();
  return (
    <>
      <div className="overview">
        <header className="page-head">
          <div className="page-title">
            <h1>{appState.pageTitle}</h1>
          </div>
          <div className="page-actions">
            {can("monitor:update") ? (
              <button
                className="btn primary"
                onClick={() => {
                  startMonitorCreate();
                }}
              >
                <Icon name="Plus"></Icon>
                {t("添加检测")}
              </button>
            ) : null}
          </div>
        </header>
        {viewState.error ? (
          <div data-tone="warn" role="alert" className="banner">
            <StatusGlyph tone="warn"></StatusGlyph>
            <div className="banner-text">
              <strong>{t("总览数据未能刷新")}</strong>
              <p>{viewState.error}</p>
            </div>
          </div>
        ) : null}
        <section data-tone={state.tone} aria-live="polite" className="state">
          <StatusGlyph tone={state.tone} size={16}></StatusGlyph>
          <h2>{state.title}</h2>
          <ul className="state-facts">
            <>
              {state.facts.map((f) => (
                <li key={f.text}>
                  <StatusGlyph tone={f.tone} size={9}></StatusGlyph>
                  {f.text}
                </li>
              ))}
            </>
          </ul>
          <p className="state-meta">
            {t("更新于 {p0}", { p0: time(appState.updatedAt) })}
          </p>
        </section>
        <section aria-label={t("关键指标")} className="readout">
          <div className="readout-item">
            <span className="readout-label">{t("在线主机")}</span>
            <span className="readout-value">
              {can("host:read") ? online : "—"}
              {can("host:read") ? (
                <small>
                  {"/ "}
                  {appState.data.hosts.length}
                </small>
              ) : null}
            </span>
            <span className="readout-note">
              {!can("host:read") ? (
                <>{t("当前账号无查看权限")}</>
              ) : offline ? (
                <>
                  <span>
                    <StatusGlyph tone="crit" size={8}></StatusGlyph>
                    {t("{p0} 台离线", { p0: offline })}
                  </span>
                </>
              ) : appState.data.hosts.length ? (
                <>{t("全部在线")}</>
              ) : (
                <>{t("尚未接入主机")}</>
              )}
              {appState.data.hosts.some((h) => h.status === "pending") ? (
                <span>
                  <StatusGlyph tone="idle" size={8}></StatusGlyph>
                  {t("{p0} 台待接入", {
                    p0: appState.data.hosts.filter(
                      (h) => h.status === "pending",
                    ).length,
                  })}
                </span>
              ) : null}
            </span>
          </div>
          <div className="readout-item">
            <span className="readout-label">{t("检测正常")}</span>
            <span className="readout-value">
              {can("monitor:read") ? count("healthy") : "—"}
              {can("monitor:read") ? (
                <small>
                  {"/ "}
                  {appState.data.monitors.length}
                </small>
              ) : null}
            </span>
            <span className="readout-note">
              {!can("monitor:read") ? (
                <>{t("当前账号无查看权限")}</>
              ) : count("failed") || count("warning") ? (
                <>
                  {count("failed") ? (
                    <span>
                      <StatusGlyph tone="crit" size={8}></StatusGlyph>
                      {t("{p0} 异常", { p0: count("failed") })}
                    </span>
                  ) : null}
                  {count("warning") ? (
                    <span>
                      <StatusGlyph tone="warn" size={8}></StatusGlyph>
                      {t("{p0} 需关注", { p0: count("warning") })}
                    </span>
                  ) : null}
                </>
              ) : appState.data.monitors.length ? (
                <>{t("没有异常检测")}</>
              ) : (
                <>{t("尚未添加检测")}</>
              )}
            </span>
          </div>
          <div className="readout-item">
            <span className="readout-label">
              {hours(viewState.period) >= 720
                ? t("{p0} 天可用率", { p0: hours(viewState.period) / 24 })
                : t("24 小时可用率")}
            </span>
            <span className="readout-value">
              {number(totals.uptime, 2)}
              {totals.uptime != null ? <small>{"%"}</small> : null}
            </span>
            <span className="readout-note">
              {totals.samples
                ? t("按 {p0} 次检测计算", {
                    p0: totals.samples.toLocaleString(i18nState.locale),
                  })
                : t("暂无检测记录")}
            </span>
          </div>
          <div className="readout-item">
            <span className="readout-label">{t("未恢复告警")}</span>
            <span className="readout-value">
              {can("alert:read") ? appState.openAlerts.length : "—"}
            </span>
            <span className="readout-note">
              {!can("alert:read") ? (
                <>{t("当前账号无查看权限")}</>
              ) : appState.openAlerts.length ? (
                <>
                  {critical ? (
                    <span>
                      <StatusGlyph tone="crit" size={8}></StatusGlyph>
                      {t("{p0} 严重", { p0: critical })}
                    </span>
                  ) : null}
                  {appState.openAlerts.length - critical ? (
                    <span>
                      <StatusGlyph tone="warn" size={8}></StatusGlyph>
                      {t("{p0} 警告", {
                        p0: appState.openAlerts.length - critical,
                      })}
                    </span>
                  ) : null}
                </>
              ) : (
                <>{t("没有待处理事件")}</>
              )}
            </span>
          </div>
        </section>
        {can("monitor:read") ? (
          <section className="panel checks-panel">
            <header className="panel-head ruled">
              <div>
                <h2>{t("服务检测")}</h2>
                <p>
                  {t("{p0}的平均响应时间与最近检测结果", {
                    p0: viewState.scope
                      ? groupName(viewState.scope.slice("group:".length))
                      : t("全部分组"),
                  })}
                </p>
              </div>
              {appState.data.monitors.length ? (
                <div className="panel-tools">
                  <div role="group" aria-label={t("时间范围")} className="seg">
                    <>
                      {[
                        { value: "1h", label: t("1 小时") },
                        { value: "24h", label: t("24 小时") },
                        { value: "7d", label: t("7 天") },
                        { value: "30d", label: t("30 天") },
                        { value: "90d", label: t("90 天") },
                      ].map((p) => (
                        <button
                          key={p.value}
                          aria-pressed={viewState.period === p.value}
                          className={classNames({
                            active: viewState.period === p.value,
                          })}
                          onClick={() => {
                            store.setState({
                              period: p.value,
                              summary: { at: 0, points: [], activity: [] },
                              overall: null,
                            });
                          }}
                        >
                          {p.label}
                        </button>
                      ))}
                    </>
                  </div>
                  {groups.length > 1 ? (
                    <select
                      aria-label={t("检测分组")}
                      value={viewState.scope}
                      onChange={(event) => {
                        store.setState({ scope: event.currentTarget.value });
                      }}
                    >
                      <option value="">{t("全部分组")}</option>
                      <>
                        {groups.map((g) => (
                          <option key={g} value={groupOption(g)}>
                            {groupName(g)}
                          </option>
                        ))}
                      </>
                    </select>
                  ) : null}
                </div>
              ) : null}
            </header>
            {!appState.data.monitors.length ? (
              <div className="empty">
                <Icon name="Radar" size={28}></Icon>
                <h3>{t("还没有检测目标")}</h3>
                <p>
                  {t(
                    "添加网站或服务地址后，当前服务器会按设定间隔检测，响应趋势和每次结果都会显示在这里。",
                  )}
                </p>
                {can("monitor:update") ? (
                  <button
                    className="btn primary"
                    onClick={() => {
                      startMonitorCreate();
                    }}
                  >
                    <Icon name="Plus"></Icon>
                    {t("添加检测")}
                  </button>
                ) : null}
              </div>
            ) : (
              <div className="checks-body">
                <div className="checks-chart">
                  <LineChart
                    series={[
                      {
                        name: t("平均响应"),
                        points: viewState.summary.points,
                        color: "var(--data-1)",
                      },
                    ]}
                    hours={hours(viewState.period)}
                    end={viewState.summary.at || undefined}
                    format={(v) => `${number(v)} ms`}
                    tickFormat={(v) => `${v}`}
                    label={t("平均响应时间（毫秒）")}
                    height={200}
                  ></LineChart>
                  <dl className="stats">
                    <>
                      {[
                        { label: t("最新"), value: latency?.latest },
                        { label: t("区间最高"), value: latency?.max },
                        { label: t("区间最低"), value: latency?.min },
                        {
                          label: t("所选时段平均"),
                          value: viewState.summary.latency,
                        },
                      ].map((item) => (
                        <div key={item.label}>
                          <dt>{item.label}</dt>
                          <dd>
                            {latencyParts(item.value)[0]}
                            <small>{latencyParts(item.value)[1]}</small>
                          </dd>
                        </div>
                      ))}
                    </>
                  </dl>
                </div>
                <div className="checks-board">
                  <div className="board-head">
                    <span>{t("最近 24 次结果，异常越严重柱越高")}</span>
                  </div>
                  {board.length ? (
                    <ol className="board">
                      <>
                        {board.map((m) => (
                          <li key={m.id}>
                            <button
                              className="board-row"
                              onClick={() => {
                                openMonitor(m.id);
                              }}
                            >
                              <StatusGlyph
                                tone={toneOf(m.status)}
                              ></StatusGlyph>
                              <span
                                title={`${m.type}　${m.target}`}
                                className="board-name"
                              >
                                <strong>{m.name}</strong>
                              </span>
                              <SignalTrace
                                history={m.history}
                                slots={24}
                              ></SignalTrace>
                              <span className="num">
                                {latencyParts(m.latency)[0]}
                                <small className="unit">
                                  {latencyParts(m.latency)[1]}
                                </small>
                              </span>
                            </button>
                          </li>
                        ))}
                      </>
                    </ol>
                  ) : (
                    <div className="empty compact">
                      <h3>
                        {appState.data.monitors.length
                          ? t("该分组没有检测")
                          : t("还没有检测目标")}
                      </h3>
                      <p>{t("添加网站或服务地址后，结果会显示在这里。")}</p>
                    </div>
                  )}
                  {scoped.length ? (
                    <footer className="board-foot">
                      <button
                        className="link"
                        onClick={() => {
                          navigate("monitors");
                        }}
                      >
                        {t("查看全部 {p0} 项检测", { p0: scoped.length })}
                      </button>
                    </footer>
                  ) : null}
                </div>
              </div>
            )}
          </section>
        ) : null}
        <div className="overview-grid">
          <div className="overview-col">
            {can("alert:read") ? (
              <section className="panel">
                <header className="panel-head">
                  <h2>
                    {t("需要关注")}
                    <span className="count">{appState.openAlerts.length}</span>
                  </h2>
                  <button
                    className="link"
                    onClick={() => {
                      navigate("alerts");
                    }}
                  >
                    {t("告警中心")}
                  </button>
                </header>
                {alertRows.length ? (
                  <ul className="event-list">
                    <>
                      {alertRows.map((a) => (
                        <li key={a.id}>
                          <button
                            className="event-row"
                            onClick={() => {
                              openAlert(a.id);
                            }}
                          >
                            <StatusBadge
                              tone={a.severity === "critical" ? "crit" : "warn"}
                              label={
                                a.severity === "critical"
                                  ? t("严重")
                                  : t("警告")
                              }
                            ></StatusBadge>
                            <span className="event-text">
                              <strong>{alertTitle(a)}</strong>
                              <small>
                                {a.resource} {duration(a.openedAt)}
                              </small>
                            </span>
                            <span className="event-aside">
                              <span>
                                {a.state === "acknowledged"
                                  ? t("{p0} 处理中", {
                                      p0: a.owner || t("已认领"),
                                    })
                                  : t("等待认领")}
                              </span>
                              {a.silenceUntil > Date.now() ? (
                                <span>{t("通知已静默")}</span>
                              ) : null}
                            </span>
                          </button>
                        </li>
                      ))}
                    </>
                  </ul>
                ) : (
                  <div className="empty compact">
                    <StatusGlyph tone="ok" size={14}></StatusGlyph>
                    <h3>{t("当前没有未恢复的告警")}</h3>
                    <p>
                      {t(
                        "检测连续异常达到阈值或主机离线时，事件会出现在这里。",
                      )}
                    </p>
                  </div>
                )}
              </section>
            ) : null}
          </div>
          <div className="overview-col">
            {can("host:read") ? (
              <section className="panel">
                <header className="panel-head">
                  <h2>
                    {t("主机")}
                    <span className="count">{appState.data.hosts.length}</span>
                  </h2>
                  <button
                    className="link"
                    onClick={() => {
                      navigate("host");
                    }}
                  >
                    {t("主机详情")}
                  </button>
                </header>
                {hostRows.length ? (
                  <div className="host-rows">
                    <div aria-hidden="true" className="host-row head">
                      <span></span>
                      <span>{t("主机")}</span>
                      <span>{"CPU"}</span>
                      <span>{t("内存")}</span>
                      <span>{t("磁盘")}</span>
                    </div>
                    <>
                      {hostRows.map((h) => (
                        <button
                          key={h.id}
                          aria-label={`${hostName(h)}，${hostState[h.status]}，${usage(
                            h,
                          )
                            .map((u) => `${u.label} ${percent(u.value)}`)
                            .join("，")}`}
                          className="host-row"
                          onClick={() => {
                            openHost(h.id);
                          }}
                        >
                          <StatusGlyph tone={toneOf(h.status)}></StatusGlyph>
                          <span className="board-name">
                            <strong>{hostName(h)}</strong>
                            <small>
                              {hostState[h.status]}{" "}
                              {h.status === "online"
                                ? relative(h.lastSeen)
                                : h.hostname}
                            </small>
                          </span>
                          <>
                            {usage(h).map((u) => (
                              <span
                                key={u.label}
                                className={classNames("host-cell", {
                                  stale: h.status !== "online",
                                })}
                              >
                                <span>{percent(u.value)}</span>
                                {u.incomplete ? (
                                  <small>{t("磁盘采样不完整")}</small>
                                ) : null}
                                <span
                                  data-tone={u.warn ? "warn" : undefined}
                                  className="meter"
                                >
                                  <i
                                    style={
                                      {
                                        width: (u.value || 0) + "%",
                                      } as CSSProperties
                                    }
                                  ></i>
                                </span>
                              </span>
                            ))}
                          </>
                        </button>
                      ))}
                    </>
                  </div>
                ) : (
                  <div className="empty compact">
                    <h3>{t("还没有主机")}</h3>
                    <p>{t("服务器本机会自动采集，也可以接入远程 Agent。")}</p>
                  </div>
                )}
              </section>
            ) : null}
            {can("audit:read") ? (
              <section className="panel">
                <header className="panel-head">
                  <h2>{t("最近操作")}</h2>
                  <button
                    className="link"
                    onClick={() => {
                      navigate("audit");
                    }}
                  >
                    {t("操作审计")}
                  </button>
                </header>
                {viewState.summary.activity.length ? (
                  <ul className="log">
                    <>
                      {viewState.summary.activity.slice(0, 6).map((a) => (
                        <li key={a.at + a.message}>
                          <time>{stamp(a.at)}</time>
                          <p
                            title={
                              a.action
                                ? `${metadataLabel(a.action)} · ${a.resource || ""}`
                                : a.message
                            }
                            className="ellipsis"
                          >
                            {a.action
                              ? `${metadataLabel(a.action)} · ${a.resource || ""}`
                              : a.message}
                          </p>
                          <span className="log-actor">{a.actor}</span>
                        </li>
                      ))}
                    </>
                  </ul>
                ) : (
                  <div className="empty compact">
                    <p>{t("暂无操作记录")}</p>
                  </div>
                )}
              </section>
            ) : null}
          </div>
        </div>
      </div>
    </>
  );
}
