import { t } from "../i18n";

export const monitorKinds = ["HTTP", "TCP", "SSL", "MySQL", "Redis"] as const;
export type MonitorKind = (typeof monitorKinds)[number];
export const monitorKindLabel = {
  HTTP: "网站 HTTP",
  TCP: "端口 TCP",
  SSL: "SSL 证书",
  MySQL: "MySQL",
  Redis: "Redis",
} as const;
export const monitorKindIcon = {
  HTTP: "Globe",
  TCP: "Network",
  SSL: "LockKeyhole",
  MySQL: "HardDrive",
  Redis: "Box",
} as const;
export const defaultMonitorInterval: Record<MonitorKind, number> = {
  HTTP: 60,
  TCP: 60,
  SSL: 86400,
  MySQL: 180,
  Redis: 60,
};
export function monitorKind(value: string): MonitorKind {
  return monitorKinds.find((kind) => kind === value) || "HTTP";
}
export const validMonitorInterval = (seconds: number) =>
  Number.isInteger(seconds) && seconds >= 15 && seconds <= 86400;
export function monitorIntervalLabel(seconds: number) {
  if (seconds === 86400) return t("每天");
  if (seconds === 3600) return t("每小时");
  if (seconds === 60) return t("每分钟");
  if (seconds % 86400 === 0) return t("每 {p0} 天", { p0: seconds / 86400 });
  if (seconds % 3600 === 0) return t("每 {p0} 小时", { p0: seconds / 3600 });
  if (seconds % 60 === 0) return t("每 {p0} 分钟", { p0: seconds / 60 });
  return t("每 {p0} 秒", { p0: seconds });
}
