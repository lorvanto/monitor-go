import { create } from "zustand";
import enUS from "../../../locales/en-US.json";
import type zhCN from "../../../locales/zh-CN.json";
import {
  fallbackLocale,
  interpolate,
  localeStorageKey,
  matchLocale,
  type Locale,
} from "./locale";
export { locales, type Locale } from "./locale";

// Source messages are stable gettext-style IDs. Catalogs never contain HTML.
export type MessageKey = keyof typeof zhCN;
export function isMessageKey(value: unknown): value is MessageKey {
  return typeof value === "string" && Object.hasOwn(enUS, value);
}
type Catalog = Record<MessageKey, string>;
const catalogs: Partial<Record<Locale, Catalog>> = { "en-US": enUS };
const loaders = {
  "zh-CN": () => import("../../../locales/zh-CN.json"),
  "en-US": () => Promise.resolve({ default: enUS }),
  "ja-JP": () => import("../../../locales/ja-JP.json"),
  "ko-KR": () => import("../../../locales/ko-KR.json"),
  "zh-TW": () => import("../../../locales/zh-TW.json"),
};
function initialLocale(): Locale {
  try {
    const stored = localStorage.getItem(localeStorageKey);
    if (stored && matchLocale(stored)) return matchLocale(stored)!;
  } catch {
    /* Private browsing may disable storage. */
  }
  return fallbackLocale;
}
const startupLocale = initialLocale();
interface LocaleState {
  locale: Locale;
  requestedLocale: Locale;
  changingLocale: boolean;
  localeLoadFailed: boolean;
}
export const useI18n = create<LocaleState>(() => ({
  locale: fallbackLocale,
  requestedLocale: startupLocale,
  changingLocale: false,
  localeLoadFailed: false,
}));
export const i18nStore = useI18n;
document.documentElement.lang = fallbackLocale;
let selection = 0;
export async function setLocale(value: string) {
  const matched = matchLocale(value);
  if (!matched) return;
  i18nStore.setState({ requestedLocale: matched });
  const current = ++selection;
  i18nStore.setState({ changingLocale: true });
  i18nStore.setState({ localeLoadFailed: false });
  try {
    catalogs[matched] ||= (await loaders[matched]()).default;
    if (current !== selection) return;
    i18nStore.setState({ locale: matched });
    document.documentElement.lang = matched;
    try {
      localStorage.setItem(localeStorageKey, matched);
    } catch {
      /* Session-only preference. */
    }
  } catch {
    if (current === selection) i18nStore.setState({ localeLoadFailed: true });
  } finally {
    if (current === selection) i18nStore.setState({ changingLocale: false });
  }
}
export const initializeLocale = () => setLocale(startupLocale);

export function t(
  key: MessageKey,
  params: Record<string, unknown> = {},
): string {
  return interpolate(
    catalogs[i18nStore.getState().locale]?.[key] || enUS[key] || key,
    params,
  );
}

// Use only for trusted, fixed server metadata, never names or user-authored content.
export function metadataLabel(source: string | undefined): string {
  return source && Object.hasOwn(enUS, source)
    ? t(source as MessageKey)
    : source || "";
}

// Lazy label maps stay reactive without recreating forms or mutating their values.
export function localizedLabels<T extends Record<string, string>>(
  source: T,
): T {
  return new Proxy(source, {
    get(target, key, receiver) {
      const value = Reflect.get(target, key, receiver);
      return typeof value === "string" ? metadataLabel(value) : value;
    },
  });
}
