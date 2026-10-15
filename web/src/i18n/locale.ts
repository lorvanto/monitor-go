export const locales = [
  { code: "zh-CN", label: "简体中文" },
  { code: "en-US", label: "English" },
  { code: "ja-JP", label: "日本語" },
  { code: "ko-KR", label: "한국어" },
  { code: "zh-TW", label: "繁體中文" },
] as const;
export type Locale = (typeof locales)[number]["code"];
export const fallbackLocale: Locale = "en-US";
export const localeStorageKey = "monitor.locale";

export function matchLocale(value: string): Locale | undefined {
  const tag = value.trim().replaceAll("_", "-").toLowerCase();
  if (/^zh(?:-|$)/.test(tag))
    return /(?:^|-)(hant|tw|hk|mo)(?:-|$)/.test(tag) ? "zh-TW" : "zh-CN";
  if (/^en(?:-|$)/.test(tag)) return "en-US";
  if (/^ja(?:-|$)/.test(tag)) return "ja-JP";
  if (/^ko(?:-|$)/.test(tag)) return "ko-KR";
}

export function interpolate(
  message: string,
  params: Record<string, unknown>,
): string {
  return message.replace(/\{(\w+)\}/g, (token, key: string) =>
    Object.hasOwn(params, key) ? String(params[key] ?? "") : token,
  );
}
