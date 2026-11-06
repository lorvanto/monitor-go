import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import MonitorsView from "../src/views/MonitorsView";
import { appStore, type Monitor, type User } from "../src/stores/app";
import {
  defaultMonitorInterval,
  monitorKindLabel,
  monitorKinds,
} from "../src/lib/monitorKinds";
import { locales, setLocale, t } from "../src/i18n";
import fixtureJSON from "./api-fixtures.json";
import { fixtureData } from "./poll-fixture";

const original = appStore.getInitialState();
let fixtures: Record<string, any>;
let requests: { path: string; method: string; body: any }[];
let periodError = 0;
let warnings: ReturnType<typeof vi.spyOn>;
function monitor(
  type: string,
  id = type,
  overrides: Partial<Monitor> = {},
): Monitor {
  return {
    id,
    type,
    name: `${type} target`,
    target:
      type === "HTTP"
        ? "https://www.example.com/health"
        : "host.example.com:443",
    group: `${type} group`,
    interval: 60,
    timeout: 10,
    expected: 200,
    keyword: "",
    keywordInvert: false,
    latencyWarnMs: 0,
    threshold: 3,
    sslDays: 14,
    username: "",
    hasSecret: false,
    tls: false,
    enabled: true,
    status: "healthy",
    latency: 12,
    detail: "OK",
    checkedAt: Date.now() - 10000,
    nextCheck: Date.now() + 50000,
    certificateExpiresAt: 0,
    version: 2,
    uptime: 99,
    history: [],
    ...overrides,
  };
}
function seed(monitors: Monitor[]) {
  fixtures["/monitors"] = monitors;
  appStore.setState((state) => ({ data: { ...state.data, monitors } }));
}
function category(type: (typeof monitorKinds)[number]) {
  return screen.getByRole("tab", {
    name: new RegExp(t(monitorKindLabel[type])),
  });
}
function openCreate() {
  fireEvent.click(screen.getAllByRole("button", { name: t("新增检测") })[0]);
  return screen.getByRole("dialog");
}
beforeEach(async () => {
  fixtures = structuredClone(fixtureJSON);
  requests = [];
  periodError = 0;
  appStore.setState(
    {
      ...original,
      initialized: true,
      booting: false,
      user: fixtureJSON["/me"] as User,
      page: "monitors",
      data: {
        hosts: [],
        monitors: [],
        alerts: [],
        roles: [],
        users: [],
        permissions: [],
      },
    },
    true,
  );
  await setLocale("en-US");
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, init?: RequestInit) => {
      const path = input.replace("/api/v1", "").split("?")[0];
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      requests.push({ path, method: init?.method || "GET", body });
      if (path === "/monitors/interval" && periodError)
        return new Response(
          JSON.stringify({
            error: { message: "Timeout must be shorter than interval" },
          }),
          { status: periodError },
        );
      if (path === "/monitors/interval") {
        fixtures["/monitors"] = fixtures["/monitors"].map((m: Monitor) =>
          body.ids.includes(m.id)
            ? { ...m, interval: body.interval, version: m.version + 1 }
            : m,
        );
        return new Response(
          JSON.stringify({ data: { ok: true, updated: body.ids.length } }),
        );
      }
      return new Response(
        JSON.stringify({
          data: structuredClone(fixtureData(fixtures, input) ?? {}),
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }),
  );
  warnings = vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => expect(warnings.mock.calls).toEqual([]));

it("scopes targets, counts, groups, filters and selections to the visible category", async () => {
  seed(monitorKinds.map((type) => monitor(type)));
  render(<MonitorsView />);
  expect(category("HTTP").getAttribute("aria-selected")).toBe("true");
  expect(screen.getByText("HTTP target")).toBeTruthy();
  expect(screen.queryByText("TCP target")).toBeNull();
  const groups = screen.getByRole("combobox", { name: t("分组") });
  expect(
    within(groups).queryByRole("option", { name: "TCP group" }),
  ).toBeNull();
  expect(screen.getByRole("button", { name: `${t("全部")} 1` })).toBeTruthy();
  fireEvent.click(screen.getByLabelText(t("选择{p0}", { p0: "HTTP target" })));
  expect(screen.getByRole("button", { name: t("批量修改周期") })).toBeTruthy();
  fireEvent.change(screen.getByLabelText(t("搜索检测目标")), {
    target: { value: "missing" },
  });
  fireEvent.click(category("TCP"));
  expect(
    (screen.getByLabelText(t("搜索检测目标")) as HTMLInputElement).value,
  ).toBe("");
  expect(screen.getByText("TCP target")).toBeTruthy();
  expect(screen.queryByRole("button", { name: t("批量修改周期") })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: t("检测筛选结果") }));
  await waitFor(() =>
    expect(
      requests.filter((r) => r.method === "POST").map((r) => r.path),
    ).toEqual(["/monitors/TCP/checks"]),
  );
});

it.each(monitorKinds)(
  "creates %s with its own fields and default interval",
  (type) => {
    render(<MonitorsView />);
    fireEvent.click(category(type));
    const dialog = openCreate();
    expect(within(dialog).queryByLabelText(t("类型"))).toBeNull();
    expect(
      (within(dialog).getByLabelText(t("周期预设")) as HTMLSelectElement).value,
    ).toBe(String(defaultMonitorInterval[type]));
    expect(
      Boolean(within(dialog).queryByLabelText(t("连续异常几次告警"))),
    ).toBe(type !== "SSL");
    if (type === "SSL") {
      expect(
        (
          within(dialog).getByLabelText(
            t("证书到期前几天提醒"),
          ) as HTMLInputElement
        ).value,
      ).toBe("14");
      expect(
        within(dialog).getByText(
          t(
            "SSL 临期立即预警；连接失败后每 60 秒复查，复查两次仍失败才确认故障。",
          ),
        ),
      ).toBeTruthy();
    }
  },
);

it("preserves a custom cycle and unsaved fields through all language switches", async () => {
  render(<MonitorsView />);
  fireEvent.click(category("SSL"));
  const dialog = openCreate();
  fireEvent.change(within(dialog).getByLabelText(t("名称")), {
    target: { value: "Custom certificate" },
  });
  fireEvent.change(within(dialog).getByLabelText(t("周期预设")), {
    target: { value: "custom" },
  });
  fireEvent.change(within(dialog).getByLabelText(t("周期单位")), {
    target: { value: "3600" },
  });
  fireEvent.change(within(dialog).getByLabelText(t("周期数值")), {
    target: { value: "12" },
  });
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(
      (within(dialog).getByLabelText(t("名称")) as HTMLInputElement).value,
    ).toBe("Custom certificate");
    expect(
      (within(dialog).getByLabelText(t("周期数值")) as HTMLInputElement).value,
    ).toBe("12");
    expect(
      (within(dialog).getByLabelText(t("周期单位")) as HTMLSelectElement).value,
    ).toBe("3600");
  }
  fireEvent.change(
    within(dialog).getByLabelText(t("目标地址"), { exact: false }),
    { target: { value: "www.example.com:443" } },
  );
  fireEvent.submit(dialog.querySelector("form")!);
  await waitFor(() =>
    expect(
      requests.find((r) => r.path === "/monitors" && r.method === "POST")?.body,
    ).toMatchObject({ type: "SSL", interval: 43200, sslDays: 14 }),
  );
  expect(category("SSL").getAttribute("aria-selected")).toBe("true");
});

it("keeps existing intervals when editing and supports changing the edited category", async () => {
  seed([monitor("TCP", "custom", { interval: 77 })]);
  render(<MonitorsView />);
  fireEvent.click(
    screen.getByRole("button", {
      name: t("编辑检测{p0}", { p0: "TCP target" }),
    }),
  );
  const dialog = screen.getByRole("dialog");
  expect(
    (within(dialog).getByLabelText(t("周期预设")) as HTMLSelectElement).value,
  ).toBe("custom");
  expect(
    (within(dialog).getByLabelText(t("周期数值")) as HTMLInputElement).value,
  ).toBe("77");
  fireEvent.change(within(dialog).getByLabelText(t("类型")), {
    target: { value: "SSL" },
  });
  expect(
    (within(dialog).getByLabelText(t("周期数值")) as HTMLInputElement).value,
  ).toBe("77");
  fireEvent.submit(dialog.querySelector("form")!);
  await waitFor(() =>
    expect(
      requests.find((r) => r.path === "/monitors/custom" && r.method === "PUT")
        ?.body,
    ).toMatchObject({ type: "SSL", interval: 77, version: 2 }),
  );
  expect(category("SSL").getAttribute("aria-selected")).toBe("true");
});

it("shows structured certificate expiry and unknown state without parsing diagnostics", () => {
  const expiry = Date.now() + 5 * 86400000;
  seed([
    monitor("SSL", "known", { name: "Known", certificateExpiresAt: expiry }),
    monitor("SSL", "unknown", {
      name: "Unknown",
      detail: "certificate expires tomorrow",
    }),
  ]);
  render(<MonitorsView />);
  expect(category("SSL").getAttribute("aria-selected")).toBe("true");
  expect(
    screen.getByRole("columnheader", { name: t("证书到期时间") }),
  ).toBeTruthy();
  expect(screen.getByText(t("剩余 {p0} 天", { p0: 5 }))).toBeTruthy();
  expect(screen.getByText(t("尚无证书信息"))).toBeTruthy();
  expect(screen.queryByLabelText(t("可用率周期"))).toBeNull();
  expect(
    screen.queryByText(t("信号条从左到右由旧到新，异常越严重柱越高")),
  ).toBeNull();
});

it("opens a cross-page target in its category and never changes an explicitly chosen tab during refresh", async () => {
  const monitors = monitorKinds.map((type) => monitor(type));
  seed(monitors);
  appStore.setState({ activeMonitor: "SSL" });
  render(<MonitorsView />);
  expect(category("SSL").getAttribute("aria-selected")).toBe("true");
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect(appStore.getState().activeMonitor).toBe("");
  fireEvent.click(
    within(screen.getByRole("dialog")).getAllByRole("button", {
      name: t("关闭"),
    })[0],
  );
  fireEvent.click(category("Redis"));
  await act(() => {
    seed([...monitors]);
  });
  expect(category("Redis").getAttribute("aria-selected")).toBe("true");
});

it("applies a batch cycle only to selected current-category IDs and preserves errors for retry", async () => {
  seed([monitor("HTTP", "a"), monitor("HTTP", "b"), monitor("SSL", "other")]);
  render(<MonitorsView />);
  fireEvent.click(screen.getByLabelText(t("选择当前列表全部检测")));
  fireEvent.click(screen.getByRole("button", { name: t("批量修改周期") }));
  const dialog = screen.getByRole("dialog");
  fireEvent.change(within(dialog).getByLabelText(t("周期预设")), {
    target: { value: "custom" },
  });
  fireEvent.change(within(dialog).getByLabelText(t("周期数值")), {
    target: { value: "3" },
  });
  expect(
    (within(dialog).getByLabelText(t("周期单位")) as HTMLSelectElement).value,
  ).toBe("60");
  periodError = 422;
  fireEvent.submit(dialog.querySelector("form")!);
  await screen.findByRole("alert");
  expect(requests.find((r) => r.path === "/monitors/interval")?.body).toEqual({
    ids: ["a", "b"],
    interval: 180,
  });
  expect(screen.getByRole("dialog")).toBe(dialog);
  periodError = 0;
  fireEvent.submit(dialog.querySelector("form")!);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(fixtures["/monitors"].map((m: Monitor) => m.interval)).toEqual([
    180, 180, 60,
  ]);
  expect(category("HTTP").getAttribute("aria-selected")).toBe("true");
});

it("validates custom periods and hides write controls from read-only users", () => {
  seed([monitor("HTTP")]);
  render(<MonitorsView />);
  const dialog = openCreate();
  fireEvent.change(within(dialog).getByLabelText(t("周期预设")), {
    target: { value: "custom" },
  });
  fireEvent.change(within(dialog).getByLabelText(t("周期单位")), {
    target: { value: "1" },
  });
  fireEvent.change(within(dialog).getByLabelText(t("周期数值")), {
    target: { value: "14" },
  });
  expect(
    (
      within(dialog).getByRole("button", {
        name: t("创建检测"),
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  fireEvent.click(within(dialog).getByRole("button", { name: t("取消") }));
  act(() =>
    appStore.setState((state) => ({
      user: { ...state.user!, permissions: ["monitor:read"] },
    })),
  );
  fireEvent.click(screen.getByLabelText(t("选择当前列表全部检测")));
  expect(screen.queryByRole("button", { name: t("批量修改周期") })).toBeNull();
  expect(screen.queryByRole("button", { name: t("新增检测") })).toBeNull();
});
