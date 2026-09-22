import { parseSessionKey } from "@/lib/session-key";

/**
 * Origin resolution for the sessions list (user item 8).
 *
 * Every row shows an "Origin" badge:
 *  1. Channel-borne sessions (telegram/discord/whatsapp/zalo/feishu/…) →
 *     platform display name (existing secondary badge styling).
 *  2. Else, designer agents (session key's agent segment ends with
 *     "-designer") → distinct colored badges: pptx-designer → "PPTX",
 *     video-designer → "Video Editor" (i18n), other designers → agent key.
 *  3. Else, channel "ws" → "Web Chat" (i18n).
 *
 * The agent key is already embedded in the session key
 * ("agent:<agentKey>:<scope>", see internal/sessions/key.go), so no extra
 * backend field is required — parseSessionKey() extracts it.
 */

export type SessionOriginKind = "platform" | "designer" | "web";

export interface SessionOrigin {
  kind: SessionOriginKind;
  /** Literal (non-translated) label — platform names and "PPTX". */
  label?: string;
  /** i18n key (sessions namespace) when the label is translatable. */
  labelKey?: "origin.webChat" | "origin.videoEditor";
}

export function resolveSessionOrigin(session: {
  channel?: string;
  key: string;
}): SessionOrigin | null {
  const channel = (session.channel ?? "").trim().toLowerCase();

  // 1) Channel-borne sessions → platform name (any non-ws channel).
  if (channel && channel !== "ws") {
    return { kind: "platform", label: platformLabel(channel) };
  }

  // 2) Designer agents → studio badges.
  const { agentId } = parseSessionKey(session.key);
  if (agentId.endsWith("-designer")) {
    if (agentId === "pptx-designer") {
      return { kind: "designer", label: "PPTX" };
    }
    if (agentId === "video-designer") {
      return { kind: "designer", labelKey: "origin.videoEditor" };
    }
    return { kind: "designer", label: agentId };
  }

  // 3) Web chat.
  if (channel === "ws") {
    return { kind: "web", labelKey: "origin.webChat" };
  }
  return null;
}

function platformLabel(channel: string): string {
  switch (channel) {
    case "telegram":
      return "Telegram";
    case "discord":
      return "Discord";
    case "whatsapp":
      return "WhatsApp";
    case "zalo":
      return "Zalo";
    case "feishu":
      return "Feishu";
    default:
      return channel.charAt(0).toUpperCase() + channel.slice(1);
  }
}
