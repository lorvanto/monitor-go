import { test } from "node:test";
import assert from "node:assert/strict";
import { catalogLanguageIssue } from "../../scripts/i18n-policy.mjs";

test("detects mixed scripts and copied English without rejecting shared names or Japanese kanji", () => {
  for (const [code, value, english] of [
    ["ko-KR", "저장 保存", "Save"],
    ["ja-JP", "保存 저장", "Save"],
    ["ja-JP", "请填写名称", "Enter a name"],
    ["zh-TW", "請输入名稱", "Enter a name"],
    ["zh-TW", "登錄", "Sign in"],
    ["en-US", "保存", "Save"],
    ["ja-JP", "Please enter a name", "Please enter a name"],
    ["ko-KR", "Please enter a name", "Please enter a name"],
    ["zh-TW", "Please enter a name", "Please enter a name"],
  ])
    assert.ok(
      catalogLanguageIssue(code, "测试", value, english),
      `${code}: ${value}`,
    );
  for (const [code, key, value, english] of [
    ["ja-JP", "操作", "操作", "Action"],
    ["ja-JP", "保存", "保存", "Save"],
    ["ja-JP", "Webhook 地址", "Webhook URL", "Webhook URL"],
    ["ko-KR", "钉钉", "DingTalk", "DingTalk"],
    ["ko-KR", "SMTP 主机", "SMTP 호스트", "SMTP host"],
    ["zh-TW", "用户名", "使用者名稱", "Username"],
    ["zh-TW", "输入", "請輸入名稱", "Enter a name"],
  ])
    assert.equal(catalogLanguageIssue(code, key, value, english), "");
});
