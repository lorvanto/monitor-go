import { notice, errorMessage, type DisplayMessage } from "../lib/messages";
import { useState } from "react";
import { createStore, useStore } from "zustand";
import { t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import {
  api,
  appStore,
  can,
  dateTime,
  notify,
  refreshAll,
  useApp,
  type Host,
} from "../stores/app";
import Icon from "./Icon";
import Modal from "./Modal";
export default function AgentAccess(inputProps: { host?: Host }) {
  const props = { ...{}, ...inputProps };
  const initialState = {
    open: false,
    mode: "create",
    alias: "",
    busy: false,
    failure: "" as DisplayMessage,
    target: undefined as Host | undefined,
    result: undefined as
      | { id: string; token: string; expiresAt: number }
      | undefined,
    server:
      location.hostname === "localhost"
        ? location.origin.replace("localhost", "127.0.0.1")
        : location.origin,
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  useApp((state) => state.user);
  useI18n();
  const quoted = (s: string) => "'" + s.replaceAll("'", "'\\''") + "'";
  const getCommand = () =>
    `chmod 600 enrollment-token\n./agent -server ${quoted(store.getState().server)} -enrollment-file enrollment-token -config agent-data/config.json`;
  function start(m: string) {
    store.setState({ mode: m });
    store.setState({
      target: props.host ? JSON.parse(JSON.stringify(props.host)) : undefined,
    });
    store.setState({ alias: "" });
    store.setState({ result: undefined });
    store.setState({ failure: "" });
    store.setState({ open: true });
  }
  async function submit() {
    store.setState({ busy: true });
    store.setState({ failure: "" });
    try {
      store.setState({
        result: await api(
          store.getState().mode === "create"
            ? "/hosts/agents"
            : `/hosts/${store.getState().target!.id}/agent`,
          "POST",
          store.getState().mode === "create"
            ? { alias: store.getState().alias }
            : {
                action: store.getState().mode,
                version: store.getState().target!.version,
              },
        ),
      });
      await refreshAll(true);
      if (store.getState().mode === "revoke") {
        store.setState({ open: false });
        notify(notice("Agent 接入已撤销，旧凭据立即失效"));
      } else {
        appStore.setState({ activeHost: store.getState().result!.id });
        notify(notice("接入码已生成，有效期 15 分钟"));
      }
    } catch (e) {
      store.setState({ failure: errorMessage(e) });
    } finally {
      store.setState({ busy: false });
    }
  }
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      notify(notice("已复制"));
    } catch {
      notify(notice("无法访问剪贴板，请手动复制"), true);
    }
  }
  function download() {
    const result = store.getState().result;
    if (!result) return;
    const url = URL.createObjectURL(
      new Blob([result.token + "\n"], { type: "text/plain" }),
    );
    const a = document.createElement("a");
    a.href = url;
    a.download = "enrollment-token";
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  const command = getCommand();
  return (
    <>
      {can("host:agents") ? (
        <>
          {props.host?.source === "agent" ? (
            <button
              className="btn"
              onClick={() => {
                start("reenroll");
              }}
            >
              <Icon name="KeyRound"></Icon>
              {t("重新接入")}
            </button>
          ) : null}
          {props.host?.source === "agent" && props.host.agentEnabled ? (
            <button
              className="btn"
              onClick={() => {
                start("revoke");
              }}
            >
              {t("撤销接入")}
            </button>
          ) : null}
          <button
            className="btn primary"
            onClick={() => {
              start("create");
            }}
          >
            <Icon name="Plus"></Icon>
            {t("接入远程主机")}
          </button>
        </>
      ) : null}
      {viewState.open ? (
        <Modal
          title={
            viewState.result?.token
              ? t("安装远程 Agent")
              : viewState.mode === "create"
                ? t("接入远程主机")
                : viewState.mode === "revoke"
                  ? t("撤销 Agent 接入")
                  : t("重新接入主机")
          }
          subtitle={
            viewState.result?.token
              ? t("接入码仅在此处显示一次，15 分钟内有效。")
              : viewState.mode === "create"
                ? t("Agent 主动向本服务上报，远程主机无需开放入站端口。")
                : viewState.target?.alias || viewState.target?.hostname
          }
          wide
          onClose={() => {
            !viewState.busy && store.setState({ open: false });
          }}
          footer={
            <>
              <button
                disabled={viewState.busy}
                className="btn"
                onClick={() => {
                  store.setState({ open: false });
                }}
              >
                {viewState.result?.token ? t("完成") : t("取消")}
              </button>
              {!viewState.result?.token ? (
                <button
                  disabled={
                    viewState.busy ||
                    (viewState.mode === "create" && !viewState.alias.trim())
                  }
                  className={classNames(
                    "btn",
                    viewState.mode === "revoke" ? "danger" : "primary",
                  )}
                  onClick={() => {
                    submit();
                  }}
                >
                  {viewState.busy
                    ? t("正在处理…")
                    : viewState.mode === "create"
                      ? t("生成接入码")
                      : viewState.mode === "revoke"
                        ? t("撤销接入")
                        : t("签发新接入码")}
                </button>
              ) : null}
            </>
          }
        >
          {viewState.result?.token ? (
            <ol className="steps">
              <li>
                <div className="step-body">
                  <h3>{t("准备 Agent 程序")}</h3>
                  <p>
                    {t("把与远程主机系统和架构匹配的")}
                    <code>{"agent"}</code>
                    {t("程序放到该主机上。在项目目录执行")}
                    <code>{"go build -o bin/agent ./cmd/agent"}</code>
                    {t("构建；跨系统部署时设置 GOOS 与 GOARCH。")}
                  </p>
                </div>
              </li>
              <li>
                <div className="step-body">
                  <h3>{t("保存接入码")}</h3>
                  <p>
                    {t(
                      "与 Agent 放在同一目录，并限制为仅当前用户可读。注册成功后可以删除该文件。",
                    )}
                  </p>
                  <div className="copy-field">
                    <input
                      value={viewState.result.token}
                      readOnly
                      aria-label={t("本次 Agent 接入码")}
                    />
                    <button
                      className="btn"
                      onClick={() => {
                        copy(viewState.result!.token);
                      }}
                    >
                      <Icon name="Copy"></Icon>
                      {t("复制")}
                    </button>
                    <button
                      className="btn"
                      onClick={() => {
                        download();
                      }}
                    >
                      <Icon name="Download"></Icon>
                      {t("下载文件")}
                    </button>
                  </div>
                  <small className="field-help">
                    {t("有效至 {p0}", {
                      p0: dateTime(viewState.result.expiresAt),
                    })}
                  </small>
                </div>
              </li>
              <li>
                <div className="step-body">
                  <h3>{t("启动并等待首次上报")}</h3>
                  <label className="field">
                    {t("服务地址")}
                    <input
                      type="url"
                      value={viewState.server}
                      onChange={(event) => {
                        store.setState({ server: event.currentTarget.value });
                      }}
                    />
                    <small>
                      {t(
                        "填写远程主机能访问到的 HTTPS 地址；127.0.0.1 只适合本机联调。",
                      )}
                    </small>
                  </label>
                  <div className="snippet">
                    <pre className="code-block">{command}</pre>
                    <button
                      aria-label={t("复制启动命令")}
                      className="icon-btn sm"
                      onClick={() => {
                        copy(command);
                      }}
                    >
                      <Icon name="Copy" size={14}></Icon>
                    </button>
                  </div>
                  <p>
                    {t(
                      "Agent 约每 15 秒上报一次，超过 90 秒未上报会产生离线告警。凭据保存在配置文件中，以后用同一个",
                    )}
                    <code>{"-config"}</code>
                    {t("启动即可。")}
                  </p>
                </div>
              </li>
            </ol>
          ) : (
            <>
              {viewState.mode === "create" ? (
                <label className="field">
                  {t("主机名称")}
                  <input
                    placeholder={t("例如：生产应用服务器")}
                    maxLength={60}
                    value={viewState.alias}
                    onKeyDown={(event) => {
                      if (event.key !== "Enter") return;
                      submit();
                    }}
                    onChange={(event) => {
                      store.setState({ alias: event.currentTarget.value });
                    }}
                  />
                </label>
              ) : viewState.mode === "revoke" ? (
                <p>
                  {t(
                    "撤销后这台主机停止接收指标，现有 Agent 凭据和接入码立即失效。已保存的历史指标与告警会保留。",
                  )}
                </p>
              ) : (
                <p>
                  {t(
                    "将签发新的接入码，旧凭据立即失效。请先停止远程主机上的旧 Agent，再用新的配置文件重新注册。历史指标会继续保留。",
                  )}
                </p>
              )}
              <p className="note">
                <Icon name="ShieldCheck" size={15}></Icon>
                {t(
                  "每台主机使用独立凭据，只能上报自身指标；Agent 不提供远程执行命令的能力。",
                )}
              </p>
            </>
          )}
          {viewState.failure ? (
            <p role="alert" className="form-error">
              {viewState.failure}
            </p>
          ) : null}
        </Modal>
      ) : null}
    </>
  );
}
