import { expect, it } from "vitest";
import { setLocale, t } from "../src/i18n";
import {
  alertTitle,
  alertDescription,
  alertActivityActor,
  alertActivityMessage,
  checkDiagnostic,
} from "../src/lib/incidentText";

it.each([
  [
    "en-US",
    "Synthetic outage: 73 consecutive check failures",
    "Monitoring engine",
  ],
  ["zh-CN", "Synthetic outage 连续 73 次检测异常", "检测引擎"],
  [
    "ja-JP",
    "Synthetic outage: チェックが 73 回連続で異常になりました",
    "監視エンジン",
  ],
  ["ko-KR", "Synthetic outage: 검사 73회 연속 이상", "모니터링 엔진"],
  ["zh-TW", "Synthetic outage 連續 73 次檢查異常", "檢查引擎"],
])(
  "renders historical engine records in %s without mutating them",
  async (locale, title, actor) => {
    await setLocale(locale);
    const alert = {
      kind: "monitor" as const,
      title: "Synthetic outage 连续 73 次检测异常",
      description: "dial tcp 127.0.0.1:59998: connect: connection refused",
    };
    const activity = {
      at: 1,
      actor: "检测引擎",
      message: "达到阈值，创建告警：" + alert.description,
    };
    const before = JSON.stringify({ alert, activity });
    expect(alertTitle(alert)).toBe(title);
    expect(alertDescription(alert)).toBe(alert.description);
    expect(alertActivityActor(activity)).toBe(actor);
    expect(alertActivityMessage(activity, alert)).toBe(
      t("达到阈值，创建告警：{detail}", { detail: alert.description }),
    );
    expect(
      alertActivityMessage(
        { ...activity, message: "资源恢复正常：TCP 连接成功" },
        alert,
      ),
    ).toBe(t("资源恢复正常：{detail}", { detail: t("TCP 连接成功") }));
    expect(
      alertActivityMessage(
        { ...activity, actor: "Reviewer", message: "确认并接手告警" },
        alert,
      ),
    ).toBe(t("已确认并认领告警"));
    expect(
      alertActivityMessage(
        { ...activity, actor: "Reviewer", message: "调整事件静默时间" },
        alert,
      ),
    ).toBe(t("事件静默时间已更新"));
    expect(JSON.stringify({ alert, activity })).toBe(before);
  },
);

it("localizes generated host/probe diagnostics and preserves resource paths and raw errors", async () => {
  await setLocale("en-US");
  expect(alertTitle({ kind: "host", title: "远程主机离线" })).toBe(
    "Remote host offline",
  );
  expect(alertTitle({ kind: "monitor", title: "证书测试 证书即将到期" })).toBe(
    "证书测试: certificate expires soon",
  );
  expect(
    alertDescription({
      kind: "host",
      description:
        "APFS 容器 / 使用率 90.1%；APFS 卷配额 /Volumes/资料 使用率 95.0%",
    }),
  ).toBe(
    "APFS container / usage 90.1%; APFS volume quota /Volumes/资料 usage 95.0%",
  );
  expect(
    alertDescription({
      kind: "host",
      description: "MEMORY · 91.2% / 90% · 5 min",
    }),
  ).toBe("Memory · 91.2% / 90% · 5 min");
  const diagnostics = [
    "TCP 连接成功",
    "MySQL SELECT 1 成功",
    "Redis 认证失败",
    "Redis 未返回 PONG",
    "检测超时",
    "检测凭据无法解密",
    "未找到目标地址",
    "不允许探测链路本地、元数据或组播地址",
    "重定向次数过多",
    "重定向协议不支持",
    "重定向不允许登录凭据",
    "检测类型不支持",
    "证书有效至 2026-12-01T00:00:00Z，剩余 46.5 天",
    'Get "https://example.com/": 重定向次数过多',
  ];
  for (const diagnostic of diagnostics)
    expect(checkDiagnostic(diagnostic)).not.toMatch(/[\u3400-\u9fff]/u);
  for (const raw of [
    "SQL error: 用户字段不存在",
    "Raw note: 检测超时",
    "HTTP 200 · expected 200 · custom 中文",
    "证书测试",
  ])
    expect(checkDiagnostic(raw)).toBe(raw);
  expect(alertTitle({ kind: "monitor", title: "用户自定义标题" })).toBe(
    "用户自定义标题",
  );
  const person = { at: 1, actor: "检测引擎", message: "确认并接手告警" };
  expect(alertActivityActor(person)).toBe("检测引擎");
  expect(
    alertActivityMessage(
      {
        ...person,
        actor: "Reviewer",
        message: "达到阈值，创建告警：自定义备注",
      },
      { kind: "monitor" },
    ),
  ).toBe("达到阈值，创建告警：自定义备注");
});

it.each(["en-US", "zh-CN", "ja-JP", "ko-KR", "zh-TW"])(
  "translates complete HTTP diagnostic templates in %s",
  async (locale) => {
    await setLocale(locale);
    expect(
      checkDiagnostic(
        "HTTP 200 · expected 200 · required response text not found · body truncated; only the first 1 MiB was checked",
      ),
    ).toBe(
      [
        t("HTTP {actual} · 预期 {expected}", {
          actual: "200",
          expected: "200",
        }),
        t("响应缺少必需文本"),
        t("响应体已截断，仅检查前 1 MiB"),
      ].join(" · "),
    );
    expect(
      checkDiagnostic("HTTP 200 · expected 200 · response time exceeds 250 ms"),
    ).toContain(t("响应时间超过 {milliseconds} ms", { milliseconds: "250" }));
  },
);
