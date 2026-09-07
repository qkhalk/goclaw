import { describe, expect, it } from "vitest";
import { stripLeadingEmoji } from "./agent-emoji";

describe("stripLeadingEmoji", () => {
  it("removes the avatar emoji when the display name starts with it", () => {
    expect(stripLeadingEmoji("🦊", "🦊 Fox Spirit")).toBe("Fox Spirit");
  });

  it("removes the emoji even without a following space", () => {
    expect(stripLeadingEmoji("🤖", "🤖Helper")).toBe("Helper");
  });

  it("keeps the name when it does not start with the emoji", () => {
    expect(stripLeadingEmoji("🦊", "Fox Spirit")).toBe("Fox Spirit");
    expect(stripLeadingEmoji("🦊", "Team 🦊 Spirit")).toBe("Team 🦊 Spirit");
  });

  it("keeps the name when no avatar emoji exists", () => {
    expect(stripLeadingEmoji(undefined, "🦊 Fox Spirit")).toBe("🦊 Fox Spirit");
    expect(stripLeadingEmoji("", "🦊 Fox Spirit")).toBe("🦊 Fox Spirit");
  });

  it("tolerates variation-selector differences", () => {
    expect(stripLeadingEmoji("⚠️", "⚠️ Ops Agent")).toBe("Ops Agent");
    expect(stripLeadingEmoji("⚠", "⚠️ Ops Agent")).toBe("Ops Agent");
    expect(stripLeadingEmoji("⚠️", "⚠ Ops Agent")).toBe("Ops Agent");
  });

  it("matches ZWJ family emoji as a single cluster", () => {
    const family = "👨‍👩‍👧‍👦 Team";
    expect(stripLeadingEmoji("👨‍👩‍👧‍👦", family)).toBe("Team");
  });

  it("only strips one leading occurrence", () => {
    expect(stripLeadingEmoji("🦊", "🦊🦊 Double")).toBe("🦊 Double");
  });

  it("returns the original name for empty input", () => {
    expect(stripLeadingEmoji("🦊", "")).toBe("");
  });
});
