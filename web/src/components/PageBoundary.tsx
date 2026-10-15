import { Component, type ReactNode } from "react";
import { t, useI18n } from "../i18n";

class Boundary extends Component<
  { children: ReactNode; fallback: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}

// A failed lazy import is cached by React; a user-initiated reload gets the current asset manifest.
export default function PageBoundary({ children }: { children: ReactNode }) {
  useI18n();
  return (
    <Boundary
      fallback={
        <section className="panel forbidden" role="alert">
          <div className="empty">
            <h3>{t("页面加载失败")}</h3>
            <p>{t("请刷新页面后重试。未保存的修改将丢失。")}</p>
            <button
              className="btn primary"
              onClick={() => window.location.reload()}
            >
              {t("刷新页面")}
            </button>
          </div>
        </section>
      }
    >
      {children}
    </Boundary>
  );
}
