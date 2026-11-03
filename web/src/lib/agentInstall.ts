export function agentInstallOrigin(server: string): string | undefined {
  try {
    const url = new URL(server.trim());
    const loopback = url.hostname === "127.0.0.1" || url.hostname === "[::1]";
    if (
      (url.protocol !== "https:" && !(url.protocol === "http:" && loopback)) ||
      url.username ||
      url.password ||
      url.search ||
      url.hash ||
      (url.pathname !== "/" && url.pathname !== "")
    )
      return undefined;
    return url.origin;
  } catch {
    return undefined;
  }
}

const quote = (value: string) => "'" + value.replaceAll("'", "'\\''") + "'";

export function agentInstallCommand(
  server: string,
  token: string,
  replace = false,
): string {
  const origin = agentInstallOrigin(server);
  if (!origin || !/^[a-f0-9]{64}$/.test(token)) return "";
  return `sh -c 'if [ "$(id -u)" -eq 0 ]; then exec sh; else exec sudo sh; fi' <<'MONITOR_GO_AGENT_INSTALL'
set -eu
umask 077
monitor_install_tmp=$(mktemp -d)
trap 'rm -rf "$monitor_install_tmp"' EXIT HUP INT TERM
curl --disable --fail --silent --show-error --tlsv1.2 --connect-timeout 15 --max-time 120 --proto '=https,http' ${quote(origin + "/api/v1/agent/install.sh")} -o "$monitor_install_tmp/install.sh"
sh "$monitor_install_tmp/install.sh" --server ${quote(origin)}${replace ? " --replace" : ""} <<'MONITOR_GO_AGENT_TOKEN'
${token}
MONITOR_GO_AGENT_TOKEN
MONITOR_GO_AGENT_INSTALL`;
}
