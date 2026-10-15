import { Fragment, useEffect, useRef, useState } from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import { t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import { api, dateTime, useApp } from "../stores/app";
interface Entry {
  id: string;
  at: number;
  actor: string;
  action: string;
  resource: string;
  requestId: string;
  method: string;
  path: string;
}
export default function AuditView() {
  const initialState = {
    items: [] as Entry[],
    query: "",
    from: "",
    to: "",
    more: false,
    loading: false,
    error: "",
    expanded: "",
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const sequence = useRef(0);
  const applied = useRef(new URLSearchParams());
  const appState = useApp();
  useI18n();
  async function load(append = false) {
    if (append && store.getState().loading) return;
    const request = ++sequence.current;
    store.setState({ loading: true });
    store.setState({ error: "" });
    try {
      const params = append
        ? new URLSearchParams(applied.current)
        : new URLSearchParams({ q: store.getState().query });
      if (!append && store.getState().from)
        params.set(
          "from",
          String(new Date(store.getState().from + "T00:00:00").getTime()),
        );
      if (!append && store.getState().to)
        params.set(
          "to",
          String(new Date(store.getState().to + "T23:59:59.999").getTime()),
        );
      if (!append) {
        if (params.toString() !== applied.current.toString())
          store.setState({ items: [], more: false, expanded: "" });
        applied.current = new URLSearchParams(params);
      }
      if (append && store.getState().items.length)
        params.set("before", store.getState().items.at(-1)!.id);
      const result = await api<{
        items: Entry[];
        hasMore: boolean;
      }>("/audit?" + params);
      if (request !== sequence.current) return;
      store.setState({
        items: append
          ? [...store.getState().items, ...result.items]
          : result.items,
      });
      store.setState({ more: result.hasMore });
    } catch (e) {
      if (request === sequence.current)
        store.setState({ error: (e as Error).message });
    } finally {
      if (request === sequence.current) store.setState({ loading: false });
    }
  }
  useEffect(() => {
    load();
    return () => {
      sequence.current++;
    };
  }, []);

  return (
    <>
      <header className="page-head">
        <div className="page-title">
          <h1>{appState.pageTitle}</h1>
          <p>
            {t(
              "记录成功的管理变更，不保存密码、密钥或文件内容；保留时长在系统设置中调整。",
            )}
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
            <Icon
              name="RefreshCw"
              className={classNames({ spin: viewState.loading })}
            ></Icon>
            {t("刷新")}
          </button>
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
      <section className="panel">
        <form
          className="toolbar audit-filter"
          onSubmit={(event) => {
            event.preventDefault();
            load();
          }}
        >
          <label className="search">
            <Icon name="Search" size={15}></Icon>
            <input
              placeholder={t("成员、操作、资源或请求编号")}
              aria-label={t("搜索审计记录")}
              maxLength={120}
              value={viewState.query}
              onChange={(event) => {
                store.setState({ query: event.currentTarget.value });
              }}
            />
          </label>
          <label>
            {t("从")}
            <input
              type="date"
              aria-label={t("开始日期")}
              value={viewState.from}
              onChange={(event) => {
                store.setState({ from: event.currentTarget.value });
              }}
            />
          </label>
          <label>
            {t("至")}
            <input
              type="date"
              aria-label={t("结束日期")}
              value={viewState.to}
              onChange={(event) => {
                store.setState({ to: event.currentTarget.value });
              }}
            />
          </label>
          <button disabled={viewState.loading} className="btn primary">
            {t("查询")}
          </button>
          {viewState.query || viewState.from || viewState.to ? (
            <button
              type="button"
              className="link"
              onClick={() => {
                store.setState({ query: "", from: "", to: "" });
                load();
              }}
            >
              {t("清除条件")}
            </button>
          ) : null}
        </form>
        {viewState.items.length ? (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>{t("时间")}</th>
                  <th>{t("成员")}</th>
                  <th>{t("操作")}</th>
                  <th>{t("资源")}</th>
                  <th className="actions">
                    <span className="sr-only">{t("请求详情")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                <>
                  {viewState.items.map((item) => (
                    <Fragment key={item.id}>
                      <tr
                        className={classNames({
                          selected: viewState.expanded === item.id,
                        })}
                      >
                        <td className="tnum nowrap muted">
                          {dateTime(item.at)}
                        </td>
                        <td>
                          <span className="person">
                            <span className="avatar sm">
                              {item.actor.slice(0, 1)}
                            </span>
                            {item.actor}
                          </span>
                        </td>
                        <td>
                          <strong>{item.action}</strong>
                        </td>
                        <td className="audit-resource">
                          <span title={item.resource} className="ellipsis">
                            {item.resource || "—"}
                          </span>
                        </td>
                        <td className="actions">
                          <button
                            aria-expanded={viewState.expanded === item.id}
                            className="link"
                            onClick={() => {
                              store.setState({
                                expanded:
                                  viewState.expanded === item.id ? "" : item.id,
                              });
                            }}
                          >
                            {viewState.expanded === item.id
                              ? t("收起")
                              : t("请求详情")}
                          </button>
                        </td>
                      </tr>
                      {viewState.expanded === item.id ? (
                        <tr className="audit-detail">
                          <td colSpan={5}>
                            <dl>
                              <dt>{t("请求编号")}</dt>
                              <dd>{item.requestId || t("早期记录未采集")}</dd>
                              <dt>{t("接口")}</dt>
                              <dd>
                                {item.method} {item.path || "—"}
                              </dd>
                            </dl>
                          </td>
                        </tr>
                      ) : null}
                    </Fragment>
                  ))}
                </>
              </tbody>
            </table>
          </div>
        ) : (
          <div className="empty">
            <Icon name="ScrollText" size={28}></Icon>
            <h3>
              {viewState.loading ? t("正在读取记录…") : t("没有匹配的操作记录")}
            </h3>
            {!viewState.loading ? (
              <p>
                {t("菜单、文件、设置、授权和监控相关的管理操作会记录在这里。")}
              </p>
            ) : null}
          </div>
        )}
        <footer className="panel-foot">
          <span>
            {t("已显示 {p0} 条，最新的在前", { p0: viewState.items.length })}
          </span>
          {viewState.more ? (
            <button
              disabled={viewState.loading}
              className="btn sm"
              onClick={() => {
                load(true);
              }}
            >
              {viewState.loading ? t("正在加载…") : t("加载更多")}
            </button>
          ) : viewState.items.length ? (
            <span>{t("已显示全部匹配记录")}</span>
          ) : null}
        </footer>
      </section>
    </>
  );
}
