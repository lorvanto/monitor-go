import { useId, useLayoutEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import Icon from "./Icon";
import LanguageSelect from "./LanguageSelect";
let openCount = 0;
let previousOverflow = "";
interface Props {
  title: string;
  subtitle?: string;
  wide?: boolean;
  drawer?: boolean;
  children?: ReactNode;
  footer?: ReactNode;
  onClose: () => void;
}
export default function Modal({
  title,
  subtitle,
  wide,
  drawer,
  children,
  footer,
  onClose,
}: Props) {
  useI18n();
  const panel = useRef<HTMLElement>(null);
  const close = useRef(onClose);
  const titleId = useId();
  useLayoutEffect(() => {
    close.current = onClose;
  });
  useLayoutEffect(() => {
    const previous = document.activeElement;
    if (openCount++ === 0) {
      previousOverflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";
    }
    function keydown(event: KeyboardEvent) {
      const dialogs = document.querySelectorAll(".overlay > [role=dialog]");
      if (!panel.current || dialogs[dialogs.length - 1] !== panel.current)
        return;
      if (event.key === "Escape") {
        event.stopPropagation();
        close.current();
      }
      if (event.key === "Tab") {
        const elements = [
          ...panel.current.querySelectorAll<HTMLElement>(
            'a[href], button:not(:disabled), input:not(:disabled):not([type="hidden"]), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]',
          ),
        ].filter((el) => el.offsetParent !== null);
        if (!elements.length) {
          event.preventDefault();
          panel.current.focus();
          return;
        }
        const first = elements[0],
          last = elements[elements.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    }
    document.addEventListener("keydown", keydown);
    const target =
      panel.current?.querySelector<HTMLElement>(
        '.modal-body input:not(:disabled):not([type="hidden"]):not([type="checkbox"]):not([type="radio"]):not([readonly]), .modal-body select:not(:disabled), .modal-body textarea:not(:disabled)',
      ) ||
      panel.current?.querySelector<HTMLElement>(
        ".modal-foot .btn.primary:not(:disabled)",
      ) ||
      panel.current?.querySelector<HTMLElement>(".modal-head .icon-btn");
    target?.focus();
    return () => {
      if (--openCount === 0) document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", keydown);
      if (previous instanceof HTMLElement && previous.isConnected)
        previous.focus();
    };
  }, []);
  return createPortal(
    <div
      className={classNames("overlay", { "overlay-drawer": drawer })}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <section
        ref={panel}
        className={classNames("modal", { wide, drawer })}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
      >
        <header className="modal-head">
          <div>
            <h2 id={titleId}>{title}</h2>
            {subtitle && <p>{subtitle}</p>}
          </div>
          <div className="modal-controls">
            <LanguageSelect />
            <button
              type="button"
              className="icon-btn"
              aria-label={t("关闭")}
              onClick={onClose}
            >
              <Icon name="X" size={18} />
            </button>
          </div>
        </header>
        <div className="modal-body">{children}</div>
        {footer && <footer className="modal-foot">{footer}</footer>}
      </section>
    </div>,
    document.body,
  );
}
