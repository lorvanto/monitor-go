import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { checkDocumentNames, checkLocalLink } from "./check-docs.mjs";

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "monitor-go-docs-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.mkdirSync(path.join(root, "docs"));
  fs.writeFileSync(path.join(root, "README.md"), "# Project\n");
  fs.writeFileSync(
    path.join(root, "docs/Guide.md"),
    "# First run\n\n## First run\n\n## 日本語\n\n```sh\n# Not a heading\n```\n",
  );
  return root;
}

test("local files, Unicode anchors and duplicate headings resolve", (t) => {
  const root = fixture(t);
  assert.equal(
    checkLocalLink(root, "README.md", "docs/Guide.md#first-run"),
    true,
  );
  assert.equal(
    checkLocalLink(root, "README.md", "docs/Guide.md#first-run-1"),
    true,
  );
  assert.equal(
    checkLocalLink(
      root,
      "README.md",
      "docs/Guide.md#%E6%97%A5%E6%9C%AC%E8%AA%9E",
    ),
    true,
  );
  assert.equal(checkLocalLink(root, "README.md", "#project"), true);
  assert.equal(
    checkLocalLink(root, "README.md", "https://example.com/"),
    false,
  );
});

test("missing files and incorrect path case fail on every platform", (t) => {
  const root = fixture(t);
  assert.throws(
    () => checkLocalLink(root, "README.md", "docs/missing.md"),
    /missing link target/,
  );
  assert.throws(
    () => checkLocalLink(root, "README.md", "docs/guide.md"),
    /missing link target|incorrect link path case/,
  );
});

test("invalid anchors and code-block comments cannot pass as headings", (t) => {
  const root = fixture(t);
  assert.throws(
    () => checkLocalLink(root, "README.md", "docs/Guide.md#missing"),
    /missing local anchor/,
  );
  assert.throws(
    () => checkLocalLink(root, "README.md", "docs/Guide.md#not-a-heading"),
    /missing local anchor/,
  );
});

test("both traversal and symlinks outside the repository are rejected", (t) => {
  const root = fixture(t);
  assert.throws(
    () => checkLocalLink(root, "README.md", "../outside.md"),
    /leaves repository/,
  );
  fs.symlinkSync(os.tmpdir(), path.join(root, "outside"), "dir");
  assert.throws(
    () => checkLocalLink(root, "README.md", "outside"),
    /resolves outside repository/,
  );
});

test("document paths must use English names without case collisions", () => {
  checkDocumentNames(["docs/zh-CN/README.md", "docs/ja-JP/README.md"]);
  assert.throws(
    () => checkDocumentNames(["docs/说明.md"]),
    /English directory and file names/,
  );
  assert.throws(
    () => checkDocumentNames(["docs/Guide.md", "docs/guide.md"]),
    /case-insensitive path collision/,
  );
});
