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
import {
  Fragment,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import Modal from "../components/Modal";
import { i18nStore, t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import {
  api,
  appStore,
  bytes,
  can,
  dateTime,
  notify,
  useApp,
} from "../stores/app";
interface Entry {
  id: string;
  parentId: string;
  kind: "file" | "folder";
  name: string;
  nameKey?: string;
  mime: string;
  size: number;
  sha256: string;
  width: number;
  height: number;
  createdAt: number;
  updatedAt: number;
  version: number;
  deletedAt: number;
  trashRoot: string;
  creator: string;
  creatorKey?: string;
}
interface Style {
  id: string;
  name: string;
  nameKey?: string;
  width: number;
  height: number;
  fit: string;
  version: number;
}
interface Tree {
  items: Entry[];
  uploadLimitMB: number;
  storageLimitMB: number;
  defaultFolder: string;
  storageUsed: number;
  trashCount: number;
  fileCount: number;
}
export default function FilesView() {
  const initialState = {
    tree: {
      items: [],
      uploadLimitMB: 32,
      storageLimitMB: 1024,
      defaultFolder: "2",
      storageUsed: 0,
      trashCount: 0,
      fileCount: 0,
    } as Tree,
    entries: [] as Entry[],
    styles: [] as Style[],
    parent: "1",
    section: "files" as "files" | "trash" | "styles",
    query: "",
    page: 1,
    total: 0,
    layout: "grid" as "grid" | "list",
    loading: true,
    error: "" as DisplayMessage,
    busy: false,
    dragging: false,
    uploading: false,
    progress: 0,
    uploadName: "",
    uploadFailures: [] as DisplayMessage[],
    mode: "" as
      | ""
      | "folder"
      | "edit"
      | "trash"
      | "restore"
      | "purge"
      | "style"
      | "archive-style",
    target: undefined as Entry | undefined,
    styleTarget: undefined as Style | undefined,
    modalError: "" as DisplayMessage,
    nameEdited: false,
    styleNameEdited: false,
    form: { name: "", parentId: "1", confirm: "" },
    styleForm: {
      name: "",
      width: 320,
      height: 240,
      fit: "cover",
      version: 0,
    },
    preview: undefined as
      | {
          file: Entry;
          kind: string;
          text?: string;
          truncated?: boolean;
        }
      | undefined,
    previewStyle: "",
    imageError: false,
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const input = useRef<HTMLInputElement | null>(null);
  const seq = useRef(0);
  const previewSeq = useRef(0);
  const reloadSeq = useRef(0);
  const searchTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );
  const activeUpload = useRef<XMLHttpRequest | undefined | undefined>(
    undefined,
  );
  const disposed = useRef(false);
  const appState = useApp();
  useI18n();
  const getActionTitle = () =>
    ({
      "": "",
      folder: t("新建文件夹"),
      edit: t("重命名或移动"),
      trash: t("移入回收站"),
      restore: t("恢复"),
      purge: t("永久删除"),
      style: store.getState().styleTarget
        ? t("编辑图片样式")
        : t("新增图片样式"),
      "archive-style": t("归档图片样式"),
    })[store.getState().mode];
  const getActionLabel = () =>
    store.getState().busy
      ? t("正在处理…")
      : {
          "": "",
          folder: t("创建文件夹"),
          edit: t("保存"),
          trash: t("移入回收站"),
          restore: t("恢复"),
          purge: t("永久删除"),
          style: t("保存样式"),
          "archive-style": t("归档样式"),
        }[store.getState().mode];
  const getFolderRows = () => {
    const out: (Entry & {
      depth: number;
    })[] = [];
    function walk(id: string, depth: number) {
      for (const f of store
        .getState()
        .tree.items.filter((v) => v.parentId === id)) {
        out.push({ ...f, depth });
        walk(f.id, depth + 1);
      }
    }
    walk("", 0);
    return out;
  };
  const getCrumbs = () => {
    const out: Entry[] = [];
    let current = store.getState().parent;
    for (let i = 0; current && i < 18; i++) {
      const f = store.getState().tree.items.find((v) => v.id === current);
      if (!f) break;
      out.unshift(f);
      current = f.parentId;
    }
    return out;
  };
  const getImageURL = () => {
    const { preview, previewStyle } = store.getState();
    return preview
      ? apiLink(
          `/api/v1/files/${preview.file.id}/image${previewStyle ? "?style=" + previewStyle : ""}`,
        )
      : "";
  };

  async function load() {
    const request = ++seq.current;
    if (store.getState().section === "styles") {
      store.setState({ loading: false });
      return;
    }
    store.setState({ loading: true });
    try {
      const v = await api<{
        items: Entry[];
        total: number;
      }>(
        `/files?parent=${store.getState().parent}&trash=${store.getState().section === "trash" ? "1" : "0"}&q=${encodeURIComponent(store.getState().query)}&page=${store.getState().page}`,
      );
      if (request !== seq.current) return;
      store.setState({ entries: v.items });
      store.setState({ total: v.total });
      store.setState({ error: "" });
      const lastPage = Math.max(1, Math.ceil(v.total / 50));
      if (store.getState().page > lastPage) store.setState({ page: lastPage });
    } catch (e) {
      if (request === seq.current) store.setState({ error: errorMessage(e) });
    } finally {
      if (request === seq.current) store.setState({ loading: false });
    }
  }
  async function reload() {
    const request = ++reloadSeq.current;
    try {
      const [folders, presets] = await Promise.all([
        api<Tree>("/files/tree"),
        api<Style[]>("/image-styles"),
      ]);
      if (request !== reloadSeq.current || disposed.current) return;
      store.setState({ tree: folders });
      store.setState({ styles: presets });
      if (!folders.items.some((f) => f.id === store.getState().parent))
        store.setState({ parent: "1" });
      await load();
    } catch (e) {
      if (request !== reloadSeq.current || disposed.current) return;
      store.setState({ error: errorMessage(e) });
      store.setState({ loading: false });
    }
  }
  function openFolder(id: string) {
    store.setState({ section: "files" });
    store.setState({ parent: id });
    store.setState({ query: "" });
    store.setState({ page: 1 });
  }
  const getUsedPercent = () =>
    Math.min(
      100,
      (store.getState().tree.storageUsed /
        (store.getState().tree.storageLimitMB * 1048576)) *
        100,
    );
  const getPages = () => Math.max(1, Math.ceil(store.getState().total / 50));
  const ext = (f: Entry) =>
    f.kind === "folder"
      ? t("文件夹")
      : f.name.includes(".")
        ? f.name.split(".").pop()!.toUpperCase().slice(0, 8)
        : t("文件");
  const day = (at: number) =>
    at ? new Date(at).toLocaleDateString(i18nStore.getState().locale) : "—";
  function imageFile(f: Entry) {
    return (
      f.kind === "file" &&
      ["image/png", "image/jpeg", "image/gif", "image/webp"].includes(f.mime) &&
      f.width > 0 &&
      f.height > 0 &&
      f.width * f.height <= 24000000 &&
      f.width <= 16384 &&
      f.height <= 16384
    );
  }
  function openAction(action: LocalState["mode"], item?: Entry) {
    store.setState({ mode: action, nameEdited: false });
    store.setState({ target: item });
    store.setState((current) => ({
      form: { ...current.form, name: resourceName(item) },
    }));
    store.setState((current) => ({
      form: {
        ...current.form,
        parentId:
          action === "restore"
            ? store.getState().tree.items.some((f) => f.id === item?.parentId)
              ? item!.parentId
              : store.getState().tree.defaultFolder
            : item?.parentId || store.getState().parent,
      },
    }));
    store.setState((current) => ({ form: { ...current.form, confirm: "" } }));
    store.setState({ modalError: "" });
  }
  function openStyle(item?: Style) {
    store.setState({ styleTarget: item, styleNameEdited: false });
    store.setState({
      styleForm: {
        ...store.getState().styleForm,
        ...(item
          ? {
              name: resourceName(item),
              width: item.width,
              height: item.height,
              fit: item.fit,
              version: item.version,
            }
          : { name: "", width: 320, height: 240, fit: "cover", version: 0 }),
      },
    });
    store.setState({ modalError: "" });
    store.setState({ mode: "style" });
  }
  async function save() {
    const styleTarget = store.getState().styleTarget;
    if (
      store.getState().mode === "purge" &&
      store.getState().form.confirm !== t("永久删除")
    )
      return;
    store.setState({ busy: true });
    store.setState({ modalError: "" });
    try {
      const f = store.getState().target;
      if (store.getState().mode === "folder")
        await api("/files/folders", "POST", {
          name:
            !store.getState().nameEdited && store.getState().target?.nameKey
              ? resourceName(store.getState().target)
              : store.getState().form.name,
          parentId: store.getState().form.parentId,
        });
      if (store.getState().mode === "edit" && f)
        await api(`/files/${f.id}`, "PATCH", {
          name:
            !store.getState().nameEdited && store.getState().target?.nameKey
              ? resourceName(store.getState().target)
              : store.getState().form.name,
          parentId: store.getState().form.parentId,
          version: f.version,
        });
      if (store.getState().mode === "trash" && f)
        await api(`/files/${f.id}`, "DELETE", { version: f.version });
      if (store.getState().mode === "restore" && f)
        await api(`/files/${f.id}/restore`, "POST", {
          name:
            !store.getState().nameEdited && store.getState().target?.nameKey
              ? resourceName(store.getState().target)
              : store.getState().form.name,
          parentId: store.getState().form.parentId,
          version: f.version,
        });
      if (store.getState().mode === "purge" && f)
        await api(`/files/${f.id}/purge`, "POST", {
          version: f.version,
          confirm: "DELETE",
        });
      if (store.getState().mode === "style")
        await api(
          styleTarget ? `/image-styles/${styleTarget.id}` : "/image-styles",
          styleTarget ? "PATCH" : "POST",
          {
            ...store.getState().styleForm,
            name:
              !store.getState().styleNameEdited && styleTarget?.nameKey
                ? resourceName(styleTarget)
                : store.getState().styleForm.name,
          },
        );
      if (store.getState().mode === "archive-style" && styleTarget)
        await api(`/image-styles/${styleTarget.id}`, "DELETE", {
          version: styleTarget.version,
        });
      store.setState({ mode: "" });
      await reload();
      notify(notice("操作已保存"));
    } catch (e) {
      store.setState({ modalError: errorMessage(e) });
    } finally {
      store.setState({ busy: false });
    }
  }
  function closePreview() {
    previewSeq.current++;
    store.setState({ preview: undefined });
  }
  async function show(f: Entry) {
    const request = ++previewSeq.current;
    if (f.kind === "folder") {
      openFolder(f.id);
      return;
    }
    if (!can("file:read")) return;
    try {
      const preview = await api<LocalState["preview"]>(
        `/files/${f.id}/preview`,
      );
      if (request !== previewSeq.current || disposed.current) return;
      store.setState({ preview });
      store.setState({ previewStyle: "" });
      store.setState({ imageError: false });
    } catch (e) {
      if (request === previewSeq.current && !disposed.current)
        notify(errorMessage(e), true);
    }
  }
  function uploadOne(file: File, folder: string) {
    return new Promise<void>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      activeUpload.current = xhr;
      xhr.open("POST", `/api/v1/files/uploads?parent=${folder}`);
      xhr.setRequestHeader("Accept-Language", i18nStore.getState().locale);
      xhr.setRequestHeader(
        "X-CSRF-Token",
        appStore.getState().user?.csrf || "",
      );
      xhr.timeout = 600000;
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable)
          store.setState({ progress: Math.round((e.loaded / e.total) * 100) });
      };
      xhr.onload = () => {
        try {
          const v = JSON.parse(xhr.responseText);
          if (xhr.status >= 200 && xhr.status < 300) resolve();
          else reject(new LocalizedError(responseMessage(v.error, "上传失败")));
        } catch {
          reject(new LocalizedError(message("上传响应异常")));
        }
      };
      xhr.onerror = () => reject(new LocalizedError(message("连接中断")));
      xhr.ontimeout = () => reject(new LocalizedError(message("上传超时")));
      xhr.onabort = () => reject(new LocalizedError(message("上传已取消")));
      const body = new FormData();
      body.append("file", file);
      xhr.send(body);
    });
  }
  async function upload(files: FileList | File[] | null) {
    if (
      !files ||
      !can("file:upload") ||
      store.getState().uploading ||
      store.getState().section !== "files"
    )
      return;
    const selected = Array.from(files);
    if (selected.length > 50) {
      notify(notice("每批最多上传 50 个文件"), true);
      return;
    }
    store.setState({ uploading: true });
    store.setState({ uploadFailures: [] });
    let count = 0;
    const folder = store.getState().parent;
    for (const file of selected) {
      if (disposed.current) break;
      store.setState({ uploadName: file.name });
      store.setState({ progress: 0 });
      if (
        !file.size ||
        file.size > store.getState().tree.uploadLimitMB * 1048576
      ) {
        store.setState({
          uploadFailures: [
            ...store.getState().uploadFailures,
            notice("{p0}：文件为空或超过 {p1} MB", {
              p0: file.name,
              p1: store.getState().tree.uploadLimitMB,
            }),
          ],
        });
        continue;
      }
      try {
        await uploadOne(file, folder);
        count++;
      } catch (e) {
        store.setState({
          uploadFailures: [
            ...store.getState().uploadFailures,
            <>
              {file.name}: {errorMessage(e)}
            </>,
          ],
        });
      }
    }
    store.setState({ uploading: false });
    activeUpload.current = undefined;
    if (input.current) input.current.value = "";
    if (!disposed.current) {
      await reload();
      if (count) notify(notice("已上传 {p0} 个文件", { p0: count }));
    }
  }
  function dropped(e: React.DragEvent<HTMLDivElement>) {
    store.setState({ dragging: false });
    upload(e.dataTransfer?.files || null);
  }
  useEffect(() => {
    closePreview();
    load();
  }, [viewState.parent, viewState.section, viewState.page]);
  useEffect(() => {
    clearTimeout(searchTimer.current);
    searchTimer.current = setTimeout(() => {
      store.setState({ page: 1 });
      load();
    }, 250);
    return () => clearTimeout(searchTimer.current);
  }, [viewState.query]);
  useEffect(() => {
    store.setState({ imageError: false });
  }, [viewState.previewStyle]);
  useEffect(() => {
    disposed.current = false;
    reload();
    return () => {
      disposed.current = true;
      seq.current++;
      previewSeq.current++;
      reloadSeq.current++;
      activeUpload.current?.abort();
      clearTimeout(searchTimer.current);
    };
  }, [store]);
  const actionTitle = getActionTitle();
  const actionLabel = getActionLabel();
  const folderRows = getFolderRows();
  const crumbs = getCrumbs();
  const imageURL = getImageURL();
  const usedPercent = getUsedPercent();
  const pages = getPages();
  return (
    <>
      <header className="page-head">
        <div className="page-title">
          <h1>{appState.pageTitle}</h1>
          <p>
            {t("{p0} 个文件，团队共享并按角色授权，文件只保存在这套实例中。", {
              p0: viewState.tree.fileCount,
            })}
          </p>
        </div>
        <div className="page-actions">
          {can("file:manage") && viewState.section === "files" ? (
            <button
              className="btn"
              onClick={() => {
                openAction("folder");
              }}
            >
              <Icon name="FolderPlus"></Icon>
              {t("新建文件夹")}
            </button>
          ) : null}
          {can("file:upload") && viewState.section === "files" ? (
            <button
              disabled={viewState.uploading}
              className="btn primary"
              onClick={() => {
                input.current?.click();
              }}
            >
              <Icon name="Upload"></Icon>
              {viewState.uploading ? t("正在上传…") : t("上传文件")}
            </button>
          ) : null}
          {can("file:styles") && viewState.section === "styles" ? (
            <button
              className="btn primary"
              onClick={() => {
                openStyle();
              }}
            >
              <Icon name="Plus"></Icon>
              {t("新增样式")}
            </button>
          ) : null}
          <input
            ref={(element) => {
              input.current = element;
            }}
            type="file"
            multiple
            tabIndex={-1}
            aria-label={t("选择要上传的文件")}
            className="sr-only"
            onChange={(event) => {
              upload((event.target as HTMLInputElement).files);
            }}
          />
        </div>
      </header>
      {viewState.error ? (
        <div data-tone="crit" role="alert" className="banner">
          <Icon name="TriangleAlert" size={15}></Icon>
          <div className="banner-text">
            <p>{viewState.error}</p>
          </div>
          <button
            className="link"
            onClick={() => {
              reload();
            }}
          >
            {t("重试")}
          </button>
        </div>
      ) : null}
      {viewState.uploading ? (
        <div role="status" className="upload-line">
          <Icon name="Upload" size={15}></Icon>
          <strong className="ellipsis">{viewState.uploadName}</strong>
          <span className="meter">
            <i style={{ width: viewState.progress + "%" } as CSSProperties}></i>
          </span>
          <span>
            {viewState.progress}
            {"%"}
          </span>
        </div>
      ) : null}
      {viewState.uploadFailures.length ? (
        <div data-tone="warn" role="alert" className="banner">
          <Icon name="TriangleAlert" size={15}></Icon>
          <div className="banner-text">
            <strong>{t("以下文件没有上传成功")}</strong>
            <>
              {viewState.uploadFailures.map((f, index) => (
                <p key={index}>{f}</p>
              ))}
            </>
          </div>
          <button
            className="link"
            onClick={() => {
              store.setState({ uploadFailures: [] });
            }}
          >
            {t("收起")}
          </button>
        </div>
      ) : null}
      <section className="panel files">
        <aside aria-label={t("目录")} className="folders">
          <div className="folders-title">{t("目录")}</div>
          <>
            {folderRows.map((f) => (
              <button
                key={f.id}
                style={{ "--depth": Math.min(f.depth, 5) } as CSSProperties}
                aria-current={
                  viewState.section === "files" && viewState.parent === f.id
                    ? "true"
                    : undefined
                }
                className={classNames("folder", {
                  active:
                    viewState.section === "files" && viewState.parent === f.id,
                })}
                onClick={() => {
                  openFolder(f.id);
                }}
              >
                <Icon name="FolderOpen" size={15}></Icon>
                <span title={resourceName(f)}>{resourceName(f)}</span>
              </button>
            ))}
          </>
          <hr />
          <button
            className={classNames("folder", {
              active: viewState.section === "styles",
            })}
            onClick={() => {
              store.setState({ section: "styles" });
            }}
          >
            <Icon name="Image" size={15}></Icon>
            <span>{t("图片样式")}</span>
            <small>{viewState.styles.length}</small>
          </button>
          <button
            className={classNames("folder", {
              active: viewState.section === "trash",
            })}
            onClick={() => {
              store.setState({ section: "trash", page: 1, query: "" });
            }}
          >
            <Icon name="Trash2" size={15}></Icon>
            <span>{t("回收站")}</span>
            <small>{viewState.tree.trashCount}</small>
          </button>
          <div className="storage">
            <span>{t("存储用量")}</span>
            <strong>
              {bytes(viewState.tree.storageUsed)}
              {" / "}
              {bytes(viewState.tree.storageLimitMB * 1048576)}
            </strong>
            <span
              data-tone={usedPercent >= 90 ? "warn" : undefined}
              role="meter"
              aria-valuenow={Math.round(usedPercent)}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-label={t("存储用量")}
              className="meter"
            >
              <i style={{ width: usedPercent + "%" } as CSSProperties}></i>
            </span>
            <span>
              {t("单文件上限 {p0} MB，回收站计入配额", {
                p0: viewState.tree.uploadLimitMB,
              })}
            </span>
          </div>
        </aside>
        <div
          className="file-main"
          onDragOver={(event) => {
            event.preventDefault();
            store.setState({
              dragging: can("file:upload") && viewState.section === "files",
            });
          }}
          onDragLeave={(event) => {
            if (event.target !== event.currentTarget) return;
            store.setState({ dragging: false });
          }}
          onDrop={(event) => {
            event.preventDefault();
            dropped(event);
          }}
        >
          {viewState.section !== "styles" ? (
            <>
              <div className="toolbar">
                <nav aria-label={t("当前位置")} className="crumbs">
                  {viewState.section === "files" ? (
                    <>
                      <>
                        {crumbs.map((f, index) => (
                          <Fragment key={f.id}>
                            {index ? (
                              <Icon name="ChevronRight" size={13}></Icon>
                            ) : null}
                            <button
                              aria-current={
                                index === crumbs.length - 1 ? "page" : undefined
                              }
                              onClick={() => {
                                openFolder(f.id);
                              }}
                            >
                              {resourceName(f)}
                            </button>
                          </Fragment>
                        ))}
                      </>
                    </>
                  ) : (
                    <strong>{t("回收站")}</strong>
                  )}
                </nav>
                <label className="search">
                  <Icon name="Search" size={15}></Icon>
                  <input
                    aria-label={t("搜索当前目录")}
                    placeholder={t("搜索当前目录")}
                    value={viewState.query}
                    onChange={(event) => {
                      store.setState({ query: event.currentTarget.value });
                    }}
                  />
                </label>
                <div role="group" aria-label={t("显示方式")} className="seg">
                  <button
                    aria-pressed={viewState.layout === "grid"}
                    aria-label={t("网格视图")}
                    title={t("网格视图")}
                    className={classNames({
                      active: viewState.layout === "grid",
                    })}
                    onClick={() => {
                      store.setState({ layout: "grid" });
                    }}
                  >
                    <Icon name="Grid2X2" size={15}></Icon>
                  </button>
                  <button
                    aria-pressed={viewState.layout === "list"}
                    aria-label={t("列表视图")}
                    title={t("列表视图")}
                    className={classNames({
                      active: viewState.layout === "list",
                    })}
                    onClick={() => {
                      store.setState({ layout: "list" });
                    }}
                  >
                    <Icon name="List" size={15}></Icon>
                  </button>
                </div>
                <button
                  aria-label={t("刷新文件")}
                  title={t("刷新")}
                  className="icon-btn"
                  onClick={() => {
                    reload();
                  }}
                >
                  <Icon
                    name="RefreshCw"
                    className={classNames({ spin: viewState.loading })}
                  ></Icon>
                </button>
              </div>
              {viewState.section === "trash" ? (
                <p className="note trash-note">
                  <Icon name="Info" size={14}></Icon>
                  {t(
                    "删除的文件和文件夹会保留在这里。恢复后重新可用；永久删除会释放存储空间且无法撤销。",
                  )}
                </p>
              ) : null}
              {viewState.loading && !viewState.entries.length ? (
                <div className="empty">{t("正在加载文件…")}</div>
              ) : (
                <>
                  {viewState.layout === "grid" && viewState.entries.length ? (
                    <div className="file-grid">
                      <>
                        {viewState.entries.map((f) => (
                          <article key={f.id} className="file-card">
                            <button
                              disabled={
                                viewState.section === "trash" ||
                                (f.kind === "file" && !can("file:read"))
                              }
                              aria-label={
                                (f.kind === "folder"
                                  ? t("打开 ")
                                  : t("预览 ")) + resourceName(f)
                              }
                              className="file-thumb"
                              onClick={() => {
                                show(f);
                              }}
                            >
                              {viewState.section === "files" &&
                              imageFile(f) &&
                              can("file:read") ? (
                                <img
                                  src={apiLink(
                                    `/api/v1/files/${f.id}/image?size=thumb`,
                                  )}
                                  alt={resourceName(f)}
                                  loading="lazy"
                                />
                              ) : (
                                <Icon
                                  name={
                                    f.kind === "folder"
                                      ? "FolderOpen"
                                      : imageFile(f)
                                        ? "Image"
                                        : "FileText"
                                  }
                                  size={34}
                                ></Icon>
                              )}
                              <span className="tag">{ext(f)}</span>
                            </button>
                            <div className="file-info">
                              <strong title={resourceName(f)}>
                                {resourceName(f)}
                              </strong>
                              <small>
                                {f.kind === "folder"
                                  ? t("文件夹")
                                  : bytes(f.size)}{" "}
                                {day(
                                  viewState.section === "trash"
                                    ? f.deletedAt
                                    : f.updatedAt,
                                )}
                              </small>
                            </div>
                            <div className="file-actions">
                              {viewState.section === "files" ? (
                                <>
                                  {can("file:manage") ? (
                                    <button
                                      className="link"
                                      onClick={() => {
                                        openAction("edit", f);
                                      }}
                                    >
                                      {t("编辑")}
                                    </button>
                                  ) : null}
                                  {f.kind === "file" && can("file:download") ? (
                                    <a
                                      href={apiLink(
                                        `/api/v1/files/${f.id}/download`,
                                      )}
                                      download
                                      className="link"
                                    >
                                      {t("下载")}
                                    </a>
                                  ) : null}
                                  {can("file:manage") ? (
                                    <button
                                      aria-label={
                                        t("移入回收站 ") + resourceName(f)
                                      }
                                      title={t("移入回收站")}
                                      className="icon-btn sm"
                                      onClick={() => {
                                        openAction("trash", f);
                                      }}
                                    >
                                      <Icon name="Trash2" size={14}></Icon>
                                    </button>
                                  ) : null}
                                </>
                              ) : can("file:manage") ? (
                                <>
                                  <button
                                    className="link"
                                    onClick={() => {
                                      openAction("restore", f);
                                    }}
                                  >
                                    {t("恢复")}
                                  </button>
                                  <button
                                    className="link danger"
                                    onClick={() => {
                                      openAction("purge", f);
                                    }}
                                  >
                                    {t("永久删除")}
                                  </button>
                                </>
                              ) : (
                                <span className="muted">{t("仅查看")}</span>
                              )}
                            </div>
                          </article>
                        ))}
                      </>
                    </div>
                  ) : viewState.entries.length ? (
                    <div className="table-wrap">
                      <table className="table">
                        <thead>
                          <tr>
                            <th>{t("名称")}</th>
                            <th className="num">{t("大小")}</th>
                            <th>{t("上传者")}</th>
                            <th>
                              {viewState.section === "trash"
                                ? t("删除时间")
                                : t("更新时间")}
                            </th>
                            <th className="actions">
                              <span className="sr-only">{t("操作")}</span>
                            </th>
                          </tr>
                        </thead>
                        <tbody>
                          <>
                            {viewState.entries.map((f) => (
                              <tr key={f.id}>
                                <td>
                                  <button
                                    disabled={
                                      viewState.section === "trash" ||
                                      (f.kind === "file" && !can("file:read"))
                                    }
                                    className="file-name"
                                    onClick={() => {
                                      show(f);
                                    }}
                                  >
                                    <Icon
                                      name={
                                        f.kind === "folder"
                                          ? "FolderOpen"
                                          : imageFile(f)
                                            ? "Image"
                                            : "FileText"
                                      }
                                    ></Icon>
                                    <span title={resourceName(f)}>
                                      {resourceName(f)}
                                    </span>
                                  </button>
                                </td>
                                <td className="num">
                                  {f.kind === "folder" ? "—" : bytes(f.size)}
                                </td>
                                <td>{f.creatorKey ? t("系统") : f.creator}</td>
                                <td className="tnum nowrap muted">
                                  {dateTime(
                                    viewState.section === "trash"
                                      ? f.deletedAt
                                      : f.updatedAt,
                                  )}
                                </td>
                                <td className="actions">
                                  <div className="row-actions">
                                    {viewState.section === "files" ? (
                                      <>
                                        {f.kind === "file" &&
                                        can("file:download") ? (
                                          <a
                                            href={apiLink(
                                              `/api/v1/files/${f.id}/download`,
                                            )}
                                            download
                                            className="link"
                                          >
                                            {t("下载")}
                                          </a>
                                        ) : null}
                                        {can("file:manage") ? (
                                          <button
                                            className="link"
                                            onClick={() => {
                                              openAction("edit", f);
                                            }}
                                          >
                                            {t("编辑")}
                                          </button>
                                        ) : null}
                                        {can("file:manage") ? (
                                          <button
                                            className={classNames(
                                              "link danger",
                                            )}
                                            onClick={() => {
                                              openAction("trash", f);
                                            }}
                                          >
                                            {t("删除")}
                                          </button>
                                        ) : null}
                                      </>
                                    ) : can("file:manage") ? (
                                      <>
                                        <button
                                          className="link"
                                          onClick={() => {
                                            openAction("restore", f);
                                          }}
                                        >
                                          {t("恢复")}
                                        </button>
                                        <button
                                          className="link danger"
                                          onClick={() => {
                                            openAction("purge", f);
                                          }}
                                        >
                                          {t("永久删除")}
                                        </button>
                                      </>
                                    ) : null}
                                  </div>
                                </td>
                              </tr>
                            ))}
                          </>
                        </tbody>
                      </table>
                    </div>
                  ) : null}
                  {!viewState.entries.length ? (
                    <div className="empty">
                      <Icon
                        name={
                          viewState.section === "trash"
                            ? "Trash2"
                            : "FolderOpen"
                        }
                        size={28}
                      ></Icon>
                      <h3>
                        {viewState.query
                          ? t("没有匹配的文件")
                          : viewState.section === "trash"
                            ? t("回收站是空的")
                            : t("这个目录还没有文件")}
                      </h3>
                      <p>
                        {viewState.section === "files" && can("file:upload")
                          ? t("点击“上传文件”，或把文件拖到这里。")
                          : t("文件会显示在这里。")}
                      </p>
                    </div>
                  ) : null}
                </>
              )}
              <footer className="panel-foot">
                <span>
                  {t("共 {p0} 项，第 {p1} / {p2} 页", {
                    p0: viewState.total,
                    p1: viewState.page,
                    p2: pages,
                  })}
                </span>
                <div className="row-actions tight">
                  <button
                    disabled={viewState.page <= 1}
                    className="btn sm"
                    onClick={() => {
                      store.setState({ page: viewState.page - 1 });
                    }}
                  >
                    {t("上一页")}
                  </button>
                  <button
                    disabled={viewState.page >= pages}
                    className="btn sm"
                    onClick={() => {
                      store.setState({ page: viewState.page + 1 });
                    }}
                  >
                    {t("下一页")}
                  </button>
                </div>
              </footer>
            </>
          ) : (
            <>
              <header className="panel-head">
                <div>
                  <h2>
                    {t("图片样式")}
                    <span className="count">
                      {viewState.styles.length}
                      {" / 30"}
                    </span>
                  </h2>
                  <p>{t("为图片生成固定尺寸的副本，原图保持不变。")}</p>
                </div>
              </header>
              {viewState.styles.length ? (
                <div className="style-grid">
                  <>
                    {viewState.styles.map((preset) => (
                      <article key={preset.id} className="style-card">
                        <div className="style-frame">
                          <i
                            style={
                              {
                                aspectRatio: `${preset.width}/${preset.height}`,
                                width:
                                  preset.width >= preset.height
                                    ? "82%"
                                    : "auto",
                                height:
                                  preset.width >= preset.height
                                    ? "auto"
                                    : "76px",
                              } as CSSProperties
                            }
                          ></i>
                        </div>
                        <strong>{resourceName(preset)}</strong>
                        <small>
                          {preset.width}
                          {" × "}
                          {preset.height}
                          {" px，"}
                          {preset.fit === "cover"
                            ? t("居中裁切")
                            : t("等比适配")}
                        </small>
                        {can("file:styles") ? (
                          <div className="row-actions start">
                            <button
                              className="link"
                              onClick={() => {
                                openStyle(preset);
                              }}
                            >
                              {t("编辑")}
                            </button>
                            <button
                              className="link danger"
                              onClick={() => {
                                store.setState({
                                  styleTarget: preset,
                                  mode: "archive-style",
                                  modalError: "",
                                });
                              }}
                            >
                              {t("归档")}
                            </button>
                          </div>
                        ) : null}
                      </article>
                    ))}
                  </>
                </div>
              ) : (
                <div className="empty">
                  <Icon name="Image" size={28}></Icon>
                  <h3>{t("还没有图片样式")}</h3>
                  <p>
                    {t("例如列表缩略图或文章封面，预览图片时即可选择样式。")}
                  </p>
                </div>
              )}
              <p className="note style-note">
                <Icon name="Info" size={14}></Icon>
                {t(
                  "在图片预览中选择样式即可查看效果并下载。支持 PNG、JPEG、GIF 和 WebP，GIF 取首帧，统一输出 PNG；尺寸过大的图片只提供原文件下载。",
                )}
              </p>
            </>
          )}
          {viewState.dragging ? (
            <div className="drop-overlay">
              <Icon name="Upload" size={28}></Icon>
              {t("松开即可上传到当前目录")}
            </div>
          ) : null}
        </div>
      </section>
      {viewState.mode ? (
        <Modal
          title={actionTitle}
          onClose={() => {
            !viewState.busy && store.setState({ mode: "" });
          }}
          footer={
            <>
              <button
                disabled={viewState.busy}
                className="btn"
                onClick={() => {
                  store.setState({ mode: "" });
                }}
              >
                {t("取消")}
              </button>
              <button
                form="file-action"
                disabled={
                  viewState.busy ||
                  (viewState.mode === "purge" &&
                    viewState.form.confirm !== t("永久删除"))
                }
                className={classNames(
                  "btn",
                  viewState.mode === "purge" ||
                    viewState.mode === "archive-style"
                    ? "danger"
                    : "primary",
                )}
              >
                {actionLabel}
              </button>
            </>
          }
        >
          <Form
            id="file-action"
            className="form"
            onSubmit={(event) => {
              event.preventDefault();
              save();
            }}
          >
            {["folder", "edit", "restore"].includes(viewState.mode) ? (
              <>
                <label className="field">
                  {t("名称")}
                  <input
                    required
                    maxLength={66}
                    placeholder={t("输入名称")}
                    value={
                      !viewState.nameEdited && viewState.target?.nameKey
                        ? resourceName(viewState.target)
                        : viewState.form.name
                    }
                    onChange={(event) => {
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          name: event.currentTarget.value,
                        },
                        nameEdited: true,
                      }));
                    }}
                  />
                </label>
                <label className="field">
                  {viewState.mode === "restore" ? t("恢复到") : t("所在目录")}
                  <select
                    value={viewState.form.parentId}
                    onChange={(event) => {
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          parentId: event.currentTarget.value,
                        },
                      }));
                    }}
                  >
                    <>
                      {folderRows
                        .filter((v) => v.id !== viewState.target?.id)
                        .map((f) => (
                          <option key={f.id} value={f.id}>
                            {"　".repeat(f.depth) + resourceName(f)}
                          </option>
                        ))}
                    </>
                  </select>
                  <small>{t("同一目录下的名称不能重复。")}</small>
                </label>
              </>
            ) : viewState.mode === "style" ? (
              <>
                <label className="field">
                  {t("样式名称")}
                  <input
                    required
                    maxLength={33}
                    placeholder={t("例如：文章封面")}
                    value={
                      !viewState.styleNameEdited &&
                      viewState.styleTarget?.nameKey
                        ? resourceName(viewState.styleTarget)
                        : viewState.styleForm.name
                    }
                    onChange={(event) => {
                      store.setState((current) => ({
                        styleForm: {
                          ...current.styleForm,
                          name: event.currentTarget.value,
                        },
                        styleNameEdited: true,
                      }));
                    }}
                  />
                </label>
                <div className="form-grid">
                  <label className="field">
                    {t("宽度（像素）")}
                    <input
                      type="number"
                      min="16"
                      max="2048"
                      required
                      value={viewState.styleForm.width}
                      onChange={(event) => {
                        store.setState((current) => ({
                          styleForm: {
                            ...current.styleForm,
                            width: Number(event.currentTarget.value),
                          },
                        }));
                      }}
                    />
                  </label>
                  <label className="field">
                    {t("高度（像素）")}
                    <input
                      type="number"
                      min="16"
                      max="2048"
                      required
                      value={viewState.styleForm.height}
                      onChange={(event) => {
                        store.setState((current) => ({
                          styleForm: {
                            ...current.styleForm,
                            height: Number(event.currentTarget.value),
                          },
                        }));
                      }}
                    />
                  </label>
                </div>
                <fieldset className="field">
                  <legend className="field-label">{t("缩放方式")}</legend>
                  <div className="choice-group">
                    <label className="choice block">
                      <input
                        type="radio"
                        value="cover"
                        checked={viewState.styleForm.fit === "cover"}
                        onChange={() => {
                          store.setState((current) => ({
                            styleForm: { ...current.styleForm, fit: "cover" },
                          }));
                        }}
                      />
                      <span className="choice-text">
                        <strong>{t("居中裁切")}</strong>
                        <small>{t("填满指定尺寸，超出部分裁掉")}</small>
                      </span>
                    </label>
                    <label className="choice block">
                      <input
                        type="radio"
                        value="contain"
                        checked={viewState.styleForm.fit === "contain"}
                        onChange={() => {
                          store.setState((current) => ({
                            styleForm: { ...current.styleForm, fit: "contain" },
                          }));
                        }}
                      />
                      <span className="choice-text">
                        <strong>{t("等比适配")}</strong>
                        <small>{t("保留完整画面，不放大原图")}</small>
                      </span>
                    </label>
                  </div>
                </fieldset>
              </>
            ) : viewState.mode === "trash" ? (
              <>
                <p>
                  {t("把“{p0}”移入回收站？{p1}之后可以在回收站中恢复。", {
                    p0: resourceName(viewState.target),
                    p1:
                      viewState.target?.kind === "folder"
                        ? t("其中的文件和文件夹会一起移入。")
                        : "",
                  })}
                </p>
              </>
            ) : viewState.mode === "purge" ? (
              <>
                <p>
                  {t("“{p0}”以及同一次删除的内容会被永久移除，无法恢复。", {
                    p0: resourceName(viewState.target),
                  })}
                </p>
                <label className="field">
                  {t("输入“永久删除”以确认")}
                  <input
                    required
                    autoComplete="off"
                    value={viewState.form.confirm}
                    onChange={(event) => {
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          confirm: event.currentTarget.value,
                        },
                      }));
                    }}
                  />
                </label>
              </>
            ) : (
              <p>
                {t("归档“{p0}”后不再提供这个样式，原图不受影响。", {
                  p0: resourceName(viewState.styleTarget),
                })}
              </p>
            )}
            {viewState.modalError ? (
              <p role="alert" className="form-error">
                {viewState.modalError}
              </p>
            ) : null}
          </Form>
        </Modal>
      ) : null}
      {viewState.preview ? (
        <Modal
          title={viewState.preview.file.name}
          subtitle={`${bytes(viewState.preview.file.size)}，${viewState.preview.file.mime}`}
          wide
          onClose={() => {
            closePreview();
          }}
          footer={
            <>
              <button
                className="btn"
                onClick={() => {
                  closePreview();
                }}
              >
                {t("关闭")}
              </button>
              {can("file:download") &&
              viewState.preview.kind === "image" &&
              viewState.previewStyle ? (
                <a href={imageURL + "&download=1"} download className="btn">
                  <Icon name="Image"></Icon>
                  {t("下载样式图")}
                </a>
              ) : null}
              {can("file:download") ? (
                <a
                  href={apiLink(
                    `/api/v1/files/${viewState.preview.file.id}/download`,
                  )}
                  download
                  className="btn primary"
                >
                  <Icon name="Download"></Icon>
                  {t("下载原文件")}
                </a>
              ) : null}
            </>
          }
        >
          {viewState.preview.kind === "image" ? (
            <>
              <div className="preview-tools">
                <label className="person">
                  {t("图片样式")}
                  <select
                    value={viewState.previewStyle}
                    onChange={(event) => {
                      store.setState({
                        previewStyle: event.currentTarget.value,
                      });
                    }}
                  >
                    <option value="">
                      {t("预览尺寸（最长 1600 × 1200）")}
                    </option>
                    <>
                      {viewState.styles.map((style) => (
                        <option key={style.id} value={style.id}>
                          {resourceName(style)}
                          {"，"}
                          {style.width}
                          {" × "}
                          {style.height}
                        </option>
                      ))}
                    </>
                  </select>
                </label>
                <span className="muted num">
                  {t("原图 {p0} × {p1}", {
                    p0: viewState.preview.file.width,
                    p1: viewState.preview.file.height,
                  })}
                </span>
              </div>
              <div className="preview-stage">
                {!viewState.imageError ? (
                  <img
                    src={imageURL}
                    alt={viewState.preview.file.name}
                    onError={() => {
                      store.setState({ imageError: true });
                    }}
                  />
                ) : (
                  <p className="form-error">
                    {t("图片无法预览，请下载原文件查看。")}
                  </p>
                )}
              </div>
            </>
          ) : viewState.preview.kind === "text" ? (
            <>
              <pre className="text-preview">{viewState.preview.text}</pre>
              {viewState.preview.truncated ? (
                <p className="note">
                  <Icon name="Info" size={14}></Icon>
                  {t("只显示前 256 KB，完整内容请下载查看。")}
                </p>
              ) : null}
            </>
          ) : (
            <div className="empty compact">
              <Icon name="FileText" size={28}></Icon>
              <h3>{t("这个文件不支持在线预览")}</h3>
              <p>{t("请下载后用本地应用打开。")}</p>
            </div>
          )}
          <dl className="kv">
            <div>
              <dt>{t("上传者")}</dt>
              <dd>
                {viewState.preview.file.creatorKey
                  ? t("系统")
                  : viewState.preview.file.creator}
              </dd>
            </div>
            <div>
              <dt>{t("上传时间")}</dt>
              <dd className="num">
                {dateTime(viewState.preview.file.createdAt)}
              </dd>
            </div>
            <div className="span-all">
              <dt>{"SHA-256"}</dt>
              <dd className="mono">{viewState.preview.file.sha256}</dd>
            </div>
          </dl>
        </Modal>
      ) : null}
    </>
  );
}
