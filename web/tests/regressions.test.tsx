import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { lazy, Suspense } from "react";
import HostView from "../src/views/HostView";
import AlertsView from "../src/views/AlertsView";
import ApplicationsView from "../src/views/ApplicationsView";
import ReleasesView from "../src/views/ReleasesView";
import PageBoundary from "../src/components/PageBoundary";
import type { Host, Alert } from "../src/lib/types";
import App from "../src/App";
import AuditView from "../src/views/AuditView";
import FilesView from "../src/views/FilesView";
import {
  action,
  api,
  appStore,
  logout,
  refreshAll,
  signIn,
  type User,
} from "../src/stores/app";
import { setLocale, t } from "../src/i18n";
import fixtureJSON from "./api-fixtures.json";

const initial = appStore.getInitialState();
const admin = fixtureJSON["/me"] as User;
let fixtures: Record<string, unknown>;
let errors: ReturnType<typeof vi.spyOn>;
const response = (data: unknown) =>
  new Response(JSON.stringify({ data }), { status: 200 });
function deferred() {
  let resolve!: (value: Response) => void;
  const promise = new Promise<Response>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
beforeEach(async () => {
  fixtures = structuredClone(fixtureJSON);
  appStore.setState({ ...initial, user: admin, booting: false }, true);
  await setLocale("en-US");
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string) => {
      const path = input.replace("/api/v1", "").split("?")[0];
      if (path === "/auth/logout" || path === "/auth/login")
        return response({});
      if (!(path in fixtures)) throw new Error(`Unexpected request: ${input}`);
      return response(fixtures[path]);
    }),
  );
  errors = vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => {
  expect(errors.mock.calls).toEqual([]);
});

it("does not restore a logged-out user when an earlier refresh completes", async () => {
  const pending = deferred();
  vi.mocked(fetch).mockImplementationOnce(() => pending.promise);
  const refresh = refreshAll();
  await logout();
  pending.resolve(response(admin));
  await refresh;
  expect(appStore.getState().user).toBeNull();
  expect(appStore.getState().navigation).toEqual([]);
});
it("ignores an old session's late 401 after a new login", async () => {
  const pending = deferred();
  vi.mocked(fetch).mockImplementationOnce(() => pending.promise);
  const oldRequest = api("/hosts").catch(() => {});
  fixtures["/me"] = { ...admin, id: "new-user", csrf: "new-session" };
  await signIn({ username: "new-user", password: "fixture-only" });
  pending.resolve(
    new Response(JSON.stringify({ error: { message: "Expired" } }), {
      status: 401,
    }),
  );
  await oldRequest;
  expect(appStore.getState().user?.id).toBe("new-user");
});
it("waits for a running refresh and refreshes again after a successful write", async () => {
  const pending = deferred();
  const base = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    if (String(input) === "/api/v1/hosts") return pending.promise;
    return base(input, init);
  });
  const refresh = refreshAll();
  await waitFor(() =>
    expect(
      vi.mocked(fetch).mock.calls.some(([p]) => String(p) === "/api/v1/hosts"),
    ).toBe(true),
  );
  let finished = false;
  const mutation = action(async () => {}).then(() => {
    finished = true;
  });
  await Promise.resolve();
  await Promise.resolve();
  expect(finished).toBe(false);
  pending.resolve(response([]));
  await Promise.all([refresh, mutation]);
  expect(
    vi.mocked(fetch).mock.calls.filter(([p]) => String(p) === "/api/v1/hosts"),
  ).toHaveLength(2);
});
it("clears session-specific selections and notices on logout", async () => {
  appStore.setState({
    activeHost: "old",
    activeMonitor: "old",
    activeAlert: "old",
    createMonitorRequested: true,
    checking: ["old"],
    toasts: [{ id: 1, message: "old", error: false }],
    updatedAt: 1,
  });
  await logout();
  expect(appStore.getState()).toMatchObject({
    activeHost: "",
    activeMonitor: "",
    activeAlert: "",
    createMonitorRequested: false,
    checking: [],
    toasts: [],
    updatedAt: 0,
  });
});
it("handles a logout network failure without an unhandled rejection", async () => {
  vi.mocked(fetch).mockRejectedValueOnce(new TypeError("Offline"));
  await expect(logout()).resolves.toBeUndefined();
  expect(appStore.getState().user).toBeNull();
  expect(appStore.getState().connectionError).not.toBe("");
});
it("boots and toggles the theme when browser preference storage is blocked", async () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new DOMException("Blocked", "SecurityError");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("Blocked", "SecurityError");
  });
  render(<App />);
  await waitFor(() => expect(screen.queryByRole("main")).not.toBeNull());
  fireEvent.click(screen.getByRole("button", { name: t("切换到深色主题") }));
  expect(document.documentElement.dataset.theme).toBe("dark");
});
it("clears audit filters even while a previous query is in flight", async () => {
  render(<AuditView />);
  await act(async () => {});
  const query = screen.getByLabelText(t("搜索审计记录"));
  fireEvent.change(query, { target: { value: "old-filter" } });
  const pending = deferred();
  vi.mocked(fetch).mockImplementationOnce(() => pending.promise);
  fireEvent.submit(query.closest("form")!);
  fireEvent.click(screen.getByRole("button", { name: t("清除条件") }));
  pending.resolve(
    response({
      items: [
        {
          id: "old",
          at: Date.now(),
          actor: "Old result",
          action: "old-filter",
          resource: "Old data",
          requestId: "",
          method: "GET",
          path: "/old",
        },
      ],
      hasMore: false,
    }),
  );
  await act(async () => {});
  expect(screen.queryByText("Old result")).toBeNull();
  expect((query as HTMLInputElement).value).toBe("");
  expect(
    vi
      .mocked(fetch)
      .mock.calls.filter(([path]) => String(path).startsWith("/api/v1/audit?")),
  ).toHaveLength(3);
});
it("keeps the latest file preview when earlier requests finish last", async () => {
  const entry = (id: string) => ({
    id,
    parentId: "1",
    name: `${id}.txt`,
    kind: "file",
    mime: "text/plain",
    size: 10,
    version: 1,
    createdAt: 1,
    updatedAt: 1,
    width: 0,
    height: 0,
    deletedAt: null,
  });
  fixtures["/files"] = { items: [entry("first"), entry("second")], total: 2 };
  render(<FilesView />);
  await act(async () => {});
  const first = deferred(),
    second = deferred();
  const base = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation((input, init) =>
    String(input).includes("/preview")
      ? String(input).includes("/first/")
        ? first.promise
        : second.promise
      : base(input, init),
  );
  fireEvent.click(
    screen.getByRole("button", { name: t("预览 ") + "first.txt" }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: t("预览 ") + "second.txt" }),
  );
  second.resolve(
    response({ file: entry("second"), kind: "text", text: "Second contents" }),
  );
  await act(async () => {});
  expect(screen.getByRole("dialog").textContent).toContain("Second contents");
  first.resolve(
    response({ file: entry("first"), kind: "text", text: "First contents" }),
  );
  await act(async () => {});
  expect(screen.getByRole("dialog").textContent).toContain("Second contents");
});

it("recovers file pagination after the last item on the final page is removed", async () => {
  const firstPage = {
    items: [
      {
        id: "1",
        parentId: "1",
        name: "Remaining folder",
        kind: "folder",
        size: 0,
        updatedAt: 1,
      },
    ],
    total: 51,
  };
  fixtures["/files"] = firstPage;
  render(<FilesView />);
  await act(async () => {});
  fixtures["/files"] = { items: [], total: 50 };
  const base = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation((input, init) =>
    String(input).includes("/files?") &&
    new URL(String(input), "http://localhost").searchParams.get("page") === "1"
      ? Promise.resolve(response({ ...firstPage, total: 50 }))
      : base(input, init),
  );
  fireEvent.click(screen.getByRole("button", { name: t("下一页") }));
  await act(async () => {});
  expect(
    screen.getByText(
      t("共 {p0} 项，第 {p1} / {p2} 页", { p0: 50, p1: 1, p2: 1 }),
    ),
  ).toBeTruthy();
  expect(screen.getByText("Remaining folder")).toBeTruthy();
});

function host(id: string): Host {
  return {
    id,
    hostname: id,
    alias: id,
    version: 1,
    source: "agent",
    status: "online",
    agentEnabled: true,
    agentVersion: "0.4.0",
    enrolledAt: 1,
    enrollmentExpires: 0,
    lastSeen: Date.now(),
    inventory: {
      os: "linux",
      kernel: "6.8",
      arch: "amd64",
      cpu: "Test CPU",
      cores: 2,
      memory: 1024,
      ips: [],
      bootTime: 1,
      source: "agent",
    },
    snapshot: {
      cpu: 10,
      memory: 20,
      memoryUsed: 205,
      uptime: 100,
      netIn: 0,
      netOut: 0,
      disks: [],
      errors: [],
    },
  };
}
it("does not show one host's chart while another host's metrics are loading", async () => {
  appStore.setState((s) => ({
    data: { ...s.data, hosts: [host("one"), host("two")] },
  }));
  fixtures["/hosts/one/metrics"] = [
    { at: Date.now() - 1000, cpu: 77, memory: 20, netIn: 2, netOut: 3 },
  ];
  const view = render(<HostView />);
  await act(async () => {});
  expect(view.container.querySelectorAll(".chart-dot").length).toBeGreaterThan(
    0,
  );
  const pending = deferred();
  vi.mocked(fetch).mockImplementationOnce(() => pending.promise);
  await act(async () => {
    appStore.setState({ activeHost: "two" });
  });
  expect(view.container.querySelectorAll(".chart-dot")).toHaveLength(0);
  pending.resolve(response([]));
  await act(async () => {});
});
it("saves the originally edited host even if polling removes it from the list", async () => {
  appStore.setState((s) => ({
    data: { ...s.data, hosts: [host("one"), host("two")] },
  }));
  fixtures["/hosts/one/metrics"] = [];
  fixtures["/hosts/two/metrics"] = [];
  fixtures["/hosts/one"] = {};
  render(<HostView />);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: t("编辑备注") }));
  fireEvent.change(screen.getByPlaceholderText(t("例如：生产应用服务器")), {
    target: { value: "Changed alias" },
  });
  await act(async () => {
    appStore.setState((s) => ({ data: { ...s.data, hosts: [host("two")] } }));
  });
  fireEvent.submit(
    screen.getByPlaceholderText(t("例如：生产应用服务器")).closest("form")!,
  );
  await act(async () => {});
  const patch = vi
    .mocked(fetch)
    .mock.calls.find(([, init]) => init?.method === "PATCH");
  expect(patch?.[0]).toBe("/api/v1/hosts/one");
  expect(JSON.parse(String(patch?.[1]?.body))).toEqual({
    alias: "Changed alias",
    version: 1,
  });
});
it("silences the originally selected incident after the live list changes", async () => {
  const incident = (id: string): Alert => ({
    id,
    kind: "monitor",
    resourceId: id,
    resource: id,
    title: id,
    description: "",
    severity: "critical",
    state: "open",
    openedAt: Date.now() - 1000,
    recoveredAt: null,
    owner: "",
    silenceUntil: 0,
    version: 1,
    activity: [],
  });
  appStore.setState((s) => ({
    data: { ...s.data, alerts: [incident("one"), incident("two")] },
  }));
  fixtures["/alerts/one/silences"] = {};
  render(<AlertsView />);
  fireEvent.click(screen.getByRole("button", { name: t("静默通知") }));
  await act(async () => {
    appStore.setState((s) => ({
      data: { ...s.data, alerts: [incident("two")] },
    }));
  });
  fireEvent.click(screen.getByRole("button", { name: t("开始静默") }));
  await act(async () => {});
  expect(fetch).toHaveBeenCalledWith(
    "/api/v1/alerts/one/silences",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ version: 1, minutes: 60 }),
    }),
  );
});
it("keeps fresh keys after selecting application A, B, then A again", async () => {
  fixtures["/applications"] = ["alpha", "beta"].map((id) => ({
    id,
    slug: id,
    name: id,
    enabled: true,
    activeKeys: 1,
    releases: 0,
    version: 1,
  }));
  fixtures["/applications/beta/keys"] = { items: [], scopes: [] };
  const first = deferred(),
    latest = deferred();
  let count = 0;
  const base = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation((input, init) =>
    String(input) === "/api/v1/applications/alpha/keys"
      ? ++count === 1
        ? first.promise
        : latest.promise
      : base(input, init),
  );
  render(<ApplicationsView />);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: /beta beta/ }));
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: /alpha alpha/ }));
  const key = (name: string) => ({
    id: name,
    name,
    prefix: "test",
    scopes: [],
    expiresAt: Date.now() + 100000,
    createdAt: 1,
    revokedAt: 0,
    lastUsedAt: 0,
    version: 1,
  });
  latest.resolve(response({ items: [key("Fresh key")], scopes: [] }));
  await act(async () => {});
  first.resolve(response({ items: [key("Stale key")], scopes: [] }));
  await act(async () => {});
  expect(screen.getByText("Fresh key")).toBeTruthy();
  expect(screen.queryByText("Stale key")).toBeNull();
});
it("ignores a release editor's late folder error after the editor closes", async () => {
  fixtures["/applications"] = [
    { id: "app", name: "App", slug: "app", enabled: true },
  ];
  render(<ReleasesView />);
  await act(async () => {});
  const pending = deferred();
  vi.mocked(fetch).mockImplementationOnce(() => pending.promise);
  fireEvent.click(screen.getAllByRole("button", { name: t("新建版本") })[0]);
  fireEvent.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: t("取消") }),
  );
  pending.resolve(
    new Response(JSON.stringify({ error: { message: "Old editor failure" } }), {
      status: 500,
    }),
  );
  await act(async () => {});
  expect(screen.queryByText("Old editor failure")).toBeNull();
});
it("shows a translated recovery action when a lazy page import fails", async () => {
  const BrokenPage = lazy(() =>
    Promise.reject(new Error("Missing page chunk")),
  );
  render(
    <PageBoundary>
      <Suspense fallback="Loading">
        <BrokenPage />
      </Suspense>
    </PageBoundary>,
  );
  await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  expect(screen.getByRole("button", { name: t("刷新页面") })).toBeTruthy();
  await act(() => setLocale("ja-JP"));
  expect(screen.getByRole("button", { name: t("刷新页面") })).toBeTruthy();
  expect(errors).toHaveBeenCalled();
  errors.mockClear();
});

it("does not append a failed audit query to results from an older filter", async () => {
  fixtures["/audit"] = {
    items: [
      {
        id: "old",
        at: Date.now(),
        actor: "Old query row",
        action: "update",
        resource: "old",
        requestId: "",
        method: "PATCH",
        path: "/old",
      },
    ],
    hasMore: true,
  };
  render(<AuditView />);
  await act(async () => {});
  expect(screen.getByText("Old query row")).toBeTruthy();
  const query = screen.getByLabelText(t("搜索审计记录"));
  fireEvent.change(query, { target: { value: "new filter" } });
  vi.mocked(fetch).mockResolvedValueOnce(
    new Response(JSON.stringify({ error: { message: "Query failed" } }), {
      status: 500,
    }),
  );
  fireEvent.submit(query.closest("form")!);
  await act(async () => {});
  expect(screen.getByText("Query failed")).toBeTruthy();
  expect(screen.queryByText("Old query row")).toBeNull();
  expect(screen.queryByRole("button", { name: t("加载更多") })).toBeNull();
});

it("polls only page dependencies and a small global alert badge", async () => {
  appStore.setState({ page: "monitors" });
  await refreshAll();
  const paths = () =>
    vi
      .mocked(fetch)
      .mock.calls.map(([path]) => String(path).replace("/api/v1", ""));
  expect(paths()).toEqual(
    expect.arrayContaining([
      "/me",
      "/me/navigation",
      "/monitors",
      "/alerts/summary",
    ]),
  );
  expect(paths()).not.toEqual(expect.arrayContaining(["/hosts"]));
  for (const path of ["/hosts", "/alerts", "/roles", "/users", "/permissions"])
    expect(paths()).not.toContain(path);
  vi.mocked(fetch).mockClear();
  appStore.setState({ page: "host" });
  await refreshAll();
  expect(paths()).toEqual(expect.arrayContaining(["/hosts", "/alerts"]));
  expect(paths()).not.toContain("/monitors");
  expect(paths()).not.toContain("/alerts/summary");
});

it("loads team data on entry and after writes, but not periodic refresh", async () => {
  appStore.setState({ page: "roles" });
  await refreshAll(true);
  for (const path of ["/roles", "/users", "/permissions"])
    expect(fetch).toHaveBeenCalledWith("/api/v1" + path, expect.anything());
  vi.mocked(fetch).mockClear();
  await refreshAll();
  for (const path of ["/roles", "/users", "/permissions"])
    expect(vi.mocked(fetch).mock.calls.map(([p]) => p)).not.toContain(
      "/api/v1" + path,
    );
  await action(async () => {});
  for (const path of ["/roles", "/users", "/permissions"])
    expect(fetch).toHaveBeenCalledWith("/api/v1" + path, expect.anything());
});

it("loads the new page after an older page's pending refresh without applying stale data", async () => {
  appStore.setState({ page: "monitors" });
  const pending = deferred();
  const base = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation((input, init) =>
    String(input) === "/api/v1/monitors" ? pending.promise : base(input, init),
  );
  const first = refreshAll();
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith("/api/v1/monitors", expect.anything()),
  );
  appStore.setState({ page: "roles" });
  const second = refreshAll(true);
  pending.resolve(response([{ id: "stale" }]));
  await Promise.all([first, second]);
  expect(appStore.getState().data.monitors).toEqual([]);
  expect(appStore.getState().data.roles).toEqual(fixtures["/roles"]);
});
