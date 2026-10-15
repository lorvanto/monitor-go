import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import HostView from "../src/views/HostView";
import OverviewView from "../src/views/OverviewView";
import { appStore, type Host, type User } from "../src/stores/app";
import { locales, setLocale, t } from "../src/i18n";
import fixtureJSON from "./api-fixtures.json";

const initial = appStore.getInitialState();
let errors: ReturnType<typeof vi.spyOn>;

function host(): Host {
  return {
    id: "mac",
    hostname: "test-mac",
    alias: "Test Mac",
    version: 1,
    source: "local",
    status: "online",
    agentEnabled: false,
    agentVersion: "",
    enrolledAt: 0,
    enrollmentExpires: 0,
    lastSeen: Date.now(),
    inventory: {
      os: "macOS 27.0.1",
      kernel: "Darwin 27.0.0",
      arch: "arm64",
      cpu: "Test CPU",
      cores: 8,
      memory: 1024,
      ips: [],
      bootTime: 1,
      source: "local",
    },
    snapshot: {
      cpu: 10,
      memory: 20,
      memoryUsed: 205,
      uptime: 100,
      netIn: 0,
      netOut: 0,
      disks: [
        { path: "/", filesystem: "apfs", used: 900, total: 1000, percent: 90 },
      ],
      errors: [],
    },
  };
}
function showHost(value: Host) {
  appStore.setState((s) => ({
    activeHost: value.id,
    data: { ...s.data, hosts: [value] },
  }));
}

beforeEach(async () => {
  appStore.setState(
    { ...initial, user: fixtureJSON["/me"] as User, booting: false },
    true,
  );
  await setLocale("en-US");
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string) => {
      const path = input.replace("/api/v1", "").split("?")[0];
      const data = path.endsWith("/metrics")
        ? []
        : ((fixtureJSON as Record<string, unknown>)[path] ?? {});
      return new Response(JSON.stringify({ data }));
    }),
  );
  errors = vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => {
  expect(errors.mock.calls).toEqual([]);
});

it.each([
  { threshold: 70, percent: 75, warn: true },
  { threshold: 95, percent: 90, warn: false },
  { threshold: 0, percent: 99, warn: false },
  { threshold: undefined, percent: 90, warn: true },
])(
  "uses the configured disk threshold $threshold in host details and overview",
  async ({ threshold, percent, warn }) => {
    const value = host();
    value.diskAlertPercent = threshold;
    value.snapshot.disks[0].percent = percent;
    showHost(value);
    const details = render(<HostView />);
    await act(async () => {});
    const row = screen.getByRole("table").querySelector("tbody tr")!;
    expect(row.querySelector(".meter")?.getAttribute("data-tone")).toBe(
      warn ? "warn" : null,
    );
    expect(
      screen.getByText(
        threshold === 0
          ? t("磁盘使用率告警已关闭")
          : t("使用率达到 {p0}% 会产生告警", { p0: threshold ?? 85 }),
      ),
    ).not.toBeNull();
    details.unmount();
    render(<OverviewView />);
    await act(async () => {});
    const overviewRow = screen.getByRole("button", { name: /Test Mac/ });
    expect(
      overviewRow
        .querySelectorAll(".host-cell .meter")[2]
        ?.getAttribute("data-tone"),
    ).toBe(warn ? "warn" : null);
  },
);

it.each(locales)(
  "$code distinguishes a shared APFS container from a volume quota and expands mount points",
  async ({ code }) => {
    await setLocale(code);
    const value = host();
    value.snapshot.disks = [
      {
        path: "/",
        filesystem: "apfs",
        used: 900,
        total: 1000,
        percent: 90,
        capacityId: "container-uuid",
        capacityKind: "apfs-container",
        mountPoints: ["/", "/System/Volumes/Data", "/System/Volumes/Preboot"],
      },
      {
        path: "/",
        filesystem: "apfs",
        used: 50,
        total: 100,
        percent: 50,
        capacityId: "quota-uuid",
        capacityKind: "apfs-quota",
        mountPoints: ["/"],
      },
    ];
    showHost(value);
    render(<HostView />);
    await act(async () => {});
    const table = screen.getByRole("table");
    expect(table.querySelectorAll("tbody tr")).toHaveLength(2);
    const containerRow = screen.getByText(t("APFS 共享容器")).closest("tr")!;
    const quotaRow = screen.getByText(t("APFS 卷配额")).closest("tr")!;
    expect(containerRow).not.toBe(quotaRow);
    const summary = within(containerRow).getByText(
      t("包含的挂载点（{p0}）", { p0: 3 }),
    );
    const disclosure = summary.closest("details")!;
    expect(disclosure.open).toBe(false);
    fireEvent.click(summary);
    expect(disclosure.open).toBe(true);
    expect(within(disclosure).getByText("/System/Volumes/Data")).not.toBeNull();
    expect(
      within(disclosure).getByText("/System/Volumes/Preboot"),
    ).not.toBeNull();
    const containerLabels = {
      "zh-CN": "APFS 共享容器",
      "en-US": "APFS shared container",
      "ja-JP": "APFS 共有コンテナ",
      "ko-KR": "APFS 공유 컨테이너",
      "zh-TW": "APFS 共用容器",
    };
    expect(screen.getByText(containerLabels[code])).not.toBeNull();
    expect(
      screen.getByText(t("APFS 容器容量共享，配额行仅显示卷限制，请勿相加。")),
    ).not.toBeNull();
  },
);

it("keeps expanded mounts and an unsaved custom host name when switching languages", async () => {
  const value = host();
  Object.assign(value.snapshot.disks[0], {
    capacityId: "container-uuid",
    capacityKind: "apfs-container",
    mountPoints: ["/", "/System/Volumes/Data"],
  });
  showHost(value);
  render(<HostView />);
  await act(async () => {});
  const summary = screen.getByText(t("包含的挂载点（{p0}）", { p0: 2 }));
  fireEvent.click(summary);
  const disclosure = summary.closest("details")!;
  fireEvent.click(screen.getByRole("button", { name: t("编辑备注") }));
  const input = within(screen.getByRole("dialog")).getByRole("textbox");
  fireEvent.change(input, { target: { value: "My Mac · 我的电脑" } });
  for (const { code } of locales) {
    await act(() => setLocale(code));
    expect((input as HTMLInputElement).value).toBe("My Mac · 我的电脑");
    expect(disclosure.open).toBe(true);
    expect(screen.getByText(t("APFS 共享容器"))).not.toBeNull();
  }
});

it("does not report healthy disks when the sample is incomplete", async () => {
  const value = host();
  value.snapshot.diskIncomplete = true;
  value.snapshot.disks[0].percent = 30;
  showHost(value);
  const details = render(<HostView />);
  await act(async () => {});
  expect(
    screen.getByText(
      "Disk sampling is incomplete. Existing alerts remain open until a complete sample can confirm recovery.",
    ),
  ).not.toBeNull();
  details.unmount();
  render(<OverviewView />);
  await act(async () => {});
  expect(screen.queryByText(t("全部运行正常"))).toBeNull();
  expect(
    screen.getAllByText(t("{p0} 台主机磁盘采样不完整", { p0: 1 })).length,
  ).toBeGreaterThan(0);
  expect(screen.getByText(t("磁盘采样不完整"))).not.toBeNull();
});

it("does not promise to retain disk alerts when the threshold is disabled", async () => {
  const value = host();
  value.diskAlertPercent = 0;
  value.snapshot.diskIncomplete = true;
  showHost(value);
  render(<HostView />);
  await act(async () => {});
  expect(screen.getByText(t("磁盘使用率告警已关闭"))).not.toBeNull();
  expect(screen.getByText(t("磁盘采样不完整"))).not.toBeNull();
  expect(
    screen.queryByText(
      t("磁盘采样不完整；保留现有告警，完整采样后再判断恢复。"),
    ),
  ).toBeNull();
});

it.each(["revoked", "pending", "offline"] as const)(
  "does not treat a $status host's historical incomplete disk sample as an active collection problem",
  async (status) => {
    const value = host();
    value.status = status;
    value.snapshot.diskIncomplete = true;
    showHost(value);
    render(<OverviewView />);
    await act(async () => {});
    expect(
      screen.queryByText(t("{p0} 台主机磁盘采样不完整", { p0: 1 })),
    ).toBeNull();
    expect(screen.queryByText(t("磁盘采样不完整"))).toBeNull();
    if (status === "offline") {
      expect(screen.queryByText(t("全部运行正常"))).toBeNull();
      expect(screen.getByText(t("{p0} 台主机离线", { p0: 1 }))).not.toBeNull();
    } else {
      expect(screen.getByText(t("全部运行正常"))).not.toBeNull();
    }
  },
);
