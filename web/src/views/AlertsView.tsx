import { notice } from "../lib/messages";
import { useEffect, useState } from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import Modal from "../components/Modal";
import NotificationsPanel from "../components/NotificationsPanel";
import StatusBadge from "../components/StatusBadge";
import StatusGlyph from "../components/StatusGlyph";
import { t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import {
  action,
  api,
  appStore,
  can,
  dateTime,
  navigate,
  refreshAll,
  relative,
  useApp,
  type Alert,
} from "../stores/app";
export default function AlertsView() {
  const initialState = {
    view: (can("alert:read") ? "incidents" : "channels") as
      | "incidents"
      | "channels"
      | "deliveries",
    filter: "all",
    query: "",
    silenceOpen: false,
    minutes: 60,
    silenceTarget: undefined as Alert | undefined,
    busy: false,
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const appState = useApp();
  useI18n();
  const getAlerts = () =>
    appStore
      .getState()
      .data.alerts.filter(
        (a) =>
          (store.getState().filter === "all" ||
            a.state === store.getState().filter) &&
          (!store.getState().query ||
            `${a.title} ${a.resource}`.includes(store.getState().query)),
      );
  const getSelected = () =>
    getAlerts().find((a) => a.id === appStore.getState().activeAlert) ||
    getAlerts()[0];
  const getTabs = () => [
    {
      key: "all",
      label: t("全部"),
      count: appStore.getState().data.alerts.length,
    },
    {
      key: "open",
      label: t("待处理"),
      count: appStore.getState().data.alerts.filter((a) => a.state === "open")
        .length,
    },
    {
      key: "acknowledged",
      label: t("处理中"),
      count: appStore
        .getState()
        .data.alerts.filter((a) => a.state === "acknowledged").length,
    },
    {
      key: "resolved",
      label: t("已恢复"),
      count: appStore
        .getState()
        .data.alerts.filter((a) => a.state === "resolved").length,
    },
  ];
  const getIsSilenced = () => {
    void appStore.getState().updatedAt;
    return !!getSelected() && getSelected().silenceUntil > Date.now();
  };
  const getOpenCount = () =>
    appStore.getState().data.alerts.filter((a) => a.state !== "resolved")
      .length;
  const severityOpen = (severity: string) =>
    appStore
      .getState()
      .data.alerts.filter(
        (a) => a.state !== "resolved" && a.severity === severity,
      ).length;
  const getRecoveredToday = () =>
    appStore
      .getState()
      .data.alerts.filter(
        (a) =>
          a.recoveredAt &&
          new Date(a.recoveredAt).toDateString() === new Date().toDateString(),
      ).length;
  function duration(a: Alert) {
    const m = Math.max(
      1,
      Math.floor(((a.recoveredAt || Date.now()) - a.openedAt) / 60000),
    );
    return m < 60
      ? t("{p0} 分钟", { p0: m })
      : m < 1440
        ? t("{p0} 小时 {p1} 分钟", { p0: Math.floor(m / 60), p1: m % 60 })
        : t("{p0} 天 {p1} 小时", {
            p0: Math.floor(m / 1440),
            p1: Math.floor((m % 1440) / 60),
          });
  }
  const silenced = (a: Alert) => {
    void appStore.getState().updatedAt;
    return a.silenceUntil > Date.now();
  };
  function stateLabel(s: string) {
    return (
      { open: t("待处理"), acknowledged: t("处理中"), resolved: t("已恢复") }[
        s
      ] || s
    );
  }
  function resource() {
    if (!getSelected()) return;
    if (getSelected().kind === "host") {
      appStore.setState({ activeHost: getSelected().resourceId });
      navigate("host");
    } else {
      appStore.setState({ activeMonitor: getSelected().resourceId });
      navigate("monitors");
    }
  }
  async function ack() {
    if (!getSelected()) return;
    store.setState({ busy: true });
    await action(
      () =>
        api(`/alerts/${getSelected()!.id}/acknowledgements`, "POST", {
          version: getSelected()!.version,
        }),
      notice("已确认并接手告警"),
    );
    store.setState({ busy: false });
  }
  function startSilence() {
    store.setState({ silenceTarget: getSelected() });
    store.setState({ silenceOpen: true });
  }
  async function silence() {
    const target = store.getState().silenceTarget;
    if (!target || store.getState().busy) return;
    store.setState({ busy: true });
    if (
      await action(
        () =>
          api(`/alerts/${target.id}/silences`, "POST", {
            version: target.version,
            minutes: store.getState().minutes,
          }),
        notice("事件静默状态已保存"),
      )
    )
      store.setState({ silenceOpen: false });
    store.setState({ busy: false });
  }
  useEffect(() => {
    const id = getSelected()?.id;
    if (id) appStore.setState({ activeAlert: id });
  }, [getSelected()?.id]);

  const alerts = getAlerts();
  const selected = getSelected();
  const tabs = getTabs();
  const isSilenced = getIsSilenced();
  const openCount = getOpenCount();
  const recoveredToday = getRecoveredToday();
  return (
    <>
      <header className="page-head">
        <div className="page-title">
          <h1>{appState.pageTitle}</h1>
          <p>
            {can("alert:read")
              ? t("{p0} 条事件未恢复，列表保留最近 200 条。", { p0: openCount })
              : t("配置通知渠道并查看投递结果。")}
          </p>
        </div>
        <div className="page-actions">
          <button
            className="btn"
            onClick={() => {
              refreshAll();
            }}
          >
            <Icon name="RefreshCw"></Icon>
            {t("刷新")}
          </button>
        </div>
      </header>
      <nav aria-label={t("告警中心")} className="tabs">
        {can("alert:read") ? (
          <button
            aria-current={viewState.view === "incidents" ? "page" : undefined}
            className={classNames("tab", {
              active: viewState.view === "incidents",
            })}
            onClick={() => {
              store.setState({ view: "incidents" });
            }}
          >
            {t("事件")}
            <span className="count">{openCount}</span>
          </button>
        ) : null}
        {can("notification:manage") ? (
          <button
            aria-current={viewState.view === "channels" ? "page" : undefined}
            className={classNames("tab", {
              active: viewState.view === "channels",
            })}
            onClick={() => {
              store.setState({ view: "channels" });
            }}
          >
            {t("通知渠道")}
          </button>
        ) : null}
        <button
          aria-current={viewState.view === "deliveries" ? "page" : undefined}
          className={classNames("tab", {
            active: viewState.view === "deliveries",
          })}
          onClick={() => {
            store.setState({ view: "deliveries" });
          }}
        >
          {t("投递记录")}
        </button>
      </nav>
      {viewState.view !== "incidents" ? (
        <NotificationsPanel mode={viewState.view}></NotificationsPanel>
      ) : (
        <>
          <section aria-label={t("事件统计")} className="readout">
            <div className="readout-item">
              <span className="readout-label">
                <StatusGlyph tone="crit" size={9}></StatusGlyph>
                {t("严重")}
              </span>
              <span className="readout-value">{severityOpen("critical")}</span>
              <span className="readout-note">{t("未恢复，优先处理")}</span>
            </div>
            <div className="readout-item">
              <span className="readout-label">
                <StatusGlyph tone="warn" size={9}></StatusGlyph>
                {t("警告")}
              </span>
              <span className="readout-value">{severityOpen("warning")}</span>
              <span className="readout-note">{t("未恢复，持续关注")}</span>
            </div>
            <div className="readout-item">
              <span className="readout-label">
                <StatusGlyph tone="info" size={9}></StatusGlyph>
                {t("处理中")}
              </span>
              <span className="readout-value">{tabs[2].count}</span>
              <span className="readout-note">{t("已有成员认领")}</span>
            </div>
            <div className="readout-item">
              <span className="readout-label">
                <StatusGlyph tone="ok" size={9}></StatusGlyph>
                {t("今日已恢复")}
              </span>
              <span className="readout-value">{recoveredToday}</span>
              <span className="readout-note">{t("由后续检测自动判断")}</span>
            </div>
          </section>
          <div className="split">
            <section aria-label={t("事件列表")} className="panel incident-list">
              <div
                role="group"
                aria-label={t("按处理状态筛选")}
                className="seg"
              >
                <>
                  {tabs.map((t) => (
                    <button
                      key={t.key}
                      aria-pressed={viewState.filter === t.key}
                      className={classNames({
                        active: viewState.filter === t.key,
                      })}
                      onClick={() => {
                        store.setState({ filter: t.key });
                      }}
                    >
                      {t.label}
                      <small>{t.count}</small>
                    </button>
                  ))}
                </>
              </div>
              <label className="search">
                <Icon name="Search" size={15}></Icon>
                <input
                  placeholder={t("事件或资源名称")}
                  aria-label={t("搜索告警")}
                  value={viewState.query}
                  onChange={(event) => {
                    store.setState({ query: event.currentTarget.value });
                  }}
                />
              </label>
              <>
                {alerts.map((a) => (
                  <button
                    key={a.id}
                    aria-current={selected?.id === a.id ? "true" : undefined}
                    className={classNames("incident", {
                      active: selected?.id === a.id,
                    })}
                    onClick={() => {
                      appStore.setState({ activeAlert: a.id });
                    }}
                  >
                    <span className="incident-top">
                      <StatusBadge
                        tone={
                          a.state === "resolved"
                            ? "ok"
                            : a.severity === "critical"
                              ? "crit"
                              : "warn"
                        }
                        label={
                          a.state === "resolved"
                            ? t("已恢复")
                            : a.severity === "critical"
                              ? t("严重")
                              : t("警告")
                        }
                      ></StatusBadge>
                      <span className="muted small">
                        {relative(a.openedAt)}
                      </span>
                    </span>
                    <h3>{a.title}</h3>
                    <span className="incident-meta">
                      <span>
                        <Icon
                          name={a.kind === "host" ? "Server" : "Radar"}
                          size={12}
                        ></Icon>
                        {a.resource}
                      </span>
                      <span>
                        {a.state === "acknowledged"
                          ? t("{p0}处理中", { p0: a.owner || t("成员") })
                          : stateLabel(a.state)}
                      </span>
                      {a.state !== "resolved" && silenced(a) ? (
                        <span>
                          <Icon name="BellOff" size={12}></Icon>
                          {t("已静默")}
                        </span>
                      ) : null}
                    </span>
                  </button>
                ))}
              </>
              {!alerts.length ? (
                <div className="empty compact">
                  <h3>
                    {appState.data.alerts.length
                      ? t("没有匹配的事件")
                      : t("还没有告警事件")}
                  </h3>
                  <p>{t("检测连续异常达到阈值或主机离线时会自动创建事件。")}</p>
                </div>
              ) : null}
            </section>
            {selected ? (
              <section
                aria-label={t("事件详情")}
                className="panel incident-detail"
              >
                <div className="incident-detail-head">
                  <div className="eyeline">
                    <span>{t("事件 #{p0}", { p0: selected.id })}</span>
                    <StatusBadge
                      tone={selected.severity === "critical" ? "crit" : "warn"}
                      label={
                        selected.severity === "critical" ? t("严重") : t("警告")
                      }
                    ></StatusBadge>
                    <span className="state-text">
                      <StatusGlyph
                        tone={
                          selected.state === "resolved"
                            ? "ok"
                            : selected.state === "acknowledged"
                              ? "info"
                              : "idle"
                        }
                        size={9}
                      ></StatusGlyph>
                      {stateLabel(selected.state)}
                    </span>
                  </div>
                  <h2>{selected.title}</h2>
                  <p>{selected.description}</p>
                  <div className="incident-actions">
                    {can("alert:handle") && selected.state === "open" ? (
                      <button
                        disabled={viewState.busy}
                        className="btn primary"
                        onClick={() => {
                          ack();
                        }}
                      >
                        <Icon name="Check"></Icon>
                        {t("确认并接手")}
                      </button>
                    ) : null}
                    {can("alert:handle") && selected.state !== "resolved" ? (
                      <button
                        className="btn"
                        onClick={() => {
                          startSilence();
                        }}
                      >
                        <Icon name="BellOff"></Icon>
                        {isSilenced ? t("调整静默") : t("静默通知")}
                      </button>
                    ) : null}
                    {can(
                      selected.kind === "host" ? "host:read" : "monitor:read",
                    ) ? (
                      <button
                        className="btn"
                        onClick={() => {
                          resource();
                        }}
                      >
                        {t("查看{p0}", {
                          p0: selected.kind === "host" ? t("主机") : t("检测"),
                        })}
                      </button>
                    ) : null}
                  </div>
                  {isSilenced ? (
                    <p className="silence-line">
                      <Icon name="BellOff" size={14}></Icon>
                      {t("通知已静默至 {p0}，检测仍在继续运行。", {
                        p0: dateTime(selected.silenceUntil),
                      })}
                    </p>
                  ) : null}
                </div>
                <dl className="kv">
                  <div>
                    <dt>{t("关联资源")}</dt>
                    <dd>{selected.resource}</dd>
                  </div>
                  <div>
                    <dt>{t("处理人")}</dt>
                    <dd>{selected.owner || t("尚未认领")}</dd>
                  </div>
                  <div>
                    <dt>{t("发现时间")}</dt>
                    <dd className="num">{dateTime(selected.openedAt)}</dd>
                  </div>
                  <div>
                    <dt>
                      {selected.recoveredAt ? t("恢复时间") : t("已持续")}
                    </dt>
                    <dd className="num">
                      {selected.recoveredAt
                        ? dateTime(selected.recoveredAt)
                        : duration(selected)}
                    </dd>
                  </div>
                </dl>
                <div className="incident-section">
                  <h3 className="section-title">{t("时间线")}</h3>
                  <ol className="timeline">
                    <>
                      {selected.activity.map((a, i) => (
                        <li key={a.at + "-" + i}>
                          <strong>{a.message}</strong>
                          <small>
                            {dateTime(a.at)} {a.actor}
                          </small>
                        </li>
                      ))}
                    </>
                  </ol>
                </div>
                <footer className="panel-foot">
                  <span className="note">
                    <Icon name="Info" size={14}></Icon>
                    {t(
                      "确认和静默不会改变故障状态，是否恢复由后续检测自动判断。",
                    )}
                  </span>
                </footer>
              </section>
            ) : (
              <section className="panel empty">
                <StatusGlyph tone="ok" size={14}></StatusGlyph>
                <h3>{t("当前没有需要处理的事件")}</h3>
                <p>{t("事件会记录异常、认领、静默和自动恢复的完整过程。")}</p>
              </section>
            )}
          </div>
        </>
      )}
      {viewState.silenceOpen && viewState.silenceTarget ? (
        <Modal
          title={t("静默通知")}
          subtitle={t(
            "静默期内跳过尚未发送的故障与恢复通知，到期后也不补发。检测会继续运行，已发出的消息无法撤回。",
          )}
          onClose={() => {
            if (!store.getState().busy) store.setState({ silenceOpen: false });
          }}
          footer={
            <>
              <button
                className="btn"
                onClick={() => {
                  if (!store.getState().busy)
                    store.setState({ silenceOpen: false });
                }}
              >
                {t("取消")}
              </button>
              <button
                disabled={viewState.busy}
                className="btn primary"
                onClick={() => {
                  silence();
                }}
              >
                {viewState.minutes ? t("开始静默") : t("取消静默")}
              </button>
            </>
          }
        >
          <fieldset className="field">
            <legend className="field-label">{t("静默时长")}</legend>
            <div className="choice-group">
              <>
                {[
                  { n: 30, label: t("30 分钟") },
                  { n: 60, label: t("1 小时") },
                  { n: 240, label: t("4 小时") },
                  { n: 1440, label: t("24 小时") },
                  { n: 0, label: t("取消静默") },
                ].map((v) => (
                  <label key={v.n} className="choice">
                    <input
                      type="radio"
                      value={v.n}
                      checked={viewState.minutes === v.n}
                      onChange={() => {
                        store.setState({ minutes: v.n });
                      }}
                    />
                    {v.label}
                  </label>
                ))}
              </>
            </div>
          </fieldset>
        </Modal>
      ) : null}
    </>
  );
}
