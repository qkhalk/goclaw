#!/usr/bin/env python3
"""check_watch.py — fetch one watched source and diff it against stored state.

RSS/Atom feeds work out of the box. HTML pages use Scrapling static fetch
(see the scraping skill) with CSS selectors.

Examples:
  # First run = baseline (records items, no alerts)
  python3 scripts/check_watch.py --name vnexpress --url "https://vnexpress.net/rss/tin-moi-nhat.rss" \
      --type rss --state ~/workspace/monitor/vnexpress.json

  # Later runs print only what changed
  python3 scripts/check_watch.py --name vnexpress --url "..." --type rss \
      --state ~/workspace/monitor/vnexpress.json

  # HTML product page: item blocks with title/link/price selectors
  python3 scripts/check_watch.py --name shop-cpu --url "https://shop.example/cpu" --type html \
      --item-css ".product-item" --title-css "h3" --link-css "a" --price-css ".price" \
      --state ~/workspace/monitor/shop-cpu.json
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
from datetime import datetime, timezone


def die(msg: str, code: int = 2) -> None:
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(code)


def first_text(el, selector: str | None) -> str | None:
    if not selector:
        return None
    try:
        found = el.css(selector)
    except Exception:
        return None
    if not found:
        return None
    v = found[0]
    return v if isinstance(v, str) else str(v).strip() or None


def load_state(path: str) -> dict:
    if os.path.isfile(path):
        try:
            with open(path, encoding="utf-8") as fh:
                return json.load(fh)
        except (json.JSONDecodeError, OSError):
            pass
    return {"items": {}}


def save_state(path: str, state: dict) -> None:
    os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
    state["updated"] = datetime.now(timezone.utc).isoformat(timespec="seconds")
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(state, fh, ensure_ascii=False)
    os.replace(tmp, path)


def fetch_rss(url: str, max_items: int) -> list[dict]:
    try:
        import feedparser
    except ImportError:
        die("feedparser not installed. Run: pip3 install feedparser  (small)")
    feed = feedparser.parse(url)
    if getattr(feed, "bozo", False) and not feed.entries:
        die(f"feed fetch/parse failed for {url}: {getattr(feed, 'bozo_exception', 'unknown')}")
    items = []
    for e in feed.entries[:max_items]:
        items.append(
            {
                "id": getattr(e, "id", None) or getattr(e, "link", None) or e.get("title", ""),
                "title": (e.get("title") or "").strip(),
                "url": getattr(e, "link", None),
                "summary": (e.get("summary") or "")[:300],
                "price": None,
            }
        )
    return items


def fetch_html(url: str, args) -> list[dict]:
    if not args.item_css:
        die("html mode requires --item-css (plus --title-css/--link-css/--price-css)")
    try:
        from scrapling.fetchers import Fetcher
    except ImportError:
        die("scrapling not installed — see the scraping skill setup: pip3 install 'scrapling[fetchers]'")
    try:
        page = Fetcher.get(url, stealthy_headers=True)
    except Exception as exc:
        die(f"fetch failed for {url}: {exc}")
    items = []
    for el in page.css(args.item_css)[: args.max_items]:
        title = first_text(el, args.title_css)
        link = first_text(el, args.link_css)
        price = first_text(el, args.price_css)
        if not title and not link:
            continue
        item_id = link or title or hashlib.sha1((title or "") + (price or "")).hexdigest()[:16]
        items.append({"id": item_id, "title": title, "url": link, "summary": None, "price": price})
    return items


def main() -> None:
    parser = argparse.ArgumentParser(description="Watch one source for new items / price changes")
    parser.add_argument("--name", required=True, help="watch label (for the report)")
    parser.add_argument("--url", required=True)
    parser.add_argument("--type", choices=("rss", "html"), default="rss")
    parser.add_argument("--item-css", help="html mode: repeated item block selector")
    parser.add_argument("--title-css", help="html mode: title selector within an item")
    parser.add_argument("--link-css", help="html mode: link selector (should yield ::attr(href))")
    parser.add_argument("--price-css", help="html mode: price selector (should yield ::text)")
    parser.add_argument("--state", required=True, help="JSON state file (persists between runs)")
    parser.add_argument("--max-items", type=int, default=50)
    parser.add_argument("--no-save", action="store_true", help="fetch and report without updating state")
    args = parser.parse_args()

    items = fetch_rss(args.url, args.max_items) if args.type == "rss" else fetch_html(args.url, args)
    state = load_state(args.state)
    known = state.get("items", {})

    new_items, changed = [], []
    for it in items:
        prev = known.get(it["id"])
        if prev is None:
            new_items.append(it)
        elif it["price"] and prev.get("price") and it["price"] != prev["price"]:
            changed.append({**it, "old_price": prev["price"]})
        elif it["price"] and prev.get("price") is None:
            changed.append({**it, "old_price": None})

    is_baseline = len(known) == 0 and not args.no_save
    if not args.no_save:
        for it in items:
            known[it["id"]] = {"title": it["title"], "url": it["url"], "price": it["price"]}
        if len(known) > 2000:  # keep state bounded: drop half the oldest inserts
            for k in list(known)[: len(known) - 1500]:
                known.pop(k, None)
        state["items"] = known
        save_state(args.state, state)

    report = {
        "name": args.name,
        "fetched": len(items),
        "baseline": is_baseline,
        "new_count": 0 if is_baseline else len(new_items),
        "new": [] if is_baseline else new_items[:10],
        "changed_count": len(changed),
        "changed": changed[:10],
        "state": args.state,
    }
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
