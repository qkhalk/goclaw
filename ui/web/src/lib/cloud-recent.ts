/** "Gần đây" (Recent) — purely client-side recents, no backend.
 *
 * Every file OPEN (preview/download) and folder NAVIGATION pushes one entry
 * into a per-user localStorage list (key `cloud.recent.<userID>`), capped at
 * 50 with dedupe by account+path (the re-visited entry moves to the front).
 */

export interface CloudRecentEntry {
  accountId: string;
  provider: string;
  email: string;
  /** Encoded remote path (same format as the `?path=` URL param). */
  path: string;
  name: string;
  isDir: boolean;
  at: number;
}

const KEY_PREFIX = "cloud.recent.";
export const RECENT_CAP = 50;

function storageKey(userId: string): string {
  return KEY_PREFIX + (userId || "anonymous");
}

export function readRecents(userId: string): CloudRecentEntry[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(storageKey(userId));
    if (!raw) return [];
    const parsed = JSON.parse(raw) as CloudRecentEntry[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function writeRecents(userId: string, entries: CloudRecentEntry[]): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(storageKey(userId), JSON.stringify(entries.slice(0, RECENT_CAP)));
  } catch {
    // Storage full / disabled — recents are a nice-to-have, never fatal.
  }
}

/** Push (or refresh) one entry; dedupes on account+path, caps at 50. */
export function pushRecent(userId: string, entry: Omit<CloudRecentEntry, "at">): void {
  const list = readRecents(userId).filter(
    (e) => !(e.accountId === entry.accountId && e.path === entry.path),
  );
  list.unshift({ ...entry, at: Date.now() });
  writeRecents(userId, list);
}

/** Drop one entry (e.g. the file no longer exists). */
export function removeRecent(userId: string, accountId: string, path: string): void {
  writeRecents(
    userId,
    readRecents(userId).filter((e) => !(e.accountId === accountId && e.path === path)),
  );
}

export function clearRecents(userId: string): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.removeItem(storageKey(userId));
  } catch {
    /* ignore */
  }
}
