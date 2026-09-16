/**
 * Client-side DOM → markdown extraction for the browser panel.
 *
 * Runs in the dashboard page against the relayed document's same-origin
 * iframe contentDocument. The server fetched + sanitized the HTML (no
 * scripts); the user's browser loaded the images/CSS and laid the page out —
 * this module turns that DOM into readable markdown for the agent, keeping
 * the parsing CPU off the server.
 *
 * With `annotateRefs` the walk also tags interactive elements with
 * data-gcref="eN" (the tag doubles as the action selector for click/type)
 * and emits an [eN] marker next to each one, so the agent can operate the
 * page via web_browse actions.
 */

const SKIP_TAGS = new Set([
  "SCRIPT", "STYLE", "NOSCRIPT", "SVG", "TEMPLATE", "IFRAME", "OBJECT", "EMBED",
  "NAV", "FOOTER", "ASIDE", "SELECT", "OPTION", "BUTTON", "INPUT",
  "TEXTAREA", "LABEL", "PICTURE", "SOURCE", "VIDEO", "AUDIO", "CANVAS", "MAP",
]);
// NB: FORM must NOT be skipped — its subtree carries the page's inputs and
// submit buttons. Skipping it hid search boxes and login forms from both the
// markdown and the [eN] ref annotation (walk() never reached the children).

const READ_SKIP_TAGS = new Set([...SKIP_TAGS]);

/** Tags whose subtree is skipped when reading but which get an [eN] ref. */
const REF_TAGS = new Set(["BUTTON", "SELECT", "TEXTAREA", "INPUT", "SUMMARY"]);

/** Max extracted markdown size sent back to the server. */
export const MAX_EXTRACT_CHARS = 200_000;

/** Minimum body text for the extraction to be considered useful. */
export const MIN_USEFUL_CHARS = 80;

/** Ref attribute stamped onto interactive elements for click/type actions. */
export const REF_ATTR = "data-gcref";

export interface ExtractResult {
  markdown: string;
  /** Live elements for refs e1..eN (index 0 = e1). Empty when not annotating. */
  refs: Element[];
}

interface WalkCtx {
  out: string[];
  listDepth: number;
  ordered: boolean[];
  counters: number[];
  /** When present, interactive elements are tagged + emitted as [eN]. */
  refs: Element[] | null;
}

/** Extract readable markdown from a document (or document body). */
export function extractMarkdown(doc: Document): string {
  const body = doc.body ?? doc.documentElement;
  if (!body) return "";
  const ctx: WalkCtx = { out: [], listDepth: 0, ordered: [], counters: [], refs: null };
  walkChildren(body, ctx);
  return clean(ctx.out.join(""));
}

/** Like extractMarkdown, but tags interactive elements and returns ref info. */
export function extractPageWithRefs(doc: Document): ExtractResult {
  const body = doc.body ?? doc.documentElement;
  if (!body) return { markdown: "", refs: [] };
  // Re-extraction on an already-tagged document (e.g. agent "extract" action)
  // must start from a clean slate, otherwise refElement() can match a stale
  // element that the new walk no longer numbers.
  doc.querySelectorAll(`[${REF_ATTR}]`).forEach((el) => el.removeAttribute(REF_ATTR));
  const ctx: WalkCtx = { out: [], listDepth: 0, ordered: [], counters: [], refs: [] };
  walkChildren(body, ctx);
  return { markdown: clean(ctx.out.join("")), refs: ctx.refs ?? [] };
}

/** Look up the ref element for an "[eN]" id (1-based) in the tagged doc. */
export function refElement(doc: Document, ref: string): Element | null {
  const n = Number.parseInt(ref.replace(/^e/i, ""), 10);
  if (!Number.isFinite(n) || n < 1) return null;
  return doc.querySelector(`[${REF_ATTR}="e${n}"]`);
}

function isInteractive(el: Element): boolean {
  const tag = el.tagName.toUpperCase();
  if (tag === "A" && el.getAttribute("href")) return true;
  if (REF_TAGS.has(tag)) return true;
  const role = el.getAttribute("role");
  return role === "button" || role === "link" || role === "textbox";
}

function describeControl(el: Element): string {
  const hint =
    el.getAttribute("placeholder") ??
    el.getAttribute("aria-label") ??
    el.getAttribute("name") ??
    el.getAttribute("type") ??
    "";
  return hint ? ` (${hint.slice(0, 60)})` : "";
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

  if (el.getAttribute("aria-hidden") === "true" || el.getAttribute("hidden") !== null) return;

  // Ref-annotation mode: interactive elements become [eN] markers. Links keep
  // their markdown form; controls get a descriptor (their subtrees are noise).
  if (ctx.refs && isInteractive(el)) {
    ctx.refs.push(el);
    el.setAttribute(REF_ATTR, `e${ctx.refs.length}`);
    const marker = `[e${ctx.refs.length}]`;
    if (tag === "A") {
      const href = (el.getAttribute("href") ?? "").trim();
      const inner = collapse(el.textContent ?? "") || href;
      ctx.out.push(` [${inner}](<${href}>) ${marker} `);
      return;
    }
    if (tag === "BUTTON" || tag === "SUMMARY" || roleIsClickable(el)) {
      const label = collapse(el.textContent ?? "") || describeControl(el);
      ctx.out.push(` ${marker} **${label || tag.toLowerCase()}** `);
      return;
    }
    // INPUT / SELECT / TEXTAREA descriptors.
    ctx.out.push(` ${marker} **${tag.toLowerCase()}**${describeControl(el)} `);
    return;
  }

  if (READ_SKIP_TAGS.has(tag)) return;

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

function roleIsClickable(el: Element): boolean {
  const role = el.getAttribute("role");
  return role === "button" || role === "link";
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
