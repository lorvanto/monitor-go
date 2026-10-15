import { errorMessage, type DisplayMessage } from "../lib/messages";
import Form from "../components/Form";
import { useState } from "react";
import { createStore, useStore } from "zustand";
import BrandMark from "../components/BrandMark";
import LanguageSelect from "../components/LanguageSelect";
import StatusGlyph from "../components/StatusGlyph";
import { t, useI18n } from "../i18n";
import { appStore, boot, signIn, useApp } from "../stores/app";
export default function AuthView() {
  const initialState = {
    username: "",
    password: "",
    name: "",
    token: "",
    error: "" as DisplayMessage,
    busy: false,
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const appState = useApp();
  useI18n();
  async function submit() {
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
            },
        !appStore.getState().initialized,
      );
    } catch (e) {
      store.setState({ error: errorMessage(e) });
    } finally {
      store.setState({ busy: false });
    }
  }

  return (
    <>
      <main className="auth">
        <div className="auth-card">
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
              disabled={viewState.busy || !!appState.connectionError}
              className="btn primary"
            >
              {appState.initialized
                ? viewState.busy
                  ? t("正在登录…")
                  : t("登录")
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
