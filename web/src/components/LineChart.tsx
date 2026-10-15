import {
  Fragment,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { createStore, useStore } from "zustand";
import { i18nStore, t, useI18n } from "../i18n";
import {
  niceTicks,
  plotted,
  stamp,
  tickLabel,
  timeTicks,
  type ChartSeries,
} from "../lib/chart";
import { classNames } from "../lib/react";
export default function LineChart(inputProps: {
  series: ChartSeries[];
  hours: number;
  label: string;
  end?: number;
  max?: number;
  binary?: boolean;
  format?: (v: number) => string;
  tickFormat?: (v: number) => string;
  height?: number;
}) {
  const props = { ...{ height: 168 }, ...inputProps };
  const initialState = {
    width: 640,
    active: null as number | null,
    keyboard: false,
  };
  type LocalState = typeof initialState;
  const [store] = useState(() => createStore<LocalState>(() => initialState));
  const viewState = useStore(store);
  const root = useRef<HTMLElement | null>(null);
  const observer = useRef<ResizeObserver | undefined | undefined>(undefined);
  useI18n();
  const pad = { top: 12, right: 14, bottom: 28, left: 52 };
  const fmt = (() =>
    props.format ||
    ((v: number) =>
      new Intl.NumberFormat(i18nStore.getState().locale, {
        maximumFractionDigits: 1,
      }).format(v)))();
  const tickFmt = (() => props.tickFormat || fmt)();
  const span = useMemo(() => {
    const latest = Math.max(
      0,
      ...props.series.flatMap((s) => s.points.map((p) => p.at)),
    );
    const end = Math.max(props.end || Date.now(), latest);
    return { start: end - props.hours * 3600000, end };
  }, [props.series, props.end, props.hours]);
  const bucket = (() => (props.hours * 3600000) / 180)();
  const visible = (() =>
    props.series.map((s) =>
      plotted(s.points).filter((p) => p.at >= span.start - bucket),
    ))();
  const gap = (() => {
    const deltas = visible
      .flatMap((points) => points.slice(1).map((p, i) => p.at - points[i].at))
      .sort((a, b) => a - b);
    const median =
      deltas.length >= 4 ? deltas[Math.floor(deltas.length / 2)] : 0;
    return Math.max(bucket, median) * 2.5;
  })();
  const hasData = (() => visible.some((v) => v.length))();
  const yTicks = (() =>
    niceTicks(
      props.max ?? Math.max(0, ...visible.flat().map((p) => p.value)),
      props.binary,
    ))();
  const yMax = (() => yTicks[yTicks.length - 1])();
  const plotW = (() => store.getState().width - pad.left - pad.right)();
  const baseY = (() => pad.top + props.height)();
  const x = (at: number) =>
    pad.left +
    ((Math.min(Math.max(at, span.start), span.end) - span.start) /
      (span.end - span.start)) *
      plotW;
  const y = (v: number) =>
    pad.top + props.height - (Math.min(v, yMax) / yMax) * props.height;
  const xTicks = (() => {
    const ticks = timeTicks(span.start, span.end, props.hours);
    return plotW / Math.max(1, ticks.length) < 56
      ? ticks.filter((_, i) => i % 2 === 0)
      : ticks;
  })();
  const drawn = (() =>
    visible.map((points, index) => {
      const segments: (typeof points)[] = [];
      for (const p of points) {
        const last = segments[segments.length - 1];
        if (!last || p.at - last[last.length - 1].at > gap) segments.push([p]);
        else last.push(p);
      }
      const paths = segments
        .filter((s) => s.length > 1)
        .map((s) => {
          const line = s
            .map(
              (p, i) =>
                `${i ? "L" : "M"}${x(p.at).toFixed(1)},${y(p.value).toFixed(1)}`,
            )
            .join("");
          return {
            line,
            area: `${line}L${x(s[s.length - 1].at).toFixed(1)},${baseY}L${x(s[0].at).toFixed(1)},${baseY}Z`,
          };
        });
      return {
        name: props.series[index].name,
        color: props.series[index].color,
        paths,
        singles: segments.filter((s) => s.length === 1).map((s) => s[0]),
        last: points[points.length - 1],
      };
    }))();
  const times = (() =>
    [...new Set(visible.flat().map((p) => p.at))].sort((a, b) => a - b))();
  function nearest(at: number) {
    const list = times;
    if (!list.length) return null;
    let best = list[0];
    for (const t of list) if (Math.abs(t - at) < Math.abs(best - at)) best = t;
    return best;
  }
  function move(e: React.PointerEvent<SVGRectElement>) {
    const box = (e.currentTarget as SVGRectElement).getBoundingClientRect();
    const ratio = (e.clientX - box.left) / box.width;
    store.setState({ keyboard: false });
    store.setState({
      active: nearest(span.start + ratio * (span.end - span.start)),
    });
  }
  function key(e: React.KeyboardEvent<HTMLDivElement>) {
    const list = times;
    if (!list.length) return;
    const active = store.getState().active;
    const index = active == null ? list.length : list.indexOf(active);
    const next =
      e.key === "ArrowLeft"
        ? Math.max(0, index - 1)
        : e.key === "ArrowRight"
          ? Math.min(list.length - 1, index + 1)
          : e.key === "Home"
            ? 0
            : e.key === "End"
              ? list.length - 1
              : -1;
    if (e.key === "Escape") store.setState({ active: null });
    if (next < 0) return;
    e.preventDefault();
    store.setState({ keyboard: true });
    store.setState({ active: list[next] });
  }
  const tip = (() => {
    const at = store.getState().active;
    if (at == null) return null;
    return {
      at,
      x: x(at),
      rows: visible.map((points, i) => {
        const p = points.find((v) => Math.abs(v.at - at) <= bucket / 2);
        return {
          name: props.series[i].name,
          color: props.series[i].color,
          value: p ? p.value : null,
        };
      }),
    };
  })();
  const tipText = (() => {
    return tip
      ? `${stamp(tip.at)} ${tip.rows.map((row) => `${row.name} ${row.value == null ? t("无数据") : fmt(row.value)}`).join("，")}`
      : "";
  })();

  const summary = (() => {
    const all = visible.flat();
    if (!all.length)
      return t("{p0}：所选时间范围内暂无采样记录", { p0: props.label });
    return `${props.label}：${props.series
      .map((s, i) => {
        const values = visible[i];
        if (!values.length) return t("{p0}无数据", { p0: s.name });
        return t("{p0}最新 {p1}，最高 {p2}", {
          p0: s.name,
          p1: fmt(values[values.length - 1].value),
          p2: fmt(Math.max(...values.map((p) => p.value))),
        });
      })
      .join("；")}`;
  })();
  useEffect(() => {
    if (!root.current) return;
    store.setState({
      width: Math.max(240, Math.round(root.current.clientWidth)),
    });
    observer.current = new ResizeObserver(([entry]) => {
      store.setState({
        width: Math.max(240, Math.round(entry.contentRect.width)),
      });
    });
    observer.current.observe(root.current);
  }, []);
  useEffect(
    () => () => {
      (() => observer.current?.disconnect())();
    },
    [],
  );

  return (
    <>
      <div
        ref={(element) => {
          root.current = element;
        }}
        tabIndex={0}
        aria-label={t("{p0}。可用左右方向键查看各时间点。", { p0: summary })}
        className="chart"
        onKeyDown={(event) => {
          key(event);
        }}
        onBlur={() => {
          store.setState({ active: null });
        }}
      >
        <svg
          width={viewState.width}
          height={props.height + pad.top + pad.bottom}
          viewBox={`0 0 ${viewState.width} ${props.height + pad.top + pad.bottom}`}
          role="img"
          aria-label={summary}
        >
          <g className="chart-grid">
            <>
              {yTicks.map((t) => (
                <line
                  key={t}
                  x1={pad.left}
                  x2={viewState.width - pad.right}
                  y1={y(t)}
                  y2={y(t)}
                ></line>
              ))}
            </>
          </g>
          <g className="chart-axis">
            <>
              {yTicks.map((t) => (
                <text
                  key={t}
                  x={pad.left - 10}
                  y={y(t)}
                  dy="0.32em"
                  textAnchor="end"
                >
                  {tickFmt(t)}
                </text>
              ))}
            </>
            <>
              {xTicks.map((t) => (
                <text key={t} x={x(t)} y={baseY + 19} textAnchor="middle">
                  {tickLabel(t, props.hours)}
                </text>
              ))}
            </>
          </g>
          <line
            x1={pad.left}
            x2={viewState.width - pad.right}
            y1={baseY}
            y2={baseY}
            className="chart-baseline"
          ></line>
          <>
            {drawn.map((s) => (
              <g
                key={s.name}
                style={{ "--series": s.color } as CSSProperties}
                className="chart-series"
              >
                {drawn.length === 1 ? (
                  <>
                    <>
                      {s.paths.map((p, i) => (
                        <path
                          key={"a" + i}
                          d={p.area}
                          className="chart-area"
                        ></path>
                      ))}
                    </>
                  </>
                ) : null}
                <>
                  {s.paths.map((p, i) => (
                    <path
                      key={"l" + i}
                      d={p.line}
                      className="chart-line"
                    ></path>
                  ))}
                </>
                <>
                  {s.singles.map((p) => (
                    <circle
                      key={p.at}
                      cx={x(p.at)}
                      cy={y(p.value)}
                      r="2.5"
                      className="chart-dot"
                    ></circle>
                  ))}
                </>
                {s.last ? (
                  <circle
                    cx={x(s.last.at)}
                    cy={y(s.last.value)}
                    r="4"
                    className="chart-end"
                  ></circle>
                ) : null}
              </g>
            ))}
          </>
          {tip ? (
            <g className="chart-cursor">
              <line x1={tip.x} x2={tip.x} y1={pad.top} y2={baseY}></line>
              <>
                {tip.rows.map((r) => (
                  <Fragment key={r.name}>
                    {r.value != null ? (
                      <circle
                        cx={tip.x}
                        cy={y(r.value)}
                        r="4"
                        style={{ fill: r.color } as CSSProperties}
                      ></circle>
                    ) : null}
                  </Fragment>
                ))}
              </>
            </g>
          ) : null}
          <rect
            x={pad.left}
            y={pad.top}
            width={Math.max(0, plotW)}
            height={props.height}
            className="chart-hit"
            onPointerMove={(event) => {
              move(event);
            }}
            onPointerLeave={() => {
              !viewState.keyboard && store.setState({ active: null });
            }}
          ></rect>
        </svg>
        {!hasData ? (
          <p className="chart-empty">{t("所选时间范围内暂无采样记录")}</p>
        ) : null}
        {tip ? (
          <div
            style={{ left: tip.x + "px" } as CSSProperties}
            aria-hidden="true"
            className={classNames("chart-tip", {
              flip: tip.x > viewState.width * 0.62,
            })}
          >
            <time>{stamp(tip.at)}</time>
            <>
              {tip.rows.map((r) => (
                <div key={r.name} className="chart-tip-row">
                  <i style={{ background: r.color } as CSSProperties}></i>
                  <strong>
                    {r.value == null ? t("无数据") : fmt(r.value)}
                  </strong>
                  <span>{r.name}</span>
                </div>
              ))}
            </>
          </div>
        ) : null}
        <span aria-live="polite" className="sr-only">
          {viewState.keyboard ? tipText : ""}
        </span>
      </div>
    </>
  );
}
