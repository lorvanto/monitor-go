import { metadataLabel } from "../i18n";

// Only the server can identify untouched seeded values. Never translate raw names.
export function resourceName(
  value: { name: string; nameKey?: string } | undefined,
) {
  return value
    ? value.nameKey
      ? metadataLabel(value.nameKey)
      : value.name
    : "";
}
export function hostName(value: {
  alias: string;
  hostname: string;
  aliasKey?: string;
}) {
  return value.aliasKey
    ? metadataLabel(value.aliasKey)
    : value.alias || value.hostname;
}
