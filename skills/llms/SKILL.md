---
name: llms
description: >-
  Generate llms.txt and llms-full.txt indexes that make a website or codebase LLM-friendly:
  structure, link curation, summarization rules, and keeping the index fresh. Use when
  making docs or a site consumable by AI agents. Keywords: llms.txt, index, docs, agents.
  Dùng khi cần tạo file llms.txt cho website hoặc codebase, tối ưu tài liệu cho AI.
license: MIT
version: 1
---

# LLMs

Create and maintain llms.txt / llms-full.txt: a curated markdown index at the site root that lets an LLM answer questions about the project without crawling blind.

## When to use
- Making a website, product, or docs set readable by LLM agents
- Summarizing a codebase into an LLM-friendly entry index
- An llms.txt exists but is stale, bloated, or missing key pages
- Preparing a repo or docs site for AI-assisted onboarding

## When NOT to use
- Answering questions from a site right now → `web_fetch` / `web_browse` (and `docs-seeker`)
- General docs quality review → `docs`
- Per-user memory or vault notes → `graphify`, vault tools

## Workflow
1. Fetch or inspect the source material: for a site, pull the docs tree via `web_fetch` (sitemap, docs index); for a codebase, walk key files with `read_file` and `list_files`.
2. Build the short `llms.txt` per convention: an H1 with the project name, a blockquote one-paragraph summary, then curated H2/H3 sections of links with one-line descriptions — what the page contains and when to read it.
3. Curate links, do not dump them: include the 10-30 pages that answer the top questions (quickstart, core concepts, API reference, config, FAQ); exclude marketing pages, paginated duplicates, and generated noise.
4. Write one-line descriptions that carry information ("Auth: OAuth2 flows, token rotation, session config"), not filler ("Everything about auth").
5. Optionally generate `llms-full.txt`: concatenate the curated pages' content in markdown, each preceded by its title as a heading; keep it under a few hundred KB or split per section.
6. Place files at the site root (`/llms.txt`, `/llms-full.txt`) and link them from the docs footer so agents can discover them.
7. Add freshness discipline: note the generation date in the file; prefer stable URLs; add a regeneration step (a script or a scheduled goclaw `cron` task) that re-fetches the sitemap and diffs page lists.
8. Validate like an agent: ask a fresh session to answer three real product questions using only llms.txt; missing answers reveal missing links.

## Output
- `llms.txt` (curated index), optional `llms-full.txt`, placement instructions, and a regeneration note (date, source, refresh mechanism).

## Routing
- Researching the docs being indexed → `docs-seeker`, `research`, `web-browse`
- Deep site inspection with a browser → `web-browse`; docs quality itself → `docs`
- Scheduling refresh jobs in goclaw → the `cron` tool

## Guardrails
- Respect robots.txt and terms of service when fetching source pages; throttle requests.
- Summarize in your own words; do not copy long excerpts wholesale into llms.txt.
- Never include private, internal-only, or credential-bearing URLs in a public index.
- A stale llms.txt misleads agents — date-stamp it and wire the refresh, or say so.
