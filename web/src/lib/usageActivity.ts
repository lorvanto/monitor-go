// Record only a coarse UTC-day flag after deliberate, trusted interaction.
// This is separate from page polling: an open dashboard is not human activity.
const storageKey = "monitor-go-usage-attempt-day";
interface ActivityState {
  enabled: boolean;
  csrf: string | undefined;
}
interface ActivityOptions {
  state: () => ActivityState;
  send: (csrf: string) => Promise<unknown>;
  now?: () => Date;
  visible?: () => boolean;
  storage?: Pick<Storage, "getItem" | "setItem">;
}
export function createUsageActivityRecorder({
  state,
  send,
  now = () => new Date(),
  visible = () => !document.hidden,
  storage,
}: ActivityOptions) {
  let attemptedDay = "";
  return (event: Pick<Event, "isTrusted" | "type">) => {
    if (
      !event.isTrusted ||
      !["pointerdown", "keydown"].includes(event.type) ||
      !visible()
    )
      return;
    const current = state();
    if (!current.enabled || !current.csrf) return;
    const day = now().toISOString().slice(0, 10);
    if (day === attemptedDay) return;
    try {
      if (storage?.getItem(storageKey) === day) return;
      storage?.setItem(storageKey, day);
    } catch {
      // In-memory deduplication remains available when browser storage is blocked.
    }
    // Bound failures too. An unavailable local endpoint never creates a retry loop.
    attemptedDay = day;
    void send(current.csrf).catch(() => {});
  };
}

export function sendUsageActivity(csrf: string) {
  return fetch("/api/v1/usage/activity", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: "{}",
    signal: AbortSignal.timeout(5000),
  });
}
