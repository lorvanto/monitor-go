import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const require = createRequire(path.join(root, "web/package.json"));
const ts = require("typescript");
const codes = ["zh-CN", "en-US", "ja-JP", "ko-KR", "zh-TW"];
const catalogs = Object.fromEntries(
  codes.map((code) => [
    code,
    JSON.parse(
      fs.readFileSync(path.join(root, `locales/${code}.json`), "utf8"),
    ),
  ]),
);
const keys = Object.keys(catalogs["zh-CN"]).sort();
const tokens = (value) =>
  [...value.matchAll(/\{\w+\}|%[ds]/g)].map((m) => m[0]).sort();
for (const code of codes) {
  assert.deepEqual(
    Object.keys(catalogs[code]).sort(),
    keys,
    `${code}: key mismatch`,
  );
  for (const key of keys) {
    const value = catalogs[code][key];
    assert.equal(typeof value, "string", `${code}: ${key}`);
    assert.ok(value.trim(), `${code}: empty message ${key}`);
    if (code === "en-US")
      assert.ok(
        !/[\u3400-\u9fff]/u.test(value),
        `English catalog contains Chinese text: ${key}`,
      );
    assert.deepEqual(
      tokens(value),
      tokens(key),
      `${code}: placeholder mismatch in ${key}`,
    );
  }
}
// JSX visible text and literal attributes must use the shared catalogs.
// Message keys in expressions are checked by TypeScript.
function inspect(node, file) {
  if (ts.isJsxOpeningElement(node) && node.tagName.getText() === "form")
    assert.equal(
      file,
      path.join("components", "Form.tsx"),
      `${file}: use Form for localized browser validation`,
    );
  if (ts.isJsxText(node)) {
    assert.ok(
      !/[\u3400-\u9fff]/u.test(node.text),
      `${file}: untranslated JSX text`,
    );
  }
  if (
    ts.isJsxAttribute(node) &&
    node.initializer &&
    ts.isStringLiteral(node.initializer)
  ) {
    assert.ok(
      !/[\u3400-\u9fff]/u.test(node.initializer.text),
      `${file}: untranslated ${node.name.getText()}`,
    );
  }
  if (
    ts.isJsxExpression(node) &&
    node.expression &&
    ts.isStringLiteral(node.expression)
  ) {
    assert.ok(
      !/[\u3400-\u9fff]/u.test(node.expression.text),
      `${file}: untranslated visible string`,
    );
  }
  ts.forEachChild(node, (child) => inspect(child, file));
}
for (const file of fs
  .readdirSync(path.join(root, "web/src"), { recursive: true })
  .filter((file) => file.endsWith(".tsx"))) {
  const source = fs.readFileSync(path.join(root, "web/src", file), "utf8");
  inspect(
    ts.createSourceFile(
      file,
      source,
      ts.ScriptTarget.Latest,
      true,
      ts.ScriptKind.TSX,
    ),
    file,
  );
}
console.log(
  `Internationalization verified: ${codes.length} locales, ${keys.length} messages; placeholders and visible JSX consistent.`,
);
