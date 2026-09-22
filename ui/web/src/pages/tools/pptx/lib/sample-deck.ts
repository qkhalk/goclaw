import type { Deck } from "../types";

/**
 * Hand-crafted demo deck for the "Load sample deck" button. Exercises every
 * decoration primitive exactly as the pptx-designer contract documents them:
 * icon-chip + corner-bracket title, band + big-icon section, icon bullets,
 * icon stats, corner-frame quote with a slide icon, and the end slide.
 * Geometry values are stage px on the 1280×720 canvas.
 */
export function sampleDeck(): Deck {
  return {
    version: 1,
    theme: {
      background: "#0f172a",
      foreground: "#f8fafc",
      accent: "#38bdf8",
      muted: "#94a3b8",
      font_heading: "Arial",
      font_body: "Calibri",
    },
    slides: [
      {
        layout: "title",
        title: "GoClaw Product Overview",
        subtitle: "One gateway for every agent, channel and deck",
        icon: "rocket",
      },
      {
        layout: "section",
        title: "Why teams switch",
        subtitle: "Three forces behind the move",
        icon: "zap",
      },
      {
        layout: "bullets",
        title: "What ships out of the box",
        bullet_icons: ["users", "globe", "shield"],
        bullets: [
          "Multi-tenant workspaces with role-based access",
          "Telegram, WhatsApp and web channels in one place",
          "AES-encrypted keys and audited tool policies",
        ],
        notes: "Pick the pillar that matches the audience and expand there.",
      },
      {
        layout: "stats",
        title: "By the numbers",
        stats: [
          { value: "6+", label: "LLM providers", icon: "layers" },
          { value: "40", label: "built-in skills", icon: "package" },
          { value: "1", label: "gateway binary", icon: "zap" },
        ],
        notes: "Estimates for illustration — replace with the customer's real figures.",
      },
      {
        layout: "quote",
        quote: "The studio turned a blank outline into a client-ready deck before the call started.",
        author: "Sofia M., solutions lead",
        decor: [{ type: "frame", variant: "corner", x: 48, y: 48, w: 1184, h: 624 }],
      },
      {
        layout: "end",
        title: "Design yours next",
        subtitle: "Ask the designer for a deck, then tune every primitive by hand",
      },
    ],
  };
}
