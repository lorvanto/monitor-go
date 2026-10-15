import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useState } from "react";
import Modal from "../src/components/Modal";
import { locales, setLocale, t, useI18n } from "../src/i18n";

class PhoneViewport extends EventTarget {
  height = 568;
  offsetTop = 0;
}
let viewport: PhoneViewport;
beforeEach(async () => {
  viewport = new PhoneViewport();
  vi.stubGlobal("visualViewport", viewport);
  await setLocale("en-US");
});
afterEach(() => vi.unstubAllGlobals());

it.each(locales)(
  "keeps the editor and actions in the visible viewport without losing $code drafts",
  async ({ code }) => {
    let saved = 0,
      closed = 0;
    function Editor() {
      useI18n();
      const [value, setValue] = useState("");
      return (
        <Modal
          title={t("编辑应用")}
          subtitle={"LongApplicationName".repeat(6)}
          onClose={() => closed++}
          footer={<button onClick={() => saved++}>{t("保存")}</button>}
        >
          <label>
            {t("说明")}
            <textarea
              value={value}
              onChange={(event) => setValue(event.currentTarget.value)}
            />
          </label>
        </Modal>
      );
    }
    render(<Editor />);
    const dialog = screen.getByRole("dialog"),
      overlay = dialog.parentElement!;
    const input = within(dialog).getByRole("textbox") as HTMLTextAreaElement;
    fireEvent.change(input, {
      target: { value: "Unsubmitted / 未保存 / 下書き / 초안" },
    });
    input.focus();
    vi.spyOn(input, "getBoundingClientRect").mockReturnValue({
      top: 350,
      bottom: 384,
    } as DOMRect);
    vi.spyOn(
      dialog.querySelector(".modal-body")!,
      "getBoundingClientRect",
    ).mockReturnValue({ top: 100, bottom: 280 } as DOMRect);
    // Keyboard opening, viewport panning and landscape/keyboard closing.
    for (const [height, top, event] of [
      [280, 0, "resize"],
      [280, 95, "scroll"],
      [360, 0, "resize"],
    ] as const) {
      act(() => {
        viewport.height = height;
        viewport.offsetTop = top;
        viewport.dispatchEvent(new Event(event));
      });
      expect(overlay.style.height).toBe(`${height}px`);
      expect(overlay.style.top).toBe(`${top}px`);
      expect(overlay.style.bottom).toBe("auto");
      expect(document.activeElement).toBe(input);
      expect(input.value).toBe("Unsubmitted / 未保存 / 下書き / 초안");
      expect(input.scrollIntoView).toHaveBeenCalledWith({
        block: "nearest",
        inline: "nearest",
      });
    }
    fireEvent.change(
      within(dialog).getByRole("combobox", { name: t("界面语言") }),
      { target: { value: code } },
    );
    await waitFor(() => expect(document.documentElement.lang).toBe(code));
    expect(screen.getByRole("dialog")).toBe(dialog);
    expect(input.value).toBe("Unsubmitted / 未保存 / 下書き / 초안");
    expect(saved).toBe(0);
    fireEvent.click(within(dialog).getByRole("button", { name: t("保存") }));
    fireEvent.click(within(dialog).getByRole("button", { name: t("关闭") }));
    expect(saved).toBe(1);
    expect(closed).toBe(1);
  },
);

it("removes viewport listeners when an editor closes", () => {
  const remove = vi.spyOn(viewport, "removeEventListener");
  const view = render(
    <Modal title="Test" onClose={() => {}}>
      Test
    </Modal>,
  );
  view.unmount();
  expect(remove.mock.calls.map(([name]) => name)).toEqual(["resize", "scroll"]);
  expect(document.body.style.overflow).toBe("");
});
