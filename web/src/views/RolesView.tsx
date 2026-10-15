import { useEffect, useState } from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import Modal from "../components/Modal";
import StatusBadge from "../components/StatusBadge";
import { metadataLabel, t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import {
  action,
  api,
  appStore,
  can,
  notify,
  useApp,
  type User,
} from "../stores/app";
export default function RolesView() {
  const initialState = {
    roleId: "2",
    draft: [] as string[],
    base: [] as string[],
    baseVersion: 0,
    tab: "permissions",
    createOpen: false,
    sensitiveConfirm: false,
    pendingRole: "",
    busy: false,
    newName: "",
    newDescription: "",
    error: "",
    userOpen: false,
    editing: null as User | null,
    username: "",
    name: "",
    password: "",
    enabled: true,
    userRoles: [] as string[],
    userError: "",
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const appState = useApp();
  useI18n();
  const getRole = () =>
    appStore
      .getState()
      .data.roles.find((r) => r.id === store.getState().roleId) ||
    appStore.getState().data.roles[0];
  const getDirty = () =>
    [...store.getState().draft].sort().join() !==
    [...store.getState().base].sort().join();
  function reset() {
    if (!getRole()) return;
    store.setState({ roleId: getRole().id });
    store.setState({ draft: [...getRole().permissions] });
    store.setState({ base: [...getRole().permissions] });
    store.setState({ baseVersion: getRole().version });
  }
  const getSensitiveKeys = () =>
    appStore
      .getState()
      .data.permissions.flatMap((g) =>
        g.items.filter((i) => i.sensitive).map((i) => i.key),
      );
  const getAddedSensitive = () =>
    store
      .getState()
      .draft.filter(
        (p) =>
          getSensitiveKeys().includes(p) && !store.getState().base.includes(p),
      );
  function choose(id: string) {
    if (id === store.getState().roleId) return;
    if (getDirty()) store.setState({ pendingRole: id });
    else {
      store.setState({ roleId: id });
      store.setState({ tab: "permissions" });
    }
  }
  function toggleGroup(keys: string[], checked: boolean) {
    if (getRole()?.builtin) return;
    store.setState({
      draft: checked
        ? [...new Set([...store.getState().draft, ...keys])]
        : store.getState().draft.filter((k) => !keys.includes(k)),
    });
  }
  async function commit() {
    if (!getRole()) return;
    store.setState({ busy: true });
    const ok = await action(
      () =>
        api(`/roles/${getRole()!.id}`, "PATCH", {
          permissions: store.getState().draft,
          version: store.getState().baseVersion,
        }),
      t("角色权限已保存，受影响成员需要重新登录"),
    );
    if (ok) {
      store.setState({ sensitiveConfirm: false });
      reset();
    }
    store.setState({ busy: false });
  }
  function save() {
    if (getAddedSensitive().length) store.setState({ sensitiveConfirm: true });
    else commit();
  }
  async function create() {
    store.setState({ error: "" });
    store.setState({ busy: true });
    try {
      const v = await api<{
        id: string;
      }>("/roles", "POST", {
        name: store.getState().newName,
        description: store.getState().newDescription,
      });
      await action(async () => {});
      store.setState({ roleId: v.id });
      store.setState({ createOpen: false });
      store.setState({ newName: "" });
      store.setState({ newDescription: "" });
      notify(t("角色已创建，请分配权限"));
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  const getPermissionTotal = () =>
    appStore.getState().data.permissions.flatMap((g) => g.items).length;
  const roleNames = (u: User) =>
    u.roles
      .map(
        (id) => appStore.getState().data.roles.find((r) => r.id === id)?.name,
      )
      .filter(Boolean)
      .join("、");
  const getMembers = () =>
    appStore
      .getState()
      .data.users.filter((u) => u.roles.includes(getRole()?.id || ""));
  function openUser(u: User | null) {
    store.setState({ editing: u ? JSON.parse(JSON.stringify(u)) : null });
    store.setState({ username: u?.username || "" });
    store.setState({ name: u?.name || "" });
    store.setState({ password: "" });
    store.setState({ enabled: u?.enabled ?? true });
    store.setState({
      userRoles: u ? [...u.roles] : getRole()?.id ? [getRole()!.id] : [],
    });
    store.setState({ userError: "" });
    store.setState({ userOpen: true });
  }
  async function saveUser() {
    const editing = store.getState().editing;
    store.setState({ busy: true });
    store.setState({ userError: "" });
    try {
      await api(
        editing ? `/users/${editing.id}` : "/users",
        editing ? "PATCH" : "POST",
        {
          username: store.getState().username,
          name: store.getState().name,
          password: store.getState().password,
          enabled: store.getState().enabled,
          roles: store.getState().userRoles,
          version: editing?.version || 0,
        },
      );
      store.setState({ userOpen: false });
      await action(async () => {}, t("成员资料与授权已保存"));
    } catch (e) {
      store.setState({ userError: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  useEffect(() => {
    reset();
  }, [getRole()?.id]);
  useEffect(() => {
    if (!getDirty()) reset();
  }, [getRole()?.version]);

  const role = getRole();
  const dirty = getDirty();
  const addedSensitive = getAddedSensitive();
  const permissionTotal = getPermissionTotal();
  const members = getMembers();
  return (
    <>
      <header className="page-head">
        <div className="page-title">
          <h1>{appState.pageTitle}</h1>
          <p>
            {t(
              "{p0} 个角色，{p1} 名成员。成员的实际权限是其全部角色权限的合集。",
              {
                p0: appState.data.roles.length,
                p1: appState.data.users.length,
              },
            )}
          </p>
        </div>
        <div className="page-actions">
          {can("user:manage") ? (
            <button
              className="btn"
              onClick={() => {
                openUser(null);
              }}
            >
              <Icon name="UserPlus"></Icon>
              {t("添加成员")}
            </button>
          ) : null}
          {can("role:manage") ? (
            <button
              className="btn primary"
              onClick={() => {
                store.setState({ createOpen: true, error: "" });
              }}
            >
              <Icon name="Plus"></Icon>
              {t("新建角色")}
            </button>
          ) : null}
        </div>
      </header>
      {role ? (
        <div className="split">
          <section aria-label={t("角色列表")} className="panel">
            <header className="panel-head">
              <h2>
                {t("角色")}
                <span className="count">{appState.data.roles.length}</span>
              </h2>
            </header>
            <div className="list">
              <>
                {appState.data.roles.map((r) => (
                  <button
                    key={r.id}
                    aria-current={
                      viewState.roleId === r.id ? "true" : undefined
                    }
                    className={classNames("list-item", {
                      active: viewState.roleId === r.id,
                    })}
                    onClick={() => {
                      choose(r.id);
                    }}
                  >
                    <Icon
                      name={r.builtin ? "ShieldCheck" : "Users"}
                      size={16}
                      className="muted"
                    ></Icon>
                    <span className="list-text">
                      <strong>{r.name}</strong>
                      <small>
                        {t("{p0} 位成员，{p1} 项权限", {
                          p0: r.members,
                          p1: r.permissions.length,
                        })}
                      </small>
                    </span>
                    {r.builtin ? (
                      <Icon
                        name="LockKeyhole"
                        size={13}
                        className="muted"
                      ></Icon>
                    ) : null}
                  </button>
                ))}
              </>
            </div>
          </section>
          <section className="panel">
            <div className="role-detail-head">
              <div>
                <h2>
                  {role.name}
                  {role.builtin ? (
                    <span className="tag">{t("内置角色，不可修改")}</span>
                  ) : null}
                </h2>
                <p>{role.description || t("尚未填写角色说明")}</p>
              </div>
            </div>
            <nav aria-label={t("角色内容")} className="tabs">
              <button
                aria-current={
                  viewState.tab === "permissions" ? "page" : undefined
                }
                className={classNames("tab", {
                  active: viewState.tab === "permissions",
                })}
                onClick={() => {
                  store.setState({ tab: "permissions" });
                }}
              >
                {t("功能权限")}
                <span className="count">{viewState.draft.length}</span>
              </button>
              <button
                aria-current={viewState.tab === "members" ? "page" : undefined}
                className={classNames("tab", {
                  active: viewState.tab === "members",
                })}
                onClick={() => {
                  store.setState({ tab: "members" });
                }}
              >
                {t("成员")}
                <span className="count">{role.members}</span>
              </button>
            </nav>
            {viewState.tab === "permissions" ? (
              <>
                {dirty && role.version !== viewState.baseVersion ? (
                  <div data-tone="warn" className="banner inset">
                    <Icon name="TriangleAlert" size={15}></Icon>
                    <div className="banner-text">
                      <p>
                        {t(
                          "这个角色刚被其他成员修改。保存时会检查冲突；也可以撤销更改并加载最新版本。",
                        )}
                      </p>
                    </div>
                  </div>
                ) : null}
                <>
                  {appState.data.permissions.map((g) => (
                    <div key={g.key} className="perm-row">
                      <div className="perm-module">
                        <strong>{metadataLabel(g.name)}</strong>
                        <small>{metadataLabel(g.description)}</small>
                      </div>
                      <div className="perm-items">
                        <>
                          {g.items.map((item) => (
                            <label
                              key={item.key}
                              title={
                                item.sensitive
                                  ? t("敏感权限，授予前需要再次确认")
                                  : undefined
                              }
                              className="choice"
                            >
                              <input
                                type="checkbox"
                                value={item.key}
                                disabled={role.builtin || !can("role:manage")}
                                checked={viewState.draft.includes(item.key)}
                                onChange={(event) => {
                                  store.setState({
                                    draft: event.currentTarget.checked
                                      ? [...viewState.draft, item.key]
                                      : viewState.draft.filter(
                                          (selectedValue) =>
                                            selectedValue !== item.key,
                                        ),
                                  });
                                }}
                              />
                              {metadataLabel(item.label)}
                              {item.sensitive ? (
                                <span className="sensitive-mark">
                                  <Icon name="LockKeyhole" size={11}></Icon>
                                  {t("敏感")}
                                </span>
                              ) : null}
                            </label>
                          ))}
                        </>
                      </div>
                      <label className="check">
                        <input
                          type="checkbox"
                          checked={g.items.every((i) =>
                            viewState.draft.includes(i.key),
                          )}
                          ref={(element) => {
                            if (element)
                              element.indeterminate =
                                g.items.some((i) =>
                                  viewState.draft.includes(i.key),
                                ) &&
                                !g.items.every((i) =>
                                  viewState.draft.includes(i.key),
                                );
                          }}
                          disabled={role.builtin || !can("role:manage")}
                          aria-label={t("全选{p0}权限", {
                            p0: metadataLabel(g.name),
                          })}
                          onChange={(event) => {
                            toggleGroup(
                              g.items.map((i) => i.key),
                              (event.target as HTMLInputElement).checked,
                            );
                          }}
                        />
                        {t("全选")}
                      </label>
                    </div>
                  ))}
                </>
                <p className="note role-note">
                  <Icon name="Info" size={14}></Icon>
                  {t(
                    "保存后服务端立即按新权限校验，受影响成员需要重新登录；系统会保留至少一名启用的超级管理员。",
                  )}
                </p>
                <footer className="savebar">
                  <span className="savebar-text">
                    {dirty ? <Icon name="Pencil" size={14}></Icon> : null}
                    {role.builtin
                      ? t("内置超级管理员始终拥有全部权限")
                      : dirty
                        ? t("有未保存的更改")
                        : t("权限已保存")}
                    <small>
                      {t("已选 {p0} / {p1} 项", {
                        p0: viewState.draft.length,
                        p1: permissionTotal,
                      })}
                    </small>
                  </span>
                  <button
                    disabled={!dirty}
                    className="btn"
                    onClick={() => {
                      reset();
                    }}
                  >
                    {t("撤销更改")}
                  </button>
                  <button
                    disabled={
                      !dirty ||
                      role.builtin ||
                      viewState.busy ||
                      !can("role:manage")
                    }
                    className="btn primary"
                    onClick={() => {
                      save();
                    }}
                  >
                    <Icon name="Check"></Icon>
                    {t("保存权限")}
                  </button>
                </footer>
              </>
            ) : (
              <>
                <>
                  {members.map((u) => (
                    <div key={u.id} className="member-row">
                      <span className="avatar">{u.name.slice(0, 1)}</span>
                      <span className="cell-main">
                        <strong>{u.name}</strong>
                        <small>
                          {u.username} {roleNames(u)}
                        </small>
                      </span>
                      <StatusBadge
                        status={u.enabled ? "enabled" : "disabled"}
                        label={u.enabled ? t("正常") : t("已停用")}
                      ></StatusBadge>
                      {can("user:manage") ? (
                        <button
                          aria-label={t("编辑成员{p0}", { p0: u.name })}
                          className="link"
                          onClick={() => {
                            openUser(u);
                          }}
                        >
                          {t("编辑")}
                        </button>
                      ) : null}
                    </div>
                  ))}
                </>
                {!members.length ? (
                  <div className="empty">
                    <Icon name="Users" size={28}></Icon>
                    <h3>{t("这个角色还没有成员")}</h3>
                    <p>{t("添加成员时可以同时分配多个角色。")}</p>
                    {can("user:manage") ? (
                      <button
                        className="btn"
                        onClick={() => {
                          openUser(null);
                        }}
                      >
                        <Icon name="UserPlus"></Icon>
                        {t("添加成员")}
                      </button>
                    ) : null}
                  </div>
                ) : null}
              </>
            )}
          </section>
        </div>
      ) : null}
      {viewState.createOpen ? (
        <Modal
          title={t("新建角色")}
          onClose={() => {
            store.setState({ createOpen: false });
          }}
          footer={
            <>
              <button
                className="btn"
                onClick={() => {
                  store.setState({ createOpen: false });
                }}
              >
                {t("取消")}
              </button>
              <button
                form="role-form"
                type="submit"
                disabled={viewState.busy}
                className="btn primary"
              >
                {t("创建角色")}
              </button>
            </>
          }
        >
          <form
            id="role-form"
            className="form"
            onSubmit={(event) => {
              event.preventDefault();
              create();
            }}
          >
            <label className="field">
              {t("角色名称")}
              <input
                required
                maxLength={24}
                placeholder={t("例如：值班工程师")}
                value={viewState.newName}
                onChange={(event) => {
                  store.setState({ newName: event.currentTarget.value });
                }}
              />
            </label>
            <label className="field">
              {t("角色说明")}
              <textarea
                rows={3}
                maxLength={160}
                placeholder={t("这个角色负责什么")}
                value={viewState.newDescription}
                onChange={(event) => {
                  store.setState({ newDescription: event.currentTarget.value });
                }}
              ></textarea>
            </label>
            <p className="note">
              <Icon name="Info" size={14}></Icon>
              {t("创建后在权限列表中勾选需要的功能。")}
            </p>
            {viewState.error ? (
              <p role="alert" className="form-error">
                {viewState.error}
              </p>
            ) : null}
          </form>
        </Modal>
      ) : null}
      {viewState.sensitiveConfirm && role ? (
        <Modal
          title={t("确认授予敏感权限")}
          subtitle={t("「{p0}」的 {p1} 位成员将获得以下权限", {
            p0: role.name,
            p1: role.members,
          })}
          onClose={() => {
            store.setState({ sensitiveConfirm: false });
          }}
          footer={
            <>
              <button
                className="btn"
                onClick={() => {
                  store.setState({ sensitiveConfirm: false });
                }}
              >
                {t("返回检查")}
              </button>
              <button
                disabled={viewState.busy}
                className="btn primary"
                onClick={() => {
                  commit();
                }}
              >
                {t("确认并保存")}
              </button>
            </>
          }
        >
          <ul className="confirm-list">
            <>
              {addedSensitive.map((key) => (
                <li key={key}>
                  <Icon name="LockKeyhole" size={14}></Icon>
                  {metadataLabel(
                    appState.data.permissions
                      .flatMap((g) => g.items)
                      .find((i) => i.key === key)?.label,
                  )}
                </li>
              ))}
            </>
          </ul>
          <p>
            {t(
              "这些权限涉及账号、授权、密钥或系统配置，请确认这些成员确实需要。",
            )}
          </p>
        </Modal>
      ) : null}
      {viewState.pendingRole ? (
        <Modal
          title={t("放弃未保存的更改？")}
          onClose={() => {
            store.setState({ pendingRole: "" });
          }}
          footer={
            <>
              <button
                className="btn"
                onClick={() => {
                  store.setState({ pendingRole: "" });
                }}
              >
                {t("继续编辑")}
              </button>
              <button
                className="btn danger"
                onClick={() => {
                  store.setState({
                    roleId: viewState.pendingRole,
                    pendingRole: "",
                  });
                }}
              >
                {t("放弃并切换")}
              </button>
            </>
          }
        >
          <p>{t("切换角色会丢弃这次未保存的勾选。")}</p>
        </Modal>
      ) : null}
      {viewState.userOpen ? (
        <Modal
          title={viewState.editing ? t("编辑成员") : t("添加成员")}
          onClose={() => {
            store.setState({ userOpen: false });
          }}
          footer={
            <>
              <button
                className="btn"
                onClick={() => {
                  store.setState({ userOpen: false });
                }}
              >
                {t("取消")}
              </button>
              <button
                form="user-form"
                type="submit"
                disabled={viewState.busy}
                className="btn primary"
              >
                {t("保存成员")}
              </button>
            </>
          }
        >
          <form
            id="user-form"
            className="form"
            onSubmit={(event) => {
              event.preventDefault();
              saveUser();
            }}
          >
            <div className="form-grid">
              <label className="field">
                {t("用户名")}
                <input
                  disabled={!!viewState.editing}
                  required
                  autoComplete="off"
                  value={viewState.username}
                  onChange={(event) => {
                    store.setState({ username: event.currentTarget.value });
                  }}
                />
              </label>
              <label className="field">
                {t("姓名")}
                <input
                  required
                  maxLength={26}
                  value={viewState.name}
                  onChange={(event) => {
                    store.setState({ name: event.currentTarget.value });
                  }}
                />
              </label>
            </div>
            <label className="field">
              {viewState.editing ? t("新密码") : t("登录密码")}
              <input
                type="password"
                required={!viewState.editing}
                minLength={12}
                autoComplete="new-password"
                placeholder={viewState.editing ? t("留空则保持原密码") : ""}
                value={viewState.password}
                onChange={(event) => {
                  store.setState({ password: event.currentTarget.value });
                }}
              />
              <small>{t("12–72 字节，请通过可信渠道告知成员。")}</small>
            </label>
            <fieldset className="field">
              <legend className="field-label">{t("角色")}</legend>
              <div className="choice-group">
                <>
                  {appState.data.roles.map((r) => (
                    <label key={r.id} className="choice">
                      <input
                        type="checkbox"
                        value={r.id}
                        checked={viewState.userRoles.includes(r.id)}
                        onChange={(event) => {
                          store.setState({
                            userRoles: event.currentTarget.checked
                              ? [...viewState.userRoles, r.id]
                              : viewState.userRoles.filter(
                                  (selectedValue) => selectedValue !== r.id,
                                ),
                          });
                        }}
                      />
                      {r.name}
                    </label>
                  ))}
                </>
              </div>
            </fieldset>
            {viewState.editing ? (
              <label className="check">
                <input
                  type="checkbox"
                  checked={viewState.enabled}
                  onChange={(event) => {
                    store.setState({ enabled: event.currentTarget.checked });
                  }}
                />
                {t("账号可以登录")}
              </label>
            ) : null}
            {viewState.userError ? (
              <p role="alert" className="form-error">
                {viewState.userError}
              </p>
            ) : null}
          </form>
        </Modal>
      ) : null}
    </>
  );
}
