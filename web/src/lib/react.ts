type ClassValue =
  | string
  | false
  | null
  | undefined
  | Record<string, unknown>
  | ClassValue[];
export function classNames(...values: ClassValue[]): string {
  return values
    .flatMap((value) =>
      !value
        ? []
        : typeof value === "string"
          ? [value]
          : Array.isArray(value)
            ? [classNames(...value)]
            : Object.keys(value).filter((key) => value[key]),
    )
    .join(" ");
}
