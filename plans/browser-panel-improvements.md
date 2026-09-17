# Browser Panel Improvements — Phased Plan

Date: 2026-09-15 · Repo: goclaw-mod · Feature: client-side Browser Panel (PR #79, 3 parts shipped)

Principle kept throughout: **server stays light** — one sanitized HTML GET per navigation; the user's browser loads every subresource and does rendering/extraction (`internal/browse/store.go:1-7`).

## Current state (verified)

Flow: `web_browse` open → `fetchRawHTML` (`internal/tools/web_fetch.go:387-416`, SSRF + domain policy + redirect re-checks via `newWebFetchClient` `web_fetch.go:285-313`) → `browse.Sanitize` (drops scripts/frames/base, strips `on*` + `javascript:` URLs, absolutizes asset URLs; `internal/browse/sanitize.go:15-141`) → `browse.Store.Put` (in-memory LRU, 20 entries / 5-min TTL / 2MB cap; `internal/browse/store.go:26-34,63-80`) → signed `?ft=` HMAC token bound to path+expiry (`internal/tools/web_browse.go:294-300`, `internal/http/file_token.go:40-66`) → `browser.panel.invoke` to newest WS client for tenant+user (`internal/gateway/browser_invoke.go:71-84,92-144`) → client loads `/v1/browse/{id}?ft=` in `sandbox="allow-same-origin"` iframe (`ui/web/src/components/chat/browser-panel.tsx:119-128`; relay CSP `script-src 'none'` at `internal/http/browse_relay.go:77`) → `extractPageWithRefs` stamps `data-gcref` + `[eN]` markers (`ui/web/src/lib/dom-extract.ts:61-67,114-131`) → `browser.panel.result` (`ui/web/src/pages/chat/hooks/use-browser-panel.ts:95-103`) → waiter with sender check (`browser_invoke.go:151-176`). Actions click/type/extract/back/reload run client-side (`use-browser-panel.ts:171-304`); URL-bar/link clicks go back through `browser.panel.open` → `OpenRelay` (`internal/gateway/methods/browser_panel.go:95-131`, `web_browse.go:262-301`). Fallback when no web client: server-side extraction, no second fetch (`web_browse.go:174-178`).

Panel lives in the single tabbed side pane (`ui/web/src/components/chat/chat-side-pane.tsx:105-116`), width clamped 320–720 (`ui/web/src/stores/use-ui-store.ts:12,81`), resizable via pointer-capture handle (`ui/web/src/components/shared/resize-handle.tsx:24-39`).

Known pain points confirmed in code: (a) fetch failures surface as one opaque error string, (b) SPA pages → thin-content error only (`use-browser-panel.ts:194-203,379-385`), (c) pane-drag glitch (reproduce first, see 2.8), (d) non-UTF-8 pages garble (1.2), (e) form fields get no refs at all (2.1 — `FORM` is in `SKIP_TAGS`, `dom-extract.ts:18`, and `walk()` returns before its children at `dom-extract.ts:134`).

## Live QA evidence (2026-09-15, post-deploy v4.5.0-integration)

Tested against http://192.168.1.103:18790/chat with https://neo.doralove.io.vn/ and https://ai.1k.io.vn/dashboard.

**Bug A — SPA pages render blank in live mode: root cause PROVEN (mechanism + visual).**
- Chain: static relay of a JS-rendered site is an empty shell → extraction < `MIN_USEFUL_CHARS` (80) → panel auto-switches to live (`use-browser-panel.ts:446-448`). Measured static text: ai.1k ≈51 chars; neo = Vite shell (bundle `/assets/index-DX7T-XJc.js` has 9 `localStorage` uses, several at boot: WS `connect` locale read + tenant bootstrap).
- Live iframe uses `sandbox="allow-scripts allow-forms"` — NO `allow-same-origin` (`browser-panel.tsx:46`) → opaque origin → **any `localStorage` access throws `SecurityError`** → SPA boot crash → white/stuck-splash frame. Neither site sends X-Frame-Options/CSP frame-ancestors (framing itself is allowed).
- Proof harness (localhost parent + sandboxed iframes): sandbox A (current attrs) → `SecurityError: The document is sandboxed and lacks the 'allow-same-origin' flag`; sandbox B (`+allow-same-origin`) → storage OK. Real-site visual: neo in A = stuck at logo splash; in B = full sign-in UI renders.
- ai.1k.io.vn/dashboard 307-redirects to `/login` server-side; panel followed it (title "9Router - AI Infrastructure Management"), same crash class applies post-login.
- **Fix (promote to Phase 1, item 1.9):** live mode gains `allow-same-origin` in the sandbox attr, guarded by `new URL(finalUrl).origin !== window.location.origin` (the `allow-scripts + allow-same-origin` combo is only dangerous when the framed doc shares the dashboard origin — for external origins the frame keeps the SITE's origin, not ours). Verified working by the harness above.

**Bug B — drag-to-expand breaks layout: root cause CONFIRMED (measurements + visual).**
- Dragging the pane separator to expand: pane 320→700px, but the chat column crushed to **288px** (below the intended `MIN_CHAT_COLUMN_PX` 360 floor) and the sidebar squeezed to 220px; empty-state text wraps into fragments (vision-confirmed). No horizontal scroll appears (flex redistributes).
- Root cause is candidate (b) from 2.8, now confirmed: `resizeChatPane`'s `dynamicMax` uses the STORED sidebar width (`chat-page.tsx:209-213`), but both the sidebar and chat columns are flex-shrinkable, so the rendered layout violates the cap. The handle's iframe shield (candidate a) is already implemented and works — `dragging && "pointer-events-none"` on the pane body (`chat-side-pane.tsx:71-77`); pointer capture held throughout; no stuck-drag after release. Double-click reset works (700→384).
- Fix: enforce a real min-width on the chat column (e.g. `min-w-[360px]`/flex-shrink guard) and clamp the pane against RENDERED widths, or make the sidebar `shrink-0` so `window.innerWidth - sidebar - MIN_CHAT_COLUMN` matches reality.

**Bug C (new, UX) — manual static↔live toggle silently overridden.**
- User clicks "Chế độ tĩnh" while a JS-rendered page is open → `toggleMode` re-opens through the relay → relay is still thin → `handleIframeLoad` auto-flips BACK to live (`use-browser-panel.ts:446-448`). Observed live: button toggles, iframe never leaves live mode.
- Fix: track `modeSource: "auto" | "user"`; the thin→live auto-switch (and its banner) only fires when the current mode was chosen automatically; a user-pinned static choice stays static with an honest "this page needs JavaScript" note (pairs with 1.4's thin-content copy).

Server logs during all navigations: clean (no `security.*`, no relay errors) — the failures are purely client-side rendering.

---

## Phase 1 — Quick wins: fetch reliability + honest errors

**1.1 Fetch header hardening (S)** — `internal/tools/web_fetch.go`
UA is a stale Chrome/120 macOS string (`web_fetch.go:26`); no `Accept-Language`. Add `Accept-Language: en,*;q=0.5` and refresh the UA to a current Chrome version (one constant, shared by both fetch paths `web_fetch.go:323,392`). Gzip is already transparent (Go Transport default; no manual `Accept-Encoding` is set, so br/zstd are not requested). Redirect chain already re-checks SSRF + domain policy per hop (`web_fetch.go:295-311`) — no change.

**1.2 Charset handling (M)** — `internal/tools/web_fetch.go`, `internal/tools/web_browse.go`
Two-layer bug: (1) `fetchRawHTML` reads raw bytes into a string (`web_fetch.go:404-410`) and `html.Parse` treats them as UTF-8 — a windows-1252/GBK page garbles at parse time; (2) `storeRelay` keeps the origin `Content-Type` (may say `charset=gbk`) while `Sanitize` re-renders UTF-8 (`web_browse.go:280-290`, `sanitize.go:69-73`) — mojibake twice over. Fix: decode with `golang.org/x/net/html/charset.NewReaderLabel` using the response Content-Type + `<meta charset>` sniff before parsing, and force `charset=utf-8` on the relayed Content-Type. Add a windows-1252 fixture test (pattern: `internal/browse/sanitize_test.go`).

**1.3 Status-code awareness (S)** — `internal/tools/web_fetch.go`, `internal/tools/web_browse.go`
`fetchRawHTML` never inspects `statusCode` (`web_fetch.go:409-415`): a 403 bot-wall or 404 page is relayed as if fine. Return a typed error for status ≥ 400 (include status + short body sniff) so both the agent result and the panel error state can say "blocked (403)" vs "not found" vs "fetch failed".

**1.4 Error taxonomy + retry in the panel UI (M)** — `use-browser-panel.ts`, `browser-panel.tsx`, i18n ×3
Today every failure collapses into `status:"error"` + a raw message note (`use-browser-panel.ts:162-165,254-256`) rendered as generic text (`browser-panel.tsx:130-134,144-154`). Introduce `errorCode` on `BrowserPanelState`: `fetch_failed | http_error | blocked_policy | thin_content | relay_expired | no_page`, map each to i18n keys (add to `ui/web/src/i18n/locales/{en,vi,zh}/chat.json` — key list per AGENTS.md i18n rule), and add a **Retry** button in the error state that calls `openURL(state.url)`. Thin-content gets its own copy: "This page needs JavaScript — open live in a new tab or ask the agent to use web_search".

**1.5 Relay-expired remount (S/M)** — `use-browser-panel.ts`
Switching tabs unmounts `BrowserPanel` (only the active tab mounts, `chat-side-pane.tsx:102-116`); the hook in `chat-page.tsx:184` keeps state, so switching back remounts the iframe at the same `relayUrl` — but past the 5-min TTL the relay 404s (`browse_relay.go:66`) and the iframe renders the 404 body text. Detect it in `handleIframeLoad` (body text "404 page not found" or pre-flight `fetch(relayUrl, {method:"HEAD"})`) → set `relay_expired` → auto re-open `state.url` via `browser.panel.open`.

**1.6 Truncation honesty (S)** — `use-browser-panel.ts`, `internal/tools/web_browse.go`
Open-flow truncation is silent client-side: `markdown.slice(0, MAX_EXTRACT_CHARS)` posts no `truncated` flag (`use-browser-panel.ts:388`; action-flow does set it, `:186-192`), and the server ignores the flag anyway (`web_browse.go:218-226` reads only content/title/finalUrl/note). Post the flag, echo it into `formatBrowseResult` as `[content truncated at N chars — pass maxChars to read more]` (marker already exists for maxChars cuts, `web_browse.go:366-368`).

**1.7 Mobile input zoom (S)** — `browser-panel.tsx`
URL bar uses `text-xs md:text-xs` (`browser-panel.tsx:104`) — under 16px triggers iOS auto-zoom (AGENTS.md mobile rule). Change to `text-base md:text-xs`.

**1.8 maxChars guidance (S)** — `internal/tools/web_browse.go`
`web_fetch` shrinks maxChars adaptively as the agent loop iterates (`web_fetch.go:222-229`); `web_browse` does not (`web_browse.go:373-378` always returns the fixed default 60000). Port the same adaptive shrink + mention default/cost in the tool description (`web_browse.go:76-85`).

**1.9 Live-mode sandbox fix — allow-same-origin for cross-origin URLs (S, PROVEN)** — `browser-panel.tsx:46`
Root cause of Bug A (see QA evidence). Change `const sandbox = live ? "allow-scripts allow-forms allow-same-origin" : "allow-same-origin"` with a guard: when `new URL(finalUrl).origin === window.location.origin` (or URL unparseable), fall back to the current no-same-origin attr — never hand `allow-scripts + allow-same-origin` to a doc that shares our origin. External sites then boot normally (harness-verified: full login UI renders where it previously white-screened). Update the header comment + `liveNote` copy: live mode still can't be read/operated by the agent (cross-origin `contentDocument` stays null) — this changes only whether the page WORKS, not agent reach.

**1.10 Respect the user's mode choice (S)** — `use-browser-panel.ts`
Fix Bug C: add `modeSource: "auto" | "user"` to state; `toggleMode` sets `"user"`, auto-fallback (`handleIframeLoad` thin→live) and auto-relay-on-open only apply when `modeSource === "auto"`. A user-pinned static view shows the thin-content note (1.4 copy) instead of silently flipping.

---

## Phase 2 — Interaction depth + panel UX

**2.1 Refs inside forms (M)** — `ui/web/src/lib/dom-extract.ts`
`FORM` sits in `SKIP_TAGS` (`dom-extract.ts:18-22`) so `walk()` returns before annotating its children (`dom-extract.ts:134`): search boxes and most real inputs get **no `[eN]` ref** — the agent cannot type into them at all. Walk into FORM subtrees in ref-annotation mode (skip text noise as today for INPUT/SELECT/TEXTAREA descriptors, `dom-extract.ts:129-131`). Highest-impact interaction fix.

**2.2 SELECT option descriptors (S)** — `dom-extract.ts`
`describeControl` shows placeholder/aria/name/type only (`dom-extract.ts:84-92`) — the agent cannot know valid values, yet the type action sets `.value` directly (`use-browser-panel.ts:270-288`). Append first ~8 `<option>` values to SELECT descriptors.

**2.3 GET form submission (M)** — `use-browser-panel.ts`, `web_browse.go`
Submit is currently swallowed (`use-browser-panel.ts:367-370`). Add a `submit` action (tool enum at `web_browse.go:98`, param docs) that builds `action?k=v` from successful controls (radio/checkbox checked, select value, typed text) and navigates via `openURL` — `action` is already absolutized by the sanitizer (`sanitize.go:33-34,118-128`). POST forms: honest note "static relay cannot POST — use the browser tool". 

**2.4 Scroll-to-ref + visible agent feedback (S)** — `use-browser-panel.ts`
Before click/type: `el.scrollIntoView({block:"center", behavior:"smooth"})` + a transient outline flash (inject a style/overlay class, remove after ~600ms). Currently nothing scrolls or highlights (`use-browser-panel.ts:259-301`), so the user cannot follow what the agent touched. Status-bar note already exists (`finish(note)`).

**2.5 Same-tab vs new-tab links (S)** — `use-browser-panel.ts`
Both the user-interception handler (`:358-366`) and the agent click path (`:291-298`) ignore `target="_blank"` and force same-tab navigation. Honor `_blank` → `window.open(href, "_blank", "noopener")` (pattern already used by the toolbar button, `browser-panel.tsx:56`).

**2.6 Ref shadowing fix (S)** — `dom-extract.ts` (also a security item)
`extractPageWithRefs` stamps `data-gcref` but never clears stale attributes (`dom-extract.ts:116`): an element stamped in pass 1 that a later pass skips (e.g. it became `aria-hidden`, check at `:110` runs before annotation) keeps its old `eN`, so `refElement`'s `querySelector` first-match (`:70-74`) can resolve a stale element — the agent acts on the wrong node. Strip all `[data-gcref]` attributes at the start of every extraction.

**2.7 URL bar history dropdown (S)** — `browser-panel.tsx`, `use-browser-panel.ts`
The session already keeps `historyRef.entries` (`use-browser-panel.ts:81`); render a small recent-URLs dropdown (custom `createPortal` — remember `pointer-events-auto` per AGENTS.md dialog rule) or a `<datalist>` on the URL input (`browser-panel.tsx:94-106`).

**2.8 Pane-drag glitch: ROOT CAUSE CONFIRMED — chat column crushed (S)** — `chat-page.tsx`, `chat-side-pane.tsx`
Live repro (see QA evidence): expanding the pane to 700px crushed the chat column to 288px (< the 360px `MIN_CHAT_COLUMN_PX` intent) and squeezed the sidebar to 220px. Candidate (a) is disproven — the iframe shield already exists (`chat-side-pane.tsx:71-77`, `dragging && "pointer-events-none"`) and held during repro. Fix: (1) give the chat column a hard min-width (`min-w-[360px]` + prevent flex-shrink below it, or `flex-shrink-0` with width from remaining space), (2) make the sidebar non-shrinkable so `dynamicMax` in `resizeChatPane` (`chat-page.tsx:209-213`) matches rendered reality, and (3) optional polish: while dragging, preview via `flex-basis` with a `user-select-none` on body. Verify: drag to extremes in both directions; chat column never <360px; no layout jump on release; double-click reset still lands on 384.

**2.9 Tab strip overflow at min width (S)** — `chat-side-pane.tsx`
Four tab buttons with truncate labels, no overflow handling (`chat-side-pane.tsx:71-98`) — long vi/zh labels at 320px width can clip. `overflow-x-auto` + icon-only below ~400px.

---

## Phase 3 — Bigger bets

**3.1 JS-rendered/SPA pages — options (decision required)**
- **A0. Live preview that actually works — promoted to 1.9 (S, do first):** live mode already exists for SPAs but currently white-screens on any page touching storage (Bug A, proven). 1.9's guarded `allow-same-origin` makes the existing live preview functional for external sites with zero new infra. After 1.9, re-evaluate how loud the remaining demand is.
- **A. Stay static + actionable fallback (S, recommended default):** keep the honest limitation; upgrade the thin-content error (`use-browser-panel.ts:194-203,379-385`) and the fallback message (`web_browse.go:340-342`) into a structured suggestion: "try web_search, an API, or the `browser` tool (server-side headless)". Tool description already warns (`web_browse.go:83-84`).
- **B. Client-side render snapshot — still rejected for same-origin relays:** running RELAY page scripts requires `allow-scripts` on a same-origin doc — exactly what the sanitizer exists to prevent (`sanitize.go:11-14`). A safe variant needs the relay on a separate origin (new port/infra) — L, defer unless demand is loud. (NB: this rejection does NOT apply to live mode framing EXTERNAL origins — that's 1.9 and it's safe.)
- **C. Hand off to the existing Rod browser tool (M):** one-click "Render with server browser" affordance in the thin-content state + agent-side hint. Heavy (headless Chrome on server) but already built (`pkg/browser`, browser.* WS methods) — opt-in per navigation, not automatic, to preserve the server-light principle.

**3.2 Navigation dedupe (M)** — `internal/browse/store.go`, `web_browse.go`, `use-browser-panel.ts`
Every open re-fetches, even the URL currently displayed (`web_browse.go:151`, `OpenRelay` `:262-275`; client `openURL` always RPCs, `use-browser-panel.ts:139-168`). Server: `Store.Put` gains a URL→id map, reuse entries younger than ~60s; client: agent open matching the displayed entry → `extractCurrent` and reply immediately. Cuts duplicate fetches on rapid back/forward + agent re-opens.

**3.3 Relay token/tenant binding (M)** — `internal/browse/store.go`, `internal/http/browse_relay.go`, `file_token.go`
`Entry` carries no tenant (`store.go:38-45`) and the `?ft=` HMAC binds only path+expiry (`file_token.go:40-44`): anyone who learns a relay URL within 5 min can read that sanitized doc (ids are 12 random bytes, `store.go:100-108`, so guessing is out; content is public web data — impact low). Optional hardening: store tenantID on the entry and verify it when a Bearer-authed caller hits the relay; document the residual model in the handler comment.

**3.4 Store headroom (S, optional)** — `store.go`
LRU 20 entries ≈ 40MB worst case (`store.go:33-34`) with lazy TTL expiry on `Get` (`:84-96`) — adequate today; revisit only if multi-tab invoke volume grows (per-tenant cap), do not pre-build.

---

## Security review items (verified, with dispositions)

- **CSP/sandbox pairing — OK.** Relay CSP `script-src 'none'; object-src 'none'; frame-ancestors 'self'` (`browse_relay.go:77`) + iframe `sandbox="allow-same-origin"` without `allow-scripts` (`browser-panel.tsx:124`): page scripts can never run; same-origin is required for `contentDocument` extraction. Two independent gates.
- **Sanitizer coverage — OK.** `on*` attrs and `javascript:`/`vbscript:`/`data:text/html` URLs stripped (`sanitize.go:102-141`); meta refresh and `<base>` dropped (`:24,87-90`); no-referrer injected (`:171-186`); noscript/frames/objects dropped (`:15-25`).
- **Result delivery — OK.** Sender must match clientID+tenant+user of the waiter; mismatches logged `security.browser_result_wrong_sender` (`browser_invoke.go:151-163`, follows the `slog.Warn("security.*")` convention); 256KB result cap (`:31-33,180-185`).
- **Synthetic click/type dispatch — contained.** `MouseEvent` dispatch is untrusted (`isTrusted=false`, `use-browser-panel.ts:300`) and confined to the iframe document; page JS never executes, so no same-origin escalation. Residual risk is acting on the wrong element — addressed by 2.6 (stale refs) and mitigated for the user by 2.4 (visible feedback).
- **Ref shadowing — fix in 2.6** (two elements sharing `data-gcref` across re-extracts, first-match lookup).
- **Token not user-bound — 3.3** (low severity, documented).
- **Subresource requests carry the user's own cookies** for the origin site (assets load client-side, by design `store.go:1-7`): the user is watching their own browser, acceptable; no-referrer prevents URL leakage (`sanitize.go:171`).

## Surface parity

- Gateway/server: all Go items above. API contract: `pkg/protocol/methods.go:329-335`, `events.go:155-159` unchanged except the optional `submit` action (additive tool-param, no wire change). Web UI: main surface, all items. CLI: N/A (no CLI surface for the panel). Desktop (`ui/desktop/frontend`): N/A — no browser-panel code there (grep: zero matches); desktop keeps the server-side fallback path (`web_browse.go:174-178`) and compiles the shared Go packages via `cmd/gateway.go:677-684` wiring.

## Non-goals

- No server-side rendering or subresource proxying — one document per navigation, forever.
- No `allow-scripts` on the same-origin RELAY frame (breaks the security model; option B stays rejected until an isolated relay origin exists). The live-preview frame keeps `allow-scripts` — cross-origin by construction, and 1.9's guarded `allow-same-origin` only ever applies to external origins.
- No server-side cookie jar / session state on fetches — stateless GETs; the user's cookies stay in their browser.
- No full POST-form automation — GET construction (2.3) only; POST flows belong to the Rod browser tool.
- Not defeating bot walls (Cloudflare etc.) — header refresh (1.1) only, no proxy pools.
- No benchmarks/load tests (per AGENTS.md skip rule); unit + integration fixtures only.
