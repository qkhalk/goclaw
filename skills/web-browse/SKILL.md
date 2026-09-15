---
name: web-browse
description: "Use whenever the task involves the web_browse tool: opening a URL in the user's browser panel and then operating that page — clicking links and buttons, filling inputs, going back or reloading — through the [eN] element refs in the returned content. Covers the open, read refs, act loop, ref freshness, when to choose web_browse over web_fetch, the static no-JS relay limitation, and the web-channel panel requirement. Also use when a web_browse call failed and the error mentions refs, the browser panel, or thin content."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - url
  - user_task
outputs:
  - page_journey
  - extracted_answer
allowed-tools:
  - web_browse
  - web_fetch
  - web_search
quality-gates:
  - result_contains_refs_before_click
  - stale_ref_not_reused
  - panel_context_confirmed
  - js_only_site_escalated
---

# Web Browse (drive pages in the user's browser panel)

`web_browse` opens a URL in the user's browser panel (web dashboard) and lets
you operate the page on their behalf while they watch it render live. The
server fetches ONE sanitized HTML document per navigation; the user's browser
loads the heavy assets (images, CSS, fonts) directly from the origin site and
extracts the page text client-side. Every interactive element in the returned
content carries an `[eN]` ref you can act on.

## Mental model

- Two modes per call: **open** (pass `url`, no `action`) or **operate** (pass
  `action` targeting the page currently shown in the panel). Never both.
- Pages are **static**: the sanitizer drops `<script>`, `<noscript>`, frames,
  objects, and every `on*` handler before the document is relayed — scripts
  never run, neither in the panel nor in the text you get back.
- Result shape:
  ```
  URL: https://example.com/docs
  Title: Example Docs
  Source: user-browser (page rendered in the user's browser panel)
  Interactive elements are tagged [eN] — act on them with {"action":"click"|"type", "ref":"eN"}. Clicking a link navigates; the user sees every step.

  ...page text with [e12] link [e5] input...
  ```
  `Source: server-fetch` means the panel was not used (fallback, see
  Channel and panel requirements). The refs hint line only appears on the
  open result — the `[eN]` tags themselves always live in the content body.

## Choose the right tool

| Situation | Tool |
|-----------|------|
| User is on the web dashboard and wants to watch the browsing, or the task needs clicking through a site | `web_browse` |
| Just read a known URL fast; nobody needs to watch | `web_fetch` |
| JSON endpoints and raw files | `web_fetch` (parses JSON; the browse relay is for HTML pages) |
| Non-web channels (Telegram, other connectors) | `web_fetch` — panel actions do not exist there |
| Discovery first | `web_search`, then open/fetch the promising hits |
| Full JS execution or heavy automation is required | server-side browser tool (headless Chrome), not web_browse — the relay never executes scripts |

## The loop: open → read refs → act

1. **Open.** `{"url":"https://example.com"}` — optional `maxChars`
   (default 60000, minimum 100) and `timeoutMs` (default 45000, range
   5000–120000).
2. **Read the result.** Locate the elements you need among the `[eN]` tags in
   the LAST result.
3. **Act** on the displayed page:
   - Click: `{"action":"click","ref":"e12"}`
   - Fill an input: `{"action":"type","ref":"e5","text":"goclaw gateway"}`
   - Refresh content and refs without navigating: `{"action":"extract"}`
     (accepts `maxChars`)
   - History: `{"action":"back"}` and `{"action":"reload"}`
4. Every action returns the fresh page content in the same shape as open —
   loop back to step 2 until the task is done.

Each action waits up to `timeoutMs` (default 45 s) for the user's browser to
extract. Page too slow? Raise `timeoutMs` once (max 120000) — do not re-fire
the same action as a retry.

## Ref rules (the #1 source of failures)

- Refs come from the **last result only** and change after every navigation
  (open, click, back, reload). Never carry a ref across two navigations.
- A click that navigates invalidates ALL previous refs — including ones you
  had not used yet.
- Lost track of the page state (long reasoning between calls, an error, a
  pause)? Re-extract: `{"action":"extract"}` — no navigation, cheap, returns
  fresh refs. When in doubt, extract before you click.

## Navigating: click vs open

- **Click the link** when the user wants to see the journey — each click
  renders in their panel, which is the whole point of web_browse.
- **Open by URL** when speed matters and the destination is already known —
  it skips the intermediate pages.

## Typing and forms

- `type` fills a field; it does not submit anything. The relayed page never
  executes scripts, so form submit buttons do not POST in the static relay.
- Prefer the site's own search URL pattern over filling search boxes: open
  `https://example.com/search?q=goclaw` directly instead of typing into the
  box and then hunting for a submit button that cannot work.
- `type` is still the right call when the filled value itself is the point
  (showing the user a completed form) — just do not expect a submit to fire.

## Thin or JS-rendered pages

A fallback open can return:

```
[No content extracted. The page may require JavaScript to render or returned a
bot-protection challenge — the relayed page never executes scripts. Try
web_search or an API instead.]
```

Client-extracted content can also come back suspiciously thin — same cause:
the page builds itself with JavaScript, and the relay strips scripts before
the browser ever sees the document. When this happens:

- Do NOT retry the same URL or hammer reload — the content will never appear.
- Tell the user plainly: this site needs JavaScript / blocks automated reads.
- Switch strategy: `web_search` for the information, `web_fetch` on an API or
  alternate/static page, or the site's prerendered URL if it has one.

## Channel and panel requirements

- **Actions** (`click`/`type`/`extract`/`back`/`reload`) work only on the
  **web channel with the user's browser panel open**. Anywhere else:
  `action "click" requires the user's browser panel (web channel, panel open)`.
  From Telegram or other channel contexts, do not attempt actions at all.
- **Open works everywhere** and never hangs: with no web client connected it
  falls back to plain server-side extraction (`Source: server-fetch`) — the
  same pipeline as web_fetch, but there are no live refs to act on. Treat a
  server-fetch result as read-only.
- `browser panel action failed: ... (the user may have closed the panel)` →
  ask the user to reopen the panel, then re-extract before acting — the old
  refs most likely died with the panel session.

## Anti-patterns

- `{"action":"click","ref":"e12","url":"https://..."}` — passing `url`
  together with `action`. The `url` is silently ignored whenever `action` is
  set; the action runs on whatever page the panel currently shows. Pick one
  mode per call.
- Inventing refs ("e13 should exist") or reusing refs from an earlier page.
  If the ref is not in the LAST result, it does not exist — extract again.
- Retrying the same dead ref repeatedly. One failure → `{"action":"extract"}`
  → use the fresh ref.
- Parsing the hint line as the source of refs — read the `[eN]` tags from the
  content body; the hint is informational and appears only on open.
- Issuing actions from Telegram/channel contexts — the panel does not exist
  there; use `web_fetch` instead.
- Looping reload on a JS-only site hoping content appears — scripts are
  stripped before relay; they will never run.

## Errors quick reference

| Error text (verbatim) | Meaning / fix |
|-----------------------|---------------|
| `url is required (or pass action to operate the currently open page)` | Open needs `url`; operating needs `action`. You passed neither. |
| `action "click" requires ref (an [eN] tag from the last page content)` | click/type need a `ref` taken from the last result. |
| `action "type" requires text` | `type` needs non-blank `text`. |
| `action "click" requires the user's browser panel (web channel, panel open)` | Wrong channel or panel closed — open-only, or ask the user to open the panel. |
| `browser panel action failed: ... (the user may have closed the panel)` | Panel closed or timed out — ask the user, then re-extract. |
| `unknown action "..." (use open/click/type/extract/back/reload, or pass url to open)` | Typo in the action name. |
| `fetch failed: ...` | Same causes as web_fetch: bad host, timeout, SSRF protection, or a domain blocked by tenant policy. |
