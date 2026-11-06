import { useState } from "react";
import { t, useI18n } from "../i18n";
import { monitorIntervalLabel } from "../lib/monitorKinds";

const presets = [30, 60, 180, 300, 900, 3600, 43200, 86400];
const units = [
  { seconds: 1, label: "秒" },
  { seconds: 60, label: "分钟" },
  { seconds: 3600, label: "小时" },
  { seconds: 86400, label: "天" },
] as const;
function initialUnit(seconds: number) {
  return [...units].reverse().find((unit) => seconds % unit.seconds === 0)!
    .seconds;
}
export default function MonitorIntervalEditor({
  value,
  onChange,
  disabled = false,
}: {
  value: number;
  onChange: (seconds: number) => void;
  disabled?: boolean;
}) {
  useI18n();
  const [custom, setCustom] = useState(!presets.includes(value));
  const [unit, setUnit] = useState<number>(() => initialUnit(value));
  const [amount, setAmount] = useState(() =>
    String(value / initialUnit(value)),
  );
  return (
    <fieldset className="field monitor-interval-editor" disabled={disabled}>
      <legend className="field-label">{t("检测间隔")}</legend>
      <label>
        <span className="sr-only">{t("周期预设")}</span>
        <select
          aria-label={t("周期预设")}
          value={custom ? "custom" : String(value)}
          onChange={(event) => {
            const next = event.currentTarget.value;
            if (next === "custom") {
              const nextUnit = initialUnit(value);
              setUnit(nextUnit);
              setAmount(String(value / nextUnit));
              setCustom(true);
            } else {
              setCustom(false);
              onChange(Number(next));
            }
          }}
        >
          {presets.map((seconds) => (
            <option key={seconds} value={seconds}>
              {monitorIntervalLabel(seconds)}
            </option>
          ))}
          <option value="custom">{t("自定义周期")}</option>
        </select>
      </label>
      {custom ? (
        <div className="monitor-interval-custom">
          <label>
            <span className="sr-only">{t("周期数值")}</span>
            <input
              aria-label={t("周期数值")}
              type="number"
              min={15 / unit}
              max={86400 / unit}
              step="any"
              required
              value={amount}
              onChange={(event) => {
                const next = event.currentTarget.value;
                setAmount(next);
                onChange(
                  next === "" ? Number.NaN : Math.round(Number(next) * unit),
                );
              }}
            />
          </label>
          <label>
            <span className="sr-only">{t("周期单位")}</span>
            <select
              aria-label={t("周期单位")}
              value={unit}
              onChange={(event) => {
                const next = Number(event.currentTarget.value);
                setUnit(next);
                onChange(
                  amount === ""
                    ? Number.NaN
                    : Math.round(Number(amount) * next),
                );
              }}
            >
              {units.map((item) => (
                <option key={item.seconds} value={item.seconds}>
                  {t(item.label)}
                </option>
              ))}
            </select>
          </label>
        </div>
      ) : null}
    </fieldset>
  );
}
