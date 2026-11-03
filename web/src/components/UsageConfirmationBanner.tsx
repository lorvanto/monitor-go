import { t, useI18n } from "../i18n";
import { appStore, navigate, useApp } from "../stores/app";
import StatusGlyph from "./StatusGlyph";

export default function UsageConfirmationBanner() {
  useI18n();
  const { required, writable } = useApp(
    ({ usageConfirmationRequired, user }) => ({
      required: usageConfirmationRequired,
      writable: !!user?.permissions.includes("system:settings"),
    }),
  );
  if (!required) return null;
  return (
    <section
      className="banner usage-confirmation-banner"
      data-tone="warn"
      role="status"
    >
      <StatusGlyph tone="warn" />
      <div className="banner-text">
        <strong>{t("待管理员确认持续上报规则")}</strong>
        <p>
          {t(
            "此实例尚未确认当前上报规则。请管理员查看目的地和字段后确认；监控和告警继续运行。",
          )}
        </p>
      </div>
      {writable ? (
        <button
          type="button"
          className="btn"
          onClick={() => {
            appStore.setState({ usageSettingsRequested: true });
            navigate("settings");
          }}
        >
          {t("查看上报规则")}
        </button>
      ) : (
        <p>{t("请联系有系统设置权限的管理员完成确认。")}</p>
      )}
    </section>
  );
}
