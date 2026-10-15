import { useEffect, useState, type CSSProperties } from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import Modal from "../components/Modal";
import StatusBadge from "../components/StatusBadge";
import { localizedLabels, metadataLabel, t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import {
  api,
  displayNavName,
  notify,
  refreshNavigation,
  useApp,
  type NavItem,
  type PermissionGroup,
} from "../stores/app";
export default function MenusView() {
  const blank = () => ({
    parentId: "",
    name: "",
    kind: "page",
    pageKey: "overview",
    url: "",
    icon: "LayoutDashboard",
    sort: 10,
    visible: true,
    permission: "",
    version: 0,
  });
  const initialState = {
    items: [] as NavItem[],
    pages: [] as { key: string; name: string; permission: string }[],
    icons: [] as string[],
    permissions: [] as PermissionGroup[],
    revision: "",
    error: "",
    loading: true,
    busy: false,
    editing: false,
    nameEdited: false,
    confirm: "" as "delete" | "reset" | "",
    target: undefined as NavItem | undefined,
    form: blank(),
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const appState = useApp();
  useI18n();
  const iconLabels: Record<string, string> = localizedLabels({
    KeyRound: "密钥",
    Package: "安装包",
    QrCode: "二维码",
    Barcode: "条码",
    LayoutDashboard: "总览",
    Activity: "活动",
    Server: "主机",
    Radar: "检测",
    Bell: "告警",
    FolderOpen: "文件夹",
    FileText: "文档",
    Image: "图片",
    ShieldCheck: "安全",
    ListTree: "菜单树",
    ScrollText: "审计",
    Settings2: "设置",
    Globe: "全球",
    Link: "链接",
    Box: "资源",
    Users: "团队",
    Wrench: "工具",
    HardDrive: "存储",
    BookOpen: "手册",
  });
  const permissionLabel = (key: string) =>
    metadataLabel(
      store
        .getState()
        .permissions.flatMap((g) => g.items)
        .find((p) => p.key === key)?.label,
    ) || t("无");
  const kindLabel: Record<string, string> = localizedLabels({
    group: "分组",
    page: "页面",
    link: "外链",
  });
  const getRows = () => {
    const out: (NavItem & {
      depth: number;
    })[] = [];
    function walk(parent: string, depth: number) {
      for (const item of store
        .getState()
        .items.filter((v) => v.parentId === parent)) {
        out.push({ ...item, depth });
        walk(item.id, depth + 1);
      }
    }
    walk("", 0);
    return out;
  };
  const getGroups = () =>
    getRows().filter(
      (v) => v.kind === "group" && v.id !== store.getState().target?.id,
    );
  async function load() {
    try {
      const v = await api<{
        items: NavItem[];
        pages: LocalState["pages"];
        icons: string[];
        permissions: PermissionGroup[];
        revision: string;
      }>("/menus");
      store.setState({ items: v.items });
      store.setState({ pages: v.pages });
      store.setState({ icons: v.icons });
      store.setState({ permissions: v.permissions });
      store.setState({ revision: v.revision });
      store.setState({ error: "" });
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ loading: false });
    }
  }
  function edit(item?: NavItem) {
    store.setState({ target: item, nameEdited: false });
    store.setState({
      form: {
        ...store.getState().form,
        ...blank(),
        ...(item
          ? {
              parentId: item.parentId,
              name: item.name,
              kind: item.kind,
              pageKey: item.pageKey,
              url: item.url,
              icon: item.icon,
              sort: item.sort,
              visible: item.visible,
              permission: item.permission,
              version: item.version,
            }
          : {}),
      },
    });
    store.setState({ error: "" });
    store.setState({ editing: true });
  }
  async function save() {
    const target = store.getState().target;
    store.setState({ busy: true });
    try {
      await api(
        target ? `/menus/${target.id}` : "/menus",
        target ? "PATCH" : "POST",
        {
          ...store.getState().form,
          // Keeping the stored name preserves its built-in translation key when
          // only visibility/order changes. Explicit name edits remain user text.
          name:
            target && !store.getState().nameEdited
              ? target.name
              : store.getState().form.name,
        },
      );
      store.setState({ editing: false });
      await load();
      await refreshNavigation();
      notify(t("菜单已保存，导航已更新"));
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  async function execute() {
    const target = store.getState().target;
    store.setState({ busy: true });
    try {
      if (store.getState().confirm === "reset")
        await api("/menus/reset", "POST", {
          revision: store.getState().revision,
        });
      else if (target)
        await api(`/menus/${target.id}`, "DELETE", {
          version: target.version,
        });
      store.setState({ confirm: "" });
      await load();
      await refreshNavigation();
      notify(t("菜单配置已更新"));
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  useEffect(() => {
    load();
  }, []);

  const rows = getRows();
  const groups = getGroups();
  return (
    <>
      <header className="page-head">
        <div className="page-title">
          <h1>{appState.pageTitle}</h1>
          <p>
            {t(
              "{p0} 个导航项，{p1} 个显示中。隐藏只影响导航展示，页面权限仍由角色授权决定。",
              {
                p0: viewState.items.length,
                p1: viewState.items.filter((v) => v.visible).length,
              },
            )}
          </p>
        </div>
        <div className="page-actions">
          <button
            disabled={viewState.loading}
            className="btn"
            onClick={() => {
              store.setState({ confirm: "reset", error: "" });
            }}
          >
            <Icon name="RotateCcw"></Icon>
            {t("恢复内置导航")}
          </button>
          <button
            className="btn primary"
            onClick={() => {
              edit();
            }}
          >
            <Icon name="Plus"></Icon>
            {t("新增菜单")}
          </button>
        </div>
      </header>
      {viewState.error && !viewState.editing && !viewState.confirm ? (
        <div data-tone="crit" role="alert" className="banner">
          <Icon name="TriangleAlert" size={15}></Icon>
          <div className="banner-text">
            <p>{viewState.error}</p>
          </div>
          <button
            className="link"
            onClick={() => {
              load();
            }}
          >
            {t("重试")}
          </button>
        </div>
      ) : null}
      <section className="panel">
        {rows.length ? (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>{t("名称")}</th>
                  <th>{t("类型")}</th>
                  <th>{t("指向")}</th>
                  <th className="num">{t("排序")}</th>
                  <th>{t("导航中")}</th>
                  <th>{t("附加访问限制")}</th>
                  <th className="actions">
                    <span className="sr-only">{t("操作")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                <>
                  {rows.map((item) => (
                    <tr key={item.id}>
                      <td>
                        <div
                          style={{ "--depth": item.depth } as CSSProperties}
                          className="menu-name"
                        >
                          <Icon name={item.icon}></Icon>
                          <strong>{displayNavName(item)}</strong>
                          {item.builtin ? (
                            <span className="tag">{t("内置")}</span>
                          ) : null}
                        </div>
                      </td>
                      <td>{kindLabel[item.kind]}</td>
                      <td>
                        <span
                          title={item.kind === "link" ? item.url : undefined}
                          className={classNames("menu-target", {
                            mono: item.kind === "link",
                          })}
                        >
                          <span className="ellipsis">
                            {item.kind === "page"
                              ? metadataLabel(
                                  viewState.pages.find(
                                    (p) => p.key === item.pageKey,
                                  )?.name,
                                )
                              : item.kind === "link"
                                ? item.url
                                : "—"}
                          </span>
                        </span>
                      </td>
                      <td className="num">{item.sort}</td>
                      <td>
                        <StatusBadge
                          status={item.visible ? "enabled" : "disabled"}
                          label={item.visible ? t("显示") : t("隐藏")}
                        ></StatusBadge>
                      </td>
                      <td className={classNames({ muted: !item.permission })}>
                        {permissionLabel(item.permission)}
                      </td>
                      <td className="actions">
                        <div className="row-actions">
                          <button
                            className="link"
                            onClick={() => {
                              edit(item);
                            }}
                          >
                            {t("编辑")}
                          </button>
                          {!item.builtin ? (
                            <button
                              className="link danger"
                              onClick={() => {
                                store.setState({
                                  target: item,
                                  confirm: "delete",
                                  error: "",
                                });
                              }}
                            >
                              {t("删除")}
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
            {viewState.loading ? t("正在加载菜单…") : t("还没有菜单")}
          </div>
        )}
        <footer className="panel-foot">
          <span>{t("最多 200 项、3 层，排序值越小越靠前")}</span>
          <span>
            {t("{p0} 个内置项", {
              p0: viewState.items.filter((v) => v.builtin).length,
            })}
          </span>
        </footer>
      </section>
      <p className="note page-note">
        <Icon name="Info" size={14}></Icon>
        {t(
          "即使隐藏了这个页面，有权限的管理员仍可从左下角的账号菜单进入“管理导航”。",
        )}
      </p>
      {viewState.editing ? (
        <Modal
          title={viewState.target ? t("编辑菜单") : t("新增菜单")}
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
                form="menu-form"
                disabled={viewState.busy}
                className="btn primary"
              >
                {viewState.busy ? t("正在保存…") : t("保存菜单")}
              </button>
            </>
          }
        >
          <form
            id="menu-form"
            className="form"
            onSubmit={(event) => {
              event.preventDefault();
              save();
            }}
          >
            <div className="form-grid">
              <label className="field">
                {t("名称")}
                <input
                  required
                  maxLength={40}
                  placeholder={t("例如：运维手册")}
                  value={
                    viewState.target?.nameKey && !viewState.nameEdited
                      ? displayNavName(viewState.target)
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
                {t("上级分组")}
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
                  <option value="">{t("顶级")}</option>
                  <>
                    {groups.map((group) => (
                      <option key={group.id} value={group.id}>
                        {"　".repeat(group.depth) + displayNavName(group)}
                      </option>
                    ))}
                  </>
                </select>
              </label>
              <label className="field">
                {t("类型")}
                <select
                  disabled={viewState.target?.builtin}
                  value={viewState.form.kind}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        kind: event.currentTarget.value,
                      },
                    }));
                  }}
                >
                  <option value="page">{t("内部页面")}</option>
                  <option value="group">{t("分组")}</option>
                  <option value="link">{t("外部链接")}</option>
                </select>
              </label>
              {viewState.form.kind === "page" ? (
                <label className="field">
                  {t("指向页面")}
                  <select
                    disabled={viewState.target?.builtin}
                    value={viewState.form.pageKey}
                    onChange={(event) => {
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          pageKey: event.currentTarget.value,
                        },
                      }));
                    }}
                  >
                    <>
                      {viewState.pages.map((p) => (
                        <option key={p.key} value={p.key}>
                          {metadataLabel(p.name)}
                        </option>
                      ))}
                    </>
                  </select>
                </label>
              ) : null}
              {viewState.form.kind === "link" ? (
                <label className="field">
                  {t("链接地址")}
                  <input
                    type="url"
                    required
                    placeholder="https://…"
                    className="mono"
                    value={viewState.form.url}
                    onChange={(event) => {
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          url: event.currentTarget.value,
                        },
                      }));
                    }}
                  />
                </label>
              ) : null}
              <label className="field">
                {t("图标")}
                <select
                  value={viewState.form.icon}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        icon: event.currentTarget.value,
                      },
                    }));
                  }}
                >
                  <>
                    {viewState.icons.map((icon) => (
                      <option key={icon} value={icon}>
                        {iconLabels[icon] || icon}
                      </option>
                    ))}
                  </>
                </select>
              </label>
              <label className="field">
                {t("排序值")}
                <input
                  type="number"
                  min="0"
                  max="9999"
                  required
                  value={viewState.form.sort}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        sort: Number(event.currentTarget.value),
                      },
                    }));
                  }}
                />
              </label>
              <label className="field">
                {t("附加访问限制")}
                <select
                  value={viewState.form.permission}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        permission: event.currentTarget.value,
                      },
                    }));
                  }}
                >
                  <option value="">{t("不附加")}</option>
                  <>
                    {viewState.permissions.map((p) => (
                      <optgroup key={p.key} label={metadataLabel(p.name)}>
                        <>
                          {p.items.map((v) => (
                            <option key={v.key} value={v.key}>
                              {metadataLabel(v.label)}
                            </option>
                          ))}
                        </>
                      </optgroup>
                    ))}
                  </>
                </select>
              </label>
              <label className="field">
                {t("在导航中")}
                <select
                  value={String(viewState.form.visible)}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        visible: event.currentTarget.value === "true",
                      },
                    }));
                  }}
                >
                  <option value="true">{t("显示")}</option>
                  <option value="false">{t("隐藏")}</option>
                </select>
              </label>
            </div>
            {viewState.target?.builtin ? (
              <p className="note">
                <Icon name="LockKeyhole" size={14}></Icon>
                {t(
                  "内置菜单的类型和指向页面不能修改，可以改名、调整位置或隐藏。",
                )}
              </p>
            ) : null}
            {viewState.error ? (
              <p role="alert" className="form-error">
                {viewState.error}
              </p>
            ) : null}
          </form>
        </Modal>
      ) : null}
      {viewState.confirm ? (
        <Modal
          title={
            viewState.confirm === "reset" ? t("恢复内置导航") : t("删除菜单")
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
                className={classNames(
                  "btn",
                  viewState.confirm === "reset" ? "primary" : "danger",
                )}
                onClick={() => {
                  execute();
                }}
              >
                {viewState.confirm === "reset"
                  ? t("恢复内置导航")
                  : t("删除菜单")}
              </button>
            </>
          }
        >
          <p>
            {viewState.confirm === "reset"
              ? t(
                  "内置菜单的名称、分组、图标、排序和显示状态会恢复默认，自定义菜单会保留。",
                )
              : t(
                  "删除“{p0}”？如果它是分组且包含子菜单，需要先移动或删除子菜单。",
                  {
                    p0: viewState.target
                      ? displayNavName(viewState.target)
                      : "",
                  },
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
