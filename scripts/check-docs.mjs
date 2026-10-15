import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptPath = fileURLToPath(import.meta.url);
const codes = ["en-US", "zh-CN", "ja-JP", "ko-KR", "zh-TW"];
const withoutCode = (source) =>
  source.replace(/^(`{3,}|~{3,})[^\n]*\n[\s\S]*?^\1\s*$/gm, "");
const inlineLinks = (source) =>
  [...source.matchAll(/\[[^\]]*\]\((<[^>]+>|[^\s)]+)(?:\s+"[^"]*")?\)/g)].map(
    (match) => match[1].replace(/^<|>$/g, ""),
  );

function inside(root, target) {
  const relative = path.relative(root, target);
  return (
    relative !== ".." &&
    !relative.startsWith(`..${path.sep}`) &&
    !path.isAbsolute(relative)
  );
}

export function checkDocumentNames(files) {
  const seen = new Set();
  for (const file of files) {
    assert.match(
      file,
      /^[\x20-\x7e]+$/,
      `${file}: use English directory and file names`,
    );
    assert.ok(
      !seen.has(file.toLowerCase()),
      `${file}: case-insensitive path collision`,
    );
    seen.add(file.toLowerCase());
  }
}

export function checkLocalLink(root, file, target) {
  if (/^[a-z][a-z0-9+.-]*:/i.test(target)) return false;
  const [location, fragment] = target.split("#", 2);
  const pathname = decodeURIComponent(location.split("?")[0]);
  const resolved = pathname
    ? path.resolve(root, path.dirname(file), pathname)
    : path.resolve(root, file);
  assert.ok(
    inside(root, resolved),
    `${file}: link leaves repository: ${target}`,
  );
  assert.ok(fs.existsSync(resolved), `${file}: missing link target: ${target}`);
  assert.ok(
    inside(fs.realpathSync(root), fs.realpathSync(resolved)),
    `${file}: link resolves outside repository: ${target}`,
  );
  // macOS commonly accepts incorrect path case; GitHub and Linux do not.
  let parent = root;
  for (const part of path
    .relative(root, resolved)
    .split(path.sep)
    .filter(Boolean)) {
    assert.ok(
      fs.readdirSync(parent).includes(part),
      `${file}: incorrect link path case: ${target}`,
    );
    parent = path.join(parent, part);
  }
  if (fragment) {
    let document = resolved;
    if (fs.statSync(document).isDirectory())
      document = path.join(document, "README.md");
    assert.ok(
      document.endsWith(".md") && fs.existsSync(document),
      `${file}: unsupported local anchor target: ${target}`,
    );
    const source = withoutCode(fs.readFileSync(document, "utf8"));
    const ids = new Set();
    for (const match of source.matchAll(
      /^ {0,3}#{1,6}\s+(.+?)(?:\s+#+)?\s*$/gm,
    )) {
      const base = match[1]
        .replace(/<[^>]*>/g, "")
        .toLowerCase()
        .replace(/[^\p{L}\p{M}\p{N}_\- ]/gu, "")
        .replace(/ /g, "-");
      let id = base;
      let suffix = 0;
      while (ids.has(id)) id = `${base}-${++suffix}`;
      ids.add(id);
    }
    for (const match of source.matchAll(/\b(?:id|name)=["']([^"']+)["']/g))
      ids.add(match[1]);
    assert.ok(
      ids.has(decodeURIComponent(fragment)),
      `${file}: missing local anchor: ${target}`,
    );
  }
  return true;
}

export function checkDocumentation(root) {
  const files = fs.readdirSync(root).filter((file) => file.endsWith(".md"));
  for (const directory of ["docs", ".github"]) {
    const entries = fs
      .readdirSync(path.join(root, directory), { recursive: true })
      .map((file) => path.join(directory, file));
    checkDocumentNames(entries);
    files.push(...entries.filter((file) => file.endsWith(".md")));
  }
  checkDocumentNames(files);
  let links = 0;
  for (const file of files) {
    const raw = fs.readFileSync(path.join(root, file), "utf8");
    if (file.startsWith(path.join(".github", "ISSUE_TEMPLATE") + path.sep)) {
      const frontmatter = raw.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/);
      assert.ok(frontmatter, `${file}: missing issue template frontmatter`);
      for (const key of ["name", "about"]) {
        const field = frontmatter[1].match(
          new RegExp(`^${key}:[ \\t]*(.*)$`, "m"),
        );
        const value = field?.[1].trim().replace(/^(["'])(.*)\1$/, "$2");
        assert.ok(value, `${file}: missing ${key}`);
      }
    }
    const source = withoutCode(raw);
    const references = new Map(
      [...source.matchAll(/^ {0,3}\[([^\]]+)\]:\s*(<[^>]+>|\S+)/gm)].map(
        (match) => [match[1].toLowerCase(), match[2].replace(/^<|>$/g, "")],
      ),
    );
    for (const match of source.matchAll(/\[([^\]]+)\]\[([^\]]*)\]/g)) {
      assert.ok(
        references.has((match[2] || match[1]).toLowerCase()),
        `${file}: missing link reference: ${match[0]}`,
      );
    }
    for (const target of [...inlineLinks(source), ...references.values()]) {
      if (checkLocalLink(root, file, target)) links++;
    }
  }
  // Every translation must remain reachable from the corresponding guide family.
  for (const code of codes) {
    for (const family of [
      "README.md",
      "INTERNATIONALIZATION.md",
      "GITHUB.md",
      "DOCKER.md",
    ]) {
      const name =
        family === "GITHUB.md" && code === "en-US" ? "RELEASING.md" : family;
      const file = `docs/${code}/${name}`;
      assert.ok(
        files.includes(path.normalize(file)),
        `${file}: missing current guide`,
      );
      const targets = inlineLinks(
        withoutCode(fs.readFileSync(path.join(root, file), "utf8")),
      );
      for (const other of codes.filter((value) => value !== code)) {
        const translatedName =
          family === "GITHUB.md" && other === "en-US" ? "RELEASING.md" : family;
        assert.ok(
          targets.includes(`../${other}/${translatedName}`),
          `${file}: missing ${other} navigation`,
        );
      }
    }
  }
  return { files: files.length, links, locales: codes.length };
}

if (process.argv[1] && path.resolve(process.argv[1]) === scriptPath) {
  const result = checkDocumentation(
    path.resolve(path.dirname(scriptPath), ".."),
  );
  console.log(
    `Documentation verified: ${result.files} Markdown files, ${result.links} local links, ${result.locales} language navigation sets; names, path case and anchors checked.`,
  );
}
