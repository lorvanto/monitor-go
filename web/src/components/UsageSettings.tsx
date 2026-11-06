import { useEffect, useRef, useState } from "react";
import { isMessageKey, t, useI18n } from "../i18n";
import { APIError, api, appStore, can, dateTime, useApp } from "../stores/app";
import { errorMessage, type DisplayMessage } from "../lib/messages";
import Form from "./Form";
import Icon from "./Icon";

export interface UsageStatus {
  enabled: boolean;
  required: boolean;
  noticeVersion: string;
  confirmationRequired: boolean;
  configured: boolean;
  consoleUrl: string;
  instanceId: string;
  lastReportAt: number;
  nextReportAt: number;
  lastError: string;
  pendingActiveDays: string[];
  preview: {
    instanceId: string;
    version: string;
    os: string;
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
  const { usageSettingsRequested } = useApp(
    ({ user, usageSettingsRequested }) => ({ user, usageSettingsRequested }),
  );
  const heading = useRef<HTMLHeadingElement>(null);
  const [status, setStatus] = useState<UsageStatus>();
  const [consentFor, setConsentFor] = useState("");
  const [noticeStale, setNoticeStale] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<DisplayMessage>("");
  const writable = can("system:settings");
  function accept(value: UsageStatus) {
    setStatus(value);
    appStore.setState({
      usageEnabled: value.enabled,
      usageConfirmationRequired: value.confirmationRequired,
    });
  }
  async function reload() {
    if (busy) return;
    setConsentFor("");
    setBusy(true);
    try {
      accept(await api<UsageStatus>("/usage"));
      setNoticeStale(false);
      setError("");
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  async function update(path: string, method: string, body: unknown) {
    if (busy || !writable) return;
    setBusy(true);
    try {
      accept(await api<UsageStatus>(path, method, body));
      if (path === "/usage") setConsentFor("");
      setError("");
    } catch (e) {
      setError(errorMessage(e));
      if (
        e instanceof APIError &&
        typeof e.display !== "string" &&
        (e.display.key ===
          "Activation destination has changed; review it again" ||
          e.display.key === "Activation notice has changed; review it again")
      ) {
        setConsentFor("");
        setNoticeStale(true);
      }
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    void reload();
  }, []);
  useEffect(() => {
    if (!usageSettingsRequested || !status) return;
    appStore.setState({ usageSettingsRequested: false });
    heading.current?.scrollIntoView({ block: "start" });
    heading.current?.focus({ preventScroll: true });
  }, [usageSettingsRequested, status]);
  const destination = consoleLink(status?.consoleUrl || "");
  const consentKey = status
    ? JSON.stringify([status.consoleUrl, status.noticeVersion])
    : "";
  const consent = !!consentKey && consentFor === consentKey;
  const canConfirm =
    !!status?.configured &&
    !!destination &&
    status?.noticeVersion === "2" &&
    !noticeStale;
  return (
    <section className="panel usage-settings" aria-labelledby="usage-title">
      <header className="panel-head ruled">
        <div>
          <h2 id="usage-title" ref={heading} tabIndex={-1}>
            {t("匿名安装统计")}
          </h2>
          <p>{t("标准客户端确认规则后持续上报，程序内不提供关闭开关。")}</p>
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
            "仅分享随机实例编号、产品版本、系统类型和按 UTC 日期汇总的界面活跃标记。不会上传账号身份、架构、部署方式、监控内容、文件、监控密钥或应用密钥。",
          )}
        </p>
        <p className="note">
          {t(
            "确认后生成独立实例凭据，仅用于验证匿名登记和后续上报；预览不会显示该凭据。",
          )}
        </p>
        <p className="note">
          {t("统计按独立安装和活跃安装计算，不代表自然人数。")}
        </p>
        <p className="note">
          {t(
            "激活后持续上报，程序内不提供关闭选项。管理后台不可用时会稍后重试，不影响本地登录、监控和告警。",
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
                <dd>
                  {status.enabled
                    ? t("持续上报中")
                    : t("待管理员确认持续上报规则")}
                </dd>
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
                <dt>{t("随机实例编号")}</dt>
                <dd className="mono">{status.instanceId || t("确认后生成")}</dd>
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
                {t("请部署者配置管理后台地址，再阅读并确认持续上报规则。")}
              </p>
            ) : null}
            {status.confirmationRequired && status.noticeVersion !== "2" ? (
              <p role="alert" className="form-error">
                {t("Activation notice has changed; review it again")}
              </p>
            ) : null}
            {writable ? (
              status.confirmationRequired ? (
                <Form
                  className="usage-consent"
                  onSubmit={(event) => {
                    event.preventDefault();
                    if (consent && canConfirm)
                      void update("/usage", "PATCH", {
                        enabled: true,
                        usageConsent: true,
                        usageNoticeVersion: status.noticeVersion,
                        usageConsoleUrl: status.consoleUrl,
                      });
                  }}
                >
                  <label className="check">
                    <input
                      type="checkbox"
                      required
                      checked={consent}
                      disabled={busy || !canConfirm}
                      onChange={(event) =>
                        setConsentFor(
                          event.currentTarget.checked ? consentKey : "",
                        )
                      }
                    />
                    {t(
                      "我已阅读目的地、字段和规则，同意激活后持续上报，且程序内无法关闭。",
                    )}
                  </label>
                  <button
                    className="btn primary"
                    disabled={busy || !canConfirm || !consent}
                  >
                    {t("确认规则并激活上报")}
                  </button>
                </Form>
              ) : null
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
                    {status.preview.instanceId || t("确认后生成")}
                  </dd>
                </div>
                <div>
                  <dt>{t("产品版本")}</dt>
                  <dd>{status.preview.version}</dd>
                </div>
                <div>
                  <dt>{t("系统类型")}</dt>
                  <dd>{status.preview.os}</dd>
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
          </>
        )}
      </div>
    </section>
  );
}
