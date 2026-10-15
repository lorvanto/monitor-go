import { resourceName } from "../lib/resourceLabels";
import { useEffect, useState } from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import Modal from "../components/Modal";
import StatusBadge from "../components/StatusBadge";
import { t, useI18n } from "../i18n";
import {
  api,
  bytes,
  can,
  notify,
  refreshNavigation,
  useApp,
} from "../stores/app";
interface Settings {
  rawRetentionDays: number;
  summaryRetentionDays: number;
  instanceName: string;
  useDefaultName: boolean;
  uploadLimitMB: number;
  storageLimitMB: number;
  defaultFolder: string;
  auditDays: number;
  hostCPUPercent: number;
  hostMemoryPercent: number;
  hostDiskPercent: number;
  hostAlertMinutes: number;
  version: number;
}
interface Info {
  version: string;
  goVersion: string;
  platform: string;
  database: string;
  schema: number;
  databaseSize: number;
  files: number;
  folders: number;
  storageUsed: number;
  pendingGC: number;
}
export default function SettingsView() {
  const initialState = {
    form: {
      instanceName: "",
      useDefaultName: true,
      uploadLimitMB: 32,
      storageLimitMB: 1024,
      defaultFolder: "2",
      auditDays: 90,
      rawRetentionDays: 7,
      summaryRetentionDays: 90,
      hostCPUPercent: 90,
      hostMemoryPercent: 90,
      hostDiskPercent: 85,
      hostAlertMinutes: 5,
      version: 0,
    } as Settings,
    info: undefined as Info | undefined,
    folders: [] as {
      id: string;
      name: string;
      parentId: string;
      nameKey?: string;
    }[],
    error: "",
    busy: false,
    clear: false,
    loading: true,
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const appState = useApp();
  useI18n();
  async function load() {
    try {
      const [settings, system] = await Promise.all([
        api<Settings>("/settings"),
        api<Info>("/system"),
      ]);
      store.setState({ form: { ...store.getState().form, ...settings } });
      store.setState({ info: system });
      if (can("file:access")) {
        store.setState({
          folders: (
            await api<{
              items: LocalState["folders"];
            }>("/files/tree")
          ).items,
        });
      }
      store.setState({ error: "" });
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ loading: false });
    }
  }
  function folderName(id: string) {
    const parts: string[] = [];
    for (let i = 0; id && i < 18; i++) {
      const f = store.getState().folders.find((v) => v.id === id);
      if (!f) break;
      parts.unshift(resourceName(f));
      id = f.parentId;
    }
    return parts.join(" / ");
  }
  async function save() {
    store.setState({ busy: true });
    try {
      await api("/settings", "PATCH", store.getState().form);
      await load();
      await refreshNavigation();
      notify(t("系统设置已保存"));
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  async function clearCache() {
    store.setState({ busy: true });
    try {
      const v = await api<{
        removed: number;
      }>("/files/cache/clear", "POST", {});
      store.setState({ clear: false });
      notify(t("已清理 {p0} 个缓存文件", { p0: v.removed }));
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  useEffect(() => {
    load();
  }, []);

  return (
    <>
      <header className="page-head">
        <div className="page-title">
          <h1>{appState.pageTitle}</h1>
          <p>{t("这些设置对整套实例生效，修改会记录到操作审计。")}</p>
        </div>
        <div className="page-actions">
          <button
            disabled={viewState.busy}
            className="btn"
            onClick={() => {
              load();
            }}
          >
            <Icon name="RefreshCw"></Icon>
            {t("重新读取")}
          </button>
          {can("system:settings") ? (
            <button
              form="system-settings"
              disabled={viewState.busy || viewState.loading}
              className="btn primary"
            >
              <Icon name="Check"></Icon>
              {viewState.busy ? t("正在保存…") : t("保存设置")}
            </button>
          ) : null}
        </div>
      </header>
      {viewState.error ? (
        <div data-tone="crit" role="alert" className="banner">
          <Icon name="TriangleAlert" size={15}></Icon>
          <div className="banner-text">
            <p>{viewState.error}</p>
          </div>
        </div>
      ) : null}
      <div className="settings">
        <form
          id="system-settings"
          className="settings-main"
          onSubmit={(event) => {
            event.preventDefault();
            save();
          }}
        >
          <fieldset
            disabled={
              !can("system:settings") || viewState.busy || viewState.loading
            }
            className="panel"
          >
            <section className="settings-section">
              <div>
                <h2>{t("实例名称")}</h2>
                <p>
                  {t("显示在侧栏顶部和浏览器标签中，便于团队区分不同的安装。")}
                </p>
              </div>
              <div className="form">
                <label className="check">
                  <input
                    type="checkbox"
                    checked={viewState.form.useDefaultName}
                    onChange={(event) =>
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          useDefaultName: event.currentTarget.checked,
                        },
                      }))
                    }
                  />
                  {t("使用默认实例名称（随界面语言切换）")}
                </label>
                <label className="field">
                  {t("名称")}
                  <input
                    required={!viewState.form.useDefaultName}
                    disabled={viewState.form.useDefaultName}
                    maxLength={40}
                    placeholder={t("例如：研发运维中心")}
                    value={
                      viewState.form.useDefaultName
                        ? t("我的监控中心")
                        : viewState.form.instanceName
                    }
                    onChange={(event) => {
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          instanceName: event.currentTarget.value,
                        },
                      }));
                    }}
                  />
                </label>
              </div>
            </section>
            <section className="settings-section">
              <div>
                <h2>{t("文件与存储")}</h2>
                <p>
                  {t("原文件保存在实例数据目录中，回收站里的文件也计入配额。")}
                </p>
              </div>
              <div className="form">
                <div className="form-grid">
                  <label className="field">
                    {t("单文件上限（MB）")}
                    <input
                      type="number"
                      min="1"
                      max="1024"
                      required
                      value={viewState.form.uploadLimitMB}
                      onChange={(event) => {
                        store.setState((current) => ({
                          form: {
                            ...current.form,
                            uploadLimitMB: Number(event.currentTarget.value),
                          },
                        }));
                      }}
                    />
                    <small>{"1–1024 MB"}</small>
                  </label>
                  <label className="field">
                    {t("存储配额（MB）")}
                    <input
                      type="number"
                      min="10"
                      max="1048576"
                      required
                      value={viewState.form.storageLimitMB}
                      onChange={(event) => {
                        store.setState((current) => ({
                          form: {
                            ...current.form,
                            storageLimitMB: Number(event.currentTarget.value),
                          },
                        }));
                      }}
                    />
                    <small>{t("至少 10 MB")}</small>
                  </label>
                </div>
                <label className="field">
                  {t("默认上传目录")}
                  <select
                    disabled={!viewState.folders.length}
                    value={viewState.form.defaultFolder}
                    onChange={(event) => {
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          defaultFolder: event.currentTarget.value,
                        },
                      }));
                    }}
                  >
                    {!viewState.folders.length ? (
                      <option value={viewState.form.defaultFolder}>
                        {t("目录 #{p0}（需要文件权限才能选择）", {
                          p0: viewState.form.defaultFolder,
                        })}
                      </option>
                    ) : null}
                    <>
                      {viewState.folders.map((f) => (
                        <option key={f.id} value={f.id}>
                          {folderName(f.id)}
                        </option>
                      ))}
                    </>
                  </select>
                  <small>
                    {t(
                      "没有指定目录的上传会保存到这里；在文件页面上传时使用当前目录。",
                    )}
                  </small>
                </label>
              </div>
            </section>
            <section className="settings-section">
              <div>
                <h2>{t("主机告警阈值")}</h2>
                <p>
                  {t(
                    "阈值为 0 时关闭该项告警。CPU 和内存需持续超限，磁盘按最新采样判断。",
                  )}
                </p>
                <p>
                  {t(
                    "采样需覆盖整个持续时长，间隔超过 45 秒或缺失数据时不新建持续超限告警；最新有效值低于阈值时恢复。",
                  )}
                </p>
              </div>
              <div className="form">
                {(
                  [
                    ["hostCPUPercent", "CPU 阈值（%）", 0, 100],
                    ["hostMemoryPercent", "内存阈值（%）", 0, 100],
                    ["hostDiskPercent", "磁盘阈值（%）", 0, 100],
                    ["hostAlertMinutes", "持续时长（分钟）", 1, 60],
                  ] as const
                ).map(([key, label, min, max]) => (
                  <label className="field" key={key}>
                    {t(label)}
                    <input
                      type="number"
                      required
                      min={min}
                      max={max}
                      step={1}
                      value={viewState.form[key]}
                      onChange={(event) =>
                        store.setState((current) => ({
                          form: {
                            ...current.form,
                            [key]: Number(event.currentTarget.value),
                          },
                        }))
                      }
                    />
                  </label>
                ))}
              </div>
            </section>
            <section className="settings-section">
              <div>
                <h2>{t("指标保留策略")}</h2>
                <p>
                  {t(
                    "每小时先汇总已结束时段，再清理过期原始指标。缩短保留期会删除更早的数据。",
                  )}
                </p>
              </div>
              <div className="form-grid">
                <label className="field">
                  {t("原始指标保留天数")}
                  <input
                    type="number"
                    min="1"
                    max="30"
                    required
                    value={viewState.form.rawRetentionDays}
                    onChange={(event) =>
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          rawRetentionDays: Number(event.currentTarget.value),
                        },
                      }))
                    }
                  />
                  <small>{t("1–30 天")}</small>
                </label>
                <label className="field">
                  {t("小时汇总保留天数")}
                  <input
                    type="number"
                    min="90"
                    max="730"
                    required
                    value={viewState.form.summaryRetentionDays}
                    onChange={(event) =>
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          summaryRetentionDays: Number(
                            event.currentTarget.value,
                          ),
                        },
                      }))
                    }
                  />
                  <small>{t("90–730 天")}</small>
                </label>
              </div>
            </section>
            <section className="settings-section">
              <div>
                <h2>{t("操作审计")}</h2>
                <p>
                  {t(
                    "系统每小时清理一次过期记录。缩短保留时间会删除更早的审计记录。",
                  )}
                </p>
              </div>
              <label className="field">
                {t("保留天数")}
                <input
                  type="number"
                  min="7"
                  max="365"
                  required
                  value={viewState.form.auditDays}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        auditDays: Number(event.currentTarget.value),
                      },
                    }));
                  }}
                />
                <small>{t("7–365 天")}</small>
              </label>
            </section>
          </fieldset>
          {!can("system:settings") ? (
            <p className="note">
              <Icon name="LockKeyhole" size={14}></Icon>
              {t("当前账号只能查看设置，修改需要“修改系统设置”权限。")}
            </p>
          ) : null}
        </form>
        <aside className="settings-side">
          <section className="panel">
            <header className="panel-head ruled">
              <h2>{t("运行信息")}</h2>
              {viewState.info && !viewState.error ? (
                <StatusBadge status="healthy" label={t("运行中")}></StatusBadge>
              ) : null}
            </header>
            {viewState.info ? (
              <dl className="kv rows panel-body">
                <div>
                  <dt>{t("应用版本")}</dt>
                  <dd>{viewState.info.version}</dd>
                </div>
                <div>
                  <dt>{t("运行环境")}</dt>
                  <dd>{viewState.info.goVersion}</dd>
                </div>
                <div>
                  <dt>{t("操作系统")}</dt>
                  <dd>{viewState.info.platform}</dd>
                </div>
                <div>
                  <dt>{t("数据库")}</dt>
                  <dd>
                    {t("{p0}，结构版本 {p1}", {
                      p0: viewState.info.database,
                      p1: viewState.info.schema,
                    })}
                  </dd>
                </div>
                <div>
                  <dt>{t("数据库占用")}</dt>
                  <dd className="num">{bytes(viewState.info.databaseSize)}</dd>
                </div>
                <div>
                  <dt>{t("原文件占用")}</dt>
                  <dd className="num">{bytes(viewState.info.storageUsed)}</dd>
                </div>
                <div>
                  <dt>{t("目录与文件")}</dt>
                  <dd className="num">
                    {t("{p0} 个目录，{p1} 个文件", {
                      p0: viewState.info.folders,
                      p1: viewState.info.files,
                    })}
                  </dd>
                </div>
                <div>
                  <dt>{t("待回收对象")}</dt>
                  <dd className="num">{viewState.info.pendingGC}</dd>
                </div>
              </dl>
            ) : (
              <p className="empty compact">
                {viewState.loading ? t("正在读取…") : t("暂无信息")}
              </p>
            )}
          </section>
          <section className="panel">
            <header className="panel-head">
              <div>
                <h2>{t("图片缓存")}</h2>
                <p>
                  {t(
                    "缩略图和样式图的副本，上限 128 MB。清理后会在下次预览时重新生成，不影响原文件。",
                  )}
                </p>
              </div>
            </header>
            {can("system:settings") ? (
              <div className="panel-body">
                <button
                  type="button"
                  className="btn"
                  onClick={() => {
                    store.setState({ clear: true });
                  }}
                >
                  <Icon name="Trash2"></Icon>
                  {t("清理图片缓存")}
                </button>
              </div>
            ) : null}
          </section>
          <p className="note">
            <Icon name="Info" size={14}></Icon>
            {t(
              "备份前请先停止服务，再完整复制数据目录，包括 SQLite 文件、加密主密钥和 uploads 原文件目录。",
            )}
          </p>
        </aside>
      </div>
      {viewState.clear ? (
        <Modal
          title={t("清理图片缓存")}
          onClose={() => {
            !viewState.busy && store.setState({ clear: false });
          }}
          footer={
            <>
              <button
                disabled={viewState.busy}
                className="btn"
                onClick={() => {
                  store.setState({ clear: false });
                }}
              >
                {t("取消")}
              </button>
              <button
                disabled={viewState.busy}
                className="btn primary"
                onClick={() => {
                  clearCache();
                }}
              >
                {t("清理缓存")}
              </button>
            </>
          }
        >
          <p>
            {t(
              "已生成的缩略图和样式图会被删除，原文件保留；下次预览时会重新生成。",
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
