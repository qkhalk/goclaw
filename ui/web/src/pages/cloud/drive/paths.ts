/** Remote path helpers for the Drive shell.
 *
 * Convention (matches the old FilesBrowser): the `?path=` URL param stores the
 * remote path with each segment encodeURIComponent-ed and joined with "/".
 * Root is "" (absent param) or "/".
 */

export type SortKey = "name" | "size" | "modified";
export type SortDir = "asc" | "desc";

export interface SortSpec {
  key: SortKey;
  dir: SortDir;
}

export type ViewMode = "grid" | "list";

export const VIEW_MODE_STORAGE_KEY = "cloud.viewMode";
export const SORT_STORAGE_KEY = "cloud.sort";
/** Rail provider groups the user collapsed (JSON array of provider ids). */
export const RAIL_COLLAPSED_STORAGE_KEY = "cloud.railCollapsed";

/** Normalize a raw ?path= param to a canonical encoded path ("/" for root). */
export function normalizePath(raw: string | null | undefined): string {
  const value = (raw ?? "/").trim();
  if (!value || value === "/") return "/";
  return value.startsWith("/") ? value : `/${value}`;
}

/** Display name of the last segment (decoded). */
export function pathDisplayName(path: string): string {
  const segs = path.split("/").filter(Boolean);
  const last = segs[segs.length - 1];
  return last ? safeDecode(last) : "";
}

/** Breadcrumbs for a path: root first, then each decoded segment with its
 * accumulated encoded path. */
export function pathCrumbs(path: string): { name: string; path: string }[] {
  const segs = path.split("/").filter(Boolean);
  return segs.map((seg, i) => ({
    name: safeDecode(seg),
    path: "/" + segs.slice(0, i + 1).join("/"),
  }));
}

/** Full path of a child entry (encodes the new segment). */
export function childPath(parent: string, name: string): string {
  const encoded = encodeURIComponent(name);
  return parent === "/" ? `/${encoded}` : `${parent}/${encoded}`;
}

/** Parent directory path ("/" for root-level entries). */
export function parentPath(path: string): string {
  const idx = path.lastIndexOf("/");
  if (idx <= 0) return "/";
  return path.slice(0, idx);
}

function safeDecode(seg: string): string {
  try {
    return decodeURIComponent(seg);
  } catch {
    return seg;
  }
}
