export default function BrandMark(inputProps: { size?: number }) {
  const props = { ...{ size: 22 }, ...inputProps };

  return (
    <>
      <svg
        width={props.size}
        height={props.size}
        viewBox="0 0 22 22"
        aria-hidden="true"
        className="brand-mark"
      >
        <rect x="1" y="12" width="3" height="6" rx="1"></rect>
        <rect x="5.5" y="10" width="3" height="8" rx="1"></rect>
        <rect x="10" y="3" width="3" height="15" rx="1"></rect>
        <rect x="14.5" y="11" width="3" height="7" rx="1"></rect>
        <rect x="19" y="9" width="2" height="9" rx="1"></rect>
      </svg>
    </>
  );
}
