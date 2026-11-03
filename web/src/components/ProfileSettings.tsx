import { useId, useState } from "react";
import { createStore, useStore } from "zustand";
import { t, useI18n } from "../i18n";
import { errorMessage, notice, type DisplayMessage } from "../lib/messages";
import { api, appStore, notify, type User } from "../stores/app";
import Form from "./Form";
import Modal from "./Modal";

export default function ProfileSettings({ onClose }: { onClose: () => void }) {
  useI18n();
  const formId = useId();
  const [store] = useState(() =>
    createStore(() => ({
      username: appStore.getState().user?.username || "",
      name: appStore.getState().user?.name || "",
      version: appStore.getState().user?.version || 0,
      currentPassword: "",
      busy: false,
      error: "" as DisplayMessage,
    })),
  );
  const state = useStore(store);
  const close = () => {
    if (!store.getState().busy) onClose();
  };
  async function save() {
    if (store.getState().busy) return;
    const { username, name, currentPassword, version } = store.getState();
    store.setState({ busy: true, error: "" });
    try {
      const user = await api<User>("/me", "PATCH", {
        username,
        name,
        currentPassword,
        version,
      });
      appStore.setState((current) => ({
        user,
        data: {
          ...current.data,
          users: current.data.users.map((member) =>
            member.id === user.id
              ? {
                  ...member,
                  username: user.username,
                  name: user.name,
                  version: user.version,
                }
              : member,
          ),
        },
      }));
      store.setState({ currentPassword: "" });
      notify(notice("账号设置已保存"));
      onClose();
    } catch (error) {
      store.setState({ error: errorMessage(error) });
    } finally {
      store.setState({ busy: false });
    }
  }
  return (
    <Modal
      title={t("账号设置")}
      subtitle={t("修改登录用户名后，下次登录请使用新用户名。")}
      onClose={close}
      footer={
        <>
          <button className="btn" disabled={state.busy} onClick={close}>
            {t("取消")}
          </button>
          <button
            className="btn primary"
            type="submit"
            form={formId}
            disabled={state.busy}
          >
            {state.busy ? t("正在保存…") : t("保存设置")}
          </button>
        </>
      }
    >
      <Form
        id={formId}
        className="form"
        onSubmit={(event) => {
          event.preventDefault();
          void save();
        }}
      >
        <label className="field">
          {t("姓名")}
          <input
            required
            autoComplete="name"
            maxLength={80}
            disabled={state.busy}
            value={state.name}
            onChange={(event) =>
              store.setState({ name: event.currentTarget.value })
            }
          />
        </label>
        <label className="field">
          {t("登录用户名")}
          <input
            required
            minLength={3}
            maxLength={40}
            pattern="[A-Za-z0-9][A-Za-z0-9_.\-]{2,39}"
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            aria-describedby={formId + "-username-hint"}
            disabled={state.busy}
            value={state.username}
            onChange={(event) =>
              store.setState({ username: event.currentTarget.value })
            }
          />
        </label>
        <p className="muted" id={formId + "-username-hint"}>
          {t(
            "用户名需为 3–40 位小写英文字母、数字、下划线、点或连字符，且以字母或数字开头。",
          )}
        </p>
        <label className="field">
          {t("当前密码")}
          <input
            required
            type="password"
            autoComplete="current-password"
            maxLength={72}
            disabled={state.busy}
            value={state.currentPassword}
            onChange={(event) =>
              store.setState({ currentPassword: event.currentTarget.value })
            }
          />
        </label>
        {state.error ? (
          <p className="form-error" role="alert">
            {state.error}
          </p>
        ) : null}
      </Form>
    </Modal>
  );
}
