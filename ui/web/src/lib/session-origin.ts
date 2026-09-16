import { parseSessionKey } from "@/lib/session-key";

export type SessionOrigin =
  | "web"
  | "telegram"
  | "video"
  | "pptx"
  | "subagent"
  | "cron"
  | "team"
  | "heartbeat"
  | "channel";

/** Derive where a session came from: a studio surface (video/pptx), the web
 * chat, a messaging channel, or an internal runner (subagent/cron/team).
 * Mirrors internal/sessions/key.go scope formats. */
export function sessionOrigin(
  key: string,
  channel?: string | null,
): SessionOrigin {
  const { agentId, scope } = parseSessionKey(key);
  if (agentId === "video-designer") return "video";
  if (agentId === "pptx-designer") return "pptx";
  if (scope.startsWith("subagent:")) return "subagent";
  if (scope.startsWith("cron:")) return "cron";
  if (scope.startsWith("team:")) return "team";
  if (scope.startsWith("heartbeat")) return "heartbeat";
  if (channel && channel !== "ws") {
    if (channel === "telegram") return "telegram";
    return "channel";
  }
  return "web";
}
