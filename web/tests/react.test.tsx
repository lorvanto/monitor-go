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
        JSON.stringify({ data: structuredClone(fixtures[path] ?? {}) }),
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
    within(screen.getByRole("dialog", { name: "Inner" })).getByRole("button"),
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
