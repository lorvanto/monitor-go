import { locales, setLocale, t, useI18n } from "../i18n";
export default function LanguageSelect() {
  const i18nState = useI18n();

  return (
    <>
      <label className="language-select">
        <span>{t("语言")}</span>
        <select
          value={i18nState.locale}
          disabled={i18nState.changingLocale}
          aria-label={t("界面语言")}
          title={t("语言仅影响当前浏览器的显示，不会修改团队数据。")}
          onChange={(event) => {
            setLocale((event.target as HTMLSelectElement).value);
          }}
        >
          <>
            {locales.map((item) => (
              <option key={item.code} value={item.code} lang={item.code}>
                {item.label}
              </option>
            ))}
          </>
        </select>
      </label>
      {i18nState.localeLoadFailed ? (
        <p role="alert" className="form-error">
          {t("请求失败")}{" "}
          <button
            type="button"
            className="link"
            onClick={() => {
              setLocale(i18nState.requestedLocale);
            }}
          >
            {t("重试")}
          </button>
        </p>
      ) : null}
    </>
  );
}
