import { t, type MessageKey } from "../i18n";
import type { Activity, Alert } from "./types";

// Stored engine records contain rendered source text. For compatibility,
// match only complete known templates in their own fields;
// never translate names, notes, arbitrary diagnostics, or stored records.
const fixedDiagnostics = [
  "TCP 连接成功",
  "MySQL SELECT 1 成功",
  "Redis 认证失败",
  "Redis 未返回 PONG",
  "检测超时",
  "检测凭据无法解密",
  "未找到目标地址",
  "不允许探测链路本地、元数据或组播地址",
  "重定向次数过多",
  "重定向协议不支持",
  "重定向不允许登录凭据",
  "检测类型不支持",
] as const satisfies readonly MessageKey[];

export function checkDiagnostic(source: string): string {
  const fixed = fixedDiagnostics.find((key) => key === source);
  if (fixed) return t(fixed);
  const certificate = /^证书有效至 (\S+)，剩余 (-?\d+(?:\.\d+)?) 天$/.exec(
    source,
  );
  if (certificate)
    return t("证书有效至 {expires}，剩余 {days} 天", {
      expires: certificate[1],
      days: certificate[2],
    });
  const parts = source.split(" · ");
  const http = /^HTTP (\d{3}) · expected (\d{3})$/.exec(
    parts.slice(0, 2).join(" · "),
  );
  const httpDetails: Record<string, MessageKey> = {
    "forbidden response text found": "响应包含禁止文本",
    "required response text not found": "响应缺少必需文本",
    "response text matched": "响应文本匹配",
    "body truncated; only the first 1 MiB was checked":
      "响应体已截断，仅检查前 1 MiB",
  };
  if (http) {
    const extra = parts.slice(2).map((part) => {
      if (httpDetails[part]) return t(httpDetails[part]);
      const latency = /^response time exceeds (\d+) ms$/.exec(part);
      return latency
        ? t("响应时间超过 {milliseconds} ms", { milliseconds: latency[1] })
        : undefined;
    });
    if (extra.every((part) => part !== undefined))
      return [
        t("HTTP {actual} · 预期 {expected}", {
          actual: http[1],
          expected: http[2],
        }),
        ...extra,
      ].join(" · ");
  }
  // net/http wraps errors from our dialer/redirect checks with the request URL.
  const wrapped = /^(Get ".*": )(.+)$/.exec(source);
  if (wrapped) {
    const key = fixedDiagnostics.find((key) => key === wrapped[2]);
    if (key) return wrapped[1] + t(key);
  }
  return source;
}

const hostTitles = [
  "主机 CPU 使用率需要关注",
  "主机内存使用率需要关注",
  "主机磁盘空间需要关注",
  "远程主机离线",
] as const satisfies readonly MessageKey[];
export function alertTitle(alert: Pick<Alert, "kind" | "title">): string {
  if (alert.kind === "host") {
    const key = hostTitles.find((key) => key === alert.title);
    return key ? t(key) : alert.title;
  }
  const failures = /^(.*) 连续 (\d+) 次检测异常$/.exec(alert.title);
  if (failures)
    return t("{name} 连续 {count} 次检测异常", {
      name: failures[1],
      count: failures[2],
    });
  const ssl = /^(.*) 证书即将到期$/.exec(alert.title);
  if (ssl) return t("{name} 证书即将到期", { name: ssl[1] });
  const latency = /^(.*) response time exceeds the configured threshold$/.exec(
    alert.title,
  );
  if (latency) return t("{name} 响应时间超过配置阈值", { name: latency[1] });
  return alert.title;
}
export function alertDescription(
  alert: Pick<Alert, "kind" | "description">,
): string {
  if (alert.kind === "monitor") return checkDiagnostic(alert.description);
  const fixed = [
    "主机指标告警已关闭",
    "Agent 已恢复上报",
    "超过 90 秒未收到 Agent 上报，请检查主机和网络",
  ] as const;
  const key = fixed.find((key) => key === alert.description);
  if (key) return t(key);
  const usage = /^(CPU|MEMORY) · (\d+(?:\.\d+)?)% \/ (\d+)% · (\d+) min$/.exec(
    alert.description,
  );
  if (usage)
    return t("{metric} · {usage}% / {threshold}% · {minutes} 分钟", {
      metric: usage[1] === "CPU" ? "CPU" : t("内存"),
      usage: usage[2],
      threshold: usage[3],
      minutes: usage[4],
    });
  const disks = alert.description
    .split("；")
    .map((part) => /^(.*) 使用率 (\d+(?:\.\d+)?)%$/.exec(part));
  if (disks.every((disk) => disk !== null))
    return disks
      .map((disk) => {
        let path = disk![1];
        if (path.startsWith("APFS 容器 "))
          path = t("APFS 容器 {path}", { path: path.slice(8) });
        else if (path.startsWith("APFS 卷配额 "))
          path = t("APFS 卷配额 {path}", { path: path.slice(9) });
        return t("{path} 使用率 {percent}%", { path, percent: disk![2] });
      })
      .join("; ");
  return alert.description;
}
const enginePrefixes = ["达到阈值，创建告警：", "资源恢复正常："] as const;
function engineEvent(activity: Activity) {
  return activity.actor === "检测引擎"
    ? enginePrefixes.find((prefix) => activity.message.startsWith(prefix))
    : undefined;
}
export function alertActivityMessage(
  activity: Activity,
  alert: Pick<Alert, "kind">,
): string {
  if (activity.message === "确认并接手告警") return t("已确认并认领告警");
  if (activity.message === "调整事件静默时间") return t("事件静默时间已更新");
  const prefix = engineEvent(activity);
  if (!prefix) return activity.message;
  const detail = alertDescription({
    kind: alert.kind,
    description: activity.message.slice(prefix.length),
  });
  return prefix === enginePrefixes[0]
    ? t("达到阈值，创建告警：{detail}", { detail })
    : t("资源恢复正常：{detail}", { detail });
}
export function alertActivityActor(activity: Activity): string {
  return engineEvent(activity) ? t("检测引擎") : activity.actor;
}
