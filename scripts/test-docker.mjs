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
  MONITOR_VERSION: process.env.MONITOR_VERSION || "0.4.0-dev",
  MONITOR_IMAGE: process.env.MONITOR_IMAGE || "monitor-go:local",
  MONITOR_PORT: String(port),
  MONITOR_BIND_ADDRESS: "127.0.0.1",
  MONITOR_PUBLIC_URL: "",
  MONITOR_SECURE_COOKIE: "false",
  MONITOR_COLLECT_HOST: "false",
  MONITOR_TRUSTED_PROXY: "",
  MONITOR_CONSOLE_URL: "http://127.0.0.1:5099",
  MONITOR_REQUIRE_ACTIVATION: "true",
};
const platform = process.env.MONITOR_PLATFORM;
if (platform) assert.match(platform, /^linux\/(amd64|arm64)$/);
const overrideDirectory = fs.mkdtempSync(
  path.join(os.tmpdir(), "monitor-go-docker-"),
);
const fixtureBinary = path.join(overrideDirectory, "usage-console");
const override = path.join(overrideDirectory, "compose.test.json");
fs.writeFileSync(
  override,
  JSON.stringify({
    services: {
      monitor: {
        ...(platform ? { platform } : {}),
        volumes: [
          {
            type: "bind",
            source: fixtureBinary,
            target: "/opt/monitor-smoke-console",
            read_only: true,
          },
        ],
      },
    },
  }),
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
let fileID, instanceID;
async function verifyPersistence() {
  const usage = await request("/usage");
  assert.equal(usage.enabled, true);
  assert.equal(usage.required, true);
  assert.equal(usage.confirmationRequired, false);
  assert.equal(usage.configured, true);
  assert.equal(usage.instanceId, instanceID);
  assert.deepEqual(usage.pendingActiveDays, []);
  assert.equal((await request("/me")).name, "Docker smoke test");
  const response = await fetch(`${base}/api/v1/files/${fileID}/download`, {
    headers: { Cookie: cookie },
    signal: AbortSignal.timeout(10000),
  });
  assert.equal(response.status, 200);
  assert.deepEqual(Buffer.from(await response.arrayBuffer()), content);
}
try {
  // The fixture binds only the container's own loopback. No production Console
  // receives test data; the application keeps all normal TLS and network rules.
  const architecture =
    platform?.split("/")[1] ||
    { aarch64: "arm64", arm64: "arm64", x86_64: "amd64", amd64: "amd64" }[
      execFileSync("docker", ["info", "--format", "{{.Architecture}}"], {
        encoding: "utf8",
      }).trim()
    ];
  assert.ok(architecture, "unsupported Docker architecture");
  execFileSync(
    "go",
    ["build", "-o", fixtureBinary, "./scripts/testfixtures/usage-console"],
    {
      cwd: root,
      env: {
        ...process.env,
        GOOS: "linux",
        GOARCH: architecture,
        CGO_ENABLED: "0",
      },
      stdio: "inherit",
    },
  );
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
  const activationStatus = await request("/status");
  assert.deepEqual(activationStatus.activation, {
    required: true,
    configured: true,
    consoleUrl: env.MONITOR_CONSOLE_URL,
    noticeVersion: "2",
  });
  const setupBody = {
    token,
    username: "docker_smoke",
    name: "Docker smoke test",
    password: `Smoke-${randomUUID()}`,
  };
  for (const [consent, expectedStatus, message] of [
    [false, 422, "Activation consent is required"],
    [true, 502, "Activation service is unavailable; try again"],
  ]) {
    const blocked = await fetch(`${base}/api/v1/setup`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Accept-Language": "en-US",
        Origin: base,
      },
      body: JSON.stringify({
        ...setupBody,
        usageConsent: consent,
        usageNoticeVersion: "2",
        usageConsoleUrl: env.MONITOR_CONSOLE_URL,
      }),
      signal: AbortSignal.timeout(10000),
    });
    assert.equal(blocked.status, expectedStatus);
    assert.equal((await blocked.json()).error.key, message);
    assert.equal((await request("/status")).initialized, false);
  }
  // Supply a local Console fixture for mandatory activation. It is not started
  // after recreation/restore, proving subsequent operation survives an outage.
  env.MONITOR_CONSOLE_URL = "http://127.0.0.1:5099";
  await run([
    "up",
    "-d",
    "--no-build",
    "--force-recreate",
    "--wait",
    "--wait-timeout",
    "120",
  ]);
  compose(["exec", "-d", "monitor", "/opt/monitor-smoke-console"]);
  compose([
    "exec",
    "-T",
    "monitor",
    "sh",
    "-c",
    "for i in 1 2 3 4 5; do wget -q -O /dev/null http://127.0.0.1:5099/health && exit 0; sleep 1; done; exit 1",
  ]);
  assert.equal((await request("/status")).activation.required, true);
  csrf = (
    await request("/setup", {
      method: "POST",
      body: {
        ...setupBody,
        usageConsent: true,
        usageNoticeVersion: "2",
        usageConsoleUrl: env.MONITOR_CONSOLE_URL,
      },
    })
  ).csrf;
  compose(["exec", "-T", "monitor", "sh", "-c", "test ! -e /data/setup-token"]);
  const system = await request("/system");
  assert.equal(system.database, "SQLite");
  assert.equal(system.schema, 12);
  const usage = await request("/usage");
  assert.equal(usage.enabled, true);
  assert.equal(usage.required, true);
  assert.equal(usage.confirmationRequired, false);
  assert.equal(usage.noticeVersion, "2");
  assert.match(usage.instanceId, /^[0-9a-f]{32}$/);
  instanceID = usage.instanceId;
  const disable = await fetch(`${base}/api/v1/usage`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
      Origin: base,
      Cookie: cookie,
      "X-CSRF-Token": csrf,
    },
    body: JSON.stringify({ enabled: false }),
  });
  assert.equal(disable.status, 403);
  assert.equal(
    (await disable.json()).error.key,
    "Usage reporting is required and cannot be disabled",
  );
  assert.equal((await request("/usage")).enabled, true);
  assert.deepEqual(Object.keys(usage.preview).sort(), [
    "activeDays",
    "instanceId",
    "os",
    "version",
  ]);
  assert.equal(usage.preview.version, env.MONITOR_VERSION);
  assert.equal(usage.preview.os, "linux");
  assert.equal(Object.hasOwn(usage, "account"), false);
  assert.equal(system.version, env.MONITOR_VERSION);
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
  compose([
    "exec",
    "-T",
    "monitor",
    "monitor",
    "backup",
    "-data",
    "/data",
    "-out",
    "/data/online-backup",
  ]);
  const onlineArchive = compose([
    "exec",
    "-T",
    "monitor",
    "tar",
    "-C",
    "/data/online-backup",
    "-czf",
    "-",
    ".",
  ]);
  await verifyPersistence();
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
  await run(["down", "--volumes", "--remove-orphans"], restoredProject);
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
    { input: onlineArchive },
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
    `Docker verified on port ${port}: non-root, read-only root, health, English setup, mandatory disclosed activation, rejected opt-out, SQLite/session/upload persistence, recreation, cold backup/restore, and online backup/restore.`,
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
