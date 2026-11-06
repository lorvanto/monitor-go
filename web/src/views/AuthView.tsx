import { errorMessage, notice, type DisplayMessage } from "../lib/messages";
import Form from "../components/Form";
import { useState } from "react";
import { createStore, useStore } from "zustand";
import BrandMark from "../components/BrandMark";
import LanguageSelect from "../components/LanguageSelect";
import StatusGlyph from "../components/StatusGlyph";
import { t, useI18n } from "../i18n";
import {
  APIError,
  api,
  appStore,
  boot,
  signIn,
  useApp,
  type SetupStatus,
} from "../stores/app";
export default function AuthView() {
  const initialState = {
    username: "",
    password: "",
    name: "",
    token: "",
    consentFor: "",
    noticeStale: false,
    error: "" as DisplayMessage,
    busy: false,
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const appState = useApp(({ initialized, connectionError, activation }) => ({
    initialized,
    connectionError,
    activation,
  }));
  useI18n();
  const activation =
    !appState.initialized && appState.activation?.required
      ? appState.activation
      : null;
  // Consent applies to the exact destination and notice shown to the installer.
  const consentKey = activation
    ? JSON.stringify([activation.consoleUrl, activation.noticeVersion])
    : "";
  const consented = !!consentKey && viewState.consentFor === consentKey;
  const activationReady =
    !activation ||
    (!viewState.noticeStale &&
      activation.configured &&
      !!activation.consoleUrl &&
      activation.noticeVersion === "2");
  async function refreshActivation() {
    if (store.getState().busy) return;
    store.setState({ busy: true, consentFor: "", error: "" });
    try {
      const status = await api<SetupStatus>("/status");
      appStore.setState({
        initialized: status.initialized,
        activation: status.initialized ? null : status.activation || null,
        connectionError: "",
      });
      store.setState({ noticeStale: false });
    } catch (error) {
      store.setState({ error: errorMessage(error) });
    } finally {
      store.setState({ busy: false });
    }
  }
  async function submit() {
    if (store.getState().busy || appState.connectionError) return;
    if (activation && (!activation.configured || !activation.consoleUrl)) {
      store.setState({ error: notice("Activation service is not configured") });
      return;
    }
    if (
      activation &&
      (viewState.noticeStale || activation.noticeVersion !== "2")
    ) {
      store.setState({
        error: notice("Activation notice has changed; review it again"),
      });
      return;
    }
    if (activation && !consented) {
      store.setState({ error: notice("Activation consent is required") });
      return;
    }
    store.setState({ busy: true });
    store.setState({ error: "" });
    try {
      await signIn(
        appStore.getState().initialized
          ? {
              username: store.getState().username,
              password: store.getState().password,
            }
          : {
              username: store.getState().username,
              password: store.getState().password,
              name: store.getState().name,
              token: store.getState().token,
              ...(activation
                ? {
                    usageConsent: true,
                    usageNoticeVersion: activation.noticeVersion,
                    usageConsoleUrl: activation.consoleUrl,
                  }
                : {}),
            },
        !appStore.getState().initialized,
      );
    } catch (e) {
      store.setState({ error: errorMessage(e) });
      if (
        e instanceof APIError &&
        typeof e.display !== "string" &&
        (e.display.key ===
          "Activation destination has changed; review it again" ||
          e.display.key === "Activation notice has changed; review it again")
      ) {
        store.setState({ consentFor: "", noticeStale: true });
      }
    } finally {
      store.setState({ busy: false });
    }
  }

  return (
    <>
      <main className="auth">
        <div
          className={`auth-card${activation ? " auth-card-activation" : ""}`}
        >
          <div className="auth-brand">
            <BrandMark size={24}></BrandMark>
            {"Monitor Go"}
          </div>
          <LanguageSelect></LanguageSelect>
          <div className="auth-intro">
            <h1>{appState.initialized ? t("登录") : t("创建首个管理员")}</h1>
            <p>
              {appState.initialized
                ? t("使用团队成员账号登录，查看主机、检测与告警。")
                : t("填写初始化码，为这套安装设置第一个管理员账号。")}
            </p>
          </div>
          {appState.connectionError ? (
            <div data-tone="crit" role="alert" className="banner">
              <StatusGlyph tone="crit"></StatusGlyph>
              <div className="banner-text">
                <strong>{t("无法连接监控服务")}</strong>
                <p>{appState.connectionError}</p>
              </div>
              <button
                className="link"
                onClick={() => {
                  boot();
                }}
              >
                {t("重新连接")}
              </button>
            </div>
          ) : null}
          <Form
            className="panel auth-panel form"
            onSubmit={(event) => {
              event.preventDefault();
              submit();
            }}
          >
            {activation ? (
              <section
                className="activation-notice"
                aria-labelledby="activation-heading"
              >
                <h2 id="activation-heading">{t("首次安装在线激活")}</h2>
                <p>
                  {t(
                    "首次安装需要连接下方管理后台，匿名登记此安装并统计活跃情况。",
                  )}
                </p>
                <dl>
                  <dt>{t("上报目的地")}</dt>
                  <dd>
                    <code>
                      {activation.consoleUrl || t("尚未配置管理后台")}
                    </code>
                  </dd>
                </dl>
                <p>
                  {t(
                    "随机实例 ID、产品版本、系统类型和按 UTC 日期汇总的界面活跃标记。",
                  )}
                </p>
                <p>
                  {t(
                    "不会发送账号身份、架构、部署方式、监控目标或指标、文件、密码、监控密钥、应用密钥或本地成员列表。",
                  )}
                </p>
                <p>
                  {t(
                    "独立实例凭据仅用于验证登记和后续上报请求，不会在页面显示。",
                  )}
                </p>
                <p>{t("统计按独立安装和活跃安装计算，不代表自然人数。")}</p>
                <p>
                  {t(
                    "激活后持续上报，程序内不提供关闭选项。管理后台不可用时会稍后重试，不影响本地登录、监控和告警。",
                  )}
                </p>
                {!activation.configured ||
                !activation.consoleUrl ||
                activation.noticeVersion !== "2" ? (
                  <p className="form-error" role="alert">
                    {t(
                      activation.noticeVersion !== "2"
                        ? "Activation notice has changed; review it again"
                        : "Activation service is not configured",
                    )}
                  </p>
                ) : null}
                <label className="check activation-consent">
                  <input
                    type="checkbox"
                    required
                    checked={consented}
                    disabled={viewState.busy || !activationReady}
                    onChange={(event) =>
                      store.setState({
                        consentFor: event.currentTarget.checked
                          ? consentKey
                          : "",
                      })
                    }
                  />
                  <span>
                    {t(
                      "我已阅读目的地、字段和规则，同意激活后持续上报，且程序内无法关闭。",
                    )}
                  </span>
                </label>
                <button
                  type="button"
                  className="link"
                  disabled={viewState.busy}
                  onClick={refreshActivation}
                >
                  {t("刷新激活说明")}
                </button>
              </section>
            ) : null}
            {!appState.initialized ? (
              <label className="field">
                {t("初始化码")}
                <input
                  required
                  autoComplete="off"
                  value={viewState.token}
                  onChange={(event) => {
                    store.setState({ token: event.currentTarget.value });
                  }}
                />
                <small>
                  {t(
                    "在运行服务的主机上读取数据目录中的 setup-token 文件，初始化完成后该文件会自动删除。",
                  )}
                </small>
              </label>
            ) : null}
            <label className="field">
              {t("用户名")}
              <input
                required
                autoComplete="username"
                placeholder={
                  appState.initialized ? "" : t("3–40 位英文、数字、点或下划线")
                }
                value={viewState.username}
                onChange={(event) => {
                  store.setState({ username: event.currentTarget.value });
                }}
              />
            </label>
            {!appState.initialized ? (
              <label className="field">
                {t("姓名")}
                <input
                  required
                  autoComplete="name"
                  value={viewState.name}
                  onChange={(event) => {
                    store.setState({ name: event.currentTarget.value });
                  }}
                />
              </label>
            ) : null}
            <label className="field">
              {t("密码")}
              <input
                type="password"
                required
                autoComplete={
                  appState.initialized ? "current-password" : "new-password"
                }
                minLength={appState.initialized ? undefined : 12}
                value={viewState.password}
                onChange={(event) => {
                  store.setState({ password: event.currentTarget.value });
                }}
              />
              {!appState.initialized ? (
                <small>{t("12–72 字节，请使用不与其他系统共用的密码。")}</small>
              ) : null}
            </label>
            {viewState.error ? (
              <p role="alert" className="form-error">
                {viewState.error}
              </p>
            ) : null}
            <button
              disabled={
                viewState.busy ||
                !!appState.connectionError ||
                !activationReady ||
                (!!activation && !consented)
              }
              className="btn primary"
            >
              {appState.initialized
                ? viewState.busy
                  ? t("正在登录…")
                  : t("登录")
                : activation
                  ? viewState.busy
                    ? t("正在激活并创建…")
                    : t("激活并创建管理员")
                  : viewState.busy
                    ? t("正在创建…")
                    : t("创建管理员并进入")}
            </button>
          </Form>
          <p className="auth-foot">
            {t("数据保存在这台服务器的 SQLite 文件中")}
          </p>
        </div>
      </main>
    </>
  );
}
