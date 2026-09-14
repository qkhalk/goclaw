/**
 * Client-side DOM → markdown extraction for the browser panel.
 *
 * Runs in the dashboard page against the relayed document's same-origin
 * iframe contentDocument. The server fetched + sanitized the HTML (no
 * scripts); the user's browser loaded the images/CSS and laid the page out —
 * this module turns that DOM into readable markdown for the agent, keeping
 * the parsing CPU off the server.
 */

const SKIP_TAGS = new Set([
  "SCRIPT", "STYLE", "NOSCRIPT", "SVG", "TEMPLATE", "IFRAME", "OBJECT", "EMBED",
  "NAV", "FOOTER", "ASIDE", "FORM", "SELECT", "OPTION", "BUTTON", "INPUT",
  "TEXTAREA", "LABEL", "PICTURE", "SOURCE", "VIDEO", "AUDIO", "CANVAS", "MAP",
]);

/** Max extracted markdown size sent back to the server. */
export const MAX_EXTRACT_CHARS = 200_000;

/** Minimum body text for the extraction to be considered useful. */
export const MIN_USEFUL_CHARS = 80;

interface WalkCtx {
  out: string[];
  listDepth: number;
  ordered: boolean[];
  counters: number[];
}

/** Extract readable markdown from a document (or document body). */
export function extractMarkdown(doc: Document): string {
  const body = doc.body ?? doc.documentElement;
  if (!body) return "";
  const ctx: WalkCtx = { out: [], listDepth: 0, ordered: [], counters: [] };
  walkChildren(body, ctx);
  return clean(ctx.out.join(""));
}

function walkChildren(node: Element | DocumentFragment | Node, ctx: WalkCtx): void {
  for (let child = node.firstChild; child; child = child.nextSibling) {
    walk(child, ctx);
  }
}

function walk(node: Node, ctx: WalkCtx): void {
  if (node.nodeType === Node.TEXT_NODE) {
    const text = collapse((node.textContent ?? ""));
    if (text) ctx.out.push(text);
    return;
  }
  if (node.nodeType !== Node.ELEMENT_NODE) return;
  const el = node as Element;
  const tag = el.tagName.toUpperCase();

  if (SKIP_TAGS.has(tag)) return;
  if (el.getAttribute("aria-hidden") === "true" || el.getAttribute("hidden") !== null) return;

  switch (tag) {
    case "H1": return heading(el, ctx, 1);
    case "H2": return heading(el, ctx, 2);
    case "H3": return heading(el, ctx, 3);
    case "H4": return heading(el, ctx, 4);
    case "H5": return heading(el, ctx, 5);
    case "H6": return heading(el, ctx, 6);
    case "P": return block(el, ctx, "\n\n", "\n\n");
    case "DIV": case "SECTION": case "ARTICLE": case "MAIN": return block(el, ctx, "\n\n", "\n\n");
    case "BLOCKQUOTE": return blockquote(el, ctx);
    case "PRE": return pre(el, ctx);
    case "UL": case "OL": return list(el, ctx);
    case "LI": return block(el, ctx, "\n", "");
    case "BR": ctx.out.push("\n"); return;
    case "HR": ctx.out.push("\n\n---\n\n"); return;
    case "A": return link(el, ctx);
    case "STRONG": case "B": return wrap(el, ctx, "**");
    case "EM": case "I": return wrap(el, ctx, "*");
    case "CODE": return inlineCode(el, ctx);
    case "IMG": {
      const alt = (el.getAttribute("alt") ?? "").trim();
      const src = (el.getAttribute("src") ?? "").trim();
      if (src && !src.startsWith("data:")) {
        ctx.out.push(`![${alt}](<${src}>)`);
      } else if (alt) {
        ctx.out.push(alt);
      }
      return;
    }
    case "TABLE": return table(el, ctx);
    case "THEAD": case "TBODY": case "TFOOT": return walkChildren(el, ctx);
    case "TR": return tableRow(el, ctx);
    case "TD": case "TH": return block(el, ctx, " ", " |");
    case "FIGCAPTION": return block(el, ctx, "\n\n", "\n\n");
    case "FIGURE": return block(el, ctx, "\n\n", "\n\n");
    default: return walkChildren(el, ctx);
  }
}

function heading(el: Element, ctx: WalkCtx, level: number): void {
  const hashes = "#".repeat(level);
  ctx.out.push(`\n\n${hashes} `);
  walkChildren(el, ctx);
  ctx.out.push("\n\n");
}

function block(el: Element, ctx: WalkCtx, before: string, after: string): void {
  ctx.out.push(before);
  walkChildren(el, ctx);
  ctx.out.push(after);
}

function wrap(el: Element, ctx: WalkCtx, marker: string): void {
  ctx.out.push(marker);
  walkChildren(el, ctx);
  ctx.out.push(marker);
}

function inlineCode(el: Element, ctx: WalkCtx): void {
  const text = collapse(el.textContent ?? "");
  if (text) ctx.out.push("`" + text + "`");
}

function link(el: Element, ctx: WalkCtx): void {
  const href = (el.getAttribute("href") ?? "").trim();
  const inner = collapse(el.textContent ?? "");
  if (href && !href.startsWith("javascript:") && !href.startsWith("#")) {
    ctx.out.push(`[${inner}](<${href}>)`);
  } else if (inner) {
    ctx.out.push(inner);
  }
}

function blockquote(el: Element, ctx: WalkCtx): void {
  ctx.out.push("\n\n");
  const start = ctx.out.length;
  walkChildren(el, ctx);
  const quoted = ctx.out.splice(start).join("").trim().split("\n")
    .map((line) => "> " + line).join("\n");
  ctx.out.push(quoted, "\n\n");
}

function pre(el: Element, ctx: WalkCtx): void {
  // Preserve raw whitespace inside pre; skip child walkers that collapse it.
  const code = el.querySelector("code");
  const text = (code ?? el).textContent ?? "";
  ctx.out.push(`\n\n\`\`\`\n${text.replace(/\n+$/, "")}\n\`\`\`\n\n`);
}

function list(el: Element, ctx: WalkCtx): void {
  ctx.listDepth++;
  ctx.ordered.push(el.tagName === "OL");
  ctx.counters.push(0);
  ctx.out.push("\n");
  walkChildren(el, ctx);
  ctx.out.push("\n");
  ctx.listDepth--;
  ctx.ordered.pop();
  ctx.counters.pop();
}

function table(el: Element, ctx: WalkCtx): void {
  const rows = Array.from(el.querySelectorAll("tr"));
  const head = rows[0];
  if (!head) return;
  const cellText = (row: Element) =>
    Array.from(row.children).filter((c) => /^(TD|TH)$/i.test(c.tagName))
      .map((c) => collapse(c.textContent ?? "").replace(/\|/g, "\\|") || " ");

  const first = cellText(head);
  if (first.length === 0) return;
  ctx.out.push("\n\n| " + first.join(" | ") + " |\n");
  ctx.out.push("|" + first.map(() => " --- ").join("|") + "|\n");
  for (const row of rows.slice(1)) {
    const cells = cellText(row);
    if (cells.length > 0) ctx.out.push("| " + cells.join(" | ") + " |\n");
  }
  ctx.out.push("\n");
}

function tableRow(el: Element, ctx: WalkCtx): void {
  // Rows reached outside table() (thead/tbody handled there).
  walkChildren(el, ctx);
  ctx.out.push("\n");
}

function collapse(text: string): string {
  return text.replace(/[ \t\r\n]+/g, " ").trim();
}

function clean(markdown: string): string {
  return markdown
    .replace(/\n{3,}/g, "\n\n")
    .replace(/[ \t]+\n/g, "\n")
    .replace(/^ +/, "")
    .trim();
}
