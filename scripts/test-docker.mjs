import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { randomUUID, createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import { createServer } from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";

// This creates disposable Compose projects and never mounts the checkout's data/.
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const project = `monitor-go-smoke-${process.pid}-${Date.now()}`;
const restoredProject = `${project}-restore`;
const listener = createServer();
await new Promise((resolve) => listener.listen(0, "127.0.0.1", resolve));
const port = listener.address().port;
await new Promise((resolve) => listener.close(resolve));
const base = `http://127.0.0.1:${port}`;
const env = {
  ...process.env,
  MONITOR_IMAGE: process.env.MONITOR_IMAGE || "monitor-go:local",
  MONITOR_PORT: String(port),
  MONITOR_BIND_ADDRESS: "127.0.0.1",
  MONITOR_PUBLIC_URL: "",
  MONITOR_SECURE_COOKIE: "false",
  MONITOR_COLLECT_HOST: "false",
  MONITOR_TRUSTED_PROXY: "",
};
const platform = process.env.MONITOR_PLATFORM;
if (platform) assert.match(platform, /^linux\/(amd64|arm64)$/);
const overrideDirectory = platform
  ? fs.mkdtempSync(path.join(os.tmpdir(), "monitor-go-docker-"))
  : null;
const override = overrideDirectory
  ? path.join(overrideDirectory, "compose.platform.json")
  : null;
if (override)
  fs.writeFileSync(
    override,
    JSON.stringify({ services: { monitor: { platform } } }),
  );
const args = (name, command) => [
  "compose",
  "-p",
  name,
  "-f",
  "compose.yaml",
  ...(override ? ["-f", override] : []),
  ...command,
];
function compose(command, options = {}, name = project) {
  return execFileSync("docker", args(name, command), {
    cwd: root,
    env,
    maxBuffer: 32 * 1024 * 1024,
    ...options,
  });
}
async function run(command, name = project) {
  await new Promise((resolve, reject) => {
    const child = spawn("docker", args(name, command), {
      cwd: root,
      env,
      stdio: "inherit",
    });
    child.on("error", reject);
    child.on("exit", (code) =>
      code === 0 ? resolve() : reject(new Error(`Compose exited with ${code}`)),
    );
  });
}
let cookie = "",
  csrf = "";
async function request(endpoint, { method = "GET", body } = {}) {
  const headers = { "Accept-Language": "en-US", Origin: base };
  if (cookie) headers.Cookie = cookie;
  if (csrf) headers["X-CSRF-Token"] = csrf;
  if (body && !(body instanceof FormData))
    headers["Content-Type"] = "application/json";
  const response = await fetch(`${base}/api/v1${endpoint}`, {
    method,
    headers,
    body:
      body instanceof FormData ? body : body ? JSON.stringify(body) : undefined,
    signal: AbortSignal.timeout(10000),
  });
  assert.ok(response.ok, `${method} ${endpoint}: HTTP ${response.status}`);
  const setCookie = response.headers.getSetCookie()[0];
  if (setCookie) cookie = setCookie.split(";")[0];
  const data = (await response.json()).data;
  return data;
}
const content = Buffer.from("Monitor Go Docker persistence verification\n");
let fileID;
async function verifyPersistence() {
  assert.equal((await request("/me")).name, "Docker smoke test");
  const response = await fetch(`${base}/api/v1/files/${fileID}/download`, {
    headers: { Cookie: cookie },
    signal: AbortSignal.timeout(10000),
  });
  assert.equal(response.status, 200);
  assert.deepEqual(Buffer.from(await response.arrayBuffer()), content);
}
try {
  console.log(
    `Building and testing Docker on ${base} using disposable volumes.`,
  );
  await run(["up", "-d", "--build", "--wait", "--wait-timeout", "120"]);
  const container = compose(["ps", "-q", "monitor"]).toString().trim();
  const details = JSON.parse(execFileSync("docker", ["inspect", container]))[0];
  assert.equal(details.Config.User, "10001:10001");
  assert.equal(details.HostConfig.ReadonlyRootfs, true);
  assert.equal(details.State.Health.Status, "healthy");
  assert.equal(
    compose(["exec", "-T", "monitor", "id", "-u"]).toString().trim(),
    "10001",
  );
  const html = await (await fetch(base)).text();
  assert.ok(html.includes('<html lang="en-US">'));
  assert.equal((await request("/status")).initialized, false);
  const token = compose(["exec", "-T", "monitor", "cat", "/data/setup-token"])
    .toString()
    .trim();
  const keyHash = compose([
    "exec",
    "-T",
    "monitor",
    "sha256sum",
    "/data/master.key",
  ]).toString();
  csrf = (
    await request("/setup", {
      method: "POST",
      body: {
        token,
        username: "docker_smoke",
        name: "Docker smoke test",
        password: `Smoke-${randomUUID()}`,
      },
    })
  ).csrf;
  compose(["exec", "-T", "monitor", "sh", "-c", "test ! -e /data/setup-token"]);
  const system = await request("/system");
  assert.equal(system.database, "SQLite");
  assert.equal(system.schema, 5);
  if (platform) assert.equal(system.platform, platform.replace("/", " / "));
  assert.deepEqual(await request("/hosts"), []);
  const folder = await request("/files/folders", {
    method: "POST",
    body: { name: "Docker test", parentId: "1" },
  });
  const upload = new FormData();
  upload.set("file", new Blob([content]), "persistence.txt");
  const file = await request(`/files/uploads?parent=${folder.id}`, {
    method: "POST",
    body: upload,
  });
  fileID = file.id;
  assert.ok(fileID);
  assert.equal(file.sha256, createHash("sha256").update(content).digest("hex"));
  await verifyPersistence();
  await run([
    "up",
    "-d",
    "--no-build",
    "--force-recreate",
    "--wait",
    "--wait-timeout",
    "120",
  ]);
  await verifyPersistence();
  assert.equal(
    compose([
      "exec",
      "-T",
      "monitor",
      "sha256sum",
      "/data/master.key",
    ]).toString(),
    keyHash,
  );
  await run(["stop", "monitor"]);
  const archive = compose([
    "run",
    "--rm",
    "--no-deps",
    "-T",
    "--entrypoint",
    "tar",
    "monitor",
    "-C",
    "/data",
    "-czf",
    "-",
    ".",
  ]);
  assert.ok(archive.length > 0);
  compose(
    [
      "run",
      "--rm",
      "--no-deps",
      "-T",
      "--entrypoint",
      "sh",
      "monitor",
      "-c",
      'test -z "$(ls -A /data)" && tar -xzf - -C /data',
    ],
    { input: archive },
    restoredProject,
  );
  await run(
    ["up", "-d", "--no-build", "--wait", "--wait-timeout", "120"],
    restoredProject,
  );
  await verifyPersistence();
  assert.equal(
    compose(
      ["exec", "-T", "monitor", "sha256sum", "/data/master.key"],
      {},
      restoredProject,
    ).toString(),
    keyHash,
  );
  console.log(
    `Docker verified on port ${port}: non-root, read-only root, health, English setup, SQLite/session/upload persistence, recreation, and cold backup/restore.`,
  );
} finally {
  for (const name of [restoredProject, project]) {
    compose(
      ["down", "--volumes", "--remove-orphans"],
      { stdio: "inherit" },
      name,
    );
  }
  if (overrideDirectory) fs.rmSync(overrideDirectory, { recursive: true });
}
