import { describe, expect, it } from "vitest";
import { parseSkillsPageState, serializeSkillsPageState } from "./skills-page-state";

describe("skills page state", () => {
  it("uses quiet defaults for empty params", () => {
    expect(parseSkillsPageState(new URLSearchParams())).toEqual({
      tab: "core",
      q: "",
      filter: "all",
      sort: "name",
      agent: null,
    });
  });

  it("rejects invalid params", () => {
    const state = parseSkillsPageState(new URLSearchParams("tab=bad&filter=weird&sort=old&q=%20pdf%20"));
    expect(state).toMatchObject({ tab: "core", filter: "all", sort: "name", q: "pdf" });
  });

  it("parses the market tab", () => {
    expect(parseSkillsPageState(new URLSearchParams("tab=market")).tab).toBe("market");
  });

  it("serializes non-default filters while preserving modal params", () => {
    const params = new URLSearchParams("skill=abc&detailTab=evolution&file=SKILL.md");
    const next = serializeSkillsPageState(params, { tab: "custom", q: "pdf", filter: "missing-deps", sort: "deps" });
    expect(next.get("tab")).toBe("custom");
    expect(next.get("q")).toBe("pdf");
    expect(next.get("filter")).toBe("missing-deps");
    expect(next.get("sort")).toBe("deps");
    expect(next.get("skill")).toBe("abc");
    expect(next.get("detailTab")).toBe("evolution");
    expect(next.get("file")).toBe("SKILL.md");
  });

  it("serializes the market tab into the url", () => {
    const next = serializeSkillsPageState(new URLSearchParams(), { tab: "market" });
    expect(next.get("tab")).toBe("market");
  });

  it("removes default values instead of making noisy URLs", () => {
    const params = new URLSearchParams("tab=custom&q=pdf&filter=attention&sort=deps&agent=a1");
    const next = serializeSkillsPageState(params, { tab: "core", q: "", filter: "all", sort: "name", agent: null });
    expect(next.toString()).toBe("");
  });
});
