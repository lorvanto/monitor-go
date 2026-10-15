import type { ReactElement } from "react";
import { isMessageKey, t, useI18n, type MessageKey } from "../i18n";

export interface Message {
  key: MessageKey;
  params?: Record<string, unknown>;
  args?: (string | number)[];
}
export type DisplayMessage = string | ReactElement;
export const message = (
  key: MessageKey,
  params?: Message["params"],
): Message => ({ key, params });

function text(value: string | Message): string {
  if (typeof value === "string") return value;
  let index = 0;
  return t(value.key, value.params).replace(/%[ds]/g, (token) => {
    const arg = value.args?.[index++];
    return arg === undefined ? token : String(arg);
  });
}
function TranslatedMessage({ value }: { value: Message }) {
  useI18n();
  return <>{text(value)}</>;
}
export function notice(
  key: MessageKey,
  params?: Message["params"],
): DisplayMessage {
  return <TranslatedMessage value={message(key, params)} />;
}
export class LocalizedError extends Error {
  constructor(public display: string | Message) {
    super(text(display));
  }
}
export function errorMessage(error: unknown): DisplayMessage {
  if (error instanceof LocalizedError && typeof error.display !== "string")
    return <TranslatedMessage value={error.display} />;
  return error instanceof Error ? error.message : t("请求失败");
}
// Only explicit server message keys are translated. Raw names and diagnostics stay intact.
export function responseMessage(
  error:
    | { message?: string; key?: string; args?: (string | number)[] }
    | undefined,
  fallback: MessageKey = "请求失败",
): string | Message {
  return isMessageKey(error?.key)
    ? { key: error.key, args: error.args }
    : error?.message || message(fallback);
}
