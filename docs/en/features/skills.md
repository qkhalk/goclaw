# Skills & Skill Market

Skills are reusable capability packages — research, scraping, OCR,
data-analysis, media-processing, copywriting, databases and more — each
described by a `SKILL.md` file with frontmatter metadata. Agents find and use
them through hybrid search; you decide which ones are installed.

## How skills work

- Each skill is a directory with a `SKILL.md` (name, description, version,
  category in its frontmatter) plus supporting files.
- Agents discover skills with `skill_search` — **BM25 + semantic hybrid
  search** — and execute them with `use_skill`.
- Skills are granted per agent: install globally, grant selectively.
- 110+ repo-native skills ship with the project, bundled into the binary.

## Seeding: what gets installed on day one

The `skills.seed_mode` config key (env: `GOCLAW_SKILLS_SEED_MODE`) controls
what a **fresh install** seeds:

| Mode | Behavior |
|------|----------|
| `all` | The full skill bundle — legacy default, fully back-compatible |
| `core` | ~10 runtime-critical skills only (review, plan, search, studio designer deps) |
| `none` | Nothing — install what you need from the market |

Switching an existing server to `core` never **uninstalls** skills that are
already seeded — the reconciler keeps installed skills alive across upgrades;
it only affects fresh installs. Uninstall what you don't want via the market.

## Skill Market

The market turns the bundled skill kit into a **choose-what-you-install**
catalog, available in the web UI (**Skills → Market** tab) and over HTTP.

- **Browse by category** — cards with icon, name, description and version;
  client-side search and category chips across the whole catalog.
- **Install / Uninstall / Update** — installs run as background jobs with a
  progress bar and live log (multi-select installs queue one batch job).
  Installs copy from the **bundled directory on disk — no network download**.
- **"Installed vX" badges and "update available"** markers keep the state
  obvious; after installing you can jump straight into granting the skill to
  agents.
- **Kit card** — "install the entire bundle" remains one click for anyone who
  wants the old behavior.
- **Admin-only** — installing and uninstalling require the admin role;
  viewers can browse.

::: warning Custom skills are protected
Uninstall via the market only removes skills it installed (system origin).
Your own custom skills can never be removed through the market.
:::

### Skill Market API

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/skills/market` | Catalog listing (name, description, category, version, `installed`, grants) |
| `POST /v1/skills/market/install` | `{ slugs: [...], grantAgentIds? }` — background install job |
| `POST /v1/skills/market/update/{slug}` | Update one installed skill |
| `DELETE /v1/skills/market/installed/{slug}` | Uninstall (managed skills only) |

The CLI mirrors this with `goclaw skills market list|install`.

See [HTTP API](/en/api/http) for authentication and general request shape.

## Bundled skills for the studio

Two skills ship bundled specifically for the creative tools:
`pptx-deck-design` and `pptx-visual-style` power the design-only
`pptx-designer` agent behind [PPTX Studio](/en/features/tools-studio).
