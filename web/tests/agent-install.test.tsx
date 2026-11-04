import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import AgentAccess from "../src/components/AgentAccess";
import {
  agentInstallCommand,
  agentInstallOrigin,
} from "../src/lib/agentInstall";
import { appStore, type User } from "../src/stores/app";
import { setLocale, t } from "../src/i18n";
import fixtureJSON from "./api-fixtures.json";
import { fixtureData } from "./poll-fixture";

const token = "a".repeat(64);
let clipboard: ReturnType<typeof vi.fn>;
beforeEach(async () => {
  appStore.setState(
    {
      ...appStore.getInitialState(),
      user: fixtureJSON["/me"] as User,
      initialized: true,
      booting: false,
    },
    true,
  );
  await setLocale("en-US");
  clipboard = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: clipboard },
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string) => {
      const path = new URL(input, "http://localhost").pathname;
      const data =
        path === "/api/v1/hosts/agents"
          ? { id: "42", token, expiresAt: Date.now() + 900000 }
          : path === "/api/v1/agent/downloads"
            ? {
                platforms: [
                  { os: "linux", arch: "amd64" },
                  { os: "linux", arch: "arm64" },
                ],
              }
            : (fixtureData(fixtureJSON, input) ?? {});
      return new Response(JSON.stringify({ data }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
});

it("accepts HTTPS origins and loopback development without allowing URL credentials or paths", () => {
  expect(agentInstallOrigin("https://monitor.example.com:5080/")).toBe(
    "https://monitor.example.com:5080",
  );
  expect(agentInstallOrigin("http://127.0.0.1:5097")).toBe(
    "http://127.0.0.1:5097",
  );
  for (const input of [
    "http://example.com",
    "https://user:pass@example.com",
    "https://example.com/install",
    "https://example.com?token=x",
    "https://example.com/#x",
    "https://example.com\n'; touch /tmp/no; '",
  ]) {
    expect(agentInstallOrigin(input)).toBeUndefined();
    expect(agentInstallCommand(input, token)).toBe("");
  }
  const command = agentInstallCommand(
    "https://monitor.example.com:5080",
    token,
    true,
  );
  expect(command).toContain("--replace <<'MONITOR_GO_AGENT_TOKEN'");
  expect(command).toContain(`\n${token}\nMONITOR_GO_AGENT_TOKEN`);
  expect(command).not.toContain(`--token ${token}`);
  expect(command).not.toContain(`?token=${token}`);
  expect(command).not.toContain(" -k ");
});

it("copies a complete installer command and preserves the URL and enrollment code across language switches", async () => {
  render(<AgentAccess />);
  fireEvent.click(screen.getByRole("button", { name: t("接入远程主机") }));
  fireEvent.change(screen.getByPlaceholderText(t("例如：生产应用服务器")), {
    target: { value: "Remote Linux" },
  });
  fireEvent.click(screen.getByRole("button", { name: t("生成接入码") }));
  const copy = await screen.findByRole("button", { name: t("复制安装命令") });
  const dialog = screen.getByRole("dialog");
  const server = within(dialog).getByLabelText(t("服务地址"));
  fireEvent.change(server, {
    target: { value: "https://monitor.example.com:5080" },
  });
  fireEvent.click(copy);
  await waitFor(() =>
    expect(clipboard).toHaveBeenCalledWith(
      agentInstallCommand("https://monitor.example.com:5080", token),
    ),
  );
  for (const code of ["zh-CN", "ja-JP", "ko-KR", "zh-TW", "en-US"] as const) {
    await act(() => setLocale(code));
    expect(
      (within(dialog).getByLabelText(t("服务地址")) as HTMLInputElement).value,
    ).toBe("https://monitor.example.com:5080");
    expect(within(dialog).getByDisplayValue(token)).toBeTruthy();
    expect(
      (
        within(dialog).getByRole("button", {
          name: t("复制安装命令"),
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false);
  }
  fireEvent.change(within(dialog).getByLabelText(t("服务地址")), {
    target: { value: "http://remote.example.com" },
  });
  expect(
    (
      within(dialog).getByRole("button", {
        name: t("复制安装命令"),
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  expect(within(dialog).getByRole("alert")).toBeTruthy();
});
