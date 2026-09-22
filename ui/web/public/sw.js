/*
 * GoClaw dashboard service worker — a deliberately small offline shell.
 *
 * Strategy:
 *  - SPA navigations: network-first, fall back to the cached shell so the app
 *    still opens (to its login/error state) when the gateway is unreachable.
 *  - Vite-hashed bundles + public icons/fonts: cache-first (content-addressed
 *    filenames make this safe; a new deploy ships new names).
 *  - Everything else (API /v1/*, /ws, /api/*, cross-origin) passes through
 *    untouched — live/authenticated traffic must never be served from cache.
 */
const VERSION = "v1";
const SHELL_CACHE = `goclaw-shell-${VERSION}`;
const ASSET_CACHE = `goclaw-assets-${VERSION}`;

const PRECACHE = [
  "/",
  "/manifest.json",
  "/favicon.svg",
  "/icons/icon-192.png",
  "/icons/icon-512.png",
  "/icons/icon-maskable-512.png",
  "/icons/apple-touch-icon.png",
];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(SHELL_CACHE)
      .then((cache) => cache.addAll(PRECACHE))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(
          keys
            .filter((k) => k !== SHELL_CACHE && k !== ASSET_CACHE)
            .map((k) => caches.delete(k)),
        ),
      )
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;

  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  // Never touch API/WebSocket traffic.
  if (
    url.pathname.startsWith("/v1/") ||
    url.pathname.startsWith("/ws") ||
    url.pathname.startsWith("/api/")
  ) {
    return;
  }

  // SPA navigations: network first, cached shell when offline.
  if (req.mode === "navigate") {
    event.respondWith(
      fetch(req)
        .then((res) => {
          if (res && res.ok && res.type === "basic") {
            const copy = res.clone();
            caches
              .open(SHELL_CACHE)
              .then((c) => c.put("/", copy))
              .catch(() => {});
          }
          return res;
        })
        .catch(() =>
          caches
            .match("/", { ignoreSearch: true })
            .then((r) => r ?? Response.error()),
        ),
    );
    return;
  }

  const cacheFirst =
    url.pathname.startsWith("/assets/") ||
    url.pathname.startsWith("/icons/") ||
    url.pathname.startsWith("/fonts/") ||
    url.pathname === "/favicon.svg" ||
    url.pathname === "/manifest.json";
  if (cacheFirst) {
    event.respondWith(
      caches.match(req).then(
        (hit) =>
          hit ??
          fetch(req).then((res) => {
            if (res && res.ok && res.type === "basic") {
              const copy = res.clone();
              caches
                .open(ASSET_CACHE)
                .then((c) => c.put(req, copy))
                .catch(() => {});
            }
            return res;
          }),
      ),
    );
  }
  // Everything else: plain network, untouched.
});
