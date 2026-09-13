---
name: monitor
description: "Watch websites, product prices, or news feeds over time and alert on changes — RSS/Atom feeds, product listings, price drops, restocks, keyword news. Sets up recurring checks with the cron tool, stores a state baseline, diffs each run, and reports only what changed to the user's channel. Use whenever the user wants to 'theo dõi', monitor, track, or get notified about: price drops/giá giảm, restocks/hàng về, new articles/tin mới theo chủ đề, job postings, release notes, competitor pages, or any page that changes over time. Triggers: theo dõi giá, báo giá giảm, theo dõi tin, cảnh báo, monitor price, track changes, notify me, RSS, price watch, deal alert. Do NOT use for one-time scraping (use the scraping skill) or for sources the user must log into."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - watch_spec
  - schedule
outputs:
  - change_report
allowed-tools:
  - filesystem
  - shell
  - exec
  - cron
quality-gates:
  - baseline_stored
  - cron_scheduled
  - alerts_delivered
deps:
  - pip:feedparser
---

# Monitor (price & news watch)

Recurring watch over web sources: fetch on a schedule, diff against a stored
baseline, report only what changed. RSS/Atom needs no browser; HTML pages use
Scrapling static fetch (lightweight, see the scraping skill).

## How it works

1. **Baseline run** — first check records what exists today, no alerts.
2. **Scheduled runs** — cron triggers the check; the script diffs against the
   stored state and emits new items / price changes only.
3. **Delivery** — the agent (in the cron run) summarizes changes and replies in
   the channel that scheduled the job (Telegram/web), or stays silent when
   nothing changed.

## Setup a watch (interactive, first)

Ask the user for: source URL(s), what counts as interesting (all new items?
price below a threshold? keyword match), and check frequency (default: hourly
for deals, every 4–6h for news). Confirm before scheduling.

Then create the baseline:

```bash
cd {baseDir}
python3 -c "import feedparser" 2>/dev/null || pip3 install feedparser

# News / blog via RSS (preferred — clean and polite)
python3 scripts/check_watch.py --name vnexpress --url "https://vnexpress.net/rss/tin-moi-nhat.rss" \
  --type rss --state ~/workspace/monitor/vnexpress.json

# HTML product listing (Scrapling static fetch — no browser needed)
python3 scripts/check_watch.py --name shop-gpu --url "https://shop.example/gpu" --type html \
  --item-css ".product-item" --title-css "h3" --link-css "a::attr(href)" --price-css ".price::text" \
  --state ~/workspace/monitor/shop-gpu.json
```

The baseline report says `"baseline": true` — tell the user how many items were
recorded. State files live in `~/workspace/monitor/<name>.json` (the script
creates directories).

## Schedule it (cron tool)

One watch = one cron job whose prompt re-runs the check and reports:

```
cron create — name: "watch-shop-gpu", every 1h:
"Run: python3 ~/skills/monitor/scripts/check_watch.py --name shop-gpu
--url https://shop.example/gpu --type html --item-css '.product-item'
--title-css 'h3' --link-css 'a::attr(href)' --price-css '.price::text'
--state ~/workspace/monitor/shop-gpu.json
If new_count>0 or changed_count>0, tell me which items are new or changed
price (old → new). Otherwise reply with nothing and end quietly."
```

Notes:
- Use the **absolute script path** in cron prompts (cron runs may start from a
  different working directory). Resolve `{baseDir}` once at setup.
- Hourly is plenty — resist the user's "every minute" unless they insist; add
  politeness (`--delay`) when checking multiple pages of one site.
- One job per source so the user can pause/delete each watch independently.

## Reading reports & thresholds

- The script prints JSON: `new_count`/`new[]` (items never seen before) and
  `changed_count`/`changed[]` (price differs from baseline, with `old_price`).
- **Threshold filters** (e.g. "chỉ báo khi giá < 5 triệu") are the agent's job
  at report time: parse prices from `changed[]`/`new[]`, apply the rule, and
  only surface matches. Store the rule in the cron prompt verbatim so it
  survives restarts.
- Keyword filters for news work the same way: match title/summary against the
  user's keywords, report only hits.
- `baseline: true` output = initial state saved; never alert on it.

## Rules of engagement

- **Politeness:** RSS is cheap; HTML checks ≥30 min apart per source by
  default. Multi-page checks must stay inside the scraping skill's
  rules of engagement.
- **No login walls:** watch only sources reachable without the user's
  credentials. Private sources are out of scope.
- **Stop conditions:** two consecutive fetch failures → say so in the next
  report and suggest checking the URL; do not silently keep failing for days.
- Deleting a watch: `cron delete` the job **and** remove the state file.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `feedparser not installed` | `pip3 install feedparser` (small, no gate) |
| HTML fetch fails / empty items | See the scraping skill troubleshooting (JS-rendered pages need the browser install gate — ask the user first) |
| Everything shows as new every run | The site reshuffles item URLs/ids — switch `--link-css` to a stable permalink, or watch the RSS instead |
| Prices never change | Selector caught a static label; verify with `--no-save` runs and inspect `fetched` items |
| Cron fires but nothing reported | Cron runs are quiet by design when nothing changed — use `cron run` manually to test once |
