import type { Tone } from "../lib/status";
export default function StatusGlyph(inputProps: { tone: Tone; size?: number }) {
  const props = { ...{ size: 10 }, ...inputProps };

  return (
    <>
      <svg
        data-tone={props.tone}
        width={props.size}
        height={props.size}
        viewBox="0 0 10 10"
        aria-hidden="true"
        className="glyph"
      >
        {props.tone === "ok" ? (
          <circle cx="5" cy="5" r="4"></circle>
        ) : props.tone === "warn" ? (
          <path d="M5 .6 9.6 9.2H.4Z"></path>
        ) : props.tone === "crit" ? (
          <rect x="1" y="1" width="8" height="8" rx="1"></rect>
        ) : props.tone === "paused" ? (
          <g>
            <rect x="1.4" y="1" width="2.6" height="8" rx=".6"></rect>
            <rect x="6" y="1" width="2.6" height="8" rx=".6"></rect>
          </g>
        ) : props.tone === "info" ? (
          <rect
            x="2.3"
            y="2.3"
            width="5.4"
            height="5.4"
            rx=".8"
            transform="rotate(45 5 5)"
          ></rect>
        ) : (
          <circle cx="5" cy="5" r="3.3" className="ring"></circle>
        )}
      </svg>
    </>
  );
}
