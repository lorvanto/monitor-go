import { type CSSProperties, Fragment } from "react";
import { t, useI18n } from "../i18n";
import { classNames } from "../lib/react";
import { type NavItem, displayNavName, navigate, useApp } from "../stores/app";
import Icon from "./Icon";
export default function NavTree(inputProps: {
  items: NavItem[];
  depth?: number;
}) {
  const props = { ...{}, ...inputProps };
  const appState = useApp(({ page, openAlertCount }) => ({
    page,
    openAlertCount,
  }));
  useI18n();

  return (
    <>
      <>
        {props.items.map((item) => (
          <Fragment key={item.id}>
            {item.kind === "group" ? (
              <div
                style={{ "--depth": props.depth || 0 } as CSSProperties}
                className="nav-group"
              >
                <div className="nav-caption">{displayNavName(item)}</div>
                <NavTree
                  items={item.children || []}
                  depth={(props.depth || 0) + 1}
                ></NavTree>
              </div>
            ) : item.kind === "link" ? (
              <a
                style={
                  {
                    "--depth": Math.max(0, (props.depth || 0) - 1),
                  } as CSSProperties
                }
                href={item.url}
                target="_blank"
                rel="noopener noreferrer"
                className="nav-item"
              >
                <Icon name={item.icon} size={16}></Icon>
                <span className="nav-label">{displayNavName(item)}</span>
                <Icon
                  name="ArrowUpRight"
                  size={13}
                  className="nav-external"
                ></Icon>
              </a>
            ) : (
              <button
                style={
                  {
                    "--depth": Math.max(0, (props.depth || 0) - 1),
                  } as CSSProperties
                }
                aria-current={
                  appState.page === item.pageKey ? "page" : undefined
                }
                className={classNames("nav-item", {
                  active: appState.page === item.pageKey,
                })}
                onClick={() => {
                  navigate(item.pageKey);
                }}
              >
                <Icon name={item.icon} size={16}></Icon>
                <span className="nav-label">{displayNavName(item)}</span>
                {item.pageKey === "alerts" && appState.openAlertCount ? (
                  <span
                    aria-label={t("{p0} 条未恢复告警", {
                      p0: appState.openAlertCount,
                    })}
                    className="nav-count"
                  >
                    {appState.openAlertCount}
                  </span>
                ) : null}
              </button>
            )}
          </Fragment>
        ))}
      </>
    </>
  );
}
