/* GoClaw service worker — app shell caching for home-screen (PWA) installs.
 *
 * Strategy:
 *  - /assets/* (Vite content-hashed) → cache-first, immutable.
 *  - Navigation requests → network-first; offline falls back to the cached
 *    app shell so the installed app opens without connectivity.
 *  - Other same-origin GETs (icons, manifest, locale JSON) →
 *    stale-while-revalidate.
 *  - API traffic (/v1/, /ws, /health, /api/) is never touched.
 *
 * No Workbox: keep the dependency surface at zero. Bump SHELL_CACHE to
 * invalidate the previous cache on deploy.
 */
const SHELL_CACHE = "goclaw-shell-v1";
const API_PREFIXES = ["/v1/", "/ws", "/health", "/api/"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(SHELL_CACHE);
      await cache.addAll(["/", "/manifest.webmanifest"]).catch(() => {});
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(keys.filter((k) => k !== SHELL_CACHE).map((k) => caches.delete(k)));
      await self.clients.claim();
    })(),
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;

  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (API_PREFIXES.some((p) => url.pathname.startsWith(p))) return;
  if (req.mode === "websocket") return;

  if (url.pathname.startsWith("/assets/")) {
    // Hashed filenames — cache-first is safe.
    event.respondWith(cacheFirst(req));
    return;
  }

  if (req.mode === "navigate") {
    event.respondWith(networkFirstNavigation(req));
    return;
  }

  event.respondWith(staleWhileRevalidate(req));
});

async function cacheFirst(req) {
  const cached = await caches.match(req);
  if (cached) return cached;
  const res = await fetch(req);
  if (res.ok) {
    const cache = await caches.open(SHELL_CACHE);
    cache.put(req, res.clone());
  }
  return res;
}

async function networkFirstNavigation(req) {
  try {
    const res = await fetch(req);
    if (res.ok) {
      const cache = await caches.open(SHELL_CACHE);
      cache.put(req, res.clone());
    }
    return res;
  } catch {
    const cached = (await caches.match(req)) || (await caches.match("/"));
    if (cached) return cached;
    return new Response("Offline", { status: 503, headers: { "Content-Type": "text/plain" } });
  }
}

async function staleWhileRevalidate(req) {
  const cached = await caches.match(req);
  const refresh = fetch(req)
    .then((res) => {
      if (res.ok) {
        caches.open(SHELL_CACHE).then((cache) => cache.put(req, res.clone()));
      }
      return res;
    })
    .catch(() => undefined);
  return cached || (await refresh) || new Response("Offline", { status: 503 });
}
