import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import Form from "../src/components/Form";
import AuthView from "../src/views/AuthView";
import { setLocale, locales, t } from "../src/i18n";
import { api, appStore } from "../src/stores/app";
import {
  errorMessage,
  LocalizedError,
  message,
  responseMessage,
} from "../src/lib/messages";
import { apiLink } from "../src/lib/apiLinks";
import {
  groupName,
  groupOption,
  matchesGroup,
  groupQuery,
} from "../src/lib/monitorGroups";

beforeEach(async () => {
  appStore.setState({ ...appStore.getInitialState(), booting: false }, true);
  await setLocale("en-US");
});

it.each([
  ["text", { required: true }, "", "Please fill out this field."],
  ["email", {}, "invalid", "Please enter a valid email address."],
  ["url", {}, "invalid", "Please enter a valid URL."],
  [
    "number",
    { min: 15 },
    "1",
    "Please enter a value greater than or equal to 15.",
  ],
  [
    "number",
    { max: 30 },
    "31",
    "Please enter a value less than or equal to 30.",
  ],
  [
    "number",
    { step: 2, min: 0 },
    "3",
    "Please enter a value that matches the step of 2.",
  ],
  [
    "text",
    { pattern: "[A-Z]+" },
    "lower",
    "Please use the requested format for this field.",
  ],
  ["checkbox", { required: true }, "", "Please check this box."],
] as const)(
  "localizes %s constraint validation and clears it after correction",
  (type, attrs, value, expected) => {
    const view = render(
      <Form>
        <input aria-label="Field" type={type} {...attrs} defaultValue={value} />
      </Form>,
    );
    const input = view.container.querySelector("input")!;
    fireEvent.invalid(input);
    expect(input.validationMessage).toBe(expected);
    if (type === "checkbox") fireEvent.click(input);
    else
      fireEvent.input(input, {
        target: {
          value:
            type === "email"
              ? "a@example.com"
              : type === "url"
                ? "https://example.com"
                : type === "number"
                  ? "16"
                  : "VALID",
        },
      });
    expect(input.validationMessage).toBe("");
  },
);

it("keeps native errors and drafts synchronized through five languages, including portal form submit buttons", async () => {
  render(
    <>
      <Form id="sample">
        <input aria-label="Field" required />
        <select aria-label="Choice" required defaultValue="">
          <option value="">Choose</option>
          <option value="one">One</option>
        </select>
      </Form>
      <button form="sample">Submit</button>
    </>,
  );
  const input = screen.getByLabelText("Field") as HTMLInputElement;
  const select = screen.getByLabelText("Choice") as HTMLSelectElement;
  fireEvent.click(screen.getByRole("button"));
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(input.validationMessage).toBe(t("请填写此字段。"));
    expect(select.validationMessage).toBe(t("请选择一个选项。"));
  }
  fireEvent.input(input, { target: { value: "用户内容" } });
  fireEvent.change(select, { target: { value: "one" } });
  expect(input.validationMessage).toBe("");
  expect(select.validationMessage).toBe("");
  await act(() => setLocale("en-US"));
  expect(input.value).toBe("用户内容");
});

it.each([false, true])(
  "retranslates setup/login API errors without resubmitting or clearing fields (initialized=%s)",
  async (initialized) => {
    appStore.setState({ initialized });
    await setLocale("zh-CN");
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              error: {
                message: "用户名或密码不正确",
                key: "用户名或密码不正确",
                args: null,
              },
            }),
            { status: 401 },
          ),
      ),
    );
    render(<AuthView />);
    const username = screen.getByLabelText(t("用户名")) as HTMLInputElement;
    const password = screen.getByLabelText(
      (label) => label.startsWith(t("密码")),
      { selector: "input" },
    ) as HTMLInputElement;
    fireEvent.change(username, { target: { value: "reviewer" } });
    fireEvent.change(password, { target: { value: "unchanged-draft" } });
    fireEvent.submit(username.closest("form")!);
    await screen.findByRole("alert");
    for (const locale of locales) {
      await act(() => setLocale(locale.code));
      expect(screen.getByRole("alert").textContent).toBe(
        t("用户名或密码不正确"),
      );
      expect(username.value).toBe("reviewer");
      expect(password.value).toBe("unchanged-draft");
    }
    expect(fetch).toHaveBeenCalledTimes(1);
  },
);

it("uses the current language when an earlier-language request finishes late", async () => {
  await setLocale("zh-CN");
  let resolve!: (r: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  const result = api("/tools/codes", "POST", {}).catch((e) => e);
  await setLocale("en-US");
  resolve(
    new Response(
      JSON.stringify({
        error: {
          message: "请输入 12 位数字（自动校验）或 13 位完整编码",
          key: "请输入 %d 位数字（自动校验）或 %d 位完整编码",
          args: [12, 13],
        },
      }),
      { status: 422 },
    ),
  );
  const error = await result;
  render(<div role="alert">{errorMessage(error)}</div>);
  expect(screen.getByRole("alert").textContent).not.toMatch(/[\u3400-\u9fff]/u);
  expect(screen.getByRole("alert").textContent).toContain("12");
  expect(screen.getByRole("alert").textContent).toContain("13");
});

it("retranslates local network errors while preserving raw diagnostics", async () => {
  await setLocale("zh-CN");
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
  const error = await api("/status").catch((e) => e);
  render(
    <>
      <div role="alert">{errorMessage(error)}</div>
      <p>{errorMessage(new Error("自定义诊断"))}</p>
    </>,
  );
  await act(() => setLocale("en-US"));
  expect(screen.getByRole("alert").textContent).toBe(
    t("无法连接服务，请检查 Go 后端是否正在运行"),
  );
  expect(screen.getByText("自定义诊断")).toBeTruthy();
});

it("supports structured XHR errors and leaves unknown server messages literal", async () => {
  const error = new LocalizedError(
    responseMessage({
      key: "文件不能为空，且不能超过 %d MB",
      args: [32],
      message: "旧文案",
    }),
  );
  render(<div role="alert">{errorMessage(error)}</div>);
  expect(screen.getByRole("alert").textContent).toContain("32");
  await act(() => setLocale("ja-JP"));
  expect(screen.getByRole("alert").textContent).toContain("32");
  expect(responseMessage({ key: "custom", message: "保留原文" })).toBe(
    "保留原文",
  );
  expect(responseMessage(undefined)).toEqual(message("请求失败"));
});

it("puts the UI language on download/image links without changing other parameters", async () => {
  expect(apiLink("/api/v1/files/1/download")).toBe(
    "/api/v1/files/1/download?lang=en-US",
  );
  await setLocale("ja-JP");
  expect(apiLink("/api/v1/files/1/image?style=2")).toBe(
    "/api/v1/files/1/image?style=2&lang=ja-JP",
  );
});

it("distinguishes all groups, the localized default and literal user group names", async () => {
  for (const locale of locales) {
    await setLocale(locale.code);
    expect(groupName("")).toBe(t("默认分组"));
    expect(groupName("默认分组")).toBe("默认分组");
    expect(matchesGroup("", groupOption(""))).toBe(true);
    expect(matchesGroup("生产", groupOption(""))).toBe(false);
    expect(matchesGroup("生产", "")).toBe(true);
  }
  expect(groupQuery("")).toBe("");
  expect(groupQuery(groupOption(""))).toBe("&ungrouped=1");
  expect(groupQuery(groupOption("group:"))).toBe("&group=group%3A");
});
