// Shared wire fixture for the authenticated page polling endpoint.
export function fixtureData(
  fixtures: Record<string, any>,
  input: string,
): unknown {
  const url = new URL(input, "http://localhost");
  const path = url.pathname.replace("/api/v1", "");
  if (path === "/poll") {
    const page = url.searchParams.get("page");
    return {
      user: fixtures["/me"],
      navigation: fixtures["/me/navigation"],
      hosts: ["overview", "host"].includes(page || "")
        ? fixtures["/hosts"]
        : [],
      monitors: ["overview", "monitors"].includes(page || "")
        ? fixtures["/monitors"]
        : [],
      alerts: ["overview", "host", "alerts"].includes(page || "")
        ? fixtures["/alerts"].map((a: any) => ({ ...a, activity: [] }))
        : [],
      openAlertCount: fixtures["/alerts/summary"].open,
    };
  }
  if (/^\/alerts\/[^/]+\/activity$/.test(path))
    return (
      fixtures[path] ??
      fixtures["/alerts"].find((a: any) => a.id === path.split("/")[2])
        ?.activity ??
      []
    );
  return fixtures[path];
}
