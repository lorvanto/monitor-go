// 状态只用四种颜色表达，并配合形状：● 正常、▲ 需关注、■ 异常、‖ 暂停、○ 待定、◆ 处理中。
export type Tone = "ok" | "warn" | "crit" | "paused" | "idle" | "info";

const tones: Record<string, Tone> = {
  healthy: "ok",
  online: "ok",
  resolved: "ok",
  enabled: "ok",
  published: "ok",
  sent: "ok",
  valid: "ok",
  warning: "warn",
  failed: "crit",
  critical: "crit",
  offline: "crit",
  paused: "paused",
  disabled: "paused",
  withdrawn: "paused",
  suppressed: "paused",
  acknowledged: "info",
  queued: "info",
  sending: "info",
  unknown: "idle",
  pending: "idle",
  revoked: "idle",
  draft: "idle",
  expired: "idle",
  neutral: "idle",
};

export function toneOf(status: string | undefined): Tone {
  return (status && tones[status]) || "idle";
}

const order: Record<string, number> = {
  failed: 0,
  warning: 1,
  unknown: 2,
  paused: 3,
  healthy: 4,
};

// 异常优先：用于看板和列表排序。
export function severityRank(status: string) {
  return order[status] ?? 5;
}
