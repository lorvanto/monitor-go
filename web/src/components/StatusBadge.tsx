import { toneOf, type Tone } from "../lib/status";
import StatusGlyph from "./StatusGlyph";
export default function StatusBadge(inputProps: {
  status?: string;
  tone?: Tone;
  label: string;
}) {
  const props = { ...{}, ...inputProps };
  const getResolved = () => props.tone || toneOf(props.status);

  const resolved = getResolved();
  return (
    <>
      <span data-tone={resolved} className="badge">
        <StatusGlyph tone={resolved} size={9}></StatusGlyph>
        {props.label}
      </span>
    </>
  );
}
