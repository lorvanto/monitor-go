import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import UsageSettings, {
  type UsageStatus,
} from "../src/components/UsageSettings";
import UsageConfirmationBanner from "../src/components/UsageConfirmationBanner";
import {
  createUsageActivityRecorder,
  sendUsageActivity,
} from "../src/lib/usageActivity";
import { appStore, refreshAll, type User } from "../src/stores/app";
import { locales, setLocale, t } from "../src/i18n";
import fixtures from "./api-fixtures.json";

const initial: UsageStatus = {
  enabled: false,
  required: true,
  noticeVersion: "2",
  confirmationRequired: true,
  configured: true,
  consoleUrl: "https://console.example.com",
  instanceId: "",
  lastReportAt: 0,
  nextReportAt: 0,
  lastError: "",
  account: null,
  pendingActiveDays: [],
  preview: {
    instanceId: "",
    version: "test-version",
    os: "linux",
    arch: "amd64",
    deployment: "native",
    activeDays: [],
  },
};
let status: UsageStatus;
const response = (data: unknown) =>
  new Response(JSON.stringify({ data }), { status: 200 });
beforeEach(async () => {
  status = structuredClone(initial);
  appStore.setState(
    {
      ...appStore.getInitialState(),
      user: fixtures["/me"] as User,
      booting: false,
    },
    true,
  );
  localStorage.clear();
  await setLocale("en-US");
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, init?: RequestInit) => {
      if (input === "/api/v1/usage") {
        if (init?.method === "PATCH") {
          status.enabled = JSON.parse(String(init.body)).enabled;
          status.confirmationRequired = !status.enabled;
        }
        return response(structuredClone(status));
      }
      if (input === "/api/v1/usage/bind") {
        status.account = { login: "demo-owner" };
        return response(structuredClone(status));
      }
      if (input === "/api/v1/usage/unbind") {
        status.account = null;
        return response(structuredClone(status));
      }
      throw new Error(`Unexpected usage request: ${input}`);
    }),
  );
});

it.each(locales)(
  "requires explicit confirmation and offers no reporting switch in $label",
  async ({ code }) => {
    await setLocale(code);
    render(<UsageSettings />);
    await screen.findByText(t("待管理员确认持续上报规则"));
    const enable = screen.getByRole("button", {
      name: t("确认规则并激活上报"),
    }) as HTMLButtonElement;
    expect(enable.disabled).toBe(true);
    expect(vi.mocked(fetch).mock.calls).toHaveLength(1);
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(enable);
    await screen.findByText(t("持续上报中"));
    expect(appStore.getState().usageEnabled).toBe(true);
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: /Turn off reporting|关闭上报|關閉回報|送信を停止|보고 끄기/,
      }),
    ).toBeNull();
    expect(appStore.getState().usageConfirmationRequired).toBe(false);
    expect(
      vi
        .mocked(fetch)
        .mock.calls.filter(([, init]) => init?.method === "PATCH")
        .map(([, init]) => JSON.parse(String(init?.body))),
    ).toEqual([
      {
        enabled: true,
        usageConsent: true,
        usageNoticeVersion: "2",
        usageConsoleUrl: initial.consoleUrl,
      },
    ]);
  },
);

it("keeps consent and an unsent binding code across all five languages", async () => {
  const view = render(<UsageSettings />);
  await screen.findByRole("checkbox");
  fireEvent.click(screen.getByRole("checkbox"));
  await act(async () => {
    await setLocale("ja-JP");
  });
  expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(true);
  fireEvent.click(
    screen.getByRole("button", { name: t("确认规则并激活上报") }),
  );
  await screen.findByLabelText(t("一次性绑定码"));
  const secret = "ab".repeat(32);
  fireEvent.change(screen.getByLabelText(t("一次性绑定码")), {
    target: { value: secret },
  });
  const before = vi.mocked(fetch).mock.calls.length;
  for (const locale of locales) {
    await act(async () => {
      await setLocale(locale.code);
    });
    expect(
      (screen.getByLabelText(t("一次性绑定码")) as HTMLInputElement).value,
    ).toBe(secret);
    expect(view.container.textContent).not.toContain(secret);
  }
  expect(vi.mocked(fetch).mock.calls).toHaveLength(before);
  fireEvent.click(screen.getByRole("button", { name: t("绑定 Console 账号") }));
  await screen.findByText("demo-owner");
  expect(
    vi
      .mocked(fetch)
      .mock.calls.filter(([path]) => path === "/api/v1/usage/bind")
      .map(([, init]) => JSON.parse(String(init?.body))),
  ).toEqual([{ code: secret }]);
  fireEvent.click(
    screen.getByRole("button", { name: t("解除 Console 账号绑定") }),
  );
  await screen.findByLabelText(t("一次性绑定码"));
  expect(
    (screen.getByLabelText(t("一次性绑定码")) as HTMLInputElement).value,
  ).toBe("");
});

it("does not offer enabling without a configured console or without permission", async () => {
  status.configured = false;
  status.consoleUrl = "javascript:alert(1)";
  const view = render(<UsageSettings />);
  await screen.findByText(t("尚未配置管理后台"));
  expect((screen.getByRole("checkbox") as HTMLInputElement).disabled).toBe(
    true,
  );
  expect(screen.queryByRole("link")).toBeNull();
  await act(async () => {
    appStore.setState({
      user: { ...fixtures["/me"], permissions: ["system:read"] } as User,
    });
  });
  expect(screen.queryByRole("checkbox")).toBeNull();
  expect(
    screen.queryByRole("button", { name: t("确认规则并激活上报") }),
  ).toBeNull();
  view.unmount();
});

it("translates service errors when language changes and preserves the entered code", async () => {
  status.enabled = true;
  status.confirmationRequired = false;
  render(<UsageSettings />);
  await screen.findByLabelText(t("一次性绑定码"));
  fireEvent.change(screen.getByLabelText(t("一次性绑定码")), {
    target: { value: "ab".repeat(32) },
  });
  vi.mocked(fetch).mockResolvedValueOnce(
    new Response(
      JSON.stringify({
        error: { key: "Invalid binding code", message: "fallback" },
      }),
      { status: 422 },
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: t("绑定 Console 账号") }));
  await screen.findByRole("alert");
  for (const locale of locales) {
    await act(async () => {
      await setLocale(locale.code);
    });
    expect(screen.getByRole("alert").textContent).toBe(
      t("Invalid binding code"),
    );
    expect(
      (screen.getByLabelText(t("一次性绑定码")) as HTMLInputElement).value,
    ).toBe("ab".repeat(32));
  }
});

it("keeps confirmation and the binding draft after a failed registration, without background resubmission", async () => {
  render(<UsageSettings />);
  await screen.findByRole("checkbox");
  const secret = "cd".repeat(32);
  fireEvent.change(screen.getByLabelText(t("一次性绑定码")), {
    target: { value: secret },
  });
  fireEvent.click(screen.getByRole("checkbox"));
  vi.mocked(fetch).mockResolvedValueOnce(
    new Response(
      JSON.stringify({
        error: {
          key: "Activation service is unavailable; try again",
          message: "fallback",
        },
      }),
      { status: 502 },
    ),
  );
  fireEvent.click(
    screen.getByRole("button", { name: t("确认规则并激活上报") }),
  );
  await screen.findByRole("alert");
  const requests = vi.mocked(fetch).mock.calls.length;
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(screen.getByRole("alert").textContent).toBe(
      t("Activation service is unavailable; try again"),
    );
    expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(
      true,
    );
    expect(
      (screen.getByLabelText(t("一次性绑定码")) as HTMLInputElement).value,
    ).toBe(secret);
  }
  expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);
  expect(appStore.getState().usageEnabled).toBe(false);
  expect(appStore.getState().usageConfirmationRequired).toBe(true);
});

it.each([
  "Activation destination has changed; review it again",
  "Activation notice has changed; review it again",
] as const)(
  "requires a refreshed notice and new consent after %s, preserving the binding draft",
  async (key) => {
    render(<UsageSettings />);
    await screen.findByRole("checkbox");
    const secret = "ef".repeat(32);
    fireEvent.change(screen.getByLabelText(t("一次性绑定码")), {
      target: { value: secret },
    });
    fireEvent.click(screen.getByRole("checkbox"));
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ error: { key, message: key } }), {
        status: 422,
      }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: t("确认规则并激活上报") }),
    );
    await screen.findByRole("alert");
    expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(
      false,
    );
    expect((screen.getByRole("checkbox") as HTMLInputElement).disabled).toBe(
      true,
    );
    status.consoleUrl = "https://replacement.example.com";
    fireEvent.click(screen.getByRole("button", { name: t("刷新上报状态") }));
    await screen.findByText(status.consoleUrl);
    expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(
      false,
    );
    expect(
      (screen.getByLabelText(t("一次性绑定码")) as HTMLInputElement).value,
    ).toBe(secret);
    expect(
      vi
        .mocked(fetch)
        .mock.calls.filter(([, init]) => init?.method === "PATCH"),
    ).toHaveLength(1);
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(
      screen.getByRole("button", { name: t("确认规则并激活上报") }),
    );
    await screen.findByText(t("持续上报中"));
    expect(
      vi
        .mocked(fetch)
        .mock.calls.filter(([, init]) => init?.method === "PATCH")
        .map(([, init]) => JSON.parse(String(init?.body))),
    ).toEqual([
      {
        enabled: true,
        usageConsent: true,
        usageNoticeVersion: "2",
        usageConsoleUrl: initial.consoleUrl,
      },
      {
        enabled: true,
        usageConsent: true,
        usageNoticeVersion: "2",
        usageConsoleUrl: status.consoleUrl,
      },
    ]);
    expect(
      (screen.getByLabelText(t("一次性绑定码")) as HTMLInputElement).value,
    ).toBe(secret);
  },
);

it("clears consent when refreshing a changed destination and never accepts an unsupported notice", async () => {
  render(<UsageSettings />);
  await screen.findByRole("checkbox");
  fireEvent.click(screen.getByRole("checkbox"));
  status.consoleUrl = "https://new.example.com";
  status.noticeVersion = "3";
  fireEvent.click(screen.getByRole("button", { name: t("刷新上报状态") }));
  await screen.findByText(status.consoleUrl);
  expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(
    false,
  );
  expect((screen.getByRole("checkbox") as HTMLInputElement).disabled).toBe(
    true,
  );
  expect(screen.getByRole("alert").textContent).toBe(
    t("Activation notice has changed; review it again"),
  );
  expect(
    vi.mocked(fetch).mock.calls.every(([, init]) => init?.method === "GET"),
  ).toBe(true);
});

it("does not ask already enabled instances to reconfirm historical notices or offer a stop control", async () => {
  status.enabled = true;
  status.confirmationRequired = false;
  status.noticeVersion = "1";
  render(<UsageSettings />);
  await screen.findByText(t("持续上报中"));
  expect(screen.queryByRole("checkbox")).toBeNull();
  expect(
    screen.queryByRole("button", { name: t("确认规则并激活上报") }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", {
      name: /Turn off reporting|关闭上报|關閉回報|送信を停止|보고 끄기/,
    }),
  ).toBeNull();
  expect(vi.mocked(fetch).mock.calls).toHaveLength(1);
});

it("shows a non-blocking, localized policy notice to administrators and members", async () => {
  appStore.setState({ usageConfirmationRequired: true });
  render(
    <>
      <UsageConfirmationBanner />
      <button>Monitor action</button>
    </>,
  );
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(screen.getByRole("status").textContent).toContain(
      t("待管理员确认持续上报规则"),
    );
    expect(screen.getByRole("button", { name: "Monitor action" })).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(
      (
        screen.getByRole("button", {
          name: t("查看上报规则"),
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false);
  }
  fireEvent.click(screen.getByRole("button", { name: t("查看上报规则") }));
  expect(appStore.getState().page).toBe("settings");
  expect(appStore.getState().usageSettingsRequested).toBe(true);
  await act(async () =>
    appStore.setState({
      user: { ...fixtures["/me"], permissions: ["monitor:read"] } as User,
    }),
  );
  expect(screen.queryByRole("button", { name: t("查看上报规则") })).toBeNull();
  expect(
    screen.getByText(t("请联系有系统设置权限的管理员完成确认。")),
  ).toBeTruthy();
  await act(async () =>
    appStore.setState({ usageConfirmationRequired: false }),
  );
  expect(screen.queryByRole("status")).toBeNull();
});

it("consumes the policy navigation intent after the settings card loads and focuses it only once", async () => {
  appStore.setState({ usageSettingsRequested: true });
  const scroll = vi.spyOn(HTMLElement.prototype, "scrollIntoView");
  render(<UsageSettings />);
  const title = screen.getByRole("heading", {
    name: t("使用情况统计与账号绑定"),
  });
  await waitFor(() => expect(document.activeElement).toBe(title));
  expect(appStore.getState().usageSettingsRequested).toBe(false);
  expect(scroll).toHaveBeenCalledExactlyOnceWith({ block: "start" });
  for (const locale of locales) await act(() => setLocale(locale.code));
  expect(scroll).toHaveBeenCalledTimes(1);
});

it("updates the global confirmation notice from authenticated polling", async () => {
  let pending = true;
  vi.mocked(fetch).mockImplementation(async () =>
    response({
      user: fixtures["/me"],
      usageEnabled: !pending,
      usageConfirmationRequired: pending,
      navigation: { items: [], instanceName: "" },
      hosts: [],
      monitors: [],
      alerts: [],
      openAlertCount: 0,
    }),
  );
  await refreshAll();
  expect(appStore.getState().usageConfirmationRequired).toBe(true);
  pending = false;
  await refreshAll();
  expect(appStore.getState().usageConfirmationRequired).toBe(false);
});

it("never records disabled, anonymous, hidden, automatic or untrusted activity", () => {
  const send = vi.fn(async () => {});
  const storage = { getItem: vi.fn(), setItem: vi.fn() };
  const current = { enabled: false, csrf: undefined as string | undefined };
  let visible = true;
  const record = createUsageActivityRecorder({
    state: () => current,
    send,
    storage,
    visible: () => visible,
  });
  record({ type: "pointerdown", isTrusted: true });
  current.enabled = true;
  record({ type: "pointerdown", isTrusted: true });
  current.csrf = "session-token";
  record({ type: "pointerdown", isTrusted: false });
  record({ type: "visibilitychange", isTrusted: true });
  record({ type: "load", isTrusted: true });
  visible = false;
  record({ type: "keydown", isTrusted: true });
  expect(send).not.toHaveBeenCalled();
  expect(storage.getItem).not.toHaveBeenCalled();
  expect(storage.setItem).not.toHaveBeenCalled();
});

it("sends only one coarse activity flag per UTC day, including failed attempts", async () => {
  const send = vi.fn().mockRejectedValue(new Error("offline"));
  let now = new Date("2026-11-03T23:59:00Z");
  const storage = { getItem: vi.fn(() => null), setItem: vi.fn() };
  const record = createUsageActivityRecorder({
    state: () => ({ enabled: true, csrf: "session-token" }),
    send,
    storage,
    visible: () => true,
    now: () => now,
  });
  record({ type: "pointerdown", isTrusted: true });
  record({ type: "keydown", isTrusted: true });
  await Promise.resolve();
  expect(send).toHaveBeenCalledExactlyOnceWith("session-token");
  now = new Date("2026-11-04T00:01:00Z");
  record({ type: "keydown", isTrusted: true });
  await Promise.resolve();
  expect(send).toHaveBeenCalledTimes(2);
  expect(storage.setItem.mock.calls.map((call) => call[1])).toEqual([
    "2026-11-03",
    "2026-11-04",
  ]);
});

it("deduplicates across remounts and still works when browser storage is blocked", async () => {
  const send = vi.fn(async () => {});
  const options = {
    state: () => ({ enabled: true, csrf: "token" }),
    send,
    now: () => new Date("2026-11-03T12:00:00Z"),
    visible: () => true,
    storage: localStorage,
  };
  createUsageActivityRecorder(options)({ type: "keydown", isTrusted: true });
  createUsageActivityRecorder(options)({
    type: "pointerdown",
    isTrusted: true,
  });
  expect(send).toHaveBeenCalledTimes(1);
  const storage = {
    getItem: () => {
      throw new Error("blocked");
    },
    setItem: () => {
      throw new Error("blocked");
    },
  };
  const record = createUsageActivityRecorder({ ...options, storage });
  record({ type: "keydown", isTrusted: true });
  record({ type: "pointerdown", isTrusted: true });
  await Promise.resolve();
  expect(send).toHaveBeenCalledTimes(2);
});

it("posts no event, identity or field content and bounds the background request", async () => {
  vi.mocked(fetch).mockResolvedValueOnce(response({ ok: true }));
  await sendUsageActivity("csrf-token");
  await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  const [url, init] = vi.mocked(fetch).mock.calls[0];
  expect(url).toBe("/api/v1/usage/activity");
  expect(init?.body).toBe("{}");
  expect(init?.method).toBe("POST");
  expect(init?.headers).toEqual({
    "Content-Type": "application/json",
    "X-CSRF-Token": "csrf-token",
  });
  expect(init?.signal).toBeInstanceOf(AbortSignal);
});
