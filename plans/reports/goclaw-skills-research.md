# GoClaw Skill System Research — Contract, Deployment, and a Proposed Design/Video Skill Set

Date: 2026-09-15 · Repo: `C:\Users\DORA\Downloads\goclaw-mod` · Production: `/opt/goclaw/skills` (root@192.168.1.103) · READ-ONLY research

---

## 1. SKILL.md Contract (exact, with citations)

### 1.1 Discovery & parsing

- A skill = a **directory containing `SKILL.md`** at its root. The **directory name is the slug** (unique identity used for grants, dedup, access filtering). `internal/skills/loader.go:195-211` (Name and Slug both default to `d.Name()`).
- Frontmatter is delimited by `---\n ... \n---` at file start; regex `(?s)^---\n(.*?)\n---\n?`; CRLF normalized first (Windows-safe) — `internal/skills/loader.go:623, 710-728`.
- Parser tries **JSON frontmatter first** (valid JSON with non-empty `name` wins), then falls back to a **simple YAML subset** — `internal/skills/loader.go:625-666`. The YAML subset supports `key: value`, block scalars (`|`, `>`), and flat block lists (`key:\n  - item`). It does NOT support flow lists (`[a, b]`), nested maps (skipped with debug log), or `-item` without a space — `internal/skills/loader.go:730-799`.
- Skills with no frontmatter at all still load (name falls back to directory name) — `internal/skills/loader.go:633-634`.
- A non-empty frontmatter `name` **overrides** the directory-derived display name, but the slug stays the directory name — `internal/skills/loader.go:674-681` (`applyMetadata`).

### 1.2 Typed frontmatter fields (parsed into `skills.Metadata`)

`internal/skills/loader.go:33-43`:

| Field (YAML key) | Type | Notes / citation |
|---|---|---|
| `name` | string | Display name; overrides dir name — loader.go:679-681 |
| `description` | string | The critical field: indexed by BM25, shown in `<available_skills>` XML, **truncated to 200 runes** in prompts — loader.go:422-425, 466-471. Upload validation treats ≤1024 chars as the practical limit (skill-creator guidance) |
| `version` | string | Frontmatter version (display); DB `version` is a separate auto-incrementing int managed by the store — loader.go:37, seeder.go:105-106 |
| `inputs` | []string | Block list (`inputs:\n  - x`) or comma/space-separated scalar — loader.go:653-659 |
| `outputs` | []string | Same dual form — loader.go:660-664 |
| `allowed-tools` | []string | Block list only (JSON form uses `allowedTools`) — loader.go:649 |
| `quality-gates` | []string | Block list only — loader.go:650 |
| `requires` | block | Host gating: `requires:\n  bins:\n    - ffmpeg\n  os:\n    - linux` — `internal/skills/requires.go:31-36, 6-22`. Missing binary (exec.LookPath) or OS mismatch (accepts `macos`/`mac` alias for darwin) → skill stays listed/searchable but flagged `unavailable` and **skipped by auto-injection** — loader.go:59-62, requires.go:48-69, 189-199. Re-evaluated on every scan (hot reload picks up newly installed binaries) |

### 1.3 Fields stored but not typed

- `license` — free text; used by the seeder's bundled-vs-custom recovery heuristic — `internal/skills/seeder.go:273-290`.
- `slug` — read only by the **upload** path (`skills.ParseSkillFrontmatter`, `internal/skills/helpers.go:14-29`); the filesystem loader ignores it (dir name rules).
- `metadata:` nested block (e.g. skill-creator's `metadata:\n  author: GoClaw\n  version: "4.0.0"`) — survives in the raw frontmatter map but nested maps are flattened/skipped by the parser (loader.go:775-781).
- `deps:` block list — dependency manifest with prefixes `pip:<spec>`, `npm:<pkg>`, `system:<bin>` — `internal/skills/dep_manifest.go:47-80`. Checked asynchronously after seeding; missing deps → `status='archived'` + `missing_deps` persisted + WS events (`EventSkillDepsChecked`/`Complete`) — `internal/skills/seeder.go:295-332`.

### 1.4 Body conventions

- Body = everything after frontmatter. `{baseDir}` placeholder is replaced with the skill's absolute directory when loaded — loader.go:349, 362, 374. This is how SKILL.md references its own `scripts/` and `references/` files portably.
- Progressive-disclosure budget (from deployed `skill-creator`, matches loader constants): SKILL.md < 300 lines; each reference < 300 lines; description ≤ 1024 chars. Pinning inlines at most **10 KB per skill / 30 KB total** before falling back to pointer-only entries — loader.go:483-492.

---

## 2. Load Paths & Precedence

### 2.1 Six-tier hierarchy (highest priority first)

`internal/skills/loader.go:4-9, 172-183` and `cmd/gateway_setup.go:602-613`:

| # | Source tag | Path | Citation |
|---|---|---|---|
| 1 | `workspace` | `<workspace>/skills/` | loader.go:69, 96-98 |
| 2 | `agents-project` | `<workspace>/.agents/skills/` | loader.go:70, 98 |
| 3 | `agents-personal` | `~/.agents/skills/` (disable: `GOCLAW_DISABLE_PERSONAL_SKILLS=1`) | loader.go:71, 101-106 |
| 4 | `global` | `GOCLAW_SKILLS_DIR` env, else `<dataDir>/skills/` | gateway_setup.go:604-607 |
| 5 | `managed` | `<dataDir>/[tenants/<slug>/]skills-store/<skill>/<version>/SKILL.md` (latest version dir wins) | loader.go:75-79, 129-142, 261-332 |
| 6 | `builtin` | `GOCLAW_BUILTIN_SKILLS_DIR` env, else `/app/bundled-skills` | gateway_setup.go:608-613 |

First-seen wins by **slug**; higher tiers shadow lower ones entirely (loader.go:169-214, 216-256). Managed (DB-seeded) deliberately outranks builtin so agents get workspace-accessible paths instead of `/app/bundled-skills/` (loader.go:172-174).

### 2.2 Seeding (how skills enter DB + skills-store)

`cmd/gateway_setup.go:631-676`:
- Bundled dir resolved from `GOCLAW_BUNDLED_SKILLS_DIR` env, else first existing of `bundled-skills`, `/app/bundled-skills`, `skills` (repo `skills/` in dev) — gateway_setup.go:632-641.
- `skills.Seeder.Seed()` upserts each dir with SKILL.md as **system skill** (`owner_id=system`, `visibility=public`, `status=active`, versioned `file_path` under skills-store) and `CopyDir`s files into the managed dir — `internal/skills/seeder.go:55-174`. Content-hash idempotency; incomplete managed copies are re-copied (seeder.go:143-157, 450-465).
- `_`-prefixed dirs (e.g. `_shared/`) are **not skills** — copied as shared code only — seeder.go:68-71, 351-369.
- A `Reconciler` registers on-disk skills-store entries missing DB rows (a skill dropped into skills-store without a `skills` row is invisible to agents, because visibility is DB-driven) — gateway_setup.go:660-676.
- Slug conflicts with an existing custom skill are skipped (`ErrSystemSkillSlugConflict`) — seeder.go:134-138.

### 2.3 Production deployment

`/opt/goclaw/skills` on 192.168.1.103 mirrors the repo `skills/` bundle (36 entries = 34 SKILL.md skills + `_shared/` + `go-claw-engineer/kit.yaml`). The repo additionally carries `decision-log`, `fan-out`, `ship`, `web-browse` not yet present on the server. The server dir is this deployment's bundled-skills source (`GOCLAW_BUNDLED_SKILLS_DIR`), seeded into its skills-store at gateway startup.

### 2.4 Kits (skill sets)

`kit.yaml` manifest: `{name, version, description, skills: [slugs], checksum?}` — `internal/skills/kit_manager.go:25-31`. Kits reference slugs that resolve under the skills root; checksums are computed, never hand-authored. Deployed example `go-claw-engineer` kit: `plan, fix, cook, review`.

---

## 3. How Agents Get & Use Skills

### 3.1 Tools

- **`skill_search`** (`internal/tools/skill_search.go`) — BM25 (k1=1.2, b=0.75, standard IDF) over `name + " " + description` tokens (`internal/skills/search.go:44-203`); 1-char tokens dropped; per-tenant lazily rebuilt indexes keyed by loader version (skill_search.go:60-87). Optional **hybrid search** (0.3 BM25 + 0.7 embedding) when an embedding provider is wired (skill_search.go:200-296). Result payload ends with an explicit "ACTION REQUIRED: call use_skill, then read_file <location>" instruction — skill_search.go:157-163.
- **`use_skill`** (`internal/tools/use_skill.go:9-49`) — deliberate **no-op observability marker**: logs `skill.activated` for tracing; the model then reads SKILL.md itself with `read_file`.
- **`skill_manage` / `publish_skill`** — agent self-service skill creation/publishing into skills-store. Registered **only when `edition.Current().TeamFullMode`** (true on Standard/PG, false on Lite/desktop) — `cmd/gateway_setup.go:680-692`.

### 3.2 Prompt injection (two modes)

`internal/agent/loop_history_skills.go:10-52`:
- **Inline mode**: ≤ 60 skills and ≤ ~3000 estimated description tokens → full `<available_skills>` XML (name, description, location) in the system prompt; model scans descriptions directly.
- **Search mode**: above thresholds → no XML, model uses `skill_search` (systemprompt.go:888-898 even nudges "prefer skill_search over browser/web_search when the domain might have a skill").
- **Pinned skills** (`agents.other_config.pinned_skills`, `internal/store/agent_store.go:316-326`) always inline the **full SKILL.md body** via `<skill_instructions>` (10 KB/skill, 30 KB total caps) — loader.go:494-555. Pinned entries are honored even when `requires`-unavailable (explicit user choice) — loader.go:382-400.

### 3.3 Access control: grants + allowlist

- **Visibility enum** (`internal/skills/visibility.go:9-26`): `private` (default for user skills; owner-only), `internal` (grant-scoped), `public` (all agents). System/bundled skills seed as `public`.
- **`skill_agent_grants`** table (migrations 000066/000067; columns incl. `agent_id`, `skill_id`, `pinned_version`, `granted_by`, `can_manage`, `tenant_id`) + **`skill_user_grants`** (000078). Grant APIs: `GrantToAgent/RevokeFromAgent/GrantToUser/...` — `internal/store/skill_store.go:234-242`; HTTP in `internal/http/skills_grants.go`; UI dialog `ui/web/src/pages/skills/skill-agent-grants-dialog.tsx`.
- **Resolver enforcement** (`internal/agent/resolver.go:438-452`): at Loop construction, `SkillAccessStore.ListAccessible(ctx, ag.ID, "")` returns the agent's accessible slugs → `LoopConfig.SkillAllowList` (resolver.go:520). Semantics: `nil` = all (store missing or lookup error — fail-open by design, resolver.go:449-451), `[]` = none — `internal/agent/loop_types.go:391`. A **per-request skill filter overrides** the agent-level allowlist — `internal/agent/loop_history_skills.go:25-29`.
- **`ListAccessible` SQL** (`internal/store/pg/skills_grants.go:410-468`): `status IN ('published','active')` AND (`is_system` OR `visibility='public'` OR (`private` AND owner matches user/actor) OR (`internal` AND agent-grant or user-grant exists)), minus tenants that disabled the skill via `skill_tenant_configs`, all tenant-scoped.
- **Search filtering** (`internal/tools/skill_search.go:166-198`): only `managed`-source results are grant-filtered; filesystem-sourced skills (workspace/global/builtin) always pass.
- Scope note: the allowlist gates **listing/injection/search**, not raw `read_file` — a design-only agent should get a curated grant set so the prompt only advertises design skills.

### 3.4 Slash commands

Skills are invocable as `/<prefix>:<slug>` (default `/gc:<slug>`; prefix configurable via system config `SkillSlashCommandPrefixSystemConfigKey`) plus `/gc:list-skills`, `/gc:help <skill>`, `/gc:use <skill>` — `internal/agent/skill_slash_command_matching.go:11-42`, `skill_slash_commands.go:73`. Bundled skills advertise their command in the description, e.g. `description: ... (architect /gc:plan)` — `skills/plan/SKILL.md:3`.

---

## 4. Deployed Skill Inventory (34 SKILL.md skills + `_shared` + 1 kit at /opt/goclaw/skills)

| Slug | Purpose (from frontmatter description, condensed) |
|---|---|
| `architect` | Produce an architecture proposal: goal, design, files, migration, risks, test plan, rollback (`/gc:architect`) |
| `cook` | Implement plans/features with mandatory verification before completion (`/gc:cook`) |
| `copywriting` | Conversion-focused marketing copy: social, ads, email, landing sections, A/B variants |
| `data-analysis` | pandas analysis of CSV/Excel/JSON: profiling, cleaning, aggregation, charts |
| `databases` | Safe PostgreSQL/MongoDB work: queries, EXPLAIN, indexes, migration review (read-only default) |
| `debug` | Prove-the-cause bug investigation pipeline: frame→scout→diagnose→prove→fix→test (`/gc:debug`) |
| `dns-audit` | DNS health of owned domains: SPF/DKIM/DMARC, propagation, misconfigs |
| `docs` | Maintain project docs — update only on behavior/contract change, verify claims (`/gc:docs`) |
| `docs-seeker` | Find & verify official library/API docs before coding against them (llms.txt convention) |
| `docx` | Create/read/edit Word documents incl. tracked changes, comments, templates |
| `fix` | Bug fixing with mandatory root-cause analysis before any change (`/gc:fix`) |
| `fuzz` | Content discovery (ffuf) on owned web apps: hidden endpoints/files/params |
| `goclaw` | Administer/operate/debug a GoClaw gateway via the goclaw CLI/runtime package |
| `loadtest` | HTTP capacity testing (wrk/hey): ramp to the knee, soak, go-live capacity report |
| `mail-digest` | Daily Gmail digest: summarize 24h, group by sender, flag newsletter unsubscribes |
| `media-processing` | FFmpeg/ImageMagick host ops: convert/compress/trim/GIF/thumbnail/watermark/batch |
| `mission` | Drive durable missions via Mission Mode surface (create/pause/resume/complete) |
| `monitor` | Watch sites/prices/feeds over time with cron + state diffing, alert on change |
| `netstress` | Layer-4 resilience testing (iperf3/hping3) of owned infra: TCP/UDP ceilings |
| `ocr` | Tesseract OCR, Vietnamese-first (vie+eng): images, scans, scanned PDFs |
| `pdf` | Full PDF work: read/merge/split/rotate/watermark/create/fill forms/OCR |
| `plan` | Implementation-plan artifact from a user request (`/gc:plan`) |
| `pptx` | Everything .pptx: create/read/edit decks, templates, notes, combine/split |
| `recon` | Attack-surface mapping of owned systems: hosts, ports, services, versions |
| `research` | Structured technical investigation → decision-ready comparison report |
| `review` | Nine-dimension code/design review with severity and written report (`/gc:review`) |
| `scraping` | Scrapling-based scraping/crawling: adaptive selectors, anti-bot, site→Markdown |
| `security-audit` | Structured web/API/host security audit → severity-ranked findings + remediations |
| `skill-creator` | Create/update GoClaw skills with eval-driven iteration (bundled, LICENSE'd) |
| `ssl-audit` | TLS/SSL health check: cert chain, protocols, ciphers, HSTS, OCSP |
| `test` | Structured test planning/execution: unit, integration, chaos, Docker gates (`/gc:test`) |
| `ui-ux-pro-max` | Review/build web UI against GoClaw's mobile/UI/UX rule checklist (`/gc:uiux`) |
| `workspace-organizing` | Purpose-based folder conventions + pre-write discovery for team workspaces/Vault |
| `xlsx` | Everything spreadsheet: read/edit/create .xlsx/.xlsm/.csv/.tsv, formulas, charts |

Plus: `_shared/` (office OOXML pack/unpack/validators/schemas, 50 files — shared Python code symlinked from docx/pptx/xlsx `scripts/office`), and `go-claw-engineer/` (`kit.yaml` bundling plan/fix/cook/review). Repo-only, not yet on server: `decision-log`, `fan-out`, `ship`, `web-browse`.

### Structural conventions observed on the server

- `skill-creator/` — richest layout: `SKILL.md` + `references/` (20 guides) + `scripts/` (init/package/validate/eval runners) + `agents/` (eval agent templates) + `assets/` + `eval-viewer/` (47 files).
- `pdf/`, `pptx/`, `docx/`, `xlsx/` — `SKILL.md` + `scripts/` + optional root-level reference docs (`pdf/forms.md`, `pdf/reference.md`, `pptx/editing.md`, `pptx/pptxgenjs.md`) + `LICENSE.txt`; office skills share `_shared/office` via `scripts/office` symlink.
- `ui-ux-pro-max/`, `media-processing/` — single SKILL.md (181/106 lines), no scripts/references. Prompt-only skills are first-class.
- Frontmatter style: quoted long descriptions with embedded **bilingual (EN + VI) trigger keywords** and explicit "Do NOT use for ..." anti-triggers — this is what BM25 keys on.

---

## 5. Gap Analysis — Design / Video-Design Agent

### Existing partial coverage

| Capability | Existing skill/tool | Coverage | Gap |
|---|---|---|---|
| UI rule compliance | `ui-ux-pro-max` | Reviews web UI against GoClaw's mobile engineering checklist (44px targets, h-dvh, safe areas) | Engineering QA, not visual design creation; nothing about color/typography/motion |
| Media manipulation | `media-processing` | FFmpeg/ImageMagick transcode/trim/thumbnail/GIF | Mechanical ops only — no design decisions (when to cut, how to grade, pacing) |
| Decks/layout-adjacent | `pptx` | Slide creation via python-pptx | Document output, not a layout/typography system |
| Persuasive text | `copywriting` | Marketing copy, CTAs | Text only — no visual direction |
| AI generation tools | `create_image`, `create_video`, `read_image`, `read_video`, `render_video` tools (internal/tools/) | Raw generation/render primitives exist and are wired (cmd/gateway_video.go:47-73) | No skill teaches prompt craft, style consistency, or iteration strategy for them |
| Asset hygiene | `workspace-organizing` | Folder conventions for outputs | Generic; no design-asset naming/export/format conventions |

### Missing entirely (no deployed skill covers)

1. **Storyboard / shot planning** — script→scene breakdown, frame sequencing, shot grammar, continuity.
2. **Color systems** — palette construction, contrast/WCAG accessibility, color tokens, video grading intent.
3. **Typography systems** — type scales, pairing, hierarchy, kinetic type, Vietnamese diacritic rendering.
4. **Motion & transitions** — timing curves/easing, choreography, transition grammar for UI and video.
5. **Composition & layout for frames** — grids, aspect ratios, safe areas, visual hierarchy in static frames and video.
6. **Style consistency / brand direction** — moodboards, style tiles, reusable style tokens across a project's outputs.
7. **Generation prompt craft** — structured prompting for `create_image`/`create_video` (subject/style/lighting/composition/aspect/continuity), variant strategy.
8. **Design critique** — rubric-based review of produced visuals (severity-ranked, like `review` does for code).

---

## 6. Authoring Rules for New Skills

1. **Naming**: slug = directory name, must match `^[a-z0-9][a-z0-9-]*[a-z0-9]$` (`internal/skills/helpers.go:10`); lowercase-hyphen convention (all 34 deployed skills comply). Frontmatter `name` may differ in case/display but keep it equal to the slug like every bundled skill does.
2. **Where files live**:
   - Bundled/system: `skills/<slug>/` in the repo → copy to `/opt/goclaw/skills/<slug>/` (the deployment's bundled dir) → auto-seeded into DB + skills-store at next gateway start (seeder is idempotent via content hash; a changed SKILL.md creates a new version dir). Alternatively hand-place in `<dataDir>/skills-store/<slug>/<version>/` — the Reconciler registers DB rows for orphans (gateway_setup.go:660-676).
   - Agent-authored at runtime: create in `~/.goclaw/skills-store/<name>/` then call `publish_skill` (per deployed skill-creator; tool only registered in non-lite editions).
   - User upload: ZIP with SKILL.md at root or one nesting level, passes `GuardSkillContent` security scan and system-slug conflict check (`internal/http/skills_upload.go:93-181`).
3. **SKILL.md body**: <300 lines; description ≤1024 chars with bilingual EN+VI trigger keywords and "Do NOT use for ..." anti-triggers (this is the BM25 corpus — search indexes only name+description, `search.go:88-89`). Reference detail via `references/*.md` (<300 lines each) pointed to with `{baseDir}` placeholder; executable helpers in `scripts/`; shared multi-skill code in a `_`-prefixed dir.
4. **Declaration of host needs**: `requires:` block (bins/os) for gating; `deps:` block (`system:ffmpeg` etc.) for auto dep-check/archival; `allowed-tools:` to narrow the tool surface; `inputs:/outputs:/quality-gates:` for the contract.
5. **Register/seed**: no manual DB step for bundled skills — seeding is automatic (§2.2). Grant to the design agent via `skill_agent_grants` (HTTP `internal/http/skills_grants.go` or WS methods); set `internal` visibility + grants for design-only exclusivity, or pin the core ones via agent `other_config.pinned_skills` for always-inline behavior.
6. **i18n implications**: skill content itself stays English (LLM consumption — same policy as bootstrap templates); Vietnamese triggers are embedded inline in the description, not in locale files. Any new **UI strings** for the skills pages go into `ui/web/src/i18n/locales/{en,vi,ko,ru,zh}/skills.json` and `ui/desktop/frontend/src/i18n/locales/{en,vi,ru,zh}/skills.json`; any new backend user-facing error message needs a key in `internal/i18n/keys.go` + `catalog_{en,vi,zh}.go`. None of that is needed if the skill set ships without new UI/error surfaces.
7. **Edition implications**: Lite/desktop (sqliteonly) does not register `skill_manage`/`publish_skill` (`cmd/gateway_setup.go:682`, `internal/edition/edition.go:53`) — the new skill set must be fully usable read-only by agents in lite (no self-publish dependency). SQLite store implements the same skills interfaces (`internal/store/sqlitestore/skills*.go`) so seeding works on desktop too.
8. **Multi-tenant**: bundled skills seed as `is_system` (bypass tenant filter, always visible); a tenant-scoped design kit would instead live in the tenant's skills-store with `internal` visibility + per-agent grants, and per-tenant enable/disable flows through `skill_tenant_configs`.

---

## 7. Proposed ORIGINAL Skill Set — "design-studio" (12 skills)

Tuned for a design/video-design agent on goclaw. Original content authored for goclaw's contract (slug naming, bilingual triggers, progressive disclosure, `{baseDir}` references, deps/requires where relevant). Structure inspired only by goclaw's own deployed conventions; nothing ported from any external kit. Suggested kit manifest: `design-studio/kit.yaml` bundbling all 12. Grant strategy: seed as `internal` visibility + grant to the designer agent only; pin the first three.

| # | Slug | Description (1-2 lines) | `references/` files | `scripts/` | deps/requires |
|---|---|---|---|---|---|
| 1 | `creative-brief` | Intake a creative request → structured brief: audience, message, tone, deliverables, constraints, success criteria. Entry point that seeds every other design skill. | `brief-template.md`, `question-checklist.md` | — | — |
| 2 | `storyboard` | Turn a brief or script into a scene/shot plan: beat breakdown, shot list, framing notes, continuity markers, per-shot generation prompts mapped to `create_image`/`create_video`. | `shot-grammar.md`, `continuity-rules.md`, `shot-list-schema.md` | `shot_table.py` (renders shot list to MD/CSV) | — |
| 3 | `color-system` | Build a project palette: model choice, harmony strategy, WCAG contrast passes, token export (CSS vars / JSON), and grading intent for video. | `color-harmony.md`, `contrast-checks.md`, `token-export-format.md` | `contrast_check.py` (computes WCAG ratios from hex pairs) | `pip:` none (stdlib) |
| 4 | `typography` | Type system: scale construction, pairing rules, hierarchy, multilingual caveats (Vietnamese diacritics, CJK), kinetic-type basics for motion work. | `type-scale.md`, `pairing-guide.md`, `diacritics-notes.md` | `scale_gen.py` (emits scale table from ratio) | — |
| 5 | `composition` | Layout & framing: grid systems, aspect ratios, safe areas, focal hierarchy — applied to posters, slides, thumbnails, and video frames. | `grid-systems.md`, `aspect-ratio-specs.md`, `platform-deliverables.md` (per-platform size/safe-area tables) | — | — |
| 6 | `motion-design` | Movement design: duration/easing tables, transition grammar, choreography rules for UI animation and video cuts; when to move what and for how long. | `easing-reference.md`, `transition-grammar.md`, `timing-budget.md` | — | — |
| 7 | `visual-styleguide` | Consolidate creative direction into a durable style tile: palette + type + spacing + imagery rules + do/don't panel, referenced by all later outputs for consistency. | `style-tile-workflow.md`, `consistency-audit.md` | — | — |
| 8 | `image-direction` | Prompt craft & iteration for `create_image`: subject/style/lighting/composition/aspect anatomy, variant strategy, seed/reference reuse, failure diagnosis. | `prompt-anatomy.md`, `style-vocabulary.md`, `iteration-playbook.md` | — | — |
| 9 | `video-direction` | Prompt craft & sequencing for `create_video`/`render_video`: shot duration, camera language, scene continuity, spec assembly for the render pipeline. | `video-prompt-anatomy.md`, `sequence-planning.md`, `render-spec-format.md` | — | — |
| 10 | `design-review` | Rubric-based critique of produced visuals (frames, decks, key art): contrast, hierarchy, alignment, consistency, platform fit — severity-ranked findings like `review` does for code. | `critique-rubric.md`, `severity-scale.md` | — | — |
| 11 | `design-assets` | Naming, organization, versioning, and export conventions for design deliverables (formats, color profiles, sizes), aligned with `workspace-organizing`'s folder model. | `asset-naming.md`, `export-matrix.md` | — | — |
| 12 | `design-handoff` | Package design outputs for engineering: token bundle, spec sheet, asset manifest, redline notes — the contract between designer-agent and builder-agents (pairs with `delegate`). | `token-bundle-schema.md`, `spec-sheet-template.md` | `export_tokens.py` (styleguide MD → CSS-vars/JSON) | — |

Optional 13th if the agent also builds presentation deliverables: `deck-design` (apply styleguide+composition to `pptx` outputs) — defer unless needed, `pptx` already covers mechanics.

Notes on fit:
- Every skill is prompt-first (single SKILL.md + references), matching `ui-ux-pro-max`/`media-processing` precedent; only 4 tiny stdlib-Python scripts proposed — keeps the set lite-edition-safe (no `skill_manage` dependency, no heavy `deps`).
- `image-direction`/`video-direction` intentionally wrap the existing goclaw tools (`create_image`, `create_video`, `render_video`, `read_image`, `read_video`) rather than introducing new ones — zero gateway code changes required to ship the skill set.
- `color-system` + `typography` + `visual-styleguide` + `design-handoff` form the consistency backbone; `storyboard` + `motion-design` + `video-direction` form the video pipeline; `creative-brief` + `design-review` bookend it.

---

## 8. Surface parity statement (for the eventual implementation plan)

- Gateway: N/A — no code change required if skills ship as bundled dirs (auto-seeded). Only `kit.yaml` + `skills/<slug>/` files.
- API contract: N/A — existing skills upload/grant endpoints suffice.
- Web UI: N/A — skills pages render from DB rows automatically; no new strings needed unless new UI is added.
- CLI/runtime: N/A — `goclaw` skill commands operate on seeded rows unchanged.
- Editions: works on Standard and Lite (read-only consumption; no `skill_manage` dependency).
