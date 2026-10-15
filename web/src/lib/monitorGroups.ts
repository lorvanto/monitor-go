import { t } from "../i18n";

// Empty is the language-neutral default. Existing/custom names remain literal.
export const groupName = (group: string) => group || t("默认分组");
export const groupOption = (group: string) => `group:${group}`;
export const matchesGroup = (group: string, option: string) =>
  !option || groupOption(group) === option;
export const groupQuery = (option: string) => {
  if (!option) return "";
  const group = option.slice("group:".length);
  return group ? `&group=${encodeURIComponent(group)}` : "&ungrouped=1";
};
