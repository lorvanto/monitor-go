import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import AuthView from "../src/views/AuthView";
import {
  appStore,
  boot,
  type SetupActivation,
  type User,
} from "../src/stores/app";
import { locales, setLocale, t } from "../src/i18n";

const original = appStore.getInitialState();
const destination = "https://console.example.test:5090";
const activation: SetupActivation = {
  required: true,
  configured: true,
  consoleUrl: destination,
  noticeVersion: "2",
};
const user: User = {
  id: "1",
  username: "test-admin",
  name: "Test administrator",
  enabled: true,
  version: 1,
  roles: [],
  permissions: [],
  csrf: "test-csrf",
};
const draft = {
  token: "test-only-setup-token",
  username: "test-admin",
  name: "Test administrator",
  password: "test-only-password",
};
let calls: { path: string; body: unknown }[];
let responseError = "";
let currentActivation: SetupActivation;

beforeEach(async () => {
  await setLocale("en-US");
  localStorage.clear();
  appStore.setState(
    {
      ...original,
      initialized: false,
      booting: false,
      activation: { ...activation },
    },
    true,
  );
  calls = [];
  responseError = "";
  currentActivation = { ...activation };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      calls.push({
        path,
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
      });
      if (path === "/api/v1/setup" && responseError) {
        return new Response(
          JSON.stringify({
            error: { key: responseError, message: responseError },
          }),
          { status: 422 },
        );
      }
      const data =
        path === "/api/v1/status"
          ? { initialized: false, activation: currentActivation }
          : path === "/api/v1/me"
            ? user
            : path.startsWith("/api/v1/poll")
              ? {
                  user,
                  usageEnabled: true,
                  navigation: { items: [], instanceName: "" },
                  hosts: [],
                  monitors: [],
                  alerts: [],
                  openAlertCount: 0,
                }
              : { csrf: "test-csrf" };
      return new Response(JSON.stringify({ data }), { status: 200 });
    }),
  );
});

function fillDraft() {
  const keys = ["初始化码", "用户名", "姓名", "密码"] as const;
  const fields = keys.map(
    (key) =>
      screen.getByLabelText((label) => label.startsWith(t(key)), {
        selector: "input",
      }) as HTMLInputElement,
  );
  fields.forEach((input, index) =>
    fireEvent.change(input, { target: { value: Object.values(draft)[index] } }),
  );
  return fields;
}
function consent() {
  return screen.getByRole("checkbox", {
    name: t(
      "我已阅读目的地、字段和规则，同意激活后持续上报，且程序内无法关闭。",
    ),
  }) as HTMLInputElement;
}
function submit(fields: HTMLInputElement[]) {
  fireEvent.submit(fields[0].closest("form")!);
}

it("loads the server activation requirement during boot without activating", async () => {
  appStore.setState({ activation: null });
  await boot();
  expect(appStore.getState().activation).toEqual(activation);
  expect(appStore.getState().initialized).toBe(false);
  expect(calls.map((call) => call.path)).toEqual(["/api/v1/status"]);
});

it("discloses the destination and fields in every language, retains drafts and never submits without consent", async () => {
  render(<AuthView />);
  const fields = fillDraft();
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(
      screen.getByRole("heading", { name: t("首次安装在线激活") }),
    ).toBeTruthy();
    expect(screen.getByText(destination)).toBeTruthy();
    expect(
      screen.getByText(
        t("随机实例 ID、产品版本、系统类型和按 UTC 日期汇总的界面活跃标记。"),
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        t(
          "不会发送账号身份、架构、部署方式、监控目标或指标、文件、密码、监控密钥、应用密钥或本地成员列表。",
        ),
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        t(
          "激活后持续上报，程序内不提供关闭选项。管理后台不可用时会稍后重试，不影响本地登录、监控和告警。",
        ),
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(t("统计按独立安装和活跃安装计算，不代表自然人数。")),
    ).toBeTruthy();
    expect(consent().checked).toBe(false);
    expect(
      (
        screen.getByRole("button", {
          name: t("激活并创建管理员"),
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    submit(fields);
    expect(
      await screen.findByText(t("Activation consent is required")),
    ).toBeTruthy();
    expect(fields.map((input) => input.value)).toEqual(Object.values(draft));
  }
  expect(calls).toEqual([]);
  expect(JSON.stringify(localStorage)).not.toContain(draft.password);
  expect(JSON.stringify(localStorage)).not.toContain(draft.token);
});

it("keeps affirmative consent through language changes and sends the exact displayed destination once", async () => {
  render(<AuthView />);
  const fields = fillDraft();
  fireEvent.click(consent());
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(consent().checked).toBe(true);
    expect(fields.map((input) => input.value)).toEqual(Object.values(draft));
  }
  submit(fields);
  await waitFor(() => expect(appStore.getState().user?.id).toBe("1"));
  expect(calls.filter((call) => call.path === "/api/v1/setup")).toEqual([
    {
      path: "/api/v1/setup",
      body: {
        ...draft,
        usageConsent: true,
        usageNoticeVersion: "2",
        usageConsoleUrl: destination,
      },
    },
  ]);
  expect(calls.every((call) => call.path.startsWith("/api/v1/"))).toBe(true);
  expect(appStore.getState().initialized).toBe(true);
  expect(appStore.getState().activation).toBeNull();
});

it.each([
  {
    configured: false,
    consoleUrl: "",
    noticeVersion: "2",
    error: "Activation service is not configured" as const,
  },
  {
    configured: true,
    consoleUrl: destination,
    noticeVersion: "3",
    error: "Activation notice has changed; review it again" as const,
  },
])(
  "blocks unconfigured or unsupported activation: $error",
  async ({ error, ...overrides }) => {
    appStore.setState({ activation: { ...activation, ...overrides } });
    render(<AuthView />);
    const fields = fillDraft();
    expect(consent().disabled).toBe(true);
    expect(screen.getAllByText(t(error)).length).toBeGreaterThan(0);
    submit(fields);
    expect(calls).toEqual([]);
    expect(fields.map((input) => input.value)).toEqual(Object.values(draft));
  },
);

it.each([
  "Activation consent is required",
  "Activation notice has changed; review it again",
  "Activation destination has changed; review it again",
  "Activation service is not configured",
  "Activation service is unavailable; try again",
  "Activation request was rejected; try again",
] as const)(
  "keeps the draft and retranslates a rejected request: %s",
  async (error) => {
    responseError = error;
    render(<AuthView />);
    const fields = fillDraft();
    fireEvent.click(consent());
    submit(fields);
    expect(await screen.findByText(t(error))).toBeTruthy();
    for (const locale of locales) {
      await act(() => setLocale(locale.code));
      expect(screen.getByText(t(error))).toBeTruthy();
      expect(fields.map((input) => input.value)).toEqual(Object.values(draft));
    }
    expect(appStore.getState().initialized).toBe(false);
    expect(calls).toHaveLength(1);
  },
);

it.each([
  "Activation destination has changed; review it again",
  "Activation notice has changed; review it again",
])(
  "refreshes a changed notice without resetting the draft or resending consent: %s",
  async (error) => {
    responseError = error;
    render(<AuthView />);
    const fields = fillDraft();
    fireEvent.click(consent());
    submit(fields);
    await screen.findByRole("alert");
    expect(consent().checked).toBe(false);
    expect(consent().disabled).toBe(true);
    currentActivation = {
      ...activation,
      consoleUrl: "https://replacement.example.test",
    };
    fireEvent.click(screen.getByRole("button", { name: t("刷新激活说明") }));
    await screen.findByText(currentActivation.consoleUrl);
    expect(consent().checked).toBe(false);
    expect(fields.map((input) => input.value)).toEqual(Object.values(draft));
    expect(calls.map((call) => call.path)).toEqual([
      "/api/v1/setup",
      "/api/v1/status",
    ]);
    submit(fields);
    expect(calls).toHaveLength(2);
    responseError = "";
    fireEvent.click(consent());
    submit(fields);
    await waitFor(() => expect(appStore.getState().user?.id).toBe("1"));
    expect(
      calls.filter((call) => call.path === "/api/v1/setup")[1].body,
    ).toEqual({
      ...draft,
      usageConsent: true,
      usageNoticeVersion: "2",
      usageConsoleUrl: currentActivation.consoleUrl,
    });
  },
);

it("revokes consent when the visible destination changes without losing fields", async () => {
  render(<AuthView />);
  const fields = fillDraft();
  fireEvent.click(consent());
  await act(async () =>
    appStore.setState({
      activation: { ...activation, consoleUrl: "https://new.example.test" },
    }),
  );
  expect(consent().checked).toBe(false);
  expect(fields.map((input) => input.value)).toEqual(Object.values(draft));
  submit(fields);
  expect(calls).toHaveLength(0);
});

it("preserves the original setup request when activation is not required", async () => {
  appStore.setState({ activation: { ...activation, required: false } });
  render(<AuthView />);
  const fields = fillDraft();
  expect(screen.queryByRole("checkbox")).toBeNull();
  submit(fields);
  await waitFor(() => expect(appStore.getState().user?.id).toBe("1"));
  expect(calls[0]).toEqual({ path: "/api/v1/setup", body: draft });
});

it("never blocks sign-in on an already installed instance", async () => {
  appStore.setState({ initialized: true });
  render(<AuthView />);
  expect(screen.queryByRole("checkbox")).toBeNull();
  expect(screen.queryByText(destination)).toBeNull();
  fireEvent.change(screen.getByLabelText(t("用户名")), {
    target: { value: draft.username },
  });
  fireEvent.change(screen.getByLabelText(t("密码")), {
    target: { value: draft.password },
  });
  fireEvent.click(screen.getByRole("button", { name: t("登录") }));
  await waitFor(() => expect(appStore.getState().user?.id).toBe("1"));
  expect(calls[0]).toEqual({
    path: "/api/v1/auth/login",
    body: { username: draft.username, password: draft.password },
  });
});
