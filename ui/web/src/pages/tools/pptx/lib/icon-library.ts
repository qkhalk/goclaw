/**
 * Curated line-icon vocabulary for the PPTX Studio. Every icon is a set of
 * SVG path strings on a 24×24 viewBox, stroke-based (fill "none"), designed
 * for strokeWidth 2 with round caps/joins (lucide-style geometry), so the
 * same path data renders identically in the HTML preview (inline <svg>) and
 * in the .pptx export (client-side rasterization to PNG).
 *
 * No external dependency, no remote fetch: the paths below are committed
 * data, and the deck JSON contract (designer agent + parse-deck-blocks)
 * validates icon names against ICON_NAMES.
 */

export interface LineIcon {
  name: string;
  paths: string[];
}

export const ICONS: LineIcon[] = [
  {
    name: "rocket",
    paths: [
      "M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z",
      "M12 15l-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z",
      "M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0",
      "M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5",
    ],
  },
  {
    name: "chart-bar",
    paths: ["M3 3v16a2 2 0 0 0 2 2h16", "M18 17V9", "M13 17V5", "M8 17v-3"],
  },
  {
    name: "cog",
    paths: [
      "M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z",
      "M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z",
    ],
  },
  { name: "cloud", paths: ["M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9z"] },
  {
    name: "shield",
    paths: [
      "M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z",
    ],
  },
  {
    name: "zap",
    paths: [
      "M4 14a1 1 0 0 1-.78-1.63l9.9-10.2a.5.5 0 0 1 .86.46l-1.92 6.02A1 1 0 0 0 13 10h7a1 1 0 0 1 .78 1.63l-9.9 10.2a.5.5 0 0 1-.86-.46l1.92-6.02A1 1 0 0 0 11 14z",
    ],
  },
  {
    name: "globe",
    paths: [
      "M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20z",
      "M2 12h20",
      "M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z",
    ],
  },
  {
    name: "users",
    paths: [
      "M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2",
      "M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z",
      "M22 21v-2a4 4 0 0 0-3-3.87",
      "M16 3.13a4 4 0 0 1 0 7.75",
    ],
  },
  {
    name: "briefcase",
    paths: [
      "M4 6h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2z",
      "M16 20V4a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16",
    ],
  },
  {
    name: "lightbulb",
    paths: [
      "M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1 .2 2.2 1.5 3.5.7.7 1.3 1.5 1.5 2.5",
      "M9 18h6",
      "M10 22h4",
    ],
  },
  {
    name: "target",
    paths: [
      "M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20z",
      "M12 18a6 6 0 1 0 0-12 6 6 0 0 0 0 12z",
      "M12 14a2 2 0 1 0 0-4 2 2 0 0 0 0 4z",
    ],
  },
  { name: "trending-up", paths: ["M22 7l-8.5 8.5-5-5L2 17", "M16 7h6v6"] },
  {
    name: "database",
    paths: [
      "M3 5c0-1.66 4.03-3 9-3s9 1.34 9 3-4.03 3-9 3-9-1.34-9-3z",
      "M3 5v14c0 1.66 4.03 3 9 3s9-1.34 9-3V5",
      "M3 12c0 1.66 4.03 3 9 3s9-1.34 9-3",
    ],
  },
  { name: "code", paths: ["M16 18l6-6-6-6", "M8 6l-6 6 6 6"] },
  {
    name: "check-circle",
    paths: ["M22 11.08V12a10 10 0 1 1-5.93-9.14", "M22 4L12 14.01l-3-3"],
  },
  {
    name: "star",
    paths: [
      "M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z",
    ],
  },
  {
    name: "heart",
    paths: [
      "M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7z",
    ],
  },
  {
    name: "calendar",
    paths: [
      "M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z",
      "M16 2v4",
      "M8 2v4",
      "M3 10h18",
    ],
  },
  {
    name: "mail",
    paths: [
      "M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z",
      "m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7",
    ],
  },
  {
    name: "phone",
    paths: [
      "M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72 12.84 12.84 0 0 0 .7 2.81 2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45 12.84 12.84 0 0 0 2.81.7A2 2 0 0 1 22 16.92z",
    ],
  },
  {
    name: "map-pin",
    paths: ["M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0z", "M12 13a3 3 0 1 0 0-6 3 3 0 0 0 0 6z"],
  },
  {
    name: "camera",
    paths: [
      "M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z",
      "M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z",
    ],
  },
  {
    name: "music",
    paths: ["M9 18V5l12-2v13", "M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6z", "M18 19a3 3 0 1 0 0-6 3 3 0 0 0 0 6z"],
  },
  {
    name: "book-open",
    paths: ["M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z", "M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"],
  },
  {
    name: "flag",
    paths: ["M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1z", "M4 22v-7"],
  },
  {
    name: "award",
    paths: ["M12 14a6 6 0 1 0 0-12 6 6 0 0 0 0 12z", "M15.477 12.89 17 22l-5-3-5 3 1.523-9.11"],
  },
  {
    name: "clock",
    paths: ["M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20z", "M12 6v6l4 2"],
  },
  {
    name: "layers",
    paths: [
      "M12 2 2 7l10 5 10-5-10-5z",
      "M2 17l10 5 10-5",
      "M2 12l10 5 10-5",
    ],
  },
  {
    name: "package",
    paths: [
      "M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z",
      "M3.3 7l8.7 5 8.7-5",
      "M12 22V12",
    ],
  },
  {
    name: "search",
    paths: ["M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16z", "m21 21-4.35-4.35"],
  },
  { name: "filter", paths: ["M22 3H2l8 9.46V19l4 2v-8.54z"] },
  { name: "arrow-right", paths: ["M5 12h14", "m12 5 7 7-7 7"] },
  { name: "play", paths: ["m6 3 14 9-14 9V3z"] },
  {
    name: "wifi",
    paths: ["M5 13a10 10 0 0 1 14 0", "M8.5 16.5a5 5 0 0 1 7 0", "M2 8.82a15 15 0 0 1 20 0", "M12 20h.01"],
  },
  {
    name: "lock",
    paths: [
      "M5 11h14a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2z",
      "M7 11V7a5 5 0 0 1 10 0v4",
    ],
  },
  {
    name: "eye",
    paths: [
      "M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z",
      "M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z",
    ],
  },
  { name: "message-circle", paths: ["M7.9 20A9 9 0 1 0 4 16.1L2 22z"] },
  {
    name: "thumbs-up",
    paths: [
      "M7 10v12",
      "M15 5.88 14 10h5.83a2 2 0 0 1 1.92 2.56l-2.33 8A2 2 0 0 1 17.5 22H4a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2h2.76a2 2 0 0 0 1.79-1.11L12 2a3.13 3.13 0 0 1 3 3.88z",
    ],
  },
  { name: "dollar-sign", paths: ["M12 2v20", "M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"] },
  {
    name: "percent",
    paths: ["M19 5 5 19", "M6.5 9a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z", "M17.5 20a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z"],
  },
];

/** Set of valid icon names — the deck JSON contract vocabulary. */
export const ICON_NAMES: ReadonlySet<string> = new Set(ICONS.map((i) => i.name));

/** O(1) lookup by name; unknown names yield undefined (render: skip silently). */
export function getIcon(name: string | undefined | null): LineIcon | undefined {
  if (!name) return undefined;
  return ICONS.find((i) => i.name === name);
}

/** Default icon stroke width in 24-unit viewBox coordinates. */
export const DEFAULT_ICON_STROKE_WIDTH = 2;

/**
 * Full standalone SVG markup for one icon, used by the .pptx export to
 * rasterize the vector shape into a PNG data URL (same geometry as the
 * preview's inline <svg>).
 */
export function iconSvgString(
  name: string,
  color: string,
  strokeWidth: number = DEFAULT_ICON_STROKE_WIDTH,
): string | null {
  const icon = getIcon(name);
  if (!icon) return null;
  const body = icon.paths.map((d) => `<path d="${d}"/>`).join("");
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24" ` +
    `fill="none" stroke="${color}" stroke-width="${strokeWidth}" ` +
    `stroke-linecap="round" stroke-linejoin="round">${body}</svg>`
  );
}
