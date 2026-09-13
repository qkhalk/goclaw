---
name: scraping
description: "Scrape, crawl, and extract structured data or LLM-ready Markdown from websites using Scrapling (adaptive auto-healing selectors, anti-bot fetchers, pagination, site-to-Markdown). Use whenever the user asks to scrape or crawl a site, extract products/prices/listings/tables/links/articles, build a dataset from web pages, convert pages to Markdown for RAG, or collect data behind JS-rendered or Cloudflare-protected pages they are allowed to access. Triggers: scrape, scraping, crawl, crawler, extract data from website, web data collection, cào dữ liệu, lấy dữ liệu web, thu thập dữ liệu, bóc tách dữ liệu, giá sản phẩm, RAG corpus. Do NOT use for plain REST/API endpoints (call them directly) or for sites that require the user's own login credentials you have not been given."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - target_url
  - extraction_spec
outputs:
  - dataset
allowed-tools:
  - filesystem
  - shell
  - search
  - exec
quality-gates:
  - target_authorized
  - politeness_limits_set
  - dataset_generated
deps:
  - pip:scrapling[fetchers]
  - pip:markdownify
---

# Web Scraping (Scrapling)

Extract data from websites with [Scrapling](https://github.com/D4Vinci/Scrapling) —
an adaptive Python scraping framework: fetchers that handle TLS fingerprinting and
anti-bot pages, CSS/XPath extraction with auto-healing selectors, and one-call
Markdown conversion for RAG.

This skill handles public-web data collection. It does NOT handle: login
credential stuffing, paywall/auth bypass, or scraping targets the requester has no
right to scrape.

## Setup check (first run on a host)

```bash
python3 -c "import scrapling" 2>/dev/null || pip3 install 'scrapling[fetchers]'
python3 -c "import markdownify" 2>/dev/null || pip3 install markdownify   # needed for --markdown
scrapling install --force 2>/dev/null || scrapling install   # one-time: downloads browsers for browser/stealth modes
```

Static mode works without browsers; browser/stealth modes require `scrapling install`.
Prefer static mode — it is lightweight (no browser) and its TLS impersonation
passes most basic bot checks.

## Rules of engagement (before any fetch)

1. **Target authorized.** Confirm the target is public data the requester may
   collect, or they have permission from the site owner. Private/logged-in areas
   require credentials the requester explicitly provided.
2. **Politeness limits set.** Default `--delay 1` (≥1s between requests), cap
   pages with `--max-pages`, and stay off `robots.txt`-disallowed paths when the
   requester has no ownership stake. Never hammer one host concurrently.
3. **Stop on block signals.** A 403/429, escalating CAPTCHAs, or an error page
   means STOP retrying that site — report what happened and ask the requester how
   to proceed. Do not escalate to stealth mode to defeat a block on a site the
   requester does not own.
4. **Respect takedowns.** If the site's terms forbid scraping, say so and let the
   requester decide whether an official API/export exists instead.

## Choose the fetch mode

| Mode | Command | Use when |
|------|---------|----------|
| `static` (default) | Fetcher.get | Server-rendered HTML, APIs returning HTML, fast + light |
| `browser` | DynamicFetcher | JS-rendered content, infinite scroll pages, SPAs |
| `stealth` | StealthyFetcher | Anti-bot interstitials (Cloudflare Turnstile) on targets you may access |

Rule: always try `static` first; escalate only when content is missing or blocked.

## Quick extraction — Scrapling CLI (no code)

```bash
# Full page → Markdown (LLM/RAG-ready, no LLM in the loop)
scrapling extract get "https://example.com/article" out.md

# Same, output plain text or raw HTML by extension (.txt / .html)
scrapling extract get "https://example.com/article" page.txt

# JS-rendered page → Markdown (downloads browsers on first use)
scrapling extract fetch "https://example.com/spa" out.md

# Anti-bot page → Markdown
scrapling extract stealthy-fetch "https://example.com" out.md --solve-cloudflare

# Limit to a subtree before conversion
scrapling extract get "https://example.com" out.md --css-selector "main article"
```

Prefer the helper script below for repeated-element (list) extraction — the CLI
is best for whole-page Markdown/text.

## Structured extraction — helper script

`scripts/scrape.py` fetches one or many pages and writes JSON (or Markdown):

```bash
cd {baseDir}

# One page → Markdown file
python3 scripts/scrape.py "https://quotes.toscrape.com/" --markdown --out page.md

# Repeated elements → JSON with per-item fields.
# Field selectors must end in ::text or ::attr(name) so values are strings.
python3 scripts/scrape.py "https://quotes.toscrape.com/" \
  --css ".quote" \
  --fields "text:.text::text;author:.author::text;link:.author a::attr(href)" \
  --out quotes.json

# Paginated list: follow "next" up to N pages, 1s delay between fetches
python3 scripts/scrape.py "https://quotes.toscrape.com/" \
  --css ".quote" --fields "text:.text::text;author:.author::text" \
  --next "li.next a" --max-pages 10 --delay 1 --out all_quotes.json

# JS-rendered target
python3 scripts/scrape.py "https://example.com/listing" --mode browser \
  --css ".item" --fields "title:h2::text;price:.price::text" --out items.json

# Cloudflare-protected target the requester owns
python3 scripts/scrape.py "https://example.com" --mode stealth --solve-cloudflare --markdown --out page.md
```

The script prints a one-line JSON summary (pages fetched, item count, short
preview) and writes the full dataset to `--out`. Read the file with the
filesystem tool; do not assume the preview is the whole dataset.

## Adaptive selectors (survive site redesigns)

When a site changes markup, plain CSS breaks. Scrapling can remember element
fingerprints and re-find them:

```python
from scrapling.fetchers import Fetcher

page = Fetcher.get("https://example.com/products", stealthy_headers=True)
products = page.css(".product", auto_save=True)   # first run: learn + save
# later, after the site changed:
products = page.css(".product", adaptive=True, adaptive_domain="example.com")  # auto-heal
```

Use `auto_save=True` on the first scrape of a recurring job and `adaptive=True`
on subsequent runs. For one-off scrapes, skip adaptive entirely.

## Deeper needs — inline Python

For anything the script/CLI doesn't cover (login flows with provided
credentials, XHR capture, concurrent crawling), write a short Python script with
the actual library instead of fighting the helpers:

```python
from scrapling.fetchers import FetcherSession   # session = cookies across requests

with FetcherSession(impersonate="chrome") as s:
    page = s.get("https://example.com/search?q=widget", stealthy_headers=True)
    for item in page.css(".result"):
        print(item.css("h3 a::attr(href)"))
```

Crawl frameworks (`Spider`, `CrawlSpider`, `SiteToMarkdownSpider` for whole-site
RAG corpora) live in `scrapling.spiders` — use them only when pagination via
`--next` is not enough (multi-section crawls, sitemaps, throttled deep crawls).

## Output conventions

- Save datasets under the agent workspace (`scraped/<site>/<name>.json`), never
  to system paths.
- JSON for structured items (list of dicts, consistent field names); Markdown
  for documents/RAG; CSV only if the requester asks.
- Report at the end: pages fetched, items per field completeness, files written,
  and any pages skipped due to blocks (with the stop reason).

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `scrapling is not installed` | `pip3 install 'scrapling[fetchers]'` |
| Browser launch error (browser/stealth modes) | `scrapling install`, retry once |
| Empty items but page loads | Content is JS-rendered → `--mode browser`; or selector wrong → inspect HTML saved via `scrapling extract get <url> page.html` |
| `markdown() failed: requires markdownify` | `pip3 install markdownify` |
| 403/429 or challenge page | Stop per Rules of engagement unless the requester owns the target → `--mode stealth --solve-cloudflare` (stealth needs `scrapling install`) |
| Fields all null | Field selectors must end in `::text` / `::attr(name)`; verify selector against saved HTML |
| Missing later pages | `--next` selector wrong (check it matches the actual link element), or `--max-pages` too low |
