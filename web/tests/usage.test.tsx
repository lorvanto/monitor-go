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
import {
  createUsageActivityRecorder,
  sendUsageActivity,
} from "../src/lib/usageActivity";
import { appStore, type User } from "../src/stores/app";
import { locales, setLocale, t } from "../src/i18n";
import fixtures from "./api-fixtures.json";

const initial: UsageStatus = {
  enabled: false,
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
        if (init?.method === "PATCH")
          status.enabled = JSON.parse(String(init.body)).enabled;
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
  "requires explicit opt-in and allows immediate disable in $label",
  async ({ code }) => {
    await setLocale(code);
    render(<UsageSettings />);
    await screen.findByText(t("上报已关闭"));
    const enable = screen.getByRole("button", {
      name: t("启用使用情况上报"),
    }) as HTMLButtonElement;
    expect(enable.disabled).toBe(true);
    expect(vi.mocked(fetch).mock.calls).toHaveLength(1);
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(enable);
    await screen.findByText(t("已启用上报"));
    expect(appStore.getState().usageEnabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: t("立即关闭上报") }));
    await screen.findByText(t("上报已关闭"));
    expect(appStore.getState().usageEnabled).toBe(false);
    expect(
      vi
        .mocked(fetch)
        .mock.calls.filter(([, init]) => init?.method === "PATCH")
        .map(([, init]) => JSON.parse(String(init?.body))),
    ).toEqual([{ enabled: true }, { enabled: false }]);
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
  fireEvent.click(screen.getByRole("button", { name: t("启用使用情况上报") }));
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
    screen.queryByRole("button", { name: t("启用使用情况上报") }),
  ).toBeNull();
  view.unmount();
});

it("translates service errors when language changes and preserves the entered code", async () => {
  status.enabled = true;
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
