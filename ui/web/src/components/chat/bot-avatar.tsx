import { cn } from "@/lib/utils";

/**
 * Assistant/bot avatar: the GoClaw mark served from /goclaw-icon.svg
 * (replaces the generic lucide Bot glyph in chat surfaces). Sized by the
 * caller via className; `rounded-full` keeps it consistent with the circular
 * avatar chips it renders inside.
 */
export function BotAvatar({ className }: { className?: string }) {
  return (
    <img
      src="/goclaw-icon.svg"
      alt=""
      aria-hidden="true"
      className={cn("shrink-0 object-contain", className)}
    />
  );
}
