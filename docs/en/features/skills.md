# Skills & Skill Market

Skills are reusable capability packages — research, scraping, document
processing, data analysis, media production and more — each described by a
`SKILL.md` file. Agents find skills with search and invoke them as tools; you
decide which skills are installed and who can use them.

## How skills work

- Each skill is a directory with a `SKILL.md` (YAML frontmatter: name,
  description, version, category, dependencies) plus supporting files.
- Discovery uses **BM25 search** over an in-memory index
  (`internal/skills`); on editions with vector search the score blends with
  embedding similarity.
- The bundled skills directory (`skills/`, ~120 repo-native skills) is
  watched with **fsnotify**: edits hot-reload without a restart.
- Directories are resolved through a **5-tier hierarchy** (system-bundled,
  managed, tenant, agent, user levels), so the same slug can exist at
  different scopes with deterministic precedence.
- Skills are granted per agent or per user — installing globally does not
  automatically expose a skill to every agent.

Agent-side tools: `skill_search` (find), `use_skill` (execute),
`skill_manage` (create/patch/delete from conversation), `publish_skill`
(register a skill directory into the system database). In chat, the
`/gc:<skill>` prefix intercepts a message and routes it as a skill invocation.

## Seeding: what gets installed on day one

`skills.seed_mode` (env `GOCLAW_SKILLS_SEED_MODE`) controls what a **fresh
install** seeds from the bundled directory:

| Mode | Behavior |
|------|----------|
| `all` | Every bundled skill (default, back-compatible) |
| `core` | Only the runtime-critical core: `review`, `issue-to-plan`, `cook`, `fix`, `test`, `scout`, `journal`, `pptx`, `docx`, `pdf`, `xlsx`, `html-video`, `remotion` |
| `none` | Nothing — install what you need from the market |

Switching an existing deployment to `core` or `none` never uninstalls
anything: it only affects fresh installs, and the reconciler keeps installed
skills alive across upgrades.

## Skill market

The market turns the bundled skill kit into a choose-what-you-install
catalog. The catalog is built from the bundled directory itself — every
subdirectory with a `SKILL.md` is a row, annotated with installed state and
update availability. Installs are local directory copies (no network
download) and support kits (e.g. `goclaw-kit`) for batch installs.

- **Web UI** — Skills page, Market tab: browse by category, search,
  install/uninstall/update with live progress.
- **CLI** — `goclaw skills market list` and `goclaw skills market install <slug>`.
- **HTTP** — `GET /v1/skills/market`, `POST /v1/skills/market/install`,
  `POST /v1/skills/market/update/{slug}`,
  `DELETE /v1/skills/market/installed/{slug}`.

::: warning Custom skills are protected
Market uninstall only removes skills with system origin (installed from the
bundled kit). Custom skills you created can never be removed through the
market.
:::

## Dependency lifecycle

Skills can declare package dependencies; GoClaw manages their lifecycle:

- **Scanner** — inspects a skill's files and produces a dependency manifest
  (pip, npm, apk and system packages) with runtime detection.
- **Checker** — reports which dependencies are already satisfied on the host.
- **Installers** — install missing packages per ecosystem.

CLI: `goclaw skills deps scan|check|install <skill>`.

Edition support: installers for pip/npm/apk are available on the Standard
edition. The Lite desktop edition has no pip/npm/apk installers
(`SupportsPipNpm: false`, `SupportsApk: false`) and no vector search — its
skill search is BM25/FTS-only.

## Access control

Access is resolved from an access mode plus explicit grants:

- Per-skill **access mode** governs the default (e.g. open vs grant-only).
- **Per-agent and per-user grants** override the default, versioned so
  upgrades can re-grant.
- **Effective access** is the merged result for a given agent + user pair.

CLI:

```bash
goclaw skills access get <skill>       # show mode and grants
goclaw skills access set <skill> ...   # set access mode
goclaw skills access effective <skill> # inspect effective access
goclaw skills grant agent <skill> <agent-id>
goclaw skills revoke user <skill> <user-id>
```

## HTTP API

| Endpoint | Purpose |
|----------|---------|
| `GET/POST /v1/skills`, `GET/PUT/DELETE /v1/skills/{id}` | CRUD |
| `POST /v1/skills/upload` | Upload a skill archive |
| `GET/POST /v1/skills/export`, `POST /v1/skills/import` | Import/export |
| `/v1/skills/{id}/grants/agents`, `/v1/skills/{id}/grants/users` | Grant management |
| `POST /v1/skills/{id}/toggle` | Enable/disable |
| `GET /v1/skills/{id}/versions` | Version history |
| `/v1/skills/{id}/dependencies(/scan|/check|/install)`, `/v1/skills/install-deps`, `/v1/skills/rescan-deps` | Dependency lifecycle |
| `GET /v1/skills/runtimes` | Detected runtimes |
| `GET/PUT /v1/skills/{id}/tenant-config` | Per-tenant configuration |

See [HTTP API](../api/http) for authentication and general request shape.

## Per-skill evolution

Skills participate in self-evolution: usage is measured per skill and the
system can suggest improvements.

```bash
goclaw skills evolve status <skill>     # show evolution settings
goclaw skills evolve enable <skill>     # enable evolution
goclaw skills evolve disable <skill>    # disable evolution
goclaw skills evolve mode <skill> suggest_only|auto_analyze
goclaw skills metrics <skill>           # usage metrics (calls, success rate)
goclaw skills activity <skill>          # recent evolution activity
goclaw skills suggestions list <skill>  # list improvement suggestions
goclaw skills suggestions approve <skill> <suggestion-id>
goclaw skills suggestions reject <skill> <suggestion-id>
goclaw skills suggestions apply <skill> <suggestion-id>
```

HTTP equivalents live under `/v1/skills/{id}/evolution`,
`/v1/skills/{id}/metrics` and `/v1/skills/{id}/activity`. Agent-level
evolution (metrics, suggestions, self-rewrite of SOUL.md) is covered in
[Orchestration](./orchestration#self-evolution).

## Web UI

The **Skills** page combines the library (installed skills, grants, versions,
files) with the **Market** tab (catalog, install/update/uninstall). Studio
designer agents such as the PPTX designer consume bundled design skills —
see [Creative Studio](./tools-studio).
