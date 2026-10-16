import { memo } from "react";
import { t, useI18n } from "../i18n";
import {
  dateTime,
  latencyParts,
  statusLabel,
  type CheckResult,
} from "../stores/app";
function SignalTrace(inputProps: { history: CheckResult[]; slots?: number }) {
  const props = { ...{ slots: 32 }, ...inputProps };
  useI18n();
  const getBars = () => {
    const recent = props.history.slice(0, props.slots).reverse();
    return [
      ...Array<CheckResult | null>(props.slots - recent.length).fill(null),
      ...recent,
    ];
  };
  const getSummary = () => {
    const recent = props.history.slice(0, props.slots);
    if (!recent.length) return t("暂无检测记录");
    const failed = recent.filter((h) => h.state === "failed").length;
    const warning = recent.filter((h) => h.state === "warning").length;
    return t("最近 {p0} 次检测：{p1} 次异常，{p2} 次需关注，其余正常", {
      p0: recent.length,
      p1: failed,
      p2: warning,
    });
  };
  const describe = (h: CheckResult) =>
    `${dateTime(h.at)}　${statusLabel(h.state)}　${latencyParts(h.latency).join(" ")}`;

  const bars = getBars();
  const summary = getSummary();
  return (
    <>
      <span role="img" aria-label={summary} className="trace">
        <>
          {bars.map((h, i) => (
            <i
              key={i}
              data-state={h?.state || "empty"}
              title={h ? describe(h) : undefined}
            ></i>
          ))}
        </>
      </span>
    </>
  );
}

export default memo(SignalTrace);
