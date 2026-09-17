/** Brand mark for agent avatars — the GoClaw claw icon served from
 * public/goclaw-icon.svg (embedded dist), used wherever a bot glyph used
 * to represent the assistant in chat surfaces. */
export function GoclawAvatar({ className }: { className?: string }) {
  return (
    <img
      src="/goclaw-icon.svg"
      alt="GoClaw"
      draggable={false}
      className={className ?? "h-4 w-4 object-contain"}
    />
  );
}
