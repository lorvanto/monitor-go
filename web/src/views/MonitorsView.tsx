import { useEffect, useState } from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import Modal from "../components/Modal";
import SignalTrace from "../components/SignalTrace";
import StatusBadge from "../components/StatusBadge";
import { i18nStore, t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import {
  action,
  api,
  appStore,
  can,
  dateTime,
  latencyParts,
  notify,
  number,
  relative,
  runCheck,
  statusLabel,
  useApp,
  type Monitor,
} from "../stores/app";
export default function MonitorsView() {
  const initialState = {
    query: "",
    type: "",
    group: "",
    tab: "all",
    selected: [] as string[],
    creating: appStore.getState().createMonitorRequested,
    detailID: appStore.getState().activeMonitor,
    busy: false,
    error: "",
    form: {
      name: "",
      target: "",
      type: "HTTP",
      group: t("默认分组"),
      interval: 60,
      timeout: 10,
      expected: 200,
      threshold: 3,
      sslDays: 14,
      username: "",
      password: "",
      tls: false,
    },
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const appState = useApp();
  useI18n();
  useEffect(() => {
    appStore.setState({ createMonitorRequested: false, activeMonitor: "" });
  }, [store]);
  const getDetail = () =>
    appStore
      .getState()
      .data.monitors.find((m) => m.id === store.getState().detailID);
  const getGroups = () => [
    ...new Set(appStore.getState().data.monitors.map((m) => m.group)),
  ];
  const getTabs = () => [
    {
      key: "all",
      label: t("全部"),
      count: appStore.getState().data.monitors.length,
    },
    {
      key: "healthy",
      label: t("正常"),
      count: appStore
        .getState()
        .data.monitors.filter((m) => m.status === "healthy").length,
    },
    {
      key: "attention",
      label: t("需关注"),
      count: appStore
        .getState()
        .data.monitors.filter((m) =>
          ["failed", "warning", "unknown"].includes(m.status),
        ).length,
    },
    {
      key: "paused",
      label: t("已暂停"),
      count: appStore
        .getState()
        .data.monitors.filter((m) => m.status === "paused").length,
    },
  ];
  const getFiltered = () =>
    appStore
      .getState()
      .data.monitors.filter(
        (m) =>
          (!store.getState().query ||
            `${m.name} ${m.target}`
              .toLowerCase()
              .includes(store.getState().query.toLowerCase())) &&
          (!store.getState().type || m.type === store.getState().type) &&
          (!store.getState().group || m.group === store.getState().group) &&
          (store.getState().tab === "all" ||
            (store.getState().tab === "attention"
              ? ["failed", "warning", "unknown"].includes(m.status)
              : m.status === store.getState().tab)),
      );
  const getAllSelected = () =>
    getFiltered().length > 0 &&
    getFiltered().every((m) => store.getState().selected.includes(m.id));
  function selectAll(e: React.ChangeEvent<HTMLInputElement>) {
    store.setState({
      selected: (e.target as HTMLInputElement).checked
        ? getFiltered().map((m) => m.id)
        : [],
    });
  }
  async function toggle(m: Monitor) {
    await action(
      () =>
        api(`/monitors/${m.id}`, "PATCH", {
          version: m.version,
          enabled: !m.enabled,
        }),
      m.enabled ? t("检测已暂停") : t("已启用，等待真实检测"),
    );
  }
  async function create() {
    store.setState({ error: "" });
    store.setState({ busy: true });
    try {
      await api("/monitors", "POST", store.getState().form);
      store.setState({ creating: false });
      store.setState({ query: "" });
      store.setState({ tab: "all" });
      store.setState({ type: "" });
      store.setState({ group: "" });
      store.setState((current) => ({ form: { ...current.form, name: "" } }));
      store.setState((current) => ({ form: { ...current.form, target: "" } }));
      store.setState((current) => ({
        form: { ...current.form, password: "" },
      }));
      await action(async () => {}, t("检测目标已保存，调度器将自动执行"));
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  function exportCSV() {
    const rows = [
      [t("名称"), t("类型"), t("目标"), t("状态"), t("分组"), t("最近检测")],
      ...getFiltered().map((m) => [
        m.name,
        m.type,
        m.target,
        statusLabel(m.status),
        m.group,
        dateTime(m.checkedAt),
      ]),
    ];
    const csv =
      "﻿" +
      rows
        .map((row) =>
          row
            .map(
              (value) =>
                '"' +
                (/^[=+@\-\t\r]/.test(value) ? "'" + value : value).replaceAll(
                  '"',
                  '""',
                ) +
                '"',
            )
            .join(","),
        )
        .join("\r\n");
    const url = URL.createObjectURL(
      new Blob([csv], { type: "text/csv;charset=utf-8" }),
    );
    const a = document.createElement("a");
    a.href = url;
    a.download = t("监控检测.csv");
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    notify(t("已导出 {p0} 项检测", { p0: getFiltered().length }));
  }
  function resetFilter() {
    store.setState({ query: "" });
    store.setState({ type: "" });
    store.setState({ group: "" });
    store.setState({ tab: "all" });
  }
  const typeIcon: Record<string, string> = {
    HTTP: "Globe",
    TCP: "Network",
    SSL: "LockKeyhole",
    MySQL: "HardDrive",
    Redis: "Box",
  };
  const timeOnly = (at: number) =>
    at
      ? new Date(at).toLocaleString(i18nStore.getState().locale, {
          month: "2-digit",
          day: "2-digit",
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
          hour12: false,
        })
      : "—";
  function runSelected() {
    runCheck(
      store
        .getState()
        .selected.filter((id) =>
          appStore
            .getState()
            .data.monitors.some((m) => m.id === id && m.enabled),
        ),
    );
  }
  useEffect(() => {
    const ids = appStore.getState().data.monitors.map((m) => m.id);
    store.setState({
      selected: store.getState().selected.filter((id) => ids.includes(id)),
    });
  }, [appState.data.monitors]);

  const detail = getDetail();
  const groups = getGroups();
  const tabs = getTabs();
  const filtered = getFiltered();
  const allSelected = getAllSelected();
  return (
    <>
      <header className="page-head">
        <div className="page-title">
          <h1>{appState.pageTitle}</h1>
          <p>
            {t("{p0} 个检测目标，由当前服务器按设定间隔执行。", {
              p0: appState.data.monitors.length,
            })}
          </p>
        </div>
        <div className="page-actions">
          <button
            disabled={!filtered.length}
            className="btn"
            onClick={() => {
              exportCSV();
            }}
          >
            <Icon name="Download"></Icon>
            {t("导出 CSV")}
          </button>
          {can("monitor:update") ? (
            <button
              className="btn primary"
              onClick={() => {
                store.setState({ creating: true, error: "" });
              }}
            >
              <Icon name="Plus"></Icon>
              {t("新增检测")}
            </button>
          ) : null}
        </div>
      </header>
      <section className="panel">
        <div className="toolbar">
          <div role="group" aria-label={t("按状态筛选")} className="seg">
            <>
              {tabs.map((t) => (
                <button
                  key={t.key}
                  aria-pressed={viewState.tab === t.key}
                  className={classNames({ active: viewState.tab === t.key })}
                  onClick={() => {
                    store.setState({ tab: t.key });
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
              placeholder={t("名称或目标地址")}
              aria-label={t("搜索检测目标")}
              value={viewState.query}
              onChange={(event) => {
                store.setState({ query: event.currentTarget.value });
              }}
            />
            {viewState.query ? (
              <button
                aria-label={t("清空搜索")}
                className="icon-btn sm"
                onClick={() => {
                  store.setState({ query: "" });
                }}
              >
                <Icon name="X" size={13}></Icon>
              </button>
            ) : null}
          </label>
          <select
            aria-label={t("检测类型")}
            value={viewState.type}
            onChange={(event) => {
              store.setState({ type: event.currentTarget.value });
            }}
          >
            <option value="">{t("全部类型")}</option>
            <>
              {["HTTP", "TCP", "SSL", "MySQL", "Redis"].map((t) => (
                <option key={t}>{t}</option>
              ))}
            </>
          </select>
          <select
            aria-label={t("分组")}
            value={viewState.group}
            onChange={(event) => {
              store.setState({ group: event.currentTarget.value });
            }}
          >
            <option value="">{t("全部分组")}</option>
            <>
              {groups.map((g) => (
                <option key={g}>{g}</option>
              ))}
            </>
          </select>
          <span className="spacer"></span>
          {can("monitor:run") ? (
            <button
              disabled={!filtered.some((m) => m.enabled)}
              className="btn"
              onClick={() => {
                runCheck(filtered.filter((m) => m.enabled).map((m) => m.id));
              }}
            >
              <Icon name="RefreshCw"></Icon>
              {t("检测筛选结果")}
            </button>
          ) : null}
        </div>
        {viewState.selected.length ? (
          <div className="selection-bar">
            <strong>
              {t("已选择 {p0} 项", { p0: viewState.selected.length })}
            </strong>
            {can("monitor:run") ? (
              <button
                className="link"
                onClick={() => {
                  runSelected();
                }}
              >
                {t("立即检测")}
              </button>
            ) : null}
            <button
              className="link"
              onClick={() => {
                store.setState({ selected: [] });
              }}
            >
              {t("取消选择")}
            </button>
          </div>
        ) : null}
        {filtered.length ? (
          <div className="table-wrap">
            <table className="table monitor-table">
              <thead>
                <tr>
                  <th className="check-cell">
                    <input
                      type="checkbox"
                      checked={allSelected}
                      aria-label={t("选择当前列表全部检测")}
                      onChange={(event) => {
                        selectAll(event);
                      }}
                    />
                  </th>
                  <th>{t("检测目标")}</th>
                  <th>{t("状态")}</th>
                  <th className="num">{t("响应")}</th>
                  <th>{t("最近 32 次结果")}</th>
                  <th className="num">{t("24 小时可用率")}</th>
                  <th>{t("最近检测")}</th>
                  <th className="actions">
                    <span className="sr-only">{t("操作")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                <>
                  {filtered.map((m) => (
                    <tr
                      key={m.id}
                      className={classNames({
                        selected: viewState.selected.includes(m.id),
                      })}
                    >
                      <td className="check-cell c-check">
                        <input
                          type="checkbox"
                          value={m.id}
                          aria-label={t("选择{p0}", { p0: m.name })}
                          checked={viewState.selected.includes(m.id)}
                          onChange={(event) => {
                            store.setState({
                              selected: event.currentTarget.checked
                                ? [...viewState.selected, m.id]
                                : viewState.selected.filter(
                                    (selectedValue) => selectedValue !== m.id,
                                  ),
                            });
                          }}
                        />
                      </td>
                      <td className="c-target">
                        <button
                          className="target"
                          onClick={() => {
                            store.setState({ detailID: m.id });
                          }}
                        >
                          <span className="tag">{m.type}</span>
                          <span className="target-text">
                            <strong>{m.name}</strong>
                            <small title={m.target}>{m.target}</small>
                          </span>
                        </button>
                      </td>
                      <td className="c-status">
                        <StatusBadge
                          status={m.status}
                          label={statusLabel(m.status)}
                        ></StatusBadge>
                      </td>
                      <td className="num c-latency">
                        {latencyParts(m.latency)[0]}
                        <small className="unit">
                          {latencyParts(m.latency)[1]}
                        </small>
                      </td>
                      <td className="c-trace">
                        <SignalTrace history={m.history}></SignalTrace>
                      </td>
                      <td className="num c-uptime">
                        {m.uptime == null ? "—" : number(m.uptime, 2) + "%"}
                      </td>
                      <td className="nowrap muted c-last">
                        {relative(m.checkedAt)}
                      </td>
                      <td className="actions c-actions">
                        <div className="row-actions tight">
                          {can("monitor:run") ? (
                            <button
                              disabled={
                                !m.enabled || appState.checking.includes(m.id)
                              }
                              aria-label={t("立即检测{p0}", { p0: m.name })}
                              title={t("立即检测")}
                              className="icon-btn"
                              onClick={() => {
                                runCheck([m.id]);
                              }}
                            >
                              <Icon
                                name="RefreshCw"
                                size={15}
                                className={classNames({
                                  spin: appState.checking.includes(m.id),
                                })}
                              ></Icon>
                            </button>
                          ) : null}
                          {can("monitor:update") ? (
                            <button
                              aria-label={`${m.enabled ? t("暂停") : t("启用")}${m.name}`}
                              title={m.enabled ? t("暂停检测") : t("启用检测")}
                              className="icon-btn"
                              onClick={() => {
                                toggle(m);
                              }}
                            >
                              <Icon
                                name={m.enabled ? "Pause" : "Play"}
                                size={15}
                              ></Icon>
                            </button>
                          ) : null}
                          {!can("monitor:run") && !can("monitor:update") ? (
                            <span className="muted">{t("只读")}</span>
                          ) : null}
                        </div>
                      </td>
                    </tr>
                  ))}
                </>
              </tbody>
            </table>
          </div>
        ) : (
          <div className="empty">
            <Icon name="Radar" size={28}></Icon>
            <h3>
              {appState.data.monitors.length
                ? t("没有匹配的检测目标")
                : t("添加第一个检测目标")}
            </h3>
            <p>
              {appState.data.monitors.length
                ? t("换个关键词，或清除筛选条件。")
                : t("填写网站或服务地址，当前服务器会按设定间隔检测。")}
            </p>
            {appState.data.monitors.length ? (
              <button
                className="btn"
                onClick={() => {
                  resetFilter();
                }}
              >
                {t("清除筛选")}
              </button>
            ) : can("monitor:update") ? (
              <button
                className="btn primary"
                onClick={() => {
                  store.setState({ creating: true, error: "" });
                }}
              >
                <Icon name="Plus"></Icon>
                {t("新增检测")}
              </button>
            ) : null}
          </div>
        )}
        {filtered.length ? (
          <footer className="panel-foot">
            <span>
              {t("显示 {p0} / {p1} 项", {
                p0: filtered.length,
                p1: appState.data.monitors.length,
              })}
            </span>
            <span>{t("信号条从左到右由旧到新，异常越严重柱越高")}</span>
          </footer>
        ) : null}
      </section>
      <p className="note page-note">
        <Icon name="Info" size={14}></Icon>
        {t(
          "可用率按近 24 小时的成功次数计算，尚无检测时显示“—”。暂停检测不会把未恢复的告警标记为正常。",
        )}
      </p>
      {viewState.creating ? (
        <Modal
          title={t("新增检测")}
          subtitle={t("检测由当前服务器执行，请确认服务器能访问目标地址。")}
          wide
          onClose={() => {
            !viewState.busy && store.setState({ creating: false });
          }}
          footer={
            <>
              <button
                disabled={viewState.busy}
                className="btn"
                onClick={() => {
                  store.setState({ creating: false });
                }}
              >
                {t("取消")}
              </button>
              <button
                form="monitor-form"
                type="submit"
                disabled={viewState.busy}
                className="btn primary"
              >
                {viewState.busy ? t("正在保存…") : t("创建检测")}
              </button>
            </>
          }
        >
          <form
            id="monitor-form"
            className="form"
            onSubmit={(event) => {
              event.preventDefault();
              create();
            }}
          >
            <fieldset className="field">
              <legend className="field-label">{t("检测类型")}</legend>
              <div className="choice-group">
                <>
                  {["HTTP", "TCP", "SSL", "MySQL", "Redis"].map((t) => (
                    <label key={t} className="choice">
                      <input
                        type="radio"
                        value={t}
                        checked={viewState.form.type === t}
                        onChange={() => {
                          store.setState((current) => ({
                            form: { ...current.form, type: t },
                          }));
                        }}
                      />
                      <Icon name={typeIcon[t]} size={15}></Icon>
                      {t}
                    </label>
                  ))}
                </>
              </div>
            </fieldset>
            <div className="form-grid">
              <label className="field">
                {t("名称")}
                <input
                  required
                  maxLength={50}
                  placeholder={t("例如：订单服务")}
                  value={viewState.form.name}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        name: event.currentTarget.value,
                      },
                    }));
                  }}
                />
              </label>
              <label className="field">
                {t("分组")}
                <input
                  maxLength={30}
                  value={viewState.form.group}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        group: event.currentTarget.value,
                      },
                    }));
                  }}
                />
              </label>
            </div>
            <label className="field">
              {t("目标地址")}
              <input
                required
                placeholder={
                  viewState.form.type === "HTTP"
                    ? "https://api.example.com/health"
                    : "example.com:443"
                }
                className="mono"
                value={viewState.form.target}
                onChange={(event) => {
                  store.setState((current) => ({
                    form: {
                      ...current.form,
                      target: event.currentTarget.value,
                    },
                  }));
                }}
              />
              <small>
                {viewState.form.type === "HTTP"
                  ? t("支持 HTTP 与 HTTPS，HTTPS 会校验证书。")
                  : t("填写域名或 IP 加端口，IPv6 地址写成 [地址]:端口。")}
              </small>
            </label>
            <div className="form-grid three">
              <label className="field">
                {t("检测间隔（秒）")}
                <input
                  type="number"
                  min="15"
                  max="86400"
                  required
                  value={viewState.form.interval}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        interval: Number(event.currentTarget.value),
                      },
                    }));
                  }}
                />
              </label>
              <label className="field">
                {t("超时（秒）")}
                <input
                  type="number"
                  min="1"
                  max="30"
                  required
                  value={viewState.form.timeout}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        timeout: Number(event.currentTarget.value),
                      },
                    }));
                  }}
                />
              </label>
              <label className="field">
                {t("连续异常几次告警")}
                <input
                  type="number"
                  min="1"
                  max="10"
                  required
                  value={viewState.form.threshold}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        threshold: Number(event.currentTarget.value),
                      },
                    }));
                  }}
                />
              </label>
            </div>
            {viewState.form.type === "HTTP" ? (
              <label className="field">
                {t("预期状态码")}
                <input
                  type="number"
                  min="100"
                  max="599"
                  required
                  value={viewState.form.expected}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        expected: Number(event.currentTarget.value),
                      },
                    }));
                  }}
                />
              </label>
            ) : null}
            {viewState.form.type === "SSL" ? (
              <label className="field">
                {t("证书到期前几天提醒")}
                <input
                  type="number"
                  min="1"
                  max="90"
                  required
                  value={viewState.form.sslDays}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        sslDays: Number(event.currentTarget.value),
                      },
                    }));
                  }}
                />
              </label>
            ) : null}
            {viewState.form.type === "MySQL" ||
            viewState.form.type === "Redis" ? (
              <>
                <div className="form-grid">
                  <label className="field">
                    {viewState.form.type === "MySQL"
                      ? t("数据库用户名")
                      : t("用户名（可选）")}
                    <input
                      required={viewState.form.type === "MySQL"}
                      autoComplete="off"
                      value={viewState.form.username}
                      onChange={(event) => {
                        store.setState((current) => ({
                          form: {
                            ...current.form,
                            username: event.currentTarget.value,
                          },
                        }));
                      }}
                    />
                  </label>
                  <label className="field">
                    {t("密码")}
                    <input
                      type="password"
                      autoComplete="new-password"
                      value={viewState.form.password}
                      onChange={(event) => {
                        store.setState((current) => ({
                          form: {
                            ...current.form,
                            password: event.currentTarget.value,
                          },
                        }));
                      }}
                    />
                    <small>{t("加密保存，接口不会返回明文。")}</small>
                  </label>
                </div>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={viewState.form.tls}
                    onChange={(event) => {
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          tls: event.currentTarget.checked,
                        },
                      }));
                    }}
                  />
                  {t("使用 TLS 连接并校验证书")}
                </label>
              </>
            ) : null}
            <p className="note">
              <Icon name="Info" size={14}></Icon>
              {t(
                "连续异常达到设定次数后创建告警，之后一次检测成功即自动恢复。",
              )}
            </p>
            {viewState.error ? (
              <p role="alert" className="form-error">
                {viewState.error}
              </p>
            ) : null}
          </form>
        </Modal>
      ) : null}
      {detail ? (
        <Modal
          drawer
          title={detail.name}
          subtitle={t("{p0} 检测，{p1}", { p0: detail.type, p1: detail.group })}
          onClose={() => {
            store.setState({ detailID: "" });
          }}
          footer={
            <>
              {can("monitor:update") ? (
                <button
                  className="btn"
                  onClick={() => {
                    toggle(detail);
                  }}
                >
                  <Icon name={detail.enabled ? "Pause" : "Play"}></Icon>
                  {detail.enabled ? t("暂停检测") : t("启用检测")}
                </button>
              ) : null}
              <span className="grow"></span>
              <button
                className="btn"
                onClick={() => {
                  store.setState({ detailID: "" });
                }}
              >
                {t("关闭")}
              </button>
              {can("monitor:run") ? (
                <button
                  disabled={
                    !detail.enabled || appState.checking.includes(detail.id)
                  }
                  className="btn primary"
                  onClick={() => {
                    runCheck([detail.id]);
                  }}
                >
                  <Icon
                    name="RefreshCw"
                    className={classNames({
                      spin: appState.checking.includes(detail.id),
                    })}
                  ></Icon>
                  {t("立即检测")}
                </button>
              ) : null}
            </>
          }
        >
          <div className="drawer-status">
            <StatusBadge
              status={detail.status}
              label={statusLabel(detail.status)}
            ></StatusBadge>
            <span className="muted">
              {detail.checkedAt
                ? t("最近检测 {p0}", { p0: dateTime(detail.checkedAt) })
                : t("等待首次检测")}
            </span>
          </div>
          <div>
            <h3 className="section-title">{t("最近 32 次结果")}</h3>
            <SignalTrace history={detail.history}></SignalTrace>
          </div>
          <dl className="kv rows">
            <div>
              <dt>{t("目标地址")}</dt>
              <dd className="mono">{detail.target}</dd>
            </div>
            <div>
              <dt>{t("最近诊断")}</dt>
              <dd>{detail.detail || t("等待首次检测")}</dd>
            </div>
            <div>
              <dt>{t("响应时间")}</dt>
              <dd className="num">{latencyParts(detail.latency).join(" ")}</dd>
            </div>
            <div>
              <dt>{t("24 小时可用率")}</dt>
              <dd className="num">
                {detail.uptime == null ? "—" : number(detail.uptime, 2) + "%"}
              </dd>
            </div>
            <div>
              <dt>{t("检测间隔")}</dt>
              <dd>
                {t("每 {p0} 秒，超时 {p1} 秒", {
                  p0: detail.interval,
                  p1: detail.timeout,
                })}
              </dd>
            </div>
            <div>
              <dt>{t("告警条件")}</dt>
              <dd>{t("连续 {p0} 次异常", { p0: detail.threshold })}</dd>
            </div>
            {detail.type === "HTTP" ? (
              <div>
                <dt>{t("预期状态码")}</dt>
                <dd>{detail.expected}</dd>
              </div>
            ) : null}
            {detail.type === "SSL" ? (
              <div>
                <dt>{t("到期提醒")}</dt>
                <dd>{t("提前 {p0} 天", { p0: detail.sslDays })}</dd>
              </div>
            ) : null}
            <div>
              <dt>{t("下次检测")}</dt>
              <dd>
                {detail.enabled ? dateTime(detail.nextCheck) : t("已暂停")}
              </dd>
            </div>
            <div>
              <dt>{t("执行位置")}</dt>
              <dd>{t("当前服务器")}</dd>
            </div>
          </dl>
          <div>
            <h3 className="section-title">{t("检测记录")}</h3>
            {detail.history.length ? (
              <ol className="history-list">
                <>
                  {detail.history.slice(0, 12).map((h) => (
                    <li key={h.at}>
                      <time>{timeOnly(h.at)}</time>
                      <span className="state-text">
                        <StatusBadge
                          status={h.state}
                          label={statusLabel(h.state)}
                        ></StatusBadge>
                        <span title={h.detail} className="detail">
                          {h.detail}
                        </span>
                      </span>
                      <span className="num">
                        {latencyParts(h.latency).join(" ")}
                      </span>
                    </li>
                  ))}
                </>
              </ol>
            ) : (
              <p className="muted">{t("还没有检测记录。")}</p>
            )}
          </div>
        </Modal>
      ) : null}
    </>
  );
}
