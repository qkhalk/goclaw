/**
 * Avatar-emoji dedup for agent display names.
 *
 * The chat top bar, agent selector, agent cards and the detail header render
 * the configured avatar emoji next to the display name. When the name itself
 * starts with the same emoji ("🦊 Fox Spirit" with emoji "🦊"), the avatar
 * doubles ("🦊 🦊 Fox Spirit"). stripLeadingEmoji removes the duplicated
 * leading cluster so the emoji renders exactly once.
 */

/** Variation selector U+FE0F — ignorable when comparing emoji clusters. */
const VS_RE = /\uFE0F/g;

function firstGrapheme(s: string): string {
  const Seg = (Intl as unknown as {
    Segmenter?: new (locale: string, opts: { granularity: "grapheme" }) => {
      segment(input: string): Iterable<{ segment: string }>;
    };
  }).Segmenter;
  if (Seg) {
    for (const { segment } of new Seg("en", { granularity: "grapheme" }).segment(s)) {
      return segment;
    }
    return "";
  }
  return Array.from(s)[0] ?? "";
}

/** Returns `name` with a leading `emoji` cluster (and following spaces) removed when it matches. */
export function stripLeadingEmoji(
  emoji: string | null | undefined,
  name: string,
): string {
  const e = emoji?.trim();
  if (!e || !name) return name;
  const trimmed = name.trimStart();
  const first = firstGrapheme(trimmed);
  if (!first) return name;
  const same = first === e || first.replace(VS_RE, "") === e.replace(VS_RE, "");
  if (!same) return name;
  return trimmed.slice(first.length).trimStart();
}
