---
name: chrome-profile
description: >-
  Browser automation session hygiene with persistent Chrome profiles: when a persistent profile
  helps (logins, cookies), how to isolate automation profiles from the user's real one, and safe
  storage of session data. Use when automation must stay logged in across runs or carry cookies
  between steps. Keywords: chrome profile, browser automation, persistent session, cookies, login
  state, user data dir, trinh duyet, phien dang nhap. Dùng khi automation cần giữ phiên đăng nhập
  giữa các lần chạy.
license: MIT
version: 1
---

# Chrome Profile

Use persistent browser profiles correctly so automation keeps its session state without endangering the user's real browser data.

## When to use
- Automation must remain logged in across separate runs (authenticated dashboards, repeated checks).
- Multi-step browser flows where cookies or local storage carry state between steps.
- Setting up a dedicated automation browser that must not touch personal profiles.

## When NOT to use
- One-off anonymous browsing — a fresh temporary context is simpler and safer.
- The target site's terms forbid automation — stop and tell the user.
- Credentials are better supplied per-run through the app's own API instead of browser login.

## Workflow
1. **Decide persistence:** if login state is needed only within one run, use a temporary profile directory; if across runs, use a dedicated persistent directory.
2. **Isolate:** give automation its own profile directory (for example under the project's `tmp/` or the workspace) — never the user's default browser profile path. One profile per site or purpose; never share a profile across unrelated automation.
3. **Point the tool at it:** configure the browser tool (`web_browse`, or the underlying rod/CDP driver) with an explicit user-data-dir argument so every launch attaches to that profile.
4. **Log in once:** perform the login flow the first time, then restart the browser and re-check the session to confirm persistence works.
5. **Protect session data:** treat the profile directory as a secret — it contains live cookies. Keep it out of version control (add an ignore rule), out of backups and shared artifacts.
6. **Rotate and clean:** when a session goes stale or the automation retires, delete the profile directory; a fresh one gets created on next use. Never reuse a profile across security boundaries.

## Output
A working automation session that persists login state in an isolated profile, with the profile path documented and excluded from version control.

## Routing
- General page automation and reading → `web-browse`.
- Verifying frontend visuals in the automated browser → `preview`.

## Guardrails
- Never launch automation against the user's real or default browser profile.
- Never copy profiles between machines or commit them — cookies are credentials.
- If login requires two-factor authentication or legal attestation, involve the user via `ask`.
