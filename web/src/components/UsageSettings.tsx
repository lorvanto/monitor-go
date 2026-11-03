import { useEffect, useState } from "react";
import { isMessageKey, t, useI18n } from "../i18n";
import { api, appStore, can, dateTime, useApp } from "../stores/app";
import { errorMessage, type DisplayMessage } from "../lib/messages";
import Form from "./Form";
import Icon from "./Icon";

export interface UsageStatus {
  enabled: boolean;
  configured: boolean;
  consoleUrl: string;
  instanceId: string;
  lastReportAt: number;
  nextReportAt: number;
  lastError: string;
  account: { login: string } | null;
  pendingActiveDays: string[];
  preview: {
    instanceId: string;
    version: string;
    os: string;
    arch: string;
    deployment: "native" | "docker";
    activeDays: string[];
  };
}
function consoleLink(value: string): string | undefined {
  try {
    const url = new URL(value);
    if (
      !url.username &&
      !url.password &&
      (url.protocol === "https:" ||
        (url.protocol === "http:" &&
          ["127.0.0.1", "[::1]"].includes(url.hostname)))
    )
      return url.href;
  } catch {
    // Do not turn malformed configuration into a clickable destination.
  }
}
export default function UsageSettings() {
  useI18n();
  useApp(({ user }) => user);
  const [status, setStatus] = useState<UsageStatus>();
  const [consent, setConsent] = useState(false);
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<DisplayMessage>("");
  const writable = can("system:settings");
  function accept(value: UsageStatus) {
    setStatus(value);
    appStore.setState({ usageEnabled: value.enabled });
  }
  async function reload() {
    setBusy(true);
    try {
      accept(await api<UsageStatus>("/usage"));
      setError("");
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  async function update(path: string, method: string, body: unknown) {
    setBusy(true);
    try {
      accept(await api<UsageStatus>(path, method, body));
      if (path === "/usage/bind") setCode("");
      if (path === "/usage") setConsent(false);
      setError("");
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    void reload();
  }, []);
  const destination = consoleLink(status?.consoleUrl || "");
  return (
    <section className="panel usage-settings" aria-labelledby="usage-title">
      <header className="panel-head ruled">
        <div>
          <h2 id="usage-title">{t("使用情况统计与账号绑定")}</h2>
          <p>{t("默认关闭，由管理员自愿开启，可随时关闭。")}</p>
        </div>
        <button className="btn" type="button" disabled={busy} onClick={reload}>
          <Icon name="RefreshCw" />
          {t("刷新上报状态")}
        </button>
      </header>
      <div className="panel-body usage-content">
        <p className="note">
          <Icon name="Info" size={16} />
          {t(
            "仅分享随机实例编号、版本、系统、架构、部署方式和按天汇总的界面活跃标记。不会上传监控内容、文件、账号列表、监控密钥或应用密钥。",
          )}
        </p>
        <p className="note">
          {t(
            "开启后会生成独立实例凭据，用于验证登记、上报及账号绑定请求；预览不会显示该凭据。",
          )}
        </p>
        <p className="note">
          {t(
            "程序运行与有人使用分开统计。上报在后台低频执行，服务不可用或关闭上报不会影响安装、登录、监控和告警。",
          )}
        </p>
        {error ? (
          <p role="alert" className="form-error">
            {error}
          </p>
        ) : null}
        {!status ? (
          <p role="status">{busy ? t("正在读取…") : t("暂无信息")}</p>
        ) : (
          <>
            <dl className="kv rows usage-summary">
              <div>
                <dt>{t("使用情况上报")}</dt>
                <dd>{status.enabled ? t("已启用上报") : t("上报已关闭")}</dd>
              </div>
              <div>
                <dt>{t("上报目的地")}</dt>
                <dd>
                  {destination ? (
                    <a
                      href={destination}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {status.consoleUrl}
                    </a>
                  ) : (
                    t("尚未配置管理后台")
                  )}
                </dd>
              </div>
              <div>
                <dt>{t("最近成功上报")}</dt>
                <dd>
                  {status.lastReportAt
                    ? dateTime(status.lastReportAt * 1000)
                    : t("尚未上报")}
                </dd>
              </div>
              {status.enabled && status.nextReportAt > 0 ? (
                <div>
                  <dt>{t("下次上报时间")}</dt>
                  <dd>{dateTime(status.nextReportAt * 1000)}</dd>
                </div>
              ) : null}
              <div>
                <dt>{t("绑定的 Console 账号")}</dt>
                <dd>{status.account?.login || t("尚未绑定账号")}</dd>
              </div>
            </dl>
            {status.lastError && status.enabled ? (
              <p role="status" className="note">
                {isMessageKey(status.lastError)
                  ? t(status.lastError)
                  : t("Usage service is unavailable")}
              </p>
            ) : null}
            {!status.configured ? (
              <p className="note">
                {t("请先由部署者配置管理后台地址，再决定是否开启上报。")}
              </p>
            ) : null}
            {writable ? (
              status.enabled ? (
                <div className="usage-actions">
                  <button
                    type="button"
                    className="btn"
                    disabled={busy}
                    onClick={() =>
                      void update("/usage", "PATCH", { enabled: false })
                    }
                  >
                    <Icon name="Pause" />
                    {t("立即关闭上报")}
                  </button>
                  <p>
                    {t(
                      "关闭后停止上报并清除待发送的活跃记录。账号关联会保留，可在管理后台解除。",
                    )}
                  </p>
                </div>
              ) : (
                <Form
                  className="usage-consent"
                  onSubmit={(event) => {
                    event.preventDefault();
                    if (consent && status.configured)
                      void update("/usage", "PATCH", { enabled: true });
                  }}
                >
                  <label className="check">
                    <input
                      type="checkbox"
                      required
                      checked={consent}
                      disabled={busy || !status.configured}
                      onChange={(event) =>
                        setConsent(event.currentTarget.checked)
                      }
                    />
                    {t("我已了解上报内容，同意向上述管理后台分享使用情况。")}
                  </label>
                  <button
                    className="btn primary"
                    disabled={busy || !status.configured || !consent}
                  >
                    {t("启用使用情况上报")}
                  </button>
                </Form>
              )
            ) : (
              <p className="note">
                {t("当前账号只能查看设置，修改需要“修改系统设置”权限。")}
              </p>
            )}
            <details className="usage-preview">
              <summary>{t("查看上报字段")}</summary>
              <dl className="kv rows">
                <div>
                  <dt>{t("随机实例编号")}</dt>
                  <dd className="mono">
                    {status.preview.instanceId || t("开启后生成")}
                  </dd>
                </div>
                <div>
                  <dt>{t("应用版本")}</dt>
                  <dd>{status.preview.version}</dd>
                </div>
                <div>
                  <dt>{t("操作系统")}</dt>
                  <dd>{status.preview.os}</dd>
                </div>
                <div>
                  <dt>{t("处理器架构")}</dt>
                  <dd>{status.preview.arch}</dd>
                </div>
                <div>
                  <dt>{t("部署方式")}</dt>
                  <dd>
                    {status.preview.deployment === "docker"
                      ? t("Docker 容器")
                      : t("原生进程")}
                  </dd>
                </div>
                <div>
                  <dt>{t("待发送的活跃日期（UTC）")}</dt>
                  <dd>
                    {status.preview.activeDays?.length
                      ? status.preview.activeDays.join(", ")
                      : t("暂无活跃记录")}
                  </dd>
                </div>
              </dl>
            </details>
            <div className="usage-binding">
              <h3>{t("自愿绑定 Console 账号")}</h3>
              <p>
                {t(
                  "登录 Monitor Console 后生成一次性绑定码，再粘贴到这里。本地账号和可选的 GitHub 登录均可使用，不会读取实例内其他成员的身份。",
                )}
              </p>
              {destination ? (
                <a
                  className="btn"
                  href={destination}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <Icon name="ExternalLink" />
                  {t("打开管理后台")}
                </a>
              ) : null}
              {writable && status.enabled ? (
                status.account ? (
                  <button
                    className="btn"
                    type="button"
                    disabled={busy}
                    onClick={() => void update("/usage/unbind", "POST", {})}
                  >
                    {t("解除 Console 账号绑定")}
                  </button>
                ) : (
                  <Form
                    className="usage-bind-form"
                    onSubmit={(event) => {
                      event.preventDefault();
                      void update("/usage/bind", "POST", { code: code.trim() });
                    }}
                  >
                    <label className="field">
                      {t("一次性绑定码")}
                      <input
                        aria-label={t("一次性绑定码")}
                        aria-describedby="usage-code-help"
                        type="password"
                        autoComplete="off"
                        spellCheck={false}
                        required
                        minLength={64}
                        maxLength={64}
                        pattern="[a-f0-9]{64}"
                        value={code}
                        disabled={busy}
                        onChange={(event) => setCode(event.currentTarget.value)}
                      />
                      <small id="usage-code-help">
                        {t("绑定码有效期为 10 分钟，只能使用一次。")}
                      </small>
                    </label>
                    <button className="btn" disabled={busy || !code.trim()}>
                      {t("绑定 Console 账号")}
                    </button>
                  </Form>
                )
              ) : !status.enabled ? (
                <p className="note">
                  {t(
                    "启用上报后可在此绑定或解除账号；上报关闭期间可直接到管理后台解除关联。",
                  )}
                </p>
              ) : null}
            </div>
          </>
        )}
      </div>
    </section>
  );
}
