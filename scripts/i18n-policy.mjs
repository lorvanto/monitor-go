// These checks cover application-owned catalog values, never user data.
// Shared product/protocol names are intentional and need no translation.
const sharedNames = new Map([
  ["钉钉", "DingTalk"],
  ["飞书", "Feishu"],
  ["企业微信", "WeCom"],
  ["Webhook 通知", "Webhook"],
  ["Webhook 地址", "Webhook URL"],
]);
const traditionalTerms =
  /登錄|啓|密鑰|歸檔|創建|內置|端口|支持|響應|消息|使用者名(?!稱)|隊列|遠程|訪問|搜索|添加|導出|刷新/u;
const simplifiedCharacters =
  /[这为网页请录设删传输数认账钥码档错证储启应载务获态选权阈户报执标图览连线测内时区关总显检]/u;
const sharedJapaneseWords = new Set([
  "操作",
  "正常",
  "警告",
  "保存",
  "容量",
  "使用率",
  "未提供",
  "最新",
]);

export function catalogLanguageIssue(code, key, value, english) {
  if (
    code === "en-US" &&
    /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}]/u.test(
      value,
    )
  )
    return "non-English script";
  if (code === "ja-JP" && /\p{Script=Hangul}/u.test(value))
    return "Korean text in Japanese";
  if (
    code === "ja-JP" &&
    (/[这请页输检测账户录设删传认账钥码档证储启应载获态选权阈报执标图览总显]/u.test(
      value,
    ) ||
      (value === key && !sharedJapaneseWords.has(key)))
  )
    return "untranslated Chinese message in Japanese";
  if (
    code === "ko-KR" &&
    /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]/u.test(value)
  )
    return "Chinese/Japanese script in Korean";
  if (
    code === "zh-TW" &&
    (simplifiedCharacters.test(value) ||
      /[\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}]/u.test(value))
  )
    return "non-Traditional-Chinese script";
  if (code === "zh-TW" && traditionalTerms.test(value))
    return "inconsistent Traditional Chinese terminology";
  if (
    ["ja-JP", "ko-KR", "zh-TW"].includes(code) &&
    value === english &&
    /[A-Za-z]/u.test(value) &&
    sharedNames.get(key) !== value
  )
    return "untranslated English message";
  return "";
}
