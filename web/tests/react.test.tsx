import { fixtureData } from "./poll-fixture";
import { StrictMode, useState } from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import App from "../src/App";
import AuthView from "../src/views/AuthView";
import OverviewView from "../src/views/OverviewView";
import HostView from "../src/views/HostView";
import MonitorsView from "../src/views/MonitorsView";
import AlertsView from "../src/views/AlertsView";
import RolesView from "../src/views/RolesView";
import FilesView from "../src/views/FilesView";
import MenusView from "../src/views/MenusView";
import AuditView from "../src/views/AuditView";
import SettingsView from "../src/views/SettingsView";
import ApplicationsView from "../src/views/ApplicationsView";
import ReleasesView from "../src/views/ReleasesView";
import CodesView from "../src/views/CodesView";
import Modal from "../src/components/Modal";
import AgentAccess from "../src/components/AgentAccess";
import NotificationsPanel from "../src/components/NotificationsPanel";
import LineChart from "../src/components/LineChart";
import { api, appStore, navigate, type User } from "../src/stores/app";
import { setLocale, locales, t } from "../src/i18n";
import fixtureJSON from "./api-fixtures.json";
let fixtures: Record<string, any>;
const original = appStore.getInitialState();
let requests: { path: string; method: string; body: unknown }[];
let errors: ReturnType<typeof vi.spyOn>;
beforeEach(async () => {
  appStore.setState(
    {
      ...original,
      user: fixtureJSON["/me"] as User,
      initialized: true,
      booting: false,
      data: {
        hosts: [],
        monitors: [],
        alerts: [],
        roles: fixtureJSON["/roles"],
        users: fixtureJSON["/users"],
        permissions: fixtureJSON["/permissions"],
      },
    },
    true,
  );
  await setLocale("en-US");
  requests = [];
  fixtures = structuredClone(fixtureJSON);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, init?: RequestInit) => {
      const path = input.replace("/api/v1", "").split("?")[0];
      requests.push({
        path,
        method: init?.method || "GET",
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
      });
      return new Response(
        JSON.stringify({
          data: structuredClone(fixtureData(fixtures, input) ?? {}),
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }),
  );
  errors = vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => {
  expect(errors.mock.calls).toEqual([]);
});
const pages = {
  AuthView,
  OverviewView,
  HostView,
  MonitorsView,
  AlertsView,
  RolesView,
  FilesView,
  MenusView,
  AuditView,
  SettingsView,
  ApplicationsView,
  ReleasesView,
  CodesView,
};
describe.each(locales)("$code page rendering", ({ code }) => {
  it.each(Object.entries(pages))(
    "%s renders without React warnings and unmounts",
    async (_name, Page) => {
      await setLocale(code);
      const view = render(<Page />);
      await act(async () => {});
      expect(view.container.textContent?.trim().length).toBeGreaterThan(10);
      expect(view.container.textContent).not.toContain("undefined");
      view.unmount();
      await act(async () => {});
    },
  );
});
it("keeps typed login fields across all five language switches", async () => {
  render(<AuthView />);
  const username = screen.getByLabelText(t("用户名"));
  const password = screen.getByLabelText(t("密码"));
  fireEvent.change(username, { target: { value: "react-user" } });
  fireEvent.change(password, { target: { value: "test-only-password" } });
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect((username as HTMLInputElement).value).toBe("react-user");
    expect((password as HTMLInputElement).value).toBe("test-only-password");
    expect(document.documentElement.lang).toBe(locale.code);
  }
  fireEvent.submit(username.closest("form")!);
  await waitFor(() =>
    expect(requests.some((r) => r.path === "/auth/login")).toBe(true),
  );
  expect(requests.find((r) => r.path === "/auth/login")?.body).toEqual({
    username: "react-user",
    password: "test-only-password",
  });
  await act(async () => {});
});
it("consumes create-monitor navigation intent once and keeps edits on refresh", async () => {
  appStore.setState({ createMonitorRequested: true });
  render(<MonitorsView />);
  const dialog = screen.getByRole("dialog");
  expect(appStore.getState().createMonitorRequested).toBe(false);
  const input = within(dialog).getByPlaceholderText(t("例如：订单服务"));
  fireEvent.change(input, { target: { value: "Demo monitor" } });
  await act(async () => {
    appStore.setState({ updatedAt: 1 });
  });
  expect((input as HTMLInputElement).value).toBe("Demo monitor");
  fireEvent.click(within(dialog).getByRole("button", { name: t("取消") }));
  expect(screen.queryByRole("dialog")).toBeNull();
});
it("modal traps focus, restores it, and closes only the upper dialog", () => {
  function Dialogs() {
    const [outer, setOuter] = useState(false),
      [inner, setInner] = useState(false);
    return (
      <>
        <button onClick={() => setOuter(true)}>Open</button>
        {outer && (
          <Modal title="Outer" onClose={() => setOuter(false)}>
            <input aria-label="Outer input" />
            <button onClick={() => setInner(true)}>Nested</button>
            {inner && (
              <Modal title="Inner" onClose={() => setInner(false)}>
                <input aria-label="Inner input" />
              </Modal>
            )}
          </Modal>
        )}
      </>
    );
  }
  render(<Dialogs />);
  const opener = screen.getByText("Open");
  opener.focus();
  fireEvent.click(opener);
  expect(document.activeElement).toBe(screen.getByLabelText("Outer input"));
  const nested = screen.getByText("Nested");
  nested.focus();
  fireEvent.click(nested);
  expect(document.activeElement).toBe(screen.getByLabelText("Inner input"));
  expect(document.body.style.overflow).toBe("hidden");
  fireEvent.keyDown(document, { key: "Tab" });
  expect(document.activeElement).toBe(
    within(screen.getByRole("dialog", { name: "Inner" })).getByRole("combobox"),
  );
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.queryByRole("dialog", { name: "Inner" })).toBeNull();
  expect(screen.getByRole("dialog", { name: "Outer" })).toBeTruthy();
  expect(document.activeElement).toBe(nested);
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(opener);
  expect(document.body.style.overflow).toBe("");
});
it("chart handles keyboard selection and gaps without inventing samples", () => {
  const end = 1700000000000;
  const { container } = render(
    <LineChart
      label="Latency"
      hours={1}
      end={end}
      series={[
        {
          name: "HTTP",
          color: "#1a6",
          points: [
            { at: end - 3000000, value: 12 },
            { at: end - 2980000, value: 18 },
            { at: end - 1000, value: null },
            { at: end, value: 24 },
          ],
        },
      ]}
    />,
  );
  const chart = container.querySelector(".chart")!;
  fireEvent.keyDown(chart, { key: "Home" });
  expect(container.querySelector("[aria-live]")?.textContent).toContain("12");
  fireEvent.keyDown(chart, { key: "End" });
  expect(container.querySelector("[aria-live]")?.textContent).toContain("24");
  expect(container.querySelectorAll(".chart-line").length).toBe(1);
  expect(container.querySelectorAll(".chart-dot").length).toBe(1);
  fireEvent.keyDown(chart, { key: "Escape" });
  expect(container.querySelector(".chart-tip")).toBeNull();
});
it("denies unauthorized navigation and clears cached data on session expiry", async () => {
  appStore.setState({
    user: { ...fixtureJSON["/me"], permissions: [] } as User,
    page: "overview",
  });
  navigate("settings");
  expect(appStore.getState().page).toBe("overview");
  vi.mocked(fetch).mockResolvedValueOnce(
    new Response(JSON.stringify({ error: { message: "Expired" } }), {
      status: 401,
    }),
  );
  await expect(api("/me")).rejects.toThrow("Expired");
  expect(appStore.getState().user).toBeNull();
  expect(appStore.getState().data.roles).toEqual([]);
});
it("boots the shell and responds to browser hash navigation", async () => {
  render(<App />);
  await waitFor(() => expect(screen.queryByRole("main")).toBeTruthy());
  await act(async () => {
    location.hash = "#codes";
    window.dispatchEvent(new HashChangeEvent("hashchange"));
  });
  await waitFor(() =>
    expect(screen.getAllByText(t("二维码")).length).toBeGreaterThan(0),
  );
  expect(appStore.getState().page).toBe("codes");
});
it("submits application text and boolean inputs without resetting on language change", async () => {
  render(<ApplicationsView />);
  await act(async () => {});
  fireEvent.click(screen.getAllByRole("button", { name: t("创建应用") })[0]);
  fireEvent.change(screen.getByPlaceholderText(t("例如：运维桌面端")), {
    target: { value: "Desktop" },
  });
  fireEvent.change(screen.getByPlaceholderText("ops-desktop"), {
    target: { value: "demo-desktop" },
  });
  fireEvent.click(screen.getByLabelText(t("启用应用")));
  await act(() => setLocale("ja-JP"));
  fireEvent.click(screen.getByRole("button", { name: t("保存应用") }));
  await waitFor(() =>
    expect(
      requests.find((r) => r.path === "/applications" && r.method === "POST")
        ?.body,
    ).toMatchObject({ name: "Desktop", slug: "demo-desktop", enabled: false }),
  );
  await act(async () => {});
});
it("creates a folder through the modal form", async () => {
  render(<FilesView />);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: t("新建文件夹") }));
  fireEvent.change(screen.getByPlaceholderText(t("输入名称")), {
    target: { value: "Builds" },
  });
  fireEvent.click(screen.getByRole("button", { name: t("创建文件夹") }));
  await waitFor(() =>
    expect(
      requests.find((r) => r.path === "/files/folders")?.body,
    ).toMatchObject({ name: "Builds" }),
  );
  await act(async () => {});
});
it("serializes menu visibility as a boolean and order as a number", async () => {
  render(<MenusView />);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: t("新增菜单") }));
  fireEvent.change(screen.getByPlaceholderText(t("例如：运维手册")), {
    target: { value: "Docs" },
  });
  fireEvent.change(screen.getByLabelText(t("类型")), {
    target: { value: "group" },
  });
  const dialog = screen.getByRole("dialog");
  const visibility = dialog.querySelector(
    'select option[value="false"]',
  )?.parentElement;
  expect(visibility).toBeTruthy();
  fireEvent.change(visibility!, { target: { value: "false" } });
  fireEvent.submit(dialog.querySelector("form")!);
  await waitFor(() =>
    expect(
      requests.find((r) => r.path === "/menus" && r.method === "POST")?.body,
    ).toMatchObject({ name: "Docs", kind: "group", visible: false, sort: 10 }),
  );
  await act(async () => {});
});
it("preserves dirty permissions across polling and applies a checkbox group", async () => {
  const role = {
    id: "test-role",
    name: "Operators",
    description: "",
    builtin: false,
    version: 1,
    members: 0,
    permissions: [],
  };
  appStore.setState((s) => ({ data: { ...s.data, roles: [role] } }));
  render(<RolesView />);
  await act(async () => {});
  const group = screen
    .getAllByRole("checkbox")
    .find((el) => el.getAttribute("aria-label")?.startsWith("Select all"))!;
  expect(group).toBeTruthy();
  fireEvent.click(group);
  expect((group as HTMLInputElement).checked).toBe(true);
  await act(async () => {
    appStore.setState((s) => ({
      data: { ...s.data, roles: [{ ...role }] },
      updatedAt: 1,
    }));
  });
  expect((group as HTMLInputElement).checked).toBe(true);
});
it("generates codes with numeric options and invalidates download after edits", async () => {
  fixtures["/tools/codes"] = {
    kind: "qr",
    content: "hello",
    width: 400,
    height: 400,
    png: "aGVsbG8=",
    svg: "aGVsbG8=",
  };
  render(<CodesView />);
  const input = screen.getByPlaceholderText(t("网址、文本或其他内容"));
  fireEvent.change(input, { target: { value: "hello" } });
  fireEvent.submit(input.closest("form")!);
  await waitFor(() =>
    expect(requests.find((r) => r.path === "/tools/codes")?.body).toMatchObject(
      { content: "hello", kind: "qr", width: 400, level: "M" },
    ),
  );
  await act(async () => {});
  const download = screen.getByRole("button", { name: /PNG/ });
  expect((download as HTMLButtonElement).disabled).toBe(false);
  fireEvent.change(input, { target: { value: "changed" } });
  expect((download as HTMLButtonElement).disabled).toBe(true);
});
it("keeps SMTP configuration typed and splits recipients before saving", async () => {
  const { default: NotificationsPanel } = await import(
    "../src/components/NotificationsPanel"
  );
  render(<NotificationsPanel mode="channels" />);
  await act(async () => {});
  fireEvent.click(screen.getAllByRole("button", { name: t("添加渠道") })[0]);
  fireEvent.change(screen.getByLabelText(t("通知方式")), {
    target: { value: "smtp" },
  });
  fireEvent.change(screen.getByLabelText(t("渠道名称")), {
    target: { value: "Ops mail" },
  });
  fireEvent.change(screen.getByLabelText(t("SMTP 主机")), {
    target: { value: "smtp.example.com" },
  });
  fireEvent.change(screen.getByLabelText(t("端口")), {
    target: { value: "587" },
  });
  fireEvent.change(screen.getByLabelText(t("连接安全")), {
    target: { value: "starttls" },
  });
  fireEvent.change(screen.getByLabelText(t("收件邮箱"), { exact: false }), {
    target: { value: "a@example.com;b@example.com" },
  });
  fireEvent.click(screen.getByRole("button", { name: t("保存渠道") }));
  await waitFor(() =>
    expect(
      requests.find(
        (r) => r.path === "/notification-channels" && r.method === "POST",
      )?.body,
    ).toMatchObject({
      kind: "smtp",
      name: "Ops mail",
      config: {
        host: "smtp.example.com",
        port: 587,
        security: "starttls",
        to: ["a@example.com", "b@example.com"],
      },
    }),
  );
  await act(async () => {});
});
it("sends current locale, CSRF and same-origin credentials with mutations", async () => {
  await setLocale("ko-KR");
  await api("/settings", "PATCH", { instanceName: "Team" });
  expect(fetch).toHaveBeenLastCalledWith(
    "/api/v1/settings",
    expect.objectContaining({
      method: "PATCH",
      credentials: "same-origin",
      headers: expect.objectContaining({
        "Accept-Language": "ko-KR",
        "X-CSRF-Token": "test-csrf",
      }),
      body: JSON.stringify({ instanceName: "Team" }),
    }),
  );
});
it("requires confirmation before deleting a custom menu", async () => {
  fixtures["/menus"].items.push({
    id: "custom-menu",
    name: "Temporary",
    kind: "group",
    parentId: "",
    visible: true,
    icon: "FolderOpen",
    version: 3,
    sort: 99,
    builtin: false,
  });
  render(<MenusView />);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: t("删除") }));
  expect(requests.some((r) => r.method === "DELETE")).toBe(false);
  fireEvent.click(
    within(screen.getByRole("dialog")).getByRole("button", {
      name: t("删除菜单"),
    }),
  );
  await waitFor(() =>
    expect(
      requests.find(
        (r) => r.path === "/menus/custom-menu" && r.method === "DELETE",
      )?.body,
    ).toEqual({ version: 3 }),
  );
  await act(async () => {});
});
it("opens key revocation confirmation and sends the selected version", async () => {
  fixtures["/applications"] = [
    {
      id: "app-1",
      name: "Desktop",
      slug: "desktop",
      enabled: true,
      activeKeys: 1,
      releases: 0,
      version: 1,
    },
  ];
  fixtures["/applications/app-1/keys"] = {
    items: [
      {
        id: "key-1",
        name: "Deploy",
        prefix: "test",
        scopes: ["host:read"],
        version: 2,
        expiresAt: 1900000000000,
        createdAt: 1700000000000,
        revokedAt: 0,
        lastUsedAt: 0,
      },
    ],
    scopes: [],
  };
  render(<ApplicationsView />);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: t("撤销") }));
  expect(requests.some((r) => r.method === "DELETE")).toBe(false);
  fireEvent.click(
    within(screen.getByRole("dialog")).getByRole("button", {
      name: t("撤销密钥"),
    }),
  );
  await waitFor(() =>
    expect(
      requests.find(
        (r) => r.path === "/application-keys/key-1" && r.method === "DELETE",
      )?.body,
    ).toEqual({ version: 2 }),
  );
  await act(async () => {});
});
it("requests the next release page after pagination changes", async () => {
  fixtures["/applications"] = [
    { id: "app-1", name: "Desktop", slug: "desktop", enabled: true },
  ];
  fixtures["/releases"] = {
    items: [
      {
        id: "release-1",
        applicationId: "app-1",
        applicationName: "Desktop",
        name: "Desktop",
        versionName: "1.0",
        platform: "linux",
        arch: "amd64",
        source: "external",
        state: "draft",
        version: 1,
        downloadCount: 0,
      },
    ],
    total: 80,
  };
  render(<ReleasesView />);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: t("下一页") }));
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith(
      expect.stringMatching(/\/releases\?page=2&/),
      expect.anything(),
    ),
  );
  await act(async () => {});
});
it("clears all audit filters and reloads the unfiltered results", async () => {
  render(<AuditView />);
  await act(async () => {});
  const field = screen.getByRole("textbox");
  fireEvent.change(field, { target: { value: "role" } });
  const before = requests.filter((r) => r.path === "/audit").length;
  fireEvent.click(screen.getByRole("button", { name: t("清除条件") }));
  await waitFor(() =>
    expect(requests.filter((r) => r.path === "/audit").length).toBe(before + 1),
  );
  expect((field as HTMLInputElement).value).toBe("");
  await act(async () => {});
});

it("uploads files after effect cleanup and remount in StrictMode", async () => {
  const open = vi.fn(),
    send = vi.fn(),
    abort = vi.fn();
  vi.stubGlobal(
    "XMLHttpRequest",
    class {
      upload = {};
      open = open;
      send = send;
      abort = abort;
      setRequestHeader() {}
    },
  );
  const view = render(
    <StrictMode>
      <FilesView />
    </StrictMode>,
  );
  await act(async () => {});
  fireEvent.change(screen.getByLabelText(t("选择要上传的文件")), {
    target: {
      files: [new File(["content"], "fixture.txt", { type: "text/plain" })],
    },
  });
  expect(open).toHaveBeenCalledWith("POST", "/api/v1/files/uploads?parent=1");
  expect(send).toHaveBeenCalledWith(expect.any(FormData));
  view.unmount();
  expect(abort).toHaveBeenCalled();
});
it("locks published package identity while allowing notes to change", async () => {
  fixtures["/applications"] = [
    { id: "app-1", name: "Desktop", slug: "desktop", enabled: true },
  ];
  fixtures["/releases"] = {
    items: [
      {
        id: "release-1",
        applicationId: "app-1",
        applicationName: "Desktop",
        name: "Desktop",
        versionName: "1.0",
        platform: "linux",
        arch: "amd64",
        source: "external",
        downloadUrl: "https://example.com/app.zip",
        state: "published",
        everPublished: true,
        notes: "Initial",
        version: 1,
        downloadCount: 0,
      },
    ],
    total: 1,
  };
  render(<ReleasesView />);
  await act(async () => {});
  fireEvent.click(
    screen.getByRole("button", {
      name: t("编辑 {p0} {p1}", { p0: "Desktop", p1: "1.0" }),
    }),
  );
  const dialog = screen.getByRole("dialog");
  for (const label of ["所属应用", "版本号", "平台", "架构"] as const)
    expect(
      (within(dialog).getByLabelText(t(label)) as HTMLInputElement).disabled,
    ).toBe(true);
  expect(
    within(dialog).getByLabelText(t("版本号")).getAttribute("pattern"),
  ).toBe("[0-9A-Za-z][0-9A-Za-z._+\\-]{0,63}");
  expect(
    (dialog.querySelector("textarea") as HTMLTextAreaElement).disabled,
  ).toBe(false);
});
it.each(locales)(
  "renders populated host metrics and incident navigation in $code",
  async ({ code }) => {
    await setLocale(code);
    const now = Date.now();
    const host = {
      id: "host-1",
      hostname: "fixture-host",
      alias: "Database",
      version: 1,
      source: "agent" as const,
      status: "offline" as const,
      agentEnabled: true,
      agentVersion: "0.4.0",
      enrolledAt: now - 100000,
      lastSeen: now - 200000,
      enrollmentExpires: 0,
      inventory: {
        os: "linux",
        kernel: "6.8",
        arch: "amd64",
        cpu: "Test CPU",
        cores: 4,
        memory: 8589934592,
        ips: ["192.0.2.10"],
        bootTime: now - 1000000,
        source: "agent",
      },
      snapshot: {
        cpu: 25,
        memory: 40,
        memoryUsed: 3435973837,
        uptime: 1000,
        netIn: 1024,
        netOut: 2048,
        disks: [
          {
            path: "/",
            filesystem: "ext4",
            used: 100,
            total: 1000,
            percent: 10,
          },
        ],
        errors: [],
      },
    };
    const alert = {
      id: "alert-1",
      kind: "host" as const,
      resourceId: "host-1",
      resource: "Database",
      title: "Host offline",
      description: "No samples",
      severity: "critical" as const,
      state: "open" as const,
      openedAt: now - 100000,
      recoveredAt: null,
      owner: "",
      silenceUntil: 0,
      version: 1,
      activity: [],
    };
    fixtures["/hosts/host-1/metrics"] = [
      { at: now - 10000, cpu: 25, memory: 40, netIn: 1024, netOut: 2048 },
    ];
    appStore.setState((s) => ({
      data: { ...s.data, hosts: [host], alerts: [alert] },
    }));
    render(<HostView />);
    await act(async () => {});
    expect(screen.getAllByText("192.0.2.10").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: t("处理告警") }));
    expect(appStore.getState().page).toBe("alerts");
    expect(appStore.getState().activeAlert).toBe("alert-1");
  },
);

function editableMonitor() {
  return {
    id: "71",
    name: "Database probe",
    type: "MySQL",
    target: "db.example.com:3306",
    group: "Operations",
    interval: 120,
    timeout: 5,
    expected: 200,
    keyword: "",
    keywordInvert: false,
    latencyWarnMs: 0,
    threshold: 2,
    sslDays: 14,
    username: "probe",
    hasSecret: true,
    tls: true,
    enabled: true,
    status: "healthy" as const,
    latency: 10,
    detail: "OK",
    checkedAt: 1,
    nextCheck: 2,
    version: 4,
    uptime: 100,
    history: [],
  };
}
function showEditableMonitor() {
  const monitor = editableMonitor();
  fixtures["/monitors"] = [monitor];
  appStore.setState((state) => ({
    data: { ...state.data, monitors: [monitor] },
  }));
  render(<MonitorsView />);
  return monitor;
}
it("prefills monitor edits, keeps blank credentials and preserves input across languages", async () => {
  const monitor = showEditableMonitor();
  fireEvent.click(
    screen.getByRole("button", {
      name: t("编辑检测{p0}", { p0: monitor.name }),
    }),
  );
  const dialog = screen.getByRole("dialog");
  const name = within(dialog).getByLabelText(t("名称")) as HTMLInputElement;
  const password = within(dialog).getByLabelText(t("密码"), {
    exact: false,
  }) as HTMLInputElement;
  expect(name.value).toBe(monitor.name);
  expect(password.value).toBe("");
  fireEvent.change(name, { target: { value: "Edited probe" } });
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(name.value).toBe("Edited probe");
    expect(password.value).toBe("");
  }
  await act(() => setLocale("en-US"));
  fireEvent.submit(name.closest("form")!);
  await waitFor(() =>
    expect(requests.some((r) => r.method === "PUT")).toBe(true),
  );
  const sent = requests.find((r) => r.method === "PUT");
  expect(sent?.path).toBe("/monitors/71");
  expect(sent?.body).toMatchObject({
    name: "Edited probe",
    target: monitor.target,
    version: 4,
    password: "",
    clearPassword: false,
  });
  await act(async () => {});
});
it("confirms archive and submits the version captured when opened", async () => {
  const monitor = showEditableMonitor();
  fireEvent.click(
    screen.getByRole("button", {
      name: t("归档检测{p0}", { p0: monitor.name }),
    }),
  );
  expect(requests.some((r) => r.method === "DELETE")).toBe(false);
  await act(async () => {
    appStore.setState((state) => ({
      data: { ...state.data, monitors: [{ ...monitor, version: 5 }] },
    }));
  });
  fireEvent.click(screen.getByRole("button", { name: t("确认归档") }));
  await waitFor(() =>
    expect(requests.find((r) => r.method === "DELETE")?.body).toEqual({
      version: 4,
    }),
  );
  await act(async () => {});
});
it("preserves monitor edits and shows a conflict while refreshing server data", async () => {
  const monitor = showEditableMonitor();
  const normalFetch = globalThis.fetch;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, init?: RequestInit) => {
      if (init?.method === "PUT")
        return new Response(
          JSON.stringify({
            error: {
              message:
                "Configuration changed; reopen the editor to review the latest version.",
            },
          }),
          { status: 409 },
        );
      return normalFetch(input, init);
    }),
  );
  fireEvent.click(
    screen.getByRole("button", {
      name: t("编辑检测{p0}", { p0: monitor.name }),
    }),
  );
  const dialog = screen.getByRole("dialog");
  const target = within(dialog).getByLabelText(t("目标地址"), {
    exact: false,
  }) as HTMLInputElement;
  fireEvent.change(target, { target: { value: "new.example.com:3306" } });
  fireEvent.submit(target.closest("form")!);
  await waitFor(() =>
    expect(screen.getByRole("alert").textContent).toContain(
      "Configuration changed",
    ),
  );
  await waitFor(() =>
    expect(requests.some((r) => r.path === "/poll")).toBe(true),
  );
  expect(target.value).toBe("new.example.com:3306");
});
it("hides monitor edit and archive controls for readers", () => {
  const monitor = showEditableMonitor();
  act(() => {
    appStore.setState((state) => ({
      user: { ...state.user!, permissions: ["monitor:read"] },
    }));
  });
  expect(
    screen.queryByRole("button", {
      name: t("编辑检测{p0}", { p0: monitor.name }),
    }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", {
      name: t("归档检测{p0}", { p0: monitor.name }),
    }),
  ).toBeNull();
});

it("localizes the default instance name in the shell in all five languages", async () => {
  render(<App />);
  await screen.findByRole("button", {
    name: "My monitoring center Monitor Go",
  });
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(
      screen.getByRole("button", { name: `${t("我的监控中心")} Monitor Go` }),
    ).toBeTruthy();
  }
});
it("preserves a custom Chinese instance name when the interface is English", async () => {
  fixtures["/me/navigation"].instanceName = "我的监控中心";
  fixtures["/me/navigation"].instanceNameKey = "";
  render(<App />);
  expect(
    await screen.findByRole("button", { name: "我的监控中心 Monitor Go" }),
  ).toBeTruthy();
});
it("translates the default name in settings and keeps an unsaved custom name on language changes", async () => {
  render(<SettingsView />);
  const toggle = await screen.findByLabelText(
    t("使用默认实例名称（随界面语言切换）"),
  );
  await waitFor(() =>
    expect((toggle as HTMLInputElement).disabled).toBe(false),
  );
  const input = screen.getByLabelText(t("名称"), {
    exact: true,
  }) as HTMLInputElement;
  expect(input.value).toBe("My monitoring center");
  expect(input.disabled).toBe(true);
  fireEvent.click(toggle);
  fireEvent.change(input, { target: { value: "我的研发监控" } });
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(input.value).toBe("我的研发监控");
  }
});

it("selects chat Webhook formats with matching signing guidance and keeps drafts across languages", async () => {
  const { default: NotificationsPanel } = await import(
    "../src/components/NotificationsPanel"
  );
  render(<NotificationsPanel mode="channels" />);
  await act(async () => {});
  fireEvent.click(screen.getAllByRole("button", { name: t("添加渠道") })[0]);
  const dialog = screen.getByRole("dialog");
  const format = within(dialog).getByLabelText(t("消息格式"));
  fireEvent.change(format, { target: { value: "dingtalk" } });
  expect(
    within(dialog).getByText(
      t("填写钉钉机器人安全设置中的加签密钥；时间戳和签名放在 URL 参数中。"),
    ),
  ).toBeTruthy();
  expect(
    within(dialog).queryByText(
      t("签名放在 X-MonitorGo-Signature 请求头；接收端可用投递编号去重。"),
    ),
  ).toBeNull();
  const url = within(dialog).getByPlaceholderText(
    "https://hooks.example.com/monitor",
  ) as HTMLInputElement;
  fireEvent.change(url, { target: { value: "https://example.com/test-hook" } });
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(url.value).toBe("https://example.com/test-hook");
    expect((format as HTMLSelectElement).value).toBe("dingtalk");
  }
  await act(() => setLocale("en-US"));
  fireEvent.change(format, { target: { value: "wecom" } });
  expect(
    within(dialog).queryByLabelText(t("签名密钥（可选）"), { exact: false }),
  ).toBeNull();
  expect(
    within(dialog).getByText(
      t("企业微信的密钥包含在 Webhook URL 的 key 参数中，无需额外签名密钥。"),
      { exact: false },
    ),
  ).toBeTruthy();
  fireEvent.change(format, { target: { value: "feishu" } });
  expect(
    within(dialog).getByText(
      t("填写飞书机器人的签名校验密钥；时间戳和签名放在消息正文中。"),
    ),
  ).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: t("保存渠道") }));
  await waitFor(() =>
    expect(
      requests.find(
        (r) => r.path === "/notification-channels" && r.method === "POST",
      )?.body,
    ).toMatchObject({
      config: { format: "feishu", url: "https://example.com/test-hook" },
    }),
  );
  await act(async () => {});
});

it("keeps threshold drafts across languages and submits zero to disable an alert", async () => {
  render(<SettingsView />);
  await waitFor(() =>
    expect(
      (screen.getByLabelText(t("CPU 阈值（%）")) as HTMLInputElement).value,
    ).toBe("90"),
  );
  const cpu = screen.getByLabelText(t("CPU 阈值（%）")) as HTMLInputElement;
  fireEvent.change(cpu, { target: { value: "0" } });
  const duration = screen.getByLabelText(
    t("持续时长（分钟）"),
  ) as HTMLInputElement;
  fireEvent.change(duration, { target: { value: "10" } });
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(cpu.value).toBe("0");
    expect(duration.value).toBe("10");
  }
  fireEvent.submit(cpu.closest("form")!);
  await waitFor(() =>
    expect(
      requests.find((r) => r.path === "/settings" && r.method === "PATCH")
        ?.body,
    ).toMatchObject({
      hostCPUPercent: 0,
      hostMemoryPercent: 90,
      hostDiskPercent: 85,
      hostAlertMinutes: 10,
    }),
  );
  await act(async () => {});
});

it("shows the injected build version verbatim without adding a duplicate prefix", async () => {
  fixtures["/system"].version = "v0.4.0-12-gabcdef1";
  render(<SettingsView />);
  await waitFor(() =>
    expect(
      screen.getByText("v0.4.0-12-gabcdef1", { exact: true }),
    ).toBeTruthy(),
  );
});

it("translates seeded folder paths in all languages while preserving settings drafts and custom names", async () => {
  fixtures["/files/tree"].items = [
    { id: "1", parentId: "", name: "全部文件", nameKey: "全部文件" },
    { id: "2", parentId: "1", name: "未分类", nameKey: "未分类" },
    { id: "3", parentId: "1", name: "自定义目录" },
  ];
  render(<SettingsView />);
  await screen.findByRole("option", { name: "All files / Uncategorized" });
  const threshold = screen.getByLabelText(t("CPU 阈值（%）"));
  fireEvent.change(threshold, { target: { value: "77" } });
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(
      screen.getByRole("option", { name: `${t("全部文件")} / ${t("未分类")}` }),
    ).toBeTruthy();
    expect(
      screen.getByRole("option", { name: `${t("全部文件")} / 自定义目录` }),
    ).toBeTruthy();
    expect((threshold as HTMLInputElement).value).toBe("77");
  }
});

it("translates seeded roles and descriptions without translating identical custom labels", async () => {
  const roles = structuredClone(fixtures["/roles"]);
  roles[0] = {
    ...roles[0],
    name: "超级管理员",
    nameKey: "超级管理员",
    description: "管理平台全部功能",
    descriptionKey: "管理平台全部功能",
  };
  roles[1] = {
    ...roles[1],
    name: "超级管理员",
    nameKey: "",
    descriptionKey: "",
  };
  appStore.setState({ data: { ...appStore.getState().data, roles } });
  render(<RolesView />);
  fireEvent.click(screen.getByRole("button", { name: /Super administrator/ }));
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(screen.getAllByText(t("超级管理员")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("超级管理员").length).toBeGreaterThan(0);
    expect(screen.getByText(t("管理平台全部功能"))).toBeTruthy();
  }
});

it("renders seeded file and image style names in English", async () => {
  fixtures["/files/tree"].items.forEach((f: any) => {
    if (["1", "2"].includes(f.id)) f.nameKey = f.name;
  });
  fixtures["/image-styles"] = [
    {
      id: "1",
      name: "列表缩略图",
      nameKey: "列表缩略图",
      width: 320,
      height: 240,
      fit: "cover",
      version: 1,
    },
  ];
  render(<FilesView />);
  await screen.findByRole("button", { name: /Image styles/ });
  fireEvent.click(screen.getByRole("button", { name: /Image styles/ }));
  expect(await screen.findByText("List thumbnail")).toBeTruthy();
  expect(screen.queryByText("列表缩略图")).toBeNull();
});

it("localizes fixed audit action labels while retaining actor and resource text", async () => {
  fixtures["/audit"] = {
    items: [
      {
        id: "1",
        at: 1,
        actor: "自定义成员",
        action: "创建首个管理员",
        resource: "自定义资源",
        requestId: "",
        method: "POST",
        path: "/api/v1/setup",
      },
    ],
    more: false,
  };
  render(<AuditView />);
  expect(
    await screen.findByText("Create the first administrator"),
  ).toBeTruthy();
  expect(screen.getByText("自定义成员")).toBeTruthy();
  expect(screen.getByText("自定义资源")).toBeTruthy();
});

it("edits HTTP keyword and latency rules while preserving drafts across five languages", async () => {
  const monitor = {
    ...editableMonitor(),
    type: "HTTP",
    target: "https://example.com",
    keyword: "ready",
    keywordInvert: true,
    latencyWarnMs: 500,
  };
  fixtures["/monitors"] = [monitor];
  appStore.setState({
    data: { ...appStore.getState().data, monitors: [monitor] },
  });
  render(<MonitorsView />);
  fireEvent.click(
    screen.getByRole("button", {
      name: t("编辑检测{p0}", { p0: monitor.name }),
    }),
  );
  const dialog = screen.getByRole("dialog");
  const keyword = within(dialog).getByLabelText(t("响应关键字"), {
    exact: false,
  });
  expect((keyword as HTMLInputElement).value).toBe("ready");
  fireEvent.change(keyword, { target: { value: "healthy 世界" } });
  fireEvent.change(
    within(dialog).getByLabelText(t("耗时警告阈值（毫秒）"), { exact: false }),
    { target: { value: "300" } },
  );
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect((keyword as HTMLInputElement).value).toBe("healthy 世界");
  }
  fireEvent.click(within(dialog).getByRole("button", { name: t("保存") }));
  await waitFor(() =>
    expect(
      requests.find((r) => r.method === "PUT" && r.path === "/monitors/71")
        ?.body,
    ).toMatchObject({
      keyword: "healthy 世界",
      keywordInvert: true,
      latencyWarnMs: 300,
      version: 4,
    }),
  );
});

it("loads 30/90-day availability on the check page and keeps missing history empty", async () => {
  const monitor = { ...editableMonitor(), uptime: 100 };
  appStore.setState({
    data: { ...appStore.getState().data, monitors: [monitor] },
  });
  fixtures["/monitors/availability"] = {
    items: [{ monitorId: monitor.id, uptime: 93.75 }],
  };
  render(<MonitorsView />);
  const period = screen.getByLabelText(t("可用率周期"));
  fireEvent.change(period, { target: { value: "30d" } });
  expect(await screen.findByText("93.75%")).toBeTruthy();
  expect(
    screen.getByRole("columnheader", { name: "30-day availability" }),
  ).toBeTruthy();
  expect(
    vi
      .mocked(fetch)
      .mock.calls.some(([url]) =>
        String(url).includes("/monitors/availability?hours=720"),
      ),
  ).toBe(true);
  fixtures["/monitors/availability"] = { items: [] };
  fireEvent.change(period, { target: { value: "90d" } });
  await waitFor(() =>
    expect(
      vi
        .mocked(fetch)
        .mock.calls.some(([url]) =>
          String(url).includes("/monitors/availability?hours=2160"),
        ),
    ).toBe(true),
  );
  expect(
    screen.getByRole("columnheader", { name: "90-day availability" }),
  ).toBeTruthy();
  expect(screen.queryByText("93.75%")).toBeNull();
  expect(screen.queryByText("100.00%")).toBeNull();
});

it("uses the selected long range for overview totals and chart requests", async () => {
  const monitor = editableMonitor();
  appStore.setState({
    data: { ...appStore.getState().data, monitors: [monitor] },
  });
  fixtures["/overview"] = {
    at: Date.now(),
    points: [],
    activity: [],
    samples: 20,
    uptime: 75,
    latency: 10,
  };
  render(<OverviewView />);
  fireEvent.click(screen.getByRole("button", { name: t("30 天") }));
  await waitFor(() =>
    expect(
      vi
        .mocked(fetch)
        .mock.calls.some(([url]) =>
          String(url).includes("/overview?hours=720"),
        ),
    ).toBe(true),
  );
  expect(await screen.findByText("30-day availability")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: t("90 天") }));
  await waitFor(() =>
    expect(
      vi
        .mocked(fetch)
        .mock.calls.some(([url]) =>
          String(url).includes("/overview?hours=2160"),
        ),
    ).toBe(true),
  );
  expect(await screen.findByText("90-day availability")).toBeTruthy();
});

it("saves metric retention and preserves its draft through language switches", async () => {
  render(<SettingsView />);
  const raw = await screen.findByLabelText(t("原始指标保留天数"), {
    exact: false,
  });
  await waitFor(() => expect((raw as HTMLInputElement).disabled).toBe(false));
  fireEvent.change(raw, { target: { value: "3" } });
  fireEvent.change(
    screen.getByLabelText(t("小时汇总保留天数"), { exact: false }),
    { target: { value: "180" } },
  );
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect((raw as HTMLInputElement).value).toBe("3");
  }
  fireEvent.click(screen.getByRole("button", { name: t("保存设置") }));
  await waitFor(() =>
    expect(
      requests.find((r) => r.method === "PATCH" && r.path === "/settings")
        ?.body,
    ).toMatchObject({ rawRetentionDays: 3, summaryRetentionDays: 180 }),
  );
});

it("localizes all built-in menu labels and selectors while preserving custom names", async () => {
  fixtures["/menus"].items.push({
    id: "custom-chinese",
    parentId: "",
    name: "自定义菜单",
    kind: "group",
    visible: true,
    icon: "FolderOpen",
    version: 1,
    sort: 99,
    builtin: false,
  });
  const view = render(<MenusView />);
  await act(async () => {});
  expect(view.container.textContent).toContain("Monitoring & operations");
  expect(view.container.textContent).toContain("自定义菜单");
  expect(view.container.textContent?.replaceAll("自定义菜单", "")).not.toMatch(
    /[\u3400-\u9fff]/u,
  );
  fireEvent.click(screen.getByRole("button", { name: t("新增菜单") }));
  const dialog = screen.getByRole("dialog");
  expect(
    within(dialog).getByRole("combobox", { name: t("上级分组") }).textContent,
  ).toContain("Monitoring & operations");
  expect(
    within(dialog).getByRole("combobox", { name: t("指向页面") }).textContent,
  ).toContain("Overview");
  expect(
    within(dialog).getByRole("combobox", { name: t("附加访问限制") })
      .textContent,
  ).not.toMatch(/[\u3400-\u9fff]/u);
});
it("preserves built-in menu names when saving other fields after a locale switch", async () => {
  render(<MenusView />);
  await act(async () => {});
  const row = screen.getByRole("row", { name: /Monitoring & operations/ });
  fireEvent.click(within(row).getByRole("button", { name: t("编辑") }));
  expect(
    (screen.getByPlaceholderText(t("例如：运维手册")) as HTMLInputElement)
      .value,
  ).toBe("Monitoring & operations");
  fireEvent.change(screen.getByLabelText(t("排序值")), {
    target: { value: "12" },
  });
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(
      (screen.getByPlaceholderText(t("例如：运维手册")) as HTMLInputElement)
        .value,
    ).toBe(t("观测与运维"));
    expect((screen.getByLabelText(t("排序值")) as HTMLInputElement).value).toBe(
      "12",
    );
  }
  fireEvent.click(screen.getByRole("button", { name: t("保存菜单") }));
  await waitFor(() =>
    expect(
      requests.find((r) => r.path === "/menus/1" && r.method === "PATCH")?.body,
    ).toMatchObject({ name: "观测与运维", sort: 12 }),
  );
  await act(async () => {});
});
it("keeps explicit menu-name drafts unchanged through all five locales", async () => {
  render(<MenusView />);
  await act(async () => {});
  fireEvent.click(
    within(
      screen.getByRole("row", { name: /Monitoring & operations/ }),
    ).getByRole("button", { name: t("编辑") }),
  );
  fireEvent.change(screen.getByPlaceholderText(t("例如：运维手册")), {
    target: { value: "Operations team" },
  });
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(
      (screen.getByPlaceholderText(t("例如：运维手册")) as HTMLInputElement)
        .value,
    ).toBe("Operations team");
  }
  fireEvent.click(screen.getByRole("button", { name: t("保存菜单") }));
  await waitFor(() =>
    expect(
      requests.find((r) => r.path === "/menus/1" && r.method === "PATCH")?.body,
    ).toMatchObject({ name: "Operations team" }),
  );
  await act(async () => {});
});

// These fixtures model untouched defaults from a fresh installation, not translated user data.
function englishDialogFixtures() {
  for (const key of ["/files", "/files/tree"]) {
    fixtures[key].items.forEach((f: any) => {
      f.nameKey = f.name;
      if (f.creator === "系统") f.creatorKey = "系统";
    });
  }
  fixtures["/image-styles"].forEach((s: any) => (s.nameKey = s.name));
  fixtures["/applications"] = [
    {
      id: "1",
      name: "Test application",
      slug: "test",
      enabled: true,
      version: 1,
      description: "",
    },
  ];
  fixtures["/applications/1/keys"] = { items: [], scopes: [] };
  const roles = fixtures["/roles"].map((r: any) => ({
    ...r,
    nameKey: r.name,
    descriptionKey: r.description,
  }));
  appStore.setState({ data: { ...appStore.getState().data, roles } });
}
function assertEnglishDialog(dialog: HTMLElement) {
  const copy = dialog.cloneNode(true) as HTMLElement;
  copy.querySelectorAll("option[lang]").forEach((option) => option.remove());
  const text = [
    copy.textContent,
    ...Array.from(
      dialog.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>(
        "input, textarea",
      ),
    )
      .filter((f) => f.type !== "password")
      .map((f) => f.value),
    ...Array.from(
      dialog.querySelectorAll("[title], [placeholder], [aria-label]"),
    ).flatMap((f) => [
      f.getAttribute("title"),
      f.getAttribute("placeholder"),
      f.getAttribute("aria-label"),
    ]),
  ].join("\n");
  expect(text).not.toMatch(/[\u3400-\u9fff]/u);
}
it.each([
  [MonitorsView, "新增检测"],
  [FilesView, "新建文件夹"],
  [RolesView, "新建角色"],
  [RolesView, "添加成员"],
  [MenusView, "新增菜单"],
  [ApplicationsView, "创建应用"],
  [ReleasesView, "新建版本"],
  [SettingsView, "清理图片缓存"],
] as const)(
  "renders an English dialog and dropdowns (%#)",
  async (Page, key) => {
    englishDialogFixtures();
    render(<Page />);
    await act(async () => {});
    const buttons = screen.getAllByRole("button", {
      name: t(key),
    });
    fireEvent.click(buttons[0]);
    assertEnglishDialog(await screen.findByRole("dialog"));
  },
);

it("keeps seeded file rename and image-style drafts localized until explicitly edited", async () => {
  englishDialogFixtures();
  await setLocale("zh-CN");
  render(<FilesView />);
  await act(async () => {});
  fireEvent.click(screen.getAllByRole("button", { name: t("编辑") })[0]);
  const name = within(screen.getByRole("dialog")).getByPlaceholderText(
    t("输入名称"),
  ) as HTMLInputElement;
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(name.value).toBe(t("未分类"));
  }
  fireEvent.change(name, { target: { value: "自定义目录" } });
  await act(() => setLocale("en-US"));
  expect(name.value).toBe("自定义目录");
  fireEvent.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: t("取消") }),
  );
  fireEvent.click(screen.getByRole("button", { name: /Image styles/ }));
  fireEvent.click(screen.getAllByRole("button", { name: t("编辑") })[0]);
  const style = screen.getByPlaceholderText(
    t("例如：文章封面"),
  ) as HTMLInputElement;
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(style.value).toBe(t("列表缩略图"));
  }
  fireEvent.change(style, { target: { value: "自定义样式" } });
  await act(() => setLocale("en-US"));
  expect(style.value).toBe("自定义样式");
});

it("keeps the default monitor group dynamic and custom group drafts intact", async () => {
  await setLocale("zh-CN");
  render(<MonitorsView />);
  fireEvent.click(screen.getAllByRole("button", { name: t("新增检测") })[0]);
  const group = within(screen.getByRole("dialog")).getByLabelText(t("分组"), {
    exact: true,
  }) as HTMLInputElement;
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(group.value).toBe("");
    expect(group.placeholder).toBe(t("默认分组"));
  }
  fireEvent.change(group, { target: { value: "生产环境" } });
  await act(() => setLocale("en-US"));
  expect(group.value).toBe("生产环境");
});

it("translates audit action metadata, including release states, while retaining resource names", async () => {
  const actions = [
    "切换检测启停",
    "更新角色权限",
    "保存通知渠道",
    "保存安装包（草稿）",
    "保存安装包（已发布）",
    "保存安装包（已下架）",
    "调整事件静默时间",
    "轮换应用密钥",
  ];
  fixtures["/audit"] = {
    items: actions.map((action, i) => ({
      id: String(i),
      at: 1,
      actor: "Test admin",
      action,
      resource: "用户原文",
      requestId: "",
      method: "POST",
      path: "/test",
    })),
    hasMore: false,
  };
  render(<AuditView />);
  await act(async () => {});
  for (const action of actions)
    expect(screen.getByText(t(action as Parameters<typeof t>[0]))).toBeTruthy();
  expect(
    screen.getByRole("table").textContent?.replaceAll("用户原文", ""),
  ).not.toMatch(/[\u3400-\u9fff]/u);
});

it("filters the default monitor group separately from all and explicitly named groups", async () => {
  const monitors = [
    { ...editableMonitor(), id: "default", name: "Default target", group: "" },
    {
      ...editableMonitor(),
      id: "named",
      name: "Named target",
      group: "Production",
    },
  ];
  appStore.setState({ data: { ...appStore.getState().data, monitors } });
  render(<MonitorsView />);
  fireEvent.change(screen.getByRole("combobox", { name: t("分组") }), {
    target: { value: "group:" },
  });
  expect(screen.getByText("Default target", { exact: true })).toBeTruthy();
  expect(screen.queryByText("Named target", { exact: true })).toBeNull();
  await act(() => setLocale("ja-JP"));
  expect(
    (screen.getByRole("combobox", { name: t("分组") }) as HTMLSelectElement)
      .value,
  ).toBe("group:");
});

// Exercise the actual in-dialog language control, not only the locale store.
async function switchDialogLanguage(dialog: HTMLElement, code: string) {
  fireEvent.change(
    within(dialog).getByRole("combobox", { name: t("界面语言") }),
    {
      target: { value: code },
    },
  );
  await waitFor(() => expect(document.documentElement.lang).toBe(code));
}
function draftFields(root: HTMLElement) {
  return Array.from(
    root.querySelectorAll<
      HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
    >("input, select, textarea"),
  ).filter(
    (field) =>
      !field.closest(".language-select") &&
      field.type !== "file" &&
      field.type !== "hidden",
  );
}
function fieldValues(fields: ReturnType<typeof draftFields>) {
  return fields.map((field) => ({
    type: field.type,
    value: field.value,
    checked: field instanceof HTMLInputElement ? field.checked : undefined,
  }));
}
it.each([
  [MonitorsView, "新增检测"],
  [FilesView, "新建文件夹"],
  [FilesView, "新增样式"],
  [RolesView, "新建角色"],
  [RolesView, "添加成员"],
  [MenusView, "新增菜单"],
  [ApplicationsView, "创建应用"],
  [ApplicationsView, "创建密钥"],
  [ReleasesView, "新建版本"],
  [AgentAccess, "接入远程主机"],
  [() => <NotificationsPanel mode="channels" />, "添加渠道"],
] as const)(
  "keeps every draft field through Japanese, Korean and Traditional Chinese dialog switches (%#)",
  async (Page, key) => {
    englishDialogFixtures();
    fixtures["/applications/1/keys"].scopes = [
      {
        key: "monitor:read",
        name: "检测状态",
        description: "读取全部检测目标和最近状态，不包含访问凭据",
      },
    ];
    render(<Page />);
    await act(async () => {});
    if (key === "新增样式")
      fireEvent.click(screen.getByRole("button", { name: /Image styles/ }));
    fireEvent.click(screen.getAllByRole("button", { name: t(key) })[0]);
    const dialog = await screen.findByRole("dialog");
    for (const [index, field] of draftFields(dialog).entries()) {
      if (
        field.disabled ||
        (field instanceof HTMLInputElement && field.readOnly)
      )
        continue;
      if (field.type === "checkbox") fireEvent.click(field);
      else if (field.type === "radio" || field instanceof HTMLSelectElement)
        continue;
      else
        fireEvent.change(field, {
          target: {
            value:
              field.type === "number" ? "42" : `草稿 / 下書き / 초안 ${index}`,
          },
        });
    }
    const fields = draftFields(dialog);
    const expected = fieldValues(fields);
    const requestCount = requests.length;
    for (const code of ["ja-JP", "ko-KR", "zh-TW", "en-US"]) {
      await switchDialogLanguage(dialog, code);
      expect(screen.getByRole("dialog")).toBe(dialog);
      expect(draftFields(dialog)).toEqual(fields);
      expect(fieldValues(fields)).toEqual(expected);
      expect(
        within(dialog).getByRole("button", { name: t("关闭") }),
      ).toBeTruthy();
    }
    // Switching must not save, submit, or reload the draft.
    expect(requests.length).toBe(requestCount);
  },
);

it("keeps SMTP credentials, recipients and transport selections when the dialog language changes", async () => {
  render(<NotificationsPanel mode="channels" />);
  await act(async () => {});
  fireEvent.click(screen.getAllByRole("button", { name: t("添加渠道") })[0]);
  const dialog = screen.getByRole("dialog");
  fireEvent.change(within(dialog).getByLabelText(t("通知方式")), {
    target: { value: "smtp" },
  });
  for (const [label, value] of [
    ["渠道名称", "值班通知 / 当番 / 당직"],
    ["SMTP 主机", "smtp.example.com"],
    ["端口", "587"],
    ["连接安全", "starttls"],
    ["登录账号", "reviewer"],
    ["密码或授权码", "unsaved-secret-2026"],
    ["发件邮箱", "sender@example.com"],
  ] as const)
    fireEvent.change(within(dialog).getByLabelText(t(label)), {
      target: { value },
    });
  fireEvent.change(
    within(dialog).getByLabelText(t("收件邮箱"), { exact: false }),
    { target: { value: "first@example.com\nsecond@example.com" } },
  );
  const fields = draftFields(dialog),
    expected = fieldValues(fields);
  for (const code of ["ja-JP", "ko-KR", "zh-TW"]) {
    await switchDialogLanguage(dialog, code);
    expect(fieldValues(draftFields(dialog))).toEqual(expected);
  }
  fireEvent.click(within(dialog).getByRole("button", { name: t("保存渠道") }));
  await waitFor(() =>
    expect(
      requests.find(
        (r) => r.path === "/notification-channels" && r.method === "POST",
      )?.body,
    ).toMatchObject({
      kind: "smtp",
      config: {
        port: 587,
        security: "starttls",
        secret: "unsaved-secret-2026",
        to: ["first@example.com", "second@example.com"],
      },
    }),
  );
});

it.each([
  "<html>Proxy error</html>",
  "null",
  '{"data":null}',
  '{"error":{"key":"上传失败","message":"旧文案"}}',
])(
  "localizes malformed or failed package upload responses while retaining the version draft (%#)",
  async (responseText) => {
    englishDialogFixtures();
    let xhr!: { onload: () => void };
    vi.stubGlobal(
      "XMLHttpRequest",
      class {
        constructor() {
          xhr = this;
        }
        upload = {};
        onload = () => {};
        status = responseText.includes('"error"') ? 422 : 200;
        responseText = responseText;
        open() {}
        send() {}
        abort() {}
        setRequestHeader() {}
      },
    );
    render(<ReleasesView />);
    await act(async () => {});
    fireEvent.click(screen.getByRole("button", { name: t("新建版本") }));
    const dialog = screen.getByRole("dialog");
    const version = within(dialog).getByLabelText(
      t("版本号"),
    ) as HTMLInputElement;
    fireEvent.change(version, { target: { value: "2.3.4" } });
    await act(async () => {});
    const input = dialog.querySelector('input[type="file"]')!;
    fireEvent.change(input, {
      target: { files: [new File(["binary"], "package.zip")] },
    });
    act(() => xhr.onload());
    for (const code of ["ja-JP", "ko-KR", "zh-TW"]) {
      await switchDialogLanguage(dialog, code);
      expect(within(dialog).getByRole("alert").textContent).toBe(
        t(responseText.includes('"error"') ? "上传失败" : "上传响应异常"),
      );
      expect(version.value).toBe("2.3.4");
    }
  },
);

it("retains a typed deletion confirmation across languages and still rejects arbitrary text", async () => {
  englishDialogFixtures();
  fixtures["/files"].items[0].deletedAt = Date.now();
  fixtures["/files"].items[0].trashRoot = "2";
  render(<FilesView />);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: /Trash/ }));
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: t("永久删除") }));
  const dialog = screen.getByRole("dialog");
  const input = within(dialog).getByLabelText(
    t("输入“永久删除”以确认"),
  ) as HTMLInputElement;
  const confirm = () =>
    within(dialog).getByRole("button", {
      name: t("永久删除"),
    }) as HTMLButtonElement;
  expect(confirm().disabled).toBe(true);
  await switchDialogLanguage(dialog, "ja-JP");
  const phrase = t("永久删除");
  fireEvent.change(input, { target: { value: phrase } });
  for (const code of ["ko-KR", "zh-TW", "en-US"]) {
    await switchDialogLanguage(dialog, code);
    expect(input.value).toBe(phrase);
    expect(confirm().disabled).toBe(false);
  }
  fireEvent.change(input, { target: { value: "arbitrary" } });
  expect(confirm().disabled).toBe(true);
  fireEvent.submit(input.closest("form")!);
  expect(requests.some((r) => r.path.endsWith("/purge"))).toBe(false);
  fireEvent.change(input, { target: { value: t("永久删除") } });
  fireEvent.click(confirm());
  await waitFor(() =>
    expect(requests.find((r) => r.path.endsWith("/purge"))?.body).toMatchObject(
      { confirm: "DELETE", version: 1 },
    ),
  );
});

// Seed provenance mirrors the live API; literal user/historical content is not translated.
describe.each(["ja-JP", "ko-KR", "zh-TW"])("%s system text audit", (code) => {
  it.each(Object.entries(pages))(
    "checks %s labels, choices and placeholders",
    async (_name, Page) => {
      englishDialogFixtures();
      for (const user of fixtures["/users"]) user.name = "Fixture user";
      fixtures["/audit"] = { items: [], hasMore: false };
      await setLocale(code);
      render(<Page />);
      await act(async () => {});
      const copy = document.body.cloneNode(true) as HTMLElement;
      copy.querySelectorAll("option[lang]").forEach((el) => el.remove());
      const text = [
        copy.textContent,
        ...Array.from(
          copy.querySelectorAll("[placeholder], [title], [aria-label]"),
        ).flatMap((el) => [
          el.getAttribute("placeholder"),
          el.getAttribute("title"),
          el.getAttribute("aria-label"),
        ]),
      ].join("\n");
      if (code === "ko-KR")
        expect(text).not.toMatch(
          /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]/u,
        );
      if (code === "ja-JP")
        expect(text).not.toMatch(/[\p{Script=Hangul}这请页输检测账户]/u);
      if (code === "zh-TW")
        expect(text).not.toMatch(
          /[\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}这请页输检测账户]/u,
        );
    },
  );
});

it("loads full check diagnostics only when opening a detail drawer", async () => {
  const monitor = editableMonitor();
  const history = [
    {
      at: Date.now(),
      state: "healthy",
      latency: 12,
      detail: "Original detailed diagnostic",
    },
  ];
  fixtures["/monitors"] = [{ ...monitor, history }];
  appStore.setState((state) => ({
    data: {
      ...state.data,
      monitors: [
        {
          ...monitor,
          history: history.map((h) => ({
            ...h,
            state: "healthy" as const,
            detail: "",
          })),
        },
      ],
    },
  }));
  render(<MonitorsView />);
  expect(requests.some((r) => r.path.endsWith("/history"))).toBe(false);
  fireEvent.click(
    screen.getByText(monitor.name, { exact: true }).closest("button")!,
  );
  await screen.findByText("Original detailed diagnostic");
  expect(
    requests.filter((r) => r.path === `/monitors/${monitor.id}/history`),
  ).toHaveLength(1);
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(screen.getByText("Original detailed diagnostic")).toBeTruthy();
  }
  expect(
    requests.filter((r) => r.path === `/monitors/${monitor.id}/history`),
  ).toHaveLength(1);
});
