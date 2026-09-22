# goclaw-docs

Public documentation site for **[GoClaw](https://github.com/qkhalk/goclaw)** —
the multi-tenant AI agent platform. Built with
[VitePress](https://vitepress.dev) (pinned to `1.6.4`), deployed to GitHub
Pages on every push to `main`.

- **URL:** `https://<user>.github.io/goclaw-docs/` (after enabling Pages, see below)
- **Locales:** English at `/en/` (default — `/` redirects there), Vietnamese
  at `/vi/` (homepage, installation and skills fully translated; other pages
  fall back to the English page)
- **Search:** built-in local search (minisearch) — no external service

## Layout

```
en/                      English pages (source of truth)
vi/                      Vietnamese pages (subset; untranslated pages link to en/)
public/                  Static assets (logo.svg)
.vitepress/config.ts     Locales, base path, nav/sidebar per locale, local search
.github/workflows/docs.yml   Build + deploy workflow
index.md                 Redirect stub → ./en/
```

The `base` path defaults to `/goclaw-docs/` and can be overridden with the
`BASE` environment variable (useful for custom-domain deploys or local
preview parity).

## Working on the docs

**Prerequisite:** Node 20+ and [pnpm](https://pnpm.io) (version pinned via
`packageManager` in `package.json`).

```bash
pnpm install
pnpm docs:dev        # dev server with hot reload
pnpm docs:build      # production build → .vitepress/dist
pnpm docs:preview    # serve the production build locally
```

CI installs with `pnpm install --frozen-lockfile` — commit
`pnpm-lock.yaml` after any dependency change, and keep `vitepress` at an
exact version (no `^`/`~`) to avoid build drift.

### Deploying (one-time setup)

1. Push this repo to GitHub (default expectation: `qkhalk/goclaw-docs`).
2. Repo **Settings → Pages → Source: GitHub Actions**.
3. Push to `main` (or run the workflow manually) — the `Deploy Docs` workflow
   builds and publishes. Subsequent pushes to `.vitepress/**`, markdown,
   `package.json`, `pnpm-lock.yaml` or the workflow file redeploy
   automatically.

## Sync policy with the goclaw repo

This repo is **intentionally separate** from the GoClaw source repo. Content
is synced by **manual pull requests**, not automation:

- Whenever a GoClaw change affects user-facing behavior (new endpoint, new
  config key, changed command, new feature surface), open a PR here in the
  same workstream. Link the goclaw PR/issue in the docs PR description.
- The **goclaw PR template** carries a docs checklist line —
  `[ ] Docs updated? (open a goclaw-docs PR if user-facing)` — so the docs
  sync is reviewed as part of the feature work. *(Integrator note: that one
  checkbox line still needs to be added to
  `.github/PULL_REQUEST_TEMPLATE.md` in the goclaw repo.)*
- Which page to touch for what:

  | Change in goclaw | Page here |
  |------------------|-----------|
  | Install/update flow, Docker variants | `en/getting-started/install.md` (+ `vi/` mirror) |
  | New config key / env var | `en/getting-started/configuration.md` |
  | Agent, subagent, delegation behavior | `en/features/agents.md` |
  | Skills / skill market | `en/features/skills.md` (+ `vi/` mirror) |
  | Studio tools (pptx/video/watermark) | `en/features/tools-studio.md` |
  | Telegram commands & buttons | `en/channels/telegram.md` |
  | New/changed `/v1` endpoint | `en/api/http.md` |
  | Deployment/ops learnings | `en/self-hosting.md`, `en/troubleshooting.md` |

- If a section drifts badly, the source of truth is always the goclaw repo
  (`README.md`, `AGENTS.md` for architecture, `deploy/README-video-worker.md`
  for the video worker). Rewrite from source rather than patching guesses.

## License

Content follows the GoClaw project license (CC BY-NC 4.0).
