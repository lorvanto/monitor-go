import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";

beforeEach(() => {
  vi.resetModules();
  localStorage.clear();
  vi.spyOn(navigator, "languages", "get").mockReturnValue(["zh-CN", "ja-JP"]);
  vi.spyOn(navigator, "language", "get").mockReturnValue("zh-CN");
});

it.each([
  { initialized: false, page: "setup" },
  { initialized: true, page: "sign-in" },
])(
  "starts $page in English even with a Chinese browser",
  async ({ initialized }) => {
    const i18n = await import("../src/i18n");
    await i18n.initializeLocale();
    const { appStore } = await import("../src/stores/app");
    const { default: AuthView } = await import("../src/views/AuthView");
    appStore.setState({ initialized, booting: false });
    render(<AuthView />);
    expect(i18n.i18nStore.getState().locale).toBe("en-US");
    expect(document.documentElement.lang).toBe("en-US");
    expect(screen.getByRole("combobox").getAttribute("aria-label")).toBe(
      i18n.t("界面语言"),
    );
    expect((screen.getByRole("combobox") as HTMLSelectElement).value).toBe(
      "en-US",
    );
    expect(
      screen.getByRole("heading", {
        name: i18n.t(initialized ? "登录" : "创建首个管理员"),
      }),
    ).toBeTruthy();
  },
);

it("restores a saved language instead of replacing it with English", async () => {
  localStorage.setItem("monitor.locale", "zh-Hant");
  const { initializeLocale, i18nStore } = await import("../src/i18n");
  await initializeLocale();
  expect(i18nStore.getState().locale).toBe("zh-TW");
  expect(document.documentElement.lang).toBe("zh-TW");
  expect(localStorage.getItem("monitor.locale")).toBe("zh-TW");
});

it("uses English when the stored preference is invalid", async () => {
  localStorage.setItem("monitor.locale", "unsupported");
  const { initializeLocale, i18nStore } = await import("../src/i18n");
  await initializeLocale();
  expect(i18nStore.getState().locale).toBe("en-US");
});

it("uses English when browser storage is unavailable", async () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("Storage disabled");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("Storage disabled");
  });
  const { initializeLocale, i18nStore } = await import("../src/i18n");
  await expect(initializeLocale()).resolves.toBeUndefined();
  expect(i18nStore.getState().locale).toBe("en-US");
  expect(i18nStore.getState().localeLoadFailed).toBe(false);
});

it("preserves every setup field while switching all five languages", async () => {
  const { initializeLocale, setLocale, locales, t } = await import(
    "../src/i18n"
  );
  await initializeLocale();
  const { appStore } = await import("../src/stores/app");
  const { default: AuthView } = await import("../src/views/AuthView");
  appStore.setState({ initialized: false, booting: false });
  render(<AuthView />);
  const fields = ["初始化码", "用户名", "姓名", "密码"] as const;
  const inputs = fields.map(
    (field) =>
      screen.getByLabelText((label) => label.startsWith(t(field)), {
        selector: "input",
      }) as HTMLInputElement,
  );
  const values = [
    "test-setup-token",
    "test-admin",
    "Test Administrator",
    "test-only-password",
  ];
  inputs.forEach((input, index) =>
    fireEvent.change(input, { target: { value: values[index] } }),
  );
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(inputs.map((input) => input.value)).toEqual(values);
    expect(document.documentElement.lang).toBe(locale.code);
  }
});
