import { i18nStore } from "../i18n";
import type { Point } from "./types";

export interface ChartSeries {
  name: string;
  points: Point[];
  color: string;
}

// 取整刻度：步长为 1、2、2.5、5 × 10ⁿ；binary 时按 1024 进位，便于显示字节速率。
export function niceTicks(max: number, binary = false): number[] {
  if (!(max > 0)) return [0, 1];
  let unit = 1;
  if (binary) while (max / unit >= 1024) unit *= 1024;
  const scaled = max / unit;
  const rough = scaled / 4;
  const magnitude = 10 ** Math.floor(Math.log10(rough));
  const norm = rough / magnitude;
  const step =
    (norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 2.5 ? 2.5 : norm <= 5 ? 5 : 10) *
    magnitude;
  const count = Math.max(1, Math.ceil(scaled / step - 1e-9));
  return Array.from(
    { length: count + 1 },
    (_, i) => +(i * step * unit).toPrecision(12),
  );
}

// 时间刻度对齐本地整点：1 小时每 10 分钟，24 小时每 4 小时，7 天每天。
export function timeTicks(start: number, end: number, hours: number) {
  const step = hours <= 1 ? 600000 : hours <= 24 ? 14400000 : 86400000;
  const offset = new Date(start).getTimezoneOffset() * 60000;
  const ticks: number[] = [];
  for (
    let t = Math.ceil((start - offset) / step) * step + offset;
    t <= end;
    t += step
  )
    ticks.push(t);
  return ticks;
}

export function tickLabel(at: number, hours: number) {
  const d = new Date(at);
  return new Intl.DateTimeFormat(
    i18nStore.getState().locale,
    hours > 24 || (d.getHours() === 0 && d.getMinutes() === 0)
      ? { month: "numeric", day: "numeric" }
      : { hour: "2-digit", minute: "2-digit", hour12: false },
  ).format(d);
}

export function stamp(at: number) {
  return new Intl.DateTimeFormat(i18nStore.getState().locale, {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(at);
}

export function plotted(points: Point[]) {
  return points.filter(
    (p): p is { at: number; value: number } =>
      p.value != null && Number.isFinite(p.value),
  );
}

// 图表下方读数：最新、最高、最低均按时间桶平均值计算。
export function readings(points: Point[]) {
  const values = plotted(points);
  if (!values.length) return undefined;
  return {
    latest: values[values.length - 1].value,
    max: Math.max(...values.map((p) => p.value)),
    min: Math.min(...values.map((p) => p.value)),
  };
}
