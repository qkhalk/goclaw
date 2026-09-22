import { lazy, type ComponentType } from "react";

/**
 * Wraps React.lazy() with retry logic for failed chunk loads.
 * - Retries the import once on failure (handles transient network errors)
 * - If retry fails, forces a page reload to clear stale chunk URLs (e.g. after a deploy)
 * - The reload guard is PER MODULE URL: several deploys in one browser session
 *   each invalidate different chunks, so a session-wide one-shot guard would
 *   leave later failures stuck on the error screen. The same URL failing twice
 *   (genuinely broken build) still falls through to the ErrorBoundary.
 */
export function lazyWithRetry<T extends ComponentType<unknown>>(
  importFn: () => Promise<{ default: T }>,
) {
  return lazy(async () => {
    try {
      return await importFn();
    } catch (retryError) {
      // Retry once after transient failure
      try {
        return await importFn();
      } catch {
        const guardKey = `chunk-failed-reload:${extractModuleUrl(retryError)}`;
        if (!sessionStorage.getItem(guardKey)) {
          sessionStorage.setItem(guardKey, "1");
          window.location.reload();
        }
        // This module already reloaded once and still fails — let the
        // ErrorBoundary handle it.
        throw retryError;
      }
    }
  });
}

/** Best-effort extraction of the failing module URL from a chunk-load error
 * (Vite/Chrome include it in the message; fallback keeps a constant key). */
function extractModuleUrl(err: unknown): string {
  const msg = err instanceof Error ? err.message : String(err);
  const m = msg.match(/(https?:\/\/[^\s"']+\.(?:js|mjs))/);
  return m?.[1] ?? "unknown";
}
