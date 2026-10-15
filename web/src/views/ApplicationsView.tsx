import { useEffect, useRef, useState } from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import Modal from "../components/Modal";
import StatusBadge from "../components/StatusBadge";
import StatusGlyph from "../components/StatusGlyph";
import { metadataLabel, t, useI18n } from "../i18n";
import {
  dateTime,
  type Application,
  type ApplicationKey,
} from "../lib/applicationTypes";
import { classNames } from "../lib/react";
import { api, can, notify, useApp } from "../stores/app";
export default function ApplicationsView() {
  const initialState = {
    items: [] as Application[],
    selected: "",
    search: "",
    error: "",
    loading: true,
    busy: false,
    editing: false,
    keyEditing: false,
    secret: "",
    tab: "keys",
    keys: [] as ApplicationKey[],
    scopes: [] as { key: string; name: string; description: string }[],
    keyLoading: false,
    target: undefined as Application | undefined,
    rotate: undefined as ApplicationKey | undefined,
    confirm: "" as "archive" | "revoke" | "",
    revokeTarget: undefined as ApplicationKey | undefined,
    form: {
      slug: "",
      name: "",
      description: "",
      enabled: true,
      version: 0,
    },
    keyForm: { name: "", days: 90, scopes: [] as string[] },
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const keysSequence = useRef(0);
  const loadSequence = useRef(0);
  const appState = useApp();
  useI18n();
  const getApp = () =>
    store.getState().items.find((v) => v.id === store.getState().selected);
  const getFiltered = () =>
    store
      .getState()
      .items.filter((v) =>
        `${v.name} ${v.slug}`
          .toLowerCase()
          .includes(store.getState().search.toLowerCase()),
      );
  const getCurlExample = () =>
    `curl '${location.origin}/api/v1/integrations/me' \\\n  -H "Authorization: Bearer $MONITOR_GO_API_KEY"`;
  async function load() {
    const request = ++loadSequence.current;
    store.setState({ loading: true });
    try {
      const items = await api<Application[]>("/applications");
      if (request !== loadSequence.current) return;
      store.setState({ items });
      if (
        !store.getState().items.some((v) => v.id === store.getState().selected)
      )
        store.setState({ selected: store.getState().items[0]?.id || "" });
      store.setState({ error: "" });
    } catch (e) {
      if (request === loadSequence.current)
        store.setState({ error: (e as Error).message });
    } finally {
      if (request === loadSequence.current) store.setState({ loading: false });
    }
  }
  async function loadKeys() {
    const request = ++keysSequence.current;
    const id = store.getState().selected;
    store.setState({ keys: [], scopes: [], keyLoading: false });
    if (!id || !can("application:keys")) return;
    store.setState({ keyLoading: true });
    try {
      const v = await api<{
        items: ApplicationKey[];
        scopes: LocalState["scopes"];
      }>(`/applications/${id}/keys`);
      if (
        request === keysSequence.current &&
        id === store.getState().selected
      ) {
        store.setState({ keys: v.items });
        store.setState({ scopes: v.scopes });
      }
    } catch (e) {
      if (request === keysSequence.current && id === store.getState().selected)
        store.setState({ error: (e as Error).message });
    } finally {
      if (request === keysSequence.current && id === store.getState().selected)
        store.setState({ keyLoading: false });
    }
  }
  function edit(a?: Application) {
    store.setState({ target: a });
    store.setState({
      form: {
        ...store.getState().form,
        ...{
          slug: a?.slug || "",
          name: a?.name || "",
          description: a?.description || "",
          enabled: a?.enabled ?? true,
          version: a?.version || 0,
        },
      },
    });
    store.setState({ error: "" });
    store.setState({ editing: true });
  }
  async function save() {
    const target = store.getState().target;
    store.setState({ busy: true });
    try {
      const v = await api<{
        id: string;
      }>(
        target ? `/applications/${target.id}` : "/applications",
        target ? "PATCH" : "POST",
        store.getState().form,
      );
      store.setState({ editing: false });
      store.setState({ selected: v.id });
      await load();
      notify(t("应用已保存"));
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  function newKey(k?: ApplicationKey) {
    store.setState({ rotate: k });
    store.setState({
      keyForm: {
        ...store.getState().keyForm,
        ...{
          name: k?.name || "",
          days: 90,
          scopes: k ? [...k.scopes] : [],
        },
      },
    });
    store.setState({ error: "" });
    store.setState({ keyEditing: true });
  }
  async function issue() {
    const app = getApp();
    if (!app) return;
    store.setState({ busy: true });
    try {
      const v = await api<{
        token: string;
      }>(`/applications/${app.id}/keys`, "POST", {
        ...store.getState().keyForm,
        rotateId: store.getState().rotate?.id || "",
        version: store.getState().rotate?.version || 0,
      });
      store.setState({ keyEditing: false });
      store.setState({ secret: v.token });
      await load();
      await loadKeys();
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  async function execute() {
    const app = getApp();
    const revokeTarget = store.getState().revokeTarget;
    if (!app) return;
    store.setState({ busy: true });
    try {
      if (store.getState().confirm === "archive")
        await api(`/applications/${app.id}`, "DELETE", {
          version: app.version,
        });
      else if (revokeTarget)
        await api(`/application-keys/${revokeTarget.id}`, "DELETE", {
          version: revokeTarget.version,
        });
      store.setState({ confirm: "" });
      await load();
      await loadKeys();
      notify(t("操作已完成"));
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      notify(t("已复制"));
    } catch {
      notify(t("复制失败，请手动选择复制"), true);
    }
  }
  function status(k: ApplicationKey) {
    return k.revokedAt
      ? t("已撤销")
      : k.expiresAt <= Date.now()
        ? t("已过期")
        : t("有效");
  }
  const keyStatus = (k: ApplicationKey) =>
    k.revokedAt ? "revoked" : k.expiresAt <= Date.now() ? "expired" : "valid";
  const getActiveKeys = () =>
    store.getState().items.reduce((n, v) => n + v.activeKeys, 0);
  useEffect(() => {
    store.setState({ error: "" });
    loadKeys();
    return () => {
      keysSequence.current++;
    };
  }, [viewState.selected, can("application:keys")]);
  useEffect(() => {
    load();
    return () => {
      loadSequence.current++;
      keysSequence.current++;
      store.setState({ secret: "" });
    };
  }, []);
  const app = getApp();
  const filtered = getFiltered();
  const curlExample = getCurlExample();
  const activeKeys = getActiveKeys();
  return (
    <>
      <header className="page-head">
        <div className="page-title">
          <h1>{appState.pageTitle}</h1>
          <p>
            {t(
              "{p0} 个应用，{p1} 个有效密钥。每个密钥单独授权，到期、轮换或撤销后立即失效。",
              { p0: viewState.items.length, p1: activeKeys },
            )}
          </p>
        </div>
        <div className="page-actions">
          <button
            disabled={viewState.loading}
            className="btn"
            onClick={() => {
              load();
              loadKeys();
            }}
          >
            <Icon name="RefreshCw"></Icon>
            {t("刷新")}
          </button>
          {can("application:manage") ? (
            <button
              className="btn primary"
              onClick={() => {
                edit();
              }}
            >
              <Icon name="Plus"></Icon>
              {t("创建应用")}
            </button>
          ) : null}
        </div>
      </header>
      {viewState.error &&
      !viewState.editing &&
      !viewState.keyEditing &&
      !viewState.confirm ? (
        <div data-tone="crit" role="alert" className="banner">
          <Icon name="TriangleAlert" size={15}></Icon>
          <div className="banner-text">
            <p>{viewState.error}</p>
          </div>
        </div>
      ) : null}
      {viewState.loading && !viewState.items.length ? (
        <div className="panel empty">{t("正在加载应用…")}</div>
      ) : !viewState.items.length ? (
        <div className="panel empty">
          <Icon name="Box" size={28}></Icon>
          <h3>{t("从第一个应用开始")}</h3>
          <p>{t("创建应用后，可以为它签发访问密钥、发布安装包。")}</p>
          {can("application:manage") ? (
            <button
              className="btn primary"
              onClick={() => {
                edit();
              }}
            >
              <Icon name="Plus"></Icon>
              {t("创建应用")}
            </button>
          ) : null}
        </div>
      ) : (
        <div className="split">
          <section aria-label={t("应用列表")} className="panel">
            <header className="panel-head">
              <h2>
                {t("应用")}
                <span className="count">{viewState.items.length}</span>
              </h2>
            </header>
            {viewState.items.length > 5 ? (
              <label className="search list-search">
                <Icon name="Search" size={15}></Icon>
                <input
                  aria-label={t("搜索应用")}
                  placeholder={t("名称或应用标识")}
                  value={viewState.search}
                  onChange={(event) => {
                    store.setState({ search: event.currentTarget.value });
                  }}
                />
              </label>
            ) : null}
            <div className="list">
              <>
                {filtered.map((v) => (
                  <button
                    key={v.id}
                    aria-current={
                      viewState.selected === v.id ? "true" : undefined
                    }
                    className={classNames("list-item", {
                      active: viewState.selected === v.id,
                    })}
                    onClick={() => {
                      store.setState({ selected: v.id });
                    }}
                  >
                    <StatusGlyph
                      tone={v.enabled ? "ok" : "paused"}
                    ></StatusGlyph>
                    <span className="list-text">
                      <strong>{v.name}</strong>
                      <small className="mono">{v.slug}</small>
                    </span>
                    <span className="list-aside">
                      {t("{p0} 个密钥", { p0: v.activeKeys })}
                    </span>
                  </button>
                ))}
              </>
              {!filtered.length ? (
                <p className="empty compact">{t("没有匹配的应用")}</p>
              ) : null}
            </div>
          </section>
          {app ? (
            <section className="panel">
              <div className="app-detail-head">
                <div>
                  <h2>
                    {app.name}
                    <StatusBadge
                      status={app.enabled ? "enabled" : "disabled"}
                      label={app.enabled ? t("已启用") : t("已停用")}
                    ></StatusBadge>
                  </h2>
                  <span className="app-slug">{app.slug}</span>
                  <p>{app.description || t("尚未填写应用说明")}</p>
                </div>
                {can("application:manage") ? (
                  <div className="row-actions tight">
                    <button
                      className="btn sm"
                      onClick={() => {
                        edit(app);
                      }}
                    >
                      <Icon name="Pencil" size={14}></Icon>
                      {t("编辑")}
                    </button>
                    <button
                      aria-label={t("归档应用")}
                      title={t("归档应用")}
                      className="icon-btn"
                      onClick={() => {
                        store.setState({ confirm: "archive", error: "" });
                      }}
                    >
                      <Icon name="Archive"></Icon>
                    </button>
                  </div>
                ) : null}
              </div>
              <nav aria-label={t("应用内容")} className="tabs">
                <button
                  aria-current={viewState.tab === "keys" ? "page" : undefined}
                  className={classNames("tab", {
                    active: viewState.tab === "keys",
                  })}
                  onClick={() => {
                    store.setState({ tab: "keys" });
                  }}
                >
                  {t("访问密钥")}
                  <span className="count">{app.activeKeys}</span>
                </button>
                <button
                  aria-current={viewState.tab === "guide" ? "page" : undefined}
                  className={classNames("tab", {
                    active: viewState.tab === "guide",
                  })}
                  onClick={() => {
                    store.setState({ tab: "guide" });
                  }}
                >
                  {t("接入说明")}
                </button>
              </nav>
              {viewState.tab === "keys" ? (
                <>
                  <div className="toolbar">
                    <p className="note">
                      <Icon name="KeyRound" size={14}></Icon>
                      {t(
                        "完整密钥只在创建或轮换时显示一次，服务端只保存摘要。",
                      )}
                    </p>
                    <span className="spacer"></span>
                    {can("application:keys") ? (
                      <button
                        disabled={!app.enabled || viewState.keyLoading}
                        className="btn sm primary"
                        onClick={() => {
                          newKey();
                        }}
                      >
                        <Icon name="Plus" size={14}></Icon>
                        {t("创建密钥")}
                      </button>
                    ) : null}
                  </div>
                  {!can("application:keys") ? (
                    <p className="empty compact">
                      {t("查看和管理密钥需要“管理应用密钥”权限。")}
                    </p>
                  ) : viewState.keyLoading && !viewState.keys.length ? (
                    <p className="empty compact">{t("正在读取密钥…")}</p>
                  ) : !viewState.keys.length ? (
                    <div className="empty">
                      <Icon name="KeyRound" size={28}></Icon>
                      <h3>{t("还没有访问密钥")}</h3>
                      <p>{t("按用途创建密钥，只开放接入方需要的接口范围。")}</p>
                    </div>
                  ) : (
                    <div className="table-wrap">
                      <table className="table">
                        <thead>
                          <tr>
                            <th>{t("名称")}</th>
                            <th>{t("密钥")}</th>
                            <th>{t("接口范围")}</th>
                            <th>{t("状态")}</th>
                            <th>{t("到期")}</th>
                            <th>{t("最近使用")}</th>
                            <th className="actions">
                              <span className="sr-only">{t("操作")}</span>
                            </th>
                          </tr>
                        </thead>
                        <tbody>
                          <>
                            {viewState.keys.map((k) => (
                              <tr key={k.id}>
                                <td>
                                  <strong>{k.name}</strong>
                                </td>
                                <td>
                                  <span className="key-prefix">
                                    {k.prefix}
                                    {"••••"}
                                  </span>
                                </td>
                                <td>
                                  <div className="tags">
                                    <>
                                      {k.scopes.map((scope) => (
                                        <span key={scope} className="tag">
                                          {metadataLabel(
                                            viewState.scopes.find(
                                              (v) => v.key === scope,
                                            )?.name,
                                          ) || scope}
                                        </span>
                                      ))}
                                    </>
                                  </div>
                                </td>
                                <td>
                                  <StatusBadge
                                    status={keyStatus(k)}
                                    label={status(k)}
                                  ></StatusBadge>
                                </td>
                                <td className="tnum nowrap">
                                  {dateTime(k.expiresAt)}
                                </td>
                                <td className="tnum nowrap muted">
                                  {dateTime(k.lastUsedAt)}
                                </td>
                                <td className="actions">
                                  {can("application:keys") && !k.revokedAt ? (
                                    <div className="row-actions">
                                      <button
                                        disabled={!app.enabled}
                                        className="link"
                                        onClick={() => {
                                          newKey(k);
                                        }}
                                      >
                                        {t("轮换")}
                                      </button>
                                      <button
                                        className="link danger"
                                        onClick={() => {
                                          store.setState({
                                            revokeTarget: k,
                                            confirm: "revoke",
                                            error: "",
                                          });
                                        }}
                                      >
                                        {t("撤销")}
                                      </button>
                                    </div>
                                  ) : null}
                                </td>
                              </tr>
                            ))}
                          </>
                        </tbody>
                      </table>
                    </div>
                  )}
                  {viewState.keys.length ? (
                    <footer className="panel-foot">
                      <span>
                        {t(
                          "最多显示最近 100 条记录；每个应用最多 10 个有效密钥",
                        )}
                      </span>
                    </footer>
                  ) : null}
                </>
              ) : (
                <div className="guide">
                  <p className="note">
                    <Icon name="Info" size={14}></Icon>
                    {t(
                      "密钥用于程序接入。主机、检测和告警是整套实例共享的数据；安装包只限本应用。",
                    )}
                  </p>
                  <ol className="steps">
                    <li>
                      <div className="step-body">
                        <h3>{t("保存为环境变量")}</h3>
                        <p>
                          {t("把密钥保存为服务端环境变量")}
                          <code>{"MONITOR_GO_API_KEY"}</code>
                          {t("，不要写进浏览器代码或 Git 仓库。")}
                        </p>
                      </div>
                    </li>
                    <li>
                      <div className="step-body">
                        <h3>{t("验证连接")}</h3>
                        <div className="snippet">
                          <pre className="code-block">{curlExample}</pre>
                          <button
                            aria-label={t("复制示例命令")}
                            className="icon-btn sm"
                            onClick={() => {
                              copy(curlExample);
                            }}
                          >
                            <Icon name="Copy" size={14}></Icon>
                          </button>
                        </div>
                      </div>
                    </li>
                    <li>
                      <div className="step-body">
                        <h3>{t("调用已授权的接口")}</h3>
                        <div className="table-wrap panel">
                          <table className="table">
                            <thead>
                              <tr>
                                <th>
                                  {t("GET 路径（前缀 /api/v1/integrations）")}
                                </th>
                                <th>{t("需要的范围")}</th>
                              </tr>
                            </thead>
                            <tbody>
                              <>
                                {[
                                  ["/me", t("任意有效密钥")],
                                  ["/hosts", "host:read"],
                                  ["/monitors", "monitor:read"],
                                  ["/alerts", "alert:read"],
                                  ["/releases", "release:read"],
                                  [
                                    "/releases/{id}/download",
                                    "release:download",
                                  ],
                                ].map((v) => (
                                  <tr key={v[0]}>
                                    <td className="mono">{v[0]}</td>
                                    <td className="mono">{v[1]}</td>
                                  </tr>
                                ))}
                              </>
                            </tbody>
                          </table>
                        </div>
                        <p>
                          {t(
                            "每个密钥在每个来源 IP 下每 5 分钟最多 300 次请求。外链安装包的下载接口返回地址 JSON，请另起一个不带密钥请求头的请求去下载。",
                          )}
                        </p>
                      </div>
                    </li>
                  </ol>
                </div>
              )}
            </section>
          ) : null}
        </div>
      )}
      {viewState.editing ? (
        <Modal
          title={viewState.target ? t("编辑应用") : t("创建应用")}
          subtitle={t("一个应用对应一组访问密钥和安装包版本。")}
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
                form="application-form"
                disabled={viewState.busy}
                className="btn primary"
              >
                {viewState.busy ? t("正在保存…") : t("保存应用")}
              </button>
            </>
          }
        >
          <form
            id="application-form"
            className="form"
            onSubmit={(event) => {
              event.preventDefault();
              save();
            }}
          >
            <div className="form-grid">
              <label className="field">
                {t("应用名称")}
                <input
                  required
                  maxLength={60}
                  placeholder={t("例如：运维桌面端")}
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
                {t("应用标识")}
                <input
                  required
                  pattern="[a-z][a-z0-9-]{2,47}"
                  disabled={!!viewState.target}
                  placeholder="ops-desktop"
                  className="mono"
                  value={viewState.form.slug}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        slug: event.currentTarget.value,
                      },
                    }));
                  }}
                />
                <small>
                  {t("3–48 位小写字母、数字或短横线，创建后不能修改。")}
                </small>
              </label>
            </div>
            <label className="field">
              {t("说明")}
              <textarea
                rows={3}
                maxLength={300}
                placeholder={t("应用用途和维护团队")}
                value={viewState.form.description}
                onChange={(event) => {
                  store.setState((current) => ({
                    form: {
                      ...current.form,
                      description: event.currentTarget.value,
                    },
                  }));
                }}
              ></textarea>
            </label>
            <label className="check">
              <input
                type="checkbox"
                checked={viewState.form.enabled}
                onChange={(event) => {
                  store.setState((current) => ({
                    form: {
                      ...current.form,
                      enabled: event.currentTarget.checked,
                    },
                  }));
                }}
              />
              {t("启用应用")}
            </label>
            <p className="note">
              <Icon name="Info" size={14}></Icon>
              {t(
                "停用后，这个应用的所有密钥和安装包下载立即停止服务；重新启用后恢复。",
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
      {viewState.keyEditing ? (
        <Modal
          title={viewState.rotate ? t("轮换访问密钥") : t("创建访问密钥")}
          subtitle={app?.name}
          onClose={() => {
            !viewState.busy && store.setState({ keyEditing: false });
          }}
          footer={
            <>
              <button
                disabled={viewState.busy}
                className="btn"
                onClick={() => {
                  store.setState({ keyEditing: false });
                }}
              >
                {t("取消")}
              </button>
              <button
                form="key-form"
                disabled={viewState.busy || !viewState.keyForm.scopes.length}
                className="btn primary"
              >
                {viewState.busy
                  ? t("正在处理…")
                  : viewState.rotate
                    ? t("轮换密钥")
                    : t("生成密钥")}
              </button>
            </>
          }
        >
          <form
            id="key-form"
            className="form"
            onSubmit={(event) => {
              event.preventDefault();
              issue();
            }}
          >
            <div className="form-grid">
              <label className="field">
                {t("密钥名称")}
                <input
                  required
                  maxLength={40}
                  placeholder={t("例如：生产环境集成")}
                  value={viewState.keyForm.name}
                  onChange={(event) => {
                    store.setState((current) => ({
                      keyForm: {
                        ...current.keyForm,
                        name: event.currentTarget.value,
                      },
                    }));
                  }}
                />
              </label>
              <label className="field">
                {t("有效期（天）")}
                <input
                  type="number"
                  min="1"
                  max="365"
                  required
                  value={viewState.keyForm.days}
                  onChange={(event) => {
                    store.setState((current) => ({
                      keyForm: {
                        ...current.keyForm,
                        days: Number(event.currentTarget.value),
                      },
                    }));
                  }}
                />
              </label>
            </div>
            <fieldset className="field">
              <legend className="field-label">{t("接口范围")}</legend>
              <div className="form">
                <>
                  {viewState.scopes.map((s) => (
                    <label key={s.key} className="choice block">
                      <input
                        type="checkbox"
                        value={s.key}
                        checked={viewState.keyForm.scopes.includes(s.key)}
                        onChange={(event) => {
                          store.setState((current) => ({
                            keyForm: {
                              ...current.keyForm,
                              scopes: event.currentTarget.checked
                                ? [...viewState.keyForm.scopes, s.key]
                                : viewState.keyForm.scopes.filter(
                                    (selectedValue) => selectedValue !== s.key,
                                  ),
                            },
                          }));
                        }}
                      />
                      <span className="choice-text">
                        <strong>{metadataLabel(s.name)}</strong>
                        <small>{metadataLabel(s.description)}</small>
                      </span>
                    </label>
                  ))}
                </>
              </div>
            </fieldset>
            {viewState.rotate ? (
              <div data-tone="warn" className="banner">
                <Icon name="TriangleAlert" size={15}></Icon>
                <div className="banner-text">
                  <p>{t("轮换后旧密钥立即失效，请同步更新接入方的配置。")}</p>
                </div>
              </div>
            ) : null}
            {viewState.error ? (
              <p role="alert" className="form-error">
                {viewState.error}
              </p>
            ) : null}
          </form>
        </Modal>
      ) : null}
      {viewState.secret ? (
        <Modal
          title={t("保存新密钥")}
          subtitle={t("完整密钥只显示这一次，关闭后无法再查看。")}
          onClose={() => {
            store.setState({ secret: "" });
          }}
          footer={
            <>
              <button
                className="btn"
                onClick={() => {
                  copy(viewState.secret);
                }}
              >
                <Icon name="Copy"></Icon>
                {t("复制密钥")}
              </button>
              <button
                className="btn primary"
                onClick={() => {
                  store.setState({ secret: "" });
                }}
              >
                {t("我已保存")}
              </button>
            </>
          }
        >
          <code className="secret">{viewState.secret}</code>
          <p className="note">
            <Icon name="Info" size={14}></Icon>
            {t("如果遗失，请轮换或重新创建密钥。服务端只保存摘要。")}
          </p>
        </Modal>
      ) : null}
      {viewState.confirm ? (
        <Modal
          title={
            viewState.confirm === "archive" ? t("归档应用") : t("撤销密钥")
          }
          onClose={() => {
            !viewState.busy && store.setState({ confirm: "" });
          }}
          footer={
            <>
              <button
                disabled={viewState.busy}
                className="btn"
                onClick={() => {
                  store.setState({ confirm: "" });
                }}
              >
                {t("取消")}
              </button>
              <button
                disabled={viewState.busy}
                className="btn danger"
                onClick={() => {
                  execute();
                }}
              >
                {viewState.confirm === "archive"
                  ? t("归档应用")
                  : t("撤销密钥")}
              </button>
            </>
          }
        >
          <p>
            {viewState.confirm === "archive"
              ? t("归档“{p0}”会同时撤销它的全部密钥。请先归档它的安装包。", {
                  p0: app?.name,
                })
              : t("撤销“{p0}”后，使用它的服务将无法继续访问。", {
                  p0: viewState.revokeTarget?.name,
                })}
          </p>
          {viewState.error ? (
            <p role="alert" className="form-error">
              {viewState.error}
            </p>
          ) : null}
        </Modal>
      ) : null}
    </>
  );
}
