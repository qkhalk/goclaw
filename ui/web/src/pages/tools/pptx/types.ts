/**
 * Deck model of the PPTX Studio. Mirrors the pptx-designer agent's ```deck
 * contract (internal/pptx/designer_agent.go) and the web-side validation in
 * lib/parse-deck-blocks.ts. Slides are 16:9; the studio renders them at a
 * 1280×720 stage and exports via pptxgenjs at 13.33×7.5in.
 */

export interface DeckTheme {
  background: string; // #RRGGBB
  foreground: string;
  accent: string;
  muted: string;
  font_heading: string;
  font_body: string;
}

export interface StatsItem {
  value: string;
  label: string;
}

export interface ColumnContent {
  heading: string;
  bullets: string[];
}

export type SlideLayout =
  | "title"
  | "section"
  | "bullets"
  | "two_column"
  | "quote"
  | "stats"
  | "image"
  | "end";

interface SlideBase {
  notes?: string;
}

export interface Slide extends SlideBase {
  layout: SlideLayout;
  title?: string;
  subtitle?: string;
  bullets?: string[];
  left?: ColumnContent;
  right?: ColumnContent;
  quote?: string;
  author?: string;
  stats?: StatsItem[];
  source?: string;
  caption?: string;
}

export interface Deck {
  version: 1;
  theme: DeckTheme;
  slides: Slide[];
}

/** PowerPoint-safe fonts the theme pickers offer (matches the designer agent). */
export const SAFE_FONTS = [
  "Arial",
  "Calibri",
  "Georgia",
  "Verdana",
  "Tahoma",
  "Trebuchet MS",
  "Times New Roman",
  "Courier New",
] as const;

/** Web fallback stacks so previews approximate the exported fonts. */
const FONT_STACKS: Record<string, string> = {
  Arial: "Arial, 'Liberation Sans', 'Helvetica Neue', sans-serif",
  Calibri: "Calibri, Carlito, 'Segoe UI', sans-serif",
  Georgia: "Georgia, 'Times New Roman', serif",
  Verdana: "Verdana, DejaVu Sans, sans-serif",
  Tahoma: "Tahoma, Verdana, sans-serif",
  "Trebuchet MS": "'Trebuchet MS', 'Segoe UI', sans-serif",
  "Times New Roman": "'Times New Roman', Times, serif",
  "Courier New": "'Courier New', monospace",
};

export function cssFont(font: string | undefined, heading = false): string {
  const key = heading ? font : font;
  return FONT_STACKS[key ?? ""] ?? "'Segoe UI', sans-serif";
}

export const DECK_LAYOUTS: SlideLayout[] = [
  "title",
  "section",
  "bullets",
  "two_column",
  "quote",
  "stats",
  "image",
  "end",
];

export const THEME_PRESETS: { key: string; theme: DeckTheme }[] = [
  {
    key: "deep-blue",
    theme: {
      background: "#0f172a",
      foreground: "#f8fafc",
      accent: "#38bdf8",
      muted: "#94a3b8",
      font_heading: "Arial",
      font_body: "Calibri",
    },
  },
  {
    key: "paper",
    theme: {
      background: "#faf9f6",
      foreground: "#1c1917",
      accent: "#b45309",
      muted: "#78716c",
      font_heading: "Georgia",
      font_body: "Verdana",
    },
  },
  {
    key: "forest",
    theme: {
      background: "#0b1f16",
      foreground: "#eef7f0",
      accent: "#4ade80",
      muted: "#8fae9b",
      font_heading: "Trebuchet MS",
      font_body: "Calibri",
    },
  },
  {
    key: "plum",
    theme: {
      background: "#1e1b2e",
      foreground: "#f5f3fa",
      accent: "#c084fc",
      muted: "#a29ec0",
      font_heading: "Times New Roman",
      font_body: "Tahoma",
    },
  },
  {
    key: "ink-teal",
    theme: {
      background: "#0c1d21",
      foreground: "#eaf6f6",
      accent: "#2dd4bf",
      muted: "#8fb3b0",
      font_heading: "Arial",
      font_body: "Georgia",
    },
  },
];

export function defaultDeck(): Deck {
  return {
    version: 1,
    theme: { ...THEME_PRESETS[0]!.theme },
    slides: [
      { layout: "title", title: "Presentation title", subtitle: "One-line promise" },
      {
        layout: "bullets",
        title: "First section",
        bullets: ["First point", "Second point", "Third point"],
      },
      { layout: "end", title: "Takeaway", subtitle: "What the audience should do next" },
    ],
  };
}

export function blankSlide(layout: SlideLayout): Slide {
  switch (layout) {
    case "title":
      return { layout, title: "Title" };
    case "section":
      return { layout, title: "Section" };
    case "bullets":
      return { layout, title: "Section", bullets: ["First point"] };
    case "two_column":
      return {
        layout,
        title: "Comparison",
        left: { heading: "Before", bullets: ["—"] },
        right: { heading: "After", bullets: ["—"] },
      };
    case "quote":
      return { layout, quote: "A sentence worth quoting", author: "Author, role" };
    case "stats":
      return { layout, title: "By the numbers", stats: [{ value: "99%", label: "metric" }] };
    case "image":
      return { layout, title: "Visual", source: "", caption: "" };
    case "end":
      return { layout, title: "Thank you", subtitle: "" };
  }
}
