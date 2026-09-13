#!/usr/bin/env python3
"""scrape.py — Scrapling-powered fetch + structured extraction helper.

Single-page or paginated extraction into JSON/Markdown so agents don't have to
rewrite boilerplate for the common scraping patterns.

Examples:
  python scrape.py https://quotes.toscrape.com/ --markdown --out page.md
  python scrape.py https://quotes.toscrape.com/ --mode stealth --solve-cloudflare --markdown
  python scrape.py https://quotes.toscrape.com/ \
      --css ".quote" --fields "text:.text::text;author:.author::text" \
      --next "li.next a" --max-pages 10 --out quotes.json
"""
from __future__ import annotations

import argparse
import json
import sys
import time
from urllib.parse import urljoin

STDOUT_PREVIEW_CHARS = 600


def die(msg: str, code: int = 2) -> None:
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(code)


def fetch_page(mode: str, url: str, solve_cloudflare: bool):
    try:
        from scrapling.fetchers import DynamicFetcher, Fetcher, StealthyFetcher
    except ImportError:
        die(
            "scrapling is not installed. Run:\n"
            "  pip3 install 'scrapling[fetchers]'\n"
            "  scrapling install   # one-time browser download"
        )
    if mode == "browser":
        return DynamicFetcher.fetch(url, headless=True, network_idle=True)
    if mode == "stealth":
        return StealthyFetcher.fetch(url, headless=True, solve_cloudflare=solve_cloudflare)
    return Fetcher.get(url, stealthy_headers=True)


def parse_fields(spec: str) -> list[tuple[str, str]]:
    fields: list[tuple[str, str]] = []
    for chunk in spec.split(";"):
        chunk = chunk.strip()
        if not chunk:
            continue
        name, sep, selector = chunk.partition(":")
        if not sep or not name.strip() or not selector.strip():
            die(f"invalid --fields chunk {chunk!r} (expected 'name:selector')")
        fields.append((name.strip(), selector.strip()))
    if not fields:
        die("--fields produced no mappings")
    return fields


def extract_items(page, item_css: str, fields: list[tuple[str, str]]) -> list[dict]:
    items: list[dict] = []
    for el in page.css(item_css):
        row: dict = {}
        for name, selector in fields:
            try:
                found = el.css(selector)
            except Exception:
                found = []
            if not found:
                row[name] = None
                continue
            value = found[0]
            row[name] = value if isinstance(value, str) else str(value)
        items.append(row)
    return items


def next_href(page, next_selector: str, base_url: str) -> str | None:
    try:
        found = page.css(next_selector)
    except Exception:
        return None
    if not found:
        return None
    first = found[0]
    if isinstance(first, str):
        href = first
    else:
        href = (getattr(first, "attrib", {}) or {}).get("href")
    return urljoin(base_url, href) if href else None


def write_out(path: str, items: list[dict] | None, markdown: str, meta: dict) -> None:
    if items is not None:
        payload = {**meta, "count": len(items), "items": items}
        text = json.dumps(payload, ensure_ascii=False, indent=2)
    elif path.lower().endswith(".json"):
        text = json.dumps({**meta, "markdown": markdown}, ensure_ascii=False, indent=2)
    else:
        text = markdown
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(text)


def main() -> None:
    parser = argparse.ArgumentParser(description="Scrapling fetch + extraction helper")
    parser.add_argument("url", help="start URL")
    parser.add_argument("--mode", choices=("static", "browser", "stealth"), default="static")
    parser.add_argument("--solve-cloudflare", action="store_true", help="stealth mode only")
    parser.add_argument("--css", help="repeated-element selector (structured mode)")
    parser.add_argument(
        "--fields",
        help='field map "name:selector;name2:selector2" — selectors must end in ::text or ::attr(x)',
    )
    parser.add_argument("--markdown", action="store_true", help="full-page Markdown mode")
    parser.add_argument("--next", help="next-page link selector (enables pagination)")
    parser.add_argument("--max-pages", type=int, default=5)
    parser.add_argument("--delay", type=float, default=1.0, help="seconds between page fetches")
    parser.add_argument("--out", help="output file (.json / .md); prints preview to stdout either way")
    args = parser.parse_args()

    if not args.markdown and not args.css:
        die("choose one extraction mode: --markdown or --css")
    if args.css and not args.fields:
        die("--css requires --fields")

    fields = parse_fields(args.fields) if args.fields else []
    structured = bool(args.css)

    all_items: list[dict] | None = [] if structured else None
    markdown_parts: list[str] = []
    url = args.url
    pages_fetched = 0

    for page_no in range(1, max(1, args.max_pages) + 1):
        try:
            page = fetch_page(args.mode, url, args.solve_cloudflare)
        except Exception as exc:
            die(f"fetch failed for {url}: {exc}")
        pages_fetched += 1

        if structured:
            all_items.extend(extract_items(page, args.css, fields))
        else:
            try:
                markdown_parts.append(page.markdown())
            except Exception as exc:
                die(f"markdown() failed: {exc}")

        if not args.next or page_no >= args.max_pages:
            break
        nxt = next_href(page, args.next, url)
        if not nxt:
            break
        if nxt == url:
            break
        url = nxt
        time.sleep(max(0.0, args.delay))

    markdown = "\n\n---\n\n".join(markdown_parts)
    meta = {"url": args.url, "mode": args.mode, "pages_fetched": pages_fetched}

    if args.out:
        write_out(args.out, all_items, markdown, meta)

    if structured:
        preview = json.dumps(all_items[:2], ensure_ascii=False)
        summary = {
            **meta,
            "count": len(all_items),
            "out": args.out,
            "preview": preview[:STDOUT_PREVIEW_CHARS],
        }
    else:
        summary = {
            **meta,
            "chars": len(markdown),
            "out": args.out,
            "preview": markdown[:STDOUT_PREVIEW_CHARS],
        }
    print(json.dumps(summary, ensure_ascii=False))


if __name__ == "__main__":
    main()
