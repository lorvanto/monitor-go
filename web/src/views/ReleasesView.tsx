import { apiLink } from "../lib/apiLinks";
import {
  errorMessage,
  LocalizedError,
  message,
  notice,
  responseMessage,
  type DisplayMessage,
} from "../lib/messages";
import Form from "../components/Form";
import { resourceName } from "../lib/resourceLabels";
import { useEffect, useRef, useState } from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import Modal from "../components/Modal";
import StatusBadge from "../components/StatusBadge";
import { i18nStore, t, useI18n } from "../i18n";
import {
  dateTime,
  platforms,
  releaseStates,
  type Application,
  type Release,
} from "../lib/applicationTypes";
import { classNames } from "../lib/react";
import {
  api,
  appStore,
  bytes,
  can,
  navigate,
  notify,
  useApp,
} from "../stores/app";
interface FileEntry {
  nameKey?: string;
  id: string;
  parentId: string;
  name: string;
  kind: string;
  size: number;
  sha256: string;
}
export default function ReleasesView() {
  const blank = () => ({
    applicationId: "",
    name: "",
    versionName: "",
    platform: "linux",
    arch: "amd64",
    source: "file",
    fileId: "",
    downloadUrl: "",
    notes: "",
    state: "draft",
    version: 0,
  });
  const initialState = {
    items: [] as Release[],
    apps: [] as Application[],
    total: 0,
    page: 1,
    search: "",
    appFilter: "",
    state: "",
    loading: true,
    busy: false,
    error: "" as DisplayMessage,
    editing: false,
    target: undefined as Release | undefined,
    detail: undefined as Release | undefined,
    archive: undefined as Release | undefined,
    folders: [] as FileEntry[],
    files: [] as FileEntry[],
    folder: "2",
    filePage: 1,
    fileTotal: 0,
    fileSearch: "",
    selectedFile: undefined as FileEntry | undefined,
    fileLoading: false,
    uploading: false,
    progress: 0,
    uploadLimit: 32,
    form: blank(),
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const uploadInput = useRef<HTMLInputElement | null>(null);
  const xhr = useRef<XMLHttpRequest | undefined | undefined>(undefined);
  const loadSeq = useRef(0);
  const fileSeq = useRef(0);
  const editSeq = useRef(0);
  const appState = useApp(({ pageTitle, user }) => ({ pageTitle, user }));
  useI18n();
  const getLocked = () => !!store.getState().target?.everPublished;
  async function load() {
    const seq = ++loadSeq.current;
    store.setState({ loading: true });
    try {
      const q = new URLSearchParams({
        page: String(store.getState().page),
        q: store.getState().search,
        applicationId: store.getState().appFilter,
        state: store.getState().state,
      });
      const [v, a] = await Promise.all([
        api<{
          items: Release[];
          total: number;
        }>("/releases?" + q),
        api<Application[]>("/applications"),
      ]);
      if (seq !== loadSeq.current) return;
      store.setState({ items: v.items });
      store.setState({ total: v.total });
      store.setState({ apps: a });
      store.setState({ error: "" });
      if (store.getState().page > 1 && !v.items.length) {
        store.setState({ page: store.getState().page - 1 });
        await load();
      }
    } catch (e) {
      if (seq === loadSeq.current) store.setState({ error: errorMessage(e) });
    } finally {
      if (seq === loadSeq.current) store.setState({ loading: false });
    }
  }
  function filter() {
    store.setState({ page: 1 });
    load();
  }
  function folderLabel(f: FileEntry) {
    const parts = [resourceName(f)];
    let id = f.parentId;
    for (let i = 0; id && i < 17; i++) {
      const parent = store.getState().folders.find((v) => v.id === id);
      if (!parent) break;
      parts.unshift(resourceName(parent));
      id = parent.parentId;
    }
    return parts.join(" / ");
  }
  async function loadFiles() {
    const seq = ++fileSeq.current;
    if (!can("file:download")) return;
    store.setState({ fileLoading: true });
    try {
      const q = new URLSearchParams({
        parent: store.getState().folder,
        page: String(store.getState().filePage),
        q: store.getState().fileSearch,
      });
      const v = await api<{
        items: FileEntry[];
        total: number;
      }>("/files?" + q);
      if (seq !== fileSeq.current) return;
      store.setState({ files: v.items });
      store.setState({ fileTotal: v.total });
    } catch (e) {
      if (seq === fileSeq.current) store.setState({ error: errorMessage(e) });
    } finally {
      if (seq === fileSeq.current) store.setState({ fileLoading: false });
    }
  }
  function closeEditor() {
    if (store.getState().busy || store.getState().uploading) return;
    editSeq.current++;
    fileSeq.current++;
    store.setState({ editing: false, fileLoading: false });
  }
  async function edit(v?: Release) {
    const request = ++editSeq.current;
    fileSeq.current++;
    store.setState({ target: v });
    store.setState({
      form: {
        ...store.getState().form,
        ...blank(),
        ...(v
          ? {
              applicationId: v.applicationId,
              name: v.name,
              versionName: v.versionName,
              platform: v.platform,
              arch: v.arch,
              source: v.source,
              fileId: v.fileId,
              downloadUrl: v.downloadUrl,
              notes: v.notes,
              state: v.state,
              version: v.version,
            }
          : {
              applicationId:
                store.getState().appFilter ||
                store.getState().apps.find((a) => a.enabled)?.id ||
                "",
            }),
      },
    });
    store.setState({
      selectedFile: v?.fileId
        ? {
            id: v.fileId,
            parentId: "",
            name: v.fileName,
            kind: "file",
            size: v.size,
            sha256: v.sha256,
          }
        : undefined,
    });
    store.setState({ error: "" });
    store.setState({ editing: true });
    if (!getLocked() && can("file:download")) {
      try {
        const tree = await api<{
          items: FileEntry[];
          uploadLimitMB: number;
          defaultFolder: string;
        }>("/files/tree");
        if (request !== editSeq.current) return;
        store.setState({ folders: tree.items });
        store.setState({ folder: tree.defaultFolder });
        store.setState({ uploadLimit: tree.uploadLimitMB });
        store.setState({ fileSearch: "" });
        store.setState({ filePage: 1 });
        await loadFiles();
      } catch (e) {
        if (request === editSeq.current)
          store.setState({ error: errorMessage(e) });
      }
    }
  }
  function sourceChanged() {
    store.setState((current) => ({ form: { ...current.form, fileId: "" } }));
    store.setState((current) => ({
      form: { ...current.form, downloadUrl: "" },
    }));
    store.setState({ selectedFile: undefined });
  }
  function selectFile(f: FileEntry) {
    store.setState((current) => ({ form: { ...current.form, fileId: f.id } }));
    store.setState({ selectedFile: f });
  }
  function upload(event: React.ChangeEvent<HTMLInputElement>) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = "";
    if (!file || store.getState().uploading || store.getState().busy) return;
    if (!file.size || file.size > store.getState().uploadLimit * 1048576) {
      store.setState({
        error: notice("文件不能为空，且不能超过 {p0} MB", {
          p0: store.getState().uploadLimit,
        }),
      });
      return;
    }
    store.setState({ error: "" });
    store.setState({ uploading: true });
    store.setState({ progress: 0 });
    const request = new XMLHttpRequest();
    xhr.current = request;
    request.open(
      "POST",
      `/api/v1/files/uploads?parent=${encodeURIComponent(store.getState().folder)}`,
    );
    request.setRequestHeader("Accept-Language", i18nStore.getState().locale);
    request.setRequestHeader(
      "X-CSRF-Token",
      appStore.getState().user?.csrf || "",
    );
    request.timeout = 10 * 60 * 1000;
    request.upload.onprogress = (e) => {
      if (e.lengthComputable)
        store.setState({ progress: Math.round((e.loaded / e.total) * 100) });
    };
    request.onload = () => {
      try {
        const v = JSON.parse(request.responseText);
        if (request.status !== 200)
          throw new LocalizedError(responseMessage(v.error, "上传失败"));
        if (typeof v?.data?.id !== "string")
          throw new LocalizedError(message("上传响应异常"));
        selectFile(v.data);
        notify(notice("安装文件已上传"));
        loadFiles();
      } catch (e) {
        store.setState({
          error:
            e instanceof LocalizedError
              ? errorMessage(e)
              : notice("上传响应异常"),
        });
      } finally {
        store.setState({ uploading: false });
        xhr.current = undefined;
      }
    };
    request.onerror = request.ontimeout = () => {
      store.setState({
        error: notice("上传失败或超时，请检查网络并刷新文件列表"),
      });
      store.setState({ uploading: false });
      xhr.current = undefined;
    };
    request.onabort = () => {
      store.setState({ uploading: false });
    };
    const data = new FormData();
    data.append("file", file);
    request.send(data);
  }
  async function save() {
    if (store.getState().busy || store.getState().uploading) return;
    const target = store.getState().target;
    store.setState({ busy: true });
    try {
      await api(
        target ? `/releases/${target.id}` : "/releases",
        target ? "PATCH" : "POST",
        store.getState().form,
      );
      editSeq.current++;
      fileSeq.current++;
      store.setState({ editing: false, fileLoading: false });
      await load();
      notify(notice("安装包已保存"));
    } catch (e) {
      store.setState({ error: errorMessage(e) });
    } finally {
      store.setState({ busy: false });
    }
  }
  async function doArchive() {
    const archive = store.getState().archive;
    if (!archive) return;
    store.setState({ busy: true });
    try {
      await api(`/releases/${archive.id}`, "DELETE", {
        version: archive.version,
      });
      store.setState({ archive: undefined });
      await load();
      notify(notice("安装包已归档，原文件仍保留在文件管理中"));
    } catch (e) {
      store.setState({ error: errorMessage(e) });
    } finally {
      store.setState({ busy: false });
    }
  }
  const getPages = () => Math.max(1, Math.ceil(store.getState().total / 30));
  const downloadable = (v: Release) =>
    can("release:download") && v.state === "published" && v.applicationEnabled;
  useEffect(() => {
    load();
    return () => {
      loadSeq.current++;
      fileSeq.current++;
      editSeq.current++;
      xhr.current?.abort();
    };
  }, []);
  const locked = getLocked();
  const pages = getPages();
  return (
    <>
      <header className="page-head">
        <div className="page-title">
          <h1>{appState.pageTitle}</h1>
          <p>
            {t("发布后锁定文件与版本信息；本地文件下载附带 SHA-256 校验值。")}
          </p>
        </div>
        <div className="page-actions">
          <button
            disabled={viewState.loading}
            className="btn"
            onClick={() => {
              load();
            }}
          >
            <Icon name="RefreshCw"></Icon>
            {t("刷新")}
          </button>
          {can("release:manage") ? (
            <button
              disabled={!viewState.apps.length}
              className="btn primary"
              onClick={() => {
                edit();
              }}
            >
              <Icon name="Plus"></Icon>
              {t("新建版本")}
            </button>
          ) : null}
        </div>
      </header>
      {viewState.error && !viewState.editing && !viewState.archive ? (
        <div data-tone="crit" role="alert" className="banner">
          <Icon name="TriangleAlert" size={15}></Icon>
          <div className="banner-text">
            <p>{viewState.error}</p>
          </div>
        </div>
      ) : null}
      {!viewState.loading && !viewState.apps.length ? (
        <div className="panel empty">
          <Icon name="Package" size={28}></Icon>
          <h3>{t("先创建一个应用")}</h3>
          <p>{t("安装包归属于应用，每个应用可以有多个平台和架构的版本。")}</p>
          {can("application:access") ? (
            <button
              className="btn primary"
              onClick={() => {
                navigate("applications");
              }}
            >
              {t("前往应用与密钥")}
            </button>
          ) : null}
        </div>
      ) : (
        <section className="panel">
          <Form
            className="toolbar"
            onSubmit={(event) => {
              event.preventDefault();
              filter();
            }}
          >
            <label className="search">
              <Icon name="Search" size={15}></Icon>
              <input
                placeholder={t("名称或版本号")}
                aria-label={t("搜索安装包")}
                maxLength={80}
                value={viewState.search}
                onChange={(event) => {
                  store.setState({ search: event.currentTarget.value });
                }}
              />
            </label>
            <select
              aria-label={t("按应用筛选")}
              value={viewState.appFilter}
              onChange={(event) => {
                store.setState({ appFilter: event.currentTarget.value });
                filter();
              }}
            >
              <option value="">{t("全部应用")}</option>
              <>
                {viewState.apps.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </>
            </select>
            <select
              aria-label={t("按发布状态筛选")}
              value={viewState.state}
              onChange={(event) => {
                store.setState({ state: event.currentTarget.value });
                filter();
              }}
            >
              <option value="">{t("全部状态")}</option>
              <>
                {Object.entries(releaseStates).map(([key, name]) => (
                  <option key={key} value={key}>
                    {name}
                  </option>
                ))}
              </>
            </select>
            <button disabled={viewState.loading} className="btn">
              {t("搜索")}
            </button>
          </Form>
          {viewState.items.length ? (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>{t("安装包")}</th>
                    <th>{t("版本")}</th>
                    <th>{t("平台")}</th>
                    <th>{t("状态")}</th>
                    <th className="num">{t("大小")}</th>
                    <th className="num">{t("下载次数")}</th>
                    <th>{t("更新时间")}</th>
                    <th className="actions">
                      <span className="sr-only">{t("操作")}</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <>
                    {viewState.items.map((v) => (
                      <tr key={v.id}>
                        <td>
                          <button
                            className="release-name"
                            onClick={() => {
                              store.setState({ detail: v });
                            }}
                          >
                            <strong>{v.name}</strong>
                            <small>{v.applicationName}</small>
                          </button>
                        </td>
                        <td>
                          <span className="version">{v.versionName}</span>
                        </td>
                        <td className="nowrap">
                          {platforms[v.platform]} {v.arch}
                        </td>
                        <td>
                          <span className="cell-main">
                            <StatusBadge
                              status={v.state}
                              label={releaseStates[v.state]}
                            ></StatusBadge>
                            {!v.applicationEnabled ? (
                              <small>{t("应用已停用")}</small>
                            ) : null}
                          </span>
                        </td>
                        <td className="num">
                          {v.source === "file" ? bytes(v.size) : t("外部链接")}
                        </td>
                        <td className="num">{v.downloadCount}</td>
                        <td className="tnum nowrap muted">
                          {dateTime(v.updatedAt)}
                        </td>
                        <td className="actions">
                          <div className="row-actions tight">
                            {downloadable(v) ? (
                              <a
                                href={apiLink(
                                  `/api/v1/releases/${v.id}/download`,
                                )}
                                target="_blank"
                                rel="noopener noreferrer"
                                aria-label={t("下载 {p0} {p1}", {
                                  p0: v.name,
                                  p1: v.versionName,
                                })}
                                title={t("下载")}
                                className="icon-btn"
                              >
                                <Icon name="Download"></Icon>
                              </a>
                            ) : null}
                            {can("release:manage") ? (
                              <button
                                aria-label={t("编辑 {p0} {p1}", {
                                  p0: v.name,
                                  p1: v.versionName,
                                })}
                                title={t("编辑")}
                                className="icon-btn"
                                onClick={() => {
                                  edit(v);
                                }}
                              >
                                <Icon name="Pencil"></Icon>
                              </button>
                            ) : null}
                            {can("release:manage") ? (
                              <button
                                aria-label={t("归档 {p0} {p1}", {
                                  p0: v.name,
                                  p1: v.versionName,
                                })}
                                title={t("归档")}
                                className="icon-btn"
                                onClick={() => {
                                  store.setState({ archive: v, error: "" });
                                }}
                              >
                                <Icon name="Archive"></Icon>
                              </button>
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
              <Icon name="Package" size={28}></Icon>
              <h3>
                {viewState.loading ? t("正在读取版本…") : t("没有安装包")}
              </h3>
              {!viewState.loading ? (
                <p>{t("新建一个版本，或者调整筛选条件。")}</p>
              ) : null}
            </div>
          )}
          <footer className="panel-foot">
            <span>
              {t("共 {p0} 个版本，第 {p1} / {p2} 页", {
                p0: viewState.total,
                p1: viewState.page,
                p2: pages,
              })}
            </span>
            <div className="row-actions tight">
              <button
                disabled={viewState.page <= 1 || viewState.loading}
                className="btn sm"
                onClick={() => {
                  store.setState({ page: viewState.page - 1 });
                  load();
                }}
              >
                {t("上一页")}
              </button>
              <button
                disabled={viewState.page >= pages || viewState.loading}
                className="btn sm"
                onClick={() => {
                  store.setState({ page: viewState.page + 1 });
                  load();
                }}
              >
                {t("下一页")}
              </button>
            </div>
          </footer>
        </section>
      )}
      <p className="note page-note">
        <Icon name="Info" size={14}></Icon>
        {t(
          "下载次数统计成功的文件请求（含分段下载）和外链跳转，不代表安装量；通过密钥获取外链地址不计入次数。",
        )}
      </p>
      {viewState.editing ? (
        <Modal
          title={viewState.target ? t("编辑版本") : t("新建版本")}
          wide
          onClose={() => {
            closeEditor();
          }}
          footer={
            <>
              <button
                disabled={viewState.busy || viewState.uploading}
                className="btn"
                onClick={() => {
                  closeEditor();
                }}
              >
                {t("取消")}
              </button>
              <button
                form="release-form"
                disabled={
                  viewState.busy ||
                  viewState.uploading ||
                  (viewState.form.source === "file" && !viewState.form.fileId)
                }
                className="btn primary"
              >
                {viewState.busy
                  ? t("正在保存…")
                  : viewState.form.state === "published"
                    ? t("保存并发布")
                    : t("保存版本")}
              </button>
            </>
          }
        >
          <Form
            id="release-form"
            className="form"
            onSubmit={(event) => {
              event.preventDefault();
              save();
            }}
          >
            <div className="form-grid three">
              <label className="field">
                {t("所属应用")}
                <select
                  required
                  disabled={locked}
                  value={viewState.form.applicationId}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        applicationId: event.currentTarget.value,
                      },
                    }));
                  }}
                >
                  <option value="" disabled>
                    {t("选择应用")}
                  </option>
                  <>
                    {viewState.apps.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.name}
                        {a.enabled ? "" : t("（已停用）")}
                      </option>
                    ))}
                  </>
                </select>
              </label>
              <label className="field">
                {t("安装包名称")}
                <input
                  required
                  maxLength={70}
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
                {t("版本号")}
                <input
                  disabled={locked}
                  required
                  maxLength={64}
                  pattern={"[0-9A-Za-z][0-9A-Za-z._+\\-]{0,63}"}
                  placeholder="1.0.0"
                  className="mono"
                  value={viewState.form.versionName}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        versionName: event.currentTarget.value,
                      },
                    }));
                  }}
                />
              </label>
              <label className="field">
                {t("平台")}
                <select
                  disabled={locked}
                  value={viewState.form.platform}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        platform: event.currentTarget.value,
                      },
                    }));
                  }}
                >
                  <>
                    {Object.entries(platforms).map(([key, name]) => (
                      <option key={key} value={key}>
                        {name}
                      </option>
                    ))}
                  </>
                </select>
              </label>
              <label className="field">
                {t("架构")}
                <select
                  disabled={locked}
                  value={viewState.form.arch}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        arch: event.currentTarget.value,
                      },
                    }));
                  }}
                >
                  <option>{"amd64"}</option>
                  <option>{"arm64"}</option>
                  <option value="universal">{t("通用")}</option>
                  <option value="other">{t("其他")}</option>
                </select>
              </label>
              <label className="field">
                {t("发布状态")}
                <select
                  value={viewState.form.state}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        state: event.currentTarget.value,
                      },
                    }));
                  }}
                >
                  {!locked ? <option value="draft">{t("草稿")}</option> : null}
                  <option value="published">{t("已发布，允许下载")}</option>
                  <option value="withdrawn">{t("已下架，停止下载")}</option>
                </select>
              </label>
            </div>
            <fieldset className="field">
              <legend className="field-label">{t("文件来源")}</legend>
              <div className="choice-group">
                <label className="choice">
                  <input
                    type="radio"
                    value="file"
                    disabled={locked || viewState.uploading}
                    checked={viewState.form.source === "file"}
                    onChange={() => {
                      store.setState((current) => ({
                        form: { ...current.form, source: "file" },
                      }));
                      sourceChanged();
                    }}
                  />
                  {t("本地文件")}
                </label>
                <label className="choice">
                  <input
                    type="radio"
                    value="url"
                    disabled={locked || viewState.uploading}
                    checked={viewState.form.source === "url"}
                    onChange={() => {
                      store.setState((current) => ({
                        form: { ...current.form, source: "url" },
                      }));
                      sourceChanged();
                    }}
                  />
                  {t("外部 HTTPS 地址")}
                </label>
              </div>
            </fieldset>
            {viewState.form.source === "url" ? (
              <label className="field">
                {t("下载地址")}
                <input
                  type="url"
                  pattern="https://.*"
                  disabled={locked}
                  required
                  maxLength={2048}
                  placeholder="https://downloads.example.com/app.zip"
                  className="mono"
                  value={viewState.form.downloadUrl}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        downloadUrl: event.currentTarget.value,
                      },
                    }));
                  }}
                />
                <small>
                  {t(
                    "下载时跳转到这个地址；外部文件不占用实例存储，也不计算 SHA-256。",
                  )}
                </small>
              </label>
            ) : (
              <>
                {viewState.selectedFile ? (
                  <div className="artifact">
                    <Icon name="FileText"></Icon>
                    <div>
                      <strong>{viewState.selectedFile.name}</strong>
                      <small>
                        {bytes(viewState.selectedFile.size)}
                        {"　SHA-256 "}
                        {viewState.selectedFile.sha256}
                      </small>
                    </div>
                    <Icon name="CheckCircle2"></Icon>
                  </div>
                ) : null}
                {!locked && can("file:download") ? (
                  <div className="picker">
                    <div className="picker-bar">
                      <select
                        aria-label={t("选择目录")}
                        disabled={viewState.uploading}
                        value={viewState.folder}
                        onChange={(event) => {
                          store.setState({ folder: event.currentTarget.value });
                          store.setState({ filePage: 1 });
                          loadFiles();
                        }}
                      >
                        <>
                          {viewState.folders.map((f) => (
                            <option key={f.id} value={f.id}>
                              {folderLabel(f)}
                            </option>
                          ))}
                        </>
                      </select>
                      <input
                        aria-label={t("搜索当前目录文件")}
                        placeholder={t("搜索当前目录，按回车")}
                        value={viewState.fileSearch}
                        onKeyDown={(event) => {
                          if (event.key !== "Enter") return;
                          event.preventDefault();
                          store.setState({ filePage: 1 });
                          loadFiles();
                        }}
                        onChange={(event) => {
                          store.setState({
                            fileSearch: event.currentTarget.value,
                          });
                        }}
                      />
                      {can("file:upload") ? (
                        <button
                          type="button"
                          disabled={viewState.uploading}
                          className="btn sm"
                          onClick={() => {
                            uploadInput.current?.click();
                          }}
                        >
                          <Icon name="Upload" size={14}></Icon>
                          {viewState.uploading
                            ? t("上传中 {p0}%", { p0: viewState.progress })
                            : t("上传安装文件")}
                        </button>
                      ) : null}
                      <input
                        ref={(element) => {
                          uploadInput.current = element;
                        }}
                        type="file"
                        hidden
                        onChange={(event) => {
                          upload(event);
                        }}
                      />
                    </div>
                    <div className="picker-options">
                      <>
                        {viewState.files
                          .filter((v) => v.kind === "file")
                          .map((f) => (
                            <button
                              key={f.id}
                              type="button"
                              aria-pressed={viewState.form.fileId === f.id}
                              disabled={viewState.uploading}
                              className={classNames({
                                selected: viewState.form.fileId === f.id,
                              })}
                              onClick={() => {
                                selectFile(f);
                              }}
                            >
                              <Icon name="FileText" size={15}></Icon>
                              <span>{resourceName(f)}</span>
                              <small>{bytes(f.size)}</small>
                              {viewState.form.fileId === f.id ? (
                                <Icon name="Check" size={15}></Icon>
                              ) : (
                                <span></span>
                              )}
                            </button>
                          ))}
                      </>
                      {!viewState.files.some((v) => v.kind === "file") ? (
                        <p className="muted small">
                          {viewState.fileLoading
                            ? t("正在加载文件…")
                            : t("这一页没有文件，可以切换目录或上传。")}
                        </p>
                      ) : null}
                    </div>
                    <div className="picker-foot">
                      <span>
                        {t("单文件上限 {p0} MB，第 {p1} 页", {
                          p0: viewState.uploadLimit,
                          p1: viewState.filePage,
                        })}
                      </span>
                      <button
                        type="button"
                        disabled={
                          viewState.filePage <= 1 || viewState.fileLoading
                        }
                        className="link"
                        onClick={() => {
                          store.setState({ filePage: viewState.filePage - 1 });
                          loadFiles();
                        }}
                      >
                        {t("上一页")}
                      </button>
                      <button
                        type="button"
                        disabled={
                          viewState.filePage * 50 >= viewState.fileTotal ||
                          viewState.fileLoading
                        }
                        className="link"
                        onClick={() => {
                          store.setState({ filePage: viewState.filePage + 1 });
                          loadFiles();
                        }}
                      >
                        {t("下一页")}
                      </button>
                    </div>
                  </div>
                ) : !locked ? (
                  <p className="note">
                    <Icon name="Info" size={14}></Icon>
                    {t(
                      "关联本地文件需要“下载原文件”权限；也可以改用外部下载地址。",
                    )}
                  </p>
                ) : null}
              </>
            )}
            {locked ? (
              <p className="note">
                <Icon name="LockKeyhole" size={14}></Icon>
                {t(
                  "这个版本已经发布过，文件与版本信息已锁定。更新安装文件请新建版本；仍可修改说明或下架。",
                )}
              </p>
            ) : null}
            <label className="field">
              {t("版本说明")}
              <textarea
                rows={4}
                maxLength={3500}
                placeholder={t("本次更新的功能、修复和注意事项")}
                value={viewState.form.notes}
                onChange={(event) => {
                  store.setState((current) => ({
                    form: { ...current.form, notes: event.currentTarget.value },
                  }));
                }}
              ></textarea>
            </label>
            {viewState.error ? (
              <p role="alert" className="form-error">
                {viewState.error}
              </p>
            ) : null}
          </Form>
        </Modal>
      ) : null}
      {viewState.detail ? (
        <Modal
          drawer
          title={`${viewState.detail.name} ${viewState.detail.versionName}`}
          subtitle={viewState.detail.applicationName}
          onClose={() => {
            store.setState({ detail: undefined });
          }}
          footer={
            <>
              <button
                className="btn"
                onClick={() => {
                  store.setState({ detail: undefined });
                }}
              >
                {t("关闭")}
              </button>
              {downloadable(viewState.detail) ? (
                <a
                  href={apiLink(
                    `/api/v1/releases/${viewState.detail.id}/download`,
                  )}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="btn primary"
                >
                  <Icon name="Download"></Icon>
                  {t("下载安装包")}
                </a>
              ) : null}
            </>
          }
        >
          <dl className="kv">
            <div>
              <dt>{t("发布状态")}</dt>
              <dd>
                <StatusBadge
                  status={viewState.detail.state}
                  label={releaseStates[viewState.detail.state]}
                ></StatusBadge>
              </dd>
            </div>
            <div>
              <dt>{t("平台与架构")}</dt>
              <dd>
                {platforms[viewState.detail.platform]} {viewState.detail.arch}
              </dd>
            </div>
            <div>
              <dt>{t("首次发布")}</dt>
              <dd className="num">{dateTime(viewState.detail.publishedAt)}</dd>
            </div>
            <div>
              <dt>{t("更新时间")}</dt>
              <dd className="num">{dateTime(viewState.detail.updatedAt)}</dd>
            </div>
            <div>
              <dt>{t("下载次数")}</dt>
              <dd className="num">{viewState.detail.downloadCount}</dd>
            </div>
          </dl>
          <div className="artifact">
            <Icon
              name={
                viewState.detail.source === "file" ? "FileText" : "ExternalLink"
              }
            ></Icon>
            <div>
              <strong>
                {viewState.detail.source === "file"
                  ? viewState.detail.fileName
                  : t("外部安装文件")}
              </strong>
              <small>
                {viewState.detail.source === "file"
                  ? bytes(viewState.detail.size)
                  : viewState.detail.downloadUrl ||
                    t("需要“下载安装包”权限才能查看地址")}
              </small>
            </div>
            <span></span>
          </div>
          {viewState.detail.sha256 ? (
            <div>
              <h3 className="section-title">{t("SHA-256 校验值")}</h3>
              <code className="checksum">{viewState.detail.sha256}</code>
            </div>
          ) : null}
          <div>
            <h3 className="section-title">{t("版本说明")}</h3>
            <p className="release-notes">
              {viewState.detail.notes || t("暂无版本说明")}
            </p>
          </div>
        </Modal>
      ) : null}
      {viewState.archive ? (
        <Modal
          title={t("归档版本")}
          onClose={() => {
            !viewState.busy && store.setState({ archive: undefined });
          }}
          footer={
            <>
              <button
                disabled={viewState.busy}
                className="btn"
                onClick={() => {
                  store.setState({ archive: undefined });
                }}
              >
                {t("取消")}
              </button>
              <button
                disabled={viewState.busy}
                className="btn danger"
                onClick={() => {
                  doArchive();
                }}
              >
                {t("归档版本")}
              </button>
            </>
          }
        >
          <p>
            {t(
              "归档 {p0} {p1} 后将停止下载并从列表中移除。原文件仍保留在文件管理中；同一应用、版本号、平台和架构的组合不能再次使用。",
              { p0: viewState.archive.name, p1: viewState.archive.versionName },
            )}
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
