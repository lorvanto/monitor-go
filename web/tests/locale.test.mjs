import assert from "node:assert/strict";
import { test } from "node:test";
import fs from "node:fs";
import ts from "typescript";

const source = fs.readFileSync(
  new URL("../src/i18n/locale.ts", import.meta.url),
  "utf8",
);
const { outputText } = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ES2022,
  },
});
const { matchLocale, fallbackLocale, interpolate } = await import(
  `data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`
);

test("language matching supports scripts and regions with an English default", () => {
  for (const tag of ["zh-TW", "zh-HK", "zh-MO", "zh-Hant", "zh_Hant_TW"])
    assert.equal(matchLocale(tag), "zh-TW");
  for (const tag of ["zh", "zh-CN", "zh-SG", "zh-Hans"])
    assert.equal(matchLocale(tag), "zh-CN");
  assert.equal(matchLocale("ja"), "ja-JP");
  assert.equal(matchLocale("ko-KR"), "ko-KR");
  assert.equal(matchLocale("en-GB"), "en-US");
  assert.equal(matchLocale("fr-FR"), undefined);
  assert.equal(fallbackLocale, "en-US");
  assert.equal(matchLocale("english"), undefined);
});
test("interpolation permits reordered placeholders and preserves untrusted values as text", () => {
  assert.equal(
    interpolate("{name}: {count}; {name}", {
      name: "<img src=x onerror=alert(1)>",
      count: 0,
    }),
    "<img src=x onerror=alert(1)>: 0; <img src=x onerror=alert(1)>",
  );
  assert.equal(
    interpolate("{b}/{a}/{missing}", { a: 1, b: 2 }),
    "2/1/{missing}",
  );
  assert.equal(interpolate("{p}", { p: "$&{other}" }), "$&{other}");
});
