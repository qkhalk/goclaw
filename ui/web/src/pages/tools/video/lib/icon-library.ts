// ── Bundled line-icon library for `icon` storyboard scenes ──
//
// One icon family: minimal 24x24 stroke glyphs (2px, round caps/joins, no
// fill), drawn on canvas via Path2D and in the picker as inline SVG. No
// external assets, so icons render identically in the canvas preview and the
// client-side export.

export interface VideoIconDef {
  /** Stable identifier used in storyboard JSON (scene.icon.name). */
  name: string;
  /** SVG path data in a 24x24 viewBox. Stroked, never filled. */
  d: string[];
}

export const ICON_LIBRARY: VideoIconDef[] = [
  {
    name: "play",
    d: ["M6 4.5 20 12 6 19.5Z"],
  },
  {
    name: "zap",
    d: ["M13 2 3 14h9l-1 8 10-12h-9l1-8z"],
  },
  {
    name: "heart",
    d: [
      "M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z",
    ],
  },
  {
    name: "star",
    d: ["M12 2l3.09 6.26L22 9.27l-5 4.87L18.18 21 12 17.77 5.82 21 7 14.14 2 9.27l6.91-1.01L12 2z"],
  },
  {
    name: "bell",
    d: ["M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9", "M10.3 21a1.94 1.94 0 0 0 3.4 0"],
  },
  {
    name: "check",
    d: ["M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0", "m9 12 2 2 4-4"],
  },
  {
    name: "x",
    d: ["M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0", "m15 9-6 6", "m9 9 6 6"],
  },
  {
    name: "info",
    d: ["M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0", "M12 16v-4", "M12 8h.01"],
  },
  {
    name: "alert",
    d: [
      "m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3",
      "M12 9v4",
      "M12 17h.01",
    ],
  },
  {
    name: "user",
    d: ["M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2", "M16 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0"],
  },
  {
    name: "users",
    d: [
      "M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2",
      "M13 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0",
      "M22 21v-2a4 4 0 0 0-3-3.87",
      "M16 3.13a4 4 0 0 1 0 7.75",
    ],
  },
  {
    name: "home",
    d: ["m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z", "M9 22V12h6v10"],
  },
  {
    name: "search",
    d: ["M19 11a8 8 0 1 1-16 0 8 8 0 0 1 16 0", "m21 21-4.3-4.3"],
  },
  {
    name: "settings",
    d: [
      "M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z",
      "M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0",
    ],
  },
  {
    name: "mail",
    d: [
      "M2 6a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2z",
      "m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7",
    ],
  },
  {
    name: "calendar",
    d: [
      "M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z",
      "M8 2v4",
      "M16 2v4",
      "M3 10h18",
    ],
  },
  {
    name: "clock",
    d: ["M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0", "M12 6v6l4 2"],
  },
  {
    name: "map-pin",
    d: ["M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z", "M15 10a3 3 0 1 1-6 0 3 3 0 0 1 6 0"],
  },
  {
    name: "music",
    d: ["M9 18V5l12-2v13", "M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0", "M21 16a3 3 0 1 1-6 0 3 3 0 0 1 6 0"],
  },
  {
    name: "video",
    d: [
      "m16 13 5.223 3.482a.5.5 0 0 0 .777-.416V7.87a.5.5 0 0 0-.752-.432L16 10.5",
      "M4 6h10a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2z",
    ],
  },
  {
    name: "camera",
    d: [
      "M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z",
      "M15 13a3 3 0 1 1-6 0 3 3 0 0 1 6 0",
    ],
  },
  {
    name: "mic",
    d: [
      "M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z",
      "M19 10v2a7 7 0 0 1-14 0v-2",
      "M12 19v3",
    ],
  },
  {
    name: "cloud",
    d: ["M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"],
  },
  {
    name: "sun",
    d: [
      "M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0",
      "M12 2v2",
      "M12 20v2",
      "m4.93 4.93 1.41 1.41",
      "m17.66 17.66 1.41 1.41",
      "M2 12h2",
      "M20 12h2",
      "m6.34 17.66-1.41 1.41",
      "m19.07 4.93-1.41 1.41",
    ],
  },
  {
    name: "moon",
    d: ["M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"],
  },
  {
    name: "flame",
    d: [
      "M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.072-2.143-.224-4.054 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.153.433-2.294 1-3a2.5 2.5 0 0 0 2.5 2.5z",
    ],
  },
  {
    name: "globe",
    d: [
      "M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0",
      "M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20",
      "M2 12h20",
    ],
  },
  {
    name: "bookmark",
    d: ["m19 21-7-4-7 4V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v16z"],
  },
  {
    name: "trending-up",
    d: ["m22 7-8.5 8.5-5-5L2 17", "M16 7h6v6"],
  },
  {
    name: "gift",
    d: [
      "M3 8h18a1 1 0 0 1 1 1v2a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V9a1 1 0 0 1 1-1z",
      "M12 8v13",
      "M19 12v7a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2v-7",
      "M7.5 8a2.5 2.5 0 0 1 0-5C11 3 12 8 12 8s1-5 4.5-5a2.5 2.5 0 0 1 0 5",
    ],
  },
];

const ICON_MAP: Map<string, VideoIconDef> = new Map(ICON_LIBRARY.map((icon) => [icon.name, icon]));

export function getIconDef(name: string | undefined): VideoIconDef | undefined {
  if (!name) return undefined;
  return ICON_MAP.get(name);
}

/** Whitelist used by the storyboard parser (scene.icon.name must match). */
export const ICON_NAMES: ReadonlySet<string> = new Set(ICON_LIBRARY.map((icon) => icon.name));
