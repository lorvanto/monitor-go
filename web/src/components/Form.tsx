import { useLayoutEffect, useRef, type ComponentProps } from "react";
import { t, useI18n } from "../i18n";

type Field = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;
const isField = (target: EventTarget | null): target is Field =>
  target instanceof HTMLInputElement ||
  target instanceof HTMLSelectElement ||
  target instanceof HTMLTextAreaElement;

// Browser validation bubbles otherwise use the browser's language, not <html lang>.
export function validationMessage(field: Field): string {
  const v = field.validity;
  if (v.valueMissing) {
    if (field instanceof HTMLSelectElement || field.type === "radio")
      return t("请选择一个选项。");
    return field.type === "checkbox" ? t("请勾选此项。") : t("请填写此字段。");
  }
  if (v.typeMismatch) {
    if (field.type === "email") return t("请输入有效的电子邮箱地址。");
    if (field.type === "url") return t("请输入有效的网址。");
  }
  if (v.badInput) return t("请输入有效的数字。");
  if (field instanceof HTMLInputElement) {
    if (v.rangeUnderflow)
      return t("请输入不小于 {min} 的值。", { min: field.min });
    if (v.rangeOverflow)
      return t("请输入不大于 {max} 的值。", { max: field.max });
    if (v.stepMismatch)
      return t("请输入符合步长 {step} 的值。", { step: field.step || "1" });
    if (v.patternMismatch) return t("请按要求的格式填写此字段。");
  }
  if (!(field instanceof HTMLSelectElement)) {
    if (v.tooShort)
      return t("请至少输入 {min} 个字符。", { min: field.minLength });
    if (v.tooLong)
      return t("请最多输入 {max} 个字符。", { max: field.maxLength });
  }
  return v.valid ? "" : t("请检查此字段的值。");
}

export default function Form(props: ComponentProps<"form">) {
  useI18n();
  const form = useRef<HTMLFormElement>(null);
  const validated = useRef(new WeakSet<Field>());
  function update(field: Field) {
    field.setCustomValidity("");
    field.setCustomValidity(validationMessage(field));
  }
  // Also refresh messages when controlled values or the selected locale change.
  useLayoutEffect(() => {
    for (const field of Array.from(form.current?.elements || []))
      if (isField(field) && validated.current.has(field)) update(field);
  });
  return (
    <form
      {...props}
      ref={form}
      onInvalidCapture={(event) => {
        if (isField(event.target)) {
          validated.current.add(event.target);
          update(event.target);
        }
        props.onInvalidCapture?.(event);
      }}
      onInputCapture={(event) => {
        if (isField(event.target) && validated.current.has(event.target))
          update(event.target);
        props.onInputCapture?.(event);
      }}
      onChangeCapture={(event) => {
        if (isField(event.target) && validated.current.has(event.target))
          update(event.target);
        props.onChangeCapture?.(event);
      }}
    />
  );
}
