import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import App from "../src/App";
import ProfileSettings from "../src/components/ProfileSettings";
import { locales, setLocale, t } from "../src/i18n";
import { appStore, type User } from "../src/stores/app";
import fixtureJSON from "./api-fixtures.json";
import { fixtureData } from "./poll-fixture";

let requests: { body: unknown; headers: HeadersInit | undefined }[];
let failure = false;

beforeEach(async () => {
  await setLocale("en-US");
  const fixtures = structuredClone(fixtureJSON) as Record<string, any>;
  const user = { ...fixtures["/me"], permissions: [] } as User;
  fixtures["/me"] = user;
  appStore.setState(
    { ...appStore.getInitialState(), user, booting: false },
    true,
  );
  requests = [];
  failure = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, init?: RequestInit) => {
      if (input === "/api/v1/me" && init?.method === "PATCH") {
        const body = JSON.parse(String(init.body));
        requests.push({ body, headers: init.headers });
        if (failure) {
          return new Response(
            JSON.stringify({
              error: {
                key: "当前密码不正确",
                message: "Current password is incorrect",
              },
            }),
            { status: 422 },
          );
        }
        fixtures["/me"] = {
          ...user,
          username: body.username.trim().toLowerCase(),
          name: body.name.trim(),
          version: user.version + 1,
        };
        return new Response(JSON.stringify({ data: fixtures["/me"] }), {
          status: 200,
        });
      }
      return new Response(
        JSON.stringify({ data: fixtureData(fixtures, input) ?? {} }),
        { status: 200 },
      );
    }),
  );
});

function editProfile() {
  const name = screen.getByLabelText(t("姓名")) as HTMLInputElement;
  const username = screen.getByLabelText(t("登录用户名")) as HTMLInputElement;
  const password = screen.getByLabelText(t("当前密码")) as HTMLInputElement;
  fireEvent.change(username, { target: { value: "new.login" } });
  fireEvent.change(name, { target: { value: "  New Display Name  " } });
  fireEvent.change(password, { target: { value: "Current-password-123!" } });
  return { name, username, password };
}

it("saves the login username, updates the current user, and retains the session", async () => {
  const close = vi.fn();
  const csrf = appStore.getState().user?.csrf;
  render(<ProfileSettings onClose={close} />);
  editProfile();
  fireEvent.click(screen.getByRole("button", { name: t("保存设置") }));
  await waitFor(() => expect(close).toHaveBeenCalledOnce());
  expect(appStore.getState().user?.username).toBe("new.login");
  expect(appStore.getState().user?.name).toBe("New Display Name");
  expect(appStore.getState().user?.csrf).toBe(csrf);
  expect(requests[0].body).toEqual({
    username: "new.login",
    name: "  New Display Name  ",
    currentPassword: "Current-password-123!",
    version: 1,
  });
  expect(new Headers(requests[0].headers).get("X-CSRF-Token")).toBe(csrf);
});

it("retains drafts and translates password failures after a language switch", async () => {
  failure = true;
  const close = vi.fn();
  render(<ProfileSettings onClose={close} />);
  const { name, username, password } = editProfile();
  fireEvent.submit(username.closest("form")!);
  await waitFor(() =>
    expect(screen.getByRole("alert").textContent).toBe(
      "Current password is incorrect",
    ),
  );
  await act(() => setLocale("ja-JP"));
  expect(screen.getByRole("alert").textContent).toBe(t("当前密码不正确"));
  expect(username.value).toBe("new.login");
  expect(name.value).toBe("  New Display Name  ");
  expect(password.value).toBe("Current-password-123!");
  expect(appStore.getState().user?.username).toBe("react-admin");
  expect(close).not.toHaveBeenCalled();
});

it("preserves all unsaved fields through all five languages", async () => {
  render(<ProfileSettings onClose={() => {}} />);
  const { name, username, password } = editProfile();
  for (const locale of locales) {
    await act(() => setLocale(locale.code));
    expect(screen.getByLabelText(t("登录用户名"))).toBe(username);
    expect(username.value).toBe("new.login");
    expect(screen.getByLabelText(t("姓名"))).toBe(name);
    expect(name.value).toBe("  New Display Name  ");
    expect(password.value).toBe("Current-password-123!");
  }
  expect(requests).toHaveLength(0);
});

it("opens account settings from the sidebar for a user without management permissions", async () => {
  render(<App />);
  const account = await screen.findByRole("button", {
    name: /React Test react-admin/,
  });
  fireEvent.click(account);
  fireEvent.click(
    within(screen.getByRole("menu")).getByRole("menuitem", {
      name: t("账号设置"),
    }),
  );
  expect(screen.getByRole("dialog", { name: t("账号设置") })).toBeTruthy();
  expect(screen.getByLabelText(t("登录用户名"))).toBeTruthy();
  editProfile();
  fireEvent.click(screen.getByRole("button", { name: t("保存设置") }));
  await screen.findByRole("button", { name: /New Display Name new.login/ });
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("can change only the full name while preserving the login username", async () => {
  const close = vi.fn();
  render(<ProfileSettings onClose={close} />);
  fireEvent.change(screen.getByLabelText(t("姓名")), {
    target: { value: "Display only" },
  });
  fireEvent.change(screen.getByLabelText(t("当前密码")), {
    target: { value: "Current-password-123!" },
  });
  fireEvent.click(screen.getByRole("button", { name: t("保存设置") }));
  await waitFor(() => expect(close).toHaveBeenCalledOnce());
  expect(appStore.getState().user?.name).toBe("Display only");
  expect(appStore.getState().user?.username).toBe("react-admin");
});
