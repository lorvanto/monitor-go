import { useEffect, useRef, useState } from "react";
import { createStore, useStore } from "zustand";
import { t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import type { Tone } from "../lib/status";
import { api, can, dateTime, notify, useApp } from "../stores/app";
import Icon from "./Icon";
import Modal from "./Modal";
import StatusBadge from "./StatusBadge";
import StatusGlyph from "./StatusGlyph";
interface Config {
  url: string;
  host: string;
  port: number;
  security: string;
  username: string;
  secret: string;
  from: string;
  to: string[];
}
interface Channel {
  id: string;
  name: string;
  kind: string;
  enabled: boolean;
  version: number;
  config: Config;
  hasSecret: boolean;
}
interface Delivery {
  id: string;
  channel: string;
  channelId: string;
  alertId?: string;
  event: string;
  state: string;
  attempts: number;
  nextAttempt: number;
  createdAt: number;
  finishedAt: number | null;
  detail: string;
}
export default function NotificationsPanel(inputProps: {
  mode: "channels" | "deliveries";
}) {
  const props = { ...{}, ...inputProps };
  const blank = (): Channel => ({
    id: "",
    name: "",
    kind: "webhook",
    enabled: true,
    version: 0,
    hasSecret: false,
    config: {
      url: "",
      host: "",
      port: 465,
      security: "tls",
      username: "",
      secret: "",
      from: "",
      to: [],
    },
  });
  const initialState = {
    channels: [] as Channel[],
    deliveries: [] as Delivery[],
    loading: false,
    error: "",
    editing: false,
    busy: false,
    formError: "",
    recipients: "",
    clearSecret: false,
    draft: blank() as Channel,
    confirmation: undefined as
      | { kind: "test" | "archive"; channel: Channel }
      | undefined,
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const seq = useRef(0);
  const appState = useApp();
  useI18n();
  async function load() {
    const n = ++seq.current;
    store.setState({ loading: true });
    try {
      const [cs, ds] = await Promise.all([
        can("notification:manage")
          ? api<Channel[]>("/notification-channels")
          : [],
        api<Delivery[]>("/notification-deliveries"),
      ]);
      if (n === seq.current) {
        store.setState({ channels: cs });
        store.setState({ deliveries: ds });
        store.setState({ error: "" });
      }
    } catch (e) {
      if (n === seq.current) store.setState({ error: (e as Error).message });
    } finally {
      if (n === seq.current) store.setState({ loading: false });
    }
  }
  function edit(c?: Channel) {
    store.setState({ draft: c ? JSON.parse(JSON.stringify(c)) : blank() });
    store.setState({ recipients: store.getState().draft.config.to.join("\n") });
    store.setState({ clearSecret: false });
    store.setState({ formError: "" });
    store.setState({ editing: true });
  }
  async function save() {
    store.setState({ busy: true });
    store.setState({ formError: "" });
    try {
      const d = store.getState().draft;
      await api(
        d.id ? `/notification-channels/${d.id}` : "/notification-channels",
        d.id ? "PATCH" : "POST",
        {
          name: d.name,
          kind: d.kind,
          enabled: d.enabled,
          version: d.version,
          config: {
            ...d.config,
            to: store
              .getState()
              .recipients.split(/[\n,;，；]/)
              .map((s) => s.trim())
              .filter(Boolean),
          },
          clearSecret: store.getState().clearSecret,
        },
      );
      store.setState({ editing: false });
      notify(t("通知渠道已保存"));
      await load();
    } catch (e) {
      store.setState({ formError: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  async function confirm() {
    const confirmation = store.getState().confirmation;
    if (!confirmation) return;
    store.setState({ busy: true });
    try {
      const { kind, channel: c } = confirmation;
      await api(
        `/notification-channels/${c.id}${kind === "test" ? "/tests" : ""}`,
        kind === "test" ? "POST" : "DELETE",
        { version: c.version },
      );
      store.setState({ confirmation: undefined });
      notify(
        kind === "test"
          ? t("测试通知已进入队列，请查看投递记录")
          : t("渠道已归档，投递记录保留"),
      );
      await load();
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      store.setState({ busy: false });
    }
  }
  async function toggle(c: Channel) {
    store.setState({ busy: true });
    try {
      await api(`/notification-channels/${c.id}`, "PATCH", {
        ...{ name: c.name, kind: c.kind, version: c.version, config: c.config },
        enabled: !c.enabled,
      });
      notify(c.enabled ? t("渠道已停用") : t("渠道已启用"));
      await load();
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      store.setState({ busy: false });
    }
  }
  async function retry(d: Delivery) {
    store.setState({ busy: true });
    try {
      await api(`/notification-deliveries/${d.id}/retry`, "POST", {});
      notify(t("已重新加入投递队列"));
      await load();
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      store.setState({ busy: false });
    }
  }
  const stateLabel = (s: string) =>
    ({
      queued: t("等待投递"),
      sending: t("正在发送"),
      sent: t("已送达"),
      failed: t("投递失败"),
      suppressed: t("已跳过"),
    })[s] || s;
  const eventLabel = (s: string) =>
    ({ opened: t("故障告警"), resolved: t("恢复通知"), test: t("渠道测试") })[
      s
    ] || s;
  const deliveryTones: Record<string, Tone> = {
    sent: "ok",
    failed: "crit",
    suppressed: "paused",
    queued: "info",
    sending: "info",
  };
  const deliveryTone = (s: string): Tone => deliveryTones[s] || "idle";
  function latest(id: string) {
    return store.getState().deliveries.find((d) => d.channelId === id);
  }
  useEffect(() => {
    load();
  }, [appState.updatedAt]);

  return (
    <>
      <section className="panel">
        <header className="panel-head ruled">
          <div>
            <h2>{props.mode === "channels" ? t("通知渠道") : t("投递记录")}</h2>
            <p>
              {props.mode === "channels"
                ? t("新发生的故障与恢复会通知到所有已启用的渠道。")
                : t(
                    "显示最近 200 次投递；失败的投递最多尝试 4 次，也可以手动重试。",
                  )}
            </p>
          </div>
          <div className="panel-tools">
            <button
              disabled={viewState.loading}
              className="btn sm"
              onClick={() => {
                load();
              }}
            >
              <Icon
                name="RefreshCw"
                className={classNames({ spin: viewState.loading })}
              ></Icon>
              {t("刷新")}
            </button>
            {props.mode === "channels" && can("notification:manage") ? (
              <button
                className="btn sm primary"
                onClick={() => {
                  edit();
                }}
              >
                <Icon name="Plus"></Icon>
                {t("添加渠道")}
              </button>
            ) : null}
          </div>
        </header>
        {viewState.error ? (
          <div className="panel-body pad">
            <p role="alert" className="form-error">
              {viewState.error}
            </p>
          </div>
        ) : null}
        {props.mode === "channels" ? (
          <>
            {!viewState.channels.length ? (
              <div className="empty">
                <Icon name="Send" size={28}></Icon>
                <h3>{t("为告警连接一个通知渠道")}</h3>
                <p>
                  {t(
                    "支持 SMTP 邮件与通用 Webhook，启用后发送新发生的故障和恢复。",
                  )}
                </p>
                <button
                  className="btn primary"
                  onClick={() => {
                    edit();
                  }}
                >
                  <Icon name="Plus"></Icon>
                  {t("添加渠道")}
                </button>
              </div>
            ) : (
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th>{t("渠道")}</th>
                      <th>{t("发送到")}</th>
                      <th>{t("状态")}</th>
                      <th>{t("最近投递")}</th>
                      <th className="actions">
                        <span className="sr-only">{t("操作")}</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <>
                      {viewState.channels.map((c) => (
                        <tr key={c.id}>
                          <td>
                            <span className="person">
                              <Icon
                                name={c.kind === "smtp" ? "Mail" : "Webhook"}
                              ></Icon>
                              <span className="cell-main">
                                <strong>{c.name}</strong>
                                <small>
                                  {c.kind === "smtp"
                                    ? t("SMTP 邮件")
                                    : t("通用 Webhook")}
                                </small>
                              </span>
                            </span>
                          </td>
                          <td>
                            <span
                              title={
                                c.kind === "smtp"
                                  ? c.config.to.join(", ")
                                  : c.config.url
                              }
                              className="channel-dest"
                            >
                              {c.kind === "smtp"
                                ? c.config.to.join(", ")
                                : c.config.url}
                            </span>
                          </td>
                          <td>
                            <StatusBadge
                              status={c.enabled ? "enabled" : "disabled"}
                              label={c.enabled ? t("已启用") : t("已停用")}
                            ></StatusBadge>
                          </td>
                          <td>
                            {latest(c.id) ? (
                              <span className="cell-main">
                                <span className="state-text">
                                  <StatusGlyph
                                    tone={deliveryTone(latest(c.id)!.state)}
                                    size={9}
                                  ></StatusGlyph>
                                  {stateLabel(latest(c.id)!.state)}
                                </span>
                                <small>
                                  {dateTime(latest(c.id)!.createdAt)}
                                </small>
                              </span>
                            ) : (
                              <span className="muted">{t("暂无投递")}</span>
                            )}
                          </td>
                          <td className="actions">
                            <div className="row-actions">
                              <button
                                className="link"
                                onClick={() => {
                                  edit(c);
                                }}
                              >
                                {t("编辑")}
                              </button>
                              <button
                                disabled={!c.enabled || viewState.busy}
                                className="link"
                                onClick={() => {
                                  store.setState({
                                    confirmation: {
                                      kind: "test",
                                      channel: JSON.parse(JSON.stringify(c)),
                                    },
                                  });
                                }}
                              >
                                {t("发送测试")}
                              </button>
                              <button
                                disabled={viewState.busy}
                                className="link"
                                onClick={() => {
                                  toggle(c);
                                }}
                              >
                                {c.enabled ? t("停用") : t("启用")}
                              </button>
                              <button
                                aria-label={t("归档渠道 ") + c.name}
                                title={t("归档")}
                                className="icon-btn sm"
                                onClick={() => {
                                  store.setState({
                                    confirmation: {
                                      kind: "archive",
                                      channel: JSON.parse(JSON.stringify(c)),
                                    },
                                  });
                                }}
                              >
                                <Icon name="Archive" size={15}></Icon>
                              </button>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </>
                  </tbody>
                </table>
              </div>
            )}
            <footer className="panel-foot">
              <span className="note">
                <Icon name="Info" size={14}></Icon>
                {t(
                  "新渠道只接收启用之后发生的事件。静默期内未发送的通知会跳过且不补发；修改配置会取消该渠道尚未发出的投递，已经开始发送的消息仍可能送达。",
                )}
              </span>
            </footer>
          </>
        ) : (
          <>
            {!viewState.deliveries.length ? (
              <div className="empty">
                <Icon name="MailCheck" size={28}></Icon>
                <h3>{t("还没有投递记录")}</h3>
                <p>{t("添加渠道后，可以先发送一条测试通知验证连接。")}</p>
              </div>
            ) : (
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th>{t("渠道与事件")}</th>
                      <th>{t("投递状态")}</th>
                      <th>{t("结果")}</th>
                      <th>{t("创建时间")}</th>
                      <th className="actions">
                        <span className="sr-only">{t("操作")}</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <>
                      {viewState.deliveries.map((d) => (
                        <tr key={d.id}>
                          <td>
                            <span className="cell-main">
                              <strong>{d.channel}</strong>
                              <small>
                                {t("{p0} {p1} 投递 #{p2}", {
                                  p0: eventLabel(d.event),
                                  p1: d.alertId
                                    ? t("事件 #") + d.alertId
                                    : t("渠道测试"),
                                  p2: d.id,
                                })}
                              </small>
                            </span>
                          </td>
                          <td>
                            <StatusBadge
                              tone={deliveryTone(d.state)}
                              label={stateLabel(d.state)}
                            ></StatusBadge>
                          </td>
                          <td>
                            <span className="cell-main">
                              <span>{d.detail || t("等待后台处理")}</span>
                              <small>
                                {t("已尝试 {p0} 次", { p0: d.attempts })}
                                {d.state === "queued" ? (
                                  <>
                                    {t("，下次 {p0}", {
                                      p0: dateTime(d.nextAttempt),
                                    })}
                                  </>
                                ) : null}
                              </small>
                            </span>
                          </td>
                          <td className="tnum nowrap muted">
                            {dateTime(d.createdAt)}
                          </td>
                          <td className="actions">
                            {d.state === "failed" &&
                            can("notification:manage") ? (
                              <button
                                disabled={viewState.busy}
                                className="btn sm"
                                onClick={() => {
                                  retry(d);
                                }}
                              >
                                <Icon name="RotateCcw" size={14}></Icon>
                                {t("重试")}
                              </button>
                            ) : null}
                          </td>
                        </tr>
                      ))}
                    </>
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </section>
      {viewState.editing ? (
        <Modal
          title={viewState.draft.id ? t("编辑通知渠道") : t("添加通知渠道")}
          subtitle={t("启用后，新发生的告警与恢复会发送到这个渠道。")}
          wide
          onClose={() => {
            !viewState.busy && store.setState({ editing: false });
          }}
          footer={
            <>
              <button
                disabled={viewState.busy}
                className="btn"
                onClick={() => {
                  store.setState({ editing: false });
                }}
              >
                {t("取消")}
              </button>
              <button
                disabled={viewState.busy}
                className="btn primary"
                onClick={() => {
                  save();
                }}
              >
                {viewState.busy ? t("正在保存…") : t("保存渠道")}
              </button>
            </>
          }
        >
          <div className="form">
            <div className="form-grid">
              <label className="field">
                {t("渠道名称")}
                <input
                  placeholder={t("例如：运维值班通知")}
                  maxLength={40}
                  value={viewState.draft.name}
                  onChange={(event) => {
                    store.setState((current) => ({
                      draft: {
                        ...current.draft,
                        name: event.currentTarget.value,
                      },
                    }));
                  }}
                />
              </label>
              <label className="field">
                {t("通知方式")}
                <select
                  disabled={!!viewState.draft.id}
                  value={viewState.draft.kind}
                  onChange={(event) => {
                    store.setState((current) => ({
                      draft: {
                        ...current.draft,
                        kind: event.currentTarget.value,
                      },
                    }));
                  }}
                >
                  <option value="webhook">{t("通用 Webhook")}</option>
                  <option value="smtp">{t("SMTP 邮件")}</option>
                </select>
              </label>
            </div>
            {viewState.draft.kind === "webhook" ? (
              <>
                <label className="field">
                  {t("Webhook 地址")}
                  <input
                    type="url"
                    placeholder="https://hooks.example.com/monitor"
                    className="mono"
                    value={viewState.draft.config.url}
                    onChange={(event) => {
                      store.setState((current) => ({
                        draft: {
                          ...current.draft,
                          config: {
                            ...current.draft.config,
                            url: event.currentTarget.value,
                          },
                        },
                      }));
                    }}
                  />
                  <small>
                    {t(
                      "以 POST 发送 JSON 事件，接收端返回 HTTP 2xx 即视为成功；不跟随重定向。",
                    )}
                  </small>
                </label>
                <label className="field">
                  {t("签名密钥（可选）")}
                  <input
                    type="password"
                    autoComplete="new-password"
                    placeholder={
                      viewState.draft.hasSecret
                        ? t("已保存密钥，留空则保持不变")
                        : t("用于 HMAC-SHA256 签名")
                    }
                    value={viewState.draft.config.secret}
                    onChange={(event) => {
                      store.setState((current) => ({
                        draft: {
                          ...current.draft,
                          config: {
                            ...current.draft.config,
                            secret: event.currentTarget.value,
                          },
                        },
                      }));
                    }}
                  />
                  <small>
                    {t(
                      "签名放在 X-MonitorGo-Signature 请求头；接收端可用投递编号去重。",
                    )}
                  </small>
                </label>
                {viewState.draft.hasSecret ? (
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={viewState.clearSecret}
                      onChange={(event) => {
                        store.setState({
                          clearSecret: event.currentTarget.checked,
                        });
                      }}
                    />
                    {t("清除已保存的签名密钥")}
                  </label>
                ) : null}
              </>
            ) : (
              <>
                <div className="form-grid three">
                  <label className="field">
                    {t("SMTP 主机")}
                    <input
                      placeholder="smtp.example.com"
                      value={viewState.draft.config.host}
                      onChange={(event) => {
                        store.setState((current) => ({
                          draft: {
                            ...current.draft,
                            config: {
                              ...current.draft.config,
                              host: event.currentTarget.value,
                            },
                          },
                        }));
                      }}
                    />
                  </label>
                  <label className="field">
                    {t("端口")}
                    <input
                      type="number"
                      min="1"
                      max="65535"
                      value={viewState.draft.config.port}
                      onChange={(event) => {
                        store.setState((current) => ({
                          draft: {
                            ...current.draft,
                            config: {
                              ...current.draft.config,
                              port: Number(event.currentTarget.value),
                            },
                          },
                        }));
                      }}
                    />
                  </label>
                  <label className="field">
                    {t("连接安全")}
                    <select
                      value={viewState.draft.config.security}
                      onChange={(event) => {
                        store.setState((current) => ({
                          draft: {
                            ...current.draft,
                            config: {
                              ...current.draft.config,
                              security: event.currentTarget.value,
                            },
                          },
                        }));
                      }}
                    >
                      <option value="tls">{t("TLS（通常 465）")}</option>
                      <option value="starttls">
                        {t("STARTTLS（通常 587）")}
                      </option>
                    </select>
                  </label>
                </div>
                <div className="form-grid three">
                  <label className="field">
                    {t("登录账号")}
                    <input
                      autoComplete="off"
                      placeholder={t("匿名中继可留空")}
                      value={viewState.draft.config.username}
                      onChange={(event) => {
                        store.setState((current) => ({
                          draft: {
                            ...current.draft,
                            config: {
                              ...current.draft.config,
                              username: event.currentTarget.value,
                            },
                          },
                        }));
                      }}
                    />
                  </label>
                  <label className="field">
                    {t("密码或授权码")}
                    <input
                      type="password"
                      autoComplete="new-password"
                      placeholder={
                        viewState.draft.hasSecret
                          ? t("已保存，留空则保持不变")
                          : t("邮箱授权码")
                      }
                      value={viewState.draft.config.secret}
                      onChange={(event) => {
                        store.setState((current) => ({
                          draft: {
                            ...current.draft,
                            config: {
                              ...current.draft.config,
                              secret: event.currentTarget.value,
                            },
                          },
                        }));
                      }}
                    />
                  </label>
                  <label className="field">
                    {t("发件邮箱")}
                    <input
                      type="email"
                      placeholder="monitor@example.com"
                      value={viewState.draft.config.from}
                      onChange={(event) => {
                        store.setState((current) => ({
                          draft: {
                            ...current.draft,
                            config: {
                              ...current.draft.config,
                              from: event.currentTarget.value,
                            },
                          },
                        }));
                      }}
                    />
                  </label>
                </div>
                <label className="field">
                  {t("收件邮箱")}
                  <textarea
                    rows={3}
                    placeholder="oncall@example.com"
                    value={viewState.recipients}
                    onChange={(event) => {
                      store.setState({ recipients: event.currentTarget.value });
                    }}
                  ></textarea>
                  <small>
                    {t(
                      "每行一个，也可以用逗号分隔，最多 20 个。SMTP 服务接收成功不代表邮件已进入收件箱。",
                    )}
                  </small>
                </label>
              </>
            )}
            <label className="check">
              <input
                type="checkbox"
                checked={viewState.draft.enabled}
                onChange={(event) => {
                  store.setState((current) => ({
                    draft: {
                      ...current.draft,
                      enabled: event.currentTarget.checked,
                    },
                  }));
                }}
              />
              {t("启用渠道，接收新发生的故障与恢复")}
            </label>
            {viewState.formError ? (
              <p role="alert" className="form-error">
                {viewState.formError}
              </p>
            ) : null}
          </div>
        </Modal>
      ) : null}
      {viewState.confirmation ? (
        <Modal
          title={
            viewState.confirmation.kind === "test"
              ? t("发送测试通知")
              : t("归档通知渠道")
          }
          onClose={() => {
            !viewState.busy && store.setState({ confirmation: undefined });
          }}
          footer={
            <>
              <button
                disabled={viewState.busy}
                className="btn"
                onClick={() => {
                  store.setState({ confirmation: undefined });
                }}
              >
                {t("取消")}
              </button>
              <button
                disabled={viewState.busy}
                className={classNames(
                  "btn",
                  viewState.confirmation.kind === "test" ? "primary" : "danger",
                )}
                onClick={() => {
                  confirm();
                }}
              >
                {viewState.busy
                  ? t("正在提交…")
                  : viewState.confirmation.kind === "test"
                    ? t("发送测试")
                    : t("归档渠道")}
              </button>
            </>
          }
        >
          {viewState.confirmation.kind === "test" ? (
            <p>
              {t(
                "将向「{p0}」配置的{p1}实际发送一条测试消息，结果会出现在投递记录中。",
                {
                  p0: viewState.confirmation.channel.name,
                  p1:
                    viewState.confirmation.channel.kind === "smtp"
                      ? t("收件邮箱")
                      : t("Webhook 地址"),
                },
              )}
            </p>
          ) : (
            <p>
              {t("归档「{p0}」后不再向它投递，已有投递记录会保留。", {
                p0: viewState.confirmation.channel.name,
              })}
            </p>
          )}
        </Modal>
      ) : null}
    </>
  );
}
