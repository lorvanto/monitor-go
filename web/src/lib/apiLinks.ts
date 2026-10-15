import { i18nStore } from "../i18n";

// A navigation/download does not inherit the headers used by fetch or XHR.
export function apiLink(path: string): string {
  return `${path}${path.includes("?") ? "&" : "?"}lang=${i18nStore.getState().locale}`;
}
