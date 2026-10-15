import { i18nStore, t } from "../i18n";

export function dateTime(at: number | null | undefined) {
  return at
    ? new Intl.DateTimeFormat(i18nStore.getState().locale, {
        dateStyle: "medium",
        timeStyle: "medium",
        hour12: false,
      }).format(at)
    : "—";
}

export function relative(at: number) {
  if (!at) return t("尚未检测");
  const seconds = Math.max(0, Math.floor((Date.now() - at) / 1000));
  const [value, unit]: [number, Intl.RelativeTimeFormatUnit] =
    seconds < 60
      ? [seconds, "second"]
      : seconds < 3600
        ? [Math.floor(seconds / 60), "minute"]
        : seconds < 86400
          ? [Math.floor(seconds / 3600), "hour"]
          : [Math.floor(seconds / 86400), "day"];
  return new Intl.RelativeTimeFormat(i18nStore.getState().locale, {
    numeric: "auto",
  }).format(-value, unit);
}

export function number(value: number | null | undefined, digits = 1) {
  if (value == null || !Number.isFinite(value)) return "—";
  return new Intl.NumberFormat(i18nStore.getState().locale, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(value);
}

export function latencyParts(
  value: number | null | undefined,
): [string, string] {
  if (value == null) return ["—", ""];
  return value < 1000
    ? [number(value, 1), "ms"]
    : [number(value / 1000, 2), "s"];
}

export function bytes(value: number | null | undefined) {
  if (value == null) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i++;
  }
  return `${number(value, i ? 1 : 0)} ${units[i]}`;
}

export function hours(period: string) {
  return period === "1h" ? 1 : period === "7d" ? 168 : 24;
}
