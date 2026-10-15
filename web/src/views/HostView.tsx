import { hostName } from "../lib/resourceLabels";
import { useEffect, useRef, useState, type CSSProperties } from "react";
import { createStore, useStore } from "zustand";
import AgentAccess from "../components/AgentAccess";
import Icon from "../components/Icon";
import LineChart from "../components/LineChart";
import Modal from "../components/Modal";
import StatusBadge from "../components/StatusBadge";
import StatusGlyph from "../components/StatusGlyph";
import { i18nStore, localizedLabels, t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import { toneOf } from "../lib/status";
import {
  action,
  api,
  appStore,
  bytes,
  can,
  dateTime,
  hours,
  navigate,
  notify,
  number,
  refreshAll,
  relative,
  useApp,
  type Host,
  type HostMetric,
} from "../stores/app";
export default function HostView() {
  const initialState = {
    period: "24h",
    edit: false,
    draft: "",
    editTarget: undefined as Host | undefined,
    metricsFor: "",
    metrics: [] as HostMetric[],
    error: "",
    saving: false,
    hostQuery: "",
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const seq = useRef(0);
  const appState = useApp();
  useI18n();
  const getHost = () =>
    appStore
      .getState()
      .data.hosts.find((h) => h.id === appStore.getState().activeHost) ||
    appStore.getState().data.hosts[0];
  const stateLabel: Record<Host["status"], string> = localizedLabels({
    online: "在线",
    offline: "离线",
    pending: "待接入",
    revoked: "已撤销",
  });
  const hostRank = (h: Host) =>
    ({ offline: 0, online: 1, pending: 2, revoked: 3 })[h.status];
  const getHostList = () =>
    [...appStore.getState().data.hosts]
      .filter((h) =>
        `${hostName(h)} ${h.hostname} ${h.inventory.ips.join(" ")}`
          .toLowerCase()
          .includes(store.getState().hostQuery.trim().toLowerCase()),
      )
      .sort(
        (a, b) =>
          hostRank(a) - hostRank(b) ||
          hostName(a).localeCompare(hostName(b), i18nStore.getState().locale),
      );
  const getWarning = () =>
    appStore
      .getState()
      .data.alerts.find(
        (a) =>
          a.kind === "host" &&
          a.resourceId === getHost()?.id &&
          a.state !== "resolved",
      );
  function startEdit() {
    if (!getHost()) return;
    store.setState({ draft: hostName(getHost()) });
    store.setState({ editTarget: getHost() });
    store.setState({ edit: true });
  }
  async function save() {
    const target = store.getState().editTarget;
    if (!target || store.getState().saving) return;
    store.setState({ saving: true });
    if (
      await action(
        () =>
          api(`/hosts/${target.id}`, "PATCH", {
            alias: store.getState().draft,
            version: target.version,
          }),
        t("主机备注已保存"),
      )
    )
      store.setState({ edit: false });
    store.setState({ saving: false });
  }
  async function copyIP() {
    const ip = getHost()?.inventory.ips.join(", ") || t("无可用地址");
    try {
      await navigator.clipboard.writeText(ip);
      notify(t("已复制主机地址"));
    } catch {
      notify(ip);
    }
  }
  const metricScope = () => `${getHost()?.id || ""}:${store.getState().period}`;
  const series = (key: "cpu" | "memory" | "netIn" | "netOut") =>
    (store.getState().metricsFor === metricScope()
      ? store.getState().metrics
      : []
    ).map((m) => ({ at: m.at, value: m[key] }));
  const uptime = (seconds: number) =>
    t("{p0} 天 {p1} 小时", {
      p0: Math.floor(seconds / 86400),
      p1: Math.floor((seconds % 86400) / 3600),
    });
  const percent = (v: number) => `${number(v, v >= 99.95 || v < 10 ? 1 : 0)}%`;
  const rate = (v: number) => `${bytes(v)}/s`;
  const rateTick = (v: number) => (v ? bytes(v).replace(".0 ", " ") : "0");
  useEffect(() => {
    const id = getHost()?.id;
    if (id) appStore.setState({ activeHost: id });
  }, [getHost()?.id]);
  useEffect(() => {
    const id = ++seq.current;
    const host = getHost();
    const scope = metricScope();
    store.setState({ error: "" });
    void (async () => {
      if (!host) return;
      try {
        const v = await api<HostMetric[]>(
          `/hosts/${host.id}/metrics?hours=${hours(store.getState().period)}`,
        );
        if (id === seq.current) {
          store.setState({ metrics: v, metricsFor: scope });
          store.setState({ error: "" });
        }
      } catch (e) {
        if (id === seq.current) store.setState({ error: (e as Error).message });
      }
    })();
    return () => {
      seq.current++;
    };
  }, [getHost()?.id, viewState.period, appState.updatedAt]);

  const host = getHost();
  const hostList = getHostList();
  const warning = getWarning();
  return (
    <>
      <div className="host-layout">
        <aside aria-label={t("主机列表")} className="panel host-index">
          <header className="panel-head">
            <h2>
              {t("主机")}
              <span className="count">{appState.data.hosts.length}</span>
            </h2>
          </header>
          {appState.data.hosts.length > 6 ? (
            <label className="search">
              <Icon name="Search" size={15}></Icon>
              <input
                placeholder={t("名称或地址")}
                aria-label={t("筛选主机")}
                value={viewState.hostQuery}
                onChange={(event) => {
                  store.setState({ hostQuery: event.currentTarget.value });
                }}
              />
            </label>
          ) : null}
          <div className="list">
            <>
              {hostList.map((h) => (
                <button
                  key={h.id}
                  aria-current={h.id === host?.id ? "true" : undefined}
                  className={classNames("list-item", {
                    active: h.id === host?.id,
                  })}
                  onClick={() => {
                    appStore.setState({ activeHost: h.id });
                  }}
                >
                  <StatusGlyph tone={toneOf(h.status)}></StatusGlyph>
                  <span className="list-text">
                    <strong>{hostName(h)}</strong>
                    <small>
                      {h.source === "agent" ? t("远程 Agent") : t("服务器本机")}
                    </small>
                  </span>
                  <span className="list-aside">
                    {h.status === "online" && h.snapshot.cpu != null
                      ? `CPU ${number(h.snapshot.cpu, 0)}%`
                      : stateLabel[h.status]}
                  </span>
                </button>
              ))}
            </>
            {!hostList.length ? (
              <p className="empty compact">{t("没有匹配的主机")}</p>
            ) : null}
          </div>
        </aside>
        <div className="host-detail">
          {appState.data.hosts.length ? (
            <label className="field host-picker">
              {t("选择主机")}
              <select
                value={appState.activeHost}
                onChange={(event) => {
                  appStore.setState({ activeHost: event.currentTarget.value });
                }}
              >
                <>
                  {hostList.map((h) => (
                    <option key={h.id} value={h.id}>
                      {hostName(h)}
                      {"（"}
                      {stateLabel[h.status]}
                      {"） "}
                    </option>
                  ))}
                </>
              </select>
            </label>
          ) : null}
          {!host ? (
            <>
              <header className="page-head">
                <div className="page-title">
                  <h1>{t("主机")}</h1>
                </div>
                <div className="page-actions">
                  <AgentAccess></AgentAccess>
                </div>
              </header>
              <div className="panel empty">
                <Icon name="Server" size={28}></Icon>
                <h3>{t("等待主机首次采样")}</h3>
                <p>
                  {t("服务器本机采集开启后会自动出现，也可以接入远程 Agent。")}
                </p>
                <button
                  className="btn"
                  onClick={() => {
                    refreshAll();
                  }}
                >
                  {t("刷新")}
                </button>
              </div>
            </>
          ) : (
            <>
              <header className="page-head">
                <div className="page-title">
                  <h1>
                    {hostName(host)}
                    <StatusBadge
                      status={host.status}
                      label={stateLabel[host.status]}
                    ></StatusBadge>
                  </h1>
                  <p>
                    {host.alias ? (
                      <span>
                        {host.hostname}
                        {"，"}
                      </span>
                    ) : null}
                    {host.source === "agent"
                      ? t("远程 Agent 上报")
                      : t("服务器本机采集")}
                    {host.lastSeen ? (
                      <>
                        {t("，最后上报于 {p0}", {
                          p0: relative(host.lastSeen),
                        })}
                      </>
                    ) : null}
                  </p>
                </div>
                <div className="page-actions">
                  {can("host:update") ? (
                    <button
                      className="btn"
                      onClick={() => {
                        startEdit();
                      }}
                    >
                      <Icon name="Pencil"></Icon>
                      {t("编辑备注")}
                    </button>
                  ) : null}
                  <AgentAccess host={host}></AgentAccess>
                </div>
              </header>
              {host.status === "pending" || host.status === "revoked" ? (
                <div data-tone="idle" className="banner">
                  <StatusGlyph tone="idle"></StatusGlyph>
                  <div className="banner-text">
                    <p>
                      {host.status === "pending"
                        ? t(
                            "等待 Agent 注册并首次上报。接入码过期后可使用“重新接入”签发新的接入码。",
                          )
                        : t(
                            "该主机的 Agent 接入已撤销，以下保留最后一次采样与历史数据。",
                          )}
                    </p>
                  </div>
                </div>
              ) : null}
              {warning ? (
                <div
                  data-tone={warning.severity === "critical" ? "crit" : "warn"}
                  className="banner"
                >
                  <StatusGlyph
                    tone={warning.severity === "critical" ? "crit" : "warn"}
                  ></StatusGlyph>
                  <div className="banner-text">
                    <strong>{warning.title}</strong>
                    <p>{warning.description}</p>
                  </div>
                  {can("alert:access") ? (
                    <button
                      className="link"
                      onClick={() => {
                        appStore.setState({ activeAlert: warning.id });
                        navigate("alerts");
                      }}
                    >
                      {t("处理告警")}
                    </button>
                  ) : null}
                </div>
              ) : null}
              {viewState.error || host.snapshot.errors.length ? (
                <div data-tone="warn" className="banner">
                  <StatusGlyph tone="warn"></StatusGlyph>
                  <div className="banner-text">
                    <strong>{t("部分指标未能采集")}</strong>
                    <p>{viewState.error || host.snapshot.errors.join("；")}</p>
                  </div>
                </div>
              ) : null}
              <dl className="facts six">
                <div>
                  <dt>{t("主机地址")}</dt>
                  <dd>
                    <span
                      title={host.inventory.ips.join(", ")}
                      className="mono"
                    >
                      {host.inventory.ips[0] || "—"}
                    </span>
                    {host.inventory.ips.length ? (
                      <button
                        aria-label={t("复制主机地址")}
                        title={t("复制全部地址")}
                        className="icon-btn sm"
                        onClick={() => {
                          copyIP();
                        }}
                      >
                        <Icon name="Copy" size={13}></Icon>
                      </button>
                    ) : null}
                  </dd>
                </div>
                <div>
                  <dt>{t("操作系统")}</dt>
                  <dd>
                    <span>{host.inventory.os || "—"}</span>
                  </dd>
                </div>
                <div>
                  <dt>{t("处理器")}</dt>
                  <dd>
                    <span>
                      {t("{p0} 核 {p1}", {
                        p0: host.inventory.cores,
                        p1: host.inventory.arch,
                      })}
                    </span>
                  </dd>
                </div>
                <div>
                  <dt>{t("内存")}</dt>
                  <dd>
                    <span>{bytes(host.inventory.memory)}</span>
                  </dd>
                </div>
                <div>
                  <dt>{t("运行时长")}</dt>
                  <dd>
                    <span>{uptime(host.snapshot.uptime)}</span>
                  </dd>
                </div>
                <div>
                  <dt>{t("采集方式")}</dt>
                  <dd>
                    <span>
                      {host.source === "agent"
                        ? t("远程 Agent，每 15 秒")
                        : t("本机，每 15 秒")}
                    </span>
                  </dd>
                </div>
              </dl>
              <div className="section-bar">
                <h2>{t("资源趋势")}</h2>
                <div role="group" aria-label={t("时间范围")} className="seg">
                  <>
                    {[
                      { value: "1h", label: t("1 小时") },
                      { value: "24h", label: t("24 小时") },
                      { value: "7d", label: t("7 天") },
                    ].map((p) => (
                      <button
                        key={p.value}
                        aria-pressed={viewState.period === p.value}
                        className={classNames({
                          active: viewState.period === p.value,
                        })}
                        onClick={() => {
                          store.setState({ period: p.value });
                        }}
                      >
                        {p.label}
                      </button>
                    ))}
                  </>
                </div>
              </div>
              <div className="metric-grid">
                <section className="panel metric-panel">
                  <header className="panel-head">
                    <div>
                      <h2>{t("CPU 使用率")}</h2>
                      <p>
                        {t("{p0} 个逻辑核心", { p0: host.inventory.cores })}
                      </p>
                    </div>
                    <div className="metric-now">
                      <strong>{number(host.snapshot.cpu)}</strong>
                      <span>{"%"}</span>
                    </div>
                  </header>
                  <LineChart
                    series={[
                      {
                        name: "CPU",
                        points: series("cpu"),
                        color: "var(--data-1)",
                      },
                    ]}
                    hours={hours(viewState.period)}
                    max={100}
                    format={percent}
                    tickFormat={(v) => `${v}%`}
                    label={t("CPU 使用率")}
                  ></LineChart>
                </section>
                <section className="panel metric-panel">
                  <header className="panel-head">
                    <div>
                      <h2>{t("内存使用率")}</h2>
                      <p>
                        {t("已用 {p0}，共 {p1}", {
                          p0: bytes(host.snapshot.memoryUsed),
                          p1: bytes(host.inventory.memory),
                        })}
                      </p>
                    </div>
                    <div className="metric-now">
                      <strong>{number(host.snapshot.memory)}</strong>
                      <span>{"%"}</span>
                    </div>
                  </header>
                  <LineChart
                    series={[
                      {
                        name: t("内存"),
                        points: series("memory"),
                        color: "var(--data-1)",
                      },
                    ]}
                    hours={hours(viewState.period)}
                    max={100}
                    format={percent}
                    tickFormat={(v) => `${v}%`}
                    label={t("内存使用率")}
                  ></LineChart>
                </section>
              </div>
              <section className="panel metric-panel">
                <header className="panel-head">
                  <div>
                    <h2>{t("网络流量")}</h2>
                    <p>{t("非回环接口合计，按相邻两次采样的计数差计算")}</p>
                  </div>
                  <div className="net-now">
                    <div
                      style={{ "--series": "var(--data-1)" } as CSSProperties}
                    >
                      <small>
                        <i></i>
                        {t("入站")}
                      </small>
                      <strong>{rate(host.snapshot.netIn ?? 0)}</strong>
                    </div>
                    <div
                      style={{ "--series": "var(--data-2)" } as CSSProperties}
                    >
                      <small>
                        <i></i>
                        {t("出站")}
                      </small>
                      <strong>{rate(host.snapshot.netOut ?? 0)}</strong>
                    </div>
                  </div>
                </header>
                <LineChart
                  series={[
                    {
                      name: t("入站"),
                      points: series("netIn"),
                      color: "var(--data-1)",
                    },
                    {
                      name: t("出站"),
                      points: series("netOut"),
                      color: "var(--data-2)",
                    },
                  ]}
                  hours={hours(viewState.period)}
                  binary
                  format={rate}
                  tickFormat={rateTick}
                  label={t("网络流量")}
                ></LineChart>
              </section>
              <section className="panel">
                <header className="panel-head ruled">
                  <h2>
                    {t("磁盘")}
                    <span className="count">{host.snapshot.disks.length}</span>
                  </h2>
                  <p>{t("使用率达到 85% 会产生告警")}</p>
                </header>
                {host.snapshot.disks.length ? (
                  <div className="table-wrap">
                    <table className="table">
                      <thead>
                        <tr>
                          <th>{t("挂载点")}</th>
                          <th>{t("文件系统")}</th>
                          <th className="num">{t("已用")}</th>
                          <th className="num">{t("容量")}</th>
                          <th>{t("使用率")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        <>
                          {host.snapshot.disks.map((d) => (
                            <tr key={d.path}>
                              <td className="disk-path">{d.path}</td>
                              <td className="muted">{d.filesystem}</td>
                              <td className="num">{bytes(d.used)}</td>
                              <td className="num">{bytes(d.total)}</td>
                              <td>
                                <div className="disk-usage">
                                  <span
                                    data-tone={
                                      d.percent >= 85 ? "warn" : undefined
                                    }
                                    className="meter"
                                  >
                                    <i
                                      style={
                                        {
                                          width: d.percent + "%",
                                        } as CSSProperties
                                      }
                                    ></i>
                                  </span>
                                  <span>
                                    {number(d.percent)}
                                    {"%"}
                                  </span>
                                </div>
                              </td>
                            </tr>
                          ))}
                        </>
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <p className="empty compact">
                    {t("当前环境没有提供磁盘采样。")}
                  </p>
                )}
              </section>
              <section className="panel">
                <header className="panel-head ruled">
                  <h2>{t("系统信息")}</h2>
                </header>
                <dl className="kv panel-body pad">
                  <>
                    {[
                      [t("主机名称"), host.hostname],
                      [t("内核版本"), host.inventory.kernel || "—"],
                      [t("CPU 架构"), host.inventory.arch],
                      [t("CPU 型号"), host.inventory.cpu || t("未提供")],
                      [t("逻辑核心"), host.inventory.cores],
                      [t("启动时间"), dateTime(host.inventory.bootTime * 1000)],
                      [
                        t("采集来源"),
                        host.source === "agent"
                          ? t("远程 Agent")
                          : t("服务器本机"),
                      ],
                      [
                        t("Agent 版本"),
                        host.source === "agent"
                          ? host.agentVersion || t("等待上报")
                          : t("内置采集"),
                      ],
                      [t("最近采样"), dateTime(host.lastSeen)],
                      [t("全部地址"), host.inventory.ips.join(", ") || "—"],
                    ].map(([key, value]) => (
                      <div key={key}>
                        <dt>{key}</dt>
                        <dd>{value}</dd>
                      </div>
                    ))}
                  </>
                </dl>
              </section>
            </>
          )}
        </div>
      </div>
      {viewState.edit ? (
        <Modal
          title={t("编辑主机备注")}
          onClose={() => {
            if (!store.getState().saving) store.setState({ edit: false });
          }}
          footer={
            <>
              <button
                className="btn"
                onClick={() => {
                  if (!store.getState().saving) store.setState({ edit: false });
                }}
              >
                {t("取消")}
              </button>
              <button
                form="host-form"
                disabled={viewState.saving}
                className="btn primary"
              >
                {viewState.saving ? t("正在保存…") : t("保存")}
              </button>
            </>
          }
        >
          <form
            id="host-form"
            className="form"
            onSubmit={(event) => {
              event.preventDefault();
              save();
            }}
          >
            <label className="field">
              {t("备注名称")}
              <input
                maxLength={60}
                placeholder={t("例如：生产应用服务器")}
                value={viewState.draft}
                onChange={(event) => {
                  store.setState({ draft: event.currentTarget.value });
                }}
              />
              <small>
                {t("备注显示在主机列表和总览中，留空时显示主机名。")}
              </small>
            </label>
          </form>
        </Modal>
      ) : null}
    </>
  );
}
