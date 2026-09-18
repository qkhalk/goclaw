import type { Scene } from "../hooks/use-timeline";

// ── Ready-made storyboard templates ──
//
// Each preset is a complete scene list that loads into the timeline through
// replaceScenes (one undo step, so applying is always reversible). Gradients
// and icon glyphs are browser-only effects; on a server render the gradient
// scenes degrade to their flat `color` fallback and icon scenes are skipped
// (the render panel says so).

export interface StoryboardTemplate {
  /** i18n key suffix: video.templates.<key> */
  key: string;
  /** Two colors summarizing the palette on the gallery card. */
  swatch: [string, string];
  scenes: Scene[];
}

type CaptionPosition = NonNullable<Scene["caption"]>["position"];

function caption(text: string, position: CaptionPosition = "bottom"): Scene["caption"] {
  return { text, position };
}

export const STORYBOARD_TEMPLATES: StoryboardTemplate[] = [
  {
    key: "tech_news",
    swatch: ["#0F172A", "#334155"],
    scenes: [
      {
        type: "icon",
        icon: { name: "zap", color: "#7DD3FC" },
        color: "#0F172A",
        gradient: { from: "#0F172A", to: "#1E293B" },
        duration_sec: 3,
        caption: caption("Tech News", "top"),
        narration: "Opening hook: what happened today.",
      },
      {
        type: "icon",
        icon: { name: "trending-up", color: "#F8FAFC" },
        color: "#1E293B",
        gradient: { from: "#1E293B", to: "#334155" },
        duration_sec: 4,
        transition: "crossfade",
        caption: caption("Markets moved fast this week"),
        narration: "One sentence on the biggest number.",
      },
      {
        type: "icon",
        icon: { name: "globe", color: "#7DD3FC" },
        color: "#334155",
        gradient: { from: "#334155", to: "#0F172A" },
        duration_sec: 4,
        transition: "crossfade",
        caption: caption("Three stories, thirty seconds"),
        narration: "Name the three stories.",
      },
      {
        type: "icon",
        icon: { name: "check", color: "#4ADE80" },
        color: "#052E16",
        gradient: { from: "#052E16", to: "#166534" },
        duration_sec: 3,
        transition: "fade",
        caption: caption("Follow for daily tech"),
      },
    ],
  },
  {
    key: "product_teaser",
    swatch: ["#1E1B2E", "#7E22CE"],
    scenes: [
      {
        type: "icon",
        icon: { name: "star", color: "#FDE68A" },
        color: "#1E1B2E",
        gradient: { from: "#1E1B2E", to: "#5B21B6" },
        duration_sec: 3,
        caption: caption("Something new is coming", "center"),
        narration: "One line of intrigue.",
      },
      {
        type: "icon",
        icon: { name: "gift", color: "#F5F3FA" },
        color: "#4C1D95",
        gradient: { from: "#4C1D95", to: "#7E22CE" },
        duration_sec: 3,
        transition: "slide_left",
        caption: caption("Launches Friday"),
        narration: "Name the product and the date.",
      },
      {
        type: "icon",
        icon: { name: "zap", color: "#FDE68A" },
        color: "#7E22CE",
        gradient: { from: "#7E22CE", to: "#A21CAF" },
        duration_sec: 3,
        transition: "slide_left",
        caption: caption("Limited first batch"),
      },
      {
        type: "icon",
        icon: { name: "bell", color: "#F5F3FA" },
        color: "#312E81",
        gradient: { from: "#312E81", to: "#1E1B2E" },
        duration_sec: 3,
        transition: "fade",
        caption: caption("Set a reminder"),
        narration: "Call to action.",
      },
    ],
  },
  {
    key: "quote_reel",
    swatch: ["#101014", "#2E2E38"],
    scenes: [
      {
        type: "color",
        color: "#101014",
        gradient: { from: "#101014", to: "#26262E" },
        duration_sec: 4,
        caption: caption("Simplicity is the ultimate sophistication.", "center"),
        narration: "Read the quote, slowly.",
      },
      {
        type: "color",
        color: "#1C1C24",
        gradient: { from: "#1C1C24", to: "#101014" },
        duration_sec: 3,
        transition: "crossfade",
        caption: caption("Leonardo da Vinci"),
      },
      {
        type: "color",
        color: "#101014",
        gradient: { from: "#101014", to: "#2E2E38" },
        duration_sec: 4,
        transition: "fade",
        caption: caption("Make it simple. But significant.", "center"),
        narration: "Second quote.",
      },
      {
        type: "color",
        color: "#1C1C24",
        gradient: { from: "#1C1C24", to: "#101014" },
        duration_sec: 3,
        transition: "crossfade",
        caption: caption("Don Draper"),
      },
      {
        type: "icon",
        icon: { name: "music", color: "#E7E5E4" },
        color: "#101014",
        gradient: { from: "#101014", to: "#26262E" },
        duration_sec: 3,
        transition: "fade",
        caption: caption("Quote Reel"),
      },
    ],
  },
  {
    key: "photo_story",
    swatch: ["#44403C", "#A8A29E"],
    scenes: [
      {
        type: "image",
        source: "https://picsum.photos/seed/goclaw-story-morning/1080/1920",
        duration_sec: 4,
        fit: "cover",
        ken_burns: { zoom_from: 1, zoom_to: 1.12, pan: "none" },
        caption: caption("7:04. First light."),
        narration: "Set the scene.",
      },
      {
        type: "image",
        source: "https://picsum.photos/seed/goclaw-story-commute/1080/1920",
        duration_sec: 4,
        fit: "cover",
        transition: "crossfade",
        ken_burns: { zoom_from: 1.12, zoom_to: 1, pan: "none" },
        caption: caption("The long way in."),
      },
      {
        type: "image",
        source: "https://picsum.photos/seed/goclaw-story-desk/1080/1920",
        duration_sec: 4,
        fit: "cover",
        transition: "crossfade",
        ken_burns: { zoom_from: 1, zoom_to: 1.12, pan: "none" },
        caption: caption("Deep work hours."),
      },
      {
        type: "image",
        source: "https://picsum.photos/seed/goclaw-story-dusk/1080/1920",
        duration_sec: 4,
        fit: "cover",
        transition: "fade",
        ken_burns: { zoom_from: 1.12, zoom_to: 1, pan: "none" },
        caption: caption("Done for today."),
        narration: "Close the story.",
      },
    ],
  },
  {
    key: "tips_reel",
    swatch: ["#042F2E", "#0D9488"],
    scenes: [
      {
        type: "icon",
        icon: { name: "clock", color: "#5EEAD4" },
        color: "#042F2E",
        gradient: { from: "#042F2E", to: "#134E4A" },
        duration_sec: 3,
        caption: caption("3 tips to ship faster", "top"),
        narration: "Promise the payoff.",
      },
      {
        type: "icon",
        icon: { name: "check", color: "#F0FDFA" },
        color: "#134E4A",
        gradient: { from: "#134E4A", to: "#0F766E" },
        duration_sec: 4,
        transition: "slide_up",
        caption: caption("1. Write the release note first"),
        narration: "Tip one, one sentence.",
      },
      {
        type: "icon",
        icon: { name: "flame", color: "#FDE68A" },
        color: "#0F766E",
        gradient: { from: "#0F766E", to: "#0D9488" },
        duration_sec: 4,
        transition: "slide_up",
        caption: caption("2. Cut scope, not quality"),
        narration: "Tip two.",
      },
      {
        type: "icon",
        icon: { name: "bookmark", color: "#5EEAD4" },
        color: "#042F2E",
        gradient: { from: "#0D9488", to: "#042F2E" },
        duration_sec: 3,
        transition: "fade",
        caption: caption("3. Ship on Thursday"),
      },
      {
        type: "icon",
        icon: { name: "star", color: "#FDE68A" },
        color: "#134E4A",
        gradient: { from: "#134E4A", to: "#042F2E" },
        duration_sec: 3,
        transition: "crossfade",
        caption: caption("Save this for your next launch"),
        narration: "Call to action.",
      },
    ],
  },
];
