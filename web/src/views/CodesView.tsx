import { useState } from "react";
import { createStore, useStore } from "zustand";
import Icon from "../components/Icon";
import StatusBadge from "../components/StatusBadge";
import { t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import { api, notify, useApp } from "../stores/app";
interface Result {
  kind: string;
  content: string;
  width: number;
  height: number;
  png: string;
  svg: string;
}
export default function CodesView() {
  const initialState = {
    mode: "qr" as "qr" | "barcode",
    barcodeKind: "code128",
    form: {
      content: "",
      width: 400,
      height: 160,
      level: "M",
      label: true,
    },
    result: undefined as Result | undefined,
    busy: false,
    error: "",
    generatedInput: "",
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const appState = useApp();
  useI18n();
  const getKind = () =>
    store.getState().mode === "qr" ? "qr" : store.getState().barcodeKind;
  const getInput = () => ({ ...store.getState().form, kind: getKind() });
  const getDirty = () =>
    store.getState().generatedInput !== JSON.stringify(getInput());
  const names: Record<string, string> = {
    qr: "QR Code",
    code128: "Code 128",
    code39: "Code 39",
    ean13: "EAN-13",
    ean8: "EAN-8",
  };
  function switchMode(v: "qr" | "barcode") {
    store.setState({ mode: v });
    store.setState((current) => ({ form: { ...current.form, content: "" } }));
    store.setState((current) => ({
      form: { ...current.form, width: v === "qr" ? 400 : 800 },
    }));
    store.setState({ result: undefined });
    store.setState({ error: "" });
  }
  const getHint = () =>
    getKind() === "qr"
      ? t("支持中文、网址和纯文本，最多 1200 字节。")
      : getKind() === "code128"
        ? t("支持可打印 ASCII 字符，最多 80 个字符。")
        : getKind() === "code39"
          ? t("支持大写字母、数字、空格及 - . $ / + %。")
          : getKind() === "ean13"
            ? t("输入 12 位数字自动补齐校验位，或输入 13 位完整编码。")
            : t("输入 7 位数字自动补齐校验位，或输入 8 位完整编码。");
  async function generate() {
    store.setState({ busy: true });
    store.setState({ error: "" });
    const snapshot = JSON.stringify(getInput());
    try {
      store.setState({
        result: await api<Result>("/tools/codes", "POST", JSON.parse(snapshot)),
      });
      store.setState({ generatedInput: snapshot });
    } catch (e) {
      store.setState({ error: (e as Error).message });
    } finally {
      store.setState({ busy: false });
    }
  }
  function example() {
    store.setState((current) => ({
      form: {
        ...current.form,
        content:
          store.getState().mode === "qr"
            ? "https://example.com"
            : getKind() === "code128" || getKind() === "code39"
              ? "MONITOR-GO-2026"
              : getKind() === "ean13"
                ? "690123456789"
                : "9638507",
      },
    }));
    store.setState((current) => ({
      form: {
        ...current.form,
        width: store.getState().mode === "qr" ? 400 : 800,
      },
    }));
  }
  function download(format: "png" | "svg") {
    const result = store.getState().result;
    if (!result || getDirty()) return;
    const raw = atob(result[format]),
      data = Uint8Array.from(raw, (c) => c.charCodeAt(0)),
      url = URL.createObjectURL(
        new Blob([data], {
          type: format === "png" ? "image/png" : "image/svg+xml",
        }),
      );
    const a = document.createElement("a");
    a.href = url;
    a.download = `${result.kind}-${Date.now()}.${format}`;
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10000);
    notify(t("{p0} 已导出", { p0: format.toUpperCase() }));
  }

  const dirty = getDirty();
  const hint = getHint();
  return (
    <>
      <header className="page-head">
        <div className="page-title">
          <h1>{appState.pageTitle}</h1>
          <p>
            {t(
              "在这套实例内生成二维码和条码，不保存输入内容，可导出 PNG 与 SVG。",
            )}
          </p>
        </div>
      </header>
      <nav aria-label={t("编码类型")} className="tabs">
        <button
          aria-current={viewState.mode === "qr" ? "page" : undefined}
          className={classNames("tab", { active: viewState.mode === "qr" })}
          onClick={() => {
            switchMode("qr");
          }}
        >
          <Icon name="QrCode" size={15}></Icon>
          {t("二维码")}
        </button>
        <button
          aria-current={viewState.mode === "barcode" ? "page" : undefined}
          className={classNames("tab", {
            active: viewState.mode === "barcode",
          })}
          onClick={() => {
            switchMode("barcode");
          }}
        >
          <Icon name="Barcode" size={15}></Icon>
          {t("条码")}
        </button>
      </nav>
      <div className="codes">
        <section className="panel">
          <header className="panel-head ruled">
            <h2>{t("内容与尺寸")}</h2>
          </header>
          <form
            className="form panel-body pad"
            onSubmit={(event) => {
              event.preventDefault();
              generate();
            }}
          >
            {viewState.mode === "barcode" ? (
              <label className="field">
                {t("条码制式")}
                <select
                  value={viewState.barcodeKind}
                  onChange={(event) => {
                    store.setState({ barcodeKind: event.currentTarget.value });
                  }}
                >
                  <option value="code128">{t("Code 128，通用文本")}</option>
                  <option value="code39">{t("Code 39，工业标识")}</option>
                  <option value="ean13">{t("EAN-13，13 位商品编码")}</option>
                  <option value="ean8">{t("EAN-8，8 位商品编码")}</option>
                </select>
              </label>
            ) : null}
            <label className="field">
              {viewState.mode === "qr" ? t("编码内容") : t("条码内容")}
              <textarea
                rows={viewState.mode === "qr" ? 6 : 3}
                required
                maxLength={viewState.mode === "qr" ? 1200 : 80}
                placeholder={
                  viewState.mode === "qr"
                    ? t("网址、文本或其他内容")
                    : t("字母、数字或商品编码")
                }
                className={classNames({ mono: viewState.mode === "barcode" })}
                value={viewState.form.content}
                onChange={(event) => {
                  store.setState((current) => ({
                    form: {
                      ...current.form,
                      content: event.currentTarget.value,
                    },
                  }));
                }}
              ></textarea>
              <small>{hint}</small>
            </label>
            <button
              type="button"
              className="link self-start"
              onClick={() => {
                example();
              }}
            >
              {t("填入示例")}
            </button>
            <div className="form-grid">
              <label className="field">
                {t("{p0}（像素）", {
                  p0: viewState.mode === "qr" ? t("边长") : t("宽度"),
                })}
                <input
                  type="number"
                  min="128"
                  max="2048"
                  required
                  value={viewState.form.width}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        width: Number(event.currentTarget.value),
                      },
                    }));
                  }}
                />
              </label>
              {viewState.mode === "qr" ? (
                <label className="field">
                  {t("容错等级")}
                  <select
                    value={viewState.form.level}
                    onChange={(event) => {
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          level: event.currentTarget.value,
                        },
                      }));
                    }}
                  >
                    <option value="L">{t("L，约 7%")}</option>
                    <option value="M">{t("M，约 15%")}</option>
                    <option value="Q">{t("Q，约 25%")}</option>
                    <option value="H">{t("H，约 30%")}</option>
                  </select>
                </label>
              ) : (
                <label className="field">
                  {t("高度（像素）")}
                  <input
                    type="number"
                    min="80"
                    max="512"
                    required
                    value={viewState.form.height}
                    onChange={(event) => {
                      store.setState((current) => ({
                        form: {
                          ...current.form,
                          height: Number(event.currentTarget.value),
                        },
                      }));
                    }}
                  />
                </label>
              )}
            </div>
            {viewState.mode === "barcode" ? (
              <label className="check">
                <input
                  type="checkbox"
                  checked={viewState.form.label}
                  onChange={(event) => {
                    store.setState((current) => ({
                      form: {
                        ...current.form,
                        label: event.currentTarget.checked,
                      },
                    }));
                  }}
                />
                {t("在条码下方显示文字")}
              </label>
            ) : null}
            {viewState.error ? (
              <p role="alert" className="form-error">
                {viewState.error}
              </p>
            ) : null}
            <button disabled={viewState.busy} className="btn primary">
              <Icon
                name={viewState.busy ? "RefreshCw" : "Zap"}
                className={classNames({ spin: viewState.busy })}
              ></Icon>
              {viewState.busy
                ? t("正在生成…")
                : t("生成{p0}", {
                    p0: viewState.mode === "qr" ? t("二维码") : t("条码"),
                  })}
            </button>
          </form>
        </section>
        <section className="panel">
          <header className="panel-head">
            <div>
              <h2>{t("预览")}</h2>
              <p>
                {viewState.result
                  ? t("{p0}，{p1} × {p2} 像素", {
                      p0: names[viewState.result.kind],
                      p1: viewState.result.width,
                      p2: viewState.result.height,
                    })
                  : t("生成后在这里预览并导出")}
              </p>
            </div>
            {viewState.result ? (
              <StatusBadge
                tone={dirty ? "warn" : "ok"}
                label={dirty ? t("内容已修改，需重新生成") : t("已生成")}
              ></StatusBadge>
            ) : null}
          </header>
          <div
            className={classNames("code-stage", {
              stale: viewState.result && dirty,
            })}
          >
            {viewState.result ? (
              <img
                src={"data:image/png;base64," + viewState.result.png}
                alt={
                  viewState.result.kind === "qr"
                    ? t("生成的二维码")
                    : t("生成的条码")
                }
                width={viewState.result.width}
                height={viewState.result.height}
              />
            ) : (
              <div className="code-placeholder">
                <Icon
                  name={viewState.mode === "qr" ? "QrCode" : "Barcode"}
                  size={56}
                ></Icon>
                <span>{t("填写左侧内容后点击生成")}</span>
              </div>
            )}
          </div>
          <div className="code-foot">
            <p>
              {viewState.result &&
              (viewState.result.kind === "ean13" ||
                viewState.result.kind === "ean8") ? (
                <>
                  {t("完整编码")}
                  <span className="mono">{viewState.result.content}</span>
                  {"。"}
                </>
              ) : null}
              {t("图形四周保留了静区，导出后请不要裁切边缘。")}
            </p>
            <div className="row-actions tight">
              <button
                disabled={!viewState.result || dirty || viewState.busy}
                className="btn"
                onClick={() => {
                  download("svg");
                }}
              >
                <Icon name="Download"></Icon>
                {t("导出 SVG")}
              </button>
              <button
                disabled={!viewState.result || dirty || viewState.busy}
                className="btn primary"
                onClick={() => {
                  download("png");
                }}
              >
                <Icon name="Download"></Icon>
                {t("导出 PNG")}
              </button>
            </div>
          </div>
        </section>
      </div>
      <div className="code-notes">
        <div>
          <strong>{t("格式选择")}</strong>
          <p>
            {t(
              "PNG 适合分享和放进文档，SVG 适合印刷和任意缩放。正式使用前请用实际设备试扫。",
            )}
          </p>
        </div>
        <div>
          <strong>
            {viewState.mode === "qr"
              ? t("内容越长图形越密")
              : t("条码不等于商品注册")}
          </strong>
          <p>
            {viewState.mode === "qr"
              ? t(
                  "内容越长、容错越高，需要的尺寸越大；尺寸不足时会提示最小宽度。",
                )
              : t("EAN 会自动计算或校验校验位；商品编号需要由使用方合法取得。")}
          </p>
        </div>
        <div>
          <strong>{t("保持对比度")}</strong>
          <p>{t("导出的图形为黑白配色，请勿改成低对比度颜色或非等比拉伸。")}</p>
        </div>
      </div>
    </>
  );
}
