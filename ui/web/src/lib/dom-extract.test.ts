import { describe, expect, it } from "vitest";
import { extractMarkdown, extractPageWithRefs, refElement, REF_ATTR } from "./dom-extract";

function doc(html: string): Document {
  const d = document.implementation.createHTMLDocument("test");
  d.body.innerHTML = html;
  return d;
}

describe("extractPageWithRefs", () => {
  it("tags links in document order and emits [eN] markers", () => {
    const d = doc(`<p>Hello</p><a href="https://example.com/docs">Docs</a><a href="https://example.com/api">API</a>`);
    const { markdown, refs } = extractPageWithRefs(d);

    expect(refs).toHaveLength(2);
    expect(refs[0]?.getAttribute(REF_ATTR)).toBe("e1");
    expect(refs[1]?.getAttribute(REF_ATTR)).toBe("e2");
    expect(markdown).toContain("[Docs](<https://example.com/docs>) [e1]");
    expect(markdown).toContain("[API](<https://example.com/api>) [e2]");
  });

  it("tags buttons, inputs and textareas with descriptors", () => {
    const d = doc(
      `<button>Save</button><input type="text" placeholder="Search"/><textarea placeholder="Comment"></textarea>`,
    );
    const { markdown, refs } = extractPageWithRefs(d);

    expect(refs).toHaveLength(3);
    expect(markdown).toContain("[e1] **Save**");
    expect(markdown).toContain("[e2] **input** (Search)");
    expect(markdown).toContain("[e3] **textarea** (Comment)");
  });

  it("ignores anchors without href and non-interactive elements", () => {
    const d = doc(`<a name="anchor">not a link</a><span role="note">plain</span><p>text</p>`);
    const { markdown, refs } = extractPageWithRefs(d);

    expect(refs).toHaveLength(0);
    expect(markdown).not.toContain("[e1]");
    expect(markdown).toContain("not a link");
  });

  it("tags role=button divs with their text label", () => {
    const d = doc(`<div role="button">Submit form</div>`);
    const { markdown, refs } = extractPageWithRefs(d);

    expect(refs).toHaveLength(1);
    expect(markdown).toContain("[e1] **Submit form**");
  });

  it("does not mutate documents extracted without refs", () => {
    const d = doc(`<a href="https://example.com">Link</a>`);
    extractMarkdown(d);

    expect(d.querySelector(`[${REF_ATTR}]`)).toBeNull();
  });

  it("walks into FORM subtrees so inputs and submit buttons get refs", () => {
    const d = doc(
      `<form action="/search"><p>Search the docs</p>` +
        `<input type="text" name="q" placeholder="Keyword"/>` +
        `<button type="submit">Search</button></form>`,
    );
    const { markdown, refs } = extractPageWithRefs(d);

    expect(refs).toHaveLength(2);
    expect(refs[0]?.getAttribute(REF_ATTR)).toBe("e1");
    expect(markdown).toContain("Search the docs");
    expect(markdown).toContain("[e1] **input** (Keyword)");
    expect(markdown).toContain("[e2] **Search**");
  });

  it("clears stale refs from a previous extraction before re-tagging", () => {
    const d = doc(`<a href="/a">A</a><a href="/b">B</a><a href="/c">C</a>`);
    extractPageWithRefs(d); // tags e1..e3
    // Shrink the DOM: B disappears, so the new walk only tags 2 elements.
    d.body.querySelector(`[${REF_ATTR}="e2"]`)?.remove();

    const { refs } = extractPageWithRefs(d);
    expect(refs).toHaveLength(2);
    // The old "e3" tag must be gone — refElement may not match stale elements.
    expect(d.querySelector(`[${REF_ATTR}="e3"]`)).toBeNull();
    expect(refElement(d, "e3")).toBeNull();
    expect(refElement(d, "e2")?.textContent).toBe("C");
  });
});

describe("refElement", () => {
  it("resolves annotated elements and rejects invalid refs", () => {
    const d = doc(`<a href="https://example.com">Link</a><button>Go</button>`);
    const { refs } = extractPageWithRefs(d);
    expect(refs).toHaveLength(2);

    expect(refElement(d, "e1")).toBe(refs[0]);
    expect(refElement(d, "e2")).toBe(refs[1]);
    expect(refElement(d, "e0")).toBeNull();
    expect(refElement(d, "e99")).toBeNull();
    expect(refElement(d, "abc")).toBeNull();
  });
});
